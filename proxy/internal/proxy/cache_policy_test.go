package proxy

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// The provider's answer in these tests. Nothing built in finds anything in
// it; a custom entity or a stored pattern for the codename does.
const codenameAnswer = "the project codename is BLUEHERON"

// policyLoose lets codenameAnswer through and caches it.
const policyLoose = `
version: "1.0"
providers:
  allowed: [openai]
cache:
  enabled: true
  ttl_seconds: 300
output_rules:
  enabled: true
  block_on: [aws_access_key, private_key]
`

// policyLooseRewritten is policyLoose as another hand would write it: keys in
// another order, a comment, flow style swapped for block style. Lists keep
// their order.
const policyLooseRewritten = `
# the same policy
output_rules:
  block_on:
    - aws_access_key
    - private_key
  enabled: true
cache:
  ttl_seconds: 300
  enabled: true
providers:
  allowed:
    - openai
version: "1.0"
`

// policyLooseListReordered differs from policyLoose in the order of one list.
const policyLooseListReordered = `
version: "1.0"
providers:
  allowed: [openai]
cache:
  enabled: true
  ttl_seconds: 300
output_rules:
  enabled: true
  block_on: [private_key, aws_access_key]
`

// policyStrict is policyLoose with a rule that blocks codenameAnswer.
const policyStrict = `
version: "1.0"
providers:
  allowed: [openai]
cache:
  enabled: true
  ttl_seconds: 300
output_rules:
  enabled: true
  block_on: [aws_access_key, private_key, codename]
custom_entities:
  - name: codename
    pattern: "BLUEHERON"
    severity: high
    action: BLOCK
`

// policyRig is a proxy whose policy and stored patterns can be changed while
// it runs, wired the way cmd/tamga wires them: the custom scanner reads both,
// and ScanState names what it is given.
type policyRig struct {
	url    string
	cache  *cache.Cache
	keys   *apikeys.MemoryStore
	pol    atomic.Pointer[policy.Policy]
	custom *scanner.CustomScanner

	mu     sync.Mutex
	stored []scanner.CustomEntitySpec
	calls  int
	// onCall runs inside the provider, before it answers.
	onCall func(n int)
}

type policyRigOptions struct {
	cache *cache.Cache
	keys  *apikeys.MemoryStore
}

