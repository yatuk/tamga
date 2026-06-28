package proximity

import (
	"math"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

func TestScoreProximity_CreditCardNearContext(t *testing.T) {
	text := "my credit card number is 4532015112830366 for the payment"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "453201******0366",
			StartPos:   25,
			EndPos:     41,
			Confidence: 0.80,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence <= 0.80 {
		t.Fatalf("expected confidence boost, got %f", findings[0].Confidence)
	}
	if findings[0].ProximityBoost != 0.15 {
		t.Fatalf("expected ProximityBoost 0.15, got %f", findings[0].ProximityBoost)
	}
}

func TestScoreProximity_SSNNearContext(t *testing.T) {
	text := "my ssn is 123-45-6789 please keep it safe"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "ssn",
			Match:      "123-45-6789",
			StartPos:   10,
			EndPos:     21,
			Confidence: 0.70,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence <= 0.70 {
		t.Fatalf("expected confidence boost, got %f", findings[0].Confidence)
	}
	if findings[0].ProximityBoost != 0.20 {
		t.Fatalf("expected ProximityBoost 0.20, got %f", findings[0].ProximityBoost)
	}
}

func TestScoreProximity_PasswordNearBase64(t *testing.T) {
	text := "password: abc123def456ghi789jkl"
	findings := []scanner.Finding{
		{
			Type:       "secret",
			Category:   "generic_api_key",
			Match:      "abc123def456ghi789jkl",
			StartPos:   10,
			EndPos:     30,
			Confidence: 0.65,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence <= 0.65 {
		t.Fatalf("expected confidence boost, got %f", findings[0].Confidence)
	}
}

func TestScoreProximity_TooFarApart_NoBoost(t *testing.T) {
	// "credit card" and the number are separated by many words (> 5).
	text := "credit card is one two three four five six seven eight nine ten words away from 4532015112830366"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "453201******0366",
			StartPos:   len("credit card is one two three four five six seven eight nine ten words away from "),
			EndPos:     len(text),
			Confidence: 0.80,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence != 0.80 {
		t.Fatalf("expected no confidence boost (too far), got %f", findings[0].Confidence)
	}
	if findings[0].ProximityBoost != 0 {
		t.Fatalf("expected ProximityBoost 0, got %f", findings[0].ProximityBoost)
	}
}

func TestScoreProximity_NoRulesMatch_Unchanged(t *testing.T) {
	text := "here is some plain text with no sensitive context"
	findings := []scanner.Finding{
		{
			Type:       "secret",
			Category:   "aws_access_key",
			Match:      "AKIAIOSFODNN7EXAMPLE",
			StartPos:   0,
			EndPos:     20,
			Confidence: 0.90,
		},
	}

	originalConfidence := findings[0].Confidence
	ScoreProximity(text, findings)

	if findings[0].Confidence != originalConfidence {
		t.Fatalf("expected confidence unchanged, got %f", findings[0].Confidence)
	}
	if findings[0].ProximityBoost != 0 {
		t.Fatalf("expected ProximityBoost 0, got %f", findings[0].ProximityBoost)
	}
}

func TestScoreProximity_EmptyFindings(t *testing.T) {
	text := "credit card 4532015112830366"
	findings := []scanner.Finding{}

	// Should not panic.
	ScoreProximity(text, findings)
}

func TestScoreProximity_EmptyText(t *testing.T) {
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "4532015112830366",
			Confidence: 0.80,
		},
	}
	ScoreProximity("", findings)
	if findings[0].Confidence != 0.80 {
		t.Fatalf("expected unchanged on empty text")
	}
}

func TestScoreProximity_BoostClampedAtOne(t *testing.T) {
	text := "credit card 4532015112830366"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "453201******0366",
			StartPos:   12,
			EndPos:     28,
			Confidence: 0.95,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence > 1.0 {
		t.Fatalf("expected confidence clamped at 1.0, got %f", findings[0].Confidence)
	}
	if findings[0].Confidence != 1.0 {
		t.Fatalf("expected confidence 1.0, got %f", findings[0].Confidence)
	}
}

func TestScoreProximity_MultipleBoosts_HighestWins(t *testing.T) {
	// "password" + "api key" both near a base64 string → highest boost wins.
	text := "password is abc123def456ghi789jkl and the api key is abc123def456ghi789jkl"
	// Finding at the first base64 string position.
	findings := []scanner.Finding{
		{
			Type:       "secret",
			Category:   "generic_api_key",
			Match:      "abc123def456ghi789jkl",
			StartPos:   12,
			EndPos:     32,
			Confidence: 0.60,
		},
	}

	ScoreProximity(text, findings)

	// Both "password" (boost 0.15) and "api key" (boost 0.15) match, max is 0.15.
	expectedConf := 0.60 + 0.15
	if findings[0].Confidence != expectedConf {
		t.Fatalf("expected confidence %f, got %f", expectedConf, findings[0].Confidence)
	}
	if findings[0].ProximityBoost != 0.15 {
		t.Fatalf("expected ProximityBoost 0.15, got %f", findings[0].ProximityBoost)
	}
}

func TestScoreProximity_IBANContext(t *testing.T) {
	text := "please transfer to iban DE89370400440532013000 today"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "iban",
			Match:      "DE89370400440532013000",
			StartPos:   22,
			EndPos:     44,
			Confidence: 0.80,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence <= 0.80 {
		t.Fatalf("expected confidence boost for IBAN with context")
	}
	if findings[0].ProximityBoost == 0 {
		t.Fatalf("expected non-zero ProximityBoost")
	}
}

func TestScoreProximity_UpdatesConfidenceScore(t *testing.T) {
	text := "my ssn is 123-45-6789"
	score := scanner.ConfidenceScore{
		Total: 70,
		Breakdown: scanner.ConfidenceFactor{
			Format:    30,
			Algorithm: 20,
			Database:  0,
			Context:   20,
		},
		Action:    "REDACT",
		Reasoning: "score=70 action=REDACT factors=[format match (+30), algorithm validation (+20), context keywords (+20)]",
	}
	findings := []scanner.Finding{
		{
			Type:            "pii",
			Category:        "ssn",
			Match:           "123-45-6789",
			StartPos:        10,
			EndPos:          21,
			Confidence:      0.70,
			ConfidenceScore: &score,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].ConfidenceScore == nil {
		t.Fatal("confidence score should not be nil")
	}
	// 0.70 + 0.20 = 0.90 → Total should be 90
	if findings[0].ConfidenceScore.Total != 90 {
		t.Fatalf("expected ConfidenceScore.Total 90, got %d", findings[0].ConfidenceScore.Total)
	}
	if findings[0].ConfidenceScore.Action != "BLOCK" {
		t.Fatalf("expected action BLOCK at 90 total, got %s", findings[0].ConfidenceScore.Action)
	}
}

func TestScoreProximity_ContextInTurkish(t *testing.T) {
	text := "kredi kartı numarası 4532015112830366 lütfen"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "453201******0366",
			StartPos:   21,
			EndPos:     37,
			Confidence: 0.80,
		},
	}

	ScoreProximity(text, findings)

	if findings[0].Confidence <= 0.80 {
		t.Fatalf("expected confidence boost for Turkish credit card context")
	}
}

