package api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/docstore"
	"github.com/yatuk/tamga/internal/patterns"
)

type downPersister struct{}

func (downPersister) Put(string, any) error {
	return fmt.Errorf("%w: connection refused to 10.0.0.9:5432", docstore.ErrUnavailable)
}
func (downPersister) Delete(string) error { return docstore.ErrUnavailable }
func (downPersister) All(func(string, []byte) error) error {
	return nil
}

// A pattern the database could not take is a 503, and the response does not
// carry the database's own words.
func TestPatternCreate_StorageDownIs503(t *testing.T) {
	store := patterns.NewMemoryStore()
	if err := store.Persist(downPersister{}); err != nil {
		t.Fatal(err)
	}
	cfg := Config{AdminKey: "test-key", Patterns: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	post := func(body string) (int, string) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/patterns", strings.NewReader(body))
		adminHeaders(cfg.AdminKey)(req)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	status, body := post(`{"name":"codename","kind":"literal","pattern":"falcon","enabled":true}`)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", status, body)
	}
	if strings.Contains(body, "10.0.0.9") || strings.Contains(body, "connection refused") {
		t.Fatalf("the response repeats the storage error: %s", body)
	}
	if len(store.List()) != 0 {
		t.Fatal("the pattern was applied although it was not stored")
	}
	// An invalid pattern is still the caller's mistake.
	if status, _ := post(`{"name":"bad","kind":"regex","pattern":"(","enabled":true}`); status != http.StatusBadRequest {
		t.Fatalf("invalid regex: status = %d, want 400", status)
	}
}
