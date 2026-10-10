package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/auth"
	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/users"
)

const (
	refusalAdminKey = "refusal-test-admin-key-0123456789"
	refusalSecret   = "refusal-test-signing-secret-0123456789abcdef"
)

// refusalFixture serves the real route table, handlers replaced by stubs,
// with a refusal log whose window is closed by the test and not by a clock.
type refusalFixture struct {
	t     *testing.T
	h     http.Handler
	log   *refusalLog
	audit *incidents.AuditRing
	users *users.MemoryStore
	keys  *apikeys.MemoryStore

	mu     sync.Mutex
	timers []func()
}

func newRefusalFixture(t *testing.T, change func(*Config)) *refusalFixture {
	t.Helper()
	f := &refusalFixture{
		t: t, audit: incidents.NewAuditRing(4096),
		users: users.NewMemoryStore(), keys: apikeys.NewMemoryStore(),
	}
	cfg := Config{AdminKey: refusalAdminKey, JWTSecret: refusalSecret, Audit: f.audit, Users: f.users, APIKeys: f.keys}
	if change != nil {
		change(&cfg)
	}
	f.log = newRefusalLog(cfg.Audit, cfg.ClientAddr)
	f.log.after = func(_ time.Duration, fn func()) {
		f.mu.Lock()
		f.timers = append(f.timers, fn)
		f.mu.Unlock()
	}
	cfg.refusals = f.log

	// The real handlers for the public routes, which ask adminAuth on
	// their own; a stub for every other.
	table := routes(&cfg)
	for i := range table {
		if table[i].public == "" || table[i].handler == nil {
			table[i].handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", newHandler(cfg, table))
	f.h = mux
	return f
}

// endWindows runs every pending window end, as the clock would a minute on.
func (f *refusalFixture) endWindows() {
	f.mu.Lock()
	timers := f.timers
	f.timers = nil
	f.mu.Unlock()
	for _, fn := range timers {
		fn()
	}
}

func (f *refusalFixture) send(method, path, from string, headers map[string]string) int {
	f.t.Helper()
	req := httptest.NewRequest(method, "/api/v1"+path, nil)
	req.RemoteAddr = from + ":40000"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec.Code
}

// entries returns the refusal entries written so far, oldest first.
func (f *refusalFixture) entries() []incidents.AuditEntry {
	f.log.pending.Wait()
	all := f.audit.List(0)
	var out []incidents.AuditEntry
	for i := len(all) - 1; i >= 0; i-- {
		if strings.HasPrefix(all[i].Kind, auditRefused) {
			out = append(out, all[i])
		}
	}
	return out
}

func (f *refusalFixture) token(sub, role string, exp int64) string {
	f.t.Helper()
	tok, err := auth.CreateToken(auth.Claims{Sub: sub, Role: role, Iat: time.Now().Unix() + 1, Exp: exp}, []byte(refusalSecret))
	if err != nil {
		f.t.Fatal(err)
	}
	return tok
}

// holdsNoPartOf fails when an entry holds four characters in a row of the
// credential that was presented.
func holdsNoPartOf(t *testing.T, e incidents.AuditEntry, credential string) {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+4 <= len(credential); i++ {
		if strings.Contains(string(raw), credential[i:i+4]) {
			t.Fatalf("the entry holds %q of the credential: %s", credential[i:i+4], raw)
		}
	}
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func adminKeyHeader(key string) map[string]string {
	return map[string]string{"X-Tamga-Admin-Key": key}
}

func TestRefusalAudit_WrongKey(t *testing.T) {
	f := newRefusalFixture(t, nil)
	wrong := "Zq9-wrong-KEY-value-7731-xyzzy"
	if got := f.send("GET", "/stats?range=7d&secret=Qm4Xw8", "203.0.113.7", adminKeyHeader(wrong)); got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got)
	}
	es := f.entries()
	if len(es) != 1 {
		t.Fatalf("%d entries, want 1: %+v", len(es), es)
	}
	e := es[0]
	if e.Kind != auditRefused || e.Target != "GET /stats" || e.Actor != actorUnknown {
		t.Errorf("entry = %+v", e)
	}
	if e.Detail["reason"] != refusedInvalidKey || e.Detail["source"] != "203.0.113.7" || e.Detail["status"] != http.StatusUnauthorized {
		t.Errorf("detail = %v", e.Detail)
	}
	holdsNoPartOf(t, e, wrong)
	holdsNoPartOf(t, e, "Qm4Xw8") // nothing of the query string either
	if ok, at := f.audit.Verify(); !ok {
		t.Errorf("audit chain broken at %d", at)
	}
}

// A request that presents nothing was refused no credential, and every
// unauthenticated visit of the API is one: it leaves no entry.
func TestRefusalAudit_NoCredentialIsNotRecorded(t *testing.T) {
	f := newRefusalFixture(t, nil)
	if got := f.send("GET", "/stats", "203.0.113.7", nil); got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got)
	}
	if es := f.entries(); len(es) != 0 {
		t.Fatalf("entries: %+v", es)
	}
}

