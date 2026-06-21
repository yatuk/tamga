package normalize

import (
	"strings"
	"testing"
)

// ── isSingleLetter ─────────────────────────────────────────────────────────

func TestIsSingleLetter(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"a", true},
		{"Z", true},
		{"ab", false},
		{"1", false},
		{"", false},
		{"ğ", false}, // multi-byte, not ASCII letter
	}
	for _, tt := range tests {
		got := isSingleLetter(tt.token)
		if got != tt.want {
			t.Errorf("isSingleLetter(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

// ── splitRunLen ────────────────────────────────────────────────────────────

func TestSplitRunLen(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantAt int // expected run length at position 0
	}{
		{"3 single letters", "a b c", 5},                 // a, , b, , c = 5 tokens
		{"6 single letters (ignore)", "i g n o r e", 11}, // 6 letters + 5 spaces = 11 tokens
		{"2 single letters (below threshold)", "a b", 0}, // only 2 letters < 3
		{"multi-letter word breaks run", "a b hello", 0}, // hello is not single letter
		{"double space breaks run", "a  b  c", 0},        // double space is not " "
		{"number breaks run", "a 1 b", 0},                // "1" is not a letter
		{"empty string start", "", 0},
	}
	for _, tt := range tests {
		tokens := tokenize(tt.input)
		got := splitRunLen(tokens, 0)
		if got != tt.wantAt {
			t.Errorf("[%s] splitRunLen(%q) = %d, want %d (tokens=%v)",
				tt.name, tt.input, got, tt.wantAt, tokens)
		}
	}
}

// ── collapseSplitWords — main function ────────────────────────────────────

func TestCollapseSplitWords(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		// Character-by-character splitting → collapsed
		{"ignore split", "i g n o r e", "ignore"},
		{"previous split", "p r e v i o u s", "previous"},
		{"system split", "s y s t e m", "system"},
		{"please ignore split", "p l e a s e i g n o r e", "pleaseignore"},
		// Note: "please ignore" → "pleaseignore" because the run includes all letters.
		// The space between "please" and "ignore" is also collapsed since both
		// sides are single letters.

		// Below threshold (only 2 single letters) → unchanged
		{"two single letters unchanged", "a b", "a b"},
		{"I am unchanged", "I am", "I am"},
		{"a dog unchanged", "a dog", "a dog"},

		// Multi-word text: only the split run is collapsed
		{"mixed text", "hello i g n o r e world", "hello ignore world"},

		// Multi-letter words act as boundaries
		{"multi-letter boundary", "please ignore previous", "please ignore previous"},

		// Mixed case preserved
		{"uppercase split", "I G N O R E", "IGNORE"},

		// Double spaces prevent collapsing the full run, but "c d e" is still 3
		// single letters → gets collapsed independently
		{"double space splits runs", "a b  c d e", "a b  cde"},

		// Empty input
		{"empty", "", ""},
		{"no letters", "123 456", "123 456"},
	}
	for _, tt := range tests {
		got := collapseSplitWords(tt.input)
		if got != tt.expect {
			t.Errorf("[%s]\n  input=%q\n  got=%q\n  want=%q", tt.name, tt.input, got, tt.expect)
		} else {
			t.Logf("[%s] OK: %q → %q", tt.name, tt.input, got)
		}
	}
}

// ── collapseSplitWords — injection evasion scenarios ──────────────────────

func TestCollapseSplitWords_EvasionScenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expectIn string // collapsed variant should contain this
	}{
		{"ignore previous evasion", "i g n o r e p r e v i o u s", "ignoreprevious"},
		{"system prompt evasion", "s y s t e m p r o m p t", "systemprompt"},
		{"forget instructions evasion", "f o r g e t i n s t r u c t i o n s", "forgetinstructions"},
		{"jailbreak evasion", "j a i l b r e a k", "jailbreak"},
		{"DAN mode evasion", "D A N m o d e", "DANmode"},
	}
	for _, tt := range tests {
		got := collapseSplitWords(tt.input)
		if !strings.Contains(got, tt.expectIn) {
			t.Errorf("[%s] FAIL\n  input=%q\n  got=%q\n  expected to contain %q",
				tt.name, tt.input, got, tt.expectIn)
		} else {
			t.Logf("[%s] OK: %q → %q", tt.name, tt.input, got)
		}
	}
}

// ── End-to-end: Apply pipeline with CollapseSplits enabled ─────────────────

func TestApply_CollapseSplitsIntegration(t *testing.T) {
	opts := Default()
	opts.CollapseSplits = true

	tests := []struct {
		name     string
		input    string
		expectIn string
	}{
		{"split ignore detected", "i g n o r e previous", "ignore"},
		{"split system detected", "show s y s t e m prompt", "system"},
		{"normal text unchanged", "hello world", "hello world"},
	}
	for _, tt := range tests {
		r := Apply(tt.input, opts)
		full := r.Text()
		if !strings.Contains(full, tt.expectIn) {
			t.Errorf("[%s] FAIL\n  input=%q\n  canonical=%q\n  decoded=%v\n  expected %q in combined text",
				tt.name, tt.input, r.Canonical, r.Decoded, tt.expectIn)
		} else {
			t.Logf("[%s] OK — canonical=%q decoded=%v", tt.name, r.Canonical, r.Decoded)
		}
	}
}

// ── CollapseSplits disabled does nothing ──────────────────────────────────

func TestApply_CollapseSplitsDisabled(t *testing.T) {
	opts := Default()
	opts.CollapseSplits = false

	input := "i g n o r e previous"
	r := Apply(input, opts)
	full := r.Text()
	if strings.Contains(full, "ignore") {
		t.Errorf("CollapseSplits disabled but 'ignore' found: canonical=%q decoded=%v", r.Canonical, r.Decoded)
	}
}
