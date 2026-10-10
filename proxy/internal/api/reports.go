package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// analyzerHTTPTimeout bounds one report request end to end: connecting,
// generating the PDF and reading it. A variable so tests can shorten it.
var analyzerHTTPTimeout = 15 * time.Second

// maxReportBytes caps what is read of a report. The two reports are a few
// pages each; anything near this size is not one of them.
const maxReportBytes = 16 << 20

// Machine-readable codes in the body of a report error, next to the "error"
// sentence the console shows.
const (
	codeAnalyzerNotConfigured = "analyzer_not_configured"
	codeAnalyzerUnavailable   = "analyzer_unavailable"
	codeAnalyzerTimeout       = "analyzer_timeout"
	codeAnalyzerError         = "analyzer_error"
)

// The paths of the two reports on the analyzer. They are constants: nothing
// in a caller's request chooses where the proxy connects or what it asks for.
const (
	analyzerOwaspPdfPath    = "/api/v1/reports/owasp/pdf"
	analyzerIncidentPdfPath = "/api/v1/reports/incident/pdf"
)

// reportCountParams are the query parameters of the incident report that the
// analyzer reads as integers. The console fills them from the statistics it
// is showing.
var reportCountParams = []string{"total_requests", "blocked", "redacted", "warned", "period_hours"}

// analyzerHTTPClient is the client for the analyzer's HTTP API.
//
// It does not go through the egress guard on purpose. The guard is for
// destinations that arrive through the API or the policy file and refuses
// internal addresses; the analyzer is an internal service whose address the
// operator sets in the proxy's own environment (TAMGA_ANALYZER_HTTP_URL),
// the same trust as the database URL. What protects this call instead: the
// path is a constant, the query is rebuilt from a fixed list of parameters,
// no caller header is forwarded, no proxy from the environment is used and
// no redirect is followed.
var analyzerHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// handleOwaspPdfReport returns the OWASP compliance report the analyzer
// generates as a PDF.
func (cfg Config) handleOwaspPdfReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out := url.Values{}
	if v := q.Get("range"); v != "" {
		out.Set("range", v)
	}
	cfg.proxyReportToAnalyzer(w, r, analyzerOwaspPdfPath, out)
}

// handleIncidentPdfReport returns the incident summary the analyzer
// generates as a PDF from the counts in the query.
func (cfg Config) handleIncidentPdfReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out := url.Values{}
	if v := q.Get("range"); v != "" {
		out.Set("range", v)
	}
	for _, name := range reportCountParams {
		v := q.Get(name)
		if v == "" {
			continue
		}
		if _, err := strconv.ParseUint(v, 10, 31); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("%s must be a whole number", name),
			})
			return
		}
		out.Set(name, v)
	}
	cfg.proxyReportToAnalyzer(w, r, analyzerIncidentPdfPath, out)
}

