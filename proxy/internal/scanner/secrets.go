package scanner

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/yatuk/tamga/internal/scanner/normalize"
)

type secretPattern struct {
	Name       string
	Regex      *regexp.Regexp
	Severity   string
	confidence float64
	validate   func(string) bool
}

var (
	secretPatterns []secretPattern
	secretOnce     sync.Once

	// Parsed once with compileSecretPatterns
	connStringRe *regexp.Regexp
	dotenvRe     *regexp.Regexp
)

func compileSecretPatterns() {
	// AWS access key ID: known 3-4 char prefix + 16 A-Z0-9 (20 chars total).
	awsAccessRe := `(?:A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}`

	secretPatterns = []secretPattern{
		{
			Name:       "aws_access_key",
			Regex:      regexp.MustCompile(`\b` + awsAccessRe + `\b`),
			Severity:   "critical",
			confidence: 0.96,
			validate:   verifyAWSAccessKeyID,
		},
		{
			Name:       "aws_secret_key",
			Regex:      regexp.MustCompile(`(?i)aws_?secret_?access_?key[\s:="']+([A-Za-z0-9/+=]{40})`),
			Severity:   "critical",
			confidence: 0.94,
			validate:   verifyAWSSecretValue,
		},
		{
			// Broader context WITHOUT "access": catches AWS_SECRET_KEY,
			// aws.secretKey, amazon_secret_token, etc.  The strict pattern
			// (above) handles aws_secret_access_key; this one covers the
			// remaining config/env/code patterns so they don't overlap.
			Name:       "aws_secret_key_broad",
			Regex:      regexp.MustCompile(`(?i)(?:aws|amazon)[\W_]*(?:secret|private)(?:[\W_]*(?:key|token|credential))?[\W_:=]+([A-Za-z0-9/+=]{40})`),
			Severity:   "critical",
			confidence: 0.90,
			validate:   verifyAWSSecretValue,
		},
		{
			// Raw 40-char base64-like string — high false-positive risk, so
			// severity stays low (WARN) and confidence is conservative.
			// This catches standalone secret keys that aren't accompanied by
			// an "aws" or "secret" label (e.g. in logs or plain-text dumps).
			Name:       "aws_secret_key_raw",
			Regex:      regexp.MustCompile(`\b[A-Za-z0-9/+=]{40}\b`),
			Severity:   "low",
			confidence: 0.55,
			validate:   verifyAWSSecretValue,
		},
		{
			Name:       "github_token",
			Regex:      regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{36,255}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyGitHubToken,
		},
		{
			Name:       "anthropic_key",
			Regex:      regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9_-]{20,}\b`),
			Severity:   "critical",
			confidence: 0.96,
			validate:   verifyAnthropicKey,
		},
		{
			Name:       "openai_key",
			Regex:      regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{20,}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyOpenAIKey,
		},
		{
			Name:       "stripe_key",
			Regex:      regexp.MustCompile(`\b(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{24,}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyStripeKey,
		},
		{
			Name:       "jwt_token",
			Regex:      regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
			Severity:   "high",
			confidence: 0.88,
			validate:   verifyJWTFormat,
		},
		{
			Name:       "private_key",
			Regex:      regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
			Severity:   "critical",
			confidence: 0.97,
			validate:   verifyPrivateKeyPEM,
		},
		{
			Name:       "generic_api_key",
			Regex:      regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|api_secret)[\s:="']+([A-Za-z0-9\-_.]{20,})`),
			Severity:   "high",
			confidence: 0.72,
			validate:   verifyGenericAPIValue,
		},
		{
			Name:       "slack_webhook",
			Regex:      regexp.MustCompile(`https://hooks\.slack\.com/services/[A-Z0-9]+/[A-Z0-9]+/[A-Za-z0-9]+`),
			Severity:   "high",
			confidence: 0.93,
			validate:   verifySlackWebhookURL,
		},
		{
			// A PEM header on its own: the body is often cut off or pasted
			// separately, and the header already says what follows. PGP
			// blocks and encrypted keys use the same framing.
			Name:       "private_key",
			Regex:      regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ ){0,2}PRIVATE KEY(?: BLOCK)?-----`),
			Severity:   "high",
			confidence: 0.85,
		},
		{
			// Fine-grained personal access tokens (github_pat_…); the
			// classic gh?_ prefixes are matched above.
			Name:       "github_token",
			Regex:      regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,255}\b`),
			Severity:   "critical",
			confidence: 0.95,
		},
		{
			// Slack bot, user, app and refresh tokens.
			Name:       "slack_token",
			Regex:      regexp.MustCompile(`\bxox[abeoprs]-[A-Za-z0-9]{6,}(?:-[A-Za-z0-9]{6,}){1,4}\b`),
			Severity:   "critical",
			confidence: 0.93,
		},
		{
			Name:       "google_api_key",
			Regex:      regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`),
			Severity:   "critical",
			confidence: 0.93,
		},
		{
			// Twilio account (AC) and API key (SK) identifiers: 32 hex digits.
			Name:       "twilio_sid",
			Regex:      regexp.MustCompile(`\b(?:AC|SK)[0-9a-f]{32}\b`),
			Severity:   "high",
			confidence: 0.88,
		},
		{
			// HTTP Basic credentials are a base64 "user:password" pair.
			Name:       "basic_auth",
			Regex:      regexp.MustCompile(`(?i)\bAuthorization:\s*Basic\s+[A-Za-z0-9+/]{8,}={0,2}`),
			Severity:   "critical",
			confidence: 0.93,
			validate:   verifyBasicAuth,
		},
		{
			// A password stated next to its label, in English or Turkish:
			// "password: hunter2", "parolam Yazilim123!", "şifresi: …".
			Name:       "password",
			Regex:      regexp.MustCompile(`(?i)\b(?:passwords?|passwd|pwd|parola|[şs]ifre)(?:m|n|s[ıi]|m[ıi]z|n[ıi]z)?(?:\s+is\b|\s*[:=])?\s+\S{5,64}`),
			Severity:   "high",
			confidence: 0.75,
			validate:   verifyPasswordValue,
		},
	}

	// Full URI through first path segment (password is group 3).
	connStringRe = regexp.MustCompile(`(?i)\b(postgres|mysql|mongodb|redis)(?:\+srv)?://([^:\s]+):([^@]+)@[^\s"'<>)\]]+`)
	dotenvRe = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*["']?([^\s#'"\r\n]+)`)
}

// SecretScanner detects API keys, tokens, passwords and other secrets.
type SecretScanner struct{}

// NewSecretScanner creates a scanner that detects hardcoded API keys and credentials.
func NewSecretScanner() *SecretScanner {
	secretOnce.Do(compileSecretPatterns)
	return &SecretScanner{}
}

func (s *SecretScanner) Name() string { return "secret" }

func (s *SecretScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	secretOnce.Do(compileSecretPatterns)
	var findings []Finding

	// Normalize to defeat Unicode confusables and zero-width splits.
	normResult := normalize.Apply(string(content), normalize.Default())
	normText := normResult.Text()
	normBytes := []byte(normText)

	for _, p := range secretPatterns {
		matches := p.Regex.FindAllIndex(content, -1)
		for _, loc := range matches {
			matched := string(content[loc[0]:loc[1]])
			if p.validate != nil && !p.validate(matched) {
				continue
			}
			findings = append(findings, makeSecretFinding(p.Name, p.Severity, matched, loc[0], loc[1], p.confidence))
		}
	}

	findings = append(findings, scanConnectionStrings(content)...)
	findings = append(findings, scanDotenvLines(content)...)

	// Re-scan normalized text for Unicode-evaded secrets.
	for _, p := range secretPatterns {
		for _, loc := range p.Regex.FindAllIndex(normBytes, -1) {
			matched := string(normBytes[loc[0]:loc[1]])
			if p.validate != nil && !p.validate(matched) {
				continue
			}
			f := makeSecretFinding(p.Name, p.Severity, matched, loc[0], loc[1], p.confidence)
			if f.Metadata == nil {
				f.Metadata = map[string]string{}
			}
			f.Metadata["detected_via"] = "unicode_normalization"
			findings = append(findings, f)
		}
	}
	findings = append(findings, scanConnectionStrings(normBytes)...)
	findings = append(findings, scanDotenvLines(normBytes)...)

	return findings, nil
}

func makeSecretFinding(name, severity, raw string, start, end int, conf float64) Finding {
	masked := maskSecret(raw)
	return Finding{
		Type:       "secret",
		Category:   name,
		Severity:   severity,
		Match:      masked,
		StartPos:   start,
		EndPos:     end,
		Confidence: conf,
	}
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:8] + "..."
}

// scanConnectionStrings emits a finding for the full URI and a separate finding for the password segment only.
func scanConnectionStrings(content []byte) []Finding {
	var out []Finding
	all := connStringRe.FindAllSubmatchIndex(content, -1)
	for _, m := range all {
		if len(m) < 8 {
			continue
		}
		fullStart, fullEnd := m[0], m[1]
		passStart, passEnd := m[6], m[7]
		raw := string(content[fullStart:fullEnd])
		pass := string(content[passStart:passEnd])
		if len(pass) == 0 || strings.Contains(pass, "://") {
			continue
		}
		out = append(out,
			makeSecretFinding("connection_string", "critical", raw, fullStart, fullEnd, 0.92),
			makeSecretFinding("connection_string_password", "critical", pass, passStart, passEnd, 0.94),
		)
	}
	return out
}

// scanDotenvLines matches KEY=VALUE lines where the key suggests a secret and the value looks like a credential.
func scanDotenvLines(content []byte) []Finding {
	var out []Finding
	all := dotenvRe.FindAllSubmatchIndex(content, -1)
	for _, m := range all {
		if len(m) < 6 {
			continue
		}
		keyStart, keyEnd := m[2], m[3]
		valStart, valEnd := m[4], m[5]
		key := string(content[keyStart:keyEnd])
		val := string(content[valStart:valEnd])
		if !dotenvKeyLooksSensitive(key) || !dotenvValueLooksSecret(val) {
			continue
		}
		raw := key + "=" + val
		out = append(out, makeSecretFinding("dotenv_secret", "high", raw, m[0], m[1], 0.80))
	}
	return out
}

func dotenvKeyLooksSensitive(key string) bool {
	u := strings.ToUpper(key)
	return strings.Contains(u, "SECRET") || strings.Contains(u, "PASSWORD") || strings.Contains(u, "TOKEN") ||
		strings.Contains(u, "API_KEY") || (strings.HasSuffix(u, "_KEY") && !strings.Contains(u, "PUBLIC")) ||
		(strings.Contains(u, "AUTH") && strings.Contains(u, "KEY"))
}

func dotenvValueLooksSecret(val string) bool {
	val = strings.Trim(val, `"'`)
	if len(val) < 12 {
		return false
	}
	// Reject obvious placeholders
	lower := strings.ToLower(val)
	if strings.Contains(lower, "changeme") || strings.Contains(lower, "example") || strings.Contains(lower, "your_") {
		return false
	}
	return true
}

