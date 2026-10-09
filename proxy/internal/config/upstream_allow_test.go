package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseEnvNames(t *testing.T) {
	got, err := ParseEnvNames("TAMGA_UPSTREAM_KEY_ENVS", " GATEWAY_TOKEN, ,VLLM_KEY_2 ,")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"GATEWAY_TOKEN", "VLLM_KEY_2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, err := ParseEnvNames("TAMGA_UPSTREAM_KEY_ENVS", ""); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}

	// One of the proxy's own variables in the list stops the proxy from
	// starting, in any case: on Windows the lower-case name reads the same
	// variable.
	for _, raw := range []string{"TAMGA_ADMIN_KEY", "GATEWAY_TOKEN,TAMGA_VAULT_KEY", "TAMGA_DB_URL", "tamga_admin_key", "Tamga_Jwt_Secret", "TAMGA_"} {
		for _, variable := range []string{"TAMGA_UPSTREAM_KEY_ENVS", "TAMGA_NOTIFY_URL_ENVS"} {
			_, err := ParseEnvNames(variable, raw)
			if err == nil {
				t.Errorf("%s=%q accepted", variable, raw)
				continue
			}
			if !strings.Contains(err.Error(), variable) || !strings.Contains(err.Error(), "own variables") {
				t.Errorf("%s=%q: unhelpful error: %v", variable, raw, err)
			}
		}
	}
	// Not a name a policy could refer to.
	for _, raw := range []string{"gateway_token", "Gateway_Token", "GATEWAY TOKEN", "$GATEWAY_TOKEN", "A=B", "1TOKEN", "_TOKEN", "GATEWAY-TOKEN", "GATEWAY_TOKEN;X"} {
		if _, err := ParseEnvNames("TAMGA_UPSTREAM_KEY_ENVS", raw); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
}

func TestParseUpstreamAllow(t *testing.T) {
	nets, hosts, hostPorts, err := ParseUpstreamAllow(" 10.20.0.0/16, 127.0.0.1 ,Ollama.Internal.,vllm.corp.example:8000,192.168.7.7:9000,[::1]:8080,::1,::ffff:10.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	var n []string
	for _, p := range nets {
		n = append(n, p.String())
	}
	if want := []string{"10.20.0.0/16", "127.0.0.1/32", "::1/128", "10.0.0.9/32"}; !reflect.DeepEqual(n, want) {
		t.Errorf("nets = %v, want %v", n, want)
	}
	if want := []string{"ollama.internal"}; !reflect.DeepEqual(hosts, want) {
		t.Errorf("hosts = %v, want %v", hosts, want)
	}
	if want := []string{"vllm.corp.example:8000", "192.168.7.7:9000", "[::1]:8080"}; !reflect.DeepEqual(hostPorts, want) {
		t.Errorf("hostPorts = %v, want %v", hostPorts, want)
	}

	if nets, hosts, hostPorts, err := ParseUpstreamAllow(""); err != nil || len(nets)+len(hosts)+len(hostPorts) != 0 {
		t.Errorf("empty: %v %v %v %v", nets, hosts, hostPorts, err)
	}
	for _, raw := range []string{
		"0.0.0.0/0", "::/0", "::ffff:0.0.0.0/96", "10.0.0.0/8,0.0.0.0/0",
		"*", "*.corp.example", "https://ollama.internal", "ollama.internal/v1", "ollama internal",
		"ollama.internal:", "ollama.internal:0", "ollama.internal:70000", "ollama.internal:http", ":8080", "*.x:80",
		"2130706433", "0x7f.0.0.1", "127.1", "10.0.0.0/33",
	} {
		if _, _, _, err := ParseUpstreamAllow(raw); err == nil {
			t.Errorf("%q accepted", raw)
		} else if !strings.Contains(err.Error(), "TAMGA_UPSTREAM_ALLOW") {
			t.Errorf("%q: error does not name the variable: %v", raw, err)
		}
	}
}

// Load reads the three lists and refuses to start on a bad one. The alerts
// list is a different list: what it holds does not become an upstream.
func TestLoad_UpstreamLists(t *testing.T) {
	for _, k := range []string{"TAMGA_UPSTREAM_ALLOW", "TAMGA_UPSTREAM_KEY_ENVS", "TAMGA_NOTIFY_URL_ENVS", "TAMGA_ALERT_ALLOW_PRIVATE", "TAMGA_TRUSTED_PROXIES"} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UpstreamKeyEnvs)+len(cfg.NotifyURLEnvs)+len(cfg.UpstreamAllowHosts)+len(cfg.UpstreamAllowNets)+len(cfg.UpstreamAllowHostPorts) != 0 {
		t.Fatal("the default is not empty")
	}

	t.Setenv("TAMGA_ALERT_ALLOW_PRIVATE", "10.9.0.0/16,splunk.internal")
	t.Setenv("TAMGA_UPSTREAM_ALLOW", "ollama.internal:11434,10.20.0.0/16")
	t.Setenv("TAMGA_UPSTREAM_KEY_ENVS", "GATEWAY_TOKEN")
	t.Setenv("TAMGA_NOTIFY_URL_ENVS", "ALERT_HOOK")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.UpstreamKeyEnvs, []string{"GATEWAY_TOKEN"}) || !reflect.DeepEqual(cfg.NotifyURLEnvs, []string{"ALERT_HOOK"}) ||
		!reflect.DeepEqual(cfg.UpstreamAllowHostPorts, []string{"ollama.internal:11434"}) || len(cfg.UpstreamAllowNets) != 1 || len(cfg.UpstreamAllowHosts) != 0 {
		t.Fatalf("got %+v %+v %+v %+v %+v", cfg.UpstreamKeyEnvs, cfg.NotifyURLEnvs, cfg.UpstreamAllowHostPorts, cfg.UpstreamAllowNets, cfg.UpstreamAllowHosts)
	}
	if cfg.UpstreamAllowNets[0].String() != "10.20.0.0/16" {
		t.Fatalf("the alerts list leaked into the upstream list: %v", cfg.UpstreamAllowNets)
	}
	if len(cfg.AlertAllowNets) != 1 || cfg.AlertAllowNets[0].String() != "10.9.0.0/16" || !reflect.DeepEqual(cfg.AlertAllowHosts, []string{"splunk.internal"}) {
		t.Fatalf("the alerts list changed: %v %v", cfg.AlertAllowNets, cfg.AlertAllowHosts)
	}

	for k, v := range map[string]string{
		"TAMGA_UPSTREAM_KEY_ENVS": "TAMGA_ADMIN_KEY",
		"TAMGA_NOTIFY_URL_ENVS":   "TAMGA_DB_URL",
		"TAMGA_UPSTREAM_ALLOW":    "0.0.0.0/0",
	} {
		t.Setenv(k, v)
		if _, err := Load(); err == nil {
			t.Errorf("%s=%s: the proxy would start", k, v)
		}
		t.Setenv(k, "")
	}
}
