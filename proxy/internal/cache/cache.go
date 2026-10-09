// Package cache implements an LRU exact-match prompt cache with a
// content-based SHA-256 key. Phase 3C — the cache short-circuits identical
// request payloads and returns the cached response body, saving upstream
// calls when a dashboard or runbook hits the same prompt repeatedly.
//
// A hit is answered without the provider seeing the request, so the provider
// does not check the caller's credential on a hit. The key therefore names
// who the answer was produced for (see Identity), and a request that names
// nobody does not use the cache at all.
//
// "Semantic" similarity (MiniLM/E5) is a future-phase swap-out — the public
// interface below is deliberately small so we can replace the implementation
// without changing call sites.
package cache

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/redisx"
)

// MaxTTL is the longest an entry is served, whatever the policy asks for.
// A hit skips the provider's check of the credential, so this is also the
// longest a credential revoked at the provider keeps getting answers it
// already paid for. An hour covers the repeated-prompt traffic the cache is
// for and stays well inside the time a leaked key takes to be noticed and
// rotated.
const MaxTTL = time.Hour

// DefaultTTL is used when the policy enables the cache without a TTL.
const DefaultTTL = 5 * time.Minute

// EffectiveTTL turns a policy's ttl_seconds into the TTL entries are stored
// with. clamped reports that the policy asked for more than MaxTTL.
func EffectiveTTL(seconds int) (ttl time.Duration, clamped bool) {
	if seconds <= 0 {
		return DefaultTTL, false
	}
	if int64(seconds) > int64(MaxTTL/time.Second) {
		return MaxTTL, true
	}
	return time.Duration(seconds) * time.Second, false
}

// Entry is a stored response payload.
type Entry struct {
	Key         string
	Provider    string
	Model       string
	Body        []byte
	ContentType string
	StoredAt    time.Time
	TTL         time.Duration
}

// Cache is safe for concurrent use.
// When SetRedis is called with an enabled client, the cache becomes a
// two-tier structure: the local LRU continues to serve hot paths and
// Redis acts as the shared fallback so sibling replicas benefit from
// each other's cached completions.
//
// Entries keyed on a provider credential stay in this process: the digest
// that names the credential is made with a key no other process has.
type Cache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	index    map[string]*list.Element
	hits     int64
	misses   int64
	rdx      redisx.Client
	// secret keys the credential digests. It is drawn at start and never
	// leaves the process, so a digest means nothing in a log, in Redis, in
	// another replica or after a restart.
	secret []byte
	now    func() time.Time
	// flushedAt is when Flush last ran. An entry Redis hands back from
	// before then is not served.
	flushedAt time.Time
}

// New creates an LRU exact-match prompt cache with the given capacity.
func New(capacity int) *Cache {
	if capacity < 16 {
		capacity = 512
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		// Without a key no credential can be digested, and requests that
		// carry one do not use the cache.
		secret = nil
	}
	return &Cache{
		capacity: capacity,
		ll:       list.New(),
		index:    map[string]*list.Element{},
		secret:   secret,
		now:      time.Now,
	}
}

// SetClock replaces the clock entries are stamped and expired with. Tests
// use it to reach the TTL ceiling without waiting for it.
func (c *Cache) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

// SetRedis attaches a redisx client; once Enabled() returns true the
// cache also mirrors Set/Get to Redis under the "tamga:cache:<key>" prefix.
func (c *Cache) SetRedis(r redisx.Client) {
	if r == nil {
		return
	}
	c.rdx = r
}

func (c *Cache) redisEnabled() bool { return c.rdx != nil && c.rdx.Enabled() }

func (c *Cache) redisKey(k string) string { return "tamga:cache:" + k }

// credentialHeaders are the request headers a provider reads a credential
// from. All that are present go into the digest, whichever route the request
// came in on: every one of them is forwarded, and which one a custom upstream
// checks is not known here.
var credentialHeaders = []string{"Authorization", "X-Api-Key", "Api-Key", "X-Goog-Api-Key"}

// scopeHeaders narrow what a credential is checked against at the provider.
// They are digested with the credential and are not one by themselves.
var scopeHeaders = []string{"Openai-Organization", "Openai-Project"}

// credentialQueryParam carries the key on Gemini-style URLs.
const credentialQueryParam = "key"

// Identity names who an entry may be served to.
type Identity struct {
	value string
	// local is set when value holds a credential digest, which only this
	// process can reproduce.
	local bool
}

// OK reports whether the request may use the cache.
func (i Identity) OK() bool { return i.value != "" }

