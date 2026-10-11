package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/upstream"
)

// The ROUTE action end to end: a request that holds what a rule will not
// send to the provider is answered by the organisation's own model server,
// and the provider is never called for it.

const (
	routeTCKN      = "10000000146" // passes the checksum, belongs to no one
	routeEmail     = "ayse.yilmaz@example.com"
	routeCallerKey = "sk-callers-external-provider-key-7f3a"
	routeOwnKey    = "own-model-servers-key-c41d"
	routePoolName  = "internal-llm"
	routeOwnModel  = "internal-model-1"
)

// routePolicy routes a TCKN, masks an email, blocks a secret and strips an
// injection that arrives in a tool result. %s is the internal server.
const routePolicy = `
version: "1.0"
name: route-test
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
    types: [tc_kimlik]
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
    sensitivity: low
  injection_detection:
    action: STRIP
    sensitivity: low
    applies_to: [tool]
exceptions:
  - rule: pii
    roles: [security_lead]
    reason: test
cache:
  enabled: true
  ttl_seconds: 300
output_rules:
  enabled: true
  block_on: [aws_access_key]
  streaming:
    enabled: true
    max_buffer_bytes: 4096
providers:
  allowed: [openai, anthropic, gemini]
  pools:
    internal-llm:
      internal: true
      dialect: openai
      model: internal-model-1
      allow_private_addresses: true
      providers:
        - name: vllm-1
          base_url: %s
          api_key_env: INTERNAL_LLM_API_KEY
          priority: 1
`

// routeUpstream is a model server that counts and keeps what it was sent.
type routeUpstream struct {
	srv    *httptest.Server
	hits   atomic.Int64
	mu     sync.Mutex
	bodies []string
	heads  []http.Header
	paths  []string
	// status, answer and stream shape what it says back.
	status int
	answer string
	stream bool
}

