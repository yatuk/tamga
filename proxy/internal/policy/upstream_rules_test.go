package policy

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/scanner"
)

// envSentinel stands for a secret in the proxy's environment. Nothing a
// policy does may make it show up in an error or a log line.
const envSentinel = "SENTINEL-policy-env-41c8e2"

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestKeyEnvAllowed(t *testing.T) {
	listed := &UpstreamRules{KeyEnvs: []string{"GATEWAY_TOKEN", "TAMGA_ADMIN_KEY", "lower_token"}}
	for _, tc := range []struct {
		name          string
		strict, extra bool
	}{
		{"OPENAI_API_KEY", true, true},
		{"ANTHROPIC_API_KEY", true, true},
		{"AZURE_OPENAI_API_KEY", true, true},
		{"MISTRAL_API_KEY", true, true},
		// Regional and numbered keys, as the shipped policy's example names them.
		{"ANTHROPIC_API_KEY_EU", true, true},
		{"OPENAI_API_KEY_EU", true, true},
		{"OPENAI_API_KEY_2", true, true},
		{"OPENAI_API_KEY_EU_WEST_1", true, true},

		// The proxy's own, whatever else is configured.
		{"TAMGA_ADMIN_KEY", false, false},
		{"TAMGA_VAULT_KEY", false, false},
		{"TAMGA_DB_URL", false, false},
		{"TAMGA_API_KEY", false, false},
		{"TAMGA_UPSTREAM_API_KEY_EU", false, false},
		// Windows reads these as their upper-case twins.
		{"tamga_admin_key", false, false},
		{"Tamga_Admin_Key", false, false},
		{"Tamga_Admin_Key_API_KEY", false, false},
		{"openai_api_key", false, false},
		{"Openai_Api_Key", false, false},
		{"lower_token", false, false},

		// Not a provider key.
		{"DATABASE_URL", false, false},
		{"REDIS_URL", false, false},
		{"PATH", false, false},
		{"AWS_SECRET_ACCESS_KEY", false, false},
		{"VAULT_TOKEN", false, false},
		{"API_KEY", false, false},
		{"_API_KEY", false, false},
		{"X_API_KEY_", false, false},
		{"X_API_KEYS", false, false},
		{"", false, false},

		// Nothing is trimmed or cleaned up before the judgement.
		{" OPENAI_API_KEY ", false, false},
		{"OPENAI_API_KEY\n", false, false},
		{"OPENAI_API_KEY\x00", false, false},
		{"OPENAI\x00_API_KEY", false, false},
		{"OPENAI API_KEY", false, false},
		{"$OPENAI_API_KEY", false, false},
		{"${OPENAI_API_KEY}", false, false},
		{"A=B_API_KEY", false, false},
		{"123", false, false},
		{"1_API_KEY", false, false},
		{"true", false, false},
		{"ÖPENAI_API_KEY", false, false},

		// Only through the operator's list.
		{"GATEWAY_TOKEN", false, true},
		{"GATEWAY_TOKEN ", false, false},
		{"gateway_token", false, false},
	} {
		if got := (*UpstreamRules)(nil).KeyEnvAllowed(tc.name); got != tc.strict {
			t.Errorf("strict: KeyEnvAllowed(%q) = %v, want %v", tc.name, got, tc.strict)
		}
		if got := (&UpstreamRules{}).KeyEnvAllowed(tc.name); got != tc.strict {
			t.Errorf("zero value: KeyEnvAllowed(%q) = %v, want %v", tc.name, got, tc.strict)
		}
		if got := listed.KeyEnvAllowed(tc.name); got != tc.extra {
			t.Errorf("with list: KeyEnvAllowed(%q) = %v, want %v", tc.name, got, tc.extra)
		}
	}
}

