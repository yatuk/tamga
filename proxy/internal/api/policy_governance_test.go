package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
)

const govBasePolicy = `version: "1.0"
name: base
rules:
  pii_detection:
    action: REDACT
    sensitivity: medium
    notify:
      - channel: slack
        webhook: https://example.invalid/hook-SECRET
  secret_detection:
    action: BLOCK
providers:
  allowed: [openai, anthropic]
custom_entities:
  - name: EMPLOYEE_ID
    pattern: 'EMP-[0-9]{6}'
    severity: high
    action: REDACT
`

const govDualControl = "governance:\n  require_dual_control: true\n"

const (
	govAdminKey  = "static-admin-key-for-tests"
	govJWTSecret = "jwt-secret-for-tests-0123456789abcdef"
)

// govFixture is the management API behind its real auth middleware, with a
// policy file, a history and an audit log, and one credential of each kind.
type govFixture struct {
	t       *testing.T
	cfg     Config
	handler http.Handler
	path    string
	audit   *incidents.AuditRing
	hist    *history.FileStore

	adminA, adminB     apikeys.CreatedKey
	analystA, analystB apikeys.CreatedKey
}

func newGovFixture(t *testing.T, policyYAML string) *govFixture {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(path, []byte(policyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	pol, err := policy.LoadFromFile(path)
	if err != nil {
		t.Fatalf("fixture policy: %v", err)
	}
	hist, err := history.NewFileStore(filepath.Join(dir, "history"))
	if err != nil {
		t.Fatal(err)
	}
	keys := apikeys.NewMemoryStore()
	f := &govFixture{t: t, path: path, audit: incidents.NewAuditRing(64), hist: hist}
	for _, k := range []struct {
		dst   *apikeys.CreatedKey
		scope string
	}{{&f.adminA, "admin"}, {&f.adminB, "admin"}, {&f.analystA, "write"}, {&f.analystB, "write"}} {
		created, err := keys.Create("same label", k.scope)
		if err != nil {
			t.Fatal(err)
		}
		*k.dst = created
	}
	f.cfg = Config{
		AdminKey:      govAdminKey,
		JWTSecret:     govJWTSecret,
		APIKeys:       keys,
		PolicyPath:    path,
		PolicyStore:   policy.NewPolicyStore(pol),
		PolicyHistory: hist,
		Audit:         f.audit,
		Started:       time.Now(),
	}
	f.handler = NewHandler(f.cfg)
	return f
}

type govResponse struct {
	status int
	raw    string
	body   map[string]interface{}
}

func (r govResponse) code() string { s, _ := r.body["code"].(string); return s }

// do sends a request with a key in X-Tamga-Admin-Key. extra headers are set
// as given, so a test can try to pass itself off as someone else.
func (f *govFixture) do(method, path, key, body string, extra map[string]string) govResponse {
	f.t.Helper()
	req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
	if key != "" {
		req.Header.Set("X-Tamga-Admin-Key", key)
	}
	if strings.HasPrefix(strings.TrimSpace(body), "{") {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	out := govResponse{status: w.Code, raw: w.Body.String()}
	_ = json.Unmarshal(w.Body.Bytes(), &out.body)
	return out
}

func (f *govFixture) put(key, yamlText string) govResponse {
	return f.do(http.MethodPut, "/policies", key, yamlText, nil)
}

func (f *govFixture) propose(key, yamlText string) govResponse {
	body, _ := json.Marshal(map[string]string{"message": "test", "yaml": yamlText})
	return f.do(http.MethodPost, "/policies/proposals", key, string(body), nil)
}

func (f *govFixture) approve(key, id string) govResponse {
	return f.do(http.MethodPost, "/policies/proposals/"+id+"/approve", key, "", nil)
}

func (f *govFixture) reject(key, id string) govResponse {
	return f.do(http.MethodPost, "/policies/proposals/"+id+"/reject", key, "", nil)
}

func (f *govFixture) mustPropose(key, yamlText string) string {
	f.t.Helper()
	res := f.propose(key, yamlText)
	if res.status != http.StatusCreated {
		f.t.Fatalf("propose: %d %s", res.status, res.raw)
	}
	id, _ := res.body["id"].(string)
	return id
}

func (f *govFixture) fileText() string {
	f.t.Helper()
	b, err := os.ReadFile(f.path)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// lastAudit returns the newest audit entry of a kind.
func (f *govFixture) lastAudit(kind string) (incidents.AuditEntry, bool) {
	for _, e := range f.audit.List(0) {
		if e.Kind == kind {
			return e, true
		}
	}
	return incidents.AuditEntry{}, false
}

func stringsOf(v interface{}) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		out := make([]string, 0, len(s))
		for _, x := range s {
			str, _ := x.(string)
			out = append(out, str)
		}
		return out
	}
	return nil
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func govEdit(old, new string) string { return strings.Replace(govBasePolicy, old, new, 1) }

// --- 1. identity ---

// The actor is the credential. Headers and body fields that name someone
// else change nothing, for every kind of credential.
func TestActorComesFromTheCredential(t *testing.T) {
	f := newGovFixture(t, govBasePolicy)
	token, err := auth.CreateToken(auth.Claims{Sub: "gh-4711", Email: "ada@example.invalid", Role: "admin"}, []byte(govJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	spoof := map[string]string{
		"X-Tamga-Actor":      "mallory",
		"X-Tamga-User-Id":    "mallory",
		"X-Tamga-User-Email": "mallory@example.invalid",
		"X-Tamga-Role":       "admin",
	}
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"static admin key", map[string]string{"X-Tamga-Admin-Key": govAdminKey}, ActorAdminKey},
		{"api key, admin scope", map[string]string{"X-Tamga-Admin-Key": f.adminA.RawKey}, "apikey:" + f.adminA.ID},
		{"api key, write scope", map[string]string{"X-Tamga-Admin-Key": f.analystA.RawKey}, "apikey:" + f.analystA.ID},
		{"session token", map[string]string{"Authorization": "Bearer " + token}, "user:gh-4711"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			for k, v := range spoof {
				headers[k] = v
			}
			for k, v := range tc.headers {
				headers[k] = v
			}
			body, _ := json.Marshal(map[string]string{
				"message": "m", "yaml": govEdit("action: REDACT", "action: WARN"),
				"author": "mallory", "actor": "mallory", "approved_by": "mallory", "rejected_by": "mallory",
			})
			res := f.do(http.MethodPost, "/policies/proposals", "", string(body), headers)
			if res.status != http.StatusCreated {
				t.Fatalf("propose: %d %s", res.status, res.raw)
			}
			if got := res.body["author"]; got != tc.want {
				t.Errorf("author = %v, want %q", got, tc.want)
			}
			id, _ := res.body["id"].(string)
			if e, ok := f.lastAudit("policy.proposal.create"); !ok || e.Actor != tc.want || e.Target != id {
				t.Errorf("audit entry = %+v, want actor %q", e, tc.want)
			}

			rej := f.do(http.MethodPost, "/policies/proposals/"+id+"/reject", "", `{"rejected_by":"mallory"}`, headers)
			if rej.status != http.StatusOK || rej.body["rejected_by"] != tc.want {
				t.Errorf("reject: %d rejected_by=%v, want %q", rej.status, rej.body["rejected_by"], tc.want)
			}
			if strings.Contains(res.raw+rej.raw, "mallory") {
				t.Errorf("a client-supplied name reached the record: %s %s", res.raw, rej.raw)
			}
		})
	}

	// Two keys with the same label are two actors; the secret is not the actor.
	if f.adminA.ID == f.adminB.ID {
		t.Fatal("fixture keys share an id")
	}
	for _, e := range f.audit.List(0) {
		if strings.Contains(e.Actor, f.adminA.RawKey) || strings.Contains(e.Actor, govAdminKey) {
			t.Errorf("an actor carries a secret: %q", e.Actor)
		}
	}
}

func TestIdentityAccessors(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if ActorFrom(r.Context()) != "" || RoleFrom(r.Context()) != "" {
		t.Error("a bare request has an identity")
	}
	r = withIdentity(r, "apikey:k1", "analyst")
	if ActorFrom(r.Context()) != "apikey:k1" || RoleFrom(r.Context()) != "analyst" {
		t.Errorf("got %q / %q", ActorFrom(r.Context()), RoleFrom(r.Context()))
	}
}

// --- 2. audit ---

// Every way of changing the policy leaves an audit entry with who, from
// where and what by path, and never a value.
func TestEveryPolicyChangeIsAudited(t *testing.T) {
	cases := []struct {
		name        string
		run         func(t *testing.T, f *govFixture)
		kind        string
		source      string
		actor       func(f *govFixture) string
		changed     string
		wantHistory bool
	}{
		{
			name: "PUT",
			run: func(t *testing.T, f *govFixture) {
				if res := f.put(f.adminA.RawKey, govEdit("action: REDACT", "action: WARN")); res.status != http.StatusOK {
					t.Fatalf("%d %s", res.status, res.raw)
				}
			},
			kind: "policy.put", source: sourcePut,
			actor:   func(f *govFixture) string { return "apikey:" + f.adminA.ID },
			changed: "rules.pii_detection.action", wantHistory: true,
		},
		{
			name: "reload after an edit on disk",
			run: func(t *testing.T, f *govFixture) {
				_ = os.WriteFile(f.path, []byte(govEdit("allowed: [openai, anthropic]", "allowed: [openai]")), 0o600)
				// A reload installs the file as it is, privileged sections
				// included, so it is not for an analyst.
				if res := f.do(http.MethodPost, "/policies/reload", f.analystA.RawKey, "", nil); res.status != http.StatusForbidden {
					t.Fatalf("analyst reload: %d %s", res.status, res.raw)
				}
				if res := f.do(http.MethodPost, "/policies/reload", f.adminA.RawKey, "", nil); res.status != http.StatusOK {
					t.Fatalf("%d %s", res.status, res.raw)
				}
			},
			kind: "policy.reload", source: sourceReload,
			actor:   func(f *govFixture) string { return "apikey:" + f.adminA.ID },
			changed: "providers.allowed", wantHistory: true,
		},
		{
			name: "proposal approve",
			run: func(t *testing.T, f *govFixture) {
				id := f.mustPropose(f.adminA.RawKey, govEdit("hook-SECRET", "hook-OTHER"))
				if res := f.approve(f.adminB.RawKey, id); res.status != http.StatusOK {
					t.Fatalf("%d %s", res.status, res.raw)
				}
			},
			kind: "policy.proposal.approve", source: sourceApprove,
			actor:   func(f *govFixture) string { return "apikey:" + f.adminB.ID },
			changed: "rules.pii_detection.notify", wantHistory: true,
		},
		{
			name: "custom entity add",
			run: func(t *testing.T, f *govFixture) {
				res := f.do(http.MethodPost, "/policies/custom-entities", f.analystA.RawKey,
					`{"name":"BADGE","pattern":"BDG-[0-9]{4}","severity":"low","action":"LOG"}`, nil)
				if res.status != http.StatusCreated {
					t.Fatalf("%d %s", res.status, res.raw)
				}
			},
			kind: "policy.custom_entity.add", source: sourceEntityAdd,
			actor:   func(f *govFixture) string { return "apikey:" + f.analystA.ID },
			changed: "custom_entities.BADGE", wantHistory: true,
		},
		{
			name: "custom entity delete",
			run: func(t *testing.T, f *govFixture) {
				if res := f.do(http.MethodDelete, "/policies/custom-entities/EMPLOYEE_ID", govAdminKey, "", nil); res.status != http.StatusOK {
					t.Fatalf("%d %s", res.status, res.raw)
				}
			},
			kind: "policy.custom_entity.delete", source: sourceEntityDelete,
			actor:   func(*govFixture) string { return ActorAdminKey },
			changed: "custom_entities.EMPLOYEE_ID", wantHistory: true,
		},
		{
			name: "rollback",
			run: func(t *testing.T, f *govFixture) {
				rev, err := f.hist.AppendRevision(history.Revision{Author: "seed", YAML: govEdit("action: BLOCK", "action: WARN")})
				if err != nil {
					t.Fatal(err)
				}
				if res := f.do(http.MethodPost, "/policies/rollback/"+rev.ID, f.adminA.RawKey, "", nil); res.status != http.StatusOK {
					t.Fatalf("%d %s", res.status, res.raw)
				}
			},
			kind: "policy.rollback", source: sourceRollback,
			actor:   func(f *govFixture) string { return "apikey:" + f.adminA.ID },
			changed: "rules.secret_detection.action", wantHistory: true,
		},
		{
			name: "file watcher",
			run: func(t *testing.T, f *govFixture) {
				// What main's watcher callback does after it reloaded the store.
				before := f.cfg.PolicyStore.GetPolicy()
				_ = os.WriteFile(f.path, []byte(govBasePolicy+"vault:\n  enabled: true\n"), 0o600)
				if err := f.cfg.PolicyStore.Reload(f.path); err != nil {
					t.Fatal(err)
				}
				RecordPolicyFileChange(f.audit, f.hist, f.path, before, f.cfg.PolicyStore.GetPolicy())
			},
			kind: "policy.file_change", source: sourceFileWatcher,
			actor:   func(*govFixture) string { return "" },
			changed: "vault.enabled", wantHistory: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGovFixture(t, govBasePolicy)
			revsBefore, _ := f.hist.ListRevisions()
			tc.run(t, f)

			e, ok := f.lastAudit(tc.kind)
			if !ok {
				t.Fatalf("no %s entry; log: %+v", tc.kind, f.audit.List(0))
			}
			if e.Actor != tc.actor(f) {
				t.Errorf("actor = %q, want %q", e.Actor, tc.actor(f))
			}
			if e.Detail["source"] != tc.source {
				t.Errorf("source = %v, want %q", e.Detail["source"], tc.source)
			}
			changed := stringsOf(e.Detail["changed"])
			if !contains(changed, tc.changed) {
				t.Errorf("changed = %v, want it to hold %q", changed, tc.changed)
			}
			section, _, _ := strings.Cut(tc.changed, ".")
			if !contains(stringsOf(e.Detail["sections"]), section) {
				t.Errorf("sections = %v, want %q", e.Detail["sections"], section)
			}
			if section == "rules" && len(stringsOf(e.Detail["rules"])) != 1 {
				t.Errorf("rules = %v, want the one rule", e.Detail["rules"])
			}

			// Paths only: no webhook, no pattern text, no key.
			logged, _ := json.Marshal(f.audit.List(0))
			for _, secret := range []string{"hook-SECRET", "hook-OTHER", "example.invalid", "EMP-[0-9]", "BDG-[0-9]", f.adminA.RawKey, f.analystA.RawKey, govAdminKey} {
				if strings.Contains(string(logged), secret) {
					t.Errorf("the audit log holds %q: %s", secret, logged)
				}
			}
			if ok, at := f.audit.Verify(); !ok {
				t.Errorf("audit chain broken at %d", at)
			}

			revsAfter, _ := f.hist.ListRevisions()
			added := len(revsAfter) - len(revsBefore)
			if tc.name == "rollback" {
				added-- // the revision the test seeded
			}
			if tc.wantHistory && added != 1 {
				t.Errorf("history grew by %d revisions, want 1", added)
			}
			if tc.wantHistory && len(revsAfter) > 0 {
				newest := revsAfter[0]
				for _, r := range revsAfter {
					if r.Timestamp.After(newest.Timestamp) {
						newest = r
					}
				}
				if newest.YAML != f.fileText() {
					t.Errorf("the newest revision is not the text in force")
				}
			}
		})
	}
}

// A reload that changes nothing, which is what the watcher sees after a
// write through the API, is not a file change and adds no revision.
func TestFileWatcherIgnoresItsOwnEcho(t *testing.T) {
	f := newGovFixture(t, govBasePolicy)
	if res := f.put(f.adminA.RawKey, govEdit("action: REDACT", "action: WARN")); res.status != http.StatusOK {
		t.Fatalf("%d %s", res.status, res.raw)
	}
	active := f.cfg.PolicyStore.GetPolicy()
	reloaded, err := policy.LoadFromFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	RecordPolicyFileChange(f.audit, f.hist, f.path, active, reloaded)
	if _, ok := f.lastAudit("policy.file_change"); ok {
		t.Error("an unchanged reload was recorded as a file change")
	}
	if revs, _ := f.hist.ListRevisions(); len(revs) != 1 {
		t.Errorf("history holds %d revisions, want the PUT's one", len(revs))
	}
}

// --- 3. dual control ---

func TestDirectWritesRefusedUnderDualControl(t *testing.T) {
	base := govBasePolicy + govDualControl
	cases := []struct {
		name string
		run  func(f *govFixture, key string) govResponse
	}{
		{"PUT", func(f *govFixture, key string) govResponse {
			return f.put(key, strings.Replace(base, "action: REDACT", "action: WARN", 1))
		}},
		{"PUT that turns dual control off", func(f *govFixture, key string) govResponse {
			return f.put(key, govBasePolicy)
		}},
		{"custom entity add", func(f *govFixture, key string) govResponse {
			return f.do(http.MethodPost, "/policies/custom-entities", key, `{"name":"BADGE","pattern":"BDG-[0-9]{4}"}`, nil)
		}},
		{"custom entity delete", func(f *govFixture, key string) govResponse {
			return f.do(http.MethodDelete, "/policies/custom-entities/EMPLOYEE_ID", key, "", nil)
		}},
		{"rollback", func(f *govFixture, key string) govResponse {
			rev, _ := f.hist.AppendRevision(history.Revision{Author: "seed", YAML: govBasePolicy})
			return f.do(http.MethodPost, "/policies/rollback/"+rev.ID, key, "", nil)
		}},
	}
	for _, tc := range cases {
		for _, who := range []string{"api key", "static admin key"} {
			t.Run(tc.name+"/"+who, func(t *testing.T) {
				f := newGovFixture(t, base)
				key := f.adminA.RawKey
				if who == "static admin key" {
					key = govAdminKey
				}
				res := tc.run(f, key)
				if res.status != http.StatusConflict || res.code() != codeDualControl {
					t.Fatalf("got %d %s, want 409 %s", res.status, res.raw, codeDualControl)
				}
				msg, _ := res.body["error"].(string)
				if !strings.Contains(msg, "proposal") {
					t.Errorf("the error does not point to proposals: %q", msg)
				}
				if got := strings.Contains(msg, "static admin key"); got != (who == "static admin key") {
					t.Errorf("mention of the static key = %v for %s: %q", got, who, msg)
				}
				if f.fileText() != base {
					t.Error("the policy file was written")
				}
				if !f.cfg.PolicyStore.GetPolicy().Governance.RequireDualControl {
					t.Error("dual control was turned off")
				}
			})
		}
	}
}

// Dual control is turned off by a proposal a second person approves, and by
// nothing less. With only the static admin key there is no second person.
func TestTurningDualControlOffNeedsASecondPerson(t *testing.T) {
	base := govBasePolicy + govDualControl
	cases := []struct {
		name         string
		author       func(f *govFixture) string
		approver     func(f *govFixture) string
		wantStatus   int
		wantCode     string
		wantInError  string
		wantDualCtrl bool
	}{
		{"author approves own proposal",
			func(f *govFixture) string { return f.adminA.RawKey }, func(f *govFixture) string { return f.adminA.RawKey },
			http.StatusForbidden, codeSelfApproval, "author", true},
		{"static admin key on both sides",
			func(*govFixture) string { return govAdminKey }, func(*govFixture) string { return govAdminKey },
			http.StatusForbidden, codeSelfApproval, "static admin key", true},
		{"static admin key approves a key's proposal",
			func(f *govFixture) string { return f.adminA.RawKey }, func(*govFixture) string { return govAdminKey },
			http.StatusForbidden, codeSelfApproval, "static admin key", true},
		{"a second admin approves",
			func(f *govFixture) string { return f.adminA.RawKey }, func(f *govFixture) string { return f.adminB.RawKey },
			http.StatusOK, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGovFixture(t, base)
			id := f.mustPropose(tc.author(f), govBasePolicy)
			// A spoofed actor header does not make the approver someone else.
			res := f.do(http.MethodPost, "/policies/proposals/"+id+"/approve", tc.approver(f), "", map[string]string{"X-Tamga-Actor": "second-person"})
			if res.status != tc.wantStatus || res.code() != tc.wantCode {
				t.Fatalf("approve: %d %s, want %d %q", res.status, res.raw, tc.wantStatus, tc.wantCode)
			}
			if msg, _ := res.body["error"].(string); !strings.Contains(msg, tc.wantInError) {
				t.Errorf("error %q does not mention %q", msg, tc.wantInError)
			}
			if got := f.cfg.dualControlOn(); got != tc.wantDualCtrl {
				t.Errorf("dual control on = %v, want %v", got, tc.wantDualCtrl)
			}
			prop, _ := f.hist.GetProposal(id)
			wantState := "open"
			if tc.wantStatus == http.StatusOK {
				wantState = "approved"
				if prop.ApprovedBy != "apikey:"+f.adminB.ID {
					t.Errorf("approved_by = %q", prop.ApprovedBy)
				}
			}
			if prop.Status != wantState {
				t.Errorf("proposal is %q, want %q", prop.Status, wantState)
			}
		})
	}
}