func newRouteUpstream(t *testing.T, answer string) *routeUpstream {
	t.Helper()
	u := &routeUpstream{status: http.StatusOK, answer: answer}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.bodies = append(u.bodies, string(b))
		u.heads = append(u.heads, r.Header.Clone())
		u.paths = append(u.paths, r.URL.Path)
		status, text, stream := u.status, u.answer, u.stream
		u.mu.Unlock()
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fl := w.(http.Flusher)
			for _, p := range pieces(text, 8) {
				_, _ = w.Write(openAIChunk(p))
				fl.Flush()
			}
			_, _ = w.Write(openAIFinish)
			_, _ = w.Write(openAIDone)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		enc, _ := json.Marshal(text)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + string(enc) + `}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *routeUpstream) set(f func(u *routeUpstream)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	f(u)
}

func (u *routeUpstream) lastBody() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.bodies) == 0 {
		return ""
	}
	return u.bodies[len(u.bodies)-1]
}

// seen is everything the server was sent: paths, headers and bodies.
func (u *routeUpstream) seen() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	var b strings.Builder
	for i := range u.bodies {
		b.WriteString(u.paths[i] + "\n")
		_ = u.heads[i].Write(&b)
		b.WriteString(u.bodies[i] + "\n---\n")
	}
	return b.String()
}

type routeRig struct {
	url      string
	external *routeUpstream
	internal *routeUpstream
	pol      *policy.Policy

	mu     sync.Mutex
	events []events.Event
}

type routeOptions struct {
	// policy replaces routePolicy; build makes the policy from the internal
	// server's URL without the loader (what validation would refuse).
	policy string
	build  func(internalURL string) *policy.Policy
	config *config.Config
	// noRegistry leaves the proxy without a pool registry.
	noRegistry bool
}

func newRouteRig(t *testing.T, o routeOptions) *routeRig {
	t.Helper()
	t.Setenv("INTERNAL_LLM_API_KEY", routeOwnKey)
	g := &routeRig{
		external: newRouteUpstream(t, "answer of the outside provider"),
		internal: newRouteUpstream(t, "answer of the internal model"),
	}
	// The operator listed loopback, where the test's internal server is.
	rules := &policy.UpstreamRules{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}
	t.Cleanup(policy.SetUpstreamRules(rules))

	switch {
	case o.build != nil:
		g.pol = o.build(g.internal.srv.URL)
	default:
		y := o.policy
		if y == "" {
			y = routePolicy
		}
		g.pol = mustPolicy(t, fmt.Sprintf(y, g.internal.srv.URL))
	}
	get := func() *policy.Policy { return g.pol }

	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)

	cfg := o.config
	if cfg == nil {
		cfg = &config.Config{}
	}
	registry := testRegistry()
	registry.Register(scanner.NewJailbreakScanner())
	ext, _ := url.Parse(g.external.srv.URL)
	hc := HandlerConfig{
		Registry:     registry,
		GetPolicy:    get,
		UpstreamURLs: map[string]*url.URL{"openai": ext, "anthropic": ext, "gemini": ext},
		Config:       cfg,
		Bus:          bus,
		Cache:        cache.New(64),
	}
	if !o.noRegistry {
		hc.UpstreamRegistry = upstream.NewRegistry(upstream.Options{GetPolicy: get, Egress: upstream.NewEgress(rules)})
	}
	srv := httptest.NewServer(NewHandler(hc))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

type routeAnswer struct {
	status int
	header http.Header
	body   string
}

func (g *routeRig) post(t *testing.T, path, body string, headers map[string]string) routeAnswer {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, g.url+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+routeCallerKey)
	req.Header.Set("OpenAI-Organization", "org-callers-account")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return routeAnswer{resp.StatusCode, resp.Header, string(b)}
}

func (g *routeRig) chat(t *testing.T, content string) routeAnswer {
	t.Helper()
	return g.post(t, "/openai/v1/chat/completions", routeChat(content), nil)
}

func routeChat(content string) string {
	b, _ := json.Marshal(map[string]interface{}{
		"model":    "gpt-4o",
		"messages": []map[string]string{{"role": "user", "content": content}},
	})
	return string(b)
}

// lastEvent waits for the n-th request event and returns it.
func (g *routeRig) lastEvent(t *testing.T, n int) events.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		g.mu.Lock()
		var own []events.Event
		for _, e := range g.events {
			if e.EventType == "request_scanned" || e.EventType == "request_blocked" {
				own = append(own, e)
			}
		}
		g.mu.Unlock()
		if len(own) >= n {
			return own[n-1]
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d request events published, want %d", len(own), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (g *routeRig) wantHits(t *testing.T, external, internal int64) {
	t.Helper()
	if n := g.external.hits.Load(); n != external {
		t.Fatalf("the outside provider was called %d times, want %d:\n%s", n, external, g.external.seen())
	}
	if n := g.internal.hits.Load(); n != internal {
		t.Fatalf("the internal server was called %d times, want %d", n, internal)
	}
}

// The acceptance test of the plan: a request with a TCKN under a ROUTE rule
// never reaches the outside provider, the internal server gets it as it was
// written, and the caller gets 200.
func TestRoute_TCKNGoesToTheInternalPoolAndNowhereElse(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	text := "Müşterinin TC kimlik numarası " + routeTCKN + ", e-postası " + routeEmail + "."
	got := g.chat(t, text)

	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", got.status, got.body)
	}
	g.wantHits(t, 0, 1)
	if got.header.Get(routedToHeader) != routePoolName {
		t.Errorf("%s = %q, want %q", routedToHeader, got.header.Get(routedToHeader), routePoolName)
	}
	if !strings.Contains(got.body, "answer of the internal model") {
		t.Errorf("the answer is not the internal model's: %s", got.body)
	}

	// As written: nothing masked, though the email is under a REDACT rule
	// (ROUTE beats REDACT). Only the model's name is the pool's.
	var sent struct {
		Model    string `json:"model"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(g.internal.lastBody()), &sent); err != nil {
		t.Fatalf("the routed body is not JSON: %v: %s", err, g.internal.lastBody())
	}
	if sent.Model != routeOwnModel {
		t.Errorf("model = %q, want the pool's %q", sent.Model, routeOwnModel)
	}
	if len(sent.Messages) != 1 || sent.Messages[0].Content != text {
		t.Errorf("the text was changed on the way: %+v", sent.Messages)
	}
	if g.internal.paths[0] != "/v1/chat/completions" {
		t.Errorf("path = %q", g.internal.paths[0])
	}

	// The caller's key is for the outside provider. The internal server
	// gets the pool's own and nothing of the caller's account.
	seen := g.internal.seen()
	if strings.Contains(seen, routeCallerKey) || strings.Contains(seen, "org-callers-account") {
		t.Fatalf("the caller's provider credential reached the internal server:\n%s", seen)
	}
	if auth := g.internal.heads[0].Get("Authorization"); auth != "Bearer "+routeOwnKey {
		t.Errorf("Authorization at the internal server = %q, want the pool's key", auth)
	}
	for name := range g.internal.heads[0] {
		if strings.HasPrefix(strings.ToLower(name), "x-tamga-") && name != "X-Tamga-Request-Id" {
			t.Errorf("header %s was sent to the internal server", name)
		}
	}

	e := g.lastEvent(t, 1)
	if e.Action != "ROUTE" || e.EventType != "request_scanned" || e.RoutePool != routePoolName {
		t.Errorf("event = action %q, type %q, pool %q", e.Action, e.EventType, e.RoutePool)
	}
	if len(e.Findings) == 0 || e.Model != routeOwnModel || e.Provider != "openai" {
		t.Errorf("event = %d findings, model %q, provider %q", len(e.Findings), e.Model, e.Provider)
	}
	// The provider's price list does not apply to the organisation's servers.
	if e.CostKnown || e.InputTokens != 7 || e.OutputTokens != 3 {
		t.Errorf("event cost known = %v, tokens %d/%d; want an unknown cost and the tokens the server reported", e.CostKnown, e.InputTokens, e.OutputTokens)
	}
	if j := events.EventToJSON(e); j.RoutePool != routePoolName {
		t.Errorf("the console's event has route_pool %q", j.RoutePool)
	}
}

