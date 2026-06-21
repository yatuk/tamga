package scanner

import (
	"context"
	"strings"
	"testing"
)

func TestVerifyAWSAccessKeyID(t *testing.T) {
	pos := []string{
		"AKIAIOSFODNN7EXAMPLE",
		"ASIAY34FZKBOKMXTHY3Q",
		"A3T0SFODNN7EXAMPLE12",
	}
	neg := []string{
		"AKIAIOSFODNN7EXAMPL",
		"XKIAIOSFODNN7EXAMPLE",
		"AKIAIOSFODNN7EXAMPL!",
	}
	for _, s := range pos {
		if !verifyAWSAccessKeyID(s) {
			t.Errorf("want valid AWS access key id: %q", s)
		}
	}
	for _, s := range neg {
		if verifyAWSAccessKeyID(s) {
			t.Errorf("want invalid AWS access key id: %q", s)
		}
	}
}

func TestVerifyGitHubToken(t *testing.T) {
	pos := []string{
		"ghp_" + strings.Repeat("a", 36),
		"gho_" + strings.Repeat("b", 36),
		"ghs_" + strings.Repeat("c", 40),
	}
	neg := []string{
		"ghx_" + strings.Repeat("a", 36),
		"ghp_" + strings.Repeat("a", 30),
		"token " + strings.Repeat("a", 40),
	}
	for _, s := range pos {
		if !verifyGitHubToken(s) {
			t.Errorf("want valid GitHub token: %q", s)
		}
	}
	for _, s := range neg {
		if verifyGitHubToken(s) {
			t.Errorf("want invalid GitHub token: %q", s)
		}
	}
}

func TestVerifyOpenAIKey(t *testing.T) {
	pos := []string{
		"sk-" + strings.Repeat("a", 48),
		"sk-proj-" + strings.Repeat("b", 40),
	}
	neg := []string{
		"sk-ant-" + strings.Repeat("a", 80),
		"sk-" + strings.Repeat("x", 10),
		"pk-live-" + strings.Repeat("a", 40),
	}
	for _, s := range pos {
		if !verifyOpenAIKey(s) {
			t.Errorf("want valid OpenAI-style key: %q", s)
		}
	}
	for _, s := range neg {
		if verifyOpenAIKey(s) {
			t.Errorf("want invalid OpenAI key: %q", s)
		}
	}
}

func TestVerifyAnthropicKey(t *testing.T) {
	pos := []string{"sk-ant-api03-" + strings.Repeat("x", 80)}
	neg := []string{"sk-ant-" + strings.Repeat("x", 10), "sk-proj-" + strings.Repeat("a", 40)}
	for _, s := range pos {
		if !verifyAnthropicKey(s) {
			t.Errorf("want valid Anthropic key: %q", s)
		}
	}
	for _, s := range neg {
		if verifyAnthropicKey(s) {
			t.Errorf("want invalid Anthropic key: %q", s)
		}
	}
}

func TestVerifyStripeKey(t *testing.T) {
	pos := []string{
		"sk_live_" + strings.Repeat("a", 24),
		"pk_test_" + strings.Repeat("b", 24),
	}
	neg := []string{
		"sk_live_" + strings.Repeat("a", 10),
		"sk_life_" + strings.Repeat("a", 24),
	}
	for _, s := range pos {
		if !verifyStripeKey(s) {
			t.Errorf("want valid Stripe key: %q", s)
		}
	}
	for _, s := range neg {
		if verifyStripeKey(s) {
			t.Errorf("want invalid Stripe key: %q", s)
		}
	}
}

func TestVerifyJWTFormat(t *testing.T) {
	// Minimal valid JWT-shaped token (header.payload.sig) with decodable base64url segments.
	pos := []string{
		// jwt.io sample (format-only; not a real secret)
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
	}
	neg := []string{
		"not.a.jwt",
		"eyJ!!!.eyJ!!!.bad!!!",
	}
	for _, s := range pos {
		if !verifyJWTFormat(s) {
			t.Errorf("want valid JWT format: %q", s)
		}
	}
	for _, s := range neg {
		if verifyJWTFormat(s) {
			t.Errorf("want invalid JWT format: %q", s)
		}
	}
}

