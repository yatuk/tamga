package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner"
)

// TokenizeEdits is Tokenize for a request scanned by segment. It returns the
// edits that put a numbered placeholder in place of each positioned PII
// finding, and the placeholder -> original mapping.
//
// Findings that overlap inside a segment share one placeholder, whose
// original is the union of their text: two overlapping matches are one
// stretch of sensitive text, and restoring it must give back all of it.
func TokenizeEdits(segs []extract.Segment, findings []scanner.Finding) ([]extract.Edit, map[string]string) {
	type span struct {
		seg, from, to int
		category      string
	}
	var spans []span
	for _, f := range findings {
		if f.Type != "pii" && f.Type != "custom" {
			continue
		}
		if !f.Placed() || f.Seg < 0 || f.Seg >= len(segs) || f.EndPos > len(segs[f.Seg].Text) {
			continue
		}
		spans = append(spans, span{f.Seg, f.StartPos, f.EndPos, f.Category})
	}
	if len(spans) == 0 {
		return nil, nil
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].seg != spans[j].seg {
			return spans[i].seg < spans[j].seg
		}
		return spans[i].from < spans[j].from
	})

	mapping := make(map[string]string, len(spans))
	counters := make(map[string]int)
	var edits []extract.Edit
	for i := 0; i < len(spans); i++ {
		cur := spans[i]
		for i+1 < len(spans) && spans[i+1].seg == cur.seg && spans[i+1].from < cur.to {
			if spans[i+1].to > cur.to {
				cur.to = spans[i+1].to
			}
			i++
		}
		cat := strings.ToUpper(cur.category)
		counters[cat]++
		token := fmt.Sprintf("[TAMGA_%s_%d]", cat, counters[cat])
		mapping[token] = segs[cur.seg].Text[cur.from:cur.to]
		edits = append(edits, extract.Edit{Seg: cur.seg, From: cur.from, To: cur.to, Replacement: token})
	}
	return edits, mapping
}

// restoreValue returns original in the form it must take inside body. In a
// JSON document the original sits inside a string, so a quote, a backslash or
// a line break in it has to be escaped or the document breaks.
func restoreValue(body []byte, original string) []byte {
	if !json.Valid(body) {
		return []byte(original)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(original); err != nil {
		return []byte(original)
	}
	quoted := bytes.TrimRight(buf.Bytes(), "\n")
	return quoted[1 : len(quoted)-1]
}