func TestScoreProximity_BadPositions_Skipped(t *testing.T) {
	text := "credit card 4532015112830366"
	findings := []scanner.Finding{
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "4532015112830366",
			StartPos:   -1, // invalid start
			EndPos:     16,
			Confidence: 0.80,
		},
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "4532015112830366",
			StartPos:   200, // beyond text length
			EndPos:     216,
			Confidence: 0.80,
		},
		{
			Type:       "pii",
			Category:   "credit_card",
			Match:      "4532015112830366",
			StartPos:   10,
			EndPos:     5, // end before start
			Confidence: 0.80,
		},
	}

	ScoreProximity(text, findings)

	// All should remain unchanged.
	for i, f := range findings {
		if f.Confidence != 0.80 {
			t.Fatalf("finding %d: expected unchanged confidence for bad positions, got %f", i, f.Confidence)
		}
		if f.ProximityBoost != 0 {
			t.Fatalf("finding %d: expected zero ProximityBoost for bad positions", i)
		}
	}
}

// ── Window tests ────────────────────────────────────────────────────────────

func TestWindow_CreditCardNearContext(t *testing.T) {
	text := "here is my credit card 4532015112830366 for payment"
	matches := Window(text, `(?i)credit\s*card`, `\b\d{13,19}\b`, 5)

	if len(matches) == 0 {
		t.Fatal("expected window to find credit card number near 'credit card'")
	}
	if matches[0].Text != "4532015112830366" {
		t.Fatalf("expected '4532015112830366', got %q", matches[0].Text)
	}
}