func newPolicyRig(t *testing.T, policyYAML string, opt policyRigOptions) *policyRig {
	t.Helper()
	g := &policyRig{cache: opt.cache, keys: opt.keys}
	if g.cache == nil {
		g.cache = cache.New(64)
	}
	if g.keys == nil {
		g.keys = apikeys.NewMemoryStore()
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		g.mu.Lock()
		g.calls++
		n, hook := g.calls, g.onCall
		g.mu.Unlock()
		if hook != nil {
			hook(n)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(codenameAnswer) + `}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	g.custom = scanner.NewCustomScanner(g.specs)
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewPIIScanner())
	reg.Register(scanner.NewSecretScanner())
	reg.Register(scanner.NewInjectionScanner())
	reg.Register(g.custom)
	g.usePolicy(t, policyYAML)

	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     reg,
		GetPolicy:    g.pol.Load,
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       &config.Config{},
		Cache:        g.cache,
		Keys:         g.keys,
		ScanState:    func() string { return CustomSpecsDigest(g.specs()) },
	}))
	t.Cleanup(srv.Close)
	g.url = srv.URL
	return g
}

// specs is what the custom scanner compiles: the policy's entities and the
// stored patterns.
func (g *policyRig) specs() []scanner.CustomEntitySpec {
	var out []scanner.CustomEntitySpec
	if p := g.pol.Load(); p != nil {
		for _, ce := range p.CustomEntities {
			out = append(out, scanner.CustomEntitySpec{Name: ce.Name, Pattern: ce.Pattern, Severity: ce.Severity, Confidence: ce.Confidence})
		}
	}
	g.mu.Lock()
	out = append(out, g.stored...)
	g.mu.Unlock()
	return out
}

// usePolicy installs a freshly loaded policy, as a reload does.
func (g *policyRig) usePolicy(t *testing.T, yaml string) *policy.Policy {
	t.Helper()
	p := mustPolicy(t, yaml)
	g.install(p)
	return p
}

func (g *policyRig) install(p *policy.Policy) {
	g.pol.Store(p)
	g.custom.Refresh()
}

// storePatterns replaces the stored patterns, as the patterns API does.
func (g *policyRig) storePatterns(specs ...scanner.CustomEntitySpec) {
	g.mu.Lock()
	g.stored = specs
	g.mu.Unlock()
	g.custom.Refresh()
}

func (g *policyRig) upstreamCalls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *policyRig) post(t *testing.T, headers map[string]string) cacheReply {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, g.url+"/v1/chat/completions", strings.NewReader(cacheChat("what is the codename?")))
	req.Header.Set("Content-Type", "application/json")
	if headers == nil {
		headers = map[string]string{"Authorization": credA}
	}
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

func wantCalls(t *testing.T, what string, g *policyRig, want int) {
	t.Helper()
	if got := g.upstreamCalls(); got != want {
		t.Fatalf("%s: the provider was called %d times, want %d", what, got, want)
	}
}

func wantBlocked(t *testing.T, what string, got cacheReply) {
	t.Helper()
	if got.status != http.StatusForbidden || got.cache == "hit" || strings.Contains(got.body, "BLUEHERON") {
		t.Fatalf("%s: got %d, X-Tamga-Cache=%q, body %s; want a 403 that is not a hit and does not carry the answer",
			what, got.status, got.cache, got.body)
	}
}

// The finding: an answer stored under one policy was served for up to an
// hour after the policy changed, without being scanned under the new one.
func TestCache_PolicyChangeIsAMiss(t *testing.T) {
	g := newPolicyRig(t, policyLoose, policyRigOptions{})

	wantReply(t, "first, loose policy", g.post(t, nil), http.StatusOK, "miss", "BLUEHERON")
	wantReply(t, "second, loose policy", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "loose policy", g, 1)

	// The stricter policy would have blocked what is stored.
	g.usePolicy(t, policyStrict)
	wantBlocked(t, "first, strict policy", g.post(t, nil))
	wantCalls(t, "first, strict policy", g, 2)
	// Blocked, so not stored under the strict policy either.
	wantBlocked(t, "second, strict policy", g.post(t, nil))
	wantCalls(t, "second, strict policy", g, 3)

	// The loose policy again, loaded afresh: another *Policy, same content.
	g.usePolicy(t, policyLoose)
	wantReply(t, "back on the loose policy", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "back on the loose policy", g, 3)
}

// A policy file rewritten without changing what it says is the same policy.
// One with a list in another order is not: its first request misses, and
// what it stores is its own.
func TestCache_RewrittenPolicy(t *testing.T) {
	loose, rewritten, reordered := mustPolicy(t, policyLoose), mustPolicy(t, policyLooseRewritten), mustPolicy(t, policyLooseListReordered)
	if a, b := computePolicyDigest(loose), computePolicyDigest(rewritten); a == "" || a != b {
		t.Fatalf("key order, comments and style changed the digest: %q and %q", a, b)
	}
	if a, b := computePolicyDigest(loose), computePolicyDigest(reordered); a == b {
		t.Fatal("a reordered list did not change the digest; the test below would prove nothing")
	}
	if a, b := computePolicyDigest(loose), computePolicyDigest(mustPolicy(t, policyStrict)); a == b {
		t.Fatal("the strict policy has the digest of the loose one")
	}

	g := newPolicyRig(t, policyLoose, policyRigOptions{})
	wantReply(t, "first", g.post(t, nil), http.StatusOK, "miss", "BLUEHERON")

	g.install(rewritten)
	wantReply(t, "rewritten file", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "rewritten file", g, 1)

	g.install(reordered)
	wantReply(t, "reordered list", g.post(t, nil), http.StatusOK, "miss", "BLUEHERON")
	wantReply(t, "reordered list, again", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "reordered list", g, 2)
}

// A pattern stored through the API is not in the policy. It changes what the
// output scan finds all the same.
func TestCache_StoredPatternChangeIsAMiss(t *testing.T) {
	// codename is in block_on from the start; nothing finds one yet.
	strictWithoutEntity := strings.Split(policyStrict, "custom_entities:")[0]
	g := newPolicyRig(t, strictWithoutEntity, policyRigOptions{})

	wantReply(t, "no pattern", g.post(t, nil), http.StatusOK, "miss", "BLUEHERON")
	wantReply(t, "no pattern, again", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")

	g.storePatterns(scanner.CustomEntitySpec{Name: "codename", Pattern: "BLUEHERON", Severity: "high", Confidence: 0.8})
	wantBlocked(t, "pattern stored", g.post(t, nil))
	wantCalls(t, "pattern stored", g, 2)

	g.storePatterns()
	wantReply(t, "pattern deleted", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "pattern deleted", g, 2)
}

// The custom scanner reads the live policy, so an answer that comes back
// after a reload may have been scanned under the new entities while the key
// names the old policy. It is not stored.
func TestCache_NothingStoredAcrossAPolicyChange(t *testing.T) {
	g := newPolicyRig(t, policyLoose, policyRigOptions{})
	loose := g.pol.Load()
	other := mustPolicy(t, policyLooseListReordered)
	g.mu.Lock()
	g.onCall = func(n int) {
		if n == 1 {
			g.install(other)
		}
	}
	g.mu.Unlock()

	wantReply(t, "answered across the change", g.post(t, nil), http.StatusOK, "miss", "BLUEHERON")
	if _, _, size := g.cache.Stats(); size != 0 {
		t.Fatalf("an answer scanned across a policy change was stored: %d entries", size)
	}
	// The very policy the key was made under, back in force.
	g.install(loose)
	wantReply(t, "old policy again", g.post(t, nil), http.StatusOK, "miss", "BLUEHERON")
	wantReply(t, "old policy, stored now", g.post(t, nil), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "three requests", g, 2)
}

// sharedRedis is the one Redis two replicas point at.
type sharedRedis struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *sharedRedis) Enabled() bool              { return true }
func (s *sharedRedis) Ping(context.Context) error { return nil }
func (s *sharedRedis) Close() error               { return nil }
func (s *sharedRedis) Incr(context.Context, string, int64, time.Duration) (int64, error) {
	return 0, nil
}
func (s *sharedRedis) IncrFloat(context.Context, string, float64, time.Duration) (float64, error) {
	return 0, nil
}
func (s *sharedRedis) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	return v, ok, nil
}
func (s *sharedRedis) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = append([]byte(nil), value...)
	return nil
}
func (s *sharedRedis) Del(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

// Two replicas behind one Redis. On the same policy the second is served
// what the first stored; on different policies neither is served the
// other's, whichever of them is the stricter.
func TestCache_ReplicasShareOnlyUnderTheSamePolicy(t *testing.T) {
	rdx := &sharedRedis{data: map[string][]byte{}}
	keys := apikeys.NewMemoryStore()
	key, _ := keys.CreateWith(apikeys.Params{Label: "app", Scope: apikeys.ScopeProxy, OrgID: "acme"})
	// No provider credential: the answer is keyed on the Tamga key, and only
	// such an entry is mirrored to Redis.
	h := map[string]string{keyHeader: key.RawKey}

	replica := func(policyYAML string) *policyRig {
		c := cache.New(64)
		c.SetRedis(rdx)
		return newPolicyRig(t, policyYAML, policyRigOptions{cache: c, keys: keys})
	}
	a, b, strict := replica(policyLoose), replica(policyLoose), replica(policyStrict)

	wantReply(t, "replica a", a.post(t, h), http.StatusOK, "miss", "BLUEHERON")
	wantReply(t, "replica b, same policy", b.post(t, h), http.StatusOK, "hit", "BLUEHERON")
	wantCalls(t, "replica b", b, 0)

	wantBlocked(t, "replica on the strict policy", strict.post(t, h))
	wantCalls(t, "replica on the strict policy", strict, 1)

	// b moves to the strict policy; a has not reloaded yet.
	b.usePolicy(t, policyStrict)
	wantBlocked(t, "replica b after its reload", b.post(t, h))
	wantCalls(t, "replica b after its reload", b, 1)
	wantReply(t, "replica a, still loose", a.post(t, h), http.StatusOK, "hit", "BLUEHERON")
}

// The digest is worked out when a policy is first seen, not for each request
// that runs under it.
func TestCache_PolicyDigestIsComputedOncePerPolicy(t *testing.T) {
	g := newPolicyRig(t, policyLoose, policyRigOptions{})
	before := policyDigests.computed.Load()
	for i := 0; i < 25; i++ {
		if got := g.post(t, nil); got.status != http.StatusOK {
			t.Fatalf("request %d: got %d", i, got.status)
		}
	}
	if n := policyDigests.computed.Load() - before; n != 1 {
		t.Fatalf("25 requests under one policy computed the digest %d times, want 1", n)
	}

	g.usePolicy(t, policyStrict)
	for i := 0; i < 25; i++ {
		g.post(t, nil)
	}
	if n := policyDigests.computed.Load() - before; n != 2 {
		t.Fatalf("two policies computed the digest %d times, want 2", n)
	}

	// Many requests arriving at once after a reload still compute it once.
	fresh := mustPolicy(t, policyLoose)
	var wg sync.WaitGroup
	digests := make([]string, 32)
	for i := range digests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			digests[i] = policyDigest(fresh)
		}(i)
	}
	wg.Wait()
	if n := policyDigests.computed.Load() - before; n != 3 {
		t.Fatalf("concurrent first use computed the digest %d times in all, want 3", n)
	}
	for _, d := range digests {
		if d == "" || d != digests[0] {
			t.Fatalf("concurrent callers got different digests: %q and %q", d, digests[0])
		}
	}
}

// A policy that cannot be named is not cached under.
func TestCache_UnnamedPolicyDoesNotUseTheCache(t *testing.T) {
	if policyDigest(nil) != "" {
		t.Fatal("no policy has a digest")
	}
	// Not a number has no JSON form, so this policy has no digest.
	unnamed := &policy.Policy{
		Cache:          &policy.CacheConfig{Enabled: true, TTLSeconds: 300},
		OutputRules:    &policy.OutputRules{Enabled: true},
		CustomEntities: []policy.CustomEntity{{Name: "never", Pattern: "ZZZ-NOT-IN-THE-ANSWER", Confidence: math.NaN()}},
	}
	if d := policyDigest(unnamed); d != "" {
		t.Fatalf("expected no digest, got %q", d)
	}

	g := newPolicyRig(t, policyLoose, policyRigOptions{})
	g.install(unnamed)
	for i := 1; i <= 2; i++ {
		got := g.post(t, nil)
		if got.status != http.StatusOK || got.cache == "hit" {
			t.Fatalf("request %d: got %d, X-Tamga-Cache=%q", i, got.status, got.cache)
		}
	}
	wantCalls(t, "unnamed policy", g, 2)
	if _, _, size := g.cache.Stats(); size != 0 {
		t.Fatalf("stored %d entries under a policy with no digest", size)
	}
}

func TestCustomSpecsDigest(t *testing.T) {
	a := scanner.CustomEntitySpec{Name: "codename", Pattern: "BLUEHERON", Severity: "high", Confidence: 0.8}
	b := scanner.CustomEntitySpec{Name: "ticket", Pattern: `TCK-\d+`, Severity: "low", Confidence: 0.8}
	if CustomSpecsDigest([]scanner.CustomEntitySpec{a, b}) != CustomSpecsDigest([]scanner.CustomEntitySpec{b, a}) {
		t.Fatal("the order the patterns are listed in changed the digest")
	}
	changed := a
	changed.Pattern = "BLUEHERON|REDKITE"
	seen := map[string]string{}
	for name, specs := range map[string][]scanner.CustomEntitySpec{
		"none":    nil,
		"a":       {a},
		"a, b":    {a, b},
		"changed": {changed},
		// A name and a pattern that would read the same run together.
		"shifted": {{Name: "code", Pattern: "nameBLUEHERON", Severity: "high", Confidence: 0.8}},
	} {
		d := CustomSpecsDigest(specs)
		if other, dup := seen[d]; dup {
			t.Fatalf("%q and %q have the same digest", name, other)
		}
		seen[d] = name
	}
}

func BenchmarkPolicyDigest(b *testing.B) {
	p, err := policy.LoadFromBytes([]byte(policyStrict))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("remembered", func(b *testing.B) {
		policyDigest(p)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			policyDigest(p)
		}
	})
	b.Run("computed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			computePolicyDigest(p)
		}
	})
	b.Run("scan_state_100_patterns", func(b *testing.B) {
		specs := make([]scanner.CustomEntitySpec, 100)
		for i := range specs {
			specs[i] = scanner.CustomEntitySpec{Name: "pattern-" + strconv.Itoa(i), Pattern: `[A-Z]{3}-\d{4,8}-` + strconv.Itoa(i), Severity: "medium", Confidence: 0.8}
		}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			CustomSpecsDigest(specs)
		}
	})
}