func TestSecretScanner_AcceptsSyntheticSecrets(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()

	body := `MY_API_SECRET=` + strings.Repeat("Z", 32) + `
aws_key=AKIAIOSFODNN7EXAMPLE
AWS_SECRET_ACCESS_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD
github=ghp_` + strings.Repeat("a", 36) + `
openai=sk-` + strings.Repeat("b", 48) + `
anthropic=sk-ant-api03-` + strings.Repeat("c", 80) + `
stripe=sk_live_` + strings.Repeat("d", 24) + `
jwt=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c
slack=https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX
conn=postgres://appuser:SuperSecretDbPass9@db.internal:5432/mydb?sslmode=require
`
	fs, err := s.Scan(ctx, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	cats := map[string]int{}
	for _, f := range fs {
		if f.Type == "secret" {
			cats[f.Category]++
		}
	}
	want := []string{
		"aws_access_key", "aws_secret_key", "github_token", "openai_key",
		"anthropic_key", "stripe_key", "jwt_token", "slack_webhook",
		"connection_string", "connection_string_password", "dotenv_secret",
	}
	for _, w := range want {
		if cats[w] < 1 {
			t.Errorf("missing category %q in findings: %#v", w, cats)
		}
	}
}

func TestSecretScanner_ConnectionStringPasswordSpan(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()
	raw := []byte(`uri postgres://u:P4ssw0rdOnlyHere@host.example/db`)
	fs, err := s.Scan(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	var pass *Finding
	for i := range fs {
		if fs[i].Category == "connection_string_password" {
			pass = &fs[i]
			break
		}
	}
	if pass == nil {
		t.Fatal("expected connection_string_password")
	}
	got := string(raw[pass.StartPos:pass.EndPos])
	if got != "P4ssw0rdOnlyHere" {
		t.Fatalf("password span: want P4ssw0rdOnlyHere, got %q", got)
	}
}

func TestSecretScanner_PrivateKeyPEM(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()
	pem := `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBALfakefakefakefakefakefakefakefakefakefakefakefakefakefake
morelinesmorelinesmorelinesmorelinesmorelinesmorelinesmorelinesmorelines
-----END RSA PRIVATE KEY-----`
	fs, err := s.Scan(ctx, []byte(pem))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSecretCat(fs, "private_key") {
		t.Fatal("expected private_key finding")
	}
}

func TestSecretScanner_RejectsBadJWT(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()
	fs, _ := s.Scan(ctx, []byte(`tok eyJhbGciOiJIUzI1NiJ9.!!!not-base64!!!.!!!not-base64!!!`))
	for _, f := range fs {
		if f.Category == "jwt_token" {
			t.Fatal("should not accept invalid JWT segments")
		}
	}
}

func TestSecretScanner_AWSSecretKeyBroad(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()

	// Broad context patterns (WITHOUT "access" — catches what strict misses).
	tests := []struct {
		name string
		body string
		want string // category
	}{
		{"AWS_SECRET_KEY env", "AWS_SECRET_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD", "aws_secret_key_broad"},
		{"aws.secretKey config", "aws.secretKey = \"abcdefghijklmnopqrstuvwxyz0123456789ABCD\"", "aws_secret_key_broad"},
		{"amazon_secret_token", "amazon_secret_token: abcdefghijklmnopqrstuvwxyz0123456789ABCD", "aws_secret_key_broad"},
		{"AWS_PRIVATE_KEY", "AWS_PRIVATE_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD", "aws_secret_key_broad"},
		{"aws secret key=value", "aws_secret_key = abcdefghijklmnopqrstuvwxyz0123456789ABCD", "aws_secret_key_broad"},
		{"amazon private credential", "amazon_private_credential: abcdefghijklmnopqrstuvwxyz0123456789ABCD", "aws_secret_key_broad"},
		// Strict pattern (WITH "access") still matches aws_secret_key (not broad)
		{"AWS_SECRET_ACCESS_KEY uses strict", "AWS_SECRET_ACCESS_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD", "aws_secret_key"},
	}
	for _, tt := range tests {
		fs, err := s.Scan(ctx, []byte(tt.body))
		if err != nil {
			t.Fatal(err)
		}
		if !hasSecretCat(fs, tt.want) {
			t.Errorf("[%s] missing category %q in findings", tt.name, tt.want)
			for _, f := range fs {
				t.Logf("  found: type=%s cat=%s match=%s", f.Type, f.Category, f.Match)
			}
		}
	}
}

func TestSecretScanner_AWSSecretKeyRaw(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()

	// Raw 40-char base64 string — should be detected as low severity.
	rawKey := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	body := "here is a raw key: " + rawKey + " in some logs"
	fs, err := s.Scan(ctx, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSecretCat(fs, "aws_secret_key_raw") {
		t.Error("raw 40-char key not detected")
	}
	// Verify low severity (WARN only).
	for _, f := range fs {
		if f.Category == "aws_secret_key_raw" && f.Severity != "low" {
			t.Errorf("raw key should have low severity, got %s", f.Severity)
		}
	}
}

func TestSecretScanner_AWSSecretKeyRaw_NoFalseOnShort(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()

	// Strings shorter or longer than exactly 40 chars should NOT match.
	benign := []string{
		"short39chars" + string(make([]byte, 39-13)), // 39 chars
		"a really long string that is more than 40 chars but not exactly 40",
	}
	for _, b := range benign {
		fs, _ := s.Scan(ctx, []byte(b))
		if hasSecretCat(fs, "aws_secret_key_raw") {
			t.Errorf("short/long string should not match raw: %q", b[:40])
		}
	}
}

func TestSecretScanner_AWSSecretKey_NoDoubleCount(t *testing.T) {
	s := NewSecretScanner()
	ctx := context.Background()

	// AWS_SECRET_ACCESS_KEY should match strict only, not broad.
	body := "AWS_SECRET_ACCESS_KEY=abcdefghijklmnopqrstuvwxyz0123456789ABCD"
	fs, err := s.Scan(ctx, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !hasSecretCat(fs, "aws_secret_key") {
		t.Error("strict pattern should match")
	}
	if hasSecretCat(fs, "aws_secret_key_broad") {
		t.Error("broad pattern should NOT match when 'access' is present")
	}
	// Raw pattern will also match the 40-char body, creating 2 findings total.
	// This is expected: strict + raw are different categories.
}

func hasSecretCat(fs []Finding, cat string) bool {
	for _, f := range fs {
		if f.Type == "secret" && f.Category == cat {
			return true
		}
	}
	return false
}
