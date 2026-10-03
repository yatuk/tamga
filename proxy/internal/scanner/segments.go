package scanner

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/yatuk/tamga/internal/extract"
)

// ScanSegments runs the registered scanners over each segment of a request
// and returns the findings with the role and path of the text they were
// found in.
//
// A finding's StartPos and EndPos are offsets into its segment's Text, and
// only set when the match was found in the text as written. A match found
// only in a normalised view (folded letters, spelled-out digits) has no
// position: the scanners know it is there, not where, so it can be blocked
// but not cut out.
//
// The same value is reported once per segment, however many views it was
// found in.
func (r *Registry) ScanSegments(ctx context.Context, segs []extract.Segment, cfg PipelineConfig) ([]Finding, error) {
	var all []Finding
	perSeg, firstErr := r.scanUnits(ctx, segs, cfg)
	for i := range perSeg {
		perSeg[i] = dedupeFindings(perSeg[i])
		all = append(all, perSeg[i]...)
	}

	all = append(all, r.scanAcrossSegments(ctx, segs, perSeg)...)
	return all, firstErr
}

// dropEchoes removes the unpositioned PII findings that are only another
// view of a positioned one.
//
// The scanners read the text as written and several transformed views of it
// (folded letters, separators removed, digits reversed, ROT13). A value in
// the text is therefore found once with a position and again, without one,
// in every view that still looks like it — a ROT13'd e-mail address is
// e-mail shaped. Those are not further values, and treating them as values
// that cannot be located would make every redaction impossible.
//
// Telling them apart by comparing matches is guesswork. Instead the
// positioned values are blanked out and the text is scanned again: whatever
// is still found was never an echo.
func (r *Registry) dropEchoes(ctx context.Context, found []Finding, index int, seg extract.Segment, cfg PipelineConfig) []Finding {
	placed, floating := 0, 0
	for _, f := range found {
		if !redactable(f) {
			continue
		}
		if f.Placed() {
			placed++
		} else {
			floating++
		}
	}
	if placed == 0 || floating == 0 {
		return found
	}

	blanked := []byte(seg.Text)
	for _, f := range found {
		if redactable(f) && f.Placed() {
			for k := f.StartPos; k < f.EndPos; k++ {
				blanked[k] = '#'
			}
		}
	}
	again, err := r.ScanAllWithConfig(ctx, blanked, cfg)
	if err != nil {
		// Without the second scan the echoes cannot be told from real
		// values; keeping them all is the safe reading.
		return found
	}

	out := found[:0:0]
	for _, f := range found {
		if !redactable(f) || f.Placed() {
			out = append(out, f)
		}
	}
	for j := range again {
		if redactable(again[j]) {
			placeFinding(&again[j], index, seg)
			out = append(out, again[j])
		}
	}
	return out
}

// placeFinding ties a finding to its segment and settles whether its
// position can be trusted.
func placeFinding(f *Finding, index int, seg extract.Segment) {
	f.Seg = index
	f.Role = string(seg.Role)
	f.Path = seg.Path
	if f.Metadata["detected_via"] != "" || f.StartPos < 0 || f.StartPos >= f.EndPos || f.EndPos > len(seg.Text) {
		f.StartPos, f.EndPos = 0, 0
	}
}

// Placed reports whether the finding has a position in its segment's text.
func (f Finding) Placed() bool { return f.EndPos > f.StartPos }

// dedupeFindings collapses the findings of one segment that describe the
// same value.
//
// Positioned findings of one category follow the rules Presidio uses: on the
// same span the higher confidence stays; when one span contains another the
// longer stays; spans that only intersect both stay.
//
// A finding without a position is an echo when a positioned finding of the
// same category masks to the same value — the scanner found one number in
// both the raw and a normalised view. Reversed-digit findings are echoes when
// there are no more of them than positioned ones. What is left without a
// position is a value that only a normalised view revealed.
func dedupeFindings(in []Finding) []Finding {
	if len(in) < 2 {
		return in
	}
	type group struct{ typ, cat string }
	order := []group{}
	groups := map[group][]Finding{}
	for _, f := range in {
		g := group{f.Type, f.Category}
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], f)
	}

	var out []Finding
	for _, g := range order {
		var placed, floating []Finding
		for _, f := range groups[g] {
			if f.Placed() {
				placed = append(placed, f)
			} else {
				floating = append(floating, f)
			}
		}
		placed = dedupePlaced(placed)

		sigs := map[string]bool{}
		for _, p := range placed {
			sigs[maskSignature(p.Match)] = true
		}
		reversed := 0
		for _, f := range floating {
			if f.Metadata["detected_via"] == "reversed_digits" {
				reversed++
			}
		}
		dropped := 0
		seen := map[string]bool{}
		var kept []Finding
		for _, f := range floating {
			sig := maskSignature(f.Match)
			switch {
			case sigs[sig]:
				dropped++
			case f.Metadata["detected_via"] == "reversed_digits" && len(placed) > 0 && reversed <= len(placed):
				dropped++
			case seen[sig]:
				dropped++
			default:
				seen[sig] = true
				kept = append(kept, f)
			}
		}
		if dropped > 0 && len(placed) > 0 {
			if placed[0].Metadata == nil {
				placed[0].Metadata = map[string]string{}
			} else {
				placed[0].Metadata = copyMeta(placed[0].Metadata)
			}
			placed[0].Metadata["variants"] = strconv.Itoa(dropped)
		}
		out = append(out, placed...)
		out = append(out, kept...)
	}
	return out
}