// Without dual control an author may approve their own proposal, as before.
func TestSelfApprovalAllowedWithoutDualControl(t *testing.T) {
	f := newGovFixture(t, govBasePolicy)
	id := f.mustPropose(govAdminKey, govEdit("action: REDACT", "action: WARN"))
	if res := f.approve(govAdminKey, id); res.status != http.StatusOK {
		t.Fatalf("%d %s", res.status, res.raw)
	}
}

func TestSettledProposalCannotBeSettledAgain(t *testing.T) {
	cases := []struct {
		name   string
		first  string
		second string
	}{
		{"approve then approve", "approve", "approve"},
		{"approve then reject", "approve", "reject"},
		{"reject then approve", "reject", "approve"},
		{"reject then reject", "reject", "reject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGovFixture(t, govBasePolicy)
			proposed := govEdit("action: REDACT", "action: WARN")
			id := f.mustPropose(f.adminA.RawKey, proposed)
			act := func(what string) govResponse {
				if what == "approve" {
					return f.approve(f.adminB.RawKey, id)
				}
				return f.reject(f.adminB.RawKey, id)
			}
			if res := act(tc.first); res.status != http.StatusOK {
				t.Fatalf("first %s: %d %s", tc.first, res.status, res.raw)
			}
			textAfterFirst := f.fileText()
			auditAfterFirst := len(f.audit.List(0))
			res := act(tc.second)
			if res.status != http.StatusConflict || res.code() != codeProposalClosed {
				t.Fatalf("second %s: %d %s, want 409 %s", tc.second, res.status, res.raw, codeProposalClosed)
			}
			wantState := map[string]string{"approve": "approved", "reject": "rejected"}[tc.first]
			if res.body["status"] != wantState {
				t.Errorf("status in body = %v, want %q", res.body["status"], wantState)
			}
			if prop, _ := f.hist.GetProposal(id); prop.Status != wantState {
				t.Errorf("proposal is %q, want %q", prop.Status, wantState)
			}
			if f.fileText() != textAfterFirst || len(f.audit.List(0)) != auditAfterFirst {
				t.Error("the refused call changed the file or the audit log")
			}
		})
	}
}