func TestNotifyEnvAllowed(t *testing.T) {
	listed := &UpstreamRules{NotifyEnvs: []string{"ALERT_HOOK"}, KeyEnvs: []string{"GATEWAY_TOKEN"}}
	for _, tc := range []struct {
		name          string
		strict, extra bool
	}{
		{"SLACK_WEBHOOK_URL", true, true},
		{"TEAMS_WEBHOOK_URL", true, true},
		{"TAMGA_WEBHOOK_URL", false, false},
		{"TAMGA_ADMIN_KEY", false, false},
		{"tamga_admin_key", false, false},
		{"slack_webhook_url", false, false},
		{"DATABASE_URL", false, false},
		{"OPENAI_API_KEY", false, false},
		{"_WEBHOOK_URL", false, false},
		{"X:-https://d.example", false, false},
		{"ALERT_HOOK", false, true},
		// The key list is not the notify list.
		{"GATEWAY_TOKEN", false, false},
	} {
		if got := (*UpstreamRules)(nil).NotifyEnvAllowed(tc.name); got != tc.strict {
			t.Errorf("strict: NotifyEnvAllowed(%q) = %v, want %v", tc.name, got, tc.strict)
		}
		if got := listed.NotifyEnvAllowed(tc.name); got != tc.extra {
			t.Errorf("with list: NotifyEnvAllowed(%q) = %v, want %v", tc.name, got, tc.extra)
		}
	}
}

func TestCheckBaseURL_VendorHostsPerRoute(t *testing.T) {
	for _, tc := range []struct {
		route, raw string
		ok         bool
	}{
		{"openai", "https://api.openai.com", true},
		{"openai", "https://api.openai.com/", true},
		{"openai", "https://api.openai.com:443", true},
		{"openai", "HTTPS://API.OPENAI.COM.", true},
		{"openai", "  https://api.openai.com  ", true},
		{"anthropic", "https://api.anthropic.com", true},
		{"gemini", "https://generativelanguage.googleapis.com", true},
		{"mistral", "https://api.mistral.ai", true},
		{"azure", "https://myres.openai.azure.com", true},
		{"azure", "https://my-res-01.openai.azure.com", true},
		{"bedrock", "https://bedrock-runtime.us-east-1.amazonaws.com", true},
		{"bedrock", "https://bedrock-runtime.eu-central-1.amazonaws.com", true},
		{"bedrock", "https://bedrock-runtime.us-gov-west-1.amazonaws.com", true},
		{"local", "http://127.0.0.1:11434", true},
		{"local", "http://127.0.0.1:11434/v1", true},

		// Another route's vendor is not this route's.
		{"openai", "https://api.anthropic.com", false},
		{"anthropic", "https://api.openai.com", false},
		{"openai", "https://myres.openai.azure.com", false},
		{"azure", "https://api.openai.com", false},
		{"openai", "https://bedrock-runtime.us-east-1.amazonaws.com", false},
		{"openai", "http://127.0.0.1:11434", false},

		// Look-alikes.
		{"openai", "https://api.openai.com.evil.example", false},
		{"openai", "https://api.openai.com.other.example", false},
		{"openai", "https://evil-api.openai.com", false},
		{"openai", "https://evilapi.openai.com", false},
		{"openai", "https://x.api.openai.com", false},
		{"openai", "https://openai.com", false},
		{"openai", "https://api.openai.com@evil.example/", false},
		{"openai", "https://api.openai.com@other.example", false},
		{"openai", "https://other.example#@api.openai.com", false},
		{"openai", "https://user:pw@api.openai.com", false},
		{"openai", "https://api.openai.com\\@evil.example", false},
		{"openai", "https://api。openai。com", false},
		{"openai", "https://арі.openai.com", false},
		{"openai", "https://api.openai.com..", false},
		{"openai", "https://api.openai.com%2eevil.example", false},
		{"azure", "https://evilopenai.azure.com", false},
		{"azure", "https://xopenai.azure.com", false},
		{"azure", "https://openai.azure.com", false},
		{"azure", "https://.openai.azure.com", false},
		{"azure", "https://a.b.openai.azure.com", false},
		{"azure", "https://-a.openai.azure.com", false},
		{"azure", "https://x.openai.azure.com.other.example", false},
		{"bedrock", "https://bedrock-runtime.evil.amazonaws.com", false},
		{"bedrock", "https://bedrock-runtime.us-east-1.amazonaws.com.evil.example", false},
		{"bedrock", "https://evilbedrock-runtime.us-east-1.amazonaws.com", false},
		{"bedrock", "https://bedrock.us-east-1.amazonaws.com", false},
		{"bedrock", "https://bedrock-runtime.a.b.amazonaws.com", false},

		// Scheme, port, path, query.
		{"openai", "http://api.openai.com", false},
		{"openai", "ftp://api.openai.com", false},
		{"openai", "//api.openai.com", false},
		{"openai", "api.openai.com", false},
		{"openai", "https://api.openai.com:8443", false},
		{"openai", "https://api.openai.com:80", false},
		{"openai", "https://api.openai.com/v1", false},
		{"openai", "https://api.openai.com/x?y=1", false},
		{"openai", "https://api.openai.com/a/../b", false},
		{"openai", "https://api.openai.com?x=1", false},
		{"openai", "https://api.openai.com?", false},
		{"openai", "https://api.openai.com#f", false},
		{"openai", "https://api.openai.com\n", true}, // trimmed, like the pool does
		{"openai", "https://api.openai.com\x00", false},
		{"openai", "https://api.open\nai.com", false},

		// No host, and addresses in every spelling.
		{"openai", "https://:443", false},
		{"openai", "https://", false},
		{"openai", "https://127.0.0.1", false},
		{"openai", "https://[::1]", false},
		{"openai", "https://[::ffff:127.0.0.1]", false},
		{"openai", "https://2130706433", false},
		{"openai", "https://0x7f.0.0.1", false},
		{"openai", "https://127.1", false},
		{"openai", "https://localhost", false},
		{"openai", "https://169.254.169.254", false},
		{"openai", "https://93.184.216.34", false},
		{"local", "http://127.0.0.1:8443", false},
		{"local", "http://127.0.0.1", false},
		{"local", "https://127.0.0.1:11434", false},
		{"local", "http://localhost:11434", false},
		{"local", "http://127.0.0.1:11434/a/../b", false},
		{"local", "http://127.0.0.1:11434//x", false},

		// Not a route.
		{"OpenAI", "https://api.openai.com", false},
		{"azure_openai", "https://myres.openai.azure.com", false},
		{" openai", "https://api.openai.com", false},
		{"custom", "https://api.openai.com", false},
	} {
		for name, rules := range map[string]*UpstreamRules{"nil": nil, "zero": {}} {
			_, err := rules.CheckBaseURL(tc.route, tc.raw)
			if (err == nil) != tc.ok {
				t.Errorf("%s rules, route %q: CheckBaseURL(%q) = %v, want ok=%v", name, tc.route, tc.raw, err, tc.ok)
			}
			if err != nil && !errors.Is(err, ErrEndpointURL) && !errors.Is(err, ErrEndpointNotAllowed) {
				t.Errorf("route %q, %q: error %v is neither kind", tc.route, tc.raw, err)
			}
			if err != nil && strings.Contains(err.Error(), "evil") {
				t.Errorf("route %q, %q: the error repeats the URL: %v", tc.route, tc.raw, err)
			}
		}
	}
}