func TestWindow_TooFarApart(t *testing.T) {
	text := "credit card is one two three four five six seven eight nine ten words away 4532015112830366"
	matches := Window(text, `(?i)credit\s*card`, `\b\d{13,19}\b`, 5)

	if len(matches) != 0 {
		t.Fatalf("expected no matches when too far, got %d", len(matches))
	}
}

func TestWindow_MultipleTargets(t *testing.T) {
	text := "credit card 4532015112830366 and another credit card 6011111111111117"
	matches := Window(text, `(?i)credit\s*card`, `\b\d{13,19}\b`, 5)

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestWindow_EmptyInputs(t *testing.T) {
	if Window("", "a", "b", 1) != nil {
		t.Fatal("expected nil on empty text")
	}
	if Window("x", "", "b", 1) != nil {
		t.Fatal("expected nil on empty context pattern")
	}
	if Window("x", "a", "", 1) != nil {
		t.Fatal("expected nil on empty target pattern")
	}
}

func TestWindow_InvalidRegex(t *testing.T) {
	// Unclosed group should cause compile error → nil result.
	if Window("text", `(?i)(unclosed`, `\d+`, 5) != nil {
		t.Fatal("expected nil on invalid context regex")
	}
}

// ── Word utilities tests ────────────────────────────────────────────────────

func TestWordRanges(t *testing.T) {
	ranges := wordRanges("hello world, this is a test!")
	if len(ranges) != 6 {
		t.Fatalf("expected 6 words, got %d", len(ranges))
	}
}

func TestWordRanges_WithUnderscores(t *testing.T) {
	ranges := wordRanges("api_key is secret_token")
	if len(ranges) != 3 { // api_key, is, secret_token
		t.Fatalf("expected 3 words, got %d", len(ranges))
	}
}

func TestWordIndexByByte(t *testing.T) {
	text := "hello world test"
	words := wordRanges(text)

	idx := wordIndexByByte(words, 1) // inside "hello"
	if idx != 0 {
		t.Fatalf("expected word index 0, got %d", idx)
	}

	idx = wordIndexByByte(words, 7) // inside "world"
	if idx != 1 {
		t.Fatalf("expected word index 1, got %d", idx)
	}
}

func TestWordIndexByByte_BetweenWords(t *testing.T) {
	text := "hello world test"
	words := wordRanges(text)

	// Space between "hello" and "world" → closest preceding word is "hello" (0).
	idx := wordIndexByByte(words, 5)
	if idx != 0 {
		t.Fatalf("expected word index 0 (closest to space), got %d", idx)
	}
}

func TestScoreProximity_ConfidenceScoreUpdatedCorrectly(t *testing.T) {
	text := "ssn 123-45-6789"
	cs := scanner.ConfidenceScore{
		Total: 70,
		Breakdown: scanner.ConfidenceFactor{
			Format:    30,
			Algorithm: 20,
			Database:  0,
			Context:   20,
		},
		Action:    "REDACT",
		Reasoning: "test reasoning",
	}
	findings := []scanner.Finding{
		{
			Type:            "pii",
			Category:        "ssn",
			Match:           "123-45-6789",
			StartPos:        4,
			EndPos:          15,
			Confidence:      0.70,
			ConfidenceScore: &cs,
		},
	}

	ScoreProximity(text, findings)

	f := &findings[0]
	expectedConf := 0.90 // 0.70 + 0.20
	if math.Abs(f.Confidence-expectedConf) > 0.0001 {
		t.Fatalf("expected confidence %f, got %f", expectedConf, f.Confidence)
	}
	if f.ProximityBoost != 0.20 {
		t.Fatalf("expected ProximityBoost 0.20, got %f", f.ProximityBoost)
	}
	if f.ConfidenceScore.Total != 90 {
		t.Fatalf("expected Total 90, got %d", f.ConfidenceScore.Total)
	}
	if f.ConfidenceScore.Action != "BLOCK" {
		t.Fatalf("expected action BLOCK, got %s", f.ConfidenceScore.Action)
	}
}