// Identity works out who a request's answer belongs to.
//
// A request that carries a provider credential is named by a keyed digest of
// it: two callers who present the same credential share entries and nobody
// else does, which is exactly what the provider would have allowed. A request
// that carries none is named by its verified Tamga key (keyID); that is the
// case where the proxy supplies the provider credential itself and its own
// key check is the only one there is. A request with neither is not named
// and does not use the cache.
//
// proxySigns says the proxy replaces the Authorization header with its own
// signature (Bedrock with the host's AWS credentials), so the one the caller
// sent is checked by nobody and is left out. A caller's own AWS signature is
// different on every request and proves nothing when replayed, so a request
// that depends on one is not cached.
//
// The credential is never stored or returned, only its digest.
func (c *Cache) Identity(r *http.Request, keyID string, proxySigns bool) Identity {
	if c == nil || r == nil {
		return Identity{}
	}
	if len(c.secret) > 0 {
		mac := hmac.New(sha256.New, c.secret)
		found := false
		for _, name := range credentialHeaders {
			values := r.Header.Values(name)
			if name == "Authorization" {
				if proxySigns {
					continue
				}
				for _, v := range values {
					if strings.HasPrefix(strings.TrimSpace(v), "AWS4-HMAC-SHA256") {
						return Identity{}
					}
				}
			}
			if writeValues(mac, name, values) {
				found = true
			}
		}
		if r.URL != nil && strings.Contains(r.URL.RawQuery, credentialQueryParam) {
			if writeValues(mac, "?"+credentialQueryParam, r.URL.Query()[credentialQueryParam]) {
				found = true
			}
		}
		if found {
			for _, name := range scopeHeaders {
				writeValues(mac, name, r.Header.Values(name))
			}
			return Identity{value: "cred:" + hex.EncodeToString(mac.Sum(nil)), local: true}
		}
	} else if carriesCredential(r) {
		return Identity{}
	}
	if keyID != "" {
		return Identity{value: "key:" + keyID}
	}
	return Identity{}
}

// carriesCredential reports whether the request has anything Identity would
// have digested.
func carriesCredential(r *http.Request) bool {
	for _, name := range credentialHeaders {
		for _, v := range r.Header.Values(name) {
			if strings.TrimSpace(v) != "" {
				return true
			}
		}
	}
	return r.URL != nil && strings.TrimSpace(r.URL.Query().Get(credentialQueryParam)) != ""
}

// writeValues adds the non-empty values under name to h and reports whether
// there were any.
func writeValues(h hash.Hash, name string, values []string) bool {
	wrote := false
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		writeField(h, name)
		writeField(h, v)
		wrote = true
	}
	return wrote
}

// writeField writes one length-prefixed field, so no value can run into the
// next and read as a different pair.
func writeField(h hash.Hash, s string) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s)))
	h.Write(n[:])
	h.Write([]byte(s))
}

// KeyParts is everything that decides whether two requests get the same
// answer and whether the second may be given the first's.
type KeyParts struct {
	// Org is the organisation of the verified Tamga key. For a request
	// without one it is only what the request claims, and Verified keeps
	// such an entry apart from every verified caller's.
	Org      string
	Verified bool
	Identity Identity
	Provider string
	Model    string
	// Path is the request path. Some providers name the model or the
	// operation there and not in the body.
	Path string
	// Query is the request's query string; Key leaves the credential out.
	Query string
	// Body is the body as sent to the provider, after any redaction.
	Body []byte
	// Policy names the policy the answer is scanned under: a digest of its
	// content, the same on every replica that runs the same policy. An entry
	// is only ever found again under the policy that let it be stored, so a
	// change of policy needs no eviction: the next request asks under another
	// key, and what the old policy stored ages out.
	Policy string
	// Scan names whatever else the scanners work from that can differ between
	// two moments or two replicas: the scanner build and the patterns stored
	// at runtime.
	Scan string
}

// localKeyPrefix marks a key built on a credential digest. Such an entry is
// not mirrored to Redis: no other process could ever ask for it.
const localKeyPrefix = "l:"

// Key hashes the parts into a cache key. It returns "" when the identity
// names nobody, and "" is never looked up or stored.
func Key(p KeyParts) string {
	if !p.Identity.OK() {
		return ""
	}
	h := sha256.New()
	writeField(h, "tamga-cache-v3")
	if p.Verified {
		writeField(h, "verified")
	} else {
		writeField(h, "claimed")
	}
	writeField(h, p.Org)
	writeField(h, p.Identity.value)
	writeField(h, p.Provider)
	writeField(h, p.Model)
	writeField(h, p.Path)
	writeField(h, queryWithoutCredential(p.Query))
	writeField(h, p.Policy)
	writeField(h, p.Scan)
	h.Write(p.Body)
	key := hex.EncodeToString(h.Sum(nil))
	if p.Identity.local {
		return localKeyPrefix + key
	}
	return key
}

