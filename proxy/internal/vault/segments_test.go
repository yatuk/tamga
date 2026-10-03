package vault

import (
	"encoding/json"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/scanner"
)

func TestTokenizeEdits(t *testing.T) {
	segs := []extract.Segment{
		{Path: "m[0]", Text: "mail ayse@example.com or veli@example.com"},
		{Path: "m[1]", Text: "id 38461027540"},
	}
	findings := []scanner.Finding{
		{Type: "pii", Category: "email", Seg: 0, StartPos: 5, EndPos: 21},
		{Type: "pii", Category: "email", Seg: 0, StartPos: 25, EndPos: 41},
		{Type: "pii", Category: "tc_kimlik", Seg: 1, StartPos: 3, EndPos: 14},
		{Type: "pii", Category: "email", Seg: 0},                                   // no position: not tokenised
		{Type: "injection", Category: "jailbreak", Seg: 0, StartPos: 0, EndPos: 4}, // not PII
	}
	edits, mapping := TokenizeEdits(segs, findings)
	if len(edits) != 3 || len(mapping) != 3 {
		t.Fatalf("%d edits, %d mappings, want 3 and 3", len(edits), len(mapping))
	}
	want := map[string]string{
		"[TAMGA_EMAIL_1]":     "ayse@example.com",
		"[TAMGA_EMAIL_2]":     "veli@example.com",
		"[TAMGA_TC_KIMLIK_1]": "38461027540",
	}
	for token, original := range want {
		if mapping[token] != original {
			t.Errorf("mapping[%s] = %q, want %q", token, mapping[token], original)
		}
	}
}

// Two findings over one stretch of text are one placeholder, and restoring
// it gives the whole stretch back.
func TestTokenizeEdits_OverlapSharesOnePlaceholder(t *testing.T) {
	segs := []extract.Segment{{Path: "m[0]", Text: "0123456789"}}
	findings := []scanner.Finding{
		{Type: "pii", Category: "a", Seg: 0, StartPos: 2, EndPos: 6},
		{Type: "pii", Category: "b", Seg: 0, StartPos: 4, EndPos: 8},
	}
	edits, mapping := TokenizeEdits(segs, findings)
	if len(edits) != 1 || edits[0].From != 2 || edits[0].To != 8 {
		t.Fatalf("edits = %+v, want one edit over [2,8)", edits)
	}
	if mapping[edits[0].Replacement] != "234567" {
		t.Fatalf("original = %q, want the union %q", mapping[edits[0].Replacement], "234567")
	}
}

// Finding B9: an original with a quote, a backslash or a line break in it
// broke the JSON of the response it was restored into.
func TestRestore_EscapesInsideJSON(t *testing.T) {
	original := "he said \"hi\"\\ and left\nnext line"
	mapping := map[string]string{"[TAMGA_NOTE_1]": original}

	resp := []byte(`{"choices":[{"message":{"content":"You wrote: [TAMGA_NOTE_1]."}}]}`)
	restored := Restore(resp, mapping)
	var doc struct {
		Choices []struct {
			Message struct{ Content string }
		}
	}
	if err := json.Unmarshal(restored, &doc); err != nil {
		t.Fatalf("restored response is not JSON: %v\n%s", err, restored)
	}
	if got, want := doc.Choices[0].Message.Content, "You wrote: "+original+"."; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}

	// Outside JSON the original goes in as it is.
	if got := string(Restore([]byte("plain [TAMGA_NOTE_1]"), mapping)); got != "plain "+original {
		t.Fatalf("plain text restore = %q", got)
	}
}
