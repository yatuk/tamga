package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/store"
)

// TestMetricsHistograms_OK verifies GET /api/v1/metrics/histograms returns
// 200 JSON with histogram bucket data.
func TestMetricsHistograms_OK(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/metrics/histograms", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("want application/json, got %q", ct)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	hists, ok := out["histograms"].([]interface{})
	if !ok {
		t.Fatalf("histograms not a list: %T", out["histograms"])
	}
	if len(hists) == 0 {
		t.Fatal("expected at least one histogram entry")
	}
	// Each entry should have name, count, sum, buckets.
	for _, h := range hists {
		hm, ok := h.(map[string]interface{})
		if !ok {
			t.Fatalf("histogram entry not a map: %T", h)
		}
		if _, ok := hm["name"].(string); !ok {
			t.Fatal("histogram missing 'name'")
		}
		if _, ok := hm["count"]; !ok {
			t.Fatal("histogram missing 'count'")
		}
		if _, ok := hm["sum"]; !ok {
			t.Fatal("histogram missing 'sum'")
		}
		if _, ok := hm["buckets"].([]interface{}); !ok {
			t.Fatal("histogram missing 'buckets'")
		}
	}
}

// TestMetricsHistograms_Unauthorized verifies the endpoint requires auth.
func TestMetricsHistograms_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:     "secret-key",
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/metrics/histograms")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

// TestMetricsHistograms_NoAuthInDevMode verifies the endpoint is accessible
// without auth when no admin key or JWT secret is configured.
func TestMetricsHistograms_NoAuthInDevMode(t *testing.T) {
	cfg := Config{
		AdminKey:     "",
		JWTSecret:    "",
		DevMode:      true, // explicit dev mode bypass
		Metrics:      &events.Metrics{},
		Started:      time.Now(),
		PolicyPath:   "/tmp/p.yaml",
		ScannerCount: 1,
		Store:        store.NewNoopStoreSilent(),
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/metrics/histograms")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 in dev mode, got %d: %s", resp.StatusCode, b)
	}
}

// ── Observe / Histogram tests ───────────────────────────────────────────

func TestObserveScan_RecordsValue(t *testing.T) {
	// ObserveScan should not panic and should record a value.
	ObserveScan(42.5)
	ObserveScan(0.0)
	ObserveScan(1000.0)
}

func TestObserveTotal_RecordsValue(t *testing.T) {
	ObserveTotal(150.0)
	ObserveTotal(0.0)
	ObserveTotal(5000.0)
}

func TestObserveScanner_RecordsByScanner(t *testing.T) {
	ObserveScanner("pii", 5.2)
	ObserveScanner("secrets", 12.8)
	ObserveScanner("injection", 1.0)
	ObserveScanner("", 0.0) // empty scanner name — still should not panic
}

func TestObserveMetrics_NilSafe(t *testing.T) {
	for i := 0; i < 10; i++ {
		ObserveScan(float64(i) * 10.0)
		ObserveTotal(float64(i) * 20.0)
		ObserveScanner("nil-safe-scanner", float64(i))
	}
}

func TestObserveMetrics_MultipleObservations(t *testing.T) {
	// Multiple observations of the same histogram should accumulate.
	for i := 0; i < 100; i++ {
		ObserveScan(50.0)
	}
}

// ── labelPairsToMap tests ───────────────────────────────────────────────

func TestLabelPairsToMap_Standard(t *testing.T) {
	labels := []*dto.LabelPair{
		{Name: strPtr("scanner"), Value: strPtr("pii")},
		{Name: strPtr("status"), Value: strPtr("ok")},
	}
	out := labelPairsToMap(labels)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(out), out)
	}
	if out["scanner"] != "pii" {
		t.Fatalf("scanner: %q", out["scanner"])
	}
	if out["status"] != "ok" {
		t.Fatalf("status: %q", out["status"])
	}
}

func TestLabelPairsToMap_EmptyLabels(t *testing.T) {
	out := labelPairsToMap(nil)
	if out != nil {
		t.Fatalf("nil input should return nil, got %v", out)
	}
	out = labelPairsToMap([]*dto.LabelPair{})
	if out != nil {
		t.Fatalf("empty slice should return nil, got %v", out)
	}
}

func TestLabelPairsToMap_DuplicateKeys(t *testing.T) {
	labels := []*dto.LabelPair{
		{Name: strPtr("key"), Value: strPtr("first")},
		{Name: strPtr("key"), Value: strPtr("second")},
	}
	out := labelPairsToMap(labels)
	if len(out) != 1 {
		t.Fatalf("duplicate keys: expected 1 entry, got %d: %v", len(out), out)
	}
	// Last value wins (Go map overwrite semantics).
	if out["key"] != "second" {
		t.Fatalf("duplicate last value: %q", out["key"])
	}
}

func TestLabelPairsToMap_SpecialCharacters(t *testing.T) {
	labels := []*dto.LabelPair{
		{Name: strPtr("emoji"), Value: strPtr("hello world")},
		{Name: strPtr("slash"), Value: strPtr("a/b")},
		{Name: strPtr("unicode"), Value: strPtr("cafe")},
	}
	out := labelPairsToMap(labels)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(out), out)
	}
	if out["emoji"] != "hello world" {
		t.Fatalf("emoji: %q", out["emoji"])
	}
	if out["slash"] != "a/b" {
		t.Fatalf("slash: %q", out["slash"])
	}
	if out["unicode"] != "cafe" {
		t.Fatalf("unicode: %q", out["unicode"])
	}
}

func TestLabelPairsToMap_EmptyKey(t *testing.T) {
	labels := []*dto.LabelPair{
		{Name: strPtr(""), Value: strPtr("value")},
	}
	out := labelPairsToMap(labels)
	if out != nil {
		t.Fatalf("empty key should be skipped, returning nil: got %v", out)
	}
}

func TestLabelPairsToMap_EmptyValue(t *testing.T) {
	labels := []*dto.LabelPair{
		{Name: strPtr("key"), Value: strPtr("")},
	}
	out := labelPairsToMap(labels)
	if len(out) != 1 {
		t.Fatalf("empty value is valid: expected 1 entry, got %d: %v", len(out), out)
	}
	if out["key"] != "" {
		t.Fatalf("empty value: %q", out["key"])
	}
}

// strPtr helper for creating *string from a string literal.
func strPtr(s string) *string { return &s }
