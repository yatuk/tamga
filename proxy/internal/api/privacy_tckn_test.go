package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/store"
)

// subjTCKNRaw is a checksum-valid national id that belongs to nobody.
const subjTCKNRaw = "10000000146"

// matchSubjectStore is a request log that can also be asked by the stored
// forms of an identifier. It keeps what it was asked with.
type matchSubjectStore struct {
	*subjectStore
	forms [][]string
}

func (s *matchSubjectStore) SubjectAccessByMatch(_ context.Context, _ string, forms []string, _ int) ([]store.RequestLogRow, error) {
	s.calls++
	s.forms = append(s.forms, forms)
	if s.err != nil {
		return nil, s.err
	}
	return make([]store.RequestLogRow, s.rows), nil
}

func (s *matchSubjectStore) EraseSubjectByMatch(_ context.Context, _ string, forms []string) (int, error) {
	s.calls++
	s.forms = append(s.forms, forms)
	if s.err != nil {
		return 0, s.err
	}
	return s.deleted, nil
}

func newMatchSubjectFixture(t *testing.T, st *matchSubjectStore) *subjectFixture {
	t.Helper()
	f := &subjectFixture{t: t, audit: newSubjectFixture(t, nil).audit, store: st.subjectStore}
	f.h = testMux(Config{AdminKey: "subject-admin-key", Audit: f.audit, DefaultOrgID: histOrg, Store: st})
	return f
}

// withFindingHashKey gives the process a finding hash key for one test.
func withFindingHashKey(t *testing.T) *store.FindingHasher {
	t.Helper()
	h, err := store.NewFindingHasher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), "")
	if err != nil {
		t.Fatal(err)
	}
	prev := store.CurrentFindingHasher()
	store.SetFindingHasher(h)
	t.Cleanup(func() { store.SetFindingHasher(prev) })
	return h
}

// The operator sends the national id they were given. The server turns it
// into the forms the log holds, and the id is in nothing that outlives the
// request: not what the store is asked with, the answer or the audit entry.
func TestSubjectByNationalID(t *testing.T) {
	h := withFindingHashKey(t)
	for _, method := range []string{"POST", "DELETE"} {
		st := &matchSubjectStore{subjectStore: &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 2, deleted: 5}}
		f := newMatchSubjectFixture(t, st)
		rec := f.do(method, "/events/subject", `{"tckn":"`+subjTCKNRaw+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", method, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), subjTCKNRaw) {
			t.Errorf("%s: the answer repeats the id: %s", method, rec.Body.String())
		}
		if len(st.forms) != 1 {
			t.Fatalf("%s: the store was asked %d times, want 1", method, len(st.forms))
		}
		forms := st.forms[0]
		want := map[string]bool{
			"10*******46":         true,
			h.Hash("10*******46"): true,
		}
		legacy := 0
		for _, form := range forms {
			if strings.Contains(form, subjTCKNRaw) {
				t.Errorf("%s: the store was given the id: %q", method, form)
			}
			delete(want, form)
			if strings.HasPrefix(form, store.LegacyMatchPrefix) {
				legacy++
			}
		}
		if len(want) != 0 || legacy != 1 || len(forms) != 3 {
			t.Errorf("%s: forms %v: want the masked match, its keyed digest and the unkeyed digest of earlier versions", method, forms)
		}

		kind, countKey, count := auditSubjectAccess, "rows", 2
		if method == "DELETE" {
			kind, countKey, count = auditSubjectErase, "deleted", 5
		}
		raw, _ := json.Marshal(f.audit.List(64))
		for _, leak := range append([]string{subjTCKNRaw, "10*******46"}, forms...) {
			if strings.Contains(string(raw), leak) {
				t.Errorf("%s: the audit log holds %q: %s", method, leak, raw)
			}
		}
		got := f.entries(kind)
		if len(got) != 1 {
			t.Fatalf("%s: %d audit entries, want 1", method, len(got))
		}
		wantSubjectEntry(t, got[0], ActorAdminKey, selectorTCKN, countKey, count, subjectOK)
	}
}

