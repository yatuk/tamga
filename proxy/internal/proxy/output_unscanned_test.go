package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

// An answer whose scan stopped early and found nothing to stop or mask it has
// not been vouched for. It is sent on only when output_rules.fail_open says
// so, as a stream is.
func TestOutputScan_UnfinishedScanFollowsFailOpen(t *testing.T) {
	const answer = "nothing to find here, CUT-HERE"
	tests := []struct {
		name     string
		failOpen bool
		status   int
	}{
		{name: "fail_open unset", status: http.StatusServiceUnavailable},
		{name: "fail_open set", failOpen: true, status: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := fmt.Sprintf(rewritePolicy, "REDACT", 5000)
			if tc.failOpen {
				yaml = strings.Replace(yaml, "output_rules:\n", "output_rules:\n  fail_open: true\n", 1)
			}
			g := newRewriteRig(t, yaml, answer, nil, cutShortScanner{when: "CUT-HERE", err: errBudgetSpent})
			resp, err := http.Post(g.url+"/v1/chat/completions", "application/json", bytes.NewReader([]byte(userChat("hello"))))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d: %s", resp.StatusCode, tc.status, body)
			}
			if resp.Header.Get("X-Tamga-Scan-Degraded") != "true" {
				t.Fatal("the response does not say its scan was incomplete")
			}
			if tc.failOpen {
				if !bytes.Contains(body, []byte("CUT-HERE")) {
					t.Fatalf("fail_open is set and the answer was not forwarded: %s", body)
				}
				return
			}
			if bytes.Contains(body, []byte("CUT-HERE")) {
				t.Fatalf("the unscanned answer reached the client: %s", body)
			}
			if got := readRefusal(t, body); got.Error.Code != "tamga_scan_unavailable" {
				t.Fatalf("code = %q, want tamga_scan_unavailable", got.Error.Code)
			}
			if hits, _ := g.upstream(); hits != 1 {
				t.Fatalf("the provider was called %d times, want 1", hits)
			}
		})
	}
}

// What the scanners have compiled is what the cache key has to name: between
// a change of the patterns and the refresh that follows, it is still the old
// set.
func TestCustomScanner_FingerprintIsWhatWasCompiled(t *testing.T) {
	specs := []scanner.CustomEntitySpec{{Name: "A", Pattern: `A-\d{4}`, Severity: "high", Confidence: 0.9}}
	s := scanner.NewCustomScanner(func() []scanner.CustomEntitySpec { return specs })
	s.Refresh()
	before := s.Fingerprint()
	if before == "" {
		t.Fatal("no fingerprint after a refresh")
	}
	specs = append(specs, scanner.CustomEntitySpec{Name: "B", Pattern: `B-\d{4}`, Severity: "high", Confidence: 0.9})
	if s.Fingerprint() != before {
		t.Fatal("the fingerprint changed before the scanner compiled the new set")
	}
	s.Refresh()
	if s.Fingerprint() == before {
		t.Fatal("the fingerprint did not change with the compiled set")
	}
}
