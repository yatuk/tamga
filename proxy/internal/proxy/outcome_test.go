package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/pricing"
	"github.com/yatuk/tamga/internal/scanner"
)

// These tests are about the one event a request publishes when it is over:
// the request log is written from it, so what it lacks the record lacks.

const outcomePolicy = `
version: "7"
name: bank-policy
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
providers:
  allowed: [openai]
cache:
  enabled: true
  ttl_seconds: 300
output_rules:
  enabled: true
  block_on: [aws_access_key]
  redact_on: [phone_tr]
  streaming:
    enabled: true
`

type outcomeRig struct {
	url  string
	keys *apikeys.MemoryStore

	mu     sync.Mutex
	events []events.Event
}

// newOutcomeRig starts a proxy in front of a provider that answers with
// upstream, and collects every event the proxy publishes.
func newOutcomeRig(t *testing.T, policyYAML string, upstream http.HandlerFunc) *outcomeRig {
	t.Helper()
	g := &outcomeRig{keys: apikeys.NewMemoryStore()}
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	u, _ := url.Parse(up.URL)

	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)

	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Cache:        cache.New(16),
		Keys:         g.keys,
		Bus:          bus,
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

func (g *outcomeRig) post(t *testing.T, body string, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, g.url+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", credA)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// requestEvents waits for n request events and returns them, in order.
func (g *outcomeRig) requestEvents(t *testing.T, n int) []events.Event {
	t.Helper()
	var out []events.Event
	eventually(t, "the request events", func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		out = out[:0]
		for _, e := range g.events {
			if e.EventType == "request_scanned" || e.EventType == "request_blocked" {
				out = append(out, e)
			}
		}
		return len(out) >= n
	})
	if len(out) != n {
		t.Fatalf("%d request events, want %d: one per request", len(out), n)
	}
	return out
}

func jsonAnswer(text, usage string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + text + `"}}]` + usage + `}`))
	}
}

func outcomeChat(model, content string) string {
	return `{"model":"` + model + `","messages":[{"role":"user","content":"` + content + `"}]}`
}