func verifyAWSAccessKeyID(s string) bool {
	if len(s) != 20 {
		return false
	}
	prefixes := []string{
		"A3T", "AKIA", "AGPA", "AIDA", "AROA", "AIPA", "ANPA", "ANVA", "ASIA",
	}
	ok := false
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func verifyAWSSecretValue(s string) bool {
	for i := 0; i+40 <= len(s); i++ {
		chunk := s[i : i+40]
		if isBase64SecretChunk(chunk) {
			return true
		}
	}
	return false
}

func isBase64SecretChunk(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=' {
			continue
		}
		return false
	}
	return true
}

func verifyGitHubToken(s string) bool {
	prefixes := []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			tail := s[len(p):]
			if len(tail) < 36 {
				return false
			}
			for _, r := range tail {
				if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
					continue
				}
				return false
			}
			return true
		}
	}
	return false
}

func verifyOpenAIKey(s string) bool {
	if strings.HasPrefix(s, "sk-ant-") {
		return false
	}
	if !strings.HasPrefix(s, "sk-") {
		return false
	}
	rest := s[3:]
	if len(rest) < 20 {
		return false
	}
	for _, r := range rest {
		if r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func verifyAnthropicKey(s string) bool {
	if !strings.HasPrefix(s, "sk-ant-") {
		return false
	}
	if len(s) < 40 {
		return false
	}
	for _, r := range s {
		if r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func verifyStripeKey(s string) bool {
	var rest string
	switch {
	case strings.HasPrefix(s, "sk_live_"):
		rest = s[8:]
	case strings.HasPrefix(s, "sk_test_"):
		rest = s[8:]
	case strings.HasPrefix(s, "pk_live_"):
		rest = s[8:]
	case strings.HasPrefix(s, "pk_test_"):
		rest = s[8:]
	default:
		return false
	}
	if len(rest) < 24 {
		return false
	}
	for _, r := range rest {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func verifyJWTFormat(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if len(p) < 4 {
			return false
		}
		if _, err := jwtDecodeSegment(p); err != nil {
			return false
		}
	}
	return true
}

// jwtDecodeSegment decodes a JWT segment (base64url, with StdEncoding fallback).
func jwtDecodeSegment(p string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(padJWTBase64(p))
	if err == nil {
		return b, nil
	}
	p2 := strings.ReplaceAll(p, "-", "+")
	p2 = strings.ReplaceAll(p2, "_", "/")
	return base64.StdEncoding.DecodeString(padJWTBase64(p2))
}

func padJWTBase64(s string) string {
	switch len(s) % 4 {
	case 2:
		return s + "=="
	case 3:
		return s + "="
	default:
		return s
	}
}

// verifyBasicAuth decodes the credential and requires the "user:password"
// shape, so the header name quoted in documentation is not a finding.
func verifyBasicAuth(s string) bool {
	i := strings.LastIndexAny(s, " \t")
	if i < 0 {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(s[i+1:])
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(s[i+1:], "="))
		if err != nil {
			return false
		}
	}
	colon := strings.IndexByte(string(raw), ':')
	if colon <= 0 || colon == len(raw)-1 {
		return false
	}
	for _, c := range raw {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// verifyPasswordValue decides whether the word after a password label is a
// password or just the next word of a sentence. A password has a digit or a
// symbol in it; when no "is", ":" or "=" ties it to the label it must also be
// long and mix letters with digits. "password policy" and "password is at
// least 12" are prose; "password: hunter2" is not.
func verifyPasswordValue(s string) bool {
	i := strings.LastIndexAny(s, " \t\n")
	if i < 0 {
		return false
	}
	label := strings.TrimSpace(s[:i])
	value := strings.TrimRight(s[i+1:], `.,;"')`)
	tied := strings.HasSuffix(label, ":") || strings.HasSuffix(label, "=") || strings.HasSuffix(strings.ToLower(label), " is")

	var letters, digits, symbols int
	for _, r := range value {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		default:
			symbols++
		}
	}
	if digits+symbols == 0 {
		return false
	}
	// "password: <placeholder>", "password=${VAR}" and friends name a slot.
	if strings.ContainsAny(value, "<>{}$*") || strings.Contains(value, "...") {
		return false
	}
	if tied {
		return len(value) >= 5
	}
	return len(value) >= 8 && letters > 0 && digits > 0
}

func verifyPrivateKeyPEM(s string) bool {
	if !strings.Contains(s, "BEGIN") || !strings.Contains(s, "PRIVATE KEY") || !strings.Contains(s, "END") {
		return false
	}
	if len(s) < 80 {
		return false
	}
	return true
}

func verifyGenericAPIValue(s string) bool {
	// Full match is like "api_key=..." or "API_KEY: ..."
	for _, sep := range []string{"=", ":"} {
		i := strings.Index(s, sep)
		if i >= 0 && i+1 < len(s) {
			val := strings.TrimSpace(s[i+1:])
			val = strings.Trim(val, `"'`)
			return len(val) >= 20
		}
	}
	return len(s) >= 20
}

func verifySlackWebhookURL(s string) bool {
	if !strings.HasPrefix(s, "https://hooks.slack.com/services/") {
		return false
	}
	tail := strings.TrimPrefix(s, "https://hooks.slack.com/services/")
	parts := strings.Split(tail, "/")
	return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
}
