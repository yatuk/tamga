package scanner

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner/normalize"
)

// ScanSegments runs the registered scanners over each segment of a request
// and returns the findings with the role and path of the text they were
// found in.
//
// A finding's StartPos and EndPos are offsets into its segment's Text. They
// are set when the match was found in the text as written, or in a reading
// of it that knows where its bytes came from (fullwidth digits, zero-width
// splits, entities, number words: normalize.View). A match found only in a
// reading that is another text (what a base64 token decodes to) has no
// position: the scanners know it is there, not where, so it can be blocked
// but not cut out.
//
// Findings are counted by where they are, not by what they hold: the same
// value at two places is two findings, and one place found through several
// readings is one.
func (r *Registry) ScanSegments(ctx context.Context, segs []extract.Segment, cfg PipelineConfig) ([]Finding, error) {
	var all []Finding
	perSeg, firstErr := r.scanUnits(ctx, segs, cfg)
	for i := range perSeg {
		perSeg[i] = dedupeFindings(perSeg[i])
		all = append(all, perSeg[i]...)
	}

	across, acrossErr := r.scanAcrossSegments(ctx, segs, perSeg)
	all = append(all, across...)
	if firstErr == nil {
		firstErr = acrossErr
	}
	// Verdicts on a whole segment with its role: the language check.
	whole, wholeErr := r.scanWholeSegments(ctx, segs)
	all = append(all, whole...)
	if firstErr == nil {
		firstErr = wholeErr
	}
	// What only the place of a text in the conversation shows. It belongs
	// to the jailbreak scanner's rules and is left out with it.
	if r.has("jailbreak") {
		all = append(all, ConversationFindings(segs, all)...)
	}
	return all, firstErr
}

// remoteSegmentCalls is how many segments are with the remote scanner at a
// time. A long conversation is hundreds of segments; one connection carries
// the calls, and a handful at once is as fast as it gets without turning one
// request into a burst.
const remoteSegmentCalls = 8

// ScanSegmentsRemote is ScanSegments with the per-segment scan done by a
// remote scanner, for a proxy that has given its scanning to the scanner
// service. The findings are the ones ScanSegments returns for the same
// scanners.
//
// scan is called once per segment and takes one text; it is expected to
// settle its findings as ScanRaw does, which is what the service does. A
// finding therefore belongs to the segment it was found in and carries its
// role and path, also when it has no position. Joining the segments into
// one text and sharing the findings out by offset lost exactly those: a
// finding from a normalised view fits no segment.
//
// What needs more than one segment cannot be asked of a scanner that sees
// one text: the instruction split over neighbouring segments, and what the
// place of a text in the conversation says. Both are done here, with r's own
// injection scanners, as ScanSegments does them.
//
// An error from any call is returned without findings. The caller falls
// back to its own scanners; half a scan must not be read as a whole one.
func (r *Registry) ScanSegmentsRemote(ctx context.Context, segs []extract.Segment, scan func(context.Context, []byte) ([]Finding, error)) ([]Finding, error) {
	perSeg := make([][]Finding, len(segs))
	errs := make([]error, len(segs))
	one := func(i int) {
		if segs[i].Text == "" {
			return
		}
		found, err := scan(ctx, []byte(segs[i].Text))
		if err != nil {
			errs[i] = err
			return
		}
		for j := range found {
			placeFinding(&found[j], i, segs[i])
		}
		perSeg[i] = dedupeFindings(found)
	}
	if len(segs) == 1 {
		one(0)
	} else {
		var wg sync.WaitGroup
		next := make(chan int)
		for w := 0; w < remoteSegmentCalls && w < len(segs); w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range next {
					one(i)
				}
			}()
		}
		for i := range segs {
			next <- i
		}
		close(next)
		wg.Wait()
	}
	var all []Finding
	for i := range segs {
		if errs[i] != nil {
			return nil, errs[i]
		}
		all = append(all, perSeg[i]...)
	}

	across, err := r.scanAcrossSegments(ctx, segs, perSeg)
	all = append(all, across...)
	if r.has("jailbreak") {
		all = append(all, ConversationFindings(segs, all)...)
	}
	return all, err
}

// has reports whether a scanner of that name is registered.
func (r *Registry) has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.scanners {
		if s.Name() == name {
			return true
		}
	}
	return false
}

// dropEchoes removes the unpositioned findings of a maskable type (PII,
// secrets, custom entities, competitor names) that are only another view of
// a positioned one.
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

