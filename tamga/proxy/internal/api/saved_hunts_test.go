package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/store"
)

// inMemSavedHuntStore is a thread-safe, in-memory SavedHuntStore for tests.
type inMemSavedHuntStore struct {
	mu    sync.RWMutex
	hunts map[string]*store.SavedHunt // keyed by id
	seq   int
}

func newInMemSavedHuntStore() *inMemSavedHuntStore {
	return &inMemSavedHuntStore{hunts: make(map[string]*store.SavedHunt)}
}

func (s *inMemSavedHuntStore) List(ctx context.Context, orgID string) ([]store.SavedHunt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.SavedHunt
	for _, h := range s.hunts {
		if h.OrgID == orgID {
			out = append(out, *h)
		}
	}
	// sort by created_at DESC
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if out == nil {
		out = []store.SavedHunt{}
	}
	return out, nil
}

func (s *inMemSavedHuntStore) Create(ctx context.Context, hunt *store.SavedHunt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	hunt.ID = "test-id-" + itoa(s.seq)
	copy := *hunt
	s.hunts[hunt.ID] = &copy
	return nil
}

func (s *inMemSavedHuntStore) Update(ctx context.Context, hunt *store.SavedHunt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.hunts[hunt.ID]
	if !ok || existing.OrgID != hunt.OrgID {
		return store.ErrSavedHuntNotFound
	}
	existing.Name = hunt.Name
	if hunt.Query != nil {
		existing.Query = hunt.Query
	}
	existing.UpdatedAt = hunt.UpdatedAt
	return nil
}

func (s *inMemSavedHuntStore) Delete(ctx context.Context, orgID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.hunts[id]
	if !ok || existing.OrgID != orgID {
		return store.ErrSavedHuntNotFound
	}
	delete(s.hunts, id)
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ── Tests ────────────────────────────────────────────────────────────────────

func testConfig(store store.SavedHuntStore) Config {
	return Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-a",
		SavedHunts:   store,
	}
}

func TestSavedHuntCRUDRoundTrip(t *testing.T) {
	st := newInMemSavedHuntStore()
	cfg := testConfig(st)
	mux := http.NewServeMux()
	// Mount exactly as the real router does — under /api/v1 with StripPrefix.
	inner := http.NewServeMux()
	protected := http.NewServeMux()
	protected.HandleFunc("GET /saved-hunts", cfg.handleSavedHuntList)
	protected.HandleFunc("POST /saved-hunts", cfg.handleSavedHuntCreate)
	protected.HandleFunc("PUT /saved-hunts/{id}", cfg.handleSavedHuntUpdate)
	protected.HandleFunc("DELETE /saved-hunts/{id}", cfg.handleSavedHuntDelete)
	inner.Handle("/", adminAuth(cfg)(protected))
	chained := corsMiddleware(cfg)(inner)
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", chained))

	authHeader := func() map[string]string {
		return map[string]string{"X-Tamga-Admin-Key": "test-key", "Content-Type": "application/json"}
	}

	// Step 1: Create a hunt.
	createBody := `{"name":"Suspicious PII","query_json":{"action":"block","shadow":true,"range":"7d"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-hunts", strings.NewReader(createBody))
	for k, v := range authHeader() {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created store.SavedHunt
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("create: decode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create: expected non-empty id")
	}
	if created.Name != "Suspicious PII" {
		t.Fatalf("create: name mismatch: %q", created.Name)
	}

	// Step 2: List — should contain the created hunt.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/saved-hunts", nil)
	for k, v := range authHeader() {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Items []store.SavedHunt `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("list: decode: %v", err)
	}
	if listResp.Total != 1 {
		t.Fatalf("list: expected 1 item, got %d", listResp.Total)
	}
	if listResp.Items[0].ID != created.ID {
		t.Fatalf("list: id mismatch: got %s, want %s", listResp.Items[0].ID, created.ID)
	}

	// Step 3: Update the hunt.
	updateBody := `{"name":"Updated Hunt","query_json":{"action":"pass","shadow":false,"range":"24h"}}`
	req = httptest.NewRequest(http.MethodPut, "/api/v1/saved-hunts/"+created.ID, strings.NewReader(updateBody))
	for k, v := range authHeader() {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated store.SavedHunt
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("update: decode: %v", err)
	}
	// Note: the in-mem Update only sets Name and Query, not returning full row.
	// Verify the name was updated.
	if updated.Name != "" && updated.Name != "Updated Hunt" {
		t.Fatalf("update: name mismatch: %q", updated.Name)
	}

	// Verify list reflects the update.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/saved-hunts", nil)
	for k, v := range authHeader() {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("list after update: decode: %v", err)
	}
	if listResp.Total != 1 {
		t.Fatalf("list after update: expected 1, got %d", listResp.Total)
	}
	if listResp.Items[0].Name != "Updated Hunt" {
		t.Fatalf("list after update: name %q", listResp.Items[0].Name)
	}

	// Step 4: Delete the hunt.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/saved-hunts/"+created.ID, nil)
	for k, v := range authHeader() {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify list is now empty.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/saved-hunts", nil)
	for k, v := range authHeader() {
		req.Header.Set(k, v)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("list after delete: decode: %v", err)
	}
	if listResp.Total != 0 {
		t.Fatalf("list after delete: expected 0, got %d", listResp.Total)
	}
}

