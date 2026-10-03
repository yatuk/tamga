package extract

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Edit replaces Text[From:To] of segment Seg (an index into the segments
// Extract returned) with Replacement.
type Edit struct {
	Seg         int
	From, To    int
	Replacement string
}

// Rewrite applies edits to the text of their segments and returns the body
// with those strings re-encoded in place.
//
// Only the string tokens that hold an edited segment change. Every other
// byte — key order, number formatting, whitespace, fields this proxy does not
// know — is copied as it was, so the provider receives the request the client
// built, minus what was redacted.
//
// Edits that overlap inside one segment are merged: the union of their ranges
// is replaced by the replacement of the first. Rewrite fails, changing
// nothing, when an edit does not fit its segment.
func Rewrite(body []byte, segs []Segment, edits []Edit) ([]byte, error) {
	if len(edits) == 0 {
		return body, nil
	}
	bySeg := map[int][]Edit{}
	for _, e := range edits {
		if e.Seg < 0 || e.Seg >= len(segs) {
			return nil, fmt.Errorf("extract: edit for segment %d of %d", e.Seg, len(segs))
		}
		if e.From < 0 || e.To > len(segs[e.Seg].Text) || e.From >= e.To {
			return nil, fmt.Errorf("extract: edit [%d,%d) outside segment %s", e.From, e.To, segs[e.Seg].Path)
		}
		bySeg[e.Seg] = append(bySeg[e.Seg], e)
	}

	// New text per edited segment.
	newText := map[int]string{}
	for i, list := range bySeg {
		newText[i] = applyEdits(segs[i].Text, list)
	}

	// One replacement per string token of the body. Nested segments share
	// their outer token, so they are grouped by it.
	type tokenKey struct{ start, end int }
	tokens := map[tokenKey][]int{}
	for i := range newText {
		k := tokenKey{segs[i].Start, segs[i].End}
		tokens[k] = append(tokens[k], i)
	}
	type replacement struct {
		start, end int
		token      []byte
	}
	var reps []replacement
	for k, idxs := range tokens {
		first := segs[idxs[0]]
		var tok []byte
		var err error
		switch {
		case first.Nested:
			tok, err = rewriteNested(body[k.start:k.end], segs, idxs, newText)
		case first.Encoding == "base64":
			tok, err = rewriteBase64(body[k.start:k.end], newText[idxs[0]])
		default:
			tok = encodeString(newText[idxs[0]])
		}
		if err != nil {
			return nil, err
		}
		reps = append(reps, replacement{k.start, k.end, tok})
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].start < reps[j].start })

	var out bytes.Buffer
	out.Grow(len(body) + 64)
	pos := 0
	for _, r := range reps {
		if r.start < pos {
			return nil, fmt.Errorf("extract: overlapping tokens at byte %d", r.start)
		}
		out.Write(body[pos:r.start])
		out.Write(r.token)
		pos = r.end
	}
	out.Write(body[pos:])
	return out.Bytes(), nil
}

// applyEdits rewrites text, merging edits whose ranges touch or overlap.
func applyEdits(text string, edits []Edit) string {
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].From != edits[j].From {
			return edits[i].From < edits[j].From
		}
		return edits[i].To > edits[j].To
	})
	var b strings.Builder
	pos := 0
	for i := 0; i < len(edits); i++ {
		e := edits[i]
		end := e.To
		for i+1 < len(edits) && edits[i+1].From < end {
			if edits[i+1].To > end {
				end = edits[i+1].To
			}
			i++
		}
		b.WriteString(text[pos:e.From])
		b.WriteString(e.Replacement)
		pos = end
	}
	b.WriteString(text[pos:])
	return b.String()
}

// encodeString returns s as a JSON string token. HTML escaping is off: the
// token goes to an API, and "<" must not turn into "<" on the way.
func encodeString(s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// rewriteNested re-encodes a string token that holds a JSON document, after
// replacing the inner string tokens of the edited segments.
func rewriteNested(outerToken []byte, segs []Segment, idxs []int, newText map[int]string) ([]byte, error) {
	var doc string
	if err := json.Unmarshal(outerToken, &doc); err != nil {
		return nil, fmt.Errorf("extract: nested document is not a string: %w", err)
	}
	sort.Slice(idxs, func(a, b int) bool { return segs[idxs[a]].InnerStart > segs[idxs[b]].InnerStart })
	for _, i := range idxs {
		s := segs[i]
		if s.InnerStart < 0 || s.InnerEnd > len(doc) || s.InnerStart >= s.InnerEnd {
			return nil, fmt.Errorf("extract: nested range of %s outside its document", s.Path)
		}
		doc = doc[:s.InnerStart] + string(encodeString(newText[i])) + doc[s.InnerEnd:]
	}
	return encodeString(doc), nil
}

// rewriteBase64 re-encodes a base64 text attachment, keeping a data URI
// prefix when the token had one.
func rewriteBase64(token []byte, text string) ([]byte, error) {
	var old string
	if err := json.Unmarshal(token, &old); err != nil {
		return nil, fmt.Errorf("extract: attachment is not a string: %w", err)
	}
	prefix := ""
	if head, _, ok := strings.Cut(old, ";base64,"); ok {
		prefix = head + ";base64,"
	}
	return encodeString(prefix + base64.StdEncoding.EncodeToString([]byte(text))), nil
}

// RewriteChecked is Rewrite followed by proof that it did what was asked: the
// new body is extracted again and every segment must read exactly as the
// edits say, the edited ones changed and all others untouched. A rewrite that
// cannot be confirmed is returned as an error, never as a body.
//
// This is the guard against the rewriter and the extractor disagreeing with
// each other or with the JSON they produce; a redaction that silently did not
// happen would send the original text to the provider.
func RewriteChecked(provider string, body []byte, segs []Segment, edits []Edit) ([]byte, error) {
	out, err := Rewrite(body, segs, edits)
	if err != nil {
		return nil, err
	}
	if len(edits) == 0 {
		return out, nil
	}
	want := make([]string, len(segs))
	for i, s := range segs {
		want[i] = s.Text
	}
	bySeg := map[int][]Edit{}
	for _, e := range edits {
		bySeg[e.Seg] = append(bySeg[e.Seg], e)
	}
	for i, list := range bySeg {
		want[i] = applyEdits(segs[i].Text, list)
	}

	after, ok := Extract(provider, out)
	if !ok {
		return nil, fmt.Errorf("extract: rewritten body is no longer a recognised request")
	}
	// A segment edited down to nothing is no longer emitted.
	j := 0
	for i := range segs {
		if want[i] == "" {
			continue
		}
		if j >= len(after.Segments) || after.Segments[j].Path != segs[i].Path || after.Segments[j].Text != want[i] {
			return nil, fmt.Errorf("extract: rewrite of %s could not be confirmed", segs[i].Path)
		}
		j++
	}
	if j != len(after.Segments) {
		return nil, fmt.Errorf("extract: rewritten body has %d segments, expected %d", len(after.Segments), j)
	}
	return out, nil
}
