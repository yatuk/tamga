package scanner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// The samples are put together here so that no file holds a string a
// secret scanner would take for a key.
func fill(alphabet string, n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		b.WriteByte(alphabet[i%len(alphabet)])
	}
	return b.String()
}

const (
	upperDigits = "QW7ZX9PLM2"
	mixed       = "qW7zX9pLm2Rt"
	hexDigits   = "9f3e7d1c5b"
)

// A sample for every marker in secretPrefixes: the marker followed by a
// value the category's pattern accepts.
func prefixSamples() map[string]map[string]string {
	body := func(category, prefix string) string {
		switch category {
		case "aws_access_key":
			return fill(upperDigits, 20-len(prefix))
		case "github_token":
			return fill(mixed, 40)
		case "anthropic_key":
			return fill(mixed, 48)
		case "openai_key":
			return fill(mixed, 40)
		case "stripe_key":
			return fill(mixed, 28)
		case "slack_token":
			return fill("7391", 11) + "-" + fill(mixed, 24)
		case "google_api_key":
			return fill(mixed, 35)
		case "twilio_sid":
			return fill(hexDigits, 32)
		case "gitlab_token":
			return fill(mixed, 24)
		case "npm_token":
			return fill(mixed, 36)
		case "sendgrid_key":
			return fill(mixed, 22) + "." + fill(mixed, 43)
		case "square_token":
			if prefix == "sq0csp-" {
				return fill(mixed, 43)
			}
			return fill(mixed, 22)
		case "pypi_token":
			return strings.TrimPrefix(fakePyPIToken(), prefix)
		case "artifactory_key":
			return fill(mixed, 69)
		}
		return ""
	}
	out := map[string]map[string]string{}
	for category, prefixes := range secretPrefixes {
		out[category] = map[string]string{}
		for _, p := range prefixes {
			out[category][p] = p + body(category, p)
		}
	}
	return out
}

func secretFindings(t *testing.T, text string) []Finding {
	t.Helper()
	found, err := NewSecretScanner().Scan(context.Background(), []byte(text))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return found
}

// Every marker is one the category's pattern really matches, and the match
// of the finding is that marker and nothing after it.
func TestSecretMatch_KeepsOnlyTheVendorMarker(t *testing.T) {
	for category, samples := range prefixSamples() {
		for prefix, sample := range samples {
			body := strings.TrimPrefix(sample, prefix)
			seen := false
			for _, f := range secretFindings(t, "key "+sample+" end") {
				if f.Category != category {
					continue
				}
				seen = true
				if want := prefix + "****"; f.Match != want {
					t.Errorf("%s %q: match = %q, want %q", category, prefix, f.Match, want)
				}
				if strings.Contains(f.Match, body[:4]) {
					t.Errorf("%s %q: match %q holds part of the value", category, prefix, f.Match)
				}
			}
			if !seen {
				t.Errorf("%s: the pattern does not match a value that starts with %q", category, prefix)
			}
		}
	}
}

// The kinds without a vendor marker keep nothing. Several of them kept
// eight characters of the value before: a connection string's password, a
// bare 40-character key, a short label followed by the secret.
func TestSecretMatch_NoMarkerNoCharacters(t *testing.T) {
	value := fill(mixed, 40)
	texts := map[string]string{
		"aws_secret_key":             "aws_secret_access_key=" + value,
		"aws_secret_key_raw":         "dump: " + value,
		"generic_api_key":            "apikey=" + value,
		"password":                   "pwd: " + value[:20] + "!",
		"connection_string":          "postgres://app:" + value[:16] + "@db.example.invalid:5432/app",
		"connection_string_password": "postgres://app:" + value[:16] + "@db.example.invalid:5432/app",
		"dotenv_secret":              "DB_TOKEN=" + value,
		"jwt_token":                  "eyJ" + fill(mixed, 21) + "." + fill(mixed, 24) + "." + fill(mixed, 24),
		"slack_webhook":              "https://hooks.slack.com/services/T" + fill(upperDigits, 8) + "/B" + fill(upperDigits, 8) + "/" + value[:24],
		"basic_auth":                 "Authorization: Basic dXNlcjpwYXNzd29yZDEyMw==",
		"private_key":                "-----BEGIN RSA PRIVATE KEY-----\n" + fill(mixed, 120) + "\n-----END RSA PRIVATE KEY-----",
	}
	for category, text := range texts {
		seen := false
		for _, f := range secretFindings(t, text) {
			if f.Category != category {
				continue
			}
			seen = true
			if f.Match != "****" {
				t.Errorf("%s: match = %q, want ****", category, f.Match)
			}
		}
		if !seen {
			t.Errorf("%s: no finding for the sample", category)
		}
	}
}

