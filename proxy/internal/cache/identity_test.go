package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── Identity and key ────────────────────────────────────────────

func requestWith(target string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

const testCredential = "sk-test-4f9c2a7e1b6d"

func TestIdentity_NobodyIsNotNamed(t *testing.T) {
	c := New(16)
	id := c.Identity(requestWith("/v1/chat/completions", nil), "", false)
	if id.OK() {
		t.Fatalf("a request with no credential and no key was named: %+v", id)
	}
	if k := Key(KeyParts{Org: "acme", Identity: id, Provider: "openai", Body: []byte("x")}); k != "" {
		t.Fatalf("a request that names nobody got a key: %q", k)
	}
	// Headers that only scope a credential are not one.
	id = c.Identity(requestWith("/v1/chat/completions", map[string]string{"OpenAI-Organization": "org-1", "Authorization": "  "}), "", false)
	if id.OK() {
		t.Fatalf("scope headers alone named a caller: %+v", id)
	}
}

func TestIdentity_DigestIsNotTheCredential(t *testing.T) {
	c := New(16)
	for _, headers := range []map[string]string{
		{"Authorization": "Bearer " + testCredential},
		{"X-Api-Key": testCredential},
		{"Api-Key": testCredential},
		{"X-Goog-Api-Key": testCredential},
	} {
		id := c.Identity(requestWith("/v1/chat/completions", headers), "", false)
		if !id.OK() || !id.local {
			t.Fatalf("%v: want a process-local identity, got %+v", headers, id)
		}
		key := Key(KeyParts{Org: "acme", Identity: id, Provider: "openai", Body: []byte("x")})
		for _, s := range []string{id.value, key, fmt.Sprintf("%+v", id)} {
			if strings.Contains(s, testCredential) || strings.Contains(s, testCredential[3:]) {
				t.Fatalf("%v: the credential is readable in %q", headers, s)
			}
		}
	}
	// The key from a Gemini-style URL is a credential too, and it does not
	// stay in the part of the key that describes the request.
	withKey := requestWith("/gemini/v1beta/models/m:generateContent?alt=json&key="+testCredential, nil)
	id := c.Identity(withKey, "", false)
	if !id.OK() {
		t.Fatal("?key= did not name the caller")
	}
	if q := queryWithoutCredential(withKey.URL.RawQuery); q != "alt=json" {
		t.Fatalf("query kept for the key = %q, want alt=json", q)
	}
}

func TestIdentity_SameCredentialSameCacheOnly(t *testing.T) {
	a, b := New(16), New(16)
	h := map[string]string{"Authorization": "Bearer " + testCredential}
	first := a.Identity(requestWith("/v1/chat/completions", h), "", false)
	again := a.Identity(requestWith("/v1/chat/completions", h), "tamga-key-7", false)
	if first.value != again.value {
		t.Fatal("the same credential gave two identities in one cache")
	}
	other := a.Identity(requestWith("/v1/chat/completions", map[string]string{"Authorization": "Bearer sk-test-other"}), "", false)
	if other.value == first.value {
		t.Fatal("two credentials gave one identity")
	}
	// Another process has another key: its digests of the same credential
	// are different, so a digest is of no use outside the process it was
	// made in.
	if elsewhere := b.Identity(requestWith("/v1/chat/completions", h), "", false); elsewhere.value == first.value {
		t.Fatal("two caches gave the same digest for a credential")
	}
	// The same key in a different header is a different thing to a provider.
	asAPIKey := a.Identity(requestWith("/v1/chat/completions", map[string]string{"X-Api-Key": "Bearer " + testCredential}), "", false)
	if asAPIKey.value == first.value {
		t.Fatal("the header a credential came in is not part of its digest")
	}
	scoped := a.Identity(requestWith("/v1/chat/completions", map[string]string{"Authorization": "Bearer " + testCredential, "OpenAI-Project": "proj_1"}), "", false)
	if scoped.value == first.value {
		t.Fatal("a credential scoped to a project shares with the unscoped one")
	}
}

func TestIdentity_KeyNamesTheCallerWhenThereIsNoCredential(t *testing.T) {
	c := New(16)
	id := c.Identity(requestWith("/local/v1/chat/completions", nil), "tamga-key-7", false)
	if !id.OK() || id.local || id.value != "key:tamga-key-7" {
		t.Fatalf("identity = %+v, want the key id, shareable", id)
	}
	if other := c.Identity(requestWith("/local/v1/chat/completions", nil), "tamga-key-8", false); other.value == id.value {
		t.Fatal("two keys gave one identity")
	}
}

func TestIdentity_AWSSignatures(t *testing.T) {
	c := New(16)
	sig := map[string]string{"Authorization": "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261009/us-east-1/bedrock/aws4_request, SignedHeaders=host, Signature=abc"}
	// The caller's own signature is forwarded: it is new on every request
	// and proves nothing when replayed, so nothing is cached on it, with or
	// without a Tamga key.
	if id := c.Identity(requestWith("/bedrock/model/m/invoke", sig), "tamga-key-7", false); id.OK() {
		t.Fatalf("a caller-signed request was named: %+v", id)
	}
	// The proxy signs: the caller's Authorization is dropped on the way out
	// and the Tamga key is who the answer is for.
	if id := c.Identity(requestWith("/bedrock/model/m/invoke", sig), "tamga-key-7", true); id.value != "key:tamga-key-7" {
		t.Fatalf("identity = %+v, want the key id", id)
	}
	if id := c.Identity(requestWith("/bedrock/model/m/invoke", sig), "", true); id.OK() {
		t.Fatalf("an anonymous request the proxy signs was named: %+v", id)
	}
}

func TestKey_Components(t *testing.T) {
	c := New(16)
	id := c.Identity(requestWith("/v1/chat/completions", map[string]string{"Authorization": "Bearer " + testCredential}), "", false)
	base := KeyParts{Org: "acme", Verified: true, Identity: id, Provider: "openai", Model: "gpt-4o", Path: "/v1/chat/completions", Query: "a=1&b=2", Body: []byte(`{"x":1}`)}
	want := Key(base)
	if !strings.HasPrefix(want, localKeyPrefix) {
		t.Fatalf("a key on a credential digest is not marked local: %q", want)
	}
	same := base
	same.Query = "b=2&key=whatever&a=1"
	if Key(same) != want {
		t.Error("the order of the query or the credential in it changed the key")
	}
	change := map[string]func(*KeyParts){
		"org":      func(p *KeyParts) { p.Org = "globex" },
		"verified": func(p *KeyParts) { p.Verified = false },
		"identity": func(p *KeyParts) { p.Identity = Identity{value: "key:k1"} },
		"provider": func(p *KeyParts) { p.Provider = "azure" },
		"model":    func(p *KeyParts) { p.Model = "gpt-4o-mini" },
		"path":     func(p *KeyParts) { p.Path = "/v1/completions" },
		"query":    func(p *KeyParts) { p.Query = "a=1&b=3" },
		"body":     func(p *KeyParts) { p.Body = []byte(`{"x":2}`) },
	}
	for name, f := range change {
		p := base
		f(&p)
		if Key(p) == want {
			t.Errorf("changing the %s did not change the key", name)
		}
	}
	// One field cannot run into the next.
	a, b := base, base
	a.Org, a.Provider = "ab", "c"
	b.Org, b.Provider = "a", "bc"
	if Key(a) == Key(b) {
		t.Error("fields are not kept apart in the key")
	}
}

// ── TTL ceiling ─────────────────────────────────────────────────

func TestEffectiveTTL(t *testing.T) {
	cases := []struct {
		seconds int
		want    time.Duration
		clamped bool
	}{
		{0, DefaultTTL, false},
		{-5, DefaultTTL, false},
		{300, 5 * time.Minute, false},
		{3600, MaxTTL, false},
		{3601, MaxTTL, true},
		{31536000, MaxTTL, true},
		{int(^uint(0) >> 1), MaxTTL, true},
	}
	for _, tc := range cases {
		got, clamped := EffectiveTTL(tc.seconds)
		if got != tc.want || clamped != tc.clamped {
			t.Errorf("EffectiveTTL(%d) = %v, %v; want %v, %v", tc.seconds, got, clamped, tc.want, tc.clamped)
		}
	}
}

func TestCache_EntryExpiresAtTheCeiling(t *testing.T) {
	c := New(16)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	c.SetClock(func() time.Time { return now })

	e := &Entry{Key: "k", Body: []byte("data"), TTL: 30 * 24 * time.Hour}
	c.Set(e)
	if e.TTL != MaxTTL {
		t.Fatalf("stored TTL = %v, want it clamped to %v", e.TTL, MaxTTL)
	}
	now = now.Add(MaxTTL - time.Second)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("entry gone before the ceiling")
	}
	now = now.Add(2 * time.Second)
	if _, ok := c.Get("k"); ok {
		t.Fatal("entry served after the ceiling")
	}
}