// An approval applies the stored text, judged against the policy and the
// rules of the moment of approval.
func TestProposalIsRevalidatedAtApproval(t *testing.T) {
	t.Run("policy changed since: stale", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		id := f.mustPropose(f.adminA.RawKey, govEdit("action: REDACT", "action: WARN"))
		time.Sleep(20 * time.Millisecond)
		inBetween := govEdit("allowed: [openai, anthropic]", "allowed: [openai]")
		if res := f.put(f.adminB.RawKey, inBetween); res.status != http.StatusOK {
			t.Fatalf("%d %s", res.status, res.raw)
		}
		res := f.approve(f.adminB.RawKey, id)
		if res.status != http.StatusConflict || res.code() != codeProposalStale {
			t.Fatalf("got %d %s, want 409 %s", res.status, res.raw, codeProposalStale)
		}
		if f.fileText() != inBetween {
			t.Error("the stale proposal overwrote the newer policy")
		}
		if prop, _ := f.hist.GetProposal(id); prop.Status != "open" {
			t.Errorf("proposal is %q, want open", prop.Status)
		}
		// It can still be rejected, and a fresh one goes through.
		if res := f.reject(f.adminA.RawKey, id); res.status != http.StatusOK {
			t.Errorf("reject of the stale proposal: %d %s", res.status, res.raw)
		}
		fresh := f.mustPropose(f.adminA.RawKey, strings.Replace(inBetween, "action: REDACT", "action: WARN", 1))
		if res := f.approve(f.adminB.RawKey, fresh); res.status != http.StatusOK {
			t.Errorf("fresh proposal: %d %s", res.status, res.raw)
		}
	})

	t.Run("stored text no longer loads: 422, nothing applied", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		// As if written when the bounds were looser: put into the store as is.
		prop, err := f.hist.CreateProposal(history.Proposal{Author: "apikey:" + f.adminA.ID, YAML: govBasePolicy + "rate_limit:\n  max_tokens_per_day: -5\n"})
		if err != nil {
			t.Fatal(err)
		}
		res := f.approve(f.adminB.RawKey, prop.ID)
		if res.status != http.StatusUnprocessableEntity || !strings.Contains(res.raw, "rate_limit.max_tokens_per_day") {
			t.Fatalf("got %d %s, want 422 naming the field", res.status, res.raw)
		}
		if f.fileText() != govBasePolicy {
			t.Error("the policy file was written")
		}
		if got, _ := f.hist.GetProposal(prop.ID); got.Status != "open" {
			t.Errorf("proposal is %q, want open", got.Status)
		}
	})

	t.Run("dual control turned on after the proposal was written", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		id := f.mustPropose(f.adminA.RawKey, govEdit("action: REDACT", "action: WARN"))
		// The file on disk gains dual control; no history revision, so the
		// proposal is not stale, but its author can no longer approve it.
		_ = os.WriteFile(f.path, []byte(govBasePolicy+govDualControl), 0o600)
		if err := f.cfg.PolicyStore.Reload(f.path); err != nil {
			t.Fatal(err)
		}
		if res := f.approve(f.adminA.RawKey, id); res.status != http.StatusForbidden || res.code() != codeSelfApproval {
			t.Fatalf("got %d %s, want 403 %s", res.status, res.raw, codeSelfApproval)
		}
	})

	t.Run("what is applied is the stored text", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		proposed := govEdit("action: REDACT", "action: WARN")
		id := f.mustPropose(f.adminA.RawKey, proposed)
		res := f.do(http.MethodPost, "/policies/proposals/"+id+"/approve", f.adminB.RawKey, `{"yaml":"name: other\n"}`, nil)
		if res.status != http.StatusOK {
			t.Fatalf("%d %s", res.status, res.raw)
		}
		if f.fileText() != proposed {
			t.Error("the file does not hold the proposal's text")
		}
		if got := stringsOf(res.body["changed"]); !reflect.DeepEqual(got, []string{"rules.pii_detection.action"}) {
			t.Errorf("changed = %v", got)
		}
	})
}

