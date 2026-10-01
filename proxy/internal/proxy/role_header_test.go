package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
)

const roleExceptionPolicy = `
version: "1.0"
rules:
  secret_detection:
    action: BLOCK
    sensitivity: low
exceptions:
  - rule: "secret_detection"
    roles: ["admin"]
    reason: "test waiver"
providers:
  allowed: [openai]
`

func postWithRole(t *testing.T, cfg *config.Config, role string) (status int, upstreamHeaders http.Header) {
	t.Helper()
	var mu sync.Mutex
	var seen http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)

	pol := mustPolicy(t, roleExceptionPolicy)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       cfg,
	}))
	defer srv.Close()

	body := []byte(`{"messages":[{"role":"user","content":"key AKIAIOSFODNN7EXAMPLE"}]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req.Header.Set("X-Tamga-Role", role)
	}
	req.Header.Set("X-Tamga-Org-Id", "org-internal")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	mu.Lock()
	defer mu.Unlock()
	return resp.StatusCode, seen
}

// A caller must not be able to waive a rule by claiming a role in a header.
func TestRoleHeader_IgnoredByDefault(t *testing.T) {
	status, _ := postWithRole(t, &config.Config{}, "admin")
	if status != http.StatusForbidden {
		t.Fatalf("self-declared X-Tamga-Role: admin must not bypass BLOCK; got %d", status)
	}
}

func TestRoleHeader_HonouredWhenTrusted(t *testing.T) {
	status, upstream := postWithRole(t, &config.Config{TrustRoleHeader: true}, "admin")
	if status != http.StatusOK {
		t.Fatalf("with TrustRoleHeader the exception should apply; got %d", status)
	}
	// Caller-supplied identity headers are for the proxy only.
	for _, name := range []string{"X-Tamga-Role", "X-Tamga-Org-Id"} {
		if v := upstream.Get(name); v != "" {
			t.Errorf("caller header %s=%q was forwarded to the provider", name, v)
		}
	}
}

func TestRoleHeader_StrictModeStillWins(t *testing.T) {
	status, _ := postWithRole(t, &config.Config{TrustRoleHeader: true, StrictMode: true}, "admin")
	if status != http.StatusForbidden {
		t.Fatalf("strict mode must disable exceptions even for a trusted role; got %d", status)
	}
}
