package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReports_OwaspPdf_NoAnalyzerURL(t *testing.T) {
	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: "", // not configured
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/owasp/pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestReports_IncidentPdf_NoAnalyzerURL(t *testing.T) {
	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: "", // not configured
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/incident/pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestReports_OwaspPdf_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: "http://analyzer:8000",
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	// No admin key header — should be 401.
	resp, err := http.Get(ts.URL + "/api/v1/reports/owasp/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestReports_OwaspPdf_ProxiesToAnalyzer(t *testing.T) {
	// Start a fake analyzer that returns a dummy PDF.
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/owasp/pdf" {
			http.NotFound(w, r)
			return
		}
		// Verify query params are forwarded.
		if r.URL.Query().Get("range") != "7d" {
			t.Errorf("expected range=7d, got %s", r.URL.Query().Get("range"))
		}
		if r.URL.Query().Get("org_id") != "test-org" {
			t.Errorf("expected org_id=test-org, got %s", r.URL.Query().Get("org_id"))
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename=tamga-owasp-report.pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("%PDF-1.4 fake pdf content"))
	}))
	defer fakeAnalyzer.Close()

	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: fakeAnalyzer.URL,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/owasp/pdf?range=7d&org_id=test-org", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/pdf") {
		t.Fatalf("expected Content-Type application/pdf, got %s", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd == "" {
		t.Fatal("expected Content-Disposition header")
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "%PDF-1.4") {
		t.Fatalf("expected PDF content, got %s", string(body))
	}
}

func TestReports_IncidentPdf_ProxiesToAnalyzer(t *testing.T) {
	// Start a fake analyzer that returns a dummy incident PDF.
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/incident/pdf" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("total_requests") != "100" {
			t.Errorf("expected total_requests=100, got %s", r.URL.Query().Get("total_requests"))
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename=tamga-incident-report.pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("%PDF-1.4 incident summary"))
	}))
	defer fakeAnalyzer.Close()

	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: fakeAnalyzer.URL,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/incident/pdf?total_requests=100&blocked=10&redacted=5&warned=2&period_hours=24", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "%PDF-1.4") {
		t.Fatalf("expected PDF content, got %s", string(body))
	}
}

func TestReports_OwaspPdf_AnalyzerReturns501(t *testing.T) {
	// When the analyzer returns 501 (ReportLab not installed), the proxy
	// must forward the status code and JSON error body — not swallow it
	// as a generic 503 Service Unavailable.
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/owasp/pdf" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(`{"error":"PDF generation unavailable: ReportLab not installed","available":false}`))
	}))
	defer fakeAnalyzer.Close()

	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: fakeAnalyzer.URL,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/owasp/pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ReportLab not installed") {
		t.Fatalf("expected error about ReportLab, got %s", string(body))
	}
	if !strings.Contains(string(body), `"available":false`) {
		t.Fatalf("expected available:false in body, got %s", string(body))
	}
}

func TestReports_IncidentPdf_AnalyzerReturns501(t *testing.T) {
	// Same as OWASP variant — verify incident endpoint also forwards 501.
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/incident/pdf" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(`{"error":"PDF generation unavailable: ReportLab not installed","available":false}`))
	}))
	defer fakeAnalyzer.Close()

	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: fakeAnalyzer.URL,
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/incident/pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ReportLab not installed") {
		t.Fatalf("expected error about ReportLab, got %s", string(body))
	}
}

func TestReports_AnalyzerDown(t *testing.T) {
	// Use an unreachable address to simulate analyzer being down.
	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: "http://127.0.0.1:19999", // nothing listening here
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/reports/owasp/pdf", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestBuildAnalyzerURL(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		path     string
		rawQuery string
		want     string
		wantErr  bool
	}{
		{
			name:     "basic",
			base:     "http://analyzer:8000",
			path:     "/api/v1/reports/owasp/pdf",
			rawQuery: "",
			want:     "http://analyzer:8000/api/v1/reports/owasp/pdf",
		},
		{
			name:     "with query params",
			base:     "http://analyzer:8000",
			path:     "/api/v1/reports/owasp/pdf",
			rawQuery: "range=7d&org_id=test",
			want:     "http://analyzer:8000/api/v1/reports/owasp/pdf?range=7d&org_id=test",
		},
		{
			name:     "base with trailing slash",
			base:     "http://analyzer:8000/",
			path:     "/api/v1/reports/incident/pdf",
			rawQuery: "total_requests=100",
			want:     "http://analyzer:8000/api/v1/reports/incident/pdf?total_requests=100",
		},
		{
			name:     "path without leading slash",
			base:     "http://analyzer:8000",
			path:     "api/v1/reports/owasp/pdf",
			rawQuery: "",
			want:     "http://analyzer:8000/api/v1/reports/owasp/pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAnalyzerURL(tt.base, tt.path, tt.rawQuery)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildAnalyzerURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("buildAnalyzerURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