// A proposal is judged when it is written too.
func TestProposalCreateValidates(t *testing.T) {
	f := newGovFixture(t, govBasePolicy)
	cases := []struct {
		name string
		yaml string
		want int
	}{
		{"empty", "  ", http.StatusBadRequest},
		{"not YAML", "name: [unterminated\n", http.StatusBadRequest},
		{"breaks a bound", govBasePolicy + "rate_limit:\n  max_tokens_per_day: -5\n", http.StatusUnprocessableEntity},
		{"no name", "version: \"1.0\"\n", http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if res := f.propose(f.adminA.RawKey, tc.yaml); res.status != tc.want {
				t.Errorf("got %d %s, want %d", res.status, res.raw, tc.want)
			}
		})
	}
	if props, _ := f.hist.ListProposals(); len(props) != 0 {
		t.Errorf("%d refused proposals were stored", len(props))
	}
}

// --- 4. privilege split ---

func TestPrivilegeSplitOnPut(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []string // privileged sections; nil means an analyst may
	}{
		{"providers", govEdit("allowed: [openai, anthropic]", "allowed: [openai]"), []string{"providers"}},
		{"exceptions", govBasePolicy + "exceptions:\n  - rule: pii_detection\n    roles: [admin]\n    reason: test\n", []string{"exceptions"}},
		{"governance", govBasePolicy + govDualControl, []string{"governance"}},
		{"data", govBasePolicy + "data:\n  retention_days: 30\n", []string{"data"}},
		{"pricing", govBasePolicy + "pricing:\n  active_tier: team\n", []string{"pricing"}},
		{"vault", govBasePolicy + "vault:\n  enabled: true\n", []string{"vault"}},
		{"cache", govBasePolicy + "cache:\n  enabled: true\n", []string{"cache"}},
		{"body_limits", govBasePolicy + "body_limits:\n  default:\n    max_bytes: 4096\n", []string{"body_limits"}},
		{"rate_limit", govBasePolicy + "rate_limit:\n  max_requests_per_minute: 10\n", []string{"rate_limit"}},
		{"cost", govBasePolicy + "cost:\n  max_tokens_per_day: 1000\n", []string{"cost"}},
		{"scan", govBasePolicy + "scan:\n  on_error: block\n", []string{"scan"}},
		{"operator_state", govBasePolicy + "operator_state:\n  enabled: true\n", []string{"operator_state"}},
		{"canary", govBasePolicy + "canary:\n  enabled: true\n", []string{"canary"}},
		{"notify", govEdit("hook-SECRET", "hook-OTHER"), []string{"rules.pii_detection.notify"}},
		{"notify removed", govEdit("    notify:\n      - channel: slack\n        webhook: https://example.invalid/hook-SECRET\n", ""), []string{"rules.pii_detection.notify"}},
		{"two at once", govEdit("hook-SECRET", "hook-OTHER") + "vault:\n  enabled: true\n", []string{"rules.pii_detection.notify", "vault"}},

		{"rules", govEdit("action: REDACT", "action: WARN"), nil},
		{"output_rules", govBasePolicy + "output_rules:\n  enabled: true\n  buffer_bytes: 65536\n  block_on: [secret]\n", nil},
		{"custom_entities", govEdit("severity: high", "severity: low"), nil},
		{"competitors", govBasePolicy + "competitors:\n  - name: Acme\n    patterns: ['acme']\n    severity: low\n    action: log\n    enabled: true\n", nil},
		{"name and version", strings.Replace(govEdit("name: base", "name: renamed"), `"1.0"`, `"1.1"`, 1), nil},
		{"same policy, keys reordered and commented", `# reordered
custom_entities:
  - {severity: high, action: REDACT, pattern: 'EMP-[0-9]{6}', name: EMPLOYEE_ID}
providers: {allowed: ["openai", "anthropic"]}   # untouched
rules:
  secret_detection: {action: BLOCK}
  pii_detection:
    notify: [{webhook: "https://example.invalid/hook-SECRET", channel: slack}]
    sensitivity: medium
    action: REDACT
name: base
version: '1.0'
`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The analyst.
			f := newGovFixture(t, govBasePolicy)
			res := f.put(f.analystA.RawKey, tc.yaml)
			if tc.want == nil {
				if res.status != http.StatusOK {
					t.Fatalf("analyst: %d %s, want 200", res.status, res.raw)
				}
				if f.fileText() != tc.yaml {
					t.Error("analyst: the file was not written")
				}
			} else {
				if res.status != http.StatusForbidden || res.code() != codeAdminRequired {
					t.Fatalf("analyst: %d %s, want 403 %s", res.status, res.raw, codeAdminRequired)
				}
				if got := stringsOf(res.body["privileged_sections"]); !reflect.DeepEqual(got, tc.want) {
					t.Errorf("privileged_sections = %v, want %v", got, tc.want)
				}
				if msg, _ := res.body["error"].(string); msg == "" || strings.Contains(res.raw, "hook-") {
					t.Errorf("error body: %s", res.raw)
				}
				if f.fileText() != govBasePolicy {
					t.Error("analyst: the file was written")
				}
				if _, ok := f.lastAudit("policy.put"); ok {
					t.Error("analyst: a refused write was audited as a write")
				}
				// The same refusal as a proposal, so it cannot be parked
				// for someone to wave through.
				if prop := f.propose(f.analystA.RawKey, tc.yaml); prop.status != http.StatusForbidden ||
					!reflect.DeepEqual(stringsOf(prop.body["privileged_sections"]), tc.want) {
					t.Errorf("analyst proposal: %d %s", prop.status, prop.raw)
				}
			}
			// The admin, on a fresh fixture: always allowed.
			f = newGovFixture(t, govBasePolicy)
			if res := f.put(f.adminA.RawKey, tc.yaml); res.status != http.StatusOK {
				t.Errorf("admin: %d %s, want 200", res.status, res.raw)
			}
		})
	}
}