func TestRoute_CleanRequestGoesToTheProvider(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	got := g.chat(t, "What is the capital of France?")
	if got.status != http.StatusOK || got.header.Get(routedToHeader) != "" {
		t.Fatalf("status %d, %s %q", got.status, routedToHeader, got.header.Get(routedToHeader))
	}
	g.wantHits(t, 1, 0)
	if !strings.Contains(g.external.seen(), routeCallerKey) {
		t.Error("the caller's key did not reach the provider it is for")
	}
	if e := g.lastEvent(t, 1); e.Action != "PASS" || e.RoutePool != "" {
		t.Errorf("event = action %q, pool %q", e.Action, e.RoutePool)
	}
}

// The pool is down: the request is blocked. It is not retried against the
// provider, however often the caller tries.
func TestRoute_PoolDownBlocksAndNeverFallsBack(t *testing.T) {
	for _, tc := range []struct {
		name string
		down func(g *routeRig)
	}{
		{"connection refused", func(g *routeRig) { g.internal.srv.Close() }},
		{"the server answers 500", func(g *routeRig) {
			g.internal.set(func(u *routeUpstream) { u.status = http.StatusInternalServerError })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newRouteRig(t, routeOptions{config: &config.Config{UpstreamMaxRetries: 1}})
			tc.down(g)
			for i := 1; i <= 8; i++ {
				got := g.chat(t, "TC kimlik: "+routeTCKN)
				if got.status != http.StatusServiceUnavailable {
					t.Fatalf("status = %d, want 503: %s", got.status, got.body)
				}
				var body struct {
					Error struct{ Code, Type, Reason string } `json:"error"`
				}
				if err := json.Unmarshal([]byte(got.body), &body); err != nil || body.Error.Code != routeUnavailableCode || body.Error.Reason != upstream.RouteUpstream {
					t.Fatalf("body = %s (%v)", got.body, err)
				}
				if got.header.Get("Retry-After") == "" || got.header.Get(routedToHeader) != "" {
					t.Errorf("Retry-After %q, %s %q", got.header.Get("Retry-After"), routedToHeader, got.header.Get(routedToHeader))
				}
				if strings.Contains(got.body, routeTCKN) || strings.Contains(got.body, g.internal.srv.URL) {
					t.Fatalf("the refusal repeats the text or the address: %s", got.body)
				}
				if n := g.external.hits.Load(); n != 0 {
					t.Fatalf("attempt %d: the outside provider was called %d times", i, n)
				}
				e := g.lastEvent(t, i)
				if e.Action != "BLOCK" || e.EventType != "request_blocked" || e.RoutePool != routePoolName {
					t.Fatalf("event = action %q, type %q, pool %q", e.Action, e.EventType, e.RoutePool)
				}
			}
			if upstream.RouteUnavailableStats()[[2]string{routePoolName, upstream.RouteUpstream}] < 8 {
				t.Error("the refusals were not counted")
			}
		})
	}
}

