package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// reportGet asks the management API on a test server for a report, with the
// admin key and any extra headers.
func reportGet(t *testing.T, cfg Config, pathAndQuery string, header http.Header) (*http.Response, []byte) {
	t.Helper()
	cfg.AdminKey = "secret-key"
	cfg.Started = time.Now()
	ts := httptest.NewServer(testMux(cfg))
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+pathAndQuery, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("X-Tamga-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

// reportErrorBody decodes the JSON body of a failed report.
func reportErrorBody(t *testing.T, resp *http.Response, body []byte) map[string]string {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("error Content-Type = %q, want application/json", ct)
	}
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("error body is not a JSON object of strings: %v: %s", err, body)
	}
	if m["error"] == "" {
		t.Fatalf("error body has no \"error\" sentence for the console: %s", body)
	}
	return m
}

func TestReports_NoAnalyzerURL(t *testing.T) {
	for _, path := range []string{"/api/v1/reports/owasp/pdf", "/api/v1/reports/incident/pdf"} {
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: ""}, path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", path, resp.StatusCode)
		}
		if m := reportErrorBody(t, resp, body); m["code"] != codeAnalyzerNotConfigured {
			t.Fatalf("%s: code = %q", path, m["code"])
		}
	}
}

func TestReports_BadAnalyzerURL(t *testing.T) {
	for _, base := range []string{"analyzer:8444", "ftp://analyzer", "http://", "http://user:pw@analyzer:8444", "http://analyzer:8444?x=1"} {
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: base}, "/api/v1/reports/owasp/pdf", nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", base, resp.StatusCode)
		}
		if m := reportErrorBody(t, resp, body); m["code"] != codeAnalyzerNotConfigured {
			t.Fatalf("%s: code = %q", base, m["code"])
		}
		if strings.Contains(string(body), "pw") {
			t.Fatalf("%s: body repeats the configured URL: %s", base, body)
		}
	}
}

func TestReports_OwaspPdf_Unauthorized(t *testing.T) {
	cfg := Config{
		AdminKey:        "secret-key",
		Started:         time.Now(),
		AnalyzerHTTPURL: "http://analyzer:8444",
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
	pdf := "%PDF-1.4 fake pdf content"
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/owasp/pdf" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.RawQuery; got != "range=7d" {
			t.Errorf("query = %q, want only range=7d", got)
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename=from-the-analyzer.pdf")
		w.Header().Set("Set-Cookie", "a=b")
		_, _ = w.Write([]byte(pdf))
	}))
	defer fakeAnalyzer.Close()

	// org_id and anything unknown stay behind: the analyzer does not scope by
	// them, and a caller must not be the one to name a tenant.
	resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL},
		"/api/v1/reports/owasp/pdf?range=7d&org_id=test-org&unknown=1", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "attachment; filename=tamga-owasp-report.pdf" {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if resp.Header.Get("Set-Cookie") != "" {
		t.Fatal("a header of the analyzer's answer was passed on")
	}
	if string(body) != pdf {
		t.Fatalf("body = %q, want the analyzer's bytes", body)
	}
}

func TestReports_IncidentPdf_ProxiesToAnalyzer(t *testing.T) {
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/incident/pdf" {
			http.NotFound(w, r)
			return
		}
		want := url.Values{
			"total_requests": {"100"}, "blocked": {"10"}, "redacted": {"5"},
			"warned": {"2"}, "period_hours": {"24"},
		}
		if got := r.URL.Query().Encode(); got != want.Encode() {
			t.Errorf("query = %q, want %q", got, want.Encode())
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 incident summary"))
	}))
	defer fakeAnalyzer.Close()

	resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL},
		"/api/v1/reports/incident/pdf?total_requests=100&blocked=10&redacted=5&warned=2&period_hours=24", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "attachment; filename=tamga-incident-report.pdf" {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	if !strings.HasPrefix(string(body), "%PDF-1.4") {
		t.Fatalf("expected PDF content, got %s", body)
	}
}

func TestReports_IncidentPdf_RejectsBadCounts(t *testing.T) {
	var hits atomic.Int32
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer fakeAnalyzer.Close()

	for _, q := range []string{"total_requests=abc", "blocked=-1", "period_hours=1.5", "warned=99999999999999999999"} {
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL}, "/api/v1/reports/incident/pdf?"+q, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", q, resp.StatusCode)
		}
		reportErrorBody(t, resp, body)
	}
	if hits.Load() != 0 {
		t.Fatalf("the analyzer was asked %d times for requests that are refused here", hits.Load())
	}
}

