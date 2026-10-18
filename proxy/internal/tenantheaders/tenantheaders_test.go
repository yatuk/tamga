package tenantheaders

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Made-up identifiers.
const (
	tenantID = "00000000-0000-0000-0000-00000000000a"
	policyID = "00000000-0000-0000-0000-00000000000b"
	wsA      = "11111111-1111-1111-1111-111111111111"
	wsB      = "22222222-2222-2222-2222-222222222222"
	orgA     = "33333333-3333-3333-3333-333333333333"
)

func full() Config {
	return Config{
		MicrosoftTenantID: tenantID,
		MicrosoftPolicyID: policyID,
		ChatGPTWorkspaces: []string{wsA, wsB},
		AnthropicOrgs:     []string{orgA},
		GoogleDomains:     []string{"example.com", "example.org"},
		GitHubEnterprises: []string{"example-enterprise"},
	}
}

func TestApply_PerHost(t *testing.T) {
	c := full()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, header, want string
	}{
		{"login.microsoftonline.com", MicrosoftHeader, tenantID + ":" + policyID},
		{"LOGIN.LIVE.COM:443", MicrosoftHeader, tenantID + ":" + policyID},
		{"chatgpt.com", ChatGPTHeader, wsA + "," + wsB},
		{"ab.chatgpt.com", ChatGPTHeader, wsA + "," + wsB},
		{"claude.ai", AnthropicHeader, orgA},
		{"api.anthropic.com", AnthropicHeader, orgA},
		{"accounts.google.com", GoogleHeader, "example.com,example.org"},
		{"gemini.google.com.", GoogleHeader, "example.com,example.org"},
		{"github.com", GitHubHeader, "example-enterprise"},
		{"api.business.githubcopilot.com", GitHubHeader, "example-enterprise"},
	} {
		h := http.Header{}
		got := c.Apply(tc.host, h)
		if len(got) != 1 || h.Get(tc.header) != tc.want {
			t.Errorf("%s: set %v, %s = %q, want %q", tc.host, got, tc.header, h.Get(tc.header), tc.want)
		}
	}
	for _, host := range []string{"example.invalid", "notchatgpt.com", "google.com.example.invalid", "gist.github.com", "outlook.office.com"} {
		h := http.Header{}
		if got := c.Apply(host, h); got != nil || len(h) != 0 {
			t.Errorf("%s: set %v", host, got)
		}
	}
}

// What the client sent is replaced, never kept beside the proxy's value.
func TestApply_Overwrites(t *testing.T) {
	h := http.Header{}
	h.Add(ChatGPTHeader, "99999999-9999-9999-9999-999999999999")
	h.Add(ChatGPTHeader, wsA)
	h.Set(AnthropicHeader+"3", "stray")
	full().Apply("chatgpt.com", h)
	if v := h.Values(ChatGPTHeader); len(v) != 1 || v[0] != wsA+","+wsB {
		t.Fatalf("ChatGPT header = %v", v)
	}
	full().Apply("claude.ai", h)
	if h.Get(AnthropicHeader+"3") != "" {
		t.Fatal("a stray continuation line was kept")
	}
}

func TestApply_EmptyVendorSetsNothing(t *testing.T) {
	h := http.Header{}
	h.Set(ChatGPTHeader, wsA)
	if got := (Config{}).Apply("chatgpt.com", h); got != nil {
		t.Fatalf("set %v", got)
	}
	// Not configured means not touched: the client's header stays.
	if h.Get(ChatGPTHeader) != wsA {
		t.Fatal("an unconfigured vendor's header was changed")
	}
}

func TestApply_AnthropicContinuationLines(t *testing.T) {
	var orgs []string
	for i := 0; i < 120; i++ {
		orgs = append(orgs, fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
	}
	c := Config{AnthropicOrgs: orgs}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	names := c.Apply("claude.ai", h)
	if len(names) != 3 {
		t.Fatalf("names = %v, want three lines", names)
	}
	first := h.Get(AnthropicHeader)
	if !strings.HasSuffix(first, ";n=3") {
		t.Fatalf("first line %q does not declare n=3", first)
	}
	var all []string
	all = append(all, strings.Split(strings.TrimSuffix(first, ";n=3"), ",")...)
	all = append(all, strings.Split(h.Get(AnthropicHeader+"1"), ",")...)
	all = append(all, strings.Split(h.Get(AnthropicHeader+"2"), ",")...)
	if strings.Join(all, ",") != strings.Join(orgs, ",") {
		t.Fatal("the lines do not hold the ids in order")
	}
}

func TestValidate(t *testing.T) {
	for name, c := range map[string]Config{
		"tenant without policy": {MicrosoftTenantID: tenantID},
		"tenant not a uuid":     {MicrosoftTenantID: "contoso", MicrosoftPolicyID: policyID},
		"workspace not a uuid":  {ChatGPTWorkspaces: []string{"org-abc"}},
		"workspace with comma":  {ChatGPTWorkspaces: []string{wsA + "," + wsB}},
		"org with newline":      {AnthropicOrgs: []string{orgA + "\r\nX-Evil: 1"}},
		"upper-case domain":     {GoogleDomains: []string{"Example.com"}},
		"domain with space":     {GoogleDomains: []string{"example.com, example.org"}},
		"github with comma":     {GitHubEnterprises: []string{"a,b"}},
		"too many github":       {GitHubEnterprises: strings.Split(strings.Repeat("e,", 21)[:41], ",")},
	} {
		if c.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := (Config{GoogleDomains: []string{"example.com", "consumer_accounts"}}).Validate(); err != nil {
		t.Errorf("a domain and a keyword: %v", err)
	}
}
