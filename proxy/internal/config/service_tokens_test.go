package config

import (
	"strings"
	"testing"
)

func TestServiceTokens(t *testing.T) {
	reset := func(t *testing.T) {
		for _, k := range []string{
			"TAMGA_REQUIRE_SERVICE_TOKENS", "TAMGA_ANALYZER_ADDR", "TAMGA_CLASSIFIER_ADDR",
			"TAMGA_SCANNER_SERVICE_ADDR", "TAMGA_ANALYZER_TOKEN", "TAMGA_CLASSIFIER_TOKEN",
			"TAMGA_SCANNER_SERVICE_TOKEN", "TAMGA_SERVICE_TLS_CA", "TAMGA_SERVICE_TLS_CERT",
			"TAMGA_SERVICE_TLS_KEY",
		} {
			t.Setenv(k, "")
		}
	}

	t.Run("off by default, tokens optional", func(t *testing.T) {
		reset(t)
		t.Setenv("TAMGA_CLASSIFIER_ADDR", "classifier:50052")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.RequireServiceTokens || cfg.AnalyzerToken != "" || cfg.ClassifierToken != "" {
			t.Fatalf("defaults: %+v", cfg)
		}
	})

	t.Run("tokens are read and trimmed", func(t *testing.T) {
		reset(t)
		t.Setenv("TAMGA_ANALYZER_TOKEN", " test-analyzer-token\n")
		t.Setenv("TAMGA_CLASSIFIER_TOKEN", "test-classifier-token")
		t.Setenv("TAMGA_SCANNER_SERVICE_TOKEN", "test-scanner-token")
		t.Setenv("TAMGA_SERVICE_TLS_CA", "/etc/tamga/service-ca.pem")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.AnalyzerToken != "test-analyzer-token" || cfg.ClassifierToken != "test-classifier-token" ||
			cfg.ScannerServiceToken != "test-scanner-token" || cfg.ServiceTLSCA != "/etc/tamga/service-ca.pem" {
			t.Fatalf("read: %+v", cfg)
		}
	})

	t.Run("required: the analyzer always needs one", func(t *testing.T) {
		reset(t)
		t.Setenv("TAMGA_REQUIRE_SERVICE_TOKENS", "true")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "TAMGA_ANALYZER_TOKEN") {
			t.Fatalf("want an error naming TAMGA_ANALYZER_TOKEN, got %v", err)
		}
	})

	t.Run("required: a service without an address needs none", func(t *testing.T) {
		reset(t)
		t.Setenv("TAMGA_REQUIRE_SERVICE_TOKENS", "true")
		t.Setenv("TAMGA_ANALYZER_TOKEN", "test-analyzer-token")
		if _, err := Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})

	for _, s := range []struct{ addrEnv, addr, tokenEnv string }{
		{"TAMGA_CLASSIFIER_ADDR", "classifier:50052", "TAMGA_CLASSIFIER_TOKEN"},
		{"TAMGA_SCANNER_SERVICE_ADDR", "scanner-service:50052", "TAMGA_SCANNER_SERVICE_TOKEN"},
	} {
		t.Run("required: "+s.addrEnv, func(t *testing.T) {
			reset(t)
			t.Setenv("TAMGA_REQUIRE_SERVICE_TOKENS", "true")
			t.Setenv("TAMGA_ANALYZER_TOKEN", "test-analyzer-token")
			t.Setenv(s.addrEnv, s.addr)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), s.tokenEnv) {
				t.Fatalf("want an error naming %s, got %v", s.tokenEnv, err)
			}
			t.Setenv(s.tokenEnv, "test-token")
			if _, err := Load(); err != nil {
				t.Fatalf("with the token: %v", err)
			}
		})
	}
}
