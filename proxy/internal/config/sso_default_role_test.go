package config

import "testing"

func TestParseSSODefaultRole(t *testing.T) {
	for in, want := range map[string]string{"": "", "  ": "", "viewer": "viewer", " Viewer ": "viewer"} {
		got, err := ParseSSODefaultRole(in)
		if err != nil || got != want {
			t.Errorf("ParseSSODefaultRole(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	// No role that writes or administers can be handed out by default.
	for _, in := range []string{"admin", "analyst", "viewer,admin", "true", "none", "*"} {
		if got, err := ParseSSODefaultRole(in); err == nil {
			t.Errorf("ParseSSODefaultRole(%q) = %q, want an error", in, got)
		}
	}
}

// The default is closed, and a value that is not understood stops the
// proxy from starting instead of being read as one or the other.
func TestLoad_SSODefaultRole(t *testing.T) {
	t.Setenv("TAMGA_SSO_DEFAULT_ROLE", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SSODefaultRole != "" {
		t.Errorf("default SSODefaultRole = %q, want empty", cfg.SSODefaultRole)
	}
	t.Setenv("TAMGA_SSO_DEFAULT_ROLE", "viewer")
	if cfg, err = Load(); err != nil || cfg.SSODefaultRole != "viewer" {
		t.Errorf("viewer: %v, %v", cfg, err)
	}
	t.Setenv("TAMGA_SSO_DEFAULT_ROLE", "admin")
	if _, err = Load(); err == nil {
		t.Error("TAMGA_SSO_DEFAULT_ROLE=admin was accepted")
	}
}
