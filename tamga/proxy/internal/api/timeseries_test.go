package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/store"
)

func TestTimeseries_EmptyRecent(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Recent:       events.NewRecentBuffer(10),
		DefaultOrgID: "org-1",
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/timeseries?range=7d&bucket=day", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["range"] != "7d" {
		t.Errorf("expected range '7d', got %v", body["range"])
	}
	points, ok := body["points"].([]interface{})
	if !ok {
		t.Fatalf("expected points array")
	}
	// Should have points for the 7-day window (1 per day)
	if len(points) == 0 {
		t.Error("expected at least some points in timeseries")
	}
}

func TestTimeseries_InvalidBucket(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Recent:       events.NewRecentBuffer(10),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/timeseries?bucket=century", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid bucket, got %d", resp.StatusCode)
	}
}

func TestTimeseries_WithData(t *testing.T) {
	rb := events.NewRecentBuffer(50)
	now := time.Now().UTC()
	rb.Add(events.Event{
		RequestID:     "ts-1",
		EventType:     "request_scanned",
		Action:        "BLOCK",
		Provider:      "openai",
		Model:         "gpt-4o",
		ScanLatencyMs: 5.0,
		Timestamp:     now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID:     "ts-2",
		EventType:     "request_blocked",
		Action:        "BLOCK",
		Provider:      "anthropic",
		Model:         "claude-3",
		ScanLatencyMs: 8.0,
		Timestamp:     now.Add(-30 * time.Minute),
	})
	rb.Add(events.Event{
		RequestID:     "ts-3",
		EventType:     "request_scanned",
		Action:        "REDACT",
		Provider:      "openai",
		Model:         "gpt-4o",
		ScanLatencyMs: 3.0,
		Timestamp:     now.Add(-2 * time.Hour),
	})

	cfg := Config{
		AdminKey:     "test-key",
		Recent:       rb,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/timeseries?range=24h&bucket=hour", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	points := body["points"].([]interface{})
	if len(points) == 0 {
		t.Error("expected non-empty timeseries points")
	}
	// Verify at least one point has blocked/redacted > 0
	foundBlocked := false
	foundRedacted := false
	for _, p := range points {
		pt := p.(map[string]interface{})
		if blocked, ok := pt["blocked"].(float64); ok && blocked > 0 {
			foundBlocked = true
		}
		if redacted, ok := pt["redacted"].(float64); ok && redacted > 0 {
			foundRedacted = true
		}
	}
	if !foundBlocked {
		t.Error("expected at least one point with blocked > 0")
	}
	if !foundRedacted {
		t.Error("expected at least one point with redacted > 0")
	}
}

func TestBreakdown_EmptyRecent(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Recent:       events.NewRecentBuffer(10),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/findings/breakdown?range=7d", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	for _, k := range []string{"range", "by_type", "by_category", "by_severity", "type_by_category"} {
		if _, ok := body[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
}

func TestBreakdown_WithData(t *testing.T) {
	rb := events.NewRecentBuffer(50)
	now := time.Now().UTC()
	rb.Add(events.Event{
		RequestID: "bd-1",
		EventType: "request_scanned",
		Action:    "BLOCK",
		Provider:  "openai",
		Model:     "gpt-4o",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "email", Severity: "high"},
			{Type: "secret", Category: "api_key", Severity: "critical"},
		},
		Timestamp: now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID: "bd-2",
		EventType: "request_scanned",
		Action:    "REDACT",
		Provider:  "anthropic",
		Model:     "claude-3",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "credit_card", Severity: "high"},
		},
		Timestamp: now.Add(-30 * time.Minute),
	})

	cfg := Config{
		AdminKey:     "test-key",
		Recent:       rb,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/findings/breakdown?range=7d", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	byType := body["by_type"].(map[string]interface{})
	// pii should appear twice
	if v, ok := byType["pii"].(float64); !ok || v != 2 {
		t.Errorf("expected pii count 2, got %v", byType["pii"])
	}
	if v, ok := byType["secret"].(float64); !ok || v != 1 {
		t.Errorf("expected secret count 1, got %v", byType["secret"])
	}
}

func TestModelStats_EmptyRecent(t *testing.T) {
	cfg := Config{
		AdminKey:     "test-key",
		Recent:       events.NewRecentBuffer(10),
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats/models?range=7d", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["range"] != "7d" {
		t.Errorf("expected range '7d', got %v", body["range"])
	}
	if _, ok := body["by_model"]; !ok {
		t.Error("missing by_model")
	}
	if _, ok := body["by_family"]; !ok {
		t.Error("missing by_family")
	}
}