func TestPrivilegeSplitOnProposals(t *testing.T) {
	privileged := govEdit("allowed: [openai, anthropic]", "allowed: [openai]")
	open := govEdit("action: REDACT", "action: WARN")

	t.Run("analyst cannot approve an admin's privileged proposal", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		id := f.mustPropose(f.adminA.RawKey, privileged)
		res := f.approve(f.analystA.RawKey, id)
		if res.status != http.StatusForbidden || !reflect.DeepEqual(stringsOf(res.body["privileged_sections"]), []string{"providers"}) {
			t.Fatalf("approve: %d %s", res.status, res.raw)
		}
		if f.fileText() != govBasePolicy {
			t.Error("the file was written")
		}
		// Nor turn it down.
		if res := f.reject(f.analystA.RawKey, id); res.status != http.StatusForbidden {
			t.Errorf("reject: %d %s, want 403", res.status, res.raw)
		}
		if prop, _ := f.hist.GetProposal(id); prop.Status != "open" {
			t.Errorf("proposal is %q, want open", prop.Status)
		}
		if res := f.approve(f.adminB.RawKey, id); res.status != http.StatusOK {
			t.Errorf("admin approve: %d %s", res.status, res.raw)
		}
	})

	t.Run("analysts propose and approve what is open to them", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy+govDualControl)
		proposed := strings.Replace(govBasePolicy+govDualControl, "action: REDACT", "action: WARN", 1)
		id := f.mustPropose(f.analystA.RawKey, proposed)
		if res := f.approve(f.analystA.RawKey, id); res.status != http.StatusForbidden || res.code() != codeSelfApproval {
			t.Fatalf("own proposal under dual control: %d %s", res.status, res.raw)
		}
		if res := f.approve(f.analystB.RawKey, id); res.status != http.StatusOK {
			t.Fatalf("second analyst: %d %s", res.status, res.raw)
		}
		if f.fileText() != proposed {
			t.Error("the proposal was not applied")
		}
	})

	t.Run("analyst withdraws own proposal, lists proposals", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		id := f.mustPropose(f.analystA.RawKey, open)
		if res := f.do(http.MethodGet, "/policies/proposals", f.analystB.RawKey, "", nil); res.status != http.StatusOK {
			t.Errorf("list: %d %s", res.status, res.raw)
		}
		if res := f.reject(f.analystA.RawKey, id); res.status != http.StatusOK {
			t.Errorf("withdraw: %d %s", res.status, res.raw)
		}
	})

	t.Run("rollback stays with the admin role", func(t *testing.T) {
		f := newGovFixture(t, govBasePolicy)
		rev, _ := f.hist.AppendRevision(history.Revision{Author: "seed", YAML: open})
		if res := f.do(http.MethodPost, "/policies/rollback/"+rev.ID, f.analystA.RawKey, "", nil); res.status != http.StatusForbidden {
			t.Errorf("analyst rollback: %d %s, want 403", res.status, res.raw)
		}
	})
}