// A finding made in a reading of the text says so in Metadata["detected_via"].
// When the reading knew where its bytes came from, the finding's position is
// the span of the text as written and Metadata["position"] says "source".
// Without that, the offsets are those of the reading and mean nothing in the
// text: that is how a finding from a scanner of an earlier version arrives,
// and it is treated as one without a position.
const (
	metaDetectedVia = "detected_via"
	metaPosition    = "position"
	positionSource  = "source"
)

// viewOnly reports whether a finding was made in a reading of the text that
// could not say where in the text the match is.
func viewOnly(f Finding) bool {
	return f.Metadata[metaDetectedVia] != "" && f.Metadata[metaPosition] != positionSource
}

// asWritten reports whether a finding was made in the text as written.
func asWritten(f Finding) bool { return f.Metadata[metaDetectedVia] == "" }

// locateInView settles the position of a finding whose StartPos and EndPos
// are offsets into a reading of the text: the span of the text the match was
// read from when the reading knows it, and otherwise the offsets as they are,
// marked as the reading's.
func locateInView(f *Finding, view normalize.View, via string) {
	if f.Metadata == nil {
		f.Metadata = map[string]string{}
	}
	f.Metadata[metaDetectedVia] = via
	if from, to, ok := view.Span(f.StartPos, f.EndPos); ok {
		f.StartPos, f.EndPos = from, to
		f.Metadata[metaPosition] = positionSource
	}
}

// placeFinding ties a finding to its segment and settles whether its
// position can be trusted.
func placeFinding(f *Finding, index int, seg extract.Segment) {
	f.Seg = index
	f.Role = string(seg.Role)
	f.Path = seg.Path
	if viewOnly(*f) || f.StartPos < 0 || f.StartPos >= f.EndPos || f.EndPos > len(seg.Text) {
		f.StartPos, f.EndPos = 0, 0
	}
}

// Placed reports whether the finding has a position in its segment's text.
func (f Finding) Placed() bool { return f.EndPos > f.StartPos }

// dedupeFindings collapses the findings that describe the same place in the
// text. What they hold is not compared: the same value written at two
// places is two findings, and each has to be masked.
//
// Positioned findings of one category in one segment follow the rules
// Presidio uses: on the same span one stays (the one made in the text as
// written, or else the more confident); when one span contains another the
// longer stays; spans that only intersect both stay. A finding made in a
// reading of the text that lies inside one made in the text as written is
// that text read another way, whatever category the reading gave it, and is
// dropped: a phone number with its digits turned round is not also an
// identity number.
//
// A finding without a position is never taken for an echo of a positioned
// one. Whether it is one cannot be told from the two matches, and dropEchoes
// has already answered it by scanning the text again without the positioned
// values; what is left was found only in a reading that is another text, and
// a value that is dropped here is a value that is not masked. Unpositioned
// findings of one category that show the same value are reported once: none
// of them can be masked, and one is enough to say so. A secret's match holds
// none of its value and a card's only its last four digits, so both are
// compared by fingerprint, and one without a fingerprint is kept (see
// blindValue).
func dedupeFindings(in []Finding) []Finding {
	if len(in) < 2 {
		return in
	}
	in = dropWrittenEchoes(in)
	type group struct {
		seg      int
		typ, cat string
	}
	order := []group{}
	groups := map[group][]Finding{}
	for _, f := range in {
		g := group{f.Seg, f.Type, f.Category}
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
		// A verdict on the text (an injection, moderated content) is not
		// masked, so one that repeats a positioned one may go.
		verdicts := map[string]bool{}
		if !maskableTypes[g.typ] {
			for _, p := range placed {
				verdicts[valueSignature(p)] = true
			}
		}
		out = append(out, placed...)

		seen := map[string]int{}
		var kept []Finding
		var merged []int
		for _, f := range floating {
			if blindValue(f) {
				kept, merged = append(kept, f), append(merged, 0)
				continue
			}
			sig := valueSignature(f)
			if verdicts[sig] {
				continue
			}
			if k, ok := seen[sig]; ok {
				merged[k]++
				continue
			}
			seen[sig] = len(kept)
			kept, merged = append(kept, f), append(merged, 0)
		}
		for k := range kept {
			addVariants(&kept[k], merged[k])
		}
		out = append(out, kept...)
	}
	return out
}