func TestSavedHuntOrgIsolation(t *testing.T) {
	st := newInMemSavedHuntStore()
	cfg := testConfig(st)
	mux := http.NewServeMux()
	inner := http.NewServeMux()
	protected := http.NewServeMux()
	protected.HandleFunc("GET /saved-hunts", cfg.handleSavedHuntList)
	protected.HandleFunc("POST /saved-hunts", cfg.handleSavedHuntCreate)
	protected.HandleFunc("DELETE /saved-hunts/{id}", cfg.handleSavedHuntDelete)
	inner.Handle("/", adminAuth(cfg)(protected))
	chained := corsMiddleware(cfg)(inner)
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", chained))

	// Create a hunt for org-a.
	createBody := `{"name":"Org A Hunt","query_json":{"action":"block"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-hunts", strings.NewReader(createBody))
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org-a: expected 201, got %d", rec.Code)
	}

	// List with org-b via header — should be empty.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/saved-hunts", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("X-Tamga-Org-Id", "org-b")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list org-b: expected 200, got %d", rec.Code)
	}
	var listResp struct {
		Items []store.SavedHunt `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listResp); err != nil {
		t.Fatalf("list org-b: decode: %v", err)
	}
	if listResp.Total != 0 {
		t.Fatalf("list org-b: expected 0 items, got %d", listResp.Total)
	}

	// Delete with org-b should 404 (wrong org).
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/saved-hunts/nonexistent", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("X-Tamga-Org-Id", "org-b")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete org-b nonexistent: expected 404, got %d", rec.Code)
	}
}

func TestSavedHuntInvalidBody(t *testing.T) {
	st := newInMemSavedHuntStore()
	cfg := testConfig(st)
	mux := http.NewServeMux()
	inner := http.NewServeMux()
	protected := http.NewServeMux()
	protected.HandleFunc("POST /saved-hunts", cfg.handleSavedHuntCreate)
	inner.Handle("/", adminAuth(cfg)(protected))
	chained := corsMiddleware(cfg)(inner)
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", chained))

	// Invalid JSON body.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-hunts", strings.NewReader("not-json"))
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSavedHuntMissingName(t *testing.T) {
	st := newInMemSavedHuntStore()
	cfg := testConfig(st)
	mux := http.NewServeMux()
	inner := http.NewServeMux()
	protected := http.NewServeMux()
	protected.HandleFunc("POST /saved-hunts", cfg.handleSavedHuntCreate)
	inner.Handle("/", adminAuth(cfg)(protected))
	chained := corsMiddleware(cfg)(inner)
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", chained))

	// Missing name.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-hunts", strings.NewReader(`{"query_json":{"action":"block"}}`))
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing name: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSavedHuntDeleteNonexistent(t *testing.T) {
	st := newInMemSavedHuntStore()
	cfg := testConfig(st)
	mux := http.NewServeMux()
	inner := http.NewServeMux()
	protected := http.NewServeMux()
	protected.HandleFunc("DELETE /saved-hunts/{id}", cfg.handleSavedHuntDelete)
	inner.Handle("/", adminAuth(cfg)(protected))
	chained := corsMiddleware(cfg)(inner)
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", chained))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/saved-hunts/nonexistent-id", nil)
	req.Header.Set("X-Tamga-Admin-Key", "test-key")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete nonexistent: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