// What is judged is what is connected to: the URL comes back with its host
// in the form that was compared.
func TestCheckBaseURL_ReturnsTheHostThatWasJudged(t *testing.T) {
	u, err := (*UpstreamRules)(nil).CheckBaseURL("openai", " HTTPS://API.OPENAI.COM.:443/ ")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "api.openai.com:443" {
		t.Fatalf("got %s://%s", u.Scheme, u.Host)
	}
}

func TestCheckBaseURL_OperatorOverrideAndLists(t *testing.T) {
	rules := &UpstreamRules{
		AllowHosts:     []string{"gateway.corp.example"},
		AllowNets:      []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")},
		AllowHostPorts: []string{"vllm.corp.example:8000", "192.168.7.7:9000"},
		RouteURLs: map[string]*url.URL{
			"azure": mustURL(t, "https://corp.internal"),
			"local": mustURL(t, "http://ollama.internal:11434"),
		},
	}
	for _, tc := range []struct {
		route, raw string
		ok         bool
	}{
		// The override: its own route, scheme, host and port.
		{"azure", "https://corp.internal", true},
		{"azure", "https://corp.internal:443/openai", true},
		{"azure", "https://CORP.internal.", true},
		{"openai", "https://corp.internal", false},
		{"azure", "https://corp.internal:8443", false},
		{"azure", "http://corp.internal", false},
		{"azure", "https://x.corp.internal", false},
		{"local", "http://ollama.internal:11434", true},
		{"local", "http://ollama.internal:6379", false},
		{"local", "http://ollama.internal", false},
		{"local", "https://ollama.internal:11434", false},
		{"openai", "http://ollama.internal:11434", false},
		// The built-in vendor hosts are still there beside an override.
		{"azure", "https://myres.openai.azure.com", true},
		{"local", "http://127.0.0.1:11434", true},

		// A listed host: any route, any port, http too. Exactly that name.
		{"openai", "http://gateway.corp.example", true},
		{"anthropic", "https://gateway.corp.example:8443/llm/v1", true},
		{"openai", "http://sub.gateway.corp.example", false},
		{"openai", "http://gateway.corp.example.evil.example", false},
		{"openai", "http://gateway.corp.example/a/../b", false},
		{"openai", "http://gateway.corp.example/a//b", false},
		{"openai", "http://gateway.corp.example/a/%2e%2e/b", false},
		{"openai", "http://user:pw@gateway.corp.example", false},
		{"openai", "http://gateway.corp.example?x=1", false},
		{"custom", "http://gateway.corp.example", true}, // judged; a pool under this key still does not load

		// A listed range: literal addresses inside it.
		{"openai", "http://10.20.3.4:8000", true},
		{"openai", "http://[::ffff:10.20.3.4]:8000", true},
		{"openai", "http://10.21.0.1:8000", false},
		{"openai", "http://169.254.169.254", false},

		// A listed host and port: that port only.
		{"openai", "http://vllm.corp.example:8000", true},
		{"openai", "http://vllm.corp.example:8001", false},
		{"openai", "http://vllm.corp.example", false},
		{"openai", "http://192.168.7.7:9000", true},
		{"openai", "http://192.168.7.7:22", false},
	} {
		_, err := rules.CheckBaseURL(tc.route, tc.raw)
		if (err == nil) != tc.ok {
			t.Errorf("route %q: CheckBaseURL(%q) = %v, want ok=%v", tc.route, tc.raw, err, tc.ok)
		}
	}
}

