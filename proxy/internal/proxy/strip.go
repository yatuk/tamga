package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// stripPlaceholder is what the model reads in place of a stripped piece of
// text. It says that something was taken out, so the model does not treat
// an empty tool result as the tool's answer.
const stripPlaceholder = "[Content removed by Tamga security policy.]"

// stripResult is a request body with the stripped segments replaced.
type stripResult struct {
	body     []byte
	stripped int
	redacted int
	mapping  map[string]string
	// edited lists the segments the rewrite changed, by index.
	edited []int
	// fields counts the strings of JSON tool results that were stripped
	// in place of their whole text (scan.strip_scope: field).
	fields int
}

var (
	errStripRaw        = errors.New("raw")        // the body was not read by segment
	errStripUnplaced   = errors.New("unplaced")   // a value to redact has no position
	errStripUnmaskable = errors.New("unmaskable") // a finding to redact is not a span of text
	errStripUnlocated  = errors.New("unlocated")  // a finding to strip is tied to no piece of text
)

// stripFailure names why a STRIP could not be carried out, for the
// X-Tamga-Strip-Fallback header and the degraded counter.
func stripFailure(err error) string {
	if errors.Is(err, extract.ErrExtracted) {
		return redactAttachment
	}
	for _, known := range []error{errStripRaw, errStripUnplaced, errStripUnmaskable, errStripUnlocated} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return redactRewrite
}

// stripSegments applies STRIP: every segment that holds a finding whose own
// action is STRIP is replaced whole by the placeholder. With fields set
// (scan.strip_scope: field), a tool result that is JSON keeps everything
// but the strings that hold what is stripped (fieldStripper).
//
// STRIP being the verdict does not excuse the rest of the request. A
// finding elsewhere whose rule says REDACT is still redacted, and if that
// cannot be done the caller blocks, as it would have without the strip.
func stripSegments(provider string, body []byte, segs []extract.Segment, findings []scanner.Finding,
	actionOf func(scanner.Finding) policy.Action, vaultOn bool, fields *fieldStripper) (stripResult, error) {
	var out stripResult
	if segs == nil {
		return out, errStripRaw
	}

	strip := make(map[int]bool)
	// spanned are the segments an instruction split over neighbouring
	// pieces reaches: they are taken out whole.
	spanned := make(map[int]bool)
	for _, f := range findings {
		if actionOf(f) != policy.ActionStrip {
			continue
		}
		// The finding has to say which piece of text it is in. One that was
		// put back on the segments from a joined text and fits none of them
		// (scanner.AssignSegments) carries no path, and its segment index is
		// only a zero: stripping that segment would take out the wrong text
		// and send the right one on.
		if f.Path == "" || f.Seg < 0 || f.Seg >= len(segs) {
			return out, errStripUnlocated
		}
		// An instruction split over neighbouring pieces is in all of them.
		last := f.Seg
		if n, err := strconv.Atoi(f.Metadata["spans_segments"]); err == nil && n > 1 {
			last = f.Seg + n - 1
		}
		for seg := f.Seg; seg <= last && seg < len(segs); seg++ {
			strip[seg] = true
			if last > f.Seg {
				spanned[seg] = true
			}
		}
	}
	if len(strip) == 0 {
		// STRIP was the verdict and no finding says where: nothing would be
		// taken out, and the request would go on as it came.
		return out, errStripUnlocated
	}

	// The tool results that keep all but some of their strings.
	narrowed := map[int][]extract.Edit{}
	if fields != nil {
		for seg := range strip {
			if spanned[seg] {
				continue
			}
			if e, ok := fields.edits(seg, segs[seg], findings, actionOf); ok {
				narrowed[seg] = e
			}
		}
	}
	// A finding that reaches into a stripped string from outside it cannot
	// be masked apart from it: that tool result is stripped whole.
	for _, f := range findings {
		if e, ok := narrowed[f.Seg]; ok && actionOf(f) != policy.ActionStrip {
			if _, crosses := withinEdits(f, e); crosses {
				delete(narrowed, f.Seg)
			}
		}
	}

	// What is left to redact: findings outside what is stripped, when any
	// of them is under a REDACT rule. In a narrowed tool result that is
	// what lies outside its stripped strings.
	var rest []scanner.Finding
	needRedact := false
	for _, f := range findings {
		if strip[f.Seg] {
			e, ok := narrowed[f.Seg]
			if !ok || actionOf(f) == policy.ActionStrip {
				continue
			}
			if inside, _ := withinEdits(f, e); inside {
				continue
			}
		}
		rest = append(rest, f)
		if actionOf(f) == policy.ActionRedact {
			needRedact = true
		}
	}

	var edits []extract.Edit
	for seg := range strip {
		if e, ok := narrowed[seg]; ok {
			edits = append(edits, e...)
			out.fields += len(e)
			continue
		}
		edits = append(edits, extract.Edit{Seg: seg, From: 0, To: len(segs[seg].Text), Replacement: stripPlaceholder})
	}
	if needRedact {
		// The same findings a REDACT verdict would mask, on the same terms.
		targets := redactTargets(rest, actionOf)
		switch redactBlocker(targets) {
		case redactUnplaced:
			return out, errStripUnplaced
		case redactUnmaskable:
			return out, errStripUnmaskable
		}
		more, mapping := maskEdits(segs, targets, vaultOn)
		edits = append(edits, more...)
		out.mapping = mapping
		out.redacted = scanner.PlacedRedactions(targets)
	}

	rewritten, err := extract.RewriteChecked(provider, body, segs, edits)
	if err != nil {
		return stripResult{}, err
	}
	out.body = rewritten
	out.stripped = len(strip)
	for _, e := range edits {
		out.edited = append(out.edited, e.Seg)
	}
	return out, nil
}

