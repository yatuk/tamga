package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

const (
	histOrg      = "11111111-1111-1111-1111-111111111111"
	histOtherOrg = "22222222-2222-2222-2222-222222222222"
)

// fakeHistory stands in for request_logs: rows per organisation, and the
// queries of store/history.go worked out over them in memory.
type fakeHistory struct {
	*store.NoopStore
	rows  []fakeRow
	err   error // every query fails with it when set
	calls int
	// exportMax, when set, stands in for store.ExportMaxRows.
	exportMax  int
	lastExport store.EventSearchParams
}

type fakeRow struct {
	org string
	rec store.RequestRecord
}

func newFakeHistory() *fakeHistory { return &fakeHistory{NoopStore: &store.NoopStore{}} }

func (f *fakeHistory) add(org, id, provider, model, action string, at time.Time, scanMs float64, findings ...scanner.Finding) {
	raw, _ := json.Marshal(findings)
	if len(findings) == 0 {
		raw = []byte("[]")
	}
	f.rows = append(f.rows, fakeRow{org: org, rec: store.RequestRecord{
		RequestID: id, Provider: provider, Model: model, ModelFamily: model + "-family",
		Endpoint: "/v1/chat/completions", ActionTaken: action, Findings: raw,
		FindingsCount: len(findings), ScanLatencyMs: scanMs, CreatedAt: at,
	}})
}

func (f *fakeHistory) window(org string, from, to time.Time) []store.RequestRecord {
	var out []store.RequestRecord
	for _, r := range f.rows {
		if r.org != org || r.rec.CreatedAt.Before(from) || r.rec.CreatedAt.After(to) {
			continue
		}
		out = append(out, r.rec)
	}
	return out
}

func (f *fakeHistory) GetRequestRecord(_ context.Context, org, id string, since time.Time) (*store.RequestRecord, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	for _, r := range f.window(org, since, time.Now().Add(time.Hour)) {
		if r.RequestID == id {
			rec := r
			return &rec, nil
		}
	}
	return nil, nil
}

func (f *fakeHistory) RequestCounts(_ context.Context, org string, from, to time.Time) ([]store.RequestCount, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	type key struct{ provider, action string }
	m := map[key]*store.RequestCount{}
	for _, r := range f.window(org, from, to) {
		k := key{r.Provider, strings.ToUpper(r.ActionTaken)}
		if m[k] == nil {
			m[k] = &store.RequestCount{Provider: k.provider, Action: k.action}
		}
		m[k].Requests++
		// As the store does: a latency of zero is one that was not recorded.
		if r.ScanLatencyMs > 0 {
			m[k].ScanLatencySumMs += r.ScanLatencyMs
			m[k].ScanSamples++
		}
	}
	var out []store.RequestCount
	for _, c := range m {
		out = append(out, *c)
	}
	return out, nil
}

