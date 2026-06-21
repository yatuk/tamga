package normalize

import (
	"strings"
	"testing"
)

// ── AWS Access Keys ────────────────────────────────────────────────────────

func TestNormalizeCredentials_AWS(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string // expected clean key in output
	}{
		{"spaces in body", "AKIA IOSF ODNN 7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"dashes in body", "AKIA-IOSF-ODNN-7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"plus signs", "AKIA + IOSFODNN + 7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"mixed separators", "AKIA IOSF-ODNN+7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"ASIA prefix with spaces", "ASIA IOSF ODNN 7EXAMPLE", "ASIAIOSFODNN7EXAMPLE"},
		{"AROA prefix with dashes", "AROA-IOSF-ODNN-7EXAMPLE", "AROAIOSFODNN7EXAMPLE"},
		{"already clean — no change", "AKIAIOSFODNN7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"mid-sentence", "my key is AKIA IOSF ODNN 7EXAMPLE thanks", "AKIAIOSFODNN7EXAMPLE"},
	}
	for _, tt := range tests {
		got := normalizeCredentials(tt.input)
		if !strings.Contains(got, tt.expect) {
			t.Errorf("[%s]\n  input=%q\n  got=%q\n  expected to contain %q",
				tt.name, tt.input, got, tt.expect)
		} else {
			t.Logf("[%s] OK: %q → %q", tt.name, tt.input, got)
		}
	}
}

func TestNormalizeCredentials_AWS_IncompleteBody(t *testing.T) {
	// If fewer than 16 valid chars are found, the original text is preserved.
	tests := []string{
		"AKIA IOSF ODNN",   // only 9 body chars
		"AKIA",             // no body
		"AKIA---",          // separators but no body
		"not a key at all", // no prefix
	}
	for _, input := range tests {
		got := normalizeCredentials(input)
		if got != input {
			t.Errorf("incomplete body should be unchanged: %q → %q", input, got)
		}
	}
}

// ── GitHub PAT ─────────────────────────────────────────────────────────────

func TestNormalizeCredentials_GitHub(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"ghp_ with spaces", "ghp_ xxxx yyyy zzzz aaaa bbbb cccc dddd eeee ffff", "ghp_xxxxyyyyzzzzaaaabbbbccccddddeeeeffff"},
		{"ghp_ with dashes", "ghp_xxxx-yyyy-zzzz-aaaa-bbbb-cccc-dddd-eeee-ffff", "ghp_xxxxyyyyzzzzaaaabbbbccccddddeeeeffff"},
		{"ghs_ prefix", "ghs_ aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii", "ghs_aaaabbbbccccddddeeeeffffgggghhhhiiii"},
		// Short body (<36 chars) → unchanged (verified by IncompleteBody test)
		{"ghu_ prefix", "ghu_ xxxx yyyy zzzz aaaa bbbb cccc dddd eeee ffff gggg", "ghu_xxxxyyyyzzzzaaaabbbbccccddddeeeeffffgggg"},
		{"already clean", "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
	}
	for _, tt := range tests {
		got := normalizeCredentials(tt.input)
		if !strings.Contains(got, tt.expect) {
			t.Errorf("[%s]\n  input=%q\n  got=%q\n  expected to contain %q",
				tt.name, tt.input, got, tt.expect)
		} else {
			t.Logf("[%s] OK: %q → (contains) %q", tt.name, tt.input, tt.expect)
		}
	}
}

func TestNormalizeCredentials_GitHub_IncompleteBody(t *testing.T) {
	// Fewer than 36 body chars → unchanged
	input := "ghp_ too short"
	got := normalizeCredentials(input)
	if got != input {
		t.Errorf("short GitHub body should be unchanged: %q → %q", input, got)
	}
}

// ── Stripe ─────────────────────────────────────────────────────────────────