func poolYAML(route, baseURL, keyEnv string) string {
	s := "version: \"1.0\"\nname: with-pool\nproviders:\n  pools:\n    " + route + ":\n      providers:\n        - name: ep\n          base_url: " + baseURL + "\n"
	if keyEnv != "" {
		s += "          api_key_env: " + keyEnv + "\n"
	}
	return s
}

// The loader is where every way of installing a policy passes, so that is
// where a pool the rules refuse is turned down. The values judged are the
// decoded ones: YAML makes strings of 123 and true, decodes !!binary and
// expands anchors before anything here sees them.
func TestLoadFromBytes_RejectsDisallowedPools(t *testing.T) {
	for _, name := range []string{"TAMGA_ADMIN_KEY", "TAMGA_VAULT_KEY", "TAMGA_DB_URL", "DATABASE_URL", "REDIS_URL", "AWS_SECRET_ACCESS_KEY"} {
		t.Setenv(name, envSentinel)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("TAMGA_ADMIN_KEY"))

	for _, tc := range []struct {
		name, yaml, field string
	}{
		{"tamga name", poolYAML("openai", "https://api.openai.com", "TAMGA_ADMIN_KEY"), "providers.pools.openai.providers[0].api_key_env"},
		{"vault key", poolYAML("openai", "https://api.openai.com", "TAMGA_VAULT_KEY"), "api_key_env"},
		{"db url", poolYAML("openai", "https://api.openai.com", "TAMGA_DB_URL"), "api_key_env"},
		{"lower case", poolYAML("openai", "https://api.openai.com", "tamga_admin_key"), "api_key_env"},
		{"mixed case", poolYAML("openai", "https://api.openai.com", "Tamga_Admin_Key_API_KEY"), "api_key_env"},
		{"database url", poolYAML("openai", "https://api.openai.com", "DATABASE_URL"), "api_key_env"},
		{"redis url", poolYAML("openai", "https://api.openai.com", "REDIS_URL"), "api_key_env"},
		{"path", poolYAML("openai", "https://api.openai.com", "PATH"), "api_key_env"},
		{"aws secret", poolYAML("openai", "https://api.openai.com", "AWS_SECRET_ACCESS_KEY"), "api_key_env"},
		{"quoted spaces", poolYAML("openai", "https://api.openai.com", `" OPENAI_API_KEY "`), "api_key_env"},
		{"quoted newline", poolYAML("openai", "https://api.openai.com", `"OPENAI_API_KEY\n"`), "api_key_env"},
		{"nul", poolYAML("openai", "https://api.openai.com", `"OPENAI_API_KEY\0"`), "api_key_env"},
		{"dollar", poolYAML("openai", "https://api.openai.com", `"$OPENAI_API_KEY"`), "api_key_env"},
		{"equals", poolYAML("openai", "https://api.openai.com", "A=B_API_KEY"), "api_key_env"},
		{"number", poolYAML("openai", "https://api.openai.com", "123"), "api_key_env"},
		{"bool", poolYAML("openai", "https://api.openai.com", "true"), "api_key_env"},
		{"binary", poolYAML("openai", "https://api.openai.com", "!!binary "+b64), "api_key_env"},

		{"other host", poolYAML("openai", "https://evil.example", "OPENAI_API_KEY"), "providers.pools.openai.providers[0].base_url"},
		{"other vendor", poolYAML("openai", "https://api.anthropic.com", ""), "base_url"},
		{"userinfo", poolYAML("openai", "https://user:pw-"+envSentinel+"@api.openai.com", ""), "base_url"},
		{"loopback", poolYAML("openai", "http://127.0.0.1:8080", ""), "base_url"},
		{"no host", poolYAML("openai", "https://:443", ""), "base_url"},
		{"not a url", poolYAML("openai", "not-a-url", ""), "base_url"},
		{"quoted newline in host", poolYAML("openai", `"https://api.openai.com\n.evil.example"`, ""), "base_url"},
		{"binary url", poolYAML("openai", "!!binary "+base64.StdEncoding.EncodeToString([]byte("https://evil.example")), ""), "base_url"},

		{"route in capitals", poolYAML("OpenAI", "https://api.openai.com", ""), "providers.pools"},
		{"alias route", poolYAML("azure_openai", "https://myres.openai.azure.com", ""), "providers.pools"},
		{"route with a space", poolYAML(`" openai"`, "https://api.openai.com", ""), "providers.pools"},
		{"custom route", poolYAML("custom", "https://api.openai.com", ""), "providers.pools"},

		// One pool written once and aliased under a second route: each route
		// judges it for itself, and anthropic's host is not openai's.
		{"anchor", "version: \"1.0\"\nname: a\nproviders:\n  pools:\n    anthropic: &p\n      providers:\n        - name: ep\n          base_url: https://api.anthropic.com\n    openai: *p\n",
			"providers.pools.openai.providers[0].base_url"},
	} {
		_, err := LoadFromBytes([]byte(tc.yaml))
		if err == nil {
			t.Errorf("%s: the policy loaded", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.field) {
			t.Errorf("%s: error does not name %q: %v", tc.name, tc.field, err)
		}
		for _, leak := range []string{envSentinel, "evil.example", "pw-"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error carries a value (%q): %v", tc.name, leak, err)
			}
		}
	}
}

