package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// longAnswer returns prose of n bytes that ends with tail.
func longAnswer(n int, tail string) string {
	const sentence = "The quarterly review covers revenue, staffing and the plan for the next two quarters in every region. "
	var sb strings.Builder
	for sb.Len()+len(tail) < n {
		sb.WriteString(sentence)
	}
	return sb.String()[:n-len(tail)] + tail
}

// An answer as long as the route accepts gets a verdict. It used to be given
// output_rules.scan_window_ms, 200 ms by default, and every answer the
// scanners needed longer for was a 503, which LiteLLM turns into a refused
// request. What is near the end is still found.
func TestLiteLLM_LongAnswerGetsAVerdict(t *testing.T) {
	if raceDetector {
		// It checks that the largest answer is scanned inside the route's
		// ten-second budget. Instrumented, the same scan took just over ten
		// seconds on a CI runner: that measures the detector, not the route.
		t.Skip("a wall-time bound; not meaningful under the race detector")
	}
	// Just under the limit on what one call may carry to be scanned.
	const size = scanMaxBodyBytes - 1024
	cases := []struct {
		name, tail, action, reason, endsWith string
	}{
		{name: "clean", tail: " That is all.", action: "NONE"},
		{name: "national id near the end", tail: " The number on file is " + litellmTCKN + ".",
			action: "BLOCKED", reason: "pii/tc_kimlik"},
		{name: "e-mail near the end", tail: " Write to " + scanEmail + ".",
			action: "GUARDRAIL_INTERVENED", endsWith: " Write to [email_REDACTED]."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
			text := longAnswer(size, tc.tail)
			start := time.Now()
			resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody("response", []string{text}, nil))
			t.Logf("%d bytes scanned in %s", len(text), time.Since(start).Round(time.Millisecond))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want a verdict: %.300s", resp.StatusCode, raw)
			}
			var out litellmReply
			if err := json.Unmarshal([]byte(raw), &out); err != nil {
				t.Fatal(err)
			}
			if out.Action != tc.action {
				t.Fatalf("action = %q, want %q (reason %q)", out.Action, tc.action, out.BlockedReason)
			}
			if !strings.Contains(out.BlockedReason, tc.reason) {
				t.Fatalf("blocked_reason = %q, want it to contain %q", out.BlockedReason, tc.reason)
			}
			if tc.endsWith != "" {
				if len(out.Texts) != 1 || !strings.HasSuffix(out.Texts[0], tc.endsWith) || strings.Contains(out.Texts[0], scanEmail) {
					t.Fatalf("the masked answer ends %q", tailOf(out.Texts))
				}
			}
		})
	}

	// Many texts are one answer with one budget, not a budget each.
	t.Run("many texts", func(t *testing.T) {
		g := newScanRig(t, scanRigOptions{policy: litellmPolicy})
		texts := make([]string, 64)
		for i := range texts {
			texts[i] = longAnswer(size/len(texts), " Done.")
		}
		texts[len(texts)-1] = longAnswer(size/len(texts), " The number on file is "+litellmTCKN+".")
		out, _, raw := g.litellm(t, "response", texts, nil)
		if out.Action != "BLOCKED" || !strings.Contains(out.BlockedReason, "pii/tc_kimlik") {
			t.Fatalf("action = %q: %.300s", out.Action, raw)
		}
	})
}

func tailOf(texts []string) string {
	if len(texts) == 0 {
		return ""
	}
	s := texts[len(texts)-1]
	if len(s) > 60 {
		s = s[len(s)-60:]
	}
	return s
}

// stuckScanner does not come back until its context is over.
type stuckScanner struct{}

func (stuckScanner) Name() string { return "stuck" }
func (stuckScanner) Scan(ctx context.Context, _ []byte) ([]scanner.Finding, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// The budget is a hard bound, and a scan that reaches it gives no verdict:
// the answer is a 503, never an action, whatever output_rules.fail_open says.
func TestLiteLLM_AnswerScanOverBudgetFailsClosed(t *testing.T) {
	for _, failOpen := range []string{"false", "true"} {
		t.Run("fail_open "+failOpen, func(t *testing.T) {
			pol := strings.Replace(litellmPolicy, "output_rules:\n  enabled: true\n", "output_rules:\n  enabled: true\n  fail_open: "+failOpen+"\n", 1)
			if pol == litellmPolicy {
				t.Fatal("the policy was not changed")
			}
			g := newScanRig(t, scanRigOptions{policy: pol, scanners: []scanner.Scanner{stuckScanner{}}})
			g.bridge.answerBudget = 150 * time.Millisecond
			start := time.Now()
			// The answer holds what output_rules blocks: a scan cut short is
			// not a finding either.
			resp, raw := g.litellmRaw(t, litellmAdmin(), litellmBody("response", []string{"Paris.", "And Rome."}, nil))
			took := time.Since(start)
			if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
				t.Fatalf("status = %d, want 503 with Retry-After: %s", resp.StatusCode, raw)
			}
			var fail litellmFailure
			if err := json.Unmarshal([]byte(raw), &fail); err != nil || fail.Error.Code != "tamga_scan_unavailable" {
				t.Fatalf("code = %q: %s", fail.Error.Code, raw)
			}
			if strings.Contains(raw, "action") || strings.Contains(raw, "NONE") {
				t.Fatalf("the 503 carries a verdict: %s", raw)
			}
			// One budget for the call, not one for each of its two texts.
			if took < 100*time.Millisecond || took > 2*time.Second {
				t.Errorf("answered after %s, want about the budget of 150ms", took)
			}
		})
	}

	// A window longer than the budget is the operator's and is kept.
	t.Run("a longer scan_window_ms is kept", func(t *testing.T) {
		pol := strings.Replace(litellmPolicy, "output_rules:\n  enabled: true\n", "output_rules:\n  enabled: true\n  scan_window_ms: 60000\n", 1)
		g := newScanRig(t, scanRigOptions{policy: pol})
		g.bridge.answerBudget = time.Nanosecond
		if out, _, raw := g.litellm(t, "response", []string{"Paris."}, nil); out.Action != "NONE" {
			t.Fatalf("action = %q: %s", out.Action, raw)
		}
	})
}

// The proxy path's window is not the bridge's: a policy with the shortest
// window still gets a verdict over the bridge.
func TestLiteLLM_AnswerIgnoresAShortScanWindow(t *testing.T) {
	pol := strings.Replace(litellmPolicy, "output_rules:\n  enabled: true\n", "output_rules:\n  enabled: true\n  scan_window_ms: 1\n", 1)
	g := newScanRig(t, scanRigOptions{policy: pol})
	out, _, raw := g.litellm(t, "response", []string{longAnswer(64*1024, " The number on file is "+litellmTCKN+".")}, nil)
	if out.Action != "BLOCKED" {
		t.Fatalf("action = %q: %.300s", out.Action, raw)
	}
}