func TestNormalizeCredentials_Stripe(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"sk_live_ with spaces", "sk_live_ aaaa bbbb cccc dddd eeee ffff", "sk_live_aaaabbbbccccddddeeeeffff"},
		{"sk_test_ with dashes", "sk_test_-aaaa-bbbb-cccc-dddd-eeee-ffff", "sk_test_aaaabbbbccccddddeeeeffff"},
		{"pk_live_ with plus", "pk_live_ + aaaa + bbbb + cccc + dddd + eeee + ffff", "pk_live_aaaabbbbccccddddeeeeffff"},
		{"already clean", "sk_live_aaaabbbbccccddddeeeeffff", "sk_live_aaaabbbbccccddddeeeeffff"},
	}
	for _, tt := range tests {
		got := normalizeCredentials(tt.input)
		if !strings.Contains(got, tt.expect) {
			t.Errorf("[%s]\n  input=%q\n  got=%q\n  expected to contain %q",
				tt.name, tt.input, got, tt.expect)
		} else {
			t.Logf("[%s] OK: %q → (contains) %q", tt.name, tt.input, tt.expect)
		}
	}
}

// ── Anthropic / OpenAI ─────────────────────────────────────────────────────

func TestNormalizeCredentials_Anthropic(t *testing.T) {
	input := "sk-ant- api03 - xxxx yyyy zzzz aaaa bbbb"
	got := normalizeCredentials(input)
	expect := "sk-ant-api03-xxxxyyyyzzzzaaaabbbb"
	if !strings.Contains(got, expect) {
		t.Errorf("Anthropic: input=%q\n  got=%q\n  expected to contain %q", input, got, expect)
	} else {
		t.Logf("Anthropic OK: %q → (contains) %q", input, expect)
	}
}

func TestNormalizeCredentials_OpenAI(t *testing.T) {
	// sk- prefix but NOT sk-ant- (that goes to Anthropic handler)
	input := "sk- proj xxxx yyyy zzzz aaaa bbbb"
	got := normalizeCredentials(input)
	expect := "sk-projxxxxyyyyzzzzaaaabbbb"
	if !strings.Contains(got, expect) {
		t.Errorf("OpenAI: input=%q\n  got=%q\n  expected to contain %q", input, got, expect)
	} else {
		t.Logf("OpenAI OK: %q → (contains) %q", input, expect)
	}
}

// ── Multiple credentials in one string ─────────────────────────────────────

func TestNormalizeCredentials_Multiple(t *testing.T) {
	input := "AWS: AKIA IOSF ODNN 7EXAMPLE and GitHub: ghp_ aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj"
	got := normalizeCredentials(input)
	if !strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("missing AWS key in: %q", got)
	}
	if !strings.Contains(got, "ghp_aaaabbbbccccddddeeeeffffgggghhhhiiiijjjj") {
		t.Errorf("missing GitHub key in: %q", got)
	}
	t.Logf("Multiple OK: %q", got)
}

// ── No false positives on normal text ──────────────────────────────────────

func TestNormalizeCredentials_BenignText(t *testing.T) {
	benign := []string{
		"hello world",
		"this is a normal sentence",
		"A K I A is not a key prefix",
		"ghp without underscore is not a prefix",
		"sk_live without enough chars is harmless",
		"", // empty
	}
	for _, input := range benign {
		got := normalizeCredentials(input)
		if got != input {
			t.Errorf("benign text changed: %q → %q", input, got)
		}
	}
}

// ── End-to-end pipeline integration ───────────────────────────────────────

func TestApply_CredentialsIntegration(t *testing.T) {
	opts := Default()
	opts.NormalizeCredentials = true

	tests := []struct {
		name     string
		input    string
		expectIn string
	}{
		{"AWS spaces", "AKIA IOSF ODNN 7EXAMPLE", "AKIAIOSFODNN7EXAMPLE"},
		{"GitHub dashes (36 body)", "ghp_ aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii", "ghp_aaaabbbbccccddddeeeeffffgggghhhhiiii"},
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

// ── Option disabled ────────────────────────────────────────────────────────

func TestApply_CredentialsDisabled(t *testing.T) {
	opts := Default()
	opts.NormalizeCredentials = false

	input := "AKIA IOSF ODNN 7EXAMPLE"
	r := Apply(input, opts)
	full := r.Text()
	if strings.Contains(full, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("NormalizeCredentials disabled but clean key found: %q", full)
	}
}
