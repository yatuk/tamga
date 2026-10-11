package scanner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/extract"
)

// Two test numbers that pass Luhn, belong to nobody and end in the same four
// digits: the repo's usual Visa-shaped one, and one under the 411111 test
// prefix.
const (
	cardA = "4532015112830366"
	cardB = "4111110000030366"
)

// fullwidth writes a number in fullwidth digits: no pattern matches it as
// written, the normalised view reads it.
func fullwidth(digits string) string {
	var b strings.Builder
	for _, r := range digits {
		b.WriteRune(r - '0' + '０')
	}
	return b.String()
}

func cardRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(NewPIIScanner())
	return reg
}

func TestMaskPAN_KeepsTheLastFourOnly(t *testing.T) {
	for in, want := range map[string]string{
		cardA:                 "************0366",
		"4532 0151 1283 0366": "************0366",
		"4532-0151-1283-0366": "************0366",
		"378282246310005":     "***********0005", // 15 digits
		"4111111111111":       "*********1111",   // 13 digits
		"6011111111111111117": "***************1117",
		"5555 5555 5555 4444": "************4444",
	} {
		if got := maskPAN(in); got != want {
			t.Errorf("maskPAN(%q) = %q, want %q", in, got, want)
		}
	}
}

// A card finding says the last four digits and what kind of card it is. It
// does not hold the first six: not in the match, and not as the BIN the
// issuer was looked up by.
func TestCardFinding_HoldsTheLastFourAndNoBIN(t *testing.T) {
	globalBINLookup = nil
	if err := InitBINLookup(filepath.Join("..", "..", "data", "bins.csv")); err != nil {
		t.Fatalf("InitBINLookup: %v", err)
	}
	if !validLuhn(cardA) || !validLuhn(cardB) {
		t.Fatal("the test numbers must pass Luhn")
	}
	longDigits := regexp.MustCompile(`\d{5,}`)
	for _, text := range []string{
		"Card " + cardA + " please",
		"Card 4532 0151 1283 0366 please",
		"Card " + fullwidth(cardA) + " please", // the normalised view
		"Card 4532.0151.1283.0366 please",      // the view without separators
	} {
		found, err := NewPIIScanner().Scan(context.Background(), []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		cards := byCategory(found, "credit_card")
		if len(cards) == 0 {
			t.Errorf("%q: no card finding", text)
			continue
		}
		for _, f := range cards {
			if f.Match != "************0366" {
				t.Errorf("%q: match = %q, want the last four only", text, f.Match)
			}
			if _, ok := f.Metadata["bin"]; ok {
				t.Errorf("%q: the finding stores the BIN: %v", text, f.Metadata)
			}
			if f.Metadata["card_brand"] != "Visa" {
				t.Errorf("%q: card_brand = %q, want it kept", text, f.Metadata["card_brand"])
			}
			if f.fp == 0 {
				t.Errorf("%q: the finding has no fingerprint to be told apart by", text)
			}
			raw, err := json.Marshal(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), cardA[:6]) || longDigits.Match(raw) {
				t.Errorf("%q: the encoded finding holds more of the number than its last four: %s", text, raw)
			}
			if back := ProtoToFindings(FindingsToProto([]Finding{f})); back[0].fp != 0 {
				t.Error("the fingerprint crossed the wire")
			}
		}
	}
}

