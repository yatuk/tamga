package proxy

import (
	"errors"
	"strconv"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
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
	for _, known := range []error{errStripRaw, errStripUnplaced, errStripUnmaskable, errStripUnlocated} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return redactRewrite
}

// stripSegments applies STRIP: every segment that holds a finding whose own
// action is STRIP is replaced whole by the placeholder.
//
// STRIP being the verdict does not excuse the rest of the request. A
// finding elsewhere whose rule says REDACT is still redacted, and if that
// cannot be done the caller blocks, as it would have without the strip.
func stripSegments(provider string, body []byte, segs []extract.Segment, findings []scanner.Finding,
	actionOf func(scanner.Finding) policy.Action, vaultOn bool) (stripResult, error) {
	var out stripResult
	if segs == nil {
		return out, errStripRaw
	}

	strip := make(map[int]bool)
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
		}
	}
	if len(strip) == 0 {
		// STRIP was the verdict and no finding says where: nothing would be
		// taken out, and the request would go on as it came.
		return out, errStripUnlocated
	}

	// What is left to redact: findings outside the stripped segments, when
	// any of them is under a REDACT rule.
	var rest []scanner.Finding
	needRedact := false
	for _, f := range findings {
		if strip[f.Seg] {
			continue
		}
		rest = append(rest, f)
		if actionOf(f) == policy.ActionRedact {
			needRedact = true
		}
	}

	var edits []extract.Edit
	for seg := range strip {
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
