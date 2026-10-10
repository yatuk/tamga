package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

// A policy that asks REDACT for a kind of finding that cannot be masked is
// refused by the validator with the field it is in, as it is by the load.
func TestPolicyValidate_RedactThatCannotBeCarriedOut(t *testing.T) {
	cfg := Config{AdminKey: "test-key", DefaultOrgID: "org-1", PolicyPath: "/tmp/test-policy.yaml", Store: store.NewNoopStoreSilent()}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	tests := []struct{ name, yaml, field, rule string }{
		{"injection rule", "name: t\nversion: \"1.0\"\nrules:\n  injection:\n    action: REDACT\n", "rules.injection.action", "cannot_redact"},
		{"moderation rule", "name: t\nversion: \"1.0\"\nrules:\n  content_moderation_detection:\n    action: REDACT\n", "rules.content_moderation_detection.action", "cannot_redact"},
		{"answer", "name: t\nversion: \"1.0\"\noutput_rules:\n  enabled: true\n  redact_on: [email, code_leak]\n", "output_rules.redact_on[1]", "cannot_redact"},
		{"competitor action", "name: t\nversion: \"1.0\"\ncompetitors:\n  - name: Acme\n    patterns: [acme]\n    enabled: true\n    action: stop\n", "competitors[0].action", "enum"},
		{"entity action", "name: t\nversion: \"1.0\"\ncustom_entities:\n  - name: a\n    pattern: \"A-[0-9]\"\n    action: mask\n", "custom_entities[0].action", "enum"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", ts.URL+"/api/v1/policies/validate", strings.NewReader(tt.yaml))
			adminHeaders(cfg.AdminKey)(req)
			req.Header.Set("Content-Type", "application/x-yaml")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			var out struct {
				Error struct {
					Code    string `json:"code"`
					Details []struct {
						Field, Rule, Message, Severity string
					} `json:"details"`
				} `json:"error"`
			}
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", resp.StatusCode)
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if len(out.Error.Details) != 1 || out.Error.Details[0].Field != tt.field || out.Error.Details[0].Rule != tt.rule {
				t.Fatalf("details = %+v, want one issue at %s (%s)", out.Error.Details, tt.field, tt.rule)
			}
		})
	}
}

// The console sends an entity's action in lower case. It is stored under
// its name and acts as that action; a value that is no action is refused.
func TestCustomEntityCreate_ActionIsStoredByName(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	_ = os.WriteFile(policyPath, []byte("version: \"1.0\"\nname: test\n"), 0o600)
	p0, _ := policy.LoadFromFile(policyPath)
	cfg := Config{AdminKey: "test-key", PolicyStore: policy.NewPolicyStore(p0), PolicyPath: policyPath}

	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/policies/custom-entities", strings.NewReader(body))
		r.Header.Set("X-Tamga-Admin-Key", "test-key")
		w := httptest.NewRecorder()
		cfg.handleCustomEntityCreate(w, r)
		return w
	}
	if w := post(`{"name":"case_no","pattern":"CN-[0-9]{4}","severity":"high","action":"redact"}`); w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	pol := cfg.PolicyStore.GetPolicy()
	if got := pol.CustomEntities[0].Action; got != "REDACT" {
		t.Errorf("stored action = %q, want REDACT", got)
	}
	f := scanner.Finding{Type: "custom", Category: "case_no", Severity: "high"}
	if got := pol.Evaluate([]scanner.Finding{f}); got != policy.ActionRedact {
		t.Errorf("a match is given %s, want REDACT", got)
	}

	w := post(`{"name":"other","pattern":"ON-[0-9]{4}","action":"mask"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "custom_entities[1].action") {
		t.Fatalf("an unknown action: status = %d: %s", w.Code, w.Body.String())
	}
	if n := len(cfg.PolicyStore.GetPolicy().CustomEntities); n != 1 {
		t.Errorf("%d entities after a refused one, want 1", n)
	}
}
