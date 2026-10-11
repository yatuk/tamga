package scanner

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestMaskableType(t *testing.T) {
	for typ, want := range map[string]bool{
		"pii": true, "secret": true, "custom": true, "competitor": true,
		"injection": false, "content_moderation": false, "operator_state": false,
		"operator_state_semantic": false, "code_leak": false, "system_prompt_leak": false,
		"": false, "something_new": false,
	} {
		if got := MaskableType(typ); got != want {
			t.Errorf("MaskableType(%q) = %v, want %v", typ, got, want)
		}
	}
	if Maskable(Finding{Type: "secret"}) {
		t.Error("a secret without a position was called maskable")
	}
	if !Maskable(Finding{Type: "secret", StartPos: 2, EndPos: 9}) {
		t.Error("a positioned secret was not called maskable")
	}
	if Maskable(Finding{Type: "injection", StartPos: 2, EndPos: 9}) {
		t.Error("an injection with a position was called maskable")
	}
}

// The marker names the kind and holds nothing of what was matched.
func TestMaskMarker(t *testing.T) {
	for _, tt := range []struct {
		f    Finding
		want string
	}{
		{Finding{Type: "pii", Category: "email", Match: "ay**@example.com"}, "[email_REDACTED]"},
		{Finding{Type: "secret", Category: "aws_access_key", Match: "AKIAIOSF..."}, "[aws_access_key_REDACTED]"},
		{Finding{Type: "custom", Category: "musteri_no", Match: "MN***34"}, "[musteri_no_REDACTED]"},
		// A competitor's category is its name: the thing being taken out.
		{Finding{Type: "competitor", Category: "Acme Guard", Match: "Ac******rd"}, "[competitor_REDACTED]"},
		{Finding{Type: "secret"}, "[secret_REDACTED]"},
	} {
		if got := MaskMarker(tt.f); got != tt.want {
			t.Errorf("MaskMarker(%s/%s) = %q, want %q", tt.f.Type, tt.f.Category, got, tt.want)
		}
	}
}

