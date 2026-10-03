package policy

import (
	"strings"
	"testing"
	"time"
)

func parseScan(t *testing.T, scan string) (*Policy, error) {
	t.Helper()
	return LoadFromBytes([]byte("version: \"1.0\"\nscan:\n" + scan + "\nproviders:\n  allowed: [openai]\n"))
}

func TestClassifierSettings_Defaults(t *testing.T) {
	p, err := parseScan(t, "  classifier:\n    enabled: true")
	if err != nil {
		t.Fatal(err)
	}
	s, on := p.ClassifierSettings()
	if !on {
		t.Fatal("classifier should be on")
	}
	if s.Timeout != 150*time.Millisecond || s.Threshold != 0.98 || s.MaxChars != 6000 {
		t.Fatalf("defaults: %+v", s)
	}
	if !s.CoversRole("user") || !s.CoversRole("tool") || s.CoversRole("system") || s.CoversRole("assistant") {
		t.Fatal("default roles must be user and tool")
	}
}

func TestClassifierSettings_OffUnlessEnabled(t *testing.T) {
	for _, scan := range []string{"  on_error: block", "  classifier:\n    threshold: 0.5"} {
		p, err := parseScan(t, scan)
		if err != nil {
			t.Fatal(err)
		}
		if _, on := p.ClassifierSettings(); on {
			t.Fatalf("classifier on for %q", scan)
		}
	}
	if _, on := (*Policy)(nil).ClassifierSettings(); on {
		t.Fatal("nil policy")
	}
}

func TestClassifierSettings_Explicit(t *testing.T) {
	p, err := parseScan(t, "  classifier:\n    enabled: true\n    timeout_ms: 120\n    threshold: 0.97\n    max_chars: 2000\n    roles: [tool, System]")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := p.ClassifierSettings()
	if s.Timeout != 120*time.Millisecond || s.Threshold != 0.97 || s.MaxChars != 2000 {
		t.Fatalf("%+v", s)
	}
	if !s.CoversRole("tool") || !s.CoversRole("system") || s.CoversRole("user") {
		t.Fatal("roles must be exactly the ones listed")
	}
}

func TestClassifierConfig_RejectsBadValues(t *testing.T) {
	tests := map[string]string{
		"  classifier:\n    enabled: true\n    threshold: 1.5":     "threshold",
		"  classifier:\n    enabled: true\n    timeout_ms: -5":     "timeout_ms",
		"  classifier:\n    enabled: true\n    roles: [customer]":  "unknown role",
		"  classifier:\n    enabled: true\n    max_chars: -1":      "max_chars",
		"  classifier:\n    enabled: true\n    treshold: 0.9":      "treshold",
		"  classifier:\n    enabled: true\n    timeout_ms: 600000": "timeout_ms",
	}
	for scan, want := range tests {
		if _, err := parseScan(t, scan); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want one naming %q", scan, err, want)
		}
	}
}