func TestModelStats_WithData(t *testing.T) {
	rb := events.NewRecentBuffer(50)
	now := time.Now().UTC()
	rb.Add(events.Event{
		RequestID:   "ms-1",
		EventType:   "request_scanned",
		Action:      "PASS",
		Provider:    "openai",
		Model:       "gpt-4o",
		ModelFamily: "gpt-4o",
		Timestamp:   now.Add(-1 * time.Hour),
	})
	rb.Add(events.Event{
		RequestID:   "ms-2",
		EventType:   "request_scanned",
		Action:      "PASS",
		Provider:    "anthropic",
		Model:       "claude-3-5-sonnet",
		ModelFamily: "claude-3-5",
		Timestamp:   now.Add(-30 * time.Minute),
	})
	rb.Add(events.Event{
		RequestID:   "ms-3",
		EventType:   "request_scanned",
		Action:      "PASS",
		Provider:    "openai",
		Model:       "gpt-4o",
		ModelFamily: "gpt-4o",
		Timestamp:   now.Add(-2 * time.Hour),
	})

	cfg := Config{
		AdminKey:     "test-key",
		Recent:       rb,
		DefaultOrgID: "org-1",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/stats/models?range=7d", nil)
	adminHeaders(cfg.AdminKey)(req)
	resp, _ := http.DefaultClient.Do(req)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	byModel := body["by_model"].(map[string]interface{})
	if v, ok := byModel["gpt-4o"].(float64); !ok || v != 2 {
		t.Errorf("expected gpt-4o count 2, got %v", byModel["gpt-4o"])
	}
	if v, ok := byModel["claude-3-5-sonnet"].(float64); !ok || v != 1 {
		t.Errorf("expected claude-3-5-sonnet count 1, got %v", byModel["claude-3-5-sonnet"])
	}
	byFamily := body["by_family"].(map[string]interface{})
	if v, ok := byFamily["gpt-4o"].(float64); !ok || v != 2 {
		t.Errorf("expected gpt-4o family count 2, got %v", byFamily["gpt-4o"])
	}
}

// ── rangeMillis pure function test ────────────────────────────────────────

func TestRangeMillis(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want int64
	}{
		{"1h", "1h", int64(time.Hour / time.Millisecond)},
		{"24h", "24h", int64(24 * time.Hour / time.Millisecond)},
		{"30d", "30d", int64(30 * 24 * time.Hour / time.Millisecond)},
		{"7d", "7d", int64(7 * 24 * time.Hour / time.Millisecond)},
		{"default empty", "", int64(7 * 24 * time.Hour / time.Millisecond)},
		{"unknown key", "bogus", int64(7 * 24 * time.Hour / time.Millisecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rangeMillis(tt.arg)
			if got != tt.want {
				t.Errorf("rangeMillis(%q) = %d; want %d", tt.arg, got, tt.want)
			}
		})
	}
}

func TestBucketMillis(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want int64
	}{
		{"minute", "minute", int64(time.Minute / time.Millisecond)},
		{"hour", "hour", int64(time.Hour / time.Millisecond)},
		{"day", "day", int64(24 * time.Hour / time.Millisecond)},
		{"unknown", "century", 0},
		{"empty", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bucketMillis(tt.arg)
			if got != tt.want {
				t.Errorf("bucketMillis(%q) = %d; want %d", tt.arg, got, tt.want)
			}
		})
	}
}

func TestPercentile(t *testing.T) {
	tests := []struct {
		name string
		xs   []float64
		p    float64
		want float64
	}{
		{"empty", nil, 0.5, 0},
		{"single", []float64{10}, 0.5, 10},
		{"p50 of 3", []float64{1, 5, 9}, 0.5, 5},
		{"p90 of 3", []float64{1, 5, 9}, 0.9, 5},
		{"p99 of 3", []float64{1, 5, 9}, 0.99, 5},
		{"p0 of 3", []float64{1, 5, 9}, 0, 1},
		{"five_items_p50", []float64{3, 1, 4, 1, 5}, 0.5, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := percentile(tt.xs, tt.p)
			if got != tt.want {
				t.Errorf("percentile(%v, %v) = %v; want %v", tt.xs, tt.p, got, tt.want)
			}
		})
	}
}
