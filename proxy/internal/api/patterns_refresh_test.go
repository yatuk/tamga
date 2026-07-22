package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/scanner"
)

// TestPatternCreate_RefreshesScanner verifies that creating a pattern via the
// API activates it immediately (the create handler calls CustomScanner.Refresh),
// rather than leaving it inactive until the next policy reload.
func TestPatternCreate_RefreshesScanner(t *testing.T) {
	store := patterns.NewMemoryStore()
	specsFromStore := func() []scanner.CustomEntitySpec {
		var out []scanner.CustomEntitySpec
		for _, p := range store.List() {
			if !p.Enabled || p.Kind != patterns.KindRegex {
				continue
			}
			out = append(out, scanner.CustomEntitySpec{
				Name: p.Name, Pattern: p.Pattern, Severity: p.Severity, Confidence: 0.8,
			})
		}
		return out
	}
	cs := scanner.NewCustomScanner(specsFromStore)

	// Force an initial compile with an empty store so a later create must
	// Refresh to take effect (the scanner caches compiled specs).
	if fs, _ := cs.Scan(context.Background(), []byte("FIB-12345678")); len(fs) != 0 {
		t.Fatalf("expected no findings before pattern exists, got %d", len(fs))
	}

	cfg := Config{
		AdminKey:      "test-key",
		Patterns:      store,
		CustomScanner: cs,
		DefaultOrgID:  "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	body := strings.NewReader(`{"name":"fib_musteri_no","kind":"regex","pattern":"FIB-\\d{8}","severity":"high","enabled":true}`)
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/patterns", body)
	adminHeaders(cfg.AdminKey)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: got %d", resp.StatusCode)
	}

	// Without the Refresh fix, the scanner would still be compiled against the
	// empty store and find nothing here.
	fs, _ := cs.Scan(context.Background(), []byte("müşteri FIB-12345678 kaydı"))
	if len(fs) == 0 {
		t.Fatal("created pattern is not active — CustomScanner.Refresh was not called on create")
	}
	if fs[0].Category != "fib_musteri_no" {
		t.Errorf("finding category = %q, want fib_musteri_no", fs[0].Category)
	}
}
