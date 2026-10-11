package scanner

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestValidLuhn(t *testing.T) {
	pos := []string{
		"4532015112830366",
		"5555555555554444",
		"378282246310005",
	}
	neg := []string{
		"4532015112830367",
		"1234567890123456",
		"4222222222222223",
	}
	for _, s := range pos {
		if !validLuhn(s) {
			t.Errorf("expected Luhn valid: %q", s)
		}
	}
	for _, s := range neg {
		if validLuhn(s) {
			t.Errorf("expected Luhn invalid: %q", s)
		}
	}
}

func TestValidTCKN(t *testing.T) {
	pos := []string{
		"10000000078",
		"12345678950",
		"11111111110",
	}
	neg := []string{
		"10000000077",
		"11111111111",
		"12345678901",
	}
	for _, s := range pos {
		if !validTCKN(s) {
			t.Errorf("expected TCKN valid: %q", s)
		}
	}
	for _, s := range neg {
		if validTCKN(s) {
			t.Errorf("expected TCKN invalid: %q", s)
		}
	}
}

func TestValidIBAN(t *testing.T) {
	pos := []string{
		"GB82 WEST 1234 5698 7654 32",
		"TR33 0006 1005 1978 6457 8413 26",
		"DE89370400440532013000",
	}
	neg := []string{
		"GB82 WEST 1234 5698 7654 31",
		"XX00 0000 0000 0000",
		"TR12",
	}
	for _, s := range pos {
		if !validIBAN(s) {
			t.Errorf("expected IBAN valid: %q", s)
		}
	}
	for _, s := range neg {
		if validIBAN(s) {
			t.Errorf("expected IBAN invalid: %q", s)
		}
	}
}

func TestValidTRMobile(t *testing.T) {
	pos := []string{
		"+90 532 123 45 67",
		"0532 123 45 67",
		"+905321234567",
	}
	neg := []string{
		"+1 555 123 4567",
		"+44 20 7946 0958",
		"1234567890",
	}
	for _, s := range pos {
		if !validTRPhone(s) {
			t.Errorf("expected TR mobile valid: %q", s)
		}
	}
	for _, s := range neg {
		if validTRPhone(s) {
			t.Errorf("expected TR mobile invalid: %q", s)
		}
	}
}

func TestValidEmailRFC5322Pragmatic(t *testing.T) {
	pos := []string{
		"user.name+tag_sorting@example.com",
		"a@b.co",
		"customer/department=shipping@example.com",
	}
	neg := []string{
		"not@an@email",
		"@foo.com",
		"user@",
	}
	for _, s := range pos {
		if !validEmailRFC5322Pragmatic(s) {
			t.Errorf("expected email valid: %q", s)
		}
	}
	for _, s := range neg {
		if validEmailRFC5322Pragmatic(s) {
			t.Errorf("expected email invalid: %q", s)
		}
	}
}

func TestIsPrivateOrSpecialIPv4(t *testing.T) {
	pub := net.ParseIP("8.8.8.8")
	priv := net.ParseIP("192.168.0.1")
	if isPrivateOrSpecialIPv4(pub) {
		t.Error("8.8.8.8 should be public")
	}
	if !isPrivateOrSpecialIPv4(priv) {
		t.Error("192.168.0.1 should be private")
	}
}

func TestPIIScanner_CreditCard(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()

	pos := []string{
		"Pay with 4532015112830366 today",
		"Card: 5555-5555-5555-4444",
		"378282246310005 is Amex",
	}
	for _, txt := range pos {
		fs, err := s.Scan(ctx, []byte(txt))
		if err != nil {
			t.Fatal(err)
		}
		if !hasCategory(fs, "credit_card") {
			t.Errorf("want credit_card in %q, got %#v", txt, fs)
		}
	}
	neg := []string{
		"not a card 4532015112830367",
		"random 1234567890123456 digits",
		"no sixteen digit pan here",
	}
	for _, txt := range neg {
		fs, err := s.Scan(ctx, []byte(txt))
		if err != nil {
			t.Fatal(err)
		}
		if hasCategory(fs, "credit_card") {
			t.Errorf("should not match credit_card: %q", txt)
		}
	}
}

