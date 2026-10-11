package scanner

import (
	"encoding/base64"
	"strings"
	"testing"
)

// The values below are put together from parts (fill, in
// secrets_mask_test.go), so that the file holds nothing a secret scanner on
// the repository host would take for a key. None of them was ever issued.

func fakePyPIToken() string {
	// The macaroon starts with its version, the length of the location and
	// the location itself.
	return "pypi-" + base64.RawURLEncoding.EncodeToString(append([]byte{2, 1, 8}, "pypi.org\x02\x24"+fill(mixed, 70)...))
}

func fakeDiscordToken(id string) string {
	return base64.RawStdEncoding.EncodeToString([]byte(id)) + "." + fill(mixed, 6) + "." + fill(mixed, 27)
}

func fakeAzureKey() string {
	return base64.StdEncoding.EncodeToString([]byte(fill(mixed, 64)))
}

// Every format added on 2026-10-11 has a value it must find and a
// look-alike it must leave alone.
func TestSecrets_VendorFormats(t *testing.T) {
	uuid := fill(hexDigits, 8) + "-" + fill(hexDigits, 4) + "-" + fill(hexDigits, 4) + "-" + fill(hexDigits, 4) + "-" + fill(hexDigits, 12)
	tests := []struct {
		name     string
		text     string
		category string
		want     bool
	}{
		{"stripe restricted key", "rk_" + "live_" + fill(mixed, 28), "stripe_key", true},
		{"stripe restricted prefix in prose", "a restricted key starts with rk_live_ and is scoped", "stripe_key", false},

		{"azure connection string", "DefaultEndpointsProtocol=https;AccountName=acct;AccountKey=" + fakeAzureKey() + ";EndpointSuffix=core.windows.net", "azure_storage_key", true},
		{"azure placeholder", "DefaultEndpointsProtocol=https;AccountName=storagesample;AccountKey=<account-key>", "azure_storage_key", false},
		{"azure key of one character", "AccountKey=" + strings.Repeat("A", 86) + "==", "azure_storage_key", false},

		{"gitlab personal access token", "token: " + "glpat-" + fill(mixed, 20), "gitlab_token", true},
		{"gitlab deploy token", "gldt-" + fill(mixed, 20), "gitlab_token", true},
		{"gitlab runner token", "glrtr-" + fill(mixed, 26), "gitlab_token", true},
		{"gitlab placeholder", "export GITLAB_TOKEN=glpat-" + strings.Repeat("x", 20), "gitlab_token", false},
		{"gitlab prefix in prose", "personal access tokens start with glpat- by default", "gitlab_token", false},

		{"npm token", "npm_" + fill(mixed, 36), "npm_token", true},
		{"npm token in .npmrc, old form", "//registry.npmjs.org/:_authToken=" + uuid, "npm_token", true},
		{"npm placeholder", "npm_" + strings.Repeat("x", 36), "npm_token", false},
		{"npm variable in .npmrc", "//registry.npmjs.org/:_authToken=${NPM_TOKEN}", "npm_token", false},
		{"npm script name", "run npm_config_registry_override_for_the_ci_job first", "npm_token", false},

		{"sendgrid key", "SG." + fill(mixed, 22) + "." + fill(mixed, 43), "sendgrid_key", true},
		{"sendgrid, parts too short", "SG." + fill(mixed, 10) + "." + fill(mixed, 20), "sendgrid_key", false},
		{"sendgrid placeholder", "SG." + strings.Repeat("x", 22) + "." + strings.Repeat("x", 43), "sendgrid_key", false},

		{"mailchimp key", fill(hexDigits, 32) + "-us21", "mailchimp_key", true},
		{"hash of zeros before a region", strings.Repeat("0", 32) + "-us1", "mailchimp_key", false},
		{"md5 followed by a word", fill(hexDigits, 32) + "-user", "mailchimp_key", false},

		{"square access token", "sq0atp-" + fill(mixed, 22), "square_token", true},
		{"square application secret", "sq0csp-" + fill(mixed, 43), "square_token", true},
		{"square, wrong length for the prefix", "sq0atp-" + fill(mixed, 30), "square_token", false},

		{"discord bot token", fakeDiscordToken("123456789012345678"), "discord_bot_token", true},
		{"three dotted words", fill("abcdefgh", 24) + "." + fill(mixed, 6) + "." + fill(mixed, 27), "discord_bot_token", false},
		{"java package name", "com.example.application.internal.module.Handler", "discord_bot_token", false},

		{"telegram bot token", "123456789:" + fill("AbC-dEf_1", 35), "telegram_bot_token", true},
		{"digits, colon and lower-case text", "1234567890:" + fill("abcdefghij", 35), "telegram_bot_token", false},
		{"a time and a word", "meet at 12:30 near the " + fill(mixed, 35), "telegram_bot_token", false},

		{"pypi token", fakePyPIToken(), "pypi_token", true},
		{"pypi prefix with another body", "pypi-AgE" + fill(mixed, 80), "pypi_token", false},

		{"artifactory key", "AKCp" + fill(mixed, 69), "artifactory_key", true},
		{"artifactory placeholder", "AKCp" + strings.Repeat("a", 69), "artifactory_key", false},

		{"credentials in a URL", "docker login https://deploy:" + fill(mixed, 12) + "@registry.example.invalid/v2/", "url_credentials", true},
		{"cloudant URL with credentials", "https://acct:" + fill(hexDigits, 24) + "@acct.cloudant.example.invalid", "url_credentials", true},
		{"URL with a placeholder", "https://user:password@example.com/path", "url_credentials", false},
		{"URL with a variable", "https://ci:${REGISTRY_PASSWORD}@registry.example.invalid", "url_credentials", false},
		{"URL with a port", "https://example.com:8443/login and mail to admin@example.com", "url_credentials", false},

		{"putty key file", "PuTTY-User-Key-File-3: ssh-ed25519\nEncryption: none", "private_key", true},
		{"putty in prose", "how do I convert a PuTTY key file to OpenSSH?", "private_key", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := false
			for _, f := range secretFindings(t, tt.text) {
				if f.Category != tt.category {
					continue
				}
				got = true
				if !strings.HasSuffix(f.Match, secretMasked) {
					t.Errorf("match %q does not end in the mask", f.Match)
				}
				if marker := strings.TrimSuffix(f.Match, secretMasked); marker != "" {
					known := false
					for _, p := range secretPrefixes[tt.category] {
						known = known || p == marker
					}
					if !known {
						t.Errorf("match %q keeps %q, which is not a marker of %s", f.Match, marker, tt.category)
					}
				}
			}
			if got != tt.want {
				t.Errorf("%s finding = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

// The new categories without a vendor marker keep no character of the value.
func TestSecretMatch_NewFormatsWithoutMarker(t *testing.T) {
	texts := map[string]string{
		"azure_storage_key":  "AccountKey=" + fakeAzureKey(),
		"mailchimp_key":      fill(hexDigits, 32) + "-us7",
		"discord_bot_token":  fakeDiscordToken("987654321098765432"),
		"telegram_bot_token": "987654321:" + fill("AbC-dEf_1", 35),
		"url_credentials":    "https://deploy:" + fill(mixed, 12) + "@registry.example.invalid",
	}
	for category, text := range texts {
		seen := false
		for _, f := range secretFindings(t, text) {
			if f.Category != category {
				continue
			}
			seen = true
			if f.Match != secretMasked {
				t.Errorf("%s: match = %q, want %s", category, f.Match, secretMasked)
			}
		}
		if !seen {
			t.Errorf("%s: no finding for the sample", category)
		}
	}
}

// A hint is a promise that no match of the pattern lacks it; a wrong one
// makes the scanner skip real keys. Every pattern is run here without its
// hints over the samples of all the tests, and each match must contain one.
func TestSecretPatternHints(t *testing.T) {
	NewSecretScanner()
	var text strings.Builder
	for _, samples := range prefixSamples() {
		for _, s := range samples {
			text.WriteString(s + "\n")
		}
	}
	for _, s := range []string{
		"AccountKey=" + fakeAzureKey(), "//registry.npmjs.org/:_authToken=" + fill(hexDigits, 8) + "-" + fill(hexDigits, 4) + "-" + fill(hexDigits, 4) + "-" + fill(hexDigits, 4) + "-" + fill(hexDigits, 12),
		fill(hexDigits, 32) + "-us21", fakeDiscordToken("123456789012345678"), "123456789:" + fill("AbC-dEf_1", 35),
		"https://deploy:" + fill(mixed, 12) + "@registry.example.invalid", "PuTTY-User-Key-File-2: ssh-rsa",
		"rk_" + "test_" + fill(mixed, 28), "eyJ" + fill(mixed, 21) + "." + fill(mixed, 24) + "." + fill(mixed, 24),
		"github_pat_" + fill(mixed, 40), "A3T" + fill(upperDigits, 17),
	} {
		text.WriteString(s + "\n")
	}
	content := []byte(text.String())
	hinted, matched := 0, 0
	for _, p := range secretPatterns {
		if len(p.hints) == 0 {
			continue
		}
		hinted++
		locs := p.Regex.FindAllIndex(content, -1)
		if len(locs) > 0 {
			matched++
		}
		for _, loc := range locs {
			m := string(content[loc[0]:loc[1]])
			ok := false
			for _, h := range p.hints {
				ok = ok || strings.Contains(m, h)
			}
			if !ok {
				t.Errorf("%s: a match holds none of the hints %q", p.Name, p.hints)
			}
		}
	}
	if matched < hinted {
		t.Errorf("%d of %d hinted patterns matched a sample: add a sample for the others", matched, hinted)
	}
}