// The caller's request decides nothing about where the proxy connects or
// which path it asks for, and none of its headers travel on.
func TestReports_CallerCannotSteerTheRequest(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
	}))
	defer other.Close()

	type seen struct {
		path, rawQuery, host string
		header               http.Header
	}
	got := make(chan seen, 16)
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery, host: r.Host, header: r.Header.Clone()}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4"))
	}))
	defer fakeAnalyzer.Close()
	analyzerHost := strings.TrimPrefix(fakeAnalyzer.URL, "http://")

	queries := []string{
		"range=" + url.QueryEscape("../../../health"),
		"range=" + url.QueryEscape("7d&total_requests=9"),
		"range=" + url.QueryEscape("7d#/api/v1/analyze"),
		"range=" + url.QueryEscape("7d\r\nX-Injected: 1"),
		"range=" + url.QueryEscape(other.URL),
		"path=/api/v1/analyze&url=" + url.QueryEscape(other.URL) + "&host=" + url.QueryEscape(strings.TrimPrefix(other.URL, "http://")),
		"range=7d&range=30d",
	}
	header := http.Header{
		"Authorization":     {"Bearer caller-token"},
		"Cookie":            {"session=abc"},
		"X-Forwarded-Host":  {strings.TrimPrefix(other.URL, "http://")},
		"X-Smuggled":        {"1"},
		"X-Tamga-Something": {"1"},
	}
	for _, q := range queries {
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL}, "/api/v1/reports/owasp/pdf?"+q, header)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", q, resp.StatusCode, body)
		}
		s := <-got
		if s.path != "/api/v1/reports/owasp/pdf" {
			t.Fatalf("%s: analyzer was asked for path %q", q, s.path)
		}
		if s.host != analyzerHost {
			t.Fatalf("%s: Host = %q, want %q", q, s.host, analyzerHost)
		}
		vals, err := url.ParseQuery(s.rawQuery)
		if err != nil {
			t.Fatalf("%s: forwarded query does not parse: %q", q, s.rawQuery)
		}
		for name := range vals {
			if name != "range" {
				t.Fatalf("%s: parameter %q was forwarded (query %q)", q, name, s.rawQuery)
			}
		}
		if len(vals["range"]) > 1 {
			t.Fatalf("%s: range forwarded %d times", q, len(vals["range"]))
		}
		for _, name := range []string{"Authorization", "Cookie", "X-Forwarded-Host", "X-Smuggled", "X-Tamga-Something", "X-Tamga-Admin-Key", "X-Injected"} {
			if s.header.Get(name) != "" {
				t.Fatalf("%s: header %s reached the analyzer", q, name)
			}
		}
	}
	if elsewhere.Load() != 0 {
		t.Fatalf("a request went to another host %d times", elsewhere.Load())
	}
}

// A redirect from the analyzer is an answer that is not a report. It is not
// followed, so a compromised or misrouted analyzer cannot send the proxy on.
func TestReports_RedirectIsNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 from elsewhere"))
	}))
	defer other.Close()
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/", http.StatusFound)
	}))
	defer fakeAnalyzer.Close()

	resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL}, "/api/v1/reports/owasp/pdf", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", resp.StatusCode, body)
	}
	if m := reportErrorBody(t, resp, body); m["code"] != codeAnalyzerError {
		t.Fatalf("code = %q", m["code"])
	}
	if resp.Header.Get("Location") != "" {
		t.Fatal("the redirect was passed on to the caller")
	}
	if elsewhere.Load() != 0 {
		t.Fatal("the redirect was followed")
	}
}

func TestReports_AnalyzerReturns501(t *testing.T) {
	// When the analyzer returns 501 (ReportLab not installed), the proxy
	// must forward the status code and JSON error body — not swallow it
	// as a generic 503 Service Unavailable.
	fakeAnalyzer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`{"error":"PDF generation unavailable: ReportLab not installed","available":false}`))
	}))
	defer fakeAnalyzer.Close()

	for _, path := range []string{"/api/v1/reports/owasp/pdf", "/api/v1/reports/incident/pdf"} {
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL}, path, nil)
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("%s: expected 501, got %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("%s: expected Content-Type application/json, got %s", path, ct)
		}
		if !strings.Contains(string(body), "ReportLab not installed") {
			t.Fatalf("%s: expected error about ReportLab, got %s", path, body)
		}
		if !strings.Contains(string(body), `"available":false`) {
			t.Fatalf("%s: expected available:false in body, got %s", path, body)
		}
	}
}