func TestRoute_BlockBeatsRoute(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	got := g.chat(t, "TC kimlik "+routeTCKN+" ve anahtar AKIAIOSFODNN7EXAMPLE")
	if got.status != http.StatusForbidden || !strings.Contains(got.body, "security_violation") {
		t.Fatalf("status = %d, want the 403 of a blocked request: %s", got.status, got.body)
	}
	g.wantHits(t, 0, 0)
	if e := g.lastEvent(t, 1); e.Action != "BLOCK" || e.RoutePool != "" {
		t.Errorf("event = action %q, pool %q", e.Action, e.RoutePool)
	}
}

// Without the TCKN the email is masked and sent to the provider; with it the
// request is routed and the email is left alone.
func TestRoute_BeatsRedact(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	if got := g.chat(t, "write to "+routeEmail); got.status != http.StatusOK || got.header.Get(routedToHeader) != "" {
		t.Fatalf("email alone: status %d, routed to %q", got.status, got.header.Get(routedToHeader))
	}
	g.wantHits(t, 1, 0)
	if strings.Contains(g.external.lastBody(), routeEmail) {
		t.Fatalf("the email reached the provider unmasked: %s", g.external.lastBody())
	}

	if got := g.chat(t, "write to "+routeEmail+", TC "+routeTCKN); got.status != http.StatusOK || got.header.Get(routedToHeader) != routePoolName {
		t.Fatalf("with a TCKN: status %d, routed to %q", got.status, got.header.Get(routedToHeader))
	}
	g.wantHits(t, 1, 1)
	if body := g.internal.lastBody(); !strings.Contains(body, routeEmail) || !strings.Contains(body, routeTCKN) {
		t.Fatalf("the routed request was masked: %s", body)
	}
	if strings.Contains(g.external.seen(), routeTCKN) {
		t.Fatal("the TCKN reached the provider")
	}
}

// A tool result that carries an injection is still stripped on the way to
// the internal model: it is as open to the attack as any other.
func TestRoute_StripStillAppliesToARoutedRequest(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	body, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Müşteri TC " + routeTCKN + " için hava durumunu getir."},
			{"role": "assistant", "content": nil, "tool_calls": []map[string]interface{}{
				{"id": "call_1", "type": "function", "function": map[string]string{"name": "weather", "arguments": "{}"}}}},
			{"role": "tool", "tool_call_id": "call_1", "content": "Sunny. " + planted},
		},
	})
	got := g.post(t, "/openai/v1/chat/completions", string(body), nil)
	if got.status != http.StatusOK || got.header.Get(routedToHeader) != routePoolName {
		t.Fatalf("status %d, routed to %q: %s", got.status, got.header.Get(routedToHeader), got.body)
	}
	g.wantHits(t, 0, 1)
	sent := g.internal.lastBody()
	if strings.Contains(sent, "Ignore all previous instructions") {
		t.Fatalf("the injection reached the internal model: %s", sent)
	}
	if !strings.Contains(sent, stripPlaceholder) || !strings.Contains(sent, routeTCKN) {
		t.Fatalf("want the tool result replaced and the TCKN as written: %s", sent)
	}
	if got.header.Get("X-Tamga-Stripped-Count") != "1" {
		t.Errorf("X-Tamga-Stripped-Count = %q", got.header.Get("X-Tamga-Stripped-Count"))
	}
}

