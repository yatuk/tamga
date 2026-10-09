package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// cutShortScanner stands in for a scanner that stops before the end of a
// text. It reports nothing itself, and only stops on text that holds `when`,
// so that a test can cut the scan of the request or of the answer.
type cutShortScanner struct {
	when string
	// err is what it returns. When nil it waits for the context to end and
	// returns that, as a scanner does at the deadline.
	err error
}

func (cutShortScanner) Name() string { return "cut_short" }
func (s cutShortScanner) Scan(ctx context.Context, content []byte) ([]scanner.Finding, error) {
	if !bytes.Contains(content, []byte(s.when)) {
		return nil, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	<-ctx.Done()
	return nil, fmt.Errorf("%w: cut_short scanner stopped: %w", scanner.ErrScanDegraded, ctx.Err())
}

var errBudgetSpent = fmt.Errorf("%w: cut_short scanner spent its time budget: %w", scanner.ErrScanDegraded, scanner.ErrTimeBudget)

const rewritePolicy = `
version: "1.0"
rules:
  pii:
    action: REDACT
    sensitivity: low
    types: [email]
custom_entities:
  - name: "musteri_no"
    pattern: "MN-\\d{4}"
    severity: "high"
    action: %s
    confidence: 0.9
output_rules:
  enabled: true
  buffer_bytes: 262144
  scan_window_ms: %d
  redact_on: [email, musteri_no]
providers:
  allowed: [openai]
`

// customerNumbers returns n customer numbers, each a match of the policy's
// custom entity.
func customerNumbers(n int) string { return strings.Repeat("MN-1234 ", n) }

type rewriteRig struct {
	url string
	mu  sync.Mutex
	// hits and sent are what the provider saw.
	hits int
	sent string
}

func (g *rewriteRig) upstream() (int, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hits, g.sent
}

// newRewriteRig returns a proxy with the policy's custom entity compiled
// into its registry, as the running proxy has it, and a provider that
// answers with answer as the assistant's message.
func newRewriteRig(t *testing.T, policyYAML, answer string, cfg *config.Config, extra ...scanner.Scanner) *rewriteRig {
	t.Helper()
	g := &rewriteRig{}
	body, _ := json.Marshal(map[string]interface{}{
		"id": "c1",
		"choices": []map[string]interface{}{{
			"index": 0, "finish_reason": "stop",
			"message": map[string]string{"role": "assistant", "content": answer},
		}},
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.hits++
		g.sent = string(b)
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	pol := mustPolicy(t, policyYAML)
	registry := scanner.NewRegistry()
	registry.Register(scanner.NewPIIScanner())
	registry.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec {
		out := make([]scanner.CustomEntitySpec, 0, len(pol.CustomEntities))
		for _, ce := range pol.CustomEntities {
			out = append(out, scanner.CustomEntitySpec{Name: ce.Name, Pattern: ce.Pattern, Severity: ce.Severity, Confidence: ce.Confidence})
		}
		return out
	}))
	for _, s := range extra {
		registry.Register(s)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     registry,
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       cfg,
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

func userChat(content string) string {
	b, _ := json.Marshal(map[string]interface{}{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": content}},
	})
	return string(b)
}

type refusal struct {
	Error struct {
		Code   string `json:"code"`
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"error"`
}

func readRefusal(t *testing.T, body []byte) refusal {
	t.Helper()
	var r refusal
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	return r
}

// A request the policy masks or strips is forwarded only when the scan read
// all of it. scan.on_error: pass lets an unscanned request through; it does
// not let a half-masked one through.
func TestRequestRewrite_IncompleteScanIsRefused(t *testing.T) {
	overCap := customerNumbers(scanner.MaxFindingsPerScan + 1)
	withEmail := "write to ayse@example.com about CUT-HERE"
	tests := []struct {
		name     string
		action   string // the custom entity's action
		onError  string
		content  string
		extra    scanner.Scanner
		timeout  int // scanner pipeline timeout, ms
		status   int
		code     string
		reason   string
		retry    bool
		metric   string
		degraded bool
	}{
		{name: "over the cap, REDACT, on_error pass", action: "REDACT", onError: "pass", content: overCap,
			status: 503, code: rewriteIncompleteCode, reason: "findings_cap", metric: "redact_incomplete_findings_cap", degraded: true},
		{name: "over the cap, REDACT, on_error not set", action: "REDACT", content: overCap,
			status: 503, code: rewriteIncompleteCode, reason: "findings_cap", metric: "redact_incomplete_findings_cap", degraded: true},
		{name: "over the cap, REDACT, on_error block", action: "REDACT", onError: "block", content: overCap,
			status: 503, code: "tamga_scan_unavailable", retry: true, degraded: true},
		{name: "over the cap, STRIP, on_error pass", action: "STRIP", onError: "pass", content: overCap,
			status: 503, code: rewriteIncompleteCode, reason: "findings_cap", metric: "strip_incomplete_findings_cap", degraded: true},
		{name: "time budget spent, REDACT, on_error pass", action: "REDACT", onError: "pass", content: withEmail,
			extra:  cutShortScanner{when: "CUT-HERE", err: errBudgetSpent},
			status: 503, code: rewriteIncompleteCode, reason: "time_budget", retry: true, metric: "redact_incomplete_time_budget", degraded: true},
		{name: "deadline, REDACT, on_error pass", action: "REDACT", onError: "pass", content: withEmail,
			extra: cutShortScanner{when: "CUT-HERE"}, timeout: 30,
			status: 503, code: rewriteIncompleteCode, reason: "deadline", retry: true, metric: "redact_incomplete_deadline", degraded: true},
		{name: "scanner failed, REDACT, on_error pass", action: "REDACT", onError: "pass", content: withEmail,
			extra:  cutShortScanner{when: "CUT-HERE", err: fmt.Errorf("backend unavailable")},
			status: 503, code: rewriteIncompleteCode, reason: "scanner_failed", retry: true, metric: "redact_incomplete_scanner_failed", degraded: true},
		{name: "scanner failed, nothing to mask, on_error pass", action: "REDACT", onError: "pass", content: "about CUT-HERE",
			extra:  cutShortScanner{when: "CUT-HERE", err: fmt.Errorf("backend unavailable")},
			status: 200, degraded: true},
		{name: "at the cap, REDACT", action: "REDACT", onError: "pass", content: customerNumbers(scanner.MaxFindingsPerScan),
			status: 200},
		{name: "at the cap, REDACT, on_error block", action: "REDACT", onError: "block", content: customerNumbers(scanner.MaxFindingsPerScan),
			status: 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := fmt.Sprintf(rewritePolicy, tc.action, 200)
			if tc.onError != "" {
				yaml += "scan:\n  on_error: " + tc.onError + "\n"
			}
			var extra []scanner.Scanner
			if tc.extra != nil {
				extra = append(extra, tc.extra)
			}
			g := newRewriteRig(t, yaml, "ok", &config.Config{ScannerPipelineTimeoutMs: tc.timeout}, extra...)
			before := scanner.ScanDegradedStats()[tc.metric]

			resp := post(t, g.url, "application/json", userChat(tc.content))
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.status, body)
			}
			if got := resp.Header.Get("X-Tamga-Scan-Degraded") == "true"; got != tc.degraded {
				t.Errorf("X-Tamga-Scan-Degraded = %q", resp.Header.Get("X-Tamga-Scan-Degraded"))
			}
			hits, sent := g.upstream()

			if tc.status != http.StatusOK {
				if hits != 0 {
					t.Fatalf("the request reached the provider %d times: %.200s", hits, sent)
				}
				r := readRefusal(t, body)
				if r.Error.Code != tc.code || r.Error.Reason != tc.reason || r.Error.Type != "service_unavailable" {
					t.Errorf("refusal = %+v, want code %q reason %q", r.Error, tc.code, tc.reason)
				}
				if got := resp.Header.Get("Retry-After") != ""; got != tc.retry {
					t.Errorf("Retry-After = %q, want set: %v", resp.Header.Get("Retry-After"), tc.retry)
				}
				if tc.metric != "" && scanner.ScanDegradedStats()[tc.metric] != before+1 {
					t.Errorf("tamga_scan_degraded_total{reason=%q} did not go up by one", tc.metric)
				}
				if bytes.Contains(body, []byte("MN-1234")) || bytes.Contains(body, []byte("ayse@example.com")) {
					t.Errorf("the refusal quotes the text: %.200s", body)
				}
				return
			}

			if hits != 1 {
				t.Fatalf("provider hits = %d, want 1", hits)
			}
			if strings.Contains(sent, "MN-1234") {
				t.Errorf("a customer number reached the provider unmasked")
			}
			if strings.Contains(tc.content, "MN-1234") {
				if got := strings.Count(sent, "[musteri_no_REDACTED]"); got != scanner.MaxFindingsPerScan {
					t.Errorf("%d values masked, want %d", got, scanner.MaxFindingsPerScan)
				}
			}
		})
	}
}

// The same holds for an answer: one that could only be masked in part is not
// sent, whatever scan.on_error says.
func TestResponseRewrite_IncompleteScanIsWithheld(t *testing.T) {
	overCap := customerNumbers(scanner.MaxFindingsPerScan + 1)
	withEmail := "write to ayse@example.com about CUT-HERE"
	tests := []struct {
		name    string
		onError string
		answer  string
		extra   scanner.Scanner
		window  int // output_rules.scan_window_ms
		status  int
		reason  string
		retry   bool
	}{
		{name: "over the cap, on_error pass", onError: "pass", answer: overCap, status: 503, reason: "findings_cap"},
		{name: "over the cap, on_error block", onError: "block", answer: overCap, status: 503, reason: "findings_cap"},
		{name: "time budget spent, on_error pass", onError: "pass", answer: withEmail,
			extra: cutShortScanner{when: "CUT-HERE", err: errBudgetSpent}, status: 503, reason: "time_budget", retry: true},
		{name: "deadline, on_error pass", onError: "pass", answer: withEmail,
			extra: cutShortScanner{when: "CUT-HERE"}, window: 30, status: 503, reason: "deadline", retry: true},
		{name: "deadline, on_error block", onError: "block", answer: withEmail,
			extra: cutShortScanner{when: "CUT-HERE"}, window: 30, status: 503, reason: "deadline", retry: true},
		{name: "at the cap", onError: "pass", answer: customerNumbers(scanner.MaxFindingsPerScan), status: 200},
		{name: "one value", onError: "block", answer: "write to ayse@example.com", status: 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window := tc.window
			if window == 0 {
				window = 5000
			}
			yaml := fmt.Sprintf(rewritePolicy, "REDACT", window) + "scan:\n  on_error: " + tc.onError + "\n"
			var extra []scanner.Scanner
			if tc.extra != nil {
				extra = append(extra, tc.extra)
			}
			g := newRewriteRig(t, yaml, tc.answer, nil, extra...)
			metric := "redact_output_incomplete_" + tc.reason
			before := scanner.ScanDegradedStats()[metric]

			resp := post(t, g.url, "application/json", userChat("hello"))
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d: %.300s", resp.StatusCode, tc.status, body)
			}
			if bytes.Contains(body, []byte("MN-1234")) || bytes.Contains(body, []byte("ayse@example.com")) {
				t.Fatalf("a value reached the client unmasked: %.200s", body)
			}
			if tc.status == http.StatusOK {
				if resp.Header.Get("X-Tamga-Scan-Degraded") != "" {
					t.Errorf("a complete scan was reported degraded")
				}
				if !bytes.Contains(body, []byte("_REDACTED]")) {
					t.Errorf("nothing was masked: %.200s", body)
				}
				return
			}
			r := readRefusal(t, body)
			if r.Error.Code != rewriteIncompleteCode || r.Error.Reason != tc.reason || r.Error.Type != "service_unavailable" {
				t.Errorf("refusal = %+v, want reason %q", r.Error, tc.reason)
			}
			if resp.Header.Get("X-Tamga-Scan-Degraded") != "true" {
				t.Errorf("X-Tamga-Scan-Degraded = %q", resp.Header.Get("X-Tamga-Scan-Degraded"))
			}
			if got := resp.Header.Get("Retry-After") != ""; got != tc.retry {
				t.Errorf("Retry-After = %q, want set: %v", resp.Header.Get("Retry-After"), tc.retry)
			}
			if scanner.ScanDegradedStats()[metric] != before+1 {
				t.Errorf("tamga_scan_degraded_total{reason=%q} did not go up by one", metric)
			}
		})
	}
}

// With the vault on, a withheld answer must stay withheld: restoring the
// placeholders must not put the provider's answer back.
func TestResponseRewrite_WithheldAnswerSurvivesVaultRestore(t *testing.T) {
	yaml := fmt.Sprintf(rewritePolicy, "REDACT", 5000) + "vault:\n  enabled: true\n"
	g := newRewriteRig(t, yaml, customerNumbers(scanner.MaxFindingsPerScan+1), nil)
	// The request has a value to tokenise, so the vault is active for it.
	resp := post(t, g.url, "application/json", userChat("write to ayse@example.com"))
	body, _ := io.ReadAll(resp.Body)
	if _, sent := g.upstream(); !strings.Contains(sent, vaultPlaceholderOpen) {
		t.Fatalf("the vault was not used for the request: %.200s", sent)
	}
	if resp.StatusCode != http.StatusServiceUnavailable || bytes.Contains(body, []byte("MN-1234")) {
		t.Fatalf("status = %d, body %.200s", resp.StatusCode, body)
	}
	if readRefusal(t, body).Error.Code != rewriteIncompleteCode {
		t.Fatalf("body = %.200s", body)
	}
}

// A streamed answer with fail_open: text the scanner could not look at goes
// through, text it was to mask and searched only in part does not.
func TestStreamGuard_PartialScanDoesNotMaskInPart(t *testing.T) {
	g := newStreamGuard()
	g.failOpen = true
	g.redacts = func(f scanner.Finding) bool { return f.Category == "email" }
	g.scan = func(text []byte) ([]scanner.Finding, error) {
		var found []scanner.Finding
		if i := bytes.Index(text, []byte("a@example.com")); i >= 0 {
			found = append(found, scanner.Finding{Type: "pii", Category: "email", StartPos: i, EndPos: i + len("a@example.com"), Confidence: 0.9})
		}
		return found, errBudgetSpent
	}
	wire, text, stopped := run(t, g, openAIChunk("first a@example.com then b@example.com and more text to release it all"), openAIFinish)
	if !stopped || g.stopReason != "redaction_incomplete" {
		t.Fatalf("stopped=%v reason=%q text=%q", stopped, g.stopReason, text)
	}
	if strings.Contains(wire, "example.com") {
		t.Fatalf("part of the text went out: %s", wire)
	}

	// Nothing to mask: fail_open still lets the text through.
	g = newStreamGuard()
	g.failOpen = true
	g.redacts = func(f scanner.Finding) bool { return f.Category == "email" }
	g.scan = func([]byte) ([]scanner.Finding, error) { return nil, errBudgetSpent }
	if _, text, stopped := run(t, g, openAIChunk("some text"), openAIFinish); stopped || text != "some text" {
		t.Fatalf("fail open: stopped=%v text=%q", stopped, text)
	}
}

// The scan API gives no verdict on a scan that was cut short, whatever the
// policy's on_error.
func TestScanAPI_OverTheCapIsNoVerdict(t *testing.T) {
	for _, onError := range []string{"pass", "block"} {
		t.Run(onError, func(t *testing.T) {
			yaml := fmt.Sprintf(rewritePolicy, "REDACT", 200) + "scan:\n  on_error: " + onError + "\n"
			pol := mustPolicy(t, yaml)
			registry := scanner.NewRegistry()
			registry.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec {
				ce := pol.CustomEntities[0]
				return []scanner.CustomEntitySpec{{Name: ce.Name, Pattern: ce.Pattern, Severity: ce.Severity, Confidence: ce.Confidence}}
			}))
			h := NewScanHandler(HandlerConfig{
				Registry:  registry,
				GetPolicy: func() *policy.Policy { return pol },
				Config:    &config.Config{},
			}, ScanOptions{AdminKey: "admin-key-for-this-test"}).(*scanHandler)
			t.Cleanup(func() { _ = h.fallback.Close() })

			body, _ := json.Marshal(map[string]string{"text": customerNumbers(scanner.MaxFindingsPerScan + 1)})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/scan", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(adminKeyHeader, "admin-key-for-this-test")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable || strings.Contains(rec.Body.String(), "redacted_text") {
				t.Fatalf("status = %d, body %.200s", rec.Code, rec.Body.String())
			}
		})
	}
}