func TestRefusalAudit_Tokens(t *testing.T) {
	f := newRefusalFixture(t, nil)
	if _, err := f.users.Assign("u-viewer", "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.Assign("u-revoked", "admin"); err != nil {
		t.Fatal(err)
	}
	foreign, err := auth.CreateToken(auth.Claims{Sub: "u-viewer", Role: "admin"}, []byte("some-other-secret-0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	revoked := f.token("u-revoked", "admin", 0)
	if _, err := f.users.RevokeSessions("u-revoked"); err != nil {
		t.Fatal(err)
	}
	// Dated as authzToken dates one; a revocation in the same second must
	// still be after it.
	for i := 0; i < 3; i++ {
		if _, err := f.users.RevokeSessions("u-revoked"); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name, token string
		method      string
		path        string
		status      int
		reason      string
		actor       string
		target      string
	}{
		{"not a token", "garbage.value.Xq7Zp2LkWm", "GET", "/stats", 401, refusedInvalidToken, actorUnknown, "GET /stats"},
		{"another secret", foreign, "GET", "/stats", 401, refusedInvalidToken, actorUnknown, "GET /stats"},
		{"expired", f.token("u-viewer", "viewer", time.Now().Add(-time.Hour).Unix()), "GET", "/stats", 401, refusedTokenExpired, actorUnknown, "GET /stats"},
		{"no role", f.token("u-nobody", "admin", 0), "GET", "/stats", 403, codeAccountNotAssigned, "user:u-nobody", "GET /stats"},
		{"permission", f.token("u-viewer", "viewer", 0), "POST", "/maintenance/retention", 403, codePermissionDenied, "user:u-viewer", "POST /maintenance/retention"},
	}
	for i, c := range cases {
		from := fmt.Sprintf("198.51.100.%d", i+1)
		if got := f.send(c.method, c.path, from, bearer(c.token)); got != c.status {
			t.Fatalf("%s: status = %d, want %d", c.name, got, c.status)
		}
		es := f.entries()
		if len(es) != i+1 {
			t.Fatalf("%s: %d entries, want %d", c.name, len(es), i+1)
		}
		e := es[i]
		if e.Detail["reason"] != c.reason || e.Actor != c.actor || e.Target != c.target || e.Detail["source"] != from || e.Detail["status"] != c.status {
			t.Errorf("%s: entry = %+v", c.name, e)
		}
		// The signature is the part of a token that is a credential.
		holdsNoPartOf(t, e, c.token[strings.LastIndex(c.token, ".")+1:])
	}
	last := f.entries()[len(cases)-1]
	if last.Detail["role"] != "viewer" || last.Detail["required_permission"] != scopeMaintenance {
		t.Errorf("permission denial detail = %v", last.Detail)
	}

	// A revoked session.
	before := len(f.entries())
	status := f.send("GET", "/stats", "198.51.100.99", bearer(revoked))
	es := f.entries()
	if status != http.StatusUnauthorized || len(es) != before+1 {
		t.Fatalf("revoked: status %d, %d new entries", status, len(es)-before)
	}
	if e := es[before]; e.Detail["reason"] != codeSessionRevoked || e.Actor != "user:u-revoked" {
		t.Errorf("revoked: entry = %+v", e)
	}

	// The same people, let in, leave nothing.
	before = len(f.entries())
	if got := f.send("GET", "/stats", "198.51.100.50", bearer(f.token("u-viewer", "viewer", 0))); got != http.StatusNoContent {
		t.Fatalf("viewer on a viewer route: status %d", got)
	}
	if got := f.send("POST", "/maintenance/retention", "198.51.100.51", adminKeyHeader(refusalAdminKey)); got != http.StatusNoContent {
		t.Fatalf("admin key: status %d", got)
	}
	if n := len(f.entries()); n != before {
		t.Fatalf("%d entries for requests that were let in", n-before)
	}
}

func TestRefusalAudit_ProxyKey(t *testing.T) {
	f := newRefusalFixture(t, nil)
	ck, err := f.keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.send("GET", "/stats", "203.0.113.9", adminKeyHeader(ck.RawKey)); got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
	es := f.entries()
	if len(es) != 1 || es[0].Detail["reason"] != refusedProxyKey || es[0].Actor != actorAPIKeyPrefix+ck.ID {
		t.Fatalf("entries = %+v", es)
	}
	// The actor is the id of the key's record, as in every other entry the
	// key leaves. The store makes that id from the first six bytes of the
	// key; nothing of the 18 bytes that are secret is in the entry.
	holdsNoPartOf(t, es[0], ck.RawKey[len("tk_")+len(ck.ID):])
}

// One source that keeps trying adds two entries per window, not one per
// attempt: the first, and the count of the rest.
func TestRefusalAudit_RepeatsAreCounted(t *testing.T) {
	f := newRefusalFixture(t, nil)
	for i := 0; i < 500; i++ {
		f.send("GET", fmt.Sprintf("/stats/%d", i), "203.0.113.7", adminKeyHeader(fmt.Sprintf("guess-%04d-abcdefgh", i)))
	}
	if es := f.entries(); len(es) != 1 {
		t.Fatalf("%d entries during the window, want 1", len(es))
	}
	f.endWindows()
	es := f.entries()
	if len(es) != 2 {
		t.Fatalf("%d entries after the window, want 2", len(es))
	}
	sum := es[1]
	if sum.Kind != auditRefusedRepeated || sum.Detail["repeats"] != 499 || sum.Detail["source"] != "203.0.113.7" ||
		sum.Detail["reason"] != refusedInvalidKey || sum.Detail["window_seconds"] != 60 {
		t.Errorf("summary = %+v", sum)
	}

	// A new window starts with a new first entry, and one that had no
	// repeats ends without a second.
	f.send("GET", "/stats", "203.0.113.7", adminKeyHeader("guess-again-abcdefgh"))
	f.endWindows()
	if es := f.entries(); len(es) != 3 || es[2].Kind != auditRefused {
		t.Fatalf("entries after a second window: %+v", es)
	}
	f.log.mu.Lock()
	open := len(f.log.open)
	f.log.mu.Unlock()
	if open != 0 {
		t.Errorf("%d runs still open after their windows ended", open)
	}
}

// Many sources cannot fill the log either: only so many are followed at a
// time, and the rest are one count.
func TestRefusalAudit_SourcesAreBounded(t *testing.T) {
	f := newRefusalFixture(t, nil)
	const sources = 1000
	for i := 0; i < sources; i++ {
		f.send("GET", "/stats", fmt.Sprintf("10.%d.%d.1", i/250, i%250), adminKeyHeader("guess-abcdefgh"))
	}
	if es := f.entries(); len(es) != refusalSources {
		t.Fatalf("%d entries during the window, want %d", len(es), refusalSources)
	}
	f.log.mu.Lock()
	open := len(f.log.open)
	f.log.mu.Unlock()
	if open != refusalSources+1 {
		t.Fatalf("%d runs open, want %d", open, refusalSources+1)
	}
	f.endWindows()
	es := f.entries()
	if len(es) != refusalSources+1 {
		t.Fatalf("%d entries after the window, want %d", len(es), refusalSources+1)
	}
	rest := es[len(es)-1]
	if rest.Kind != auditRefusedRepeated || rest.Detail["repeats"] != sources-refusalSources || rest.Detail["source"] != nil {
		t.Errorf("entry for the sources not followed = %+v", rest)
	}
}

// The answer does not wait for the audit log's store.
func TestRefusalAudit_DoesNotWaitForTheStore(t *testing.T) {
	f := newRefusalFixture(t, nil)
	release := make(chan struct{})
	f.audit.SetPersister(func(incidents.AuditEntry) { <-release })

	done := make(chan int, 1)
	go func() { done <- f.send("GET", "/stats", "203.0.113.7", adminKeyHeader("guess-abcdefgh")) }()
	select {
	case status := <-done:
		if status != http.StatusUnauthorized {
			t.Fatalf("status = %d", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the refusal waited for the audit store")
	}
	// More refusals while the store is stuck are answered too.
	for i := 0; i < 50; i++ {
		f.send("GET", "/stats", fmt.Sprintf("203.0.113.%d", 10+i), adminKeyHeader("guess-abcdefgh"))
	}
	close(release)
	if es := f.entries(); len(es) != refusalSources {
		t.Fatalf("%d entries once the store answered, want %d", len(es), refusalSources)
	}
	// The goroutine that appends is gone once there is nothing to append.
	for i := 0; ; i++ {
		f.log.mu.Lock()
		draining := f.log.draining
		f.log.mu.Unlock()
		if !draining {
			break
		}
		if i == 200 {
			t.Fatal("the appending goroutine is still running with an empty queue")
		}
		time.Sleep(time.Millisecond)
	}
}

// A stuck store and a flood together: the queue is bounded and the refusals
// beyond it are dropped, not kept in memory.
func TestRefusalAudit_QueueIsBounded(t *testing.T) {
	audit := incidents.NewAuditRing(1024)
	release := make(chan struct{})
	audit.SetPersister(func(incidents.AuditEntry) { <-release })
	l := newRefusalLog(audit, nil)
	l.after = func(time.Duration, func()) {}
	l.limit = 10 * refusalQueue

	for i := 0; i < 3*refusalQueue; i++ {
		req := httptest.NewRequest("GET", "/stats", nil)
		req.RemoteAddr = fmt.Sprintf("10.%d.%d.1:1", i/250, i%250)
		l.note(req, refusal{reason: refusedInvalidKey, status: http.StatusUnauthorized})
	}
	l.mu.Lock()
	queued := len(l.queue)
	l.mu.Unlock()
	if queued > refusalQueue {
		t.Fatalf("%d entries queued, the bound is %d", queued, refusalQueue)
	}
	close(release)
	l.pending.Wait()
	// One entry was already with the store when the queue filled.
	if n := len(audit.List(0)); n > refusalQueue+1 {
		t.Fatalf("%d entries written, the bound is %d", n, refusalQueue+1)
	}
}

// The health routes ask adminAuth whether to show the full body. A stale
// key on an uptime check is answered with the public body and is not a
// refusal.
func TestRefusalAudit_HealthProbeIsNotRecorded(t *testing.T) {
	f := newRefusalFixture(t, nil)
	if got := f.send("GET", "/health/detailed", "203.0.113.7", adminKeyHeader("stale-key-abcdefgh")); got == http.StatusUnauthorized || got == http.StatusForbidden {
		t.Fatalf("health status = %d", got)
	}
	if es := f.entries(); len(es) != 0 {
		t.Fatalf("entries: %+v", es)
	}
}

// The source is the address the deployment's trusted-proxy rules give.
func TestRefusalAudit_SourceFromClientAddr(t *testing.T) {
	f := newRefusalFixture(t, func(cfg *Config) {
		cfg.ClientAddr = func(*http.Request) string { return "192.0.2.44" }
	})
	f.send("GET", "/stats", "10.0.0.1", adminKeyHeader("guess-abcdefgh"))
	if es := f.entries(); len(es) != 1 || es[0].Detail["source"] != "192.0.2.44" {
		t.Fatalf("entries = %+v", es)
	}
}

// What the sender writes into the path is bounded and made printable.
func TestRefusalAudit_TargetIsBounded(t *testing.T) {
	f := newRefusalFixture(t, nil)
	f.send("GET", "/stats/"+strings.Repeat("a", 5000), "203.0.113.7", adminKeyHeader("guess-abcdefgh"))
	es := f.entries()
	if len(es) != 1 || len(es[0].Target) > refusalTargetMax+len("GET ") {
		t.Fatalf("entries = %+v", es)
	}
	if got := clipTarget("/a\x00b\ncé"); got != "/a?b?c?" {
		t.Errorf("clipTarget = %q", got)
	}
}

// The log's own decision and the guard's are the same one.
func TestRefusalAudit_AgreesWithTheGuard(t *testing.T) {
	f := newRefusalFixture(t, nil)
	for _, role := range []string{"viewer", "analyst", "admin"} {
		if _, err := f.users.Assign("u-"+role, role); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, rt := range fullTable() {
		if rt.public != "" || strings.Contains(rt.path, "{") {
			continue
		}
		for _, role := range []string{"viewer", "analyst", "admin"} {
			n++
			from := fmt.Sprintf("10.9.%d.%d", n/250, n%250)
			before := len(f.entries())
			status := f.send(rt.method, rt.path, from, bearer(f.token("u-"+role, role, 0)))
			f.endWindows()
			recorded := len(f.entries()) - before
			if (status == http.StatusForbidden) != (recorded == 1) {
				t.Errorf("%s as %s: status %d, %d entries", rt.pattern(), role, status, recorded)
			}
		}
	}
	if n < 60 {
		t.Fatalf("only %d cases; the table is not being walked", n)
	}
}