// When the strip cannot be carried out (the body is not read by segment) the
// routed request is blocked, as a STRIP verdict is.
func TestRoute_StripThatCannotBeDoneBlocks(t *testing.T) {
	y := strings.Replace(routePolicy, "    applies_to: [tool]\n", "", 1)
	g := newRouteRig(t, routeOptions{policy: y})
	// Not a shape the extractor knows: scanned raw.
	got := g.post(t, "/openai/v1/other", `{"note":"TC `+routeTCKN+`. `+planted+`"}`, nil)
	if got.status != http.StatusForbidden || got.header.Get("X-Tamga-Strip-Fallback") == "" {
		t.Fatalf("status %d, fallback %q: %s", got.status, got.header.Get("X-Tamga-Strip-Fallback"), got.body)
	}
	g.wantHits(t, 0, 0)
}

func TestRoute_Streaming(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	g.internal.set(func(u *routeUpstream) { u.stream = true; u.answer = "streamed by the internal model" })
	b, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o", "stream": true,
		"messages": []map[string]string{{"role": "user", "content": "TC " + routeTCKN}},
	})
	got := g.post(t, "/openai/v1/chat/completions", string(b), nil)
	if got.status != http.StatusOK || got.header.Get(routedToHeader) != routePoolName {
		t.Fatalf("status %d, routed to %q: %s", got.status, got.header.Get(routedToHeader), got.body)
	}
	if !strings.HasPrefix(got.header.Get("Content-Type"), "text/event-stream") || got.header.Get("X-Tamga-Stream-Scan") != "enabled" {
		t.Errorf("Content-Type %q, stream scan %q", got.header.Get("Content-Type"), got.header.Get("X-Tamga-Stream-Scan"))
	}
	var text strings.Builder
	for _, ev := range strings.Split(got.body, "\n\n") {
		text.WriteString(openAIProvider{}.ExtractStreamDeltaText([]byte(ev)))
	}
	if text.String() != "streamed by the internal model" || !strings.Contains(got.body, "[DONE]") {
		t.Fatalf("stream = %q:\n%s", text.String(), got.body)
	}
	g.wantHits(t, 0, 1)
	if !strings.Contains(g.internal.lastBody(), `"stream":true`) {
		t.Errorf("the routed body lost its stream flag: %s", g.internal.lastBody())
	}
	if e := g.lastEvent(t, 1); e.Action != "ROUTE" || e.RoutePool != routePoolName {
		t.Errorf("event = action %q, pool %q", e.Action, e.RoutePool)
	}
}

// The answer of the internal model is scanned by output_rules like any other.
func TestRoute_AnswerIsScanned(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	g.internal.set(func(u *routeUpstream) { u.answer = "the key is AKIAIOSFODNN7EXAMPLE" })
	got := g.chat(t, "TC "+routeTCKN)
	if got.status != http.StatusForbidden || !strings.Contains(got.body, "output_policy_block") {
		t.Fatalf("status %d: %s", got.status, got.body)
	}
	if strings.Contains(got.body, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal("the answer's secret reached the caller")
	}
	g.wantHits(t, 0, 1)
}

// The cache keeps the two apart. The same body, under the same policy and
// the same provider credential, goes to the provider for a role the rule is
// waived for and to the internal pool for everyone else; neither is served
// the other's stored answer.
func TestRoute_CacheKeepsRoutedAndProviderAnswersApart(t *testing.T) {
	g := newRouteRig(t, routeOptions{config: &config.Config{TrustRoleHeader: true}})
	body := routeChat("TC kimlik numarası " + routeTCKN)
	exempt := map[string]string{"X-Tamga-Role": "security_lead"}

	first := g.post(t, "/openai/v1/chat/completions", body, exempt)
	if first.status != http.StatusOK || first.header.Get(routedToHeader) != "" || !strings.Contains(first.body, "outside provider") {
		t.Fatalf("exempt role: status %d, routed to %q: %s", first.status, first.header.Get(routedToHeader), first.body)
	}
	g.wantHits(t, 1, 0)

	// The provider's answer is in the cache now. A caller the rule covers
	// must not be given it.
	routed := g.post(t, "/openai/v1/chat/completions", body, nil)
	if routed.header.Get("X-Tamga-Cache") == "hit" || !strings.Contains(routed.body, "internal model") || routed.header.Get(routedToHeader) != routePoolName {
		t.Fatalf("routed request: cache %q, routed to %q: %s", routed.header.Get("X-Tamga-Cache"), routed.header.Get(routedToHeader), routed.body)
	}
	g.wantHits(t, 1, 1)

	// Each is served its own entry afterwards.
	again := g.post(t, "/openai/v1/chat/completions", body, nil)
	if again.header.Get("X-Tamga-Cache") != "hit" || !strings.Contains(again.body, "internal model") || again.header.Get(routedToHeader) != routePoolName {
		t.Fatalf("routed again: cache %q, routed to %q: %s", again.header.Get("X-Tamga-Cache"), again.header.Get(routedToHeader), again.body)
	}
	back := g.post(t, "/openai/v1/chat/completions", body, exempt)
	if back.header.Get("X-Tamga-Cache") != "hit" || !strings.Contains(back.body, "outside provider") || back.header.Get(routedToHeader) != "" {
		t.Fatalf("exempt again: cache %q, routed to %q: %s", back.header.Get("X-Tamga-Cache"), back.header.Get(routedToHeader), back.body)
	}
	g.wantHits(t, 1, 1)
	if e := g.lastEvent(t, 3); e.Action != "ROUTE" || e.CacheStatus != "hit" || e.RoutePool != routePoolName {
		t.Errorf("event of the routed hit = action %q, cache %q, pool %q", e.Action, e.CacheStatus, e.RoutePool)
	}
}