func TestRequestEvent_TokensCostAndPolicy(t *testing.T) {
	g := newOutcomeRig(t, outcomePolicy, jsonAnswer("fine", `,"usage":{"prompt_tokens":1200,"completion_tokens":300}`))
	resp, body := g.post(t, outcomeChat("gpt-4o-mini", "hello there"), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	e := g.requestEvents(t, 1)[0]
	if e.InputTokens != 1200 || e.OutputTokens != 300 {
		t.Errorf("tokens = %d in, %d out; the provider reported 1200 and 300", e.InputTokens, e.OutputTokens)
	}
	row, ok := pricing.Match(pricing.Defaults(), "openai", "gpt-4o-mini")
	if !ok {
		t.Fatal("gpt-4o-mini has no built-in price")
	}
	if want := pricing.CostUSD(row, 1200, 300); !e.CostKnown || e.CostUSD != want || want <= 0 {
		t.Errorf("cost = %v (known %v), want %v", e.CostUSD, e.CostKnown, want)
	}
	// What the client was told and what is recorded are the same numbers.
	if resp.Header.Get("X-Tamga-Tokens-In") != "1200" || resp.Header.Get("X-Tamga-Cost-USD") == "" {
		t.Errorf("response headers: tokens in %q, cost %q", resp.Header.Get("X-Tamga-Tokens-In"), resp.Header.Get("X-Tamga-Cost-USD"))
	}
	if e.PolicyName != "bank-policy" || e.PolicyVersion != "7" {
		t.Errorf("policy = %q version %q, want the one the request was decided with", e.PolicyName, e.PolicyVersion)
	}
	if e.CacheStatus != "miss" {
		t.Errorf("cache status = %q, want miss", e.CacheStatus)
	}
	// The response was scanned and nothing was found.
	if !e.OutputScanned || e.OutputAction != "PASS" || len(e.OutputFindings) != 0 || e.OutputRisk.Level != "none" {
		t.Errorf("output: scanned %v action %q findings %d risk %q", e.OutputScanned, e.OutputAction, len(e.OutputFindings), e.OutputRisk.Level)
	}
	// The total covers the scan. (Their precision is durationMs's, tested
	// on its own: the clock of a test machine may tick too slowly to show
	// a scan of a few words at all.)
	if e.ScanLatencyMs < 0 || e.TotalLatencyMs < e.ScanLatencyMs {
		t.Errorf("latency: scan %v ms, total %v ms", e.ScanLatencyMs, e.TotalLatencyMs)
	}
	if e.OrgVerified {
		t.Error("a request without a key is not a verified organisation")
	}
}

func TestRequestEvent_UnknownStaysUnknown(t *testing.T) {
	t.Run("no usage in the answer", func(t *testing.T) {
		g := newOutcomeRig(t, outcomePolicy, jsonAnswer("fine", ""))
		g.post(t, outcomeChat("gpt-4o-mini", "hello there"), nil)
		e := g.requestEvents(t, 1)[0]
		if e.InputTokens != 0 || e.OutputTokens != 0 || e.CostKnown {
			t.Errorf("tokens %d/%d, cost known %v: nothing was reported, nothing is known", e.InputTokens, e.OutputTokens, e.CostKnown)
		}
	})
	t.Run("a model with no price", func(t *testing.T) {
		g := newOutcomeRig(t, outcomePolicy, jsonAnswer("fine", `,"usage":{"prompt_tokens":10,"completion_tokens":5}`))
		g.post(t, outcomeChat("nobody-priced-this-1", "hello there"), nil)
		e := g.requestEvents(t, 1)[0]
		if e.InputTokens != 10 || e.OutputTokens != 5 {
			t.Errorf("tokens = %d/%d, want 10/5", e.InputTokens, e.OutputTokens)
		}
		if e.CostKnown {
			t.Errorf("cost %v is marked known for a model with no price", e.CostUSD)
		}
	})
	t.Run("a blocked request", func(t *testing.T) {
		g := newOutcomeRig(t, outcomePolicy, jsonAnswer("fine", `,"usage":{"prompt_tokens":10,"completion_tokens":5}`))
		resp, _ := g.post(t, outcomeChat("gpt-4o-mini", "my key is AKIAIOSFODNN7EXAMPLE"), nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want the block", resp.StatusCode)
		}
		e := g.requestEvents(t, 1)[0]
		if e.EventType != "request_blocked" || e.InputTokens != 0 || e.CostKnown || e.OutputScanned || e.OutputAction != "" {
			t.Errorf("blocked request's event: %+v", e)
		}
		if e.PolicyName != "bank-policy" || e.PolicyVersion != "7" {
			t.Errorf("policy %q version %q", e.PolicyName, e.PolicyVersion)
		}
	})
}

func TestRequestEvent_OutputFindingsAndAction(t *testing.T) {
	g := newOutcomeRig(t, outcomePolicy, jsonAnswer("the key is AKIAIOSFODNN7EXAMPLE", `,"usage":{"prompt_tokens":10,"completion_tokens":5}`))
	resp, body := g.post(t, outcomeChat("gpt-4o-mini", "what is the key"), nil)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "output_policy_block") {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	e := g.requestEvents(t, 1)[0]
	if e.Action != "PASS" || e.OutputAction != "BLOCK" || !e.OutputScanned || len(e.OutputFindings) == 0 {
		t.Fatalf("event: action %q, output action %q, scanned %v, %d output findings", e.Action, e.OutputAction, e.OutputScanned, len(e.OutputFindings))
	}
	for _, f := range e.OutputFindings {
		if strings.Contains(f.Match, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("the response's secret is in the event unmasked: %q", f.Match)
		}
	}
	if want := scanner.CalculateRisk(e.OutputFindings); e.OutputRisk.Percentage != want.Percentage || want.Percentage == 0 {
		t.Errorf("output risk = %d%%, want %d%% from the findings", e.OutputRisk.Percentage, want.Percentage)
	}
	// The provider was called and answered: its tokens are owed whatever
	// was done with the answer.
	if e.InputTokens != 10 || !e.CostKnown {
		t.Errorf("tokens %d, cost known %v", e.InputTokens, e.CostKnown)
	}
}

func TestRequestEvent_CacheHit(t *testing.T) {
	g := newOutcomeRig(t, outcomePolicy, jsonAnswer("fine", `,"usage":{"prompt_tokens":40,"completion_tokens":2}`))
	g.post(t, outcomeChat("gpt-4o-mini", "same question"), nil)
	resp, _ := g.post(t, outcomeChat("gpt-4o-mini", "same question"), nil)
	if resp.Header.Get("X-Tamga-Cache") != "hit" {
		t.Fatalf("second request: X-Tamga-Cache = %q", resp.Header.Get("X-Tamga-Cache"))
	}
	evs := g.requestEvents(t, 2)
	miss, hit := evs[0], evs[1]
	if miss.CacheStatus != "miss" || miss.InputTokens != 40 || !miss.CostKnown || miss.CostUSD <= 0 {
		t.Errorf("the forwarded request: %q, %d tokens, cost %v", miss.CacheStatus, miss.InputTokens, miss.CostUSD)
	}
	// A hit calls no provider and is not charged to the budget: its cost
	// is a known zero. The cache does not keep the answer's token counts.
	if hit.CacheStatus != "hit" || !hit.CostKnown || hit.CostUSD != 0 || hit.InputTokens != 0 || hit.OutputTokens != 0 {
		t.Errorf("the hit: %q, cost %v (known %v), tokens %d/%d", hit.CacheStatus, hit.CostUSD, hit.CostKnown, hit.InputTokens, hit.OutputTokens)
	}
	if hit.OutputScanned || hit.OutputAction != "" {
		t.Errorf("a hit is served without a scan of its own: scanned %v, action %q", hit.OutputScanned, hit.OutputAction)
	}
	if hit.PolicyName != "bank-policy" {
		t.Errorf("the hit's policy = %q", hit.PolicyName)
	}
}

func TestRequestEvent_Stream(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		up := &streamUpstream{pieceSize: 8, reply: func(string) string { return "The office is open from nine to five." }}
		g := newOutcomeRig(t, outcomePolicy, up.ServeHTTP)
		g.post(t, streamRequest, nil)
		e := g.requestEvents(t, 1)[0]
		// The proxy does not read a stream's usage: unknown, not zero and
		// not the estimate the budget is charged with.
		if e.InputTokens != 0 || e.OutputTokens != 0 || e.CostKnown {
			t.Errorf("stream: tokens %d/%d, cost known %v", e.InputTokens, e.OutputTokens, e.CostKnown)
		}
		if !e.OutputScanned || e.OutputAction != "PASS" || len(e.OutputFindings) != 0 {
			t.Errorf("stream output: scanned %v, action %q, %d findings", e.OutputScanned, e.OutputAction, len(e.OutputFindings))
		}
	})
	t.Run("blocked", func(t *testing.T) {
		up := &streamUpstream{pieceSize: 8, reply: func(string) string {
			return "Sure, the access key is AKIAIOSFODNN7EXAMPLE and nothing else."
		}}
		g := newOutcomeRig(t, outcomePolicy, up.ServeHTTP)
		_, wire := g.post(t, streamRequest, nil)
		if strings.Contains(wire, "AKIAIOSFODNN7EXAMPLE") {
			t.Fatalf("the key reached the client:\n%s", wire)
		}
		// The guard finishes after the last byte went out; the event waits
		// for it, so what the stream scan found is on the request's event.
		e := g.requestEvents(t, 1)[0]
		if e.OutputAction != "BLOCK" || !e.OutputScanned || len(e.OutputFindings) == 0 {
			t.Fatalf("stream output: action %q, scanned %v, %d findings", e.OutputAction, e.OutputScanned, len(e.OutputFindings))
		}
		for _, f := range e.OutputFindings {
			if strings.Contains(f.Match, "AKIAIOSFODNN7EXAMPLE") {
				t.Errorf("the stream's secret is in the event unmasked: %q", f.Match)
			}
		}
	})
}