// The custom-entity endpoints stay at the analyst's level, with the rest of
// the policy carried through untouched.
func TestCustomEntityEndpointsStayAnalystLevel(t *testing.T) {
	full := govBasePolicy + "vault:\n  enabled: true\ncache:\n  enabled: true\n  ttl_seconds: 60\nscan:\n  on_error: block\n"
	f := newGovFixture(t, full)
	res := f.do(http.MethodPost, "/policies/custom-entities", f.analystA.RawKey, `{"name":"BADGE","pattern":"BDG-[0-9]{4}","severity":"low","action":"LOG"}`, nil)
	if res.status != http.StatusCreated {
		t.Fatalf("add: %d %s", res.status, res.raw)
	}
	res = f.do(http.MethodDelete, "/policies/custom-entities/EMPLOYEE_ID", f.analystA.RawKey, "", nil)
	if res.status != http.StatusOK {
		t.Fatalf("delete: %d %s", res.status, res.raw)
	}
	before, _ := policy.LoadFromBytes([]byte(full))
	if d := policy.PrivilegedChanges(before, f.cfg.PolicyStore.GetPolicy()); len(d) != 0 {
		t.Errorf("the entity endpoints changed privileged sections: %v", d)
	}
	// A read-scope key still cannot.
	keys := f.cfg.APIKeys
	viewer, _ := keys.Create("viewer", "read")
	if res := f.do(http.MethodPost, "/policies/custom-entities", viewer.RawKey, `{"name":"X1","pattern":"x"}`, nil); res.status != http.StatusForbidden {
		t.Errorf("viewer add: %d, want 403", res.status)
	}
}

