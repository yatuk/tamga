package proxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
)

// cachePolicy is the cache with input redaction and output scanning on. The
// two verbs are the TTL and extra top-level YAML.
const cachePolicy = `
version: "1.0"
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
providers:
  allowed: [openai, gemini]
cache:
  enabled: true
  ttl_seconds: %d
output_rules:
  enabled: true
  block_on: [aws_access_key]
  redact_on: [phone_tr]
%s
`

// Two provider credentials. Neither is a real key.
const (
	credA = "Bearer sk-test-cache-aaaaaaaaaaaaaaaa"
	credB = "Bearer sk-test-cache-bbbbbbbbbbbbbbbb"
)

type cacheRig struct {
	url   string
	cache *cache.Cache
	keys  *apikeys.MemoryStore

	mu     sync.Mutex
	calls  int
	bodies []string
	events []events.Event
	// answer makes the provider's reply from the call number and the request
	// body. The default says which call it was, so a served answer shows
	// which request it was produced for.
	answer func(n int, body string) string
	// gzip makes the provider compress its reply.
	gzip bool
}

func newCacheRig(t *testing.T, policyYAML string, cfg *config.Config) *cacheRig {
	t.Helper()
	g := &cacheRig{cache: cache.New(64), keys: apikeys.NewMemoryStore()}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.calls++
		n := g.calls
		g.bodies = append(g.bodies, string(raw))
		answer, zip := g.answer, g.gzip
		g.mu.Unlock()
		text := "answer " + strconv.Itoa(n)
		if answer != nil {
			text = answer(n, string(raw))
		}
		out := []byte(`{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(text) + `}}]}`)
		w.Header().Set("Content-Type", "application/json")
		if zip {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(out)
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			out = buf.Bytes()
		}
		_, _ = w.Write(out)
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	bus := events.NewBus()
	bus.Subscribe(func(e events.Event) {
		if e.EventType == "output_scan_hint" {
			return
		}
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)

	if cfg == nil {
		cfg = &config.Config{}
	}
	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     testRegistry(),
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u, "gemini": u},
		Config:       cfg,
		Cache:        g.cache,
		Keys:         g.keys,
		Bus:          bus,
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

func defaultCachePolicy() string { return fmt.Sprintf(cachePolicy, 300, "") }

func cacheChat(content string) string {
	return `{"model":"gpt-4o-mini","messages":[{"role":"user","content":` + strconv.Quote(content) + `}]}`
}

type cacheReply struct {
	status int
	cache  string
	body   string
	header http.Header
}

func (g *cacheRig) postTo(t *testing.T, path, body string, headers map[string]string) cacheReply {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, g.url+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return cacheReply{status: resp.StatusCode, cache: resp.Header.Get("X-Tamga-Cache"), body: string(b), header: resp.Header}
}

func (g *cacheRig) post(t *testing.T, content string, headers map[string]string) cacheReply {
	t.Helper()
	return g.postTo(t, "/v1/chat/completions", cacheChat(content), headers)
}

func (g *cacheRig) upstreamCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *cacheRig) size() int {
	_, _, n := g.cache.Stats()
	return n
}

// requestEvents waits for n request events and returns them in order.
func (g *cacheRig) requestEvents(t *testing.T, n int) []events.Event {
	t.Helper()
	var out []events.Event
	eventually(t, strconv.Itoa(n)+" request events", func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		out = out[:0]
		for _, e := range g.events {
			if e.EventType == "request_scanned" {
				out = append(out, e)
			}
		}
		return len(out) >= n
	})
	return out
}

func (g *cacheRig) outputEvents() []events.Event {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []events.Event
	for _, e := range g.events {
		if e.EventType == "output_scanned" {
			out = append(out, e)
		}
	}
	return out
}

func wantReply(t *testing.T, what string, got cacheReply, status int, cacheHdr, bodyHas string) {
	t.Helper()
	if got.status != status || got.cache != cacheHdr || !strings.Contains(got.body, bodyHas) {
		t.Fatalf("%s: got %d, X-Tamga-Cache=%q, body %s; want %d, %q, body with %q",
			what, got.status, got.cache, got.body, status, cacheHdr, bodyHas)
	}
}

// The finding: the caller was not part of the key, so a second caller in the
// organisation was answered without the provider ever seeing its credential.
func TestCache_AnotherCredentialIsNotServed(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	org := map[string]string{"X-Tamga-Org-Id": "acme"}

	first := g.post(t, "what is the plan", map[string]string{"Authorization": credA, "X-Tamga-Org-Id": "acme"})
	wantReply(t, "first caller", first, http.StatusOK, "miss", "answer 1")

	second := g.post(t, "what is the plan", map[string]string{"Authorization": credB, "X-Tamga-Org-Id": "acme"})
	wantReply(t, "second caller, another credential", second, http.StatusOK, "miss", "answer 2")
	if g.upstreamCalls() != 2 {
		t.Fatalf("the provider saw %d requests, want 2: the second credential was not checked", g.upstreamCalls())
	}

	// The same key in the header another provider reads is not the same
	// credential either.
	third := g.post(t, "what is the plan", map[string]string{"X-Api-Key": credA, "X-Tamga-Org-Id": "acme"})
	wantReply(t, "same value, another header", third, http.StatusOK, "miss", "answer 3")

	// No credential at all, naming the organisation.
	none := g.post(t, "what is the plan", org)
	wantReply(t, "no credential", none, http.StatusOK, "miss", "answer 4")
}

func TestCache_SameCredentialIsAHit(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	h := map[string]string{"Authorization": credA}

	wantReply(t, "first", g.post(t, "hello there", h), http.StatusOK, "miss", "answer 1")
	wantReply(t, "second", g.post(t, "hello there", h), http.StatusOK, "hit", "answer 1")
	if g.upstreamCalls() != 1 {
		t.Fatalf("the provider saw %d requests, want 1", g.upstreamCalls())
	}
	// A different question is a different entry.
	wantReply(t, "another body", g.post(t, "hello again", h), http.StatusOK, "miss", "answer 2")
}

// A request that names nobody never reads the cache and never writes it,
// whatever organisation it says it is from.
func TestCache_AnonymousWithoutCredentialNeverUsesTheCache(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)

	for i := 1; i <= 3; i++ {
		got := g.post(t, "status report", nil)
		wantReply(t, "anonymous", got, http.StatusOK, "miss", "answer "+strconv.Itoa(i))
	}
	if g.size() != 0 {
		t.Fatalf("an anonymous request stored %d entries", g.size())
	}

	// An application with a key in organisation acme caches the answer...
	key, _ := g.keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	app := map[string]string{keyHeader: key.RawKey}
	wantReply(t, "the application", g.post(t, "status report", app), http.StatusOK, "miss", "answer 4")
	wantReply(t, "the application again", g.post(t, "status report", app), http.StatusOK, "hit", "answer 4")
	// ...and so does one that only has a provider credential and says acme.
	cred := map[string]string{"Authorization": credA, "X-Tamga-Org-Id": "acme"}
	wantReply(t, "credential holder", g.post(t, "status report", cred), http.StatusOK, "miss", "answer 5")
	stored := g.size()

	// The anonymous caller replays the body and names the organisation.
	got := g.post(t, "status report", map[string]string{"X-Tamga-Org-Id": "acme"})
	wantReply(t, "anonymous naming acme", got, http.StatusOK, "miss", "answer 6")
	if g.size() != stored {
		t.Fatalf("the anonymous request changed the cache: %d entries, was %d", g.size(), stored)
	}
}

// The organisation in the key is the key's, not the header's: the same
// credential under keys of two organisations shares nothing, and a header
// cannot move a request into another organisation's entries.
func TestCache_NoHitAcrossOrganisations(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	acme, _ := g.keys.CreateWith(apikeys.Params{Label: "acme-app", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	acme2, _ := g.keys.CreateWith(apikeys.Params{Label: "acme-other", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	globex, _ := g.keys.CreateWith(apikeys.Params{Label: "globex-app", Scope: apikeys.ScopeProxy, OrgID: "globex"})

	with := func(k apikeys.CreatedKey, extra map[string]string) map[string]string {
		h := map[string]string{keyHeader: k.RawKey, "Authorization": credA}
		for name, v := range extra {
			h[name] = v
		}
		return h
	}
	wantReply(t, "acme", g.post(t, "quarterly numbers", with(acme, nil)), http.StatusOK, "miss", "answer 1")
	wantReply(t, "globex, same credential", g.post(t, "quarterly numbers", with(globex, nil)), http.StatusOK, "miss", "answer 2")
	wantReply(t, "globex claiming acme", g.post(t, "quarterly numbers", with(globex, map[string]string{"X-Tamga-Org-Id": "acme"})), http.StatusOK, "hit", "answer 2")
	// No key, the same credential, and a header that says acme: the claim
	// does not reach what a verified acme caller stored.
	anon := g.post(t, "quarterly numbers", map[string]string{"Authorization": credA, "X-Tamga-Org-Id": "acme"})
	wantReply(t, "unverified, claiming acme", anon, http.StatusOK, "miss", "answer 3")

	// Two keys of one organisation that present the same provider credential
	// do share: the provider would have answered either.
	wantReply(t, "acme, another key, same credential", g.post(t, "quarterly numbers", with(acme2, nil)), http.StatusOK, "hit", "answer 1")

	// Without a provider credential the Tamga key is who the answer is for,
	// and another key of the same organisation is somebody else.
	only := func(k apikeys.CreatedKey) map[string]string { return map[string]string{keyHeader: k.RawKey} }
	wantReply(t, "acme key, no credential", g.post(t, "quarterly numbers", only(acme)), http.StatusOK, "miss", "answer 4")
	wantReply(t, "the same key again", g.post(t, "quarterly numbers", only(acme)), http.StatusOK, "hit", "answer 4")
	wantReply(t, "another acme key, no credential", g.post(t, "quarterly numbers", only(acme2)), http.StatusOK, "miss", "answer 5")
	wantReply(t, "globex key, no credential", g.post(t, "quarterly numbers", only(globex)), http.StatusOK, "miss", "answer 6")
}

// A response the output scan blocks or redacts is never stored, in either
// form. The next identical request goes to the provider and is scanned again,
// so each one is decided by the policy in force and each one is an event.
func TestCache_BlockedOrRedactedOutputIsScannedAgain(t *testing.T) {
	h := map[string]string{"Authorization": credA}

	t.Run("blocked", func(t *testing.T) {
		g := newCacheRig(t, defaultCachePolicy(), nil)
		g.answer = func(int, string) string { return "the key is AKIAIOSFODNN7EXAMPLE" }
		for i := 1; i <= 2; i++ {
			got := g.post(t, "what was the key", h)
			wantReply(t, "attempt "+strconv.Itoa(i), got, http.StatusForbidden, "miss", "output_policy_block")
			if strings.Contains(got.body, "AKIAIOSFODNN7EXAMPLE") {
				t.Fatalf("attempt %d: the blocked text reached the client: %s", i, got.body)
			}
		}
		if g.upstreamCalls() != 2 || g.size() != 0 {
			t.Fatalf("provider calls = %d, entries = %d; want 2 and 0", g.upstreamCalls(), g.size())
		}
		eventually(t, "an output event for each request", func() bool { return len(g.outputEvents()) == 2 })
		for _, e := range g.outputEvents() {
			if e.OutputAction != "BLOCK" {
				t.Errorf("output event action = %q, want BLOCK", e.OutputAction)
			}
		}
	})

	t.Run("redacted", func(t *testing.T) {
		g := newCacheRig(t, defaultCachePolicy(), nil)
		g.answer = func(int, string) string { return "call me on 0532 123 45 67 tomorrow" }
		for i := 1; i <= 2; i++ {
			got := g.post(t, "what was the number", h)
			wantReply(t, "attempt "+strconv.Itoa(i), got, http.StatusOK, "miss", "tomorrow")
			if strings.Contains(got.body, "123 45 67") {
				t.Fatalf("attempt %d: the number was not redacted: %s", i, got.body)
			}
			if got.header.Get("X-Tamga-Output-Action") != "REDACT" {
				t.Fatalf("attempt %d: X-Tamga-Output-Action = %q", i, got.header.Get("X-Tamga-Output-Action"))
			}
		}
		if g.upstreamCalls() != 2 || g.size() != 0 {
			t.Fatalf("provider calls = %d, entries = %d; want 2 and 0", g.upstreamCalls(), g.size())
		}
		eventually(t, "an output event for each request", func() bool { return len(g.outputEvents()) == 2 })
	})
}

// The key is made from the body that was sent to the provider. Two requests
// that differ only in a value the proxy redacts are forwarded as the same
// bytes and share an entry, so a hit or a miss says nothing about the value:
// no caller can use the cache to test a guess at somebody's secret.
func TestCache_RedactedInputIsKeyedOnTheForwardedBody(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	h := map[string]string{"Authorization": credA}

	first := g.post(t, "write to ayse.kaya@example.com about the invoice", h)
	wantReply(t, "first", first, http.StatusOK, "miss", "answer 1")

	// Another address in the same sentence: the provider would be sent the
	// very same bytes, so it is a hit, as the first address would be again.
	other := g.post(t, "write to someone.else@example.org about the invoice", h)
	wantReply(t, "another address", other, http.StatusOK, "hit", "answer 1")
	same := g.post(t, "write to ayse.kaya@example.com about the invoice", h)
	wantReply(t, "the first address again", same, http.StatusOK, "hit", "answer 1")
	if other.header.Get("X-Tamga-Redacted-Count") != "1" {
		t.Errorf("a hit does not say what was redacted: X-Tamga-Redacted-Count = %q", other.header.Get("X-Tamga-Redacted-Count"))
	}

	// What is not redacted still tells requests apart.
	wantReply(t, "another sentence", g.post(t, "write to ayse.kaya@example.com about the refund", h), http.StatusOK, "miss", "answer 2")
	// And another credential learns nothing from any of it.
	wantReply(t, "another credential", g.post(t, "write to ayse.kaya@example.com about the invoice", map[string]string{"Authorization": credB}), http.StatusOK, "miss", "answer 3")

	g.mu.Lock()
	defer g.mu.Unlock()
	for _, b := range g.bodies {
		if strings.Contains(b, "ayse.kaya") || strings.Contains(b, "someone.else") {
			t.Fatalf("an address reached the provider: %s", b)
		}
	}
}

// A vaulted request has placeholders that are restored for this request
// only. It is neither stored nor answered from an entry that happens to have
// the same placeholder text.
func TestCache_VaultedRequestBypassesTheCache(t *testing.T) {
	g := newCacheRig(t, fmt.Sprintf(cachePolicy, 300, "vault:\n  enabled: true\n"), nil)
	h := map[string]string{"Authorization": credA}
	// The provider repeats the user's text, placeholders and all.
	g.answer = func(_ int, body string) string { return "you said: " + extractUserContent(body) }

	for i := 1; i <= 2; i++ {
		got := g.post(t, "write to ayse.kaya@example.com please", h)
		if got.status != http.StatusOK || got.cache == "hit" || !strings.Contains(got.body, "ayse.kaya@example.com") {
			t.Fatalf("attempt %d: got %d, X-Tamga-Cache=%q, body %s; want the address restored and no hit", i, got.status, got.cache, got.body)
		}
	}
	if g.upstreamCalls() != 2 || g.size() != 0 {
		t.Fatalf("provider calls = %d, entries = %d; want 2 and 0", g.upstreamCalls(), g.size())
	}
	g.mu.Lock()
	placeholder := extractUserContent(g.bodies[0])
	g.mu.Unlock()
	if strings.Contains(placeholder, "ayse.kaya") {
		t.Fatalf("the address was not tokenised: %s", placeholder)
	}

	// A request that carries the placeholder text itself is forwarded as the
	// same bytes. It is cached like any other, and the vaulted request still
	// does not read that entry.
	wantReply(t, "literal placeholder", g.post(t, placeholder, h), http.StatusOK, "miss", "you said")
	wantReply(t, "literal placeholder again", g.post(t, placeholder, h), http.StatusOK, "hit", "you said")
	got := g.post(t, "write to ayse.kaya@example.com please", h)
	if got.cache == "hit" || !strings.Contains(got.body, "ayse.kaya@example.com") {
		t.Fatalf("the vaulted request was answered from the cache: X-Tamga-Cache=%q, body %s", got.cache, got.body)
	}
}

func extractUserContent(body string) string {
	const marker = `"content":"`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	if j := strings.Index(rest, `"`); j >= 0 {
		return rest[:j]
	}
	return rest
}

// Whatever the policy asks for, an entry is gone at the ceiling.
func TestCache_TTLIsClampedToTheCeiling(t *testing.T) {
	g := newCacheRig(t, fmt.Sprintf(cachePolicy, 30*24*3600, ""), nil)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	g.cache.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }
	h := map[string]string{"Authorization": credA}

	var logs syncBuffer
	old := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = old })

	wantReply(t, "first", g.post(t, "how long", h), http.StatusOK, "miss", "answer 1")
	advance(cache.MaxTTL - time.Minute)
	wantReply(t, "just under the ceiling", g.post(t, "how long", h), http.StatusOK, "hit", "answer 1")
	advance(2 * time.Minute)
	wantReply(t, "just over the ceiling", g.post(t, "how long", h), http.StatusOK, "miss", "answer 2")
	// Stored again from now, and again for the ceiling and no longer.
	advance(cache.MaxTTL + time.Minute)
	wantReply(t, "a ceiling later", g.post(t, "how long", h), http.StatusOK, "miss", "answer 3")

	// The operator is told, once for the value and not once per response.
	if n := strings.Count(logs.String(), "above the ceiling"); n > 1 {
		t.Errorf("the clamp was logged %d times", n)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// A hit is a request like any other to whoever listens on the bus: the same
// decision event, with what the cache did.
func TestCache_HitPublishesTheDecisionEvent(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	h := map[string]string{"Authorization": credA, "X-Tamga-Org-Id": "acme", "X-Tamga-User-Id": "alice"}

	wantReply(t, "first", g.post(t, "write to ayse.kaya@example.com about the invoice", h), http.StatusOK, "miss", "answer 1")
	hit := g.post(t, "write to ayse.kaya@example.com about the invoice", h)
	wantReply(t, "second", hit, http.StatusOK, "hit", "answer 1")
	// Not eligible: the event says so, the header says only that it was not
	// served from the cache.
	wantReply(t, "anonymous", g.post(t, "hello", nil), http.StatusOK, "miss", "answer 2")

	evs := g.requestEvents(t, 3)
	if len(evs) != 3 {
		t.Fatalf("%d request events, want one per request: %+v", len(evs), evs)
	}
	for i, want := range []string{"miss", "hit", "bypass"} {
		if evs[i].CacheStatus != want {
			t.Errorf("event %d: cache status = %q, want %q", i+1, evs[i].CacheStatus, want)
		}
	}
	miss, got := evs[0], evs[1]
	if got.RequestID != hit.header.Get("X-Tamga-Request-Id") || got.RequestID == miss.RequestID {
		t.Errorf("the hit's event has request id %q, the response %q", got.RequestID, hit.header.Get("X-Tamga-Request-Id"))
	}
	if got.Action != "REDACT" || got.Action != miss.Action || len(got.Findings) != len(miss.Findings) || len(got.Findings) == 0 {
		t.Errorf("the hit's event carries action %q and %d findings; the forwarded one %q and %d",
			got.Action, len(got.Findings), miss.Action, len(miss.Findings))
	}
	if got.OrgID != "acme" || got.UserID != "alice" || got.Model != "gpt-4o-mini" || got.Provider != "openai" {
		t.Errorf("the hit's event: %+v", got)
	}
	if strings.Contains(string(got.Body), "ayse.kaya") {
		t.Errorf("the hit's event carries the unredacted body: %s", got.Body)
	}
	// A stored answer had no output findings, so a hit has no output event.
	settle()
	if n := len(g.outputEvents()); n != 0 {
		t.Errorf("%d output events for clean answers", n)
	}
}

// Neither the credential nor anything made from it is written to the log.
func TestCache_CredentialIsNotLogged(t *testing.T) {
	var logs syncBuffer
	old := log.Logger
	log.Logger = zerolog.New(&logs).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = old })

	g := newCacheRig(t, fmt.Sprintf(cachePolicy, 999999, ""), nil)
	h := map[string]string{"Authorization": credA}
	wantReply(t, "first", g.post(t, "hello there", h), http.StatusOK, "miss", "answer 1")
	wantReply(t, "second", g.post(t, "hello there", h), http.StatusOK, "hit", "answer 1")
	wantReply(t, "other", g.post(t, "hello there", map[string]string{"Authorization": credB}), http.StatusOK, "miss", "answer 2")

	out := logs.String()
	if !strings.Contains(out, "served from cache") {
		t.Fatalf("the log was not captured: %s", out)
	}
	for _, secret := range []string{credA, credB, strings.TrimPrefix(credA, "Bearer "), "cache-aaaa", "cache-bbbb", "cred:"} {
		if strings.Contains(out, secret) {
			t.Fatalf("%q is in the log: %s", secret, out)
		}
	}
	if n := strings.Count(out, "above the ceiling"); n != 1 {
		t.Errorf("the TTL clamp was logged %d times, want once: %s", n, out)
	}
}

// Some providers name the model in the path and the key in the query. The
// path is part of the key, the key parameter is the caller's identity.
func TestCache_PathAndQueryKey(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	body := `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`

	flash := "/gemini/v1beta/models/gemini-flash:generateContent"
	pro := "/gemini/v1beta/models/gemini-pro:generateContent"
	wantReply(t, "flash", g.postTo(t, flash+"?key=test-key-one", body, nil), http.StatusOK, "miss", "answer 1")
	wantReply(t, "flash again", g.postTo(t, flash+"?key=test-key-one", body, nil), http.StatusOK, "hit", "answer 1")
	wantReply(t, "another model, same body", g.postTo(t, pro+"?key=test-key-one", body, nil), http.StatusOK, "miss", "answer 2")
	wantReply(t, "another key", g.postTo(t, flash+"?key=test-key-two", body, nil), http.StatusOK, "miss", "answer 3")
	wantReply(t, "no key", g.postTo(t, flash, body, nil), http.StatusOK, "miss", "answer 4")
	wantReply(t, "another query", g.postTo(t, flash+"?alt=sse&key=test-key-one", body, nil), http.StatusOK, "miss", "answer 5")
}

// A provider that compresses its answer is read decoded: the proxy asks for
// the compression on its own account, so the scan sees the text. What is
// stored and served on a hit is that text, without a Content-Encoding the
// body would not match.
func TestCache_CompressedAnswerIsStoredDecoded(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), nil)
	g.gzip = true
	h := map[string]string{"Authorization": credA, "Accept-Encoding": "gzip"}
	for i, want := range []string{"miss", "hit"} {
		got := g.post(t, "hello there", h)
		if got.status != http.StatusOK || got.cache != want || got.header.Get("Content-Encoding") != "" {
			t.Fatalf("attempt %d: got %d, X-Tamga-Cache=%q, Content-Encoding=%q, want 200, %q and no encoding",
				i+1, got.status, got.cache, got.header.Get("Content-Encoding"), want)
		}
		if !strings.Contains(got.body, "answer 1") {
			t.Fatalf("attempt %d: the body is not the provider's text: %q", i+1, got.body)
		}
	}
}

// With TAMGA_REQUIRE_KEY on, the organisation of every request that gets as
// far as the cache is a key's.
func TestCache_RequireKey(t *testing.T) {
	g := newCacheRig(t, defaultCachePolicy(), &config.Config{RequireKey: true})
	if got := g.post(t, "hello there", map[string]string{"Authorization": credA, "X-Tamga-Org-Id": "acme"}); got.status != http.StatusUnauthorized || got.cache != "" {
		t.Fatalf("anonymous with require-key: got %d, X-Tamga-Cache=%q", got.status, got.cache)
	}
	key, _ := g.keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	h := map[string]string{keyHeader: key.RawKey, "Authorization": credA}
	wantReply(t, "first", g.post(t, "hello there", h), http.StatusOK, "miss", "answer 1")
	wantReply(t, "second", g.post(t, "hello there", h), http.StatusOK, "hit", "answer 1")

	// A revoked key is refused before the cache is looked at.
	if err := g.keys.Delete(key.ID); err != nil {
		t.Fatal(err)
	}
	if got := g.post(t, "hello there", h); got.status != http.StatusUnauthorized || got.cache != "" {
		t.Fatalf("revoked key: got %d, X-Tamga-Cache=%q, body %s", got.status, got.cache, got.body)
	}
}