func TestRequestEvent_OrgVerifiedByKey(t *testing.T) {
	g := newOutcomeRig(t, outcomePolicy, jsonAnswer("fine", ""))
	created, err := g.keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	g.post(t, outcomeChat("gpt-4o-mini", "hello there"), map[string]string{"X-Tamga-Key": created.RawKey, "X-Tamga-Org-Id": "someone-else"})
	e := g.requestEvents(t, 1)[0]
	if e.OrgID != "acme" || !e.OrgVerified {
		t.Errorf("org = %q, verified %v; want the key's organisation, verified", e.OrgID, e.OrgVerified)
	}
}

// The canary token is the one match that is not masked where it is found.
// It is masked before it is recorded.
func TestRequestOutcome_CanaryMatchIsMasked(t *testing.T) {
	o := &requestOutcome{}
	token := "tamga-canary-0123456789abcdef"
	o.recordOutput([]scanner.Finding{{Type: "system_prompt_leak", Category: "system_prompt_leak", Severity: "critical", Match: token}}, policy.ActionBlock, false)
	// A later, milder decision on the same response does not replace it.
	o.recordOutput(nil, policy.ActionPass, true)
	var e events.Event
	o.fill(&e)
	if len(e.OutputFindings) != 1 || strings.Contains(e.OutputFindings[0].Match, token) || e.OutputFindings[0].Match == "" {
		t.Errorf("recorded match = %+v, want the token masked", e.OutputFindings)
	}
	if e.OutputAction != "BLOCK" || !e.OutputScanned {
		t.Errorf("output action = %q, scanned %v", e.OutputAction, e.OutputScanned)
	}
	// A nil outcome is accepted everywhere: publishEvent is also called
	// without one.
	var none *requestOutcome
	none.setUsage(1, 1)
	none.setCost(1)
	none.recordOutput(nil, policy.ActionPass, true)
	none.fill(&e)
}

// A scan that takes a third of a millisecond is recorded as that, not as 0.
func TestDurationMs_KeepsTheFraction(t *testing.T) {
	for d, want := range map[time.Duration]float64{
		347 * time.Microsecond:                    0.347,
		2*time.Millisecond + 500*time.Microsecond: 2.5,
		0: 0,
	} {
		if got := durationMs(d); got != want {
			t.Errorf("durationMs(%v) = %v, want %v", d, got, want)
		}
	}
}