func dedupePlaced(in []Finding) []Finding {
	if len(in) < 2 {
		return in
	}
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].StartPos != in[j].StartPos {
			return in[i].StartPos < in[j].StartPos
		}
		return in[i].EndPos > in[j].EndPos
	})
	var out []Finding
	for _, f := range in {
		merged := false
		for k := range out {
			o := out[k]
			switch {
			case o.StartPos == f.StartPos && o.EndPos == f.EndPos:
				if f.Confidence > o.Confidence {
					out[k] = f
				}
				merged = true
			case o.StartPos <= f.StartPos && f.EndPos <= o.EndPos:
				merged = true // contained in a longer span
			case f.StartPos <= o.StartPos && o.EndPos <= f.EndPos:
				out[k] = f
				merged = true
			}
			if merged {
				break
			}
		}
		if !merged {
			out = append(out, f)
		}
	}
	return out
}

// maskSignature reduces a masked match to what identifies the value behind
// it — its first and last characters and its length — ignoring separators,
// so "+9* *** *** ** 67" and "+9*********67" compare equal.
func maskSignature(match string) string {
	var b strings.Builder
	for _, r := range match {
		if r == '*' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func copyMeta(m map[string]string) map[string]string {
	out := make(map[string]string, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// crossSegmentScanners are the scanners whose patterns can be split over
// neighbouring pieces of text: instructions, not identifiers.
var crossSegmentScanners = map[string]bool{"injection": true, "jailbreak": true}

// scanAcrossSegments looks for instructions split over neighbouring segments
// of the same role — "ignore all previous" in one content part and
// "instructions" in the next. Each run of such segments is joined and given
// to the injection scanners; a category already found inside one of the
// segments is not reported again.
func (r *Registry) scanAcrossSegments(ctx context.Context, segs []extract.Segment, perSeg [][]Finding) []Finding {
	r.mu.RLock()
	var scanners []Scanner
	for _, s := range r.scanners {
		if crossSegmentScanners[s.Name()] {
			scanners = append(scanners, s)
		}
	}
	r.mu.RUnlock()
	if len(scanners) == 0 {
		return nil
	}

	var out []Finding
	for start := 0; start < len(segs); {
		end := start + 1
		for end < len(segs) && segs[end].Role == segs[start].Role {
			end++
		}
		if end-start >= 2 {
			known := map[string]bool{}
			parts := make([]string, 0, end-start)
			for i := start; i < end; i++ {
				parts = append(parts, segs[i].Text)
				for _, f := range perSeg[i] {
					known[f.Type+"/"+f.Category] = true
				}
			}
			joined := []byte(strings.Join(parts, "\n"))
			for _, s := range scanners {
				found, err := s.Scan(ctx, joined)
				if err != nil {
					continue
				}
				for _, f := range found {
					key := f.Type + "/" + f.Category
					if known[key] {
						continue
					}
					known[key] = true
					f.Seg = start
					f.Role = string(segs[start].Role)
					f.Path = segs[start].Path
					f.StartPos, f.EndPos = 0, 0
					if f.Metadata == nil {
						f.Metadata = map[string]string{}
					} else {
						f.Metadata = copyMeta(f.Metadata)
					}
					f.Metadata["spans_segments"] = strconv.Itoa(end - start)
					out = append(out, f)
				}
			}
		}
		start = end
	}
	return out
}

// redactable reports whether a finding is one that REDACT removes.
func redactable(f Finding) bool { return f.Type == "pii" || f.Type == "custom" }

// RedactEdits turns the positioned PII findings of a segment scan into the
// edits that mask them.
func RedactEdits(findings []Finding) []extract.Edit {
	var edits []extract.Edit
	for _, f := range findings {
		if redactable(f) && f.Placed() {
			edits = append(edits, extract.Edit{Seg: f.Seg, From: f.StartPos, To: f.EndPos, Replacement: "[" + f.Category + "_REDACTED]"})
		}
	}
	return edits
}

// UnplacedRedactions counts the PII findings that have no position and so
// cannot be masked. A request with any of them cannot be redacted, only
// blocked or let through whole.
func UnplacedRedactions(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if redactable(f) && !f.Placed() {
			n++
		}
	}
	return n
}

// PlacedRedactions counts the PII findings RedactEdits masks.
func PlacedRedactions(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if redactable(f) && f.Placed() {
			n++
		}
	}
	return n
}

// segmentSeparator sits between segments joined for a scanner that takes one
// text. A blank line keeps a pattern from matching across two segments.
const segmentSeparator = "\n\n"

// JoinSegments returns the segment texts as one text, and where each starts
// in it.
func JoinSegments(segs []extract.Segment) ([]byte, []int) {
	var b strings.Builder
	starts := make([]int, len(segs))
	for i, s := range segs {
		if i > 0 {
			b.WriteString(segmentSeparator)
		}
		starts[i] = b.Len()
		b.WriteString(s.Text)
	}
	return []byte(b.String()), starts
}

// AssignSegments puts findings made on a JoinSegments text back on the
// segment they fall in. A finding that cannot be tied to one segment — found
// in a normalised view, or reaching over a separator — keeps its type and
// category but has no role, path or position.
func AssignSegments(findings []Finding, segs []extract.Segment, starts []int) []Finding {
	for i := range findings {
		f := &findings[i]
		seg := -1
		if f.Metadata["detected_via"] == "" && f.EndPos > f.StartPos {
			for j := range segs {
				if f.StartPos >= starts[j] && f.EndPos <= starts[j]+len(segs[j].Text) {
					seg = j
					break
				}
			}
		}
		if seg < 0 {
			f.Seg, f.StartPos, f.EndPos = 0, 0, 0
			continue
		}
		f.StartPos -= starts[seg]
		f.EndPos -= starts[seg]
		placeFinding(f, seg, segs[seg])
	}
	return dedupeFindings(findings)
}