// queryWithoutCredential is the query in a stable order with the key
// parameter removed: the credential is in the identity, as a digest.
func queryWithoutCredential(raw string) string {
	if raw == "" {
		return ""
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		// Unreadable, so not safe to pick apart. Hashed whole it can only
		// make two requests differ.
		sum := sha256.Sum256([]byte(raw))
		return "raw:" + hex.EncodeToString(sum[:])
	}
	q.Del(credentialQueryParam)
	return q.Encode()
}

// KeyFor hashes the provider + model + body into a stable key.
//
// Deprecated: the key names no caller. Use Key.
func KeyFor(provider, model string, body []byte) string {
	return KeyForOrg("", provider, model, body)
}

// KeyForOrg hashes orgID + provider + model + body.
//
// Deprecated: the key names no caller, so anyone in the organisation who
// sends the same body is given the answer. The proxy uses Key.
func KeyForOrg(orgID, provider, model string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(orgID))
	h.Write([]byte{0})
	h.Write([]byte(provider))
	h.Write([]byte{0})
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// ttlOf is how long an entry lives: its own TTL, and never more than MaxTTL
// (an entry read back from Redis may have been written with any).
func ttlOf(e *Entry) time.Duration {
	if e.TTL <= 0 || e.TTL > MaxTTL {
		return MaxTTL
	}
	return e.TTL
}

// Get returns (entry, true) when the key exists and has not expired.
func (c *Cache) Get(key string) (*Entry, bool) {
	if key == "" {
		return nil, false
	}
	c.mu.Lock()
	now := c.now()
	if el, ok := c.index[key]; ok {
		e := el.Value.(*Entry)
		if now.Sub(e.StoredAt) > ttlOf(e) {
			c.ll.Remove(el)
			delete(c.index, key)
			c.misses++
			c.mu.Unlock()
			return nil, false
		}
		c.ll.MoveToFront(el)
		c.hits++
		c.mu.Unlock()
		return e, true
	}
	flushedAt := c.flushedAt
	c.mu.Unlock()

	// Fall back to Redis (if attached) for a cross-replica cache hit.
	if c.redisEnabled() && !strings.HasPrefix(key, localKeyPrefix) {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		if raw, ok, err := c.rdx.Get(ctx, c.redisKey(key)); err == nil && ok {
			var e Entry
			// The entry must be the one asked for: it is indexed under its
			// own key, and one that names another would be served there.
			if err := json.Unmarshal(raw, &e); err == nil && e.Key == key {
				if now.Sub(e.StoredAt) <= ttlOf(&e) && e.StoredAt.After(flushedAt) {
					c.mu.Lock()
					c.insertLocked(&e)
					c.hits++
					c.mu.Unlock()
					return &e, true
				}
			}
		}
	}
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()
	return nil, false
}

// insertLocked puts e at the front, evicting the oldest when full.
func (c *Cache) insertLocked(e *Entry) {
	if el, ok := c.index[e.Key]; ok {
		el.Value = e
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(e)
	c.index[e.Key] = el
	if c.ll.Len() > c.capacity {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.index, oldest.Value.(*Entry).Key)
		}
	}
}

// Set inserts (or updates) the entry, evicting the oldest when full. The
// entry's TTL is brought under MaxTTL here, so no caller can store for
// longer, and an entry without a StoredAt is stamped now. An entry with no
// key is not stored.
func (c *Cache) Set(e *Entry) {
	if e == nil || e.Key == "" {
		return
	}
	e.TTL = ttlOf(e)
	c.mu.Lock()
	if e.StoredAt.IsZero() {
		e.StoredAt = c.now()
	}
	c.insertLocked(e)
	c.mu.Unlock()

	if c.redisEnabled() && !strings.HasPrefix(e.Key, localKeyPrefix) {
		if payload, err := json.Marshal(e); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			_ = c.rdx.Set(ctx, c.redisKey(e.Key), payload, e.TTL)
		}
	}
}

// Flush empties this process's cache and returns how many entries it held.
//
// A change of policy does not need it: the policy is part of the key (see
// KeyParts.Policy). It is for an operator who wants what is stored gone now.
//
// Entries in Redis are not deleted, since they cannot be listed by what they
// belong to. This process stops serving the ones written before the flush;
// another replica keeps serving them until they expire or it is flushed too.
// "Before" is judged by the clock of the replica that wrote the entry.
func (c *Cache) Flush() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.ll.Len()
	c.ll.Init()
	c.index = map[string]*list.Element{}
	c.flushedAt = c.now()
	return n
}

// Stats returns hit/miss counters.
func (c *Cache) Stats() (hits, misses int64, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.ll.Len()
}