// An analyzer that fails gives a 502 with a sentence, and nothing of what it
// printed.
func TestReports_AnalyzerErrorIsNotPassedOn(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500 with a traceback": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Traceback (most recent call last):\n  File \"/app/app/main.py\", line 1"))
		},
		"500 as json": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"detail":"Traceback /app/app/main.py"}`))
		},
		"200 that is not a pdf": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>Traceback /app/app/main.py</html>"))
		},
	}
	for name, h := range cases {
		fakeAnalyzer := httptest.NewServer(h)
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL}, "/api/v1/reports/owasp/pdf", nil)
		fakeAnalyzer.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("%s: expected 502, got %d", name, resp.StatusCode)
		}
		if m := reportErrorBody(t, resp, body); m["code"] != codeAnalyzerError {
			t.Fatalf("%s: code = %q", name, m["code"])
		}
		if strings.Contains(string(body), "Traceback") || strings.Contains(string(body), "/app/") {
			t.Fatalf("%s: the analyzer's output was passed on: %s", name, body)
		}
	}
}

func TestReports_AnalyzerDown(t *testing.T) {
	// A listener that is closed again: the port is free, nothing answers.
	gone := httptest.NewServer(http.NotFoundHandler())
	base := gone.URL
	gone.Close()

	start := time.Now()
	resp, body := reportGet(t, Config{AnalyzerHTTPURL: base}, "/api/v1/reports/owasp/pdf", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	m := reportErrorBody(t, resp, body)
	if m["code"] != codeAnalyzerUnavailable {
		t.Fatalf("code = %q", m["code"])
	}
	if !strings.Contains(m["error"], "not reachable") {
		t.Fatalf("error = %q", m["error"])
	}
	// The cause names an internal address; it belongs in the log.
	if strings.Contains(string(body), strings.TrimPrefix(base, "http://")) || strings.Contains(string(body), "dial tcp") {
		t.Fatalf("body gives away the analyzer's address: %s", body)
	}
	if d := time.Since(start); d > analyzerHTTPTimeout {
		t.Fatalf("a refused connection took %s", d)
	}
}

func TestReports_AnalyzerSlow(t *testing.T) {
	old := analyzerHTTPTimeout
	analyzerHTTPTimeout = 150 * time.Millisecond
	defer func() { analyzerHTTPTimeout = old }()

	release := make(chan struct{})
	slow := func(afterHeaders bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if afterHeaders {
				// The headers arrive in time; the body never finishes.
				w.Header().Set("Content-Type", "application/pdf")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("%PDF-1.4"))
				w.(http.Flusher).Flush()
			}
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
	}
	defer close(release)

	for name, afterHeaders := range map[string]bool{"no answer": false, "stalled body": true} {
		fakeAnalyzer := httptest.NewServer(slow(afterHeaders))
		start := time.Now()
		resp, body := reportGet(t, Config{AnalyzerHTTPURL: fakeAnalyzer.URL}, "/api/v1/reports/incident/pdf", nil)
		took := time.Since(start)
		fakeAnalyzer.CloseClientConnections()
		fakeAnalyzer.Close()

		if resp.StatusCode != http.StatusGatewayTimeout {
			t.Fatalf("%s: expected 504, got %d: %s", name, resp.StatusCode, body)
		}
		if m := reportErrorBody(t, resp, body); m["code"] != codeAnalyzerTimeout {
			t.Fatalf("%s: code = %q", name, m["code"])
		}
		if took > 3*time.Second {
			t.Fatalf("%s: the request hung for %s", name, took)
		}
	}
}

func TestBuildAnalyzerURL(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		path    string
		query   url.Values
		want    string
		wantErr bool
	}{
		{
			name: "basic",
			base: "http://analyzer:8444",
			path: "/api/v1/reports/owasp/pdf",
			want: "http://analyzer:8444/api/v1/reports/owasp/pdf",
		},
		{
			name:  "with query params",
			base:  "http://analyzer:8444",
			path:  "/api/v1/reports/owasp/pdf",
			query: url.Values{"range": {"7d"}},
			want:  "http://analyzer:8444/api/v1/reports/owasp/pdf?range=7d",
		},
		{
			name:  "base with trailing slash",
			base:  "http://analyzer:8444/",
			path:  "/api/v1/reports/incident/pdf",
			query: url.Values{"total_requests": {"100"}},
			want:  "http://analyzer:8444/api/v1/reports/incident/pdf?total_requests=100",
		},
		{
			name: "path without leading slash",
			base: "http://analyzer:8444",
			path: "api/v1/reports/owasp/pdf",
			want: "http://analyzer:8444/api/v1/reports/owasp/pdf",
		},
		{
			name: "base with a path prefix",
			base: "https://internal.example/analyzer/",
			path: "/api/v1/reports/owasp/pdf",
			want: "https://internal.example/analyzer/api/v1/reports/owasp/pdf",
		},
		{
			name:  "query values are encoded",
			base:  "http://analyzer:8444",
			path:  "/api/v1/reports/owasp/pdf",
			query: url.Values{"range": {"7d&x=1#f"}},
			want:  "http://analyzer:8444/api/v1/reports/owasp/pdf?range=7d%26x%3D1%23f",
		},
		{name: "no scheme", base: "analyzer:8444", path: "/x", wantErr: true},
		{name: "other scheme", base: "file:///etc/passwd", path: "/x", wantErr: true},
		{name: "no host", base: "http://", path: "/x", wantErr: true},
		{name: "credentials", base: "http://u:p@analyzer:8444", path: "/x", wantErr: true},
		{name: "query on the base", base: "http://analyzer:8444/?a=b", path: "/x", wantErr: true},
		{name: "fragment on the base", base: "http://analyzer:8444/#f", path: "/x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildAnalyzerURL(tt.base, tt.path, tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildAnalyzerURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("buildAnalyzerURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