// Without a key the id is still looked for in the forms that need none, so
// rows written before the key existed are found.
func TestSubjectByNationalID_WithoutKey(t *testing.T) {
	prev := store.CurrentFindingHasher()
	store.SetFindingHasher(nil)
	t.Cleanup(func() { store.SetFindingHasher(prev) })

	st := &matchSubjectStore{subjectStore: &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 1}}
	f := newMatchSubjectFixture(t, st)
	if rec := f.do("POST", "/events/subject", `{"tckn":"`+subjTCKNRaw+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if len(st.forms) != 1 || len(st.forms[0]) != 2 {
		t.Fatalf("forms %v, want the masked match and the unkeyed digest", st.forms)
	}
}

// A national id is never read from a URL, a mistyped one is refused without
// being repeated, and neither reaches the store or the audit log.
func TestSubjectByNationalID_Refused(t *testing.T) {
	st := &matchSubjectStore{subjectStore: &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 3, deleted: 3}}
	f := newMatchSubjectFixture(t, st)
	for _, c := range []struct{ method, query, body string }{
		{"GET", "?tckn=" + subjTCKNRaw, ""},
		// Not even beside a selector the GET does take.
		{"GET", "?user_id=u1&tckn=" + subjTCKNRaw, ""},
		{"POST", "", `{"tckn":"10000000147"}`},  // wrong check digit
		{"POST", "", `{"tckn":"1000000014"}`},   // ten digits
		{"POST", "", `{"tckn":"00000000000"}`},  // starts with 0
		{"POST", "", `{"tckn":"1000000014a"}`},  // not a digit
		{"POST", "", `{"tckn":" 10000000146"}`}, // not trimmed, not guessed at
		{"DELETE", "", `{"tckn":"%"}`},
		{"DELETE", "", `{"tckn":"10*******46"}`},
		{"DELETE", "", `{"tckn":"10000000147"}`},
	} {
		rec := f.do(c.method, "/events/subject"+c.query, c.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: %d, want 400", c.method, c.query, c.body, rec.Code)
		}
		for _, v := range []string{subjTCKNRaw, "10000000147", "1000000014"} {
			if strings.Contains(rec.Body.String(), v) {
				t.Errorf("%s %s %s: the refusal repeats the value: %s", c.method, c.query, c.body, rec.Body.String())
			}
		}
	}
	if st.calls != 0 {
		t.Errorf("the store was asked %d times by requests that are refused", st.calls)
	}
	if n := len(f.audit.List(64)); n != 0 {
		t.Errorf("%d audit entries for requests that are refused", n)
	}
}

// A store that cannot be asked by match answers as one without a request
// log: nothing is found, nothing is deleted, and that is what is said.
func TestSubjectByNationalID_StoreWithoutMatch(t *testing.T) {
	st := &subjectStore{NoopStore: store.NewNoopStoreSilent(), rows: 3, deleted: 3}
	f := newSubjectFixture(t, st)
	for method, kind := range map[string]string{"POST": auditSubjectAccess, "DELETE": auditSubjectErase} {
		rec := f.do(method, "/events/subject", `{"tckn":"`+subjTCKNRaw+`"}`)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "store not available") {
			t.Errorf("%s: %d %s", method, rec.Code, rec.Body.String())
		}
		got := f.entries(kind)
		if len(got) != 1 {
			t.Fatalf("%s: %d audit entries, want 1", method, len(got))
		}
		countKey := "rows"
		if method == "DELETE" {
			countKey = "deleted"
		}
		wantSubjectEntry(t, got[0], ActorAdminKey, selectorTCKN, countKey, 0, subjectStoreUnavailable)
	}
	if st.calls != 0 {
		t.Errorf("the store was asked %d times with a selector it does not take", st.calls)
	}
}

func TestValidTCKN(t *testing.T) {
	for id, want := range map[string]bool{
		subjTCKNRaw:    true,
		"10000000147":  false,
		"10000000156":  false,
		"00000000000":  false,
		"1000000014":   false,
		"100000001460": false,
		"":             false,
		"1000000014６":  false, // a full-width digit
	} {
		if got := validTCKN(id); got != want {
			t.Errorf("validTCKN(%q) = %v, want %v", id, got, want)
		}
	}
}
