package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/users"
)

// subjectStore stands in for the request log of the two subject routes.
type subjectStore struct {
	*store.NoopStore
	rows    int
	deleted int
	err     error
	calls   int
}

func (s *subjectStore) SubjectAccess(_ context.Context, _, _, _, _ string, _ int) ([]store.RequestLogRow, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]store.RequestLogRow, s.rows)
	for i := range out {
		out[i].RequestID = "req"
	}
	return out, nil
}

func (s *subjectStore) EraseSubject(_ context.Context, _, _, _, _ string) (int, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	return s.deleted, nil
}

const (
	subjUserID = "user_2zQ9privateSubject"
	subjEmail  = "ayse.yilmaz@example.com"
	subjTCKN   = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

type subjectFixture struct {
	t     *testing.T
	h     http.Handler
	audit *incidents.AuditRing
	store *subjectStore
}

func newSubjectFixture(t *testing.T, st *subjectStore) *subjectFixture {
	t.Helper()
	f := &subjectFixture{t: t, audit: incidents.NewAuditRing(64), store: st}
	cfg := Config{AdminKey: "subject-admin-key", Audit: f.audit, DefaultOrgID: histOrg}
	if st != nil {
		cfg.Store = st
	}
	f.h = testMux(cfg)
	return f
}

func (f *subjectFixture) do(method, pathAndQuery, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, "/api/v1"+pathAndQuery, strings.NewReader(body))
	req.Header.Set("X-Tamga-Admin-Key", "subject-admin-key")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

// entries returns the audit entries of a kind, oldest first, and fails when
// any entry of the log carries a selector's value.
func (f *subjectFixture) entries(kind string) []incidents.AuditEntry {
	f.t.Helper()
	all := f.audit.List(64)
	raw, err := json.Marshal(all)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, secret := range []string{subjUserID, subjEmail, "ayse", subjTCKN} {
		if strings.Contains(string(raw), secret) {
			f.t.Fatalf("the audit log holds the selector %q: %s", secret, raw)
		}
	}
	var out []incidents.AuditEntry
	for _, e := range all {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	// List is newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func wantSubjectEntry(t *testing.T, e incidents.AuditEntry, actor, selector, countKey string, count int, result string) {
	t.Helper()
	if e.Actor != actor {
		t.Errorf("actor = %q, want %q", e.Actor, actor)
	}
	if e.Target != "" {
		t.Errorf("target = %q: the target of a subject request is a person and is not written", e.Target)
	}
	if got := e.Detail["selector"]; got != selector {
		t.Errorf("selector = %v, want %q", got, selector)
	}
	if got := e.Detail[countKey]; got != count {
		t.Errorf("%s = %v, want %d", countKey, got, count)
	}
	if got := e.Detail["result"]; got != result {
		t.Errorf("result = %v, want %q", got, result)
	}
	if len(e.Detail) != 3 {
		t.Errorf("detail = %v, want the selector kind, the count and the result only", e.Detail)
	}
}

// Reading what is stored about a person leaves an entry: who, with which
// kind of selector, how many rows. Never the selector.
func TestSubjectAccessIsAudited(t *testing.T) {
	cases := []struct {
		name, method, query, body, selector string
	}{
		{"GET by user id", "GET", "?user_id=" + subjUserID, "", "user_id"},
		{"GET by e-mail", "GET", "?email=" + url.QueryEscape(subjEmail), "", "email"},
		{"GET by TCKN hash", "GET", "?tckn_hash=" + subjTCKN, "", "tckn_hash"},
		{"POST by user id", "POST", "", `{"user_id":"` + subjUserID + `"}`, "user_id"},
		{"POST by e-mail", "POST", "", `{"email":"` + subjEmail + `"}`, "email"},
		{"POST by TCKN hash", "POST", "", `{"tckn_hash":"` + subjTCKN + `"}`, "tckn_hash"},
		// The store answers by the first selector that is given.
		{"POST with two", "POST", "", `{"email":"` + subjEmail + `","user_id":"` + subjUserID + `"}`, "user_id"},
	}
	for _, c := range cases {
		f := newSubjectFixture(t, &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 3})
		rec := f.do(c.method, "/events/subject"+c.query, c.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", c.name, rec.Code, rec.Body.String())
		}
		got := f.entries(auditSubjectAccess)
		if len(got) != 1 {
			t.Fatalf("%s: %d audit entries, want 1", c.name, len(got))
		}
		wantSubjectEntry(t, got[0], ActorAdminKey, c.selector, "rows", 3, subjectOK)
	}
}

func TestSubjectAccessIsAuditedWhenItFails(t *testing.T) {
	f := newSubjectFixture(t, &subjectStore{NoopStore: store.NewNoopStoreSilent(), err: errors.New("pq: connection to 10.0.0.5 refused")})
	rec := f.do("POST", "/events/subject", `{"email":"`+subjEmail+`"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("the body gives away the database error: %s", rec.Body.String())
	}
	got := f.entries(auditSubjectAccess)
	if len(got) != 1 {
		t.Fatalf("%d audit entries, want 1", len(got))
	}
	wantSubjectEntry(t, got[0], ActorAdminKey, "email", "rows", 0, subjectError)

	// Without a store that keeps request logs the answer is empty, and the
	// attempt is still on the trail.
	f = newSubjectFixture(t, nil)
	if rec := f.do("GET", "/events/subject?user_id="+subjUserID, ""); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	got = f.entries(auditSubjectAccess)
	if len(got) != 1 {
		t.Fatalf("%d audit entries, want 1", len(got))
	}
	wantSubjectEntry(t, got[0], ActorAdminKey, "user_id", "rows", 0, subjectStoreUnavailable)
}

// The erase entry says what the access entry says, and no more: it used to
// keep the user id, e-mail address and TCKN hash of the person erased.
func TestSubjectEraseAuditHoldsNoSelector(t *testing.T) {
	for selector, body := range map[string]string{
		"user_id":   `{"user_id":"` + subjUserID + `"}`,
		"email":     `{"email":"` + subjEmail + `"}`,
		"tckn_hash": `{"tckn_hash":"` + subjTCKN + `"}`,
	} {
		f := newSubjectFixture(t, &subjectStore{NoopStore: store.NewNoopStoreSilent(), deleted: 7})
		rec := f.do("DELETE", "/events/subject", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", selector, rec.Code, rec.Body.String())
		}
		got := f.entries(auditSubjectErase)
		if len(got) != 1 {
			t.Fatalf("%s: %d audit entries, want 1", selector, len(got))
		}
		wantSubjectEntry(t, got[0], ActorAdminKey, selector, "deleted", 7, subjectOK)
	}

	f := newSubjectFixture(t, &subjectStore{NoopStore: store.NewNoopStoreSilent(), err: errors.New("database is down")})
	if rec := f.do("DELETE", "/events/subject", `{"email":"`+subjEmail+`"}`); rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	got := f.entries(auditSubjectErase)
	if len(got) != 1 {
		t.Fatalf("%d audit entries for a failed erase, want 1", len(got))
	}
	wantSubjectEntry(t, got[0], ActorAdminKey, "email", "deleted", 0, subjectError)
}

// A request that names nobody, or is refused, reads nothing and leaves no
// entry. A TCKN hash that is not one is refused before the store is asked:
// the store matches it as a substring, and "%" would match every row.
func TestSubjectRequestsThatAreRefused(t *testing.T) {
	st := &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 3, deleted: 3}
	f := newSubjectFixture(t, st)
	for _, c := range []struct{ method, query, body string }{
		{"GET", "", ""},
		{"POST", "", `{}`},
		{"POST", "", `not json`},
		{"DELETE", "", `{}`},
		{"GET", "?tckn_hash=%25", ""},
		{"GET", "?tckn_hash=a", ""},
		{"POST", "", `{"tckn_hash":"%"}`},
		{"DELETE", "", `{"tckn_hash":"%"}`},
		{"DELETE", "", `{"tckn_hash":"e"}`},
		{"DELETE", "", `{"tckn_hash":"` + strings.ToUpper(subjTCKN) + `"}`},
		{"DELETE", "", `{"tckn_hash":"` + subjTCKN[:63] + `_"}`},
	} {
		if rec := f.do(c.method, "/events/subject"+c.query, c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: %d, want 400", c.method, c.query, c.body, rec.Code)
		}
	}
	if st.calls != 0 {
		t.Errorf("the store was asked %d times by requests that are refused", st.calls)
	}
	if n := len(f.audit.List(64)); n != 0 {
		t.Errorf("%d audit entries for requests that named nobody", n)
	}
}

// The actor is the verified one: for a signed-in admin, the subject of the
// session token.
func TestSubjectAccessAuditNamesTheSignedInUser(t *testing.T) {
	st := &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 1}
	f := newSessFixture(t, func(c *Config) {
		c.Store = st
		c.DefaultOrgID = histOrg
	})
	if _, err := f.users.Set("4242", users.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	token := f.token("4242")
	if res := f.call("POST", "/events/subject", token, `{"email":"`+subjEmail+`"}`); res.status != http.StatusOK {
		t.Fatalf("subject access: %d %s", res.status, res.raw)
	}
	for _, e := range f.audit.List(64) {
		if e.Kind == auditSubjectAccess {
			wantSubjectEntry(t, e, actorUserPrefix+"4242", "email", "rows", 1, subjectOK)
			return
		}
	}
	t.Fatal("no privacy.subject_access entry")
}
