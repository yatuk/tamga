// Package vault implements reversible PII tokenization: instead of masking a
// finding irreversibly (scanner.RedactContent), each occurrence is replaced
// with a unique numbered placeholder, the original value is held for the
// request lifetime (optionally encrypted at rest in Redis), and the response
// is rewritten to restore the originals before it reaches the caller.
//
// Round-trip:
//
//	prompt   "Ahmet için özet, TC 12345678950"
//	forwarded "[TAMGA_NAME_1] için özet, TC [TAMGA_TC_KIMLIK_1]"
//	response "[TAMGA_NAME_1] için 3 aylık özet ..."
//	client   "Ahmet için 3 aylık özet ..."
package vault

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yatuk/tamga/internal/scanner"
)

// Tokenize walks PII/custom findings in ascending StartPos order and replaces
// each matched span with a numbered, reversible placeholder. It returns the
// rewritten content and a mapping of placeholder -> original plaintext.
//
// Unlike scanner.RedactContent (a fixed, irreversible "[<category>_REDACTED]"
// mask), each occurrence gets a per-category index so the exact original can
// be restored in the response. The original value is read from the content
// span content[StartPos:EndPos] — never from Finding.Match, which is masked
// for PII findings.
func Tokenize(content []byte, findings []scanner.Finding) ([]byte, map[string]string) {
	var redactable []scanner.Finding
	for _, f := range findings {
		if f.Type != "pii" && f.Type != "custom" {
			continue
		}
		if f.StartPos < 0 || f.EndPos < 0 || f.StartPos >= f.EndPos || f.EndPos > len(content) {
			continue
		}
		redactable = append(redactable, f)
	}
	if len(redactable) == 0 {
		out := make([]byte, len(content))
		copy(out, content)
		return out, nil
	}

	sort.Slice(redactable, func(i, j int) bool {
		return redactable[i].StartPos < redactable[j].StartPos
	})

	mapping := make(map[string]string, len(redactable))
	counters := make(map[string]int)
	var buf bytes.Buffer
	pos := 0
	for _, f := range redactable {
		if f.StartPos < pos {
			// Overlapping finding — already consumed by a prior span.
			continue
		}
		if f.StartPos > pos {
			buf.Write(content[pos:f.StartPos])
		}
		cat := strings.ToUpper(f.Category)
		counters[cat]++
		token := fmt.Sprintf("[TAMGA_%s_%d]", cat, counters[cat])
		mapping[token] = string(content[f.StartPos:f.EndPos])
		buf.WriteString(token)
		pos = f.EndPos
	}
	if pos < len(content) {
		buf.Write(content[pos:])
	}
	return buf.Bytes(), mapping
}

// Restore replaces every placeholder in body with its original value. Safe to
// call with a nil/empty mapping (returns body unchanged).
func Restore(body []byte, mapping map[string]string) []byte {
	if len(mapping) == 0 {
		return body
	}
	out := body
	for token, original := range mapping {
		out = bytes.ReplaceAll(out, []byte(token), []byte(original))
	}
	return out
}