// withinEdits reports whether a finding lies inside one of the edits, and
// whether it crosses the edge of one. A finding without a position is in
// neither: it is about the text, and the text stays.
func withinEdits(f scanner.Finding, edits []extract.Edit) (inside, crosses bool) {
	if !f.Placed() {
		return false, false
	}
	for _, e := range edits {
		switch {
		case e.From <= f.StartPos && f.EndPos <= e.To:
			return true, false
		case f.StartPos < e.To && e.From < f.EndPos:
			crosses = true
		}
	}
	return false, crosses
}

// newFieldStripper returns what narrows STRIP for a policy whose
// scan.strip_scope is field, and nil for one that strips whole pieces of
// text. The tool results are scanned again with reg, as the request was.
func newFieldStripper(ctx context.Context, pol *policy.Policy, reg *scanner.Registry, cfg scanner.PipelineConfig) *fieldStripper {
	if !pol.StripFields() || reg == nil {
		return nil
	}
	return &fieldStripper{rescan: func(segs []extract.Segment) ([]scanner.Finding, error) {
		return reg.ScanSegments(ctx, segs, cfg)
	}}
}

// fieldStripper narrows STRIP in a tool result that is JSON to the strings
// that hold what is stripped (scan.strip_scope: field).
//
// It reads the tool result in the units the scanners read it in
// (normalize.JSONUnits: the strings of one object, of one array). A unit
// that holds what the policy strips loses those of its strings that hold it
// alone; when none does alone, as with an instruction split over a name and
// a description, the unit loses all its strings. Each is replaced by the
// placeholder inside its quotes, so the tool result is still JSON, with its
// keys, its numbers and its other records as they were. The tool result is
// then scanned again as it would be sent; if anything in it is still
// stripped, or it is no longer JSON, it is stripped whole, as without this
// setting.
//
// Only what the scanners find again can be narrowed. A tool result held by
// the classifier, a banned topic or the language check, or by an
// instruction that reaches it from a neighbouring segment, is stripped
// whole.
type fieldStripper struct {
	// rescan scans segments with the proxy's own scanners.
	rescan func([]extract.Segment) ([]scanner.Finding, error)
}

// narrowable reports whether a finding is one the scanners find again in
// the strings of the tool result.
func narrowable(f scanner.Finding) bool {
	return (f.Type == "injection" || f.Type == "jailbreak") && f.Category != "classifier"
}

// edits returns the edits that strip the strings of segment idx that hold
// what is stripped, and false when the segment is to be stripped whole.
func (fs *fieldStripper) edits(idx int, seg extract.Segment, findings []scanner.Finding, actionOf func(scanner.Finding) policy.Action) ([]extract.Edit, bool) {
	if fs == nil || fs.rescan == nil || seg.Role != extract.RoleTool {
		return nil, false
	}
	for _, f := range findings {
		if f.Seg == idx && actionOf(f) == policy.ActionStrip && !narrowable(f) {
			return nil, false
		}
	}
	units := normalize.JSONUnits(seg.Text)
	if units == nil {
		return nil, false
	}
	failed := false
	holds := func(text string) bool {
		one := seg
		one.Text = text
		found, err := fs.rescan([]extract.Segment{one})
		if err != nil {
			failed = true
			return true
		}
		for _, f := range found {
			if actionOf(f) == policy.ActionStrip {
				return true
			}
		}
		return false
	}
	var edits []extract.Edit
	for _, u := range units {
		if !holds(u.View.Text) {
			continue
		}
		var chosen []normalize.JSONField
		if len(u.Fields) > 1 {
			for _, field := range u.Fields {
				if holds(field.View.Text) {
					chosen = append(chosen, field)
				}
			}
		}
		if len(chosen) == 0 {
			chosen = u.Fields
		}
		for _, field := range chosen {
			edits = append(edits, extract.Edit{Seg: idx, From: field.From, To: field.To, Replacement: stripPlaceholder})
		}
	}
	if failed || len(edits) == 0 {
		return nil, false
	}
	after := extract.ApplyEdits(seg.Text, edits)
	if !json.Valid([]byte(after)) || holds(after) || failed {
		return nil, false
	}
	return edits, true
}