func TestRoute_CacheKeyNamesThePool(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+routeCallerKey)
	parts := cache.KeyParts{
		Org: "o", Identity: cache.New(1).Identity(r, "", false), Provider: "openai", Model: "m",
		Path: "/v1/chat/completions", Body: []byte(`{}`), Policy: "p", Scan: "s",
	}
	plain := cache.Key(parts)
	parts.Route = routePoolName
	routed := cache.Key(parts)
	parts.Route = "another-pool"
	if plain == "" || plain == routed || routed == cache.Key(parts) {
		t.Fatalf("keys: provider %q, routed %q, another pool %q", plain, routed, cache.Key(parts))
	}
}

// A route whose callers speak another API than the pool's servers is not
// routed, and not sent to the provider either.
func TestRoute_OtherDialectIsUnavailable(t *testing.T) {
	g := newRouteRig(t, routeOptions{})
	anthropic, _ := json.Marshal(map[string]interface{}{
		"model": "claude-sonnet-4-20250514", "max_tokens": 64,
		"messages": []map[string]string{{"role": "user", "content": "TC " + routeTCKN}},
	})
	for _, tc := range []struct{ path, body string }{
		{"/anthropic/v1/messages", string(anthropic)},
		{"/gemini/v1beta/models/gemini-2.0-flash:generateContent", `{"contents":[{"role":"user","parts":[{"text":"TC ` + routeTCKN + `"}]}]}`},
	} {
		got := g.post(t, tc.path, tc.body, nil)
		if got.status != http.StatusServiceUnavailable || !strings.Contains(got.body, routeUnavailableCode) || !strings.Contains(got.body, `"reason":"dialect"`) {
			t.Fatalf("%s: status %d: %s", tc.path, got.status, got.body)
		}
		// Sending it again cannot end differently.
		if got.header.Get("Retry-After") != "" {
			t.Errorf("%s: Retry-After = %q, want none", tc.path, got.header.Get("Retry-After"))
		}
	}
	g.wantHits(t, 0, 0)
	if e := g.lastEvent(t, 1); e.Action != "BLOCK" || e.EventType != "request_blocked" || e.RoutePool != routePoolName {
		t.Errorf("event = action %q, type %q, pool %q", e.Action, e.EventType, e.RoutePool)
	}
}