func TestLoadFromBytes_AcceptsAllowedPools(t *testing.T) {
	for name, y := range map[string]string{
		"vendor":        poolYAML("openai", "https://api.openai.com", "OPENAI_API_KEY"),
		"regional key":  poolYAML("anthropic", "https://api.anthropic.com", "ANTHROPIC_API_KEY_EU"),
		"capitals, dot": poolYAML("openai", "HTTPS://API.OPENAI.COM.", ""),
		"default port":  poolYAML("openai", "https://api.openai.com:443", ""),
		"azure":         poolYAML("azure", "https://myres.openai.azure.com", "AZURE_OPENAI_API_KEY"),
		"bedrock":       poolYAML("bedrock", "https://bedrock-runtime.eu-west-1.amazonaws.com", ""),
		"local default": poolYAML("local", "http://127.0.0.1:11434", ""),
		"no pools":      "version: \"1.0\"\nname: plain\nproviders:\n  allowed: [openai, azure_openai]\n",
	} {
		if _, err := LoadFromBytes([]byte(y)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The rules a policy is loaded under are set once by main; a test opts into
// looser ones explicitly and puts the strict default back.
func TestLoadFromBytes_UnderOperatorRules(t *testing.T) {
	listed := poolYAML("openai", "http://127.0.0.1:9999", "GATEWAY_TOKEN")
	override := poolYAML("azure", "https://corp.internal", "")
	wrongRoute := poolYAML("openai", "https://corp.internal", "")
	wrongPort := poolYAML("azure", "https://corp.internal:8443", "")

	for _, y := range []string{listed, override, wrongRoute, wrongPort} {
		if _, err := LoadFromBytes([]byte(y)); err == nil {
			t.Fatal("loaded under the strict default")
		}
	}
	restore := SetUpstreamRules(&UpstreamRules{
		KeyEnvs:   []string{"GATEWAY_TOKEN"},
		AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		RouteURLs: map[string]*url.URL{"azure": mustURL(t, "https://corp.internal")},
	})
	for name, y := range map[string]string{"listed": listed, "override": override} {
		if _, err := LoadFromBytes([]byte(y)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, y := range map[string]string{"override on another route": wrongRoute, "another port on the override host": wrongPort} {
		if _, err := LoadFromBytes([]byte(y)); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	restore()
	if ActiveUpstreamRules() != nil {
		t.Fatal("restore did not put the strict default back")
	}
	if _, err := LoadFromBytes([]byte(listed)); err == nil {
		t.Fatal("still loads after restore")
	}
}

func TestValidateSemantics_ReportsPoolsAndNotify(t *testing.T) {
	p := &Policy{
		Name: "t",
		Rules: map[string]Rule{"pii_detection": {Action: ActionWarn, Notify: []Notify{
			{Webhook: "${SLACK_WEBHOOK_URL}"}, {Webhook: "${TAMGA_ADMIN_KEY}"}, {Webhook: "https://hooks.example.com/x"},
		}}},
		Providers: &Providers{Pools: map[string]ProviderUpstreamPool{
			"openai": {Endpoints: []ProviderUpstreamEndpoint{
				{Name: "a", BaseURL: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
				{Name: "b", BaseURL: "https://evil.example", APIKeyEnv: "TAMGA_VAULT_KEY"},
			}},
			"custom": {Endpoints: []ProviderUpstreamEndpoint{{Name: "c", BaseURL: "https://api.openai.com"}}},
		}},
	}
	got := map[string]string{}
	for _, i := range ValidateSemantics(p) {
		got[i.Field] = i.Severity + "/" + i.Rule
		if strings.Contains(i.Message, "evil.example") || strings.Contains(i.Message, "TAMGA_VAULT_KEY") {
			t.Errorf("%s: message repeats a value: %s", i.Field, i.Message)
		}
	}
	for field, want := range map[string]string{
		"providers.pools.openai.providers[1].base_url":    "error/not_allowed",
		"providers.pools.openai.providers[1].api_key_env": "error/not_allowed",
		`providers.pools["custom"]`:                       "error/unknown_route",
		"rules.pii_detection.notify[1].webhook":           "warning/not_allowed",
	} {
		if got[field] != want {
			t.Errorf("%s: got %q, want %q (all: %v)", field, got[field], want, got)
		}
	}
	for _, field := range []string{"providers.pools.openai.providers[0].base_url", "providers.pools.openai.providers[0].api_key_env", "rules.pii_detection.notify[0].webhook", "rules.pii_detection.notify[2].webhook"} {
		if _, bad := got[field]; bad {
			t.Errorf("%s was reported", field)
		}
	}
}

// lockedBuf collects log output from the watcher's goroutine.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func captureLog(t *testing.T) *lockedBuf {
	t.Helper()
	buf := &lockedBuf{}
	prev := log.Logger
	log.Logger = zerolog.New(buf)
	t.Cleanup(func() { log.Logger = prev })
	return buf
}

const goodPolicyYAML = "version: \"1.0\"\nname: good-policy\nrules: {}\n"

// Startup, a reload and the file watcher all read the file through the
// loader. A file with a hostile pool fails to load as any invalid policy
// does, and the policy in force stays.
func TestFilePaths_RejectHostilePoolAndKeepThePolicy(t *testing.T) {
	t.Setenv("TAMGA_ADMIN_KEY", envSentinel)
	logs := captureLog(t)
	hostile := []byte(poolYAML("openai", "https://user:"+envSentinel+"@evil.example", "TAMGA_ADMIN_KEY"))

	dir := t.TempDir()
	path := filepath.Join(dir, "tamga-policy.yaml")

	// Startup: main calls LoadFromFile and exits on an error.
	if err := os.WriteFile(path, hostile, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(path); err == nil {
		t.Fatal("startup: the hostile file loaded")
	} else if !strings.Contains(err.Error(), "providers.pools.openai.providers[0]") {
		t.Fatalf("startup: error does not name the field: %v", err)
	} else if strings.Contains(err.Error(), envSentinel) || strings.Contains(err.Error(), "evil.example") {
		t.Fatalf("startup: error carries a value: %v", err)
	}

	// Reload (POST /policies/reload and everything that writes then reloads).
	if err := os.WriteFile(path, []byte(goodPolicyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	p0, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewPolicyStore(p0)
	stop, err := WatchPolicy(path, store, func() { t.Error("onReload ran for a rejected policy") })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if err := os.WriteFile(path, hostile, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Reload(path); err == nil {
		t.Fatal("reload: the hostile file was accepted")
	}
	if store.GetPolicy() != p0 {
		t.Fatal("reload: the policy in force changed")
	}

	// The watcher: give it time to see the write and fail the same way.
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "policy reload failed") {
		if time.Now().After(deadline) {
			t.Fatalf("the watcher never reported the rejected file; log:\n%s", logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if store.GetPolicy() != p0 {
		t.Fatal("watcher: the policy in force changed")
	}
	if out := logs.String(); strings.Contains(out, envSentinel) || strings.Contains(out, "evil.example") {
		t.Fatalf("the log carries a value:\n%s", out)
	} else if !strings.Contains(out, "providers.pools.openai.providers[0]") {
		t.Fatalf("the log does not name the field:\n%s", out)
	}
}

// A YAML error can quote what it choked on. A base_url is a string field and
// takes any scalar, so the decoder never has reason to quote one; the values
// it does quote are for fields of another type.
func TestLoadFromBytes_ParseErrorsDoNotQuoteABaseURL(t *testing.T) {
	secret := "https://user:" + envSentinel + "@evil.example"
	for name, y := range map[string]string{
		"sequence":   "version: \"1.0\"\nname: a\nproviders:\n  pools:\n    openai:\n      providers:\n        - name: ep\n          base_url: [" + secret + "]\n",
		"mapping":    "version: \"1.0\"\nname: a\nproviders:\n  pools:\n    openai:\n      providers:\n        - name: ep\n          base_url: {u: " + secret + "}\n",
		"unknown":    "version: \"1.0\"\nname: a\nproviders:\n  pools:\n    openai:\n      providers:\n        - name: ep\n          base_url: " + secret + "\n          nope: 1\n",
		"bad indent": "version: \"1.0\"\nname: a\nproviders:\n  pools:\n    openai:\n      providers:\n        - name: ep\n          base_url: " + secret + "\n       x: 1\n",
		"plain":      "version: \"1.0\"\nname: a\nproviders:\n  pools:\n    openai:\n      providers:\n        - name: ep\n          base_url: " + secret + "\n",
	} {
		_, err := LoadFromBytes([]byte(y))
		if err == nil {
			t.Errorf("%s: loaded", name)
			continue
		}
		if strings.Contains(err.Error(), envSentinel) {
			t.Errorf("%s: the error quotes the URL: %v", name, err)
		}
	}
}

// Notify references: one variable, of an allowed name, and nothing else.
func TestNotifyURL_OnlyAllowedNamesAreRead(t *testing.T) {
	logs := captureLog(t)
	old := notifyEnv
	t.Cleanup(func() { notifyEnv = old })
	var asked []string
	notifyEnv = func(name string) (string, bool) {
		asked = append(asked, name)
		switch name {
		case "SLACK_WEBHOOK_URL":
			return "https://hooks.example.com/slack", true
		case "A_WEBHOOK_URL", "B_WEBHOOK_URL", "X_WEBHOOK_URL":
			return "https://hooks.example.com/other", true
		case "ALERT_HOOK":
			return "https://hooks.example.com/listed", true
		}
		// Everything else stands for a secret.
		return envSentinel, true
	}
	urlsFor := func(webhook string) []string {
		p := &Policy{Version: "1.0", Name: "t", Rules: map[string]Rule{
			"pii_detection": {Action: ActionWarn, Sensitivity: "low", Notify: []Notify{{Webhook: webhook}}},
		}}
		return p.NotifyURLsForFindings([]scanner.Finding{{Type: "pii", Category: "email", Severity: "medium"}})
	}

	if got := urlsFor("${SLACK_WEBHOOK_URL}"); len(got) != 1 || got[0] != "https://hooks.example.com/slack" {
		t.Fatalf("${SLACK_WEBHOOK_URL} = %v", got)
	}
	asked = nil
	for _, ref := range []string{
		"${TAMGA_ADMIN_KEY}", "${tamga_admin_key}", "${TAMGA_WEBHOOK_URL}", "${DATABASE_URL}", "${OPENAI_API_KEY}",
		"${REDIS_URL}", "${PATH}", "${slack_webhook_url}", "${A_WEBHOOK_URL}${B_WEBHOOK_URL}", "${X:-https://d.example}",
		"${ALERT_HOOK}", "${}", "${ SLACK_WEBHOOK_URL }",
	} {
		if got := urlsFor(ref); len(got) != 0 {
			t.Errorf("%s resolved to %v", ref, got)
		}
	}
	if len(asked) != 0 {
		t.Errorf("the environment was read for %v", asked)
	}
	// Not a reference at all: left as written, never expanded.
	for _, v := range []string{"https://h.example/${X_WEBHOOK_URL}", "https://h.example/?k=${TAMGA_ADMIN_KEY}"} {
		if got := urlsFor(v); len(got) != 1 || got[0] != v {
			t.Errorf("%s = %v, want it left as written", v, got)
		}
	}
	if len(asked) != 0 {
		t.Errorf("the environment was read for %v", asked)
	}

	// A name the operator listed is read; the key list does not count.
	restore := SetUpstreamRules(&UpstreamRules{NotifyEnvs: []string{"ALERT_HOOK"}, KeyEnvs: []string{"GATEWAY_TOKEN"}})
	defer restore()
	if got := urlsFor("${ALERT_HOOK}"); len(got) != 1 || got[0] != "https://hooks.example.com/listed" {
		t.Errorf("listed name = %v", got)
	}
	for _, ref := range []string{"${GATEWAY_TOKEN}", "${alert_hook}", "${TAMGA_ADMIN_KEY}"} {
		if got := urlsFor(ref); len(got) != 0 {
			t.Errorf("%s resolved to %v", ref, got)
		}
	}
	if strings.Contains(logs.String(), envSentinel) {
		t.Fatalf("the log carries a value:\n%s", logs.String())
	}
}

// The shipped policy loads under the strict default, and its commented pool
// example names keys the default accepts.
func TestShippedPolicy_LoadsUnderStrictRules(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tamga-policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromBytes(data); err != nil {
		t.Fatalf("shipped policy: %v", err)
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY_EU"} {
		if !strings.Contains(string(data), fmt.Sprintf("api_key_env: %q", name)) {
			t.Fatalf("the shipped example no longer names %s; update this test", name)
		}
		if !(*UpstreamRules)(nil).KeyEnvAllowed(name) {
			t.Errorf("%s from the shipped example is refused", name)
		}
	}
}
