package config

import (
	"strings"
	"testing"
)

func TestLoad_ICAP(t *testing.T) {
	t.Setenv("TAMGA_ICAP_ADDR", "")
	t.Setenv("TAMGA_ICAP_ALLOWED_CLIENTS", "")
	t.Setenv("TAMGA_ICAP_MAX_CONNECTIONS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ICAPAddr != "" || len(cfg.ICAPAllowedClients) != 0 || cfg.ICAPMaxConnections != 256 {
		t.Fatalf("defaults: %q %v %d", cfg.ICAPAddr, cfg.ICAPAllowedClients, cfg.ICAPMaxConnections)
	}

	t.Setenv("TAMGA_ICAP_ADDR", " :1344 ")
	t.Setenv("TAMGA_ICAP_ALLOWED_CLIENTS", "10.1.2.3, 192.168.0.0/24")
	t.Setenv("TAMGA_ICAP_MAX_CONNECTIONS", "32")
	if cfg, err = Load(); err != nil {
		t.Fatal(err)
	}
	if cfg.ICAPAddr != ":1344" || len(cfg.ICAPAllowedClients) != 2 || cfg.ICAPMaxConnections != 32 {
		t.Fatalf("set: %q %v %d", cfg.ICAPAddr, cfg.ICAPAllowedClients, cfg.ICAPMaxConnections)
	}

	t.Setenv("TAMGA_ICAP_ALLOWED_CLIENTS", "squid.internal")
	if _, err = Load(); err == nil || !strings.Contains(err.Error(), "TAMGA_ICAP_ALLOWED_CLIENTS") {
		t.Fatalf("a host name is refused, naming the variable: %v", err)
	}
}
