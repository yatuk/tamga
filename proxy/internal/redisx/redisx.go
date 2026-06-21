// Package redisx is a minimal Redis facade used by the cache, budget and
// rate-limit packages to upgrade to distributed mode. When REDIS_URL is
// empty the code runs with the in-memory default and Client.Enabled()
// returns false so callers can route accordingly.
package redisx

import (
	"context"
	"errors"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Client is the narrow surface the proxy needs.
type Client interface {
	Enabled() bool
	Ping(ctx context.Context) error
	Incr(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)
	IncrFloat(ctx context.Context, key string, delta float64, ttl time.Duration) (float64, error)
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Close() error
}

// NewFromURL returns a concrete Client. When url is empty we return an
// in-memory fallback so callers can still operate single-node.
// When url is set (e.g. "redis://host:6379/0") we connect using go-redis/v9.
// If the ping fails we fall back to the in-memory client and mark it as
// "url set but unreachable" — callers still work but lose distributed mode.
func NewFromURL(url string) Client {
	if url == "" {
		return &memClient{data: map[string]memEntry{}}
	}
	opt, err := goredis.ParseURL(url)
	if err != nil {
		return &memClient{data: map[string]memEntry{}, url: url}
	}
	rdb := goredis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return &memClient{data: map[string]memEntry{}, url: url}
	}
	return &redisClient{rdb: rdb, url: url}
}

// redisClient talks to a live Redis instance.
type redisClient struct {
	rdb *goredis.Client
	url string
}

func (r *redisClient) Enabled() bool { return true }

func (r *redisClient) Ping(ctx context.Context) error { return r.rdb.Ping(ctx).Err() }

func (r *redisClient) Incr(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	v, err := r.rdb.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return 0, err
	}
	if ttl > 0 {
		_ = r.rdb.Expire(ctx, key, ttl).Err()
	}
	return v, nil
}

func (r *redisClient) IncrFloat(ctx context.Context, key string, delta float64, ttl time.Duration) (float64, error) {
	v, err := r.rdb.IncrByFloat(ctx, key, delta).Result()
	if err != nil {
		return 0, err
	}
	if ttl > 0 {
		_ = r.rdb.Expire(ctx, key, ttl).Err()
	}
	return v, nil
}

func (r *redisClient) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return b, true, nil
}

func (r *redisClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.rdb.Set(ctx, key, value, ttl).Err()
}

func (r *redisClient) Del(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, key).Err()
}

func (r *redisClient) Close() error {
	if r.rdb == nil {
		return nil
	}
	return r.rdb.Close()
}

// memClient is the in-process fallback used when REDIS_URL is empty or
// unreachable. It preserves the semantics of Client so the rest of the
// proxy can call the same methods without branching.
type memEntry struct {
	val []byte
	exp time.Time
}

type memClient struct {
	mu   sync.RWMutex
	data map[string]memEntry
	url  string // set when a URL was given but couldn't be used (telemetry only)
}

func (m *memClient) Enabled() bool { return false }

func (m *memClient) Ping(context.Context) error { return nil }

func (m *memClient) Incr(_ context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.data[key]
	var cur int64
	if e.val != nil {
		for _, b := range e.val {
			if b < '0' || b > '9' {
				continue
			}
			cur = cur*10 + int64(b-'0')
		}
	}
	cur += delta
	val := make([]byte, 0, 20)
	val = appendInt(val, cur)
	exp := time.Now().Add(ttl)
	if ttl <= 0 {
		exp = time.Time{}
	}
	m.data[key] = memEntry{val: val, exp: exp}
	return cur, nil
}

func (m *memClient) IncrFloat(_ context.Context, key string, delta float64, ttl time.Duration) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.data[key]
	var cur float64
	if e.val != nil {
		_, _ = parseFloat(e.val, &cur)
	}
	cur += delta
	val := formatFloat(cur)
	exp := time.Now().Add(ttl)
	if ttl <= 0 {
		exp = time.Time{}
	}
	m.data[key] = memEntry{val: val, exp: exp}
	return cur, nil
}

func (m *memClient) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	e, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if !e.exp.IsZero() && time.Now().After(e.exp) {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return nil, false, nil
	}
	return e.val, true, nil
}

func (m *memClient) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return errors.New("empty key")
	}
	exp := time.Now().Add(ttl)
	if ttl <= 0 {
		exp = time.Time{}
	}
	m.mu.Lock()
	m.data[key] = memEntry{val: value, exp: exp}
	m.mu.Unlock()
	return nil
}

func (m *memClient) Del(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

func (m *memClient) Close() error { return nil }

func appendInt(dst []byte, v int64) []byte {
	if v == 0 {
		return append(dst, '0')
	}
	if v < 0 {
		dst = append(dst, '-')
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return append(dst, buf[i:]...)
}

func parseFloat(b []byte, out *float64) (int, error) {
	var sign float64 = 1
	i := 0
	if i < len(b) && b[i] == '-' {
		sign = -1
		i++
	}
	var whole, frac float64
	var div float64 = 1
	seenDot := false
	for ; i < len(b); i++ {
		c := b[i]
		if c == '.' {
			seenDot = true
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		d := float64(c - '0')
		if seenDot {
			div *= 10
			frac = frac*10 + d
		} else {
			whole = whole*10 + d
		}
	}
	*out = sign * (whole + frac/div)
	return i, nil
}

func formatFloat(v float64) []byte {
	// Keep 6 decimal digits; sufficient for USD cost tracking.
	neg := v < 0
	if neg {
		v = -v
	}
	whole := int64(v)
	frac := int64((v - float64(whole)) * 1e6)
	dst := make([]byte, 0, 24)
	if neg {
		dst = append(dst, '-')
	}
	dst = appendInt(dst, whole)
	dst = append(dst, '.')
	// 6 digit fractional, zero-padded
	var buf [6]byte
	for i := 5; i >= 0; i-- {
		buf[i] = byte('0' + frac%10)
		frac /= 10
	}
	dst = append(dst, buf[:]...)
	return dst
}