// reportError writes the body of a failed report: "error" is the sentence the
// console shows, "code" tells the cases apart.
func reportError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// proxyReportToAnalyzer asks the analyzer for one report and passes the PDF
// back. path is one of the two constants above and query is what the handler
// rebuilt; nothing else of the caller's request reaches the analyzer.
//
// The failures are told apart for the console: the address not set or not
// usable and the analyzer not answering are 503, a request that ran out of
// time is 504, an answer that is not a report is 502. The analyzer's own 501
// (its PDF library is missing) is passed through. The cause is logged, not
// returned: it names an internal address.
func (cfg Config) proxyReportToAnalyzer(w http.ResponseWriter, r *http.Request, path string, query url.Values) {
	if strings.TrimSpace(cfg.AnalyzerHTTPURL) == "" {
		reportError(w, http.StatusServiceUnavailable, codeAnalyzerNotConfigured,
			"PDF reports are unavailable: the analyzer's HTTP address is not set (TAMGA_ANALYZER_HTTP_URL)")
		return
	}
	target, err := buildAnalyzerURL(cfg.AnalyzerHTTPURL, path, query)
	if err != nil {
		log.Warn().Err(err).Str("component", "api").Msg("TAMGA_ANALYZER_HTTP_URL is not usable")
		reportError(w, http.StatusServiceUnavailable, codeAnalyzerNotConfigured,
			"PDF reports are unavailable: TAMGA_ANALYZER_HTTP_URL is not a valid http(s) address")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), analyzerHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		reportError(w, http.StatusServiceUnavailable, codeAnalyzerNotConfigured,
			"PDF reports are unavailable: TAMGA_ANALYZER_HTTP_URL is not a valid http(s) address")
		return
	}
	req.Header.Set("Accept", "application/pdf, application/json")

	resp, err := analyzerHTTPClient.Do(req)
	if err != nil {
		cfg.reportTransportError(w, r, path, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		// 501 is the analyzer saying its PDF library is not installed. Its
		// body is passed on when it is the JSON object it documents.
		if resp.StatusCode == http.StatusNotImplemented && json.Valid(body) &&
			strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write(body)
			return
		}
		log.Warn().Str("component", "api").Str("path", path).Int("status", resp.StatusCode).
			Msg("analyzer refused a report request")
		reportError(w, http.StatusBadGateway, codeAnalyzerError,
			fmt.Sprintf("The analyzer could not generate the report (it answered %d)", resp.StatusCode))
		return
	}

	// Read the whole report before answering, so a failure halfway is still
	// an error with a body and not a truncated download with status 200.
	pdf, err := io.ReadAll(io.LimitReader(resp.Body, maxReportBytes+1))
	if err != nil {
		cfg.reportTransportError(w, r, path, err)
		return
	}
	if len(pdf) > maxReportBytes || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/pdf") {
		log.Warn().Str("component", "api").Str("path", path).Int("bytes", len(pdf)).
			Str("content_type", resp.Header.Get("Content-Type")).Msg("analyzer answer is not a report")
		reportError(w, http.StatusBadGateway, codeAnalyzerError,
			"The analyzer's answer was not a PDF report")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(pdf)))
	w.Header().Set("Content-Disposition", "attachment; filename="+reportFilename(path))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

// reportTransportError answers for a request to the analyzer that got no
// usable response.
func (cfg Config) reportTransportError(w http.ResponseWriter, r *http.Request, path string, err error) {
	// The caller went away: there is nobody to answer.
	if r.Context().Err() != nil {
		return
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		log.Warn().Err(err).Str("component", "api").Str("path", path).Msg("analyzer report request timed out")
		reportError(w, http.StatusGatewayTimeout, codeAnalyzerTimeout,
			fmt.Sprintf("The analyzer did not answer within %s, so the report was not generated", analyzerHTTPTimeout))
		return
	}
	log.Warn().Err(err).Str("component", "api").Str("path", path).Msg("analyzer is not reachable for reports")
	reportError(w, http.StatusServiceUnavailable, codeAnalyzerUnavailable,
		"The analyzer is not reachable, so the report was not generated. Check that it is running and that TAMGA_ANALYZER_HTTP_URL points at it")
}

// reportFilename is the download name of a report. It is set here and not
// copied from the analyzer's answer, so no header of that answer is passed on.
func reportFilename(path string) string {
	if path == analyzerIncidentPdfPath {
		return "tamga-incident-report.pdf"
	}
	return "tamga-owasp-report.pdf"
}

// buildAnalyzerURL is the address of one report: the configured base, the
// fixed path and the rebuilt query. The base must be an absolute http or
// https URL without credentials, query or fragment; a path on it is kept as
// a prefix, for an analyzer behind a path-routing ingress.
func buildAnalyzerURL(base, path string, query url.Values) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", errors.New("analyzer URL does not parse")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("analyzer URL must be http(s)://host[:port]")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("analyzer URL must not carry credentials, a query or a fragment")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawPath = ""
	u.RawQuery = query.Encode()
	return u.String(), nil
}
