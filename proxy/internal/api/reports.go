package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const analyzerHTTPTimeout = 5 * time.Second

// handleOwaspPdfReport reverse-proxies OWASP compliance PDF report generation
// to the analyzer's /api/v1/reports/owasp/pdf endpoint.
func (cfg Config) handleOwaspPdfReport(w http.ResponseWriter, r *http.Request) {
	cfg.proxyReportToAnalyzer(w, r, "/api/v1/reports/owasp/pdf")
}

// handleIncidentPdfReport reverse-proxies incident summary PDF report generation
// to the analyzer's /api/v1/reports/incident/pdf endpoint.
func (cfg Config) handleIncidentPdfReport(w http.ResponseWriter, r *http.Request) {
	cfg.proxyReportToAnalyzer(w, r, "/api/v1/reports/incident/pdf")
}

// proxyReportToAnalyzer forwards a GET request to the analyzer's HTTP API
// and streams the PDF response (or error) back to the caller.
//
// Query parameters from the original request are forwarded transparently.
// A 5s timeout protects the proxy from hanging requests.
func (cfg Config) proxyReportToAnalyzer(w http.ResponseWriter, r *http.Request, path string) {
	if cfg.AnalyzerHTTPURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "analyzer unavailable",
			"message": "analyzer HTTP URL not configured",
		})
		return
	}

	targetURL, err := buildAnalyzerURL(cfg.AnalyzerHTTPURL, path, r.URL.RawQuery)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "analyzer request failed",
			"message": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), analyzerHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "analyzer request failed",
			"message": err.Error(),
		})
		return
	}

	client := &http.Client{Timeout: analyzerHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "analyzer unavailable",
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Pass through non-200 responses: if the analyzer returned JSON (e.g.
	// 501 when ReportLab is not installed), forward it as JSON so the
	// dashboard can show a meaningful message.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write(body)
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "analyzer error",
			"message": fmt.Sprintf("analyzer returned status %d", resp.StatusCode),
		})
		return
	}

	// Stream PDF response to the caller.
	w.Header().Set("Content-Type", "application/pdf")
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}

// buildAnalyzerURL constructs the full target URL for the analyzer.
// It joins the base URL and path, then appends query parameters if present.
// Invalid URLs are returned as an error for defensive coding.
func buildAnalyzerURL(base, path, rawQuery string) (string, error) {
	// Ensure base does not have a trailing slash.
	base = strings.TrimRight(base, "/")
	// Ensure path starts with a slash.
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := base + path
	if rawQuery != "" {
		u += "?" + rawQuery
	}
	return u, nil
}