// --- 5. validation errors ---

func TestBoundErrorsAre422WithIssues(t *testing.T) {
	type issue struct{ Field, Rule, Message, Severity string }
	decode := func(t *testing.T, raw string) []issue {
		t.Helper()
		var body struct {
			Error struct {
				Code    string  `json:"code"`
				Message string  `json:"message"`
				Details []issue `json:"details"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			t.Fatalf("body is not the validation shape: %v: %s", err, raw)
		}
		if body.Error.Code != "validation_failed" || body.Error.Message == "" {
			t.Errorf("error = %+v", body.Error)
		}
		return body.Error.Details
	}
	cases := []struct {
		name       string
		yaml       string
		wantStatus int
		wantFields []string
	}{
		{"one bound", govBasePolicy + "rate_limit:\n  max_tokens_per_day: -5\n", http.StatusUnprocessableEntity, []string{"rate_limit.max_tokens_per_day"}},
		{"two bounds, both reported",
			govEdit("sensitivity: medium", "sensitivity: medium\n    minimum_confidence: 500") + "rate_limit:\n  max_tokens_per_day: -5\n",
			http.StatusUnprocessableEntity, []string{"rules.pii_detection.minimum_confidence", "rate_limit.max_tokens_per_day"}},
		{"semantic error keeps its shape", "version: \"1.0\"\n", http.StatusUnprocessableEntity, []string{"name"}},
		{"syntax error", "name: [unterminated\n", http.StatusBadRequest, nil},
		{"unknown key", govBasePolicy + "no_such_section: 1\n", http.StatusBadRequest, nil},
		{"more than one document", govBasePolicy + "---\nname: second\n", http.StatusBadRequest, nil},
	}
	for _, tc := range cases {
		for _, route := range []struct{ method, path string }{{http.MethodPut, "/policies"}, {http.MethodPost, "/policies/validate"}} {
			t.Run(tc.name+" "+route.path, func(t *testing.T) {
				f := newGovFixture(t, govBasePolicy)
				res := f.do(route.method, route.path, f.adminA.RawKey, tc.yaml, nil)
				if res.status != tc.wantStatus {
					t.Fatalf("got %d %s, want %d", res.status, res.raw, tc.wantStatus)
				}
				if f.fileText() != govBasePolicy {
					t.Error("the policy file was written")
				}
				if tc.wantStatus == http.StatusBadRequest {
					if msg, _ := res.body["error"].(string); !strings.HasPrefix(msg, "policy parse: ") {
						t.Errorf("400 body = %s", res.raw)
					}
					return
				}
				details := decode(t, res.raw)
				got := map[string]issue{}
				for _, d := range details {
					got[d.Field] = d
				}
				for _, field := range tc.wantFields {
					d, ok := got[field]
					if !ok {
						t.Errorf("no issue for %s in %s", field, res.raw)
						continue
					}
					if d.Message == "" || d.Rule == "" || d.Severity != "error" {
						t.Errorf("issue for %s is incomplete: %+v", field, d)
					}
				}
			})
		}
	}
}
