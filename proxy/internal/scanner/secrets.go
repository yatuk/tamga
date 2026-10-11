package scanner

import (
	"bytes"
	"context"
	"encoding/base64"
	"hash/maphash"
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
	// hints are literals of which every match contains at least one. A
	// text with none of them is not searched with the pattern at all, which
	// costs a small fraction of the search. Empty when the pattern has no
	// such literals (it matches without regard to case, or starts with a
	// character class).
	hints []string
}

// worth reports whether content can hold a match of the pattern.
func (p *secretPattern) worth(content []byte) bool {
	if len(p.hints) == 0 {
		return true
	}
	for _, h := range p.hints {
		if bytes.Contains(content, []byte(h)) {
			return true
		}
	}
	return false
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
			hints:      []string{"A3T", "AKIA", "AGPA", "AIDA", "AROA", "AIPA", "ANPA", "ANVA", "ASIA"},
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
			hints:      []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"},
		},
		{
			Name:       "anthropic_key",
			Regex:      regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9_-]{20,}\b`),
			Severity:   "critical",
			confidence: 0.96,
			validate:   verifyAnthropicKey,
			hints:      []string{"sk-ant-"},
		},
		{
			Name:       "openai_key",
			Regex:      regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{20,}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyOpenAIKey,
			hints:      []string{"sk-"},
		},
		{
			Name:       "stripe_key",
			Regex:      regexp.MustCompile(`\b(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{24,}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyStripeKey,
			hints:      []string{"_live_", "_test_"},
		},
		{
			Name:       "jwt_token",
			Regex:      regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
			Severity:   "high",
			confidence: 0.88,
			validate:   verifyJWTFormat,
			hints:      []string{"eyJ"},
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
			hints:      []string{"github_pat_"},
		},
		{
			// Slack bot, user, app and refresh tokens.
			Name:       "slack_token",
			Regex:      regexp.MustCompile(`\bxox[abeoprs]-[A-Za-z0-9]{6,}(?:-[A-Za-z0-9]{6,}){1,4}\b`),
			Severity:   "critical",
			confidence: 0.93,
			hints:      []string{"xox"},
		},
		{
			Name:       "google_api_key",
			Regex:      regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`),
			Severity:   "critical",
			confidence: 0.93,
			hints:      []string{"AIza"},
		},
		{
			// Twilio account (AC) and API key (SK) identifiers: 32 hex digits.
			Name:       "twilio_sid",
			Regex:      regexp.MustCompile(`\b(?:AC|SK)[0-9a-f]{32}\b`),
			Severity:   "high",
			confidence: 0.88,
			hints:      []string{"AC", "SK"},
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

		// ── Formats added on 2026-10-11 ────────────────────────────────
		// The list of vendors comes from comparing this file with the
		// plugins of Yelp's detect-secrets. Each pattern below was written
		// for Tamga from the format of the credential; the note on each
		// says where that is from, and patterns/README.md ("Secret
		// formats") has the table, with what was read and what was not.
		{
			// Stripe restricted keys (rk_); the secret and publishable
			// prefixes are matched above.
			Name:       "stripe_key",
			Regex:      regexp.MustCompile(`\brk_(?:live|test)_[A-Za-z0-9]{24,}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyStripeKey,
			hints:      []string{"rk_"},
		},
		{
			// The account key in an Azure Storage connection string: 64
			// bytes, written as 88 characters of base64. Microsoft Learn,
			// "Configure Azure Storage connection strings".
			Name:       "azure_storage_key",
			Regex:      regexp.MustCompile(`\bAccountKey\s*=\s*[A-Za-z0-9+/]{86}==`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyAzureStorageKey,
			hints:      []string{"AccountKey"},
		},
		{
			// GitLab tokens carry a prefix that says what they are:
			// personal, project and group access tokens (glpat-), OAuth
			// application secrets (gloas-), deploy tokens (gldt-), runner
			// authentication (glrt-, glrtr-), CI job (glcbt-), pipeline
			// trigger (glptt-), feed (glft-), incoming mail (glimt-), the
			// agent for Kubernetes (glagent-), SCIM (glsoat-), workspace
			// (glwt-) and feature-flag client tokens (glffct-). GitLab
			// docs, "GitLab token overview", table of token prefixes. An
			// instance can be set up to give its personal access tokens
			// another prefix; those are not matched.
			Name:       "gitlab_token",
			Regex:      regexp.MustCompile(`\bgl(?:pat|oas|dt|rtr|rt|cbt|ptt|ft|imt|agent|soat|wt|ffct)-[A-Za-z0-9_-]{20,}`),
			Severity:   "critical",
			confidence: 0.94,
			validate:   verifyGitLabToken,
			hints:      []string{"gl"},
		},
		{
			// npm access tokens: "npm_" and 36 letters and digits, the last
			// six a checksum. GitHub changelog of 2021-09-23, "npm has a new
			// access token format", which gives the prefix and the
			// checksum; the length is that of issued tokens.
			Name:       "npm_token",
			Regex:      regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifyNpmToken,
			hints:      []string{"npm_"},
		},
		{
			// The older npm token is a UUID, which only its place in an
			// .npmrc line tells from any other UUID.
			Name:       "npm_token",
			Regex:      regexp.MustCompile(`:_authToken\s*=\s*[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),
			Severity:   "critical",
			confidence: 0.93,
			hints:      []string{":_authToken"},
		},
		{
			// SendGrid API keys: "SG.", a 22-character key id, a dot and a
			// 43-character secret, 69 characters in all. The SendGrid
			// documentation ("API Keys") gives the length and not the
			// layout, which is that of issued keys.
			Name:       "sendgrid_key",
			Regex:      regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),
			Severity:   "critical",
			confidence: 0.95,
			validate:   verifySendGridKey,
			hints:      []string{"SG."},
		},
		{
			// Mailchimp API keys: 32 hex digits, a dash and the data
			// centre the account lives in ("us21"). The shape is that of
			// issued keys; the Mailchimp pages read for this change do
			// not spell it out.
			Name:       "mailchimp_key",
			Regex:      regexp.MustCompile(`\b[0-9a-f]{32}-us[0-9]{1,2}\b`),
			Severity:   "critical",
			confidence: 0.92,
			validate:   verifyMailchimpKey,
			hints:      []string{"-us"},
		},
		{
			// Square: access tokens (sq0atp-, 22 characters) and OAuth
			// application secrets (sq0csp-, 43 characters). The prefixes
			// are those of issued credentials; Square's page on its
			// credentials names the kinds and not their shape.
			Name:       "square_token",
			Regex:      regexp.MustCompile(`\bsq0(?:atp|csp)-[A-Za-z0-9_-]{22,43}`),
			Severity:   "critical",
			confidence: 0.94,
			validate:   verifySquareToken,
			hints:      []string{"sq0"},
		},
		{
			// Discord bot tokens are three base64url parts; the first is
			// the bot's user id. The structure is that of issued tokens;
			// no Discord page was read for it.
			Name:       "discord_bot_token",
			Regex:      regexp.MustCompile(`\b[A-Za-z0-9_-]{24,28}\.[A-Za-z0-9_-]{6,7}\.[A-Za-z0-9_-]{27,38}`),
			Severity:   "critical",
			confidence: 0.93,
			validate:   verifyDiscordBotToken,
		},
		{
			// Telegram bot tokens: the bot's numeric id, a colon and 35
			// characters. Telegram Bot API docs, "Authorizing your bot",
			// which show an example and give no formal layout.
			Name:       "telegram_bot_token",
			Regex:      regexp.MustCompile(`\b[0-9]{8,10}:[A-Za-z0-9_-]{35}`),
			Severity:   "critical",
			confidence: 0.92,
			validate:   verifyTelegramBotToken,
		},
		{
			// PyPI API tokens: "pypi-" and a base64url macaroon whose
			// first field names the index it is for. PyPI help, "How can
			// I use API tokens to authenticate with PyPI?", for the
			// prefix; that the body starts with the index name is read
			// off the encoding, not documented there.
			Name:       "pypi_token",
			Regex:      regexp.MustCompile(`\bpypi-AgE[A-Za-z0-9_-]{70,}`),
			Severity:   "critical",
			confidence: 0.96,
			validate:   verifyPyPIToken,
			hints:      []string{"pypi-"},
		},
		{
			// JFrog Artifactory API keys start with "AKCp". The shape is
			// that of issued keys; no JFrog page was read for it.
			Name:       "artifactory_key",
			Regex:      regexp.MustCompile(`\bAKCp[A-Za-z0-9]{60,76}\b`),
			Severity:   "critical",
			confidence: 0.90,
			validate:   verifyArtifactoryKey,
			hints:      []string{"AKCp"},
		},
		{
			// A user name and password inside a URL, as RFC 3986 allows
			// ("userinfo"): what a Cloudant, registry or broker URL with
			// credentials looks like. The database schemes are matched as
			// connection strings below.
			Name:       "url_credentials",
			Regex:      regexp.MustCompile(`\b(?:https?|ftp|sftp|amqps?|smtps?|ldaps?)://[^\s:/@"'<>]{1,64}:[^\s@/"'<>]{3,128}@[A-Za-z0-9.-]+`),
			Severity:   "high",
			confidence: 0.85,
			validate:   verifyURLCredentials,
			hints:      []string{"://"},
		},
		{
			// A PuTTY private key file (.ppk) begins with this line.
			Name:       "private_key",
			Regex:      regexp.MustCompile(`\bPuTTY-User-Key-File-[23]: [a-z0-9@.-]+`),
			Severity:   "high",
			confidence: 0.85,
			hints:      []string{"PuTTY-User-Key-File-"},
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

// Scan stops between two patterns when the context ends, and then returns
// what it found together with an ErrScanDegraded error.
func (s *SecretScanner) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	secretOnce.Do(compileSecretPatterns)
	st := &scanStop{ctx: ctx, scanner: s.Name()}
	if st.stopped() {
		return nil, st.err
	}
	var findings []Finding

	// Normalize to defeat Unicode confusables and zero-width splits.
	normResult := normalize.Apply(string(content), normalize.Default())
	normText := normResult.Text()
	normBytes := []byte(normText)

	for _, p := range secretPatterns {
		if st.stopped() {
			return findings, st.err
		}
		if !p.worth(content) {
			continue
		}
		matches := p.Regex.FindAllIndex(content, -1)
		for _, loc := range matches {
			matched := string(content[loc[0]:loc[1]])
			if p.validate != nil && !p.validate(matched) {
				continue
			}
			findings = append(findings, makeSecretFinding(p.Name, p.Severity, matched, loc[0], loc[1], p.confidence))
		}
	}

	if st.stopped() {
		return findings, st.err
	}
	findings = append(findings, scanConnectionStrings(content)...)
	findings = append(findings, scanDotenvLines(content)...)

	// Re-scan normalized text for Unicode-evaded secrets.
	for _, p := range secretPatterns {
		if st.stopped() {
			return findings, st.err
		}
		if !p.worth(normBytes) {
			continue
		}
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
	if st.stopped() {
		return findings, st.err
	}
	// Their positions are offsets into the normalised text, like those of the
	// patterns above, and are marked the same way: a mask put at them would
	// land somewhere else in the text as written.
	for _, f := range append(scanConnectionStrings(normBytes), scanDotenvLines(normBytes)...) {
		f.Metadata = map[string]string{"detected_via": "unicode_normalization"}
		findings = append(findings, f)
	}

	return findings, nil
}

func makeSecretFinding(name, severity, raw string, start, end int, conf float64) Finding {
	return Finding{
		Type:       "secret",
		Category:   name,
		Severity:   severity,
		Match:      maskSecret(name, raw),
		StartPos:   start,
		EndPos:     end,
		Confidence: conf,
		fp:         secretFingerprint(raw),
	}
}

// secretPrefixes are the markers a vendor puts in front of its keys, by
// category. Each is a literal of the category's pattern: it is public, the
// same for every key of its kind, and says nothing about the key that
// follows. Longer markers come first where one starts with another.
var secretPrefixes = map[string][]string{
	"aws_access_key": {"AKIA", "AGPA", "AIDA", "AROA", "AIPA", "ANPA", "ANVA", "ASIA", "A3T"},
	"github_token":   {"github_pat_", "ghp_", "gho_", "ghu_", "ghs_", "ghr_"},
	"anthropic_key":  {"sk-ant-"},
	"openai_key":     {"sk-"},
	"stripe_key":     {"sk_live_", "sk_test_", "pk_live_", "pk_test_", "rk_live_", "rk_test_"},
	"slack_token":    {"xoxa-", "xoxb-", "xoxe-", "xoxo-", "xoxp-", "xoxr-", "xoxs-"},
	"google_api_key": {"AIza"},
	"twilio_sid":     {"AC", "SK"},
	"gitlab_token": {
		"glagent-", "glsoat-", "glffct-", "glpat-", "gloas-", "glrtr-", "glcbt-", "glptt-",
		"glimt-", "gldt-", "glrt-", "glft-", "glwt-",
	},
	// The older npm token has no marker: its match starts with the
	// .npmrc key, which is not listed here, and keeps nothing.
	"npm_token":       {"npm_"},
	"sendgrid_key":    {"SG."},
	"square_token":    {"sq0atp-", "sq0csp-"},
	"pypi_token":      {"pypi-"},
	"artifactory_key": {"AKCp"},
}

// secretMasked stands for the whole of a secret's value.
const secretMasked = "****"

// maskSecret is what a secret finding keeps of its match: the vendor's
// marker when the category has one, and no character of the value. The
// match is stored with the event and shown in the console, and neither
// needs any part of a key to say what kind of key it was.
func maskSecret(category, raw string) string {
	for _, p := range secretPrefixes[category] {
		if strings.HasPrefix(raw, p) {
			return p + secretMasked
		}
	}
	return secretMasked
}

// fingerprintSeed is drawn once per process, so a fingerprint means nothing
// outside it.
var fingerprintSeed = maphash.MakeSeed()

// secretFingerprint tells two secrets of one category apart now that their
// matches no longer do. It stays in the finding's unexported field: it is
// compared while a scan's findings are settled and is never written out.
// Case is ignored, because the normalised views differ in case and one
// value found in two of them is one value.
func secretFingerprint(raw string) uint64 {
	fp := maphash.String(fingerprintSeed, strings.ToLower(raw))
	if fp == 0 {
		fp = 1 // zero means "no fingerprint"
	}
	return fp
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
	case strings.HasPrefix(s, "rk_live_"), strings.HasPrefix(s, "rk_test_"):
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

// variedEnough reports whether s has at least n different characters. A
// placeholder written in the shape of a key ("glpat-xxxxxxxxxxxxxxxxxxxx")
// does not.
func variedEnough(s string, n int) bool {
	var seen [256]bool
	distinct := 0
	for i := 0; i < len(s); i++ {
		if !seen[s[i]] {
			seen[s[i]] = true
			distinct++
			if distinct >= n {
				return true
			}
		}
	}
	return false
}

// verifyAzureStorageKey requires the value to be 64 bytes of base64.
func verifyAzureStorageKey(s string) bool {
	i := strings.IndexByte(s, '=')
	if i < 0 {
		return false
	}
	value := strings.TrimSpace(s[i+1:])
	raw, err := base64.StdEncoding.DecodeString(value)
	return err == nil && len(raw) == 64 && variedEnough(value, 12)
}

func verifyGitLabToken(s string) bool {
	body := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		body = s[i+1:]
	}
	return len(body) >= 20 && variedEnough(body, 8)
}

func verifyNpmToken(s string) bool {
	body := strings.TrimPrefix(s, "npm_")
	return len(body) == 36 && variedEnough(body, 8)
}

func verifySendGridKey(s string) bool {
	parts := strings.Split(s, ".")
	return len(parts) == 3 && parts[0] == "SG" && len(parts[1]) == 22 && len(parts[2]) == 43 &&
		variedEnough(parts[2], 8)
}

// verifyMailchimpKey rejects a hash that happens to stand before "-us1":
// the hex part of a key is random, so it is varied.
func verifyMailchimpKey(s string) bool {
	i := strings.LastIndex(s, "-us")
	return i == 32 && variedEnough(s[:i], 8)
}

// verifySquareToken checks the length that goes with the prefix.
func verifySquareToken(s string) bool {
	switch {
	case strings.HasPrefix(s, "sq0atp-"):
		return len(s) == len("sq0atp-")+22 && variedEnough(s[7:], 8)
	case strings.HasPrefix(s, "sq0csp-"):
		return len(s) == len("sq0csp-")+43 && variedEnough(s[7:], 8)
	}
	return false
}

// verifyDiscordBotToken decodes the first part, which must be the bot's
// user id: a number of 17 to 20 digits. Three dotted words that are not a
// token do not decode to that.
func verifyDiscordBotToken(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	id, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		id, err = base64.RawStdEncoding.DecodeString(parts[0])
		if err != nil {
			return false
		}
	}
	if len(id) < 17 || len(id) > 20 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return variedEnough(parts[2], 8)
}

func verifyTelegramBotToken(s string) bool {
	i := strings.IndexByte(s, ':')
	if i < 8 || s[0] == '0' {
		return false
	}
	secret := s[i+1:]
	var upper, lower bool
	for _, c := range secret {
		switch {
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= 'a' && c <= 'z':
			lower = true
		}
	}
	return len(secret) == 35 && upper && lower && variedEnough(secret, 8)
}

// verifyPyPIToken decodes the start of the macaroon and looks for the name
// of the index in it, which every token PyPI issues carries.
func verifyPyPIToken(s string) bool {
	body := strings.TrimPrefix(s, "pypi-")
	if len(body) < 70 {
		return false
	}
	head, err := base64.RawURLEncoding.DecodeString(body[:40])
	if err != nil {
		return false
	}
	return strings.Contains(string(head), "pypi.org")
}

func verifyArtifactoryKey(s string) bool {
	return strings.HasPrefix(s, "AKCp") && variedEnough(s[4:], 12)
}

// urlPasswordPlaceholders are what documentation writes where a password
// goes.
var urlPasswordPlaceholders = map[string]bool{
	"password": true, "passwd": true, "pass": true, "pwd": true, "secret": true,
	"token": true, "changeme": true, "example": true, "user": true, "username": true,
	"xxx": true, "xxxx": true, "xxxxx": true, "redacted": true, "parola": true, "sifre": true,
}

// verifyURLCredentials decides whether the part between ":" and "@" is a
// password or a placeholder for one.
func verifyURLCredentials(s string) bool {
	rest := s[strings.Index(s, "://")+3:]
	at := strings.LastIndexByte(rest, '@')
	colon := strings.IndexByte(rest, ':')
	if colon < 1 || at <= colon+1 {
		return false
	}
	pass := rest[colon+1 : at]
	if urlPasswordPlaceholders[strings.ToLower(pass)] {
		return false
	}
	if strings.ContainsAny(pass, "{}$*[]") {
		return false // ${VAR}, {{password}}, ****, [password]
	}
	if strings.Contains(pass, "...") {
		return false
	}
	return variedEnough(pass, 3)
}
