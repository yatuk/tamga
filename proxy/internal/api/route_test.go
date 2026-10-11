package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/upstream"
)

const routeSimPolicy = `
version: "1.0"
name: route-sim
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
    types: [tc_kimlik]
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
providers:
  pools:
    internal-llm:
      internal: true
      dialect: openai
      providers:
        - name: own-1
          base_url: https://llm.corp.example:8000
          priority: 1
`

// A simulation reports ROUTE as the verdict and names the pool. It sends
// nothing, so it says nothing about whether the pool can be reached.
func TestPolicySimulate_RouteVerdictNamesThePool(t *testing.T) {
	t.Cleanup(policy.SetUpstreamRules(&policy.UpstreamRules{AllowHostPorts: []string{"llm.corp.example:8000"}}))
	cfg := Config{AdminKey: "test-key", DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	simulate := func(text string) map[string]interface{} {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"yaml": routeSimPolicy, "sample_text": text})
		req, _ := http.NewRequest("POST", ts.URL+"/api/v1/policies/simulate", strings.NewReader(string(body)))
		adminHeaders(cfg.AdminKey)(req)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d: %s", resp.StatusCode, raw)
		}
		var out map[string]interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	out := simulate("TC kimlik 10000000146, e-posta jane.doe@example.com")
	if out["action"] != "ROUTE" || out["route_to"] != "internal-llm" {
		t.Fatalf("simulate = %v", out)
	}
	perFinding := map[string]string{}
	for _, f := range out["findings"].([]interface{}) {
		m := f.(map[string]interface{})
		perFinding[m["category"].(string)] = m["action"].(string)
	}
	if perFinding["tc_kimlik"] != "ROUTE" || perFinding["email"] != "REDACT" {
		t.Errorf("per-finding actions = %v", perFinding)
	}

	if out := simulate("e-posta jane.doe@example.com"); out["action"] != "REDACT" || out["route_to"] != nil {
		t.Fatalf("a REDACT verdict names a pool: %v", out)
	}
}

// Routed requests are counted on their own in /stats, from the request log
// and from the process counters alike, and are not counted as passed.
func TestStats_CountsRoutedRequests(t *testing.T) {
	now := time.Now().UTC()
	h := newFakeHistory()
	h.add(histOrg, "r1", "openai", "internal-model-1", "route", now.Add(-time.Minute), 1)
	h.add(histOrg, "r2", "openai", "internal-model-1", "ROUTE", now.Add(-2*time.Minute), 1)
	h.add(histOrg, "r3", "openai", "gpt-4o", "pass", now.Add(-3*time.Minute), 1)
	h.add(histOrg, "r4", "openai", "gpt-4o", "block", now.Add(-4*time.Minute), 1)
	h.rows[0].rec.RoutePool = "internal-llm"

	var st struct {
		Total  int64 `json:"total_requests"`
		Routed int64 `json:"routed_requests"`
		Passed int64 `json:"passed_requests"`
	}
	status, _, body := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/stats?range=24h")
	if err := json.Unmarshal(body, &st); status != http.StatusOK || err != nil {
		t.Fatalf("status %d, %v: %s", status, err, body)
	}
	if st.Total != 4 || st.Routed != 2 || st.Passed != 1 {
		t.Fatalf("stats from the request log = %+v", st)
	}

	// The stored row says which pool.
	status, _, body = histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/r1")
	if status != http.StatusOK || !strings.Contains(string(body), `"route_pool":"internal-llm"`) || !strings.Contains(string(body), `"action":"ROUTE"`) {
		t.Fatalf("stored detail: status %d: %s", status, body)
	}

	// Without a database: the process counters.
	m := &events.Metrics{}
	count := events.MetricsHandler(m)
	count(events.Event{EventType: "request_scanned", Action: "ROUTE"})
	count(events.Event{EventType: "request_scanned", Action: "PASS"})
	cfg := Config{AdminKey: "test-key", DefaultOrgID: histOrg, Started: time.Now(), Metrics: m, Recent: events.NewRecentBuffer(10)}
	status, _, body = histGet(t, cfg, "/stats")
	st.Total, st.Routed, st.Passed = 0, 0, 0
	if err := json.Unmarshal(body, &st); status != http.StatusOK || err != nil {
		t.Fatalf("status %d, %v: %s", status, err, body)
	}
	if st.Total != 2 || st.Routed != 1 || st.Passed != 1 {
		t.Fatalf("stats from the process counters = %+v", st)
	}

	upstream.RecordRouteUnavailable("internal-llm", upstream.RouteUpstream)
	status, _, body = histGet(t, cfg, "/metrics")
	if status != http.StatusOK || !strings.Contains(string(body), "tamga_routed_total 1\n") ||
		!strings.Contains(string(body), `tamga_route_unavailable_total{pool="internal-llm",reason="upstream"}`) {
		t.Fatalf("metrics: status %d:\n%s", status, body)
	}
}