// Policies the loader refuses, in force anyway. Each ROUTE blocks; none
// reaches the provider, and none reaches a pool that is not internal.
func TestRoute_PolicyThatShouldNotHaveLoadedFailsClosed(t *testing.T) {
	routeRule := func(to string) map[string]policy.Rule {
		return map[string]policy.Rule{"pii": {Action: policy.ActionRoute, RouteTo: to, Sensitivity: "low"}}
	}
	ep := func(u string) []policy.ProviderUpstreamEndpoint {
		return []policy.ProviderUpstreamEndpoint{{Name: "a", BaseURL: u, Priority: 1}}
	}
	for _, tc := range []struct {
		name       string
		build      func(internalURL string) *policy.Policy
		noRegistry bool
	}{
		{"ROUTE to a pool that is not internal", func(u string) *policy.Policy {
			return &policy.Policy{Version: "1.0", Name: "x", Rules: routeRule("openai"), Providers: &policy.Providers{
				Pools: map[string]policy.ProviderUpstreamPool{"openai": {Endpoints: ep(u)}}}}
		}, false},
		{"ROUTE with no pool named", func(string) *policy.Policy {
			return &policy.Policy{Version: "1.0", Name: "x", Rules: routeRule("")}
		}, false},
		{"ROUTE to a pool that is not defined", func(string) *policy.Policy {
			return &policy.Policy{Version: "1.0", Name: "x", Rules: routeRule("gone")}
		}, false},
		{"no pool registry in the proxy", func(u string) *policy.Policy {
			return &policy.Policy{Version: "1.0", Name: "x", Rules: routeRule(routePoolName), Providers: &policy.Providers{
				Pools: map[string]policy.ProviderUpstreamPool{routePoolName: {Internal: true, Dialect: policy.DialectOpenAI, AllowPrivateAddresses: true, Endpoints: ep(u)}}}}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newRouteRig(t, routeOptions{build: tc.build, noRegistry: tc.noRegistry})
			got := g.chat(t, "TC "+routeTCKN)
			if got.status != http.StatusServiceUnavailable || !strings.Contains(got.body, routeUnavailableCode) || !strings.Contains(got.body, `"reason":"no_pool"`) {
				t.Fatalf("status %d: %s", got.status, got.body)
			}
			g.wantHits(t, 0, 0)
		})
	}

	// The last line of defence: the transport does not send a routed
	// request without its internal pool.
	tr := &resilientTransport{base: defaultStreamingTransport(), routed: true}
	req := httptest.NewRequest(http.MethodPost, "http://example.invalid/v1/chat/completions", bytes.NewReader([]byte(`{}`)))
	if _, err := tr.RoundTrip(req); err == nil {
		t.Fatal("a routed request without a pool was sent")
	}
}

func TestRoute_OfflineModeNamesThePool(t *testing.T) {
	g := newRouteRig(t, routeOptions{config: &config.Config{MockUpstream: true}})
	got := g.chat(t, "TC "+routeTCKN)
	if got.status != http.StatusOK || got.header.Get(routedToHeader) != routePoolName {
		t.Fatalf("status %d, routed to %q", got.status, got.header.Get(routedToHeader))
	}
	g.wantHits(t, 0, 0)
}

func TestRewriteModel(t *testing.T) {
	for _, tc := range []struct{ name, body, model, want string }{
		{"the model is replaced, the rest is kept byte for byte",
			"{ \"model\" :  \"gpt-4o\",\n \"messages\":[{\"role\":\"user\",\"content\":\"model \\\"gpt-4o\\\" \\u00e7\"}], \"n\": 1.0 }", "own",
			"{ \"model\" :  \"own\",\n \"messages\":[{\"role\":\"user\",\"content\":\"model \\\"gpt-4o\\\" \\u00e7\"}], \"n\": 1.0 }"},
		{"a nested model is left alone", `{"metadata":{"model":"x"},"model":"gpt-4o"}`, "own", `{"metadata":{"model":"x"},"model":"own"}`},
		{"a name that needs escaping", `{"model":"a"}`, `o"w\n`, `{"model":"o\"w\\n"}`},
		{"every top-level copy", `{"model":"a","x":1,"model":"b"}`, "own", `{"model":"own","x":1,"model":"own"}`},
		{"no model named", `{"messages":[]}`, "own", `{"messages":[]}`},
		{"a model that is not a string", `{"model":null}`, "own", `{"model":null}`},
		{"no override", `{"model":"a"}`, "", `{"model":"a"}`},
		{"not an object", `["model","a"]`, "own", `["model","a"]`},
		{"not JSON", `model=a`, "own", `model=a`},
		{"cut short", `{"model":"a"`, "own", `{"model":"a"`},
	} {
		if got := string(rewriteModel([]byte(tc.body), tc.model)); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, got, tc.want)
		}
	}
}

