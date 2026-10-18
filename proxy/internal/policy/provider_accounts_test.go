package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKeyFingerprint(t *testing.T) {
	// sha256("abc"), a published test vector.
	if got := KeyFingerprint("abc"); got != "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("KeyFingerprint(abc) = %s", got)
	}
}

func TestProviderAccountRuleCheck(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
version: "1.0"
provider_accounts:
  enabled: true
  providers:
    openai:
      keys: ["` + KeyFingerprint("sk-test-allowed") + `"]
      organizations: [org-allowed]
`))
	if err != nil {
		t.Fatal(err)
	}
	r := p.ProviderAccountRule("openai")
	if r == nil || r.Log || r.Replaces() {
		t.Fatalf("rule = %+v", r)
	}
	if p.ProviderAccountRule("anthropic") != nil {
		t.Fatal("a route without an entry has no rule")
	}
	for _, c := range []struct {
		req  AccountRequest
		want string
	}{
		{AccountRequest{Credentials: []string{"sk-test-allowed"}, Organizations: []string{"org-allowed"}}, ""},
		{AccountRequest{Credentials: []string{"sk-test-allowed"}, Organizations: []string{" org-allowed "}}, ""},
		{AccountRequest{Organizations: []string{"org-allowed"}}, AccountKeyMissing},
		{AccountRequest{Credentials: []string{"sk-test-other"}, Organizations: []string{"org-allowed"}}, AccountKeyNotAllowed},
		{AccountRequest{Credentials: []string{"sk-test-allowed", "sk-test-other"}, Organizations: []string{"org-allowed"}}, AccountKeyNotAllowed},
		{AccountRequest{Credentials: []string{"sk-test-allowed"}}, AccountOrganizationMissing},
		{AccountRequest{Credentials: []string{"sk-test-allowed"}, Organizations: []string{""}}, AccountOrganizationMissing},
		{AccountRequest{Credentials: []string{"sk-test-allowed"}, Organizations: []string{"org-allowed", "org-allowed"}}, AccountOrganizationNotAllowed},
		{AccountRequest{Credentials: []string{"sk-test-allowed"}, Organizations: []string{"org-other"}}, AccountOrganizationNotAllowed},
	} {
		if got := r.Check(c.req); got != c.want {
			t.Errorf("Check(%+v) = %q, want %q", c.req, got, c.want)
		}
	}

	off, _ := LoadFromBytes([]byte("version: \"1.0\"\nprovider_accounts:\n  enabled: false\n  providers:\n    openai: {organizations: [org-allowed]}\n"))
	if off.ProviderAccountRule("openai") != nil {
		t.Fatal("a disabled section has no rule")
	}
}

func TestProviderAccountIssues(t *testing.T) {
	for _, c := range []struct {
		name, yaml, field, rule string
	}{
		{"bedrock", `bedrock: {organizations: [org-x]}`, "provider_accounts.providers.bedrock", "unknown_route"},
		{"alias", `azure_openai: {replace_key_env: AZURE_API_KEY}`, "provider_accounts.providers.azure_openai", "unknown_route"},
		{"organizations off openai", `anthropic: {organizations: [org-x]}`, "provider_accounts.providers.anthropic.organizations", "route"},
		{"workspaces off anthropic", `openai: {workspaces: [wrkspc_x]}`, "provider_accounts.providers.openai.workspaces", "route"},
		{"a raw key", `openai: {keys: [sk-test-raw-key]}`, "provider_accounts.providers.openai.keys[0]", "fingerprint"},
		{"upper-case fingerprint", `openai: {keys: ["sha256:BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD"]}`, "provider_accounts.providers.openai.keys[0]", "fingerprint"},
		{"value with a comma", `openai: {organizations: ["org-a,org-b"]}`, "provider_accounts.providers.openai.organizations[0]", "value"},
		{"a proxy secret", `openai: {replace_key_env: TAMGA_ADMIN_KEY}`, "provider_accounts.providers.openai.replace_key_env", "not_allowed"},
		{"any variable", `openai: {replace_key_env: HOME}`, "provider_accounts.providers.openai.replace_key_env", "not_allowed"},
	} {
		_, err := LoadFromBytes([]byte("version: \"1.0\"\nprovider_accounts:\n  enabled: true\n  providers:\n    " + c.yaml + "\n"))
		if err == nil {
			t.Errorf("%s: loaded", c.name)
			continue
		}
		ie, ok := err.(*IssuesError)
		if !ok {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		found := false
		for _, is := range ie.Issues {
			if is.Field == c.field && is.Rule == c.rule {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no %s/%s in %+v", c.name, c.field, c.rule, ie.Issues)
		}
	}

	if _, err := LoadFromBytes([]byte("version: \"1.0\"\nprovider_accounts:\n  enabled: true\n  action: WARN\n")); err == nil || !strings.Contains(err.Error(), "provider_accounts.action") {
		t.Errorf("action WARN: %v", err)
	}
}

func TestProviderAccountIssues_PoolKeyConflicts(t *testing.T) {
	_, err := LoadFromBytes([]byte(`
version: "1.0"
providers:
  pools:
    openai:
      strategy: fallback_chain
      providers:
        - name: primary
          base_url: https://api.openai.com
          api_key_env: OPENAI_API_KEY
provider_accounts:
  enabled: true
  providers:
    openai: {replace_key_env: OPENAI_API_KEY}
`))
	if err == nil || !strings.Contains(err.Error(), "providers.pools.openai.providers[0].api_key_env") {
		t.Fatalf("pool key and replace_key_env together: %v", err)
	}
}

func TestProviderAccountIssues_Warnings(t *testing.T) {
	p, err := LoadFromBytes([]byte(`
version: "1.0"
provider_accounts:
  enabled: true
  providers:
    openai:
      keys: ["` + KeyFingerprint("sk-test") + `"]
      replace_key_env: OPENAI_API_KEY
    gemini: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, is := range ValidateSemantics(p) {
		if strings.HasPrefix(is.Field, "provider_accounts") {
			got[is.Field+"/"+is.Rule+"/"+is.Severity] = true
		}
	}
	for _, want := range []string{
		"provider_accounts.providers.openai.keys/no_effect/warning",
		"provider_accounts.providers.gemini/no_effect/warning",
	} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}

// A policy without the section keeps its JSON form, and with it its digest.
func TestProviderAccounts_AbsentFromJSONWhenUnset(t *testing.T) {
	p, err := LoadFromBytes([]byte("version: \"1.0\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "provider_accounts") {
		t.Fatalf("unset section is in the JSON form: %s", b)
	}
}