// dropWrittenEchoes removes the positioned findings made in a reading of the
// text whose span lies inside a finding made in the text as written, in the
// same segment and of another category. Those of the same category are
// settled by span in dedupePlaced.
func dropWrittenEchoes(in []Finding) []Finding {
	var written []Finding
	readings := 0
	for _, f := range in {
		if !redactable(f) || !f.Placed() {
			continue
		}
		if asWritten(f) {
			written = append(written, f)
		} else {
			readings++
		}
	}
	if len(written) == 0 || readings == 0 {
		return in
	}
	out := in[:0:0]
	for _, f := range in {
		echo := false
		if redactable(f) && f.Placed() && !asWritten(f) {
			for _, w := range written {
				if w.Seg == f.Seg && w.StartPos <= f.StartPos && f.EndPos <= w.EndPos &&
					(w.Type != f.Type || w.Category != f.Category) {
					echo = true
					break
				}
			}
		}
		if !echo {
			out = append(out, f)
		}
	}
	return out
}

// addVariants notes on a finding how many others were merged into it.
func addVariants(f *Finding, n int) {
	if n == 0 {
		return
	}
	prev, _ := strconv.Atoi(f.Metadata["variants"])
	f.Metadata = copyMeta(f.Metadata)
	f.Metadata["variants"] = strconv.Itoa(prev + n)
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
	var merges []int
	for _, f := range in {
		merged := false
		for k := range out {
			o := out[k]
			switch {
			case o.StartPos == f.StartPos && o.EndPos == f.EndPos:
				if betterReading(f, o) {
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
				merges[k]++
				break
			}
		}
		if !merged {
			out = append(out, f)
			merges = append(merges, 0)
		}
	}
	for k := range out {
		addVariants(&out[k], merges[k])
	}
	return out
}

// betterReading says which of two findings on the same span stays: the one
// made in the text as written, whose match shows the value as it stands
// there, or else the more confident one.
func betterReading(f, o Finding) bool {
	if asWritten(f) != asWritten(o) {
		return asWritten(f)
	}
	return f.Confidence > o.Confidence
}

// valueSignature identifies the value behind a finding among the findings
// of its category: the fingerprint its scanner gave it, or else what its
// masked match shows.
func valueSignature(f Finding) string {
	if f.fp != 0 {
		return "fp:" + strconv.FormatUint(f.fp, 16)
	}
	return maskSignature(f.Match)
}

// blindValue reports whether f is a secret or a card number with nothing to
// compare: its match does not tell two values apart (a secret's holds no
// part of the value, a card's the last four digits only) and it has no
// fingerprint, which is how a finding arrives from the remote scanner
// service. It is kept. The service has the text and drops the echoes itself
// (it scans with ScanRaw); to drop here on the match alone would drop every
// further secret of a category that has one in plain sight, or a second
// card that ends in the same four digits, and a value that is dropped is
// not masked.
func blindValue(f Finding) bool {
	if f.fp != 0 {
		return false
	}
	return f.Type == "secret" || (f.Type == "pii" && f.Category == "credit_card")
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
//
// A scanner that stops here (the context ended) makes the whole scan
// incomplete, as it would inside a segment: what it found is kept and the
// error is returned.
func (r *Registry) scanAcrossSegments(ctx context.Context, segs []extract.Segment, perSeg [][]Finding) ([]Finding, error) {
	r.mu.RLock()
	var scanners []Scanner
	for _, s := range r.scanners {
		if crossSegmentScanners[s.Name()] {
			scanners = append(scanners, s)
		}
	}
	r.mu.RUnlock()
	if len(scanners) == 0 {
		return nil, nil
	}

	var out []Finding
	var deg degradation
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
					deg.note(s.Name(), DegradedError, err)
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
	return out, deg.result()
}

// redactable reports whether a finding is of a type REDACT can mask: one
// whose findings are values with a span (see maskableTypes).
func redactable(f Finding) bool { return maskableTypes[f.Type] }

// RedactEdits turns the positioned findings of a segment scan into the edits
// that mask them. It masks every finding it is given that can be masked; the
// caller decides which findings the policy wants masked. A finding of a type
// without spans gets no edit: its position is not one to cut at.
func RedactEdits(findings []Finding) []extract.Edit {
	var edits []extract.Edit
	for _, f := range findings {
		if redactable(f) && f.Placed() {
			edits = append(edits, extract.Edit{Seg: f.Seg, From: f.StartPos, To: f.EndPos, Replacement: MaskMarker(f)})
		}
	}
	return edits
}

// UnplacedRedactions counts the findings of a maskable type that have no
// position and so cannot be masked. A request with any of them cannot be
// redacted, only blocked or let through whole.
func UnplacedRedactions(findings []Finding) int {
	n := 0
	for _, f := range findings {
		if redactable(f) && !f.Placed() {
			n++
		}
	}
	return n
}

// PlacedRedactions counts the findings RedactEdits masks.
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
		if !viewOnly(*f) && f.EndPos > f.StartPos {
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