func (f *fakeHistory) RequestTimeseries(_ context.Context, org string, from, to time.Time, bucket string) ([]store.RequestBucket, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	step := map[string]time.Duration{"minute": time.Minute, "hour": time.Hour, "day": 24 * time.Hour}[bucket]
	m := map[time.Time]*store.RequestBucket{}
	lat := map[time.Time][]float64{}
	for _, r := range f.window(org, from, to) {
		k := r.CreatedAt.UTC().Truncate(step)
		if m[k] == nil {
			m[k] = &store.RequestBucket{Start: k}
		}
		m[k].Total++
		switch strings.ToUpper(r.ActionTaken) {
		case "BLOCK":
			m[k].Blocked++
		case "REDACT":
			m[k].Redacted++
		case "WARN":
			m[k].Warned++
		}
		lat[k] = append(lat[k], r.ScanLatencyMs)
	}
	var out []store.RequestBucket
	for k, b := range m {
		b.ScanP95Ms = percentile(lat[k], 0.95)
		b.ScanSamples = int64(len(lat[k]))
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

func (f *fakeHistory) FindingCounts(_ context.Context, org string, from, to time.Time) ([]store.FindingCount, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	m := map[store.FindingCount]int64{}
	for _, r := range f.window(org, from, to) {
		for _, fd := range storedFindings(r.Findings) {
			m[store.FindingCount{Type: fd.Type, Category: fd.Category, Severity: strings.ToLower(fd.Severity)}]++
		}
	}
	var out []store.FindingCount
	for k, n := range m {
		k.Count = n
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeHistory) ModelCounts(_ context.Context, org string, from, to time.Time) ([]store.ModelCount, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	m := map[store.ModelCount]int64{}
	for _, r := range f.window(org, from, to) {
		m[store.ModelCount{Provider: r.Provider, Model: r.Model, ModelFamily: r.ModelFamily}]++
	}
	var out []store.ModelCount
	for k, n := range m {
		k.Requests = n
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeHistory) RiskProfiles(_ context.Context, org string, from, to time.Time) ([]store.RiskProfile, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out []store.RiskProfile
	for _, r := range f.window(org, from, to) {
		if fs := storedFindings(r.Findings); len(fs) > 0 {
			out = append(out, store.RiskProfile{Findings: fs, Requests: 1})
		}
	}
	return out, nil
}

func (f *fakeHistory) ExportRequestRecords(_ context.Context, org string, p store.EventSearchParams, max int) ([]store.RequestRecord, bool, error) {
	f.calls++
	f.lastExport = p
	if f.err != nil {
		return nil, false, f.err
	}
	if f.exportMax > 0 {
		max = f.exportMax
	}
	var out []store.RequestRecord
	for _, r := range f.window(org, p.Since, p.Until) {
		if p.RequestID != "" && !strings.Contains(strings.ToLower(r.RequestID), strings.ToLower(p.RequestID)) {
			continue
		}
		if wanted := events.ActionSet(p.Action); len(wanted) > 0 && !events.ActionIn(wanted, r.ActionTaken) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > max {
		return out[:max], true, nil
	}
	return out, false, nil
}

func histConfig(h store.Store, ring *events.RecentBuffer) Config {
	return Config{
		AdminKey:     "test-key",
		Recent:       ring,
		DefaultOrgID: histOrg,
		DatabaseURL:  "postgres://example.invalid/tamga",
		Store:        h,
		Started:      time.Now(),
		Metrics:      &events.Metrics{},
	}
}

// histGet calls the API and returns the status, the data source header and the body.
func histGet(t *testing.T, cfg Config, path string) (int, http.Header, []byte) {
	t.Helper()
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1"+path, nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body
}

func ringEvent(id, org, action string, at time.Time) events.Event {
	typ := "request_scanned"
	if action == "BLOCK" {
		typ = "request_blocked"
	}
	return events.Event{
		RequestID: id, OrgID: org, Provider: "openai", Model: "gpt-ring", ModelFamily: "ring-family",
		EventType: typ, Action: action, Timestamp: at, ScanLatencyMs: 7,
		InputRisk:  scanner.RiskScore{Percentage: 70, Level: "high", Score: 0.7},
		OutputRisk: scanner.RiskScore{Level: "none"},
	}
}

var histPII = scanner.Finding{Type: "pii", Category: "phone_tr", Severity: "high", Match: "05** *** ** 23", Confidence: 0.5}

func TestEventDetail_InRing(t *testing.T) {
	ring := events.NewRecentBuffer(10)
	ring.Add(ringEvent("req-live", "", "REDACT", time.Now().UTC()))
	h := newFakeHistory()
	status, hdr, body := histGet(t, histConfig(h, ring), "/events/req-live")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	if got := hdr.Get(dataSourceHeader); got != sourceMemory {
		t.Errorf("source = %q, want %q", got, sourceMemory)
	}
	if h.calls != 0 {
		t.Errorf("the database was asked %d times for an event the buffer holds", h.calls)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(body, &out)
	if out["model"] != "gpt-ring" || out["output_risk"] == nil {
		t.Errorf("detail from the buffer = %s", body)
	}
}

func TestEventDetail_NotInRingButStored(t *testing.T) {
	h := newFakeHistory()
	at := time.Now().UTC().Add(-26 * time.Hour)
	// Stored lower-case, as the event-bus writer stores it.
	h.add(histOrg, "req-old", "anthropic", "claude-haiku-4-5", "block", at, 3, histPII)
	status, hdr, body := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/req-old")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	if got := hdr.Get(dataSourceHeader); got != sourceDatabase {
		t.Errorf("source = %q, want %q", got, sourceDatabase)
	}
	if strings.Contains(string(body), "null") {
		t.Errorf("an unknown field was sent as null: %s", body)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out["action"] != "BLOCK" || out["event_type"] != "request_blocked" || out["provider"] != "anthropic" {
		t.Errorf("detail = %s", body)
	}
	// Not recorded in the row: left out, not filled in from the policy
	// loaded now or with a zero.
	for _, k := range []string{"output_risk", "policy_name", "policy_version"} {
		if _, ok := out[k]; ok {
			t.Errorf("%s is not stored and must be left out: %s", k, body)
		}
	}
	risk, _ := out["input_risk"].(map[string]interface{})
	if want := scanner.CalculateRisk([]scanner.Finding{histPII}); int(risk["percentage"].(float64)) != want.Percentage {
		t.Errorf("input_risk = %v, want %d%% from the stored findings", risk, want.Percentage)
	}
	findings, _ := out["findings"].([]interface{})
	if len(findings) != 1 {
		t.Fatalf("findings = %v", out["findings"])
	}
	if f := findings[0].(map[string]interface{}); f["action_taken"] != "BLOCK" || f["category"] != "phone_tr" {
		t.Errorf("finding = %v", f)
	}
	if ts, _ := time.Parse(time.RFC3339Nano, out["timestamp"].(string)); !ts.Equal(at) {
		t.Errorf("timestamp = %v, want %v", out["timestamp"], at)
	}
}

func TestEventDetail_NotFoundAnywhere(t *testing.T) {
	h := newFakeHistory()
	status, hdr, _ := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/nope")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if hdr.Get(dataSourceHeader) != sourceDatabase || h.calls != 1 {
		t.Errorf("the database should have been asked once: source %q, calls %d", hdr.Get(dataSourceHeader), h.calls)
	}
}

func TestEventDetail_OtherOrganisation(t *testing.T) {
	h := newFakeHistory()
	now := time.Now().UTC()
	h.add(histOtherOrg, "req-theirs", "openai", "gpt", "pass", now.Add(-time.Hour), 1)
	ring := events.NewRecentBuffer(10)
	ring.Add(ringEvent("req-theirs-live", histOtherOrg, "PASS", now))
	cfg := histConfig(h, ring)
	for _, id := range []string{"req-theirs", "req-theirs-live"} {
		if status, _, body := histGet(t, cfg, "/events/"+id); status != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 for another organisation's request; body %s", id, status, body)
		}
	}
}

// A later event with the same request id (a response scan) replaces the
// request's own in the buffer. The stored row is then the request.
func TestEventDetail_RingHoldsAnotherEventKind(t *testing.T) {
	h := newFakeHistory()
	now := time.Now().UTC()
	h.add(histOrg, "req-1", "openai", "gpt", "redact", now, 2, histPII)
	ring := events.NewRecentBuffer(10)
	ring.Add(events.Event{RequestID: "req-1", EventType: "output_scan_hint", Timestamp: now})
	_, hdr, body := histGet(t, histConfig(h, ring), "/events/req-1")
	if hdr.Get(dataSourceHeader) != sourceDatabase || !strings.Contains(string(body), `"REDACT"`) {
		t.Errorf("want the stored request, got source %q body %s", hdr.Get(dataSourceHeader), body)
	}
}

func TestEventDetail_StoreUnreachable(t *testing.T) {
	h := newFakeHistory()
	h.err = errors.New("dial tcp 192.0.2.1:5432: i/o timeout")
	ring := events.NewRecentBuffer(10)
	ring.Add(events.Event{RequestID: "req-hint", EventType: "output_scan_hint", Timestamp: time.Now().UTC()})
	cfg := histConfig(h, ring)
	if status, hdr, _ := histGet(t, cfg, "/events/unknown"); status != http.StatusNotFound || hdr.Get(dataSourceHeader) != sourceMemory {
		t.Errorf("unknown id with the database down: status %d source %q, want 404 from memory", status, hdr.Get(dataSourceHeader))
	}
	// What the buffer holds is still served, as before.
	if status, _, _ := histGet(t, cfg, "/events/req-hint"); status != http.StatusOK {
		t.Errorf("buffered event with the database down: status %d, want 200", status)
	}
}

type histStats struct {
	Total     int64            `json:"total_requests"`
	Blocked   int64            `json:"blocked_requests"`
	Redacted  int64            `json:"redacted_requests"`
	Warned    int64            `json:"warned_requests"`
	Passed    int64            `json:"passed_requests"`
	Providers map[string]int64 `json:"top_providers"`
	Types     map[string]int64 `json:"top_finding_types"`
	Cats      map[string]int64 `json:"top_categories"`
	ScanAvg   float64          `json:"scanner_latency_avg_ms"`
	AvgRisk   *int             `json:"avg_input_risk_pct"`
}

type histSeries struct {
	Points []struct {
		T        time.Time `json:"t"`
		Total    int64     `json:"total"`
		Blocked  int64     `json:"blocked"`
		Redacted int64     `json:"redacted"`
		Warned   int64     `json:"warned"`
		ScanP95  float64   `json:"scan_p95"`
	} `json:"points"`
}

// seedYesterday stores the requests of a day before a restart: 6 passed,
// 3 blocked and 1 redacted over two hours of yesterday, plus rows that must
// not be counted (another organisation, and one older than every window).
func seedYesterday(h *fakeHistory) {
	base := time.Now().UTC().Add(-20 * time.Hour)
	for i := 0; i < 6; i++ {
		h.add(histOrg, "pass-"+string(rune('a'+i)), "openai", "gpt-6-luna", "PASS", base.Add(time.Duration(i)*time.Minute), 2)
	}
	for i := 0; i < 3; i++ {
		h.add(histOrg, "block-"+string(rune('a'+i)), "anthropic", "claude-haiku-4-5", "BLOCK", base.Add(-time.Hour), 4,
			scanner.Finding{Type: "secret", Category: "aws_key", Severity: "critical"})
	}
	h.add(histOrg, "redact-a", "anthropic", "claude-haiku-4-5", "redact", base.Add(-time.Hour), 10, histPII, histPII)
	h.add(histOtherOrg, "theirs", "openai", "gpt-6-luna", "BLOCK", base, 99, histPII)
	h.add(histOrg, "ancient", "openai", "gpt-6-luna", "BLOCK", base.Add(-90*24*time.Hour), 99, histPII)
}

func TestStatsAndTimeseries_FromStoreAfterRestart(t *testing.T) {
	h := newFakeHistory()
	seedYesterday(h)
	// The restart case: nothing in memory.
	cfg := histConfig(h, events.NewRecentBuffer(1000))

	for _, tc := range []struct{ rng, bucket string }{{"24h", "hour"}, {"7d", "day"}, {"30d", "day"}, {"30d", "hour"}} {
		status, hdr, body := histGet(t, cfg, "/stats?range="+tc.rng)
		if status != http.StatusOK || hdr.Get(dataSourceHeader) != sourceDatabase {
			t.Fatalf("stats %s: status %d source %q", tc.rng, status, hdr.Get(dataSourceHeader))
		}
		var st histStats
		if err := json.Unmarshal(body, &st); err != nil {
			t.Fatal(err)
		}
		if st.Total != 10 || st.Blocked != 3 || st.Redacted != 1 || st.Warned != 0 || st.Passed != 6 {
			t.Errorf("stats %s = %+v, want 10 total, 3 blocked, 1 redacted, 6 passed", tc.rng, st)
		}
		// The enrichment describes the same ten requests.
		if st.Providers["openai"] != 6 || st.Providers["anthropic"] != 4 {
			t.Errorf("top_providers = %v", st.Providers)
		}
		if st.Types["secret"] != 3 || st.Types["pii"] != 2 || st.Cats["phone_tr"] != 2 {
			t.Errorf("finding counts = %v / %v", st.Types, st.Cats)
		}
		if want := (6*2 + 3*4 + 10) / 10.0; st.ScanAvg != want {
			t.Errorf("scanner_latency_avg_ms = %v, want %v", st.ScanAvg, want)
		}
		// 3 requests at 100%, 1 at 80% (two high findings), 6 at 0%.
		if st.AvgRisk == nil || *st.AvgRisk != 38 {
			t.Errorf("avg_input_risk_pct = %v, want 38", st.AvgRisk)
		}

		status, hdr, body = histGet(t, cfg, "/timeseries?range="+tc.rng+"&bucket="+tc.bucket)
		if status != http.StatusOK || hdr.Get(dataSourceHeader) != sourceDatabase {
			t.Fatalf("timeseries %s/%s: status %d source %q", tc.rng, tc.bucket, status, hdr.Get(dataSourceHeader))
		}
		var series histSeries
		if err := json.Unmarshal(body, &series); err != nil {
			t.Fatal(err)
		}
		var total, blocked, redacted int64
		var p95 float64
		for _, p := range series.Points {
			if p.T.UTC().Truncate(time.Hour) != p.T.UTC() {
				t.Errorf("bucket %v does not start on a UTC boundary", p.T)
			}
			total, blocked, redacted = total+p.Total, blocked+p.Blocked, redacted+p.Redacted
			if p.ScanP95 > p95 {
				p95 = p.ScanP95
			}
		}
		// The chart and the tile above it are the same requests.
		if total != st.Total || blocked != st.Blocked || redacted != st.Redacted {
			t.Errorf("timeseries %s/%s sums to %d/%d/%d, stats say %d/%d/%d", tc.rng, tc.bucket, total, blocked, redacted, st.Total, st.Blocked, st.Redacted)
		}
		if p95 == 0 {
			t.Errorf("timeseries %s/%s: scan_p95 is not set from the stored latencies", tc.rng, tc.bucket)
		}
	}

	_, hdr, body := histGet(t, cfg, "/findings/breakdown?range=7d")
	var bd struct {
		ByType     map[string]int64            `json:"by_type"`
		BySeverity map[string]int64            `json:"by_severity"`
		TypeByCat  map[string]map[string]int64 `json:"type_by_category"`
	}
	_ = json.Unmarshal(body, &bd)
	if hdr.Get(dataSourceHeader) != sourceDatabase || bd.ByType["pii"] != 2 || bd.BySeverity["critical"] != 3 || bd.TypeByCat["secret"]["aws_key"] != 3 {
		t.Errorf("breakdown = %s", body)
	}

	_, hdr, body = histGet(t, cfg, "/stats/models?range=7d")
	var ms struct {
		ByModel  map[string]int64 `json:"by_model"`
		ByFamily map[string]int64 `json:"by_family"`
	}
	_ = json.Unmarshal(body, &ms)
	if hdr.Get(dataSourceHeader) != sourceDatabase || ms.ByModel["gpt-6-luna"] != 6 || ms.ByModel["claude-haiku-4-5"] != 4 || ms.ByFamily["gpt-6-luna-family"] != 6 {
		t.Errorf("model stats = %s", body)
	}

	_, hdr, body = histGet(t, cfg, "/providers")
	if hdr.Get(dataSourceHeader) != sourceDatabase || !strings.Contains(string(body), `"gpt-6-luna"`) {
		t.Errorf("providers = %s", body)
	}
}

// An empty window is an answer: the buffer is not consulted to fill it.
func TestStats_EmptyStoreIsAnAnswer(t *testing.T) {
	ring := events.NewRecentBuffer(10)
	ring.Add(ringEvent("live", "", "BLOCK", time.Now().UTC()))
	cfg := histConfig(newFakeHistory(), ring)
	cfg.Metrics.TotalRequests.Store(41)
	_, hdr, body := histGet(t, cfg, "/stats?range=24h")
	var st histStats
	_ = json.Unmarshal(body, &st)
	if hdr.Get(dataSourceHeader) != sourceDatabase || st.Total != 0 || len(st.Providers) != 0 {
		t.Errorf("stats = %s", body)
	}
}

func TestHistory_NoDoubleCounting(t *testing.T) {
	h := newFakeHistory()
	now := time.Now().UTC()
	ring := events.NewRecentBuffer(100)
	// Five requests, all flushed to the database and all still in the buffer.
	for i := 0; i < 5; i++ {
		id := "req-" + string(rune('a'+i))
		at := now.Add(-time.Duration(i+1) * time.Minute)
		h.add(histOrg, id, "openai", "gpt-ring", "BLOCK", at, 7, histPII)
		ev := ringEvent(id, "", "BLOCK", at)
		ev.Findings = []scanner.Finding{histPII}
		ring.Add(ev)
	}
	cfg := histConfig(h, ring)
	cfg.Metrics.TotalRequests.Store(5)
	cfg.Metrics.Blocked.Store(5)

	_, _, body := histGet(t, cfg, "/stats?range=24h")
	var st histStats
	_ = json.Unmarshal(body, &st)
	if st.Total != 5 || st.Blocked != 5 || st.Providers["openai"] != 5 || st.Types["pii"] != 5 {
		t.Errorf("stats = %s, want each request counted once", body)
	}
	_, _, body = histGet(t, cfg, "/timeseries?range=24h&bucket=hour")
	var series histSeries
	_ = json.Unmarshal(body, &series)
	var total int64
	for _, p := range series.Points {
		total += p.Total
	}
	if total != 5 {
		t.Errorf("timeseries total = %d, want 5", total)
	}
	_, _, body = histGet(t, cfg, "/stats/models?range=24h")
	if !strings.Contains(string(body), `"gpt-ring":5`) {
		t.Errorf("model stats = %s", body)
	}
	_, _, body = histGet(t, cfg, "/events/export?format=json&range=24h")
	var exp struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(body, &exp)
	if exp.Total != 5 {
		t.Errorf("export total = %d, want 5", exp.Total)
	}
}

func TestHistory_StoreUnreachableFallsBackToRing(t *testing.T) {
	h := newFakeHistory()
	seedYesterday(h)
	h.err = errors.New("dial tcp 192.0.2.1:5432: connect: connection refused")
	now := time.Now().UTC()
	ring := events.NewRecentBuffer(100)
	ev := ringEvent("live-1", "", "BLOCK", now.Add(-time.Minute))
	ev.Findings = []scanner.Finding{histPII}
	ring.Add(ev)
	ring.Add(ringEvent("live-2", "", "PASS", now.Add(-2*time.Minute)))
	cfg := histConfig(h, ring)
	cfg.Metrics.TotalRequests.Store(2)
	cfg.Metrics.Blocked.Store(1)

	for _, path := range []string{
		"/stats?range=24h", "/timeseries?range=24h&bucket=hour", "/timeseries?range=7d&bucket=day",
		"/findings/breakdown", "/stats/models", "/providers", "/events/export?format=json", "/events/export",
		"/events/live-1",
	} {
		status, hdr, body := histGet(t, cfg, path)
		if status != http.StatusOK {
			t.Errorf("%s: status %d with the database down, want 200 from memory; body %s", path, status, body)
		}
		if got := hdr.Get(dataSourceHeader); got != sourceMemory {
			t.Errorf("%s: source = %q, want %q", path, got, sourceMemory)
		}
	}

	_, _, body := histGet(t, cfg, "/stats?range=24h")
	var st histStats
	_ = json.Unmarshal(body, &st)
	if st.Total != 2 || st.Blocked != 1 || st.Types["pii"] != 1 || st.AvgRisk == nil || *st.AvgRisk != 70 {
		t.Errorf("stats from memory = %s", body)
	}
	_, _, body = histGet(t, cfg, "/timeseries?range=24h&bucket=hour")
	var series histSeries
	_ = json.Unmarshal(body, &series)
	var total int64
	for _, p := range series.Points {
		total += p.Total
	}
	if total != 2 {
		t.Errorf("timeseries from memory totals %d, want 2", total)
	}
}

// Without a database nothing changes: the no-op store has no history.
func TestHistory_NoDatabaseStaysOnRing(t *testing.T) {
	ring := events.NewRecentBuffer(10)
	ring.Add(ringEvent("live-1", "", "PASS", time.Now().UTC()))
	for name, cfg := range map[string]Config{
		"no url":       {AdminKey: "k", Recent: ring, DefaultOrgID: histOrg, Store: newFakeHistory()},
		"no-op store":  {AdminKey: "k", Recent: ring, DefaultOrgID: histOrg, DatabaseURL: "postgres://example.invalid/x", Store: store.NewNoopStoreSilent()},
		"no org":       {AdminKey: "k", Recent: ring, DatabaseURL: "postgres://example.invalid/x", Store: newFakeHistory()},
		"nil store":    {AdminKey: "k", Recent: ring, DefaultOrgID: histOrg, DatabaseURL: "postgres://example.invalid/x"},
		"other org id": {AdminKey: "k", Recent: ring, DefaultOrgID: histOtherOrg, Store: store.NewNoopStoreSilent()},
	} {
		if cfg.history() != nil {
			t.Errorf("%s: a request history was found", name)
		}
		for _, path := range []string{"/stats", "/timeseries", "/events/live-1", "/stats/models"} {
			if status, hdr, _ := histGet(t, cfg, path); status != http.StatusOK || hdr.Get(dataSourceHeader) != sourceMemory {
				t.Errorf("%s %s: status %d source %q", name, path, status, hdr.Get(dataSourceHeader))
			}
		}
	}
}

func TestExport_FromStore(t *testing.T) {
	h := newFakeHistory()
	seedYesterday(h)
	ring := events.NewRecentBuffer(10)
	ring.Add(ringEvent("live-only", "", "BLOCK", time.Now().UTC()))
	cfg := histConfig(h, ring)

	status, hdr, body := histGet(t, cfg, "/events/export?range=7d&action=BLOCK,REDACT&finding_type=pii&q=phone&severity=high&category=phone&technique=LLM01")
	if status != http.StatusOK || hdr.Get(dataSourceHeader) != sourceDatabase {
		t.Fatalf("status %d source %q", status, hdr.Get(dataSourceHeader))
	}
	if hdr.Get(exportTruncatedHeader) != "" {
		t.Errorf("a complete export is marked truncated")
	}
	// The filters of the event list reach the query.
	p := h.lastExport
	if p.Action != "BLOCK,REDACT" || p.FindingType != "pii" || p.Q != "phone" || p.Severity != "high" || p.Category != "phone" || p.Technique != "LLM01" {
		t.Errorf("export filters = %+v", p)
	}
	if d := p.Until.Sub(p.Since); d < 7*24*time.Hour-time.Minute || d > 7*24*time.Hour+time.Minute {
		t.Errorf("export window = %v, want 7 days", d)
	}
	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// Header, three blocked and one redacted; nothing of the other
	// organisation, nothing that is only in memory.
	if len(records) != 5 {
		t.Fatalf("csv has %d lines, want 5:\n%s", len(records), body)
	}
	for _, rec := range records[1:] {
		if rec[0] == "live-only" || rec[0] == "theirs" {
			t.Errorf("row %v does not belong in the export", rec)
		}
		if rec[0] == "redact-a" && (rec[4] != "REDACT" || rec[5] != "request_scanned" || rec[6] != "2" || rec[7] != "10.00" || rec[8] != "80") {
			t.Errorf("redacted row = %v", rec)
		}
	}

	_, _, body = histGet(t, cfg, "/events/export?format=json&request_id=BLOCK-")
	var exp struct {
		Events []events.EventJSON `json:"events"`
		Total  int                `json:"total"`
	}
	if err := json.Unmarshal(body, &exp); err != nil {
		t.Fatal(err)
	}
	if exp.Total != 3 || len(exp.Events) != 3 || exp.Events[0].Action != "BLOCK" || exp.Events[0].OutputRisk != nil || exp.Events[0].InputRiskPct != 100 {
		t.Errorf("json export = %s", body)
	}

	h.exportMax = 4
	_, hdr, body = histGet(t, cfg, "/events/export?range=7d")
	if hdr.Get(exportTruncatedHeader) != "true" {
		t.Errorf("an export cut at the row limit must say so")
	}
	if n := strings.Count(strings.TrimSpace(string(body)), "\n"); n != 4 {
		t.Errorf("truncated export has %d rows, want 4", n)
	}
}

func TestFindingBreakdown(t *testing.T) {
	byType, byCat, bySev, typeByCat := findingBreakdown([]store.FindingCount{
		{Type: "pii", Category: "email", Severity: "high", Count: 3},
		{Type: "pii", Category: "email", Severity: "low", Count: 1},
		{Type: "pii", Category: "", Severity: "", Count: 2},
		{Type: "", Category: "orphan", Severity: "medium", Count: 5},
	})
	if byType["pii"] != 6 || len(byType) != 1 {
		t.Errorf("by_type = %v", byType)
	}
	if byCat["email"] != 4 || byCat["orphan"] != 5 || len(byCat) != 2 {
		t.Errorf("by_category = %v", byCat)
	}
	if bySev["high"] != 3 || bySev["low"] != 1 || bySev["medium"] != 5 || len(bySev) != 3 {
		t.Errorf("by_severity = %v", bySev)
	}
	if typeByCat["pii"]["email"] != 4 || typeByCat[""]["orphan"] != 5 {
		t.Errorf("type_by_category = %v", typeByCat)
	}
}

func TestLoadRequestStats_RiskLeftOutWhenProfilesOverflow(t *testing.T) {
	h := &overflowHistory{fakeHistory: newFakeHistory()}
	seedYesterday(h.fakeHistory)
	_, _, body := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/stats?range=7d")
	if strings.Contains(string(body), "avg_input_risk_pct") {
		t.Errorf("an average that could not be worked out must be left out: %s", body)
	}
	if !strings.Contains(string(body), `"total_requests":10`) {
		t.Errorf("the counts are still the database's: %s", body)
	}
}

type overflowHistory struct{ *fakeHistory }

func (o *overflowHistory) RiskProfiles(context.Context, string, time.Time, time.Time) ([]store.RiskProfile, error) {
	return nil, store.ErrTooManyGroups
}

// A row written by this version says what the proxy knew when the request
// was over; the detail shows it from the row, not from the policy loaded now.
func TestEventDetail_StoredRecordSpeaksForItself(t *testing.T) {
	h := newFakeHistory()
	at := time.Now().UTC().Add(-3 * time.Hour)
	in, out, cost := 120, 45, 0.00231
	outFindings, _ := json.Marshal([]scanner.Finding{histPII})
	outRisk, _ := json.Marshal(scanner.CalculateRisk([]scanner.Finding{histPII}))
	h.rows = append(h.rows, fakeRow{org: histOrg, rec: store.RequestRecord{
		RequestID: "req-full", Provider: "openai", Model: "gpt-4o", ActionTaken: "pass",
		Findings: []byte("[]"), CreatedAt: at, ScanLatencyMs: 0.35, TotalLatencyMs: 812.5,
		InputTokens: &in, OutputTokens: &out, CostUSD: &cost, CacheStatus: "miss",
		OutputFindings: outFindings, OutputAction: "redact", OutputRisk: outRisk,
		PolicyName: "bank-policy", PolicyVersion: "7",
	}})
	status, _, body := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/req-full")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body %s", status, body)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]interface{}{
		"input_tokens": 120.0, "output_tokens": 45.0, "cost_usd": 0.00231, "cache_status": "miss",
		"policy_name": "bank-policy", "policy_version": "7", "output_action": "REDACT",
		"scan_latency_ms": 0.35, "total_latency_ms": 812.5,
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
	risk, _ := got["output_risk"].(map[string]interface{})
	if risk == nil || risk["level"] != scanner.CalculateRisk([]scanner.Finding{histPII}).Level {
		t.Errorf("output_risk = %v, want the stored one", got["output_risk"])
	}
	list, _ := got["output_findings"].([]interface{})
	if len(list) != 1 || list[0].(map[string]interface{})["match"] != histPII.Match {
		t.Errorf("output_findings = %v, want the stored, masked finding", got["output_findings"])
	}
}

// What a row does not hold is absent from the detail and from the export:
// not null, and not a zero that would read as a count or a price.
func TestStoredRecord_UnknownIsAbsent(t *testing.T) {
	h := newFakeHistory()
	at := time.Now().UTC().Add(-3 * time.Hour)
	// A row of an older version: no tokens, no cost, no latency, no policy.
	h.rows = append(h.rows, fakeRow{org: histOrg, rec: store.RequestRecord{
		RequestID: "req-poor", Provider: "openai", Model: "gpt-4o", ActionTaken: "PASS",
		Findings: []byte("[]"), CreatedAt: at,
	}})
	cfg := histConfig(h, events.NewRecentBuffer(10))
	for _, path := range []string{"/events/req-poor", "/events/export?format=json&range=24h"} {
		status, _, body := histGet(t, cfg, path)
		if status != http.StatusOK {
			t.Fatalf("%s: status = %d, body %s", path, status, body)
		}
		if !strings.Contains(string(body), "req-poor") {
			t.Fatalf("%s: the row is not in the answer: %s", path, body)
		}
		for _, k := range []string{"input_tokens", "output_tokens", "cost_usd", "cache_status", "policy_name",
			"policy_version", "output_risk", "output_action", "output_findings", "scan_latency_ms", "total_latency_ms", "null"} {
			if strings.Contains(string(body), k) {
				t.Errorf("%s: %s is not known for this row and must be absent: %s", path, k, body)
			}
		}
	}
}

// The export carries the stored tokens and cost of a request that has them.
func TestExport_StoredTokensAndCost(t *testing.T) {
	h := newFakeHistory()
	in, out, cost := 10, 0, 0.0
	h.rows = append(h.rows, fakeRow{org: histOrg, rec: store.RequestRecord{
		RequestID: "req-hit", Provider: "openai", Model: "gpt-4o", ActionTaken: "pass",
		Findings: []byte("[]"), CreatedAt: time.Now().UTC().Add(-time.Hour),
		InputTokens: &in, OutputTokens: &out, CostUSD: &cost, CacheStatus: "hit",
	}})
	_, _, body := histGet(t, histConfig(h, events.NewRecentBuffer(10)), "/events/export?format=json&range=24h")
	var doc struct {
		Events []map[string]interface{} `json:"events"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || len(doc.Events) != 1 {
		t.Fatalf("export = %s (%v)", body, err)
	}
	e := doc.Events[0]
	// A known zero is said: an output count of 0 beside a known input
	// count, and the cost of a cache hit.
	if e["input_tokens"] != 10.0 || e["output_tokens"] != 0.0 || e["cost_usd"] != 0.0 || e["cache_status"] != "hit" {
		t.Errorf("export row = %v", e)
	}
}