func TestPIIScanner_TCKimlik(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	pos := []string{"TC 10000000078", "id=12345678950"}
	neg := []string{"10000000707", "11111111111"}
	for _, txt := range pos {
		fs, _ := s.Scan(ctx, []byte(txt))
		if !hasCategory(fs, "tc_kimlik") {
			t.Errorf("want tc_kimlik in %q", txt)
		}
	}
	for _, txt := range neg {
		fs, _ := s.Scan(ctx, []byte(txt))
		if hasCategory(fs, "tc_kimlik") {
			t.Errorf("should not match tc_kimlik: %q", txt)
		}
	}
}

func TestPIIScanner_IBAN(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	pos := []string{"IBAN GB82 WEST 1234 5698 7654 32", "TR33 0006 1005 1978 6457 8413 26"}
	neg := []string{"IBAN GB82 WEST 1234 5698 7654 31", "XX00NOTANIBAN"}
	for _, txt := range pos {
		fs, _ := s.Scan(ctx, []byte(txt))
		if !hasCategory(fs, "iban") {
			t.Errorf("want iban in %q", txt)
		}
	}
	for _, txt := range neg {
		fs, _ := s.Scan(ctx, []byte(txt))
		if hasCategory(fs, "iban") {
			t.Errorf("should not match iban: %q", txt)
		}
	}
}

func TestPIIScanner_Email(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	pos := []string{"Contact a@b.co", "mail: user+tag@example.org"}
	neg := []string{"bad not@an@email", "nope @foo.com"}
	for _, txt := range pos {
		fs, _ := s.Scan(ctx, []byte(txt))
		if !hasCategory(fs, "email") {
			t.Errorf("want email in %q", txt)
		}
	}
	for _, txt := range neg {
		fs, _ := s.Scan(ctx, []byte(txt))
		if hasCategory(fs, "email") {
			t.Errorf("should not match email: %q", txt)
		}
	}
}

// The user name and password of a URL are not an e-mail address, though
// "password@example.com" has the shape of one. An address in a path, in a
// query, after "mailto:" or in the prose around a URL still is.
func TestPIIScanner_EmailNotInURLUserinfo(t *testing.T) {
	s := NewPIIScanner()
	tests := []struct {
		name, text string
		want       []string // the masked addresses found, in order
	}{
		{"user and password", "https://user:password@example.com", nil},
		{"user and password, with a path", "clone https://deploy:not-a-real-pw@git.example.invalid/org/repo.git please", nil},
		{"user only", "ssh://git@git.example.invalid/org/repo.git", nil},
		{"user only, http", "open http://admin@intranet.example.invalid/ now", nil},
		{"connection string", "postgres://app:not-a-real-pw@db.example.invalid:5432/app", nil},
		{"upper-case scheme", "HTTPS://User:Password@Example.COM/x", nil},
		{"in quotes", `url = "ftp://backup:not-a-real-pw@files.example.invalid"`, nil},
		{"mailto link", "write to mailto:ayse@example.com today", []string{maskContent("ayse@example.com")}},
		{"mailto in markdown", "[mail](mailto:ayse@example.com)", []string{maskContent("ayse@example.com")}},
		{"in a path", "https://example.com/users/ayse@example.com/profile", []string{"*"}},
		{"in a query", "https://example.com/reset?email=ayse@example.com", []string{"*"}},
		{"in a fragment", "https://example.com/#ayse@example.com", []string{"*"}},
		{"prose after a URL", "see https://example.com:8443/login and mail ayse@example.com", []string{maskContent("ayse@example.com")}},
		{"prose before a URL", "ayse@example.com sent https://user:password@example.com", []string{maskContent("ayse@example.com")}},
		{"address after the host", "https://user:password@example.com/?cc=ayse@example.com", []string{"*"}},
		{"scp-style remote is not a URL", "git clone git@git.example.invalid:org/repo.git", []string{maskContent("git@git.example.invalid")}},
		{"no scheme before the slashes", "//ayse@example.com", []string{"*"}},
	}
	for _, tt := range tests {
		fs, err := s.Scan(context.Background(), []byte(tt.text))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		var got []string
		seen := map[[2]int]bool{}
		for _, f := range fs {
			// The normalised view finds a plain address a second time.
			if f.Category != "email" || f.Metadata["detected_via"] != "" || seen[[2]int{f.StartPos, f.EndPos}] {
				continue
			}
			seen[[2]int{f.StartPos, f.EndPos}] = true
			got = append(got, f.Match)
		}
		if len(got) != len(tt.want) {
			t.Errorf("%s: e-mail findings %v, want %d", tt.name, got, len(tt.want))
			continue
		}
		for i := range got {
			// "*" stands for any match: the pattern takes the URL's path
			// or query into the local part, and that is not at issue here.
			if tt.want[i] != "*" && got[i] != tt.want[i] {
				t.Errorf("%s: finding %d is %q, want %q", tt.name, i, got[i], tt.want[i])
			}
		}
		// No view of a URL's userinfo is an address either.
		if len(tt.want) == 0 && hasCategory(fs, "email") {
			t.Errorf("%s: an e-mail finding from a normalised view: %+v", tt.name, fs)
		}
	}
}