const routeScanPolicy = `
version: "3.2"
name: route-scan-test
rules:
  pii:
    action: ROUTE
    route_to: internal-llm
    sensitivity: low
    types: [tc_kimlik]
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
    sensitivity: low
providers:
  allowed: [openai]
  pools:
    internal-llm:
      internal: true
      dialect: openai
      providers:
        - name: own-1
          base_url: https://llm.corp.example:8000
          priority: 1
`

func routeScanRig(t *testing.T) *scanRig {
	t.Helper()
	t.Cleanup(policy.SetUpstreamRules(&policy.UpstreamRules{AllowHostPorts: []string{"llm.corp.example:8000"}}))
	return newScanRig(t, scanRigOptions{policy: routeScanPolicy})
}

// POST /api/v1/scan reports ROUTE and the pool. It sends nothing anywhere.
func TestRoute_ScanReportsTheVerdictAndThePool(t *testing.T) {
	g := routeScanRig(t)
	resp, raw := g.scanRaw(t, scanAdmin(), scanBody("TC kimlik "+routeTCKN+", "+routeEmail, ""))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		Action       string  `json:"action"`
		RouteTo      string  `json:"route_to"`
		RedactedText *string `json:"redacted_text"`
		Findings     []struct{ Category, Action string }
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.Action != "ROUTE" || out.RouteTo != routePoolName || out.RedactedText != nil {
		t.Fatalf("scan = %s", raw)
	}
	perFinding := map[string]string{}
	for _, f := range out.Findings {
		perFinding[f.Category] = f.Action
	}
	if perFinding["tc_kimlik"] != "ROUTE" || perFinding["email"] != "REDACT" {
		t.Errorf("per-finding actions = %v", perFinding)
	}
	if g.upstreamBody() != "" {
		t.Error("a scan called a provider")
	}

	// Any other verdict names no pool.
	if _, raw := g.scan(t, "write to "+routeEmail, ""); strings.Contains(raw, "route_to") {
		t.Errorf("a REDACT verdict names a pool: %s", raw)
	}
}

// The LiteLLM contract has no action that sends a request elsewhere, and
// LiteLLM would forward a request Tamga lets through to its own provider. A
// request the policy routes is therefore blocked over the bridge, and the
// reason says so.
func TestRoute_LiteLLMBridgeBlocks(t *testing.T) {
	g := routeScanRig(t)
	out, resp, raw := g.litellm(t, "request", []string{"You are a helpful assistant.", "TC kimlik " + routeTCKN}, nil)
	if out.Action != "BLOCKED" {
		t.Fatalf("action = %q, want BLOCKED: %s", out.Action, raw)
	}
	if !strings.Contains(out.BlockedReason, "routing is not possible over the LiteLLM bridge") {
		t.Fatalf("blocked_reason does not say why: %q", out.BlockedReason)
	}
	if resp.Header.Get("X-Tamga-Route-Fallback") != "block:bridge" {
		t.Errorf("X-Tamga-Route-Fallback = %q", resp.Header.Get("X-Tamga-Route-Fallback"))
	}
	if strings.Contains(raw, routeTCKN) {
		t.Fatal("the reply repeats the number")
	}
	evs := g.bridgeEvents(t, 1)
	if evs[0].Action != "BLOCK" || evs[0].EventType != "request_blocked" || evs[0].RoutePool != routePoolName {
		t.Errorf("event = action %q, type %q, pool %q", evs[0].Action, evs[0].EventType, evs[0].RoutePool)
	}

	// A rule's own BLOCK keeps its own reason.
	out, _, raw = g.litellm(t, "request", []string{"key AKIAIOSFODNN7EXAMPLE and TC " + routeTCKN}, nil)
	if out.Action != "BLOCKED" || strings.Contains(out.BlockedReason, "routing") || !strings.Contains(out.BlockedReason, "secret") {
		t.Fatalf("a blocked secret: %s", raw)
	}
	// And a request the rules only mask is still masked.
	out, _, raw = g.litellm(t, "request", []string{"write to " + routeEmail}, nil)
	if out.Action != "GUARDRAIL_INTERVENED" || strings.Contains(raw, routeEmail) {
		t.Fatalf("a masked email: %s", raw)
	}
}