func TestRedactContent(t *testing.T) {
	text := "mail ayse@example.com key AKIAIOSFODNN7EXAMPLE end"
	mail := Finding{Type: "pii", Category: "email", StartPos: 5, EndPos: 21}
	key := Finding{Type: "secret", Category: "aws_access_key", StartPos: 26, EndPos: 46}
	tests := []struct {
		name     string
		findings []Finding
		want     string
	}{
		{"a secret is masked like anything else it is given",
			[]Finding{mail, key}, "mail [email_REDACTED] key [aws_access_key_REDACTED] end"},
		{"the same value reported twice is masked once",
			[]Finding{mail, mail}, "mail [email_REDACTED] key AKIAIOSFODNN7EXAMPLE end"},
		{"a finding from a normalised view is not a position in the text",
			[]Finding{mail, {Type: "pii", Category: "email", StartPos: 5, EndPos: 21, Metadata: map[string]string{"detected_via": "unicode_normalization"}}},
			"mail [email_REDACTED] key AKIAIOSFODNN7EXAMPLE end"},
		{"overlapping findings are one stretch of text",
			[]Finding{{Type: "pii", Category: "email", StartPos: 5, EndPos: 15}, {Type: "custom", Category: "host", StartPos: 10, EndPos: 21}},
			"mail [email_REDACTED] key AKIAIOSFODNN7EXAMPLE end"},
		{"a finding inside another",
			[]Finding{mail, {Type: "custom", Category: "host", StartPos: 10, EndPos: 17}},
			"mail [email_REDACTED] key AKIAIOSFODNN7EXAMPLE end"},
		{"a verdict is not a span",
			[]Finding{{Type: "injection", Category: "instruction_override", StartPos: 0, EndPos: 4}}, text},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(RedactContent([]byte(text), tt.findings)); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// A raw body is settled like a segment: one finding per value, a position
// only where the value is in the text as written.
func TestScanRaw_OneFindingPerValue(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewPIIScanner())
	reg.Register(NewSecretScanner())
	text := "mail ayse@example.com, db postgres://app:s3cretpw@db.internal:5432/x key AKIAIOSFODNN7EXAMPLE"
	findings, err := reg.ScanRaw(context.Background(), []byte(text), PipelineConfig{Mode: ModeSync})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	spans := map[[2]int]string{}
	for _, f := range findings {
		seen[f.Category]++
		if other, dup := spans[[2]int{f.StartPos, f.EndPos}]; dup && other == f.Type+"/"+f.Category {
			t.Errorf("%s reported twice for the same span", other)
		}
		spans[[2]int{f.StartPos, f.EndPos}] = f.Type + "/" + f.Category
		if f.Role != "" || f.Path != "" {
			t.Errorf("%s carries role %q and path %q: a raw body has neither", f.Category, f.Role, f.Path)
		}
		if !f.Placed() {
			t.Errorf("%s has no position, and nothing here is obfuscated: %+v", f.Category, f)
			continue
		}
	}
	// The password and host of the connection string are not an address:
	// one e-mail finding, the one that is written out.
	for cat, want := range map[string]int{"email": 1, "aws_access_key": 1, "connection_string": 1, "connection_string_password": 1} {
		if seen[cat] != want {
			t.Errorf("%d findings of %s, want %d: %+v", seen[cat], cat, want, findings)
		}
	}
	// And so the masking has one marker per stretch of text: the findings
	// inside the connection string are covered by its marker.
	out := string(RedactContent([]byte(text), findings))
	if out != "mail [email_REDACTED], db [connection_string_REDACTED] key [aws_access_key_REDACTED]" {
		t.Errorf("masked: %s", out)
	}
}

// What the secret scanner finds in the normalised view of a text says so,
// for every pattern: its offsets are not offsets into the text.
func TestSecretScanner_NormalisedViewIsMarked(t *testing.T) {
	// Fullwidth letters in the scheme: only the normalised text matches.
	text := "db ｐｏｓｔｇｒｅｓ://app:s3cretpw@db.internal:5432/x"
	findings, err := NewSecretScanner().Scan(context.Background(), []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Skip("the normalised view does not show a connection string here")
	}
	for _, f := range findings {
		if f.Metadata["detected_via"] == "" {
			t.Errorf("%s was found in the normalised view only and is not marked: %+v", f.Category, f)
		}
	}
	// The reading knows where its bytes came from: the findings are put
	// back on the text as written, fullwidth scheme included.
	settled := SettleRaw(findings, []byte(text))
	for _, f := range settled {
		if !f.Placed() {
			t.Errorf("%s has no position: %+v", f.Category, f)
			continue
		}
		if f.Category == "connection_string" && !strings.HasPrefix(text[f.StartPos:f.EndPos], "ｐｏｓｔｇｒｅｓ://") {
			t.Errorf("connection_string is placed at %q", text[f.StartPos:f.EndPos])
		}
	}
	if got := string(RedactContent([]byte(text), settled)); strings.Contains(got, "s3cretpw") || strings.Contains(got, "ｐｏｓｔ") {
		t.Errorf("masked text still holds the connection string: %s", got)
	}

	// A secret that only a decoded payload holds is another text's: it is
	// marked, and it has no position in this one.
	encoded := "run " + base64.StdEncoding.EncodeToString([]byte("postgres://app:s3cretpw@db.internal:5432/x")) + " now"
	findings, err = NewSecretScanner().Scan(context.Background(), []byte(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("the encoded connection string was not found")
	}
	settled = SettleRaw(findings, []byte(encoded))
	for _, f := range settled {
		if f.Metadata["detected_via"] == "" || f.Placed() {
			t.Errorf("%s from a decoded payload kept a position: %+v", f.Category, f)
		}
	}
	if UnplacedRedactions(settled) == 0 {
		t.Error("a secret that cannot be located must count as one that cannot be masked")
	}
}
