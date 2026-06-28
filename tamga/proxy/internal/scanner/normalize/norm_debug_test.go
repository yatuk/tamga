package normalize

import (
	"strings"
	"testing"
)

func TestNormalizeDigitGroups_StripSeparators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expectIn string // at least one variant must contain this
	}{
		// Valid TCKN: 10000000146 (11 digits).
		{"Dotted TCKN", "1.0000.0001.46", "10000000146"},
		{"Spaced TCKN", "1 0 0 0 0 0 0 0 1 4 6", "10000000146"},
		{"Dashed TCKN", "10000-0001-46", "10000000146"},
		{"Mixed separators", "1.0 0-0 0.0 0 0-1.4 6", "10000000146"},
		{"No separators (pass-through)", "10000000146", "10000000146"},
		{"Email dots preserved", "user@example.com", "user@example.com"},
	}
	for _, tt := range tests {
		variants := NormalizeDigitGroups(tt.input)
		found := false
		for _, v := range variants {
			if strings.Contains(v, tt.expectIn) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("[%s] FAIL\n  input=%q\n  variants=%v\n  expected to contain %q",
				tt.name, tt.input, variants, tt.expectIn)
		} else {
			t.Logf("[%s] OK — variants=%v", tt.name, variants)
		}
	}
}

func TestNormalizeDigitGroups_ReversedDigits(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expectIn string
	}{
		{"Reversed TCKN", "64100000001", "10000000146"},
		// 641.000.000.01 → stripped "64100000001" (11 digits) → reversed "10000000146"
		{"Dotted reversed TCKN", "641.000.000.01", "10000000146"},
		// Non-digit boundaries prevent concatenation; the 11-digit block reverses.
		{"Only reverses 11-digit blocks", "abc 64100000001 xyz", "10000000146"},
		{"Non-11-digit unchanged", "1234567890", "1234567890"},
		{"12-digit unchanged", "123456789012", "123456789012"},
	}
	for _, tt := range tests {
		variants := NormalizeDigitGroups(tt.input)
		found := false
		for _, v := range variants {
			if strings.Contains(v, tt.expectIn) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("[%s] FAIL\n  input=%q\n  variants=%v\n  expected to contain %q",
				tt.name, tt.input, variants, tt.expectIn)
		} else {
			t.Logf("[%s] OK — variants=%v", tt.name, variants)
		}
	}
}

func TestNormalizeDigitGroups_Standard(t *testing.T) {
	// Plain text with no digits should produce exactly 1 variant.
	variants := NormalizeDigitGroups("hello world")
	if len(variants) != 1 {
		t.Errorf("expected 1 variant, got %d: %v", len(variants), variants)
	}
	if variants[0] != "hello world" {
		t.Errorf("expected unchanged, got %q", variants[0])
	}
}

func TestStripSeparatorsBetweenDigits_Edges(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"", ""},
		{"abc", "abc"},
		{"1.2", "12"},
		{"1.2.3", "123"},
		{"1.2.3.4.5.6.7.8.9.0.1", "12345678901"},
		{"hello.world", "hello.world"}, // dot between letters, not digits
		// 11-digit TCKN with mixed separators → clean 11-digit string.
		{"1.0-0 0.0 0 0 0-1.4 6", "10000000146"},
		// Digits with separators then non-digit: dash is stripped (prevDigit=true).
		{"v1.2.3-release", "v123release"},
	}
	for _, tt := range tests {
		got := stripSeparatorsBetweenDigits(tt.input)
		if got != tt.expect {
			t.Errorf("stripSeparatorsBetweenDigits(%q) = %q, want %q", tt.input, got, tt.expect)
		}
	}
}

func TestDebug_BypassVectors(t *testing.T) {
	opts := Default()
	tests := []struct {
		name     string
		input    string
		expectIn string
	}{
		{"Math bold TCKN", "My ID is 𝟏𝟎𝟎𝟎𝟎𝟎𝟎𝟎𝟏𝟒𝟔", "10000000146"},
		{"Fullwidth @ email", "user＠example.com", "user@example.com"},
		{"ZWSP in TCKN", "1​0000000146", "10000000146"},
		{"Base64 email", "dXNlckBleGFtcGxlLmNvbQ==", "user@example.com"},
		{"Plain TCKN baseline", "10000000146", "10000000146"},
		{"ZWSP email", "user​@example.com", "user@example.com"},
		{"HTML entities", "&#117;&#115;&#101;&#114;&#64;example.com", "user@example.com"},
	}
	for _, tt := range tests {
		r := Apply(tt.input, opts)
		full := r.Text()
		found := strings.Contains(r.Canonical, tt.expectIn) || strings.Contains(full, tt.expectIn)
		if !found {
			t.Errorf("[%s] FAIL\n  input=%q\n  canonical=%q\n  decoded=%v\n  expected to contain %q",
				tt.name, tt.input, r.Canonical, r.Decoded, tt.expectIn)
		} else {
			t.Logf("[%s] OK — canonical=%q", tt.name, r.Canonical)
		}
	}
}