// Whatever the scanner is given, a secret's match is a marker from the
// table followed by the mask, or the mask alone.
func TestSecretMatch_IsAlwaysOneOfTheKnownForms(t *testing.T) {
	allowed := map[string]bool{"****": true}
	for _, prefixes := range secretPrefixes {
		for _, p := range prefixes {
			allowed[p+"****"] = true
		}
	}
	var text strings.Builder
	for _, samples := range prefixSamples() {
		for _, s := range samples {
			text.WriteString(s + "\n")
		}
	}
	text.WriteString("password: " + fill(mixed, 12) + "!\nAPI_SECRET=" + fill(mixed, 30) + "\n")
	text.WriteString("mysql://root:" + fill(mixed, 14) + "@db.example.invalid/x\n")

	reg := NewRegistry()
	reg.Register(NewSecretScanner())
	found, err := reg.ScanRaw(context.Background(), []byte(text.String()), PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) < 20 {
		t.Fatalf("only %d findings; the samples are not being matched", len(found))
	}
	for _, f := range found {
		if !allowed[f.Match] {
			t.Errorf("%s: match %q is not a known form", f.Category, f.Match)
		}
	}
}

// The fingerprint that tells secrets apart is not part of what a finding
// is written as.
func TestSecretFingerprint_IsNotEncoded(t *testing.T) {
	sample := "sk-" + fill(mixed, 40)
	found := secretFindings(t, sample)
	if len(found) == 0 || found[0].fp == 0 {
		t.Fatalf("no fingerprinted finding: %+v", found)
	}
	raw, err := json.Marshal(found)
	if err != nil {
		t.Fatal(err)
	}
	var fields []map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{
		"type": true, "severity": true, "match": true, "category": true, "start_pos": true,
		"end_pos": true, "confidence": true, "metadata": true,
	}
	for _, m := range fields {
		for k := range m {
			if !known[k] {
				t.Errorf("unexpected field %q in an encoded finding", k)
			}
		}
	}
	if strings.Contains(string(raw), sample[3:11]) {
		t.Errorf("encoded finding holds part of the value: %s", raw)
	}
	if back := ProtoToFindings(FindingsToProto(found)); back[0].fp != 0 {
		t.Error("the fingerprint crossed the wire")
	}
}

func countCategory(findings []Finding, category string) (placed, floating int) {
	for _, f := range findings {
		if f.Category != category {
			continue
		}
		if f.Placed() {
			placed++
		} else {
			floating++
		}
	}
	return
}

// hidden writes a key with a zero-width space inside it: the text as
// written does not match the pattern, the normalised view does.
func hidden(key string) string { return key[:8] + "​" + key[8:] }