// Two different cards that end in the same four digits have the same match,
// and one card written twice has it too. A second one must never be taken
// for an echo of the first: an echo is dropped, and what is dropped is not
// masked. That is what let a REDACT mask the plain copy and send the other.
func TestCardDedupe_SameLastFourDifferentCards(t *testing.T) {
	reg := cardRegistry()
	cfg := PipelineConfig{Mode: ModeSync}
	scan := func(text string) []Finding {
		t.Helper()
		found, err := reg.ScanSegments(context.Background(), []extract.Segment{seg(extract.RoleUser, "m[0]", text)}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}

	if p, f := countCategory(scan("pay with "+cardA+" today"), "credit_card"); p != 1 || f != 0 {
		t.Fatalf("one card: placed=%d floating=%d, want 1 and 0", p, f)
	}
	if p, f := countCategory(scan("pay with "+cardA+" or "+cardB+" today"), "credit_card"); p != 2 || f != 0 {
		t.Fatalf("two plain cards: placed=%d floating=%d, want 2 and 0", p, f)
	}
	// A card in fullwidth digits is read where it stands: it is a second
	// finding with a position of its own, whether it is another card or the
	// same one written again. Findings are counted by place, not by value.
	for _, hiddenCard := range []string{cardB, cardA} {
		text := "pay with " + cardA + " or " + fullwidth(hiddenCard) + " today"
		both := scan(text)
		if p, f := countCategory(both, "credit_card"); p != 2 || f != 0 {
			t.Fatalf("plain and fullwidth card: placed=%d floating=%d, want 2 and 0", p, f)
		}
		masked := string(RedactContent([]byte(text), both))
		if masked != "pay with [credit_card_REDACTED] or [credit_card_REDACTED] today" {
			t.Fatalf("masked = %q", masked)
		}
		raw, err := reg.ScanRaw(context.Background(), []byte(text), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if p, f := countCategory(raw, "credit_card"); p != 2 || f != 0 {
			t.Fatalf("raw scan, plain and fullwidth card: placed=%d floating=%d, want 2 and 0", p, f)
		}
	}

	// A card that only a decoded payload holds has no position. It is not
	// an echo of the plain one, whichever card it is.
	for _, hiddenCard := range []string{cardB, cardA} {
		text := "pay with " + cardA + " or " + base64.StdEncoding.EncodeToString([]byte(hiddenCard)) + " today"
		both := scan(text)
		if p, f := countCategory(both, "credit_card"); p != 1 || f == 0 {
			t.Fatalf("plain and encoded card: placed=%d floating=%d, want 1 and at least 1", p, f)
		}
		if UnplacedRedactions(both) == 0 {
			t.Fatal("the encoded card was dropped as an echo of the plain one")
		}
		raw, err := reg.ScanRaw(context.Background(), []byte(text), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if UnplacedRedactions(raw) == 0 {
			t.Fatal("raw scan: the encoded card was dropped as an echo of the plain one")
		}
	}
}

// The remote scanner service settles its findings with ScanRaw and sends
// them without fingerprints. The proxy settles them again: one card stays
// one finding, and a second card with the same last four that only a
// normalised view reveals is kept.
func TestCardDedupe_FromTheScannerService(t *testing.T) {
	reg := cardRegistry()
	remote := func(text string) []Finding {
		found, err := reg.ScanRaw(context.Background(), []byte(text), PipelineConfig{Mode: ModeSync})
		if err != nil {
			t.Fatal(err)
		}
		return SettleRaw(ProtoToFindings(FindingsToProto(found)), []byte(text))
	}
	if p, f := countCategory(remote("pay with "+cardA+" today"), "credit_card"); p != 1 || f != 0 {
		t.Fatalf("one card: placed=%d floating=%d, want 1 and 0", p, f)
	}
	for _, hiddenCard := range []string{cardB, cardA} {
		both := remote("pay with " + cardA + " or " + fullwidth(hiddenCard) + " today")
		if p, f := countCategory(both, "credit_card"); p != 2 || f != 0 {
			t.Fatalf("plain and fullwidth card: placed=%d floating=%d, want 2 and 0", p, f)
		}
		both = remote("pay with " + cardA + " or " + base64.StdEncoding.EncodeToString([]byte(hiddenCard)) + " today")
		if p, f := countCategory(both, "credit_card"); p != 1 || f == 0 {
			t.Fatalf("plain and encoded card: placed=%d floating=%d, want 1 and at least 1", p, f)
		}
	}
}

// The overlap of two chunks of a long text reports a value twice. The
// duplicate is dropped by fingerprint, and a different card with the same
// last four is not.
func TestCardChunkOverlap_KeepsDifferentCardsWithTheSameLastFour(t *testing.T) {
	a := Finding{Type: "pii", Category: "credit_card", Match: maskPAN(cardA), fp: cardFingerprint(cardA)}
	b := Finding{Type: "pii", Category: "credit_card", Match: maskPAN(cardB), fp: cardFingerprint(cardB)}
	if a.Match != b.Match || a.fp == b.fp {
		t.Fatalf("the two cards must share a match and differ in fingerprint: %q %q", a.Match, b.Match)
	}
	if got := dropOverlapDuplicates([]Finding{a, b, a}); len(got) != 2 {
		t.Fatalf("%d findings, want the two cards once each", len(got))
	}
	if cardFingerprint("4532 0151 1283 0366") != a.fp {
		t.Error("one number written with spaces must be the same card")
	}
}
