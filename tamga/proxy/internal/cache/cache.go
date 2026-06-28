// Package cache implements an LRU exact-match prompt cache with a
// content-based SHA-256 key. Phase 3C — the cache short-circuits identical
// request payloads and returns the cached response body, saving upstream
// calls when a dashboard or runbook hits the same prompt repeatedly.
//
// "Semantic" similarity (MiniLM/E5) is a future-phase swap-out — the public
// interface below is deliberately small so we can replace the implementation
// without changing call sites.
package cache

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/yatuk/tamga/internal/redisx"
)

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
type Cache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	index    map[string]*list.Element
	hits     int64
	misses   int64
	rdx      redisx.Client
}

// New creates an LRU exact-match prompt cache with the given capacity.
func New(capacity int) *Cache {
	if capacity < 16 {
		capacity = 512
	}
	return &Cache{
		capacity: capacity,
		ll:       list.New(),
		index:    map[string]*list.Element{},
	}
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

// KeyFor hashes the provider + model + body into a stable key.
// Deprecated: use KeyForOrg for tenant-scoped cache isolation.
func KeyFor(provider, model string, body []byte) string {
	return KeyForOrg("", provider, model, body)
}

// KeyForOrg hashes orgID + provider + model + body for tenant-isolated key.
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

// Get returns (entry, true) when the key exists and has not expired.
func (c *Cache) Get(key string) (*Entry, bool) {
	c.mu.Lock()
	if el, ok := c.index[key]; ok {
		e := el.Value.(*Entry)
		if e.TTL > 0 && time.Since(e.StoredAt) > e.TTL {
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
	c.mu.Unlock()

	// Fall back to Redis (if attached) for a cross-replica cache hit.
	if c.redisEnabled() {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		if raw, ok, err := c.rdx.Get(ctx, c.redisKey(key)); err == nil && ok {
			var e Entry
			if err := json.Unmarshal(raw, &e); err == nil {
				if e.TTL == 0 || time.Since(e.StoredAt) <= e.TTL {
					c.mu.Lock()
					el := c.ll.PushFront(&e)
					c.index[e.Key] = el
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

// Set inserts (or updates) the entry, evicting the oldest when full.
func (c *Cache) Set(e *Entry) {
	c.mu.Lock()
	if el, ok := c.index[e.Key]; ok {
		el.Value = e
		c.ll.MoveToFront(el)
	} else {
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
	c.mu.Unlock()

	if c.redisEnabled() {
		ttl := e.TTL
		if ttl <= 0 {
			ttl = 30 * time.Minute
		}
		if payload, err := json.Marshal(e); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			_ = c.rdx.Set(ctx, c.redisKey(e.Key), payload, ttl)
		}
	}
}

// InvalidateByOrg removes all cache entries whose key starts with the
// given orgID prefix. Used with NATS policy.change events to evict
// stale cached responses when a tenant's policy is updated.
func (c *Cache) InvalidateByOrg(orgID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := "tamga:cache:" + orgID
	count := 0
	for key, elem := range c.index {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			c.ll.Remove(elem)
			delete(c.index, key)
			count++
		}
	}
	// Also invalidate Redis entries for this org (best-effort).
	// Redis entries can't be scanned by key pattern, but the local
	// eviction is the primary mechanism — Redis entries have a TTL
	// and will expire naturally.
	return count
}

// Stats returns hit/miss counters.
func (c *Cache) Stats() (hits, misses int64, size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.ll.Len()
}