// Dropping the e-mail finding must not leave the credentials of a URL
// unmarked: they are one secret finding with a position, which REDACT masks.
func TestURLCredentials_OneFindingAndMasked(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewPIIScanner())
	reg.Register(NewSecretScanner())
	text := "push to https://deploy:" + fill(mixed, 12) + "@registry.example.invalid/v2/ and tell ayse@example.com"
	findings, err := reg.ScanRaw(context.Background(), []byte(text), PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, f := range findings {
		seen[f.Type+"/"+f.Category]++
		if !f.Placed() {
			t.Errorf("%s/%s has no position: %+v", f.Type, f.Category, f)
		}
	}
	if seen["secret/url_credentials"] != 1 || seen["pii/email"] != 1 || len(findings) != 2 {
		t.Fatalf("findings %v, want one url_credentials and one email", seen)
	}
	out := string(RedactContent([]byte(text), findings))
	if out != "push to [url_credentials_REDACTED]/v2/ and tell [email_REDACTED]" {
		t.Errorf("masked: %s", out)
	}

	// A policy that masks PII and only warns about secrets masks the
	// address and leaves the URL as written. Before, the e-mail finding
	// masked the password by accident and left the user name.
	var piiOnly []Finding
	for _, f := range findings {
		if f.Type == "pii" {
			piiOnly = append(piiOnly, f)
		}
	}
	out = string(RedactContent([]byte(text), piiOnly))
	if !strings.Contains(out, "https://deploy:"+fill(mixed, 12)+"@registry.example.invalid") || !strings.Contains(out, "[email_REDACTED]") {
		t.Errorf("masked for PII only: %s", out)
	}

	// A user name without a password is not a secret, and not an address.
	findings, err = reg.ScanRaw(context.Background(), []byte("ssh://git@git.example.invalid/org/repo.git"), PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("a URL with a user name only: %+v", findings)
	}
}

func TestPIIScanner_PhoneTR(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	pos := []string{"Ara +90 532 123 45 67", "numara 0532 123 45 67"}
	neg := []string{"US +1 555 123 4567", "UK +44 20 7946 0958"}
	for _, txt := range pos {
		fs, _ := s.Scan(ctx, []byte(txt))
		if !hasCategory(fs, "phone_tr") {
			t.Errorf("want phone_tr in %q", txt)
		}
	}
	for _, txt := range neg {
		fs, _ := s.Scan(ctx, []byte(txt))
		if hasCategory(fs, "phone_tr") {
			t.Errorf("should not match phone_tr: %q", txt)
		}
	}
}