// Two keys of one kind now have the same match. A second one, however it is
// written, must still be reported next to the one written plainly: with its
// own position when the reading it was found in knows it, and without one
// otherwise, which is what stops a REDACT from letting it through.
func TestSecretDedupe_SameKindDifferentValues(t *testing.T) {
	plain := "sk-" + fill(mixed, 40)
	other := "sk-" + fill("Zy8Kd3Nf5Hb", 40)
	reg := NewRegistry()
	reg.Register(NewSecretScanner())
	cfg := PipelineConfig{Mode: ModeSync}

	one, err := reg.ScanRaw(context.Background(), []byte("use "+plain+" here"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p, f := countCategory(one, "openai_key"); p != 1 || f != 0 {
		t.Fatalf("one key: placed=%d floating=%d, want 1 and 0", p, f)
	}

	both, err := reg.ScanRaw(context.Background(), []byte("use "+plain+" and "+hidden(other)+" here"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	// The key with a zero-width space in it is read where it stands: a
	// second finding with its own position.
	if p, f := countCategory(both, "openai_key"); p != 2 || f != 0 {
		t.Fatalf("plain and hidden key: placed=%d floating=%d, want 2 and 0", p, f)
	}
	// The same key written twice, once plainly and once hidden, is two
	// places to mask, not one value.
	twice, err := reg.ScanRaw(context.Background(), []byte("use "+plain+" and "+hidden(plain)+" here"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if p, f := countCategory(twice, "openai_key"); p != 2 || f != 0 {
		t.Fatalf("one key plain and hidden: placed=%d floating=%d, want 2 and 0", p, f)
	}
	// A key that only a decoded payload holds has no position and is not
	// an echo of the plain one, whichever key it is.
	for _, key := range []string{other, plain} {
		encoded, err := reg.ScanRaw(context.Background(), []byte("use "+plain+" and "+base64.StdEncoding.EncodeToString([]byte(key))+" here"), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if p, f := countCategory(encoded, "openai_key"); p != 1 || f == 0 {
			t.Fatalf("plain and encoded key: placed=%d floating=%d, want 1 and at least 1", p, f)
		}
		if UnplacedRedactions(encoded) == 0 {
			t.Fatal("the encoded key was dropped as an echo of the plain one")
		}
	}
}

// The remote scanner service settles its findings with ScanRaw and sends
// them without fingerprints. The proxy settles them again: one key stays one
// finding, and a further key that only a normalised view reveals is kept.
func TestSecretDedupe_FromTheScannerService(t *testing.T) {
	plain := "sk-" + fill(mixed, 40)
	other := "sk-" + fill("Zy8Kd3Nf5Hb", 40)
	reg := NewRegistry()
	reg.Register(NewSecretScanner())
	remote := func(text string) []Finding {
		found, err := reg.ScanRaw(context.Background(), []byte(text), PipelineConfig{Mode: ModeSync})
		if err != nil {
			t.Fatal(err)
		}
		return SettleRaw(ProtoToFindings(FindingsToProto(found)), []byte(text))
	}

	if p, f := countCategory(remote("use "+plain+" here"), "openai_key"); p != 1 || f != 0 {
		t.Fatalf("one key: placed=%d floating=%d, want 1 and 0", p, f)
	}
	two := remote("use " + plain + " and " + other + " here")
	if p, f := countCategory(two, "openai_key"); p != 2 || f != 0 {
		t.Fatalf("two plain keys: placed=%d floating=%d, want 2 and 0", p, f)
	}
	for _, key := range []string{other, plain} {
		both := remote("use " + plain + " and " + hidden(key) + " here")
		if p, f := countCategory(both, "openai_key"); p != 2 || f != 0 {
			t.Fatalf("plain and hidden key: placed=%d floating=%d, want 2 and 0", p, f)
		}
		both = remote("use " + plain + " and " + base64.StdEncoding.EncodeToString([]byte(key)) + " here")
		if p, f := countCategory(both, "openai_key"); p != 1 || f == 0 {
			t.Fatalf("plain and encoded key: placed=%d floating=%d, want 1 and at least 1", p, f)
		}
	}
}

// A scanner service from before this change sends the echoes too. Nothing
// tells them from further secrets, so none is dropped: a REDACT then blocks
// where it would have masked, which is the safe way to be wrong.
func TestSecretDedupe_UnsettledFindingsAreKept(t *testing.T) {
	text := "use sk-" + fill(mixed, 40) + " here"
	settled := SettleRaw(ProtoToFindings(FindingsToProto(secretFindings(t, text))), []byte(text))
	if p, f := countCategory(settled, "openai_key"); p != 1 || f == 0 {
		t.Fatalf("placed=%d floating=%d, want 1 and the echoes kept", p, f)
	}
}