// An entry that reaches the cache from Redis with a TTL nobody clamped is
// still held to the ceiling.
func TestCache_RedisEntryIsHeldToTheCeiling(t *testing.T) {
	c := New(16)
	old, _ := json.Marshal(&Entry{Key: "k", Body: []byte("data"), StoredAt: time.Now().Add(-2 * MaxTTL), TTL: 0})
	c.SetRedis(&mockRedisClient{enabled: true, getFunc: func(context.Context, string) ([]byte, bool, error) { return old, true, nil }})
	if _, ok := c.Get("k"); ok {
		t.Fatal("an entry older than the ceiling was served from Redis")
	}
}

// ── What goes to Redis ──────────────────────────────────────────

func TestCache_CredentialKeyedEntriesStayInTheProcess(t *testing.T) {
	c := New(16)
	sets, gets := 0, 0
	c.SetRedis(&mockRedisClient{
		enabled: true,
		setFunc: func(context.Context, string, []byte, time.Duration) error { sets++; return nil },
		getFunc: func(context.Context, string) ([]byte, bool, error) { gets++; return nil, false, nil },
	})
	id := c.Identity(requestWith("/v1/chat/completions", map[string]string{"Authorization": "Bearer " + testCredential}), "", false)
	local := Key(KeyParts{Org: "acme", Identity: id, Provider: "openai", Body: []byte("x")})
	if _, ok := c.Get(local); ok {
		t.Fatal("hit on an empty cache")
	}
	c.Set(&Entry{Key: local, Body: []byte("answer")})
	if sets != 0 || gets != 0 {
		t.Fatalf("an entry keyed on a credential digest went to Redis: %d sets, %d gets", sets, gets)
	}
	if _, ok := c.Get(local); !ok {
		t.Fatal("the entry is not served locally")
	}

	shared := Key(KeyParts{Org: "acme", Verified: true, Identity: c.Identity(requestWith("/v1/chat/completions", nil), "tamga-key-7", false), Provider: "openai", Body: []byte("x")})
	c.Set(&Entry{Key: shared, Body: []byte("answer")})
	if sets != 1 {
		t.Fatalf("an entry keyed on a Tamga key should be mirrored: %d sets", sets)
	}
}

func TestCache_RedisEntryForAnotherKeyIsNotServed(t *testing.T) {
	c := New(16)
	other, _ := json.Marshal(&Entry{Key: "someone-else", Body: []byte("theirs"), StoredAt: time.Now(), TTL: time.Minute})
	c.SetRedis(&mockRedisClient{enabled: true, getFunc: func(context.Context, string) ([]byte, bool, error) { return other, true, nil }})
	if _, ok := c.Get("mine"); ok {
		t.Fatal("an entry stored under another key was served")
	}
	c.SetRedis(&mockRedisClient{enabled: false})
	if _, ok := c.Get("someone-else"); ok {
		t.Fatal("the foreign entry was indexed locally")
	}
}

func TestCache_EmptyKeyIsNeverStoredOrServed(t *testing.T) {
	c := New(16)
	c.Set(&Entry{Key: "", Body: []byte("x")})
	if _, _, size := c.Stats(); size != 0 {
		t.Fatalf("an entry with no key was stored (size %d)", size)
	}
	if _, ok := c.Get(""); ok {
		t.Fatal("hit on the empty key")
	}
}