func TestPIIScanner_IPv4(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	pub := "dns 8.8.8.8"
	fs, _ := s.Scan(ctx, []byte(pub))
	if !hasCategory(fs, "ip_public") {
		t.Errorf("want ip_public: %#v", fs)
	}
	priv := "gw 192.168.1.1"
	fs, _ = s.Scan(ctx, []byte(priv))
	if !hasCategory(fs, "ip_private") {
		t.Errorf("want ip_private: %#v", fs)
	}
	neg := "not an ip 999.999.999.999"
	fs, _ = s.Scan(ctx, []byte(neg))
	if hasCategory(fs, "ip_public") || hasCategory(fs, "ip_private") {
		t.Errorf("should not match invalid IP: %#v", fs)
	}
}

func TestPIIScanner_SSN(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	pos := "ssn 123-45-6789"
	fs, _ := s.Scan(ctx, []byte(pos))
	if !hasCategory(fs, "ssn") {
		t.Fatal("expected ssn")
	}
	neg := "not 123456789"
	fs, _ = s.Scan(ctx, []byte(neg))
	if hasCategory(fs, "ssn") {
		t.Error("should not match dashed ssn pattern")
	}
}

func hasCategory(fs []Finding, cat string) bool {
	for _, f := range fs {
		if f.Type == "pii" && f.Category == cat {
			return true
		}
	}
	return false
}

func TestRedactContent_PreservesNonPII(t *testing.T) {
	s := NewPIIScanner()
	ctx := context.Background()
	raw := []byte("email user@example.com end")
	fs, _ := s.Scan(ctx, raw)
	out := RedactContent(raw, fs)
	if !strings.Contains(string(out), "[email_REDACTED]") {
		t.Fatalf("redact: %s", out)
	}
}

// TestRedactContent_MultipleFindings verifies that RedactContent correctly
// handles multiple PII findings in the same content without panicking or
// producing corrupted output (regression test for Bug 1: buffer overflow).
func TestRedactContent_MultipleFindings(t *testing.T) {
	// Content with email, credit card, and TCKN all in one string.
	content := []byte("Mail a@b.co to verify card 4532015112830366 and TCKN 10000000146 please")

	// Manually construct findings to avoid depending on scanner internals.
	// Byte positions verified against the literal content string.
	findings := []Finding{
		{Type: "pii", Category: "email", StartPos: 5, EndPos: 11},            // "a@b.co"
		{Type: "pii", Category: "credit_card", StartPos: 27, EndPos: 43},     // "4532015112830366"
		{Type: "pii", Category: "tc_kimlik", StartPos: 53, EndPos: 64},       // "10000000146"
		{Type: "injection", Category: "", StartPos: 0, EndPos: 3, Match: ""}, // a verdict, not a span: ignored
	}

	out := RedactContent(content, findings)

	outStr := string(out)

	// Verify placeholders are present.
	for _, ph := range []string{"[email_REDACTED]", "[credit_card_REDACTED]", "[tc_kimlik_REDACTED]"} {
		if !strings.Contains(outStr, ph) {
			t.Errorf("expected placeholder %q in output: %q", ph, outStr)
		}
	}

	// Verify raw PII values are absent.
	for _, raw := range []string{"a@b.co", "4532015112830366", "10000000146"} {
		if strings.Contains(outStr, raw) {
			t.Errorf("raw PII %q should not appear in redacted output: %q", raw, outStr)
		}
	}

	// Verify non-PII content is preserved.
	for _, phrase := range []string{"Mail", "to verify card", "and TCKN", "please"} {
		if !strings.Contains(outStr, phrase) {
			t.Errorf("expected non-PII phrase %q preserved in output: %q", phrase, outStr)
		}
	}
}
