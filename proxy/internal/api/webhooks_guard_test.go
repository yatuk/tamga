package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/webhooks"
)

func postJSON(t *testing.T, url, key, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	adminHeaders(key)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// A destination that points inside the network, or does not use https, is
// turned down when it is saved, with a 400 that says why.
func TestWebhookCreate_RejectsBlockedAndInsecureURLs(t *testing.T) {
	store := webhooks.NewMemoryStore()
	cfg := Config{AdminKey: "test-key", Webhooks: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	for u, want := range map[string]string{
		"http://127.0.0.1/":                         "internal or reserved address",
		"https://127.0.0.1:8443/hook":               "internal or reserved address",
		"http://169.254.169.254/latest/meta-data/":  "internal or reserved address",
		"https://10.0.0.8:5432/":                    "internal or reserved address",
		"https://[::ffff:7f00:1]/":                  "internal or reserved address",
		"http://allowed@127.0.0.1/":                 "internal or reserved address",
		"https://localhost/hook":                    "internal or reserved address",
		"https://2130706433/":                       "not a valid host name",
		"http://hooks.example.com/services/T0/B0/x": "must use https",
		"ftp://example.com/x":                       "scheme must be https",
		"file:///etc/passwd":                        "scheme must be https",
		"https://example.com/ x":                    "spaces or control characters",
	} {
		body, _ := json.Marshal(map[string]interface{}{"label": "x", "kind": "generic", "url": u, "enabled": true})
		status, out := postJSON(t, ts.URL+"/api/v1/webhooks", cfg.AdminKey, string(body))
		if status != http.StatusBadRequest || !strings.Contains(out, want) {
			t.Errorf("%s: %d %s, want 400 mentioning %q", u, status, out, want)
		}
		if strings.Contains(out, "://") {
			t.Errorf("%s: the error repeats the URL: %s", u, out)
		}
	}
	if n := len(store.List()); n != 0 {
		t.Fatalf("%d refused destinations were stored", n)
	}

	status, _ := postJSON(t, ts.URL+"/api/v1/webhooks", cfg.AdminKey, `{"label":"ok","kind":"slack","url":"https://hooks.example.com/services/T0/B0/x","enabled":true}`)
	if status != http.StatusCreated {
		t.Fatalf("a good destination: %d", status)
	}
}

// No route updates a destination today. The store's Update holds the check,
// so it is in place when one does.
func TestWebhookUpdate_RejectsBlockedAndInsecureURLs(t *testing.T) {
	store := webhooks.NewMemoryStore()
	wh, err := store.Create(webhooks.Webhook{Label: "ok", Kind: webhooks.KindSlack, URL: "https://hooks.example.com/x", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://127.0.0.1/", "http://169.254.169.254/", "https://192.168.1.1/", "http://hooks.example.com/x", "gopher://example.com/"} {
		if _, err := store.Update(wh.ID, webhooks.Webhook{URL: u, Enabled: true}); err == nil {
			t.Errorf("%s: the update was accepted", u)
		}
	}
	if got, _ := store.Get(wh.ID); got.URL != wh.URL {
		t.Fatalf("a refused update changed the URL to %q", got.URL)
	}
}

// The Test button on a destination the guard refuses: one fixed answer, the
// same whether something listens there or not, and no status code to scan
// ports with.
func TestWebhookTest_BlockedDestinationGivesTheFixedOutcome(t *testing.T) {
	listening := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the probe reached a loopback listener")
	}))
	defer listening.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	// The destinations are saved while loopback is allowed, the way an
	// existing deployment has them from before the guard, and probed under
	// the strict default.
	store := webhooks.NewMemoryStore()
	store.UseGuard(webhooks.NewGuard(webhooks.GuardConfig{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}))
	open, err := store.Create(webhooks.Webhook{Kind: webhooks.KindGeneric, URL: listening.URL + "/hook", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	shut, err := store.Create(webhooks.Webhook{Kind: webhooks.KindGeneric, URL: closedURL + "/hook", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	store.UseGuard(webhooks.NewGuard(webhooks.GuardConfig{}))

	cfg := Config{AdminKey: "test-key", Webhooks: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	var answers []string
	for _, id := range []string{open.ID, shut.ID} {
		status, out := postJSON(t, ts.URL+"/api/v1/webhooks/"+id+"/test", cfg.AdminKey, "")
		if status != http.StatusBadRequest {
			t.Errorf("status %d, want 400", status)
		}
		if strings.Contains(out, "status_code") || strings.Contains(out, "127.0.0.1") || strings.Contains(out, "refused") {
			t.Errorf("the answer says too much: %s", out)
		}
		answers = append(answers, out)
	}
	if answers[0] != answers[1] || !strings.Contains(answers[0], `"error":"destination address is not allowed"`) {
		t.Fatalf("an open and a closed port answer differently: %q vs %q", answers[0], answers[1])
	}
	if n := store.BlockedProbes()[webhooks.KindGeneric]; n != 2 {
		t.Errorf("blocked probes = %d, want 2", n)
	}
}

func TestMetrics_AlertCountersBlockedAndSkipped(t *testing.T) {
	prev := alertStats
	defer func() { alertStats = prev }()
	SetAlertStats(func() []webhooks.KindStats {
		return []webhooks.KindStats{{Kind: "slack", Sent: 7, Blocked: 5, Skipped: 9}}
	})
	rec := httptest.NewRecorder()
	Config{}.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	out := rec.Body.String()
	for _, want := range []string{
		`tamga_alerts_total{kind="slack",result="blocked"} 5`,
		`tamga_alerts_total{kind="slack",result="skipped"} 9`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics lack %q", want)
		}
	}
}
