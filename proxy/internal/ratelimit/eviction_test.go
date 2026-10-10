package ratelimit

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
)

// clock is a time the test moves.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// evictionLimiter is a limiter on the test's clock whose cleanup goroutine
// has no ticker to speak of: it runs only when a map goes over its bound.
func evictionLimiter(t *testing.T, pol *policy.Policy) (*Limiter, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)}
	l := newLimiter(func() *policy.Policy { return pol }, time.Hour, 10*time.Minute)
	l.now = c.now
	t.Cleanup(func() { _ = l.Close() })
	return l, c
}

func rpmPolicy(rpm int) *policy.Policy {
	return &policy.Policy{RateLimit: &policy.RateLimit{MaxRequestsPerMinute: rpm}}
}

func mapLen(m *sync.Map) int {
	n := 0
	m.Range(func(_, _ any) bool { n++; return true })
	return n
}

// The request count of a key was kept for the life of the process, one per
// client address for callers without a key. It goes after a day idle.
func TestEviction_IdleRequestCounters(t *testing.T) {
	l, c := evictionLimiter(t, rpmPolicy(600))
	for i := 0; i < 500; i++ {
		l.Check(fmt.Sprintf("ip:10.0.%d.%d", i/250, i%250))
	}
	c.add(23 * time.Hour)
	l.Check("tk:still-here")
	l.evictIdle(c.now())
	if n := mapLen(&l.perKeyRequests.m); n != 501 {
		t.Fatalf("%d counters after 23 hours, want all 501", n)
	}

	c.add(2 * time.Hour)
	l.evictIdle(c.now())
	if n := mapLen(&l.perKeyRequests.m); n != 1 {
		t.Fatalf("%d counters after 25 hours, want the one key that was used since", n)
	}
	if got := l.perKeyRequests.size.Load(); got != 1 {
		t.Errorf("size = %d, want 1", got)
	}
	if top := l.GetStats().TopKeys; len(top) != 1 || top["tk:still-here"] != 1 {
		t.Errorf("top keys = %v", top)
	}
	// The totals are for the life of the process and are not touched.
	if got := l.GetStats().TotalRequests; got != 501 {
		t.Errorf("total requests = %d, want 501", got)
	}
}

// A token bucket goes after ten minutes idle, a used one stays.
func TestEviction_IdleBuckets(t *testing.T) {
	l, c := evictionLimiter(t, rpmPolicy(600))
	for i := 0; i < 100; i++ {
		l.Check(fmt.Sprintf("key-%d", i))
	}
	c.add(9 * time.Minute)
	l.Check("key-7")
	c.add(2 * time.Minute)
	l.evictIdle(c.now())
	if n := mapLen(&l.m); n != 1 {
		t.Fatalf("%d buckets, want 1", n)
	}
	if _, ok := l.m.Load("key-7"); !ok {
		t.Error("the bucket that was used was dropped")
	}
	if got := l.buckets.Load(); got != 1 {
		t.Errorf("bucket count = %d, want 1", got)
	}
}

// A daily token total has its date in its key. Nothing reads it after that
// day, and nothing dropped it.
func TestEviction_DailyTokenTotalsOfPastDays(t *testing.T) {
	pol := &policy.Policy{RateLimit: &policy.RateLimit{MaxTokensPerDay: 1000}}
	l, _ := evictionLimiter(t, pol)
	for i := 0; i < 50; i++ {
		l.CheckDailyTokenQuota(fmt.Sprintf("key-%d", i), 10)
	}
	// CheckDailyTokenQuota names the day by the wall clock.
	today := time.Now().UTC()
	l.evictIdle(today)
	if n := mapLen(&l.dailyTokens.m); n != 50 {
		t.Fatalf("%d totals on their own day, want 50", n)
	}
	// Still counted: the total is not lost while its day lasts.
	if res := l.CheckDailyTokenQuota("key-3", 10); res.TokensUsed != 20 {
		t.Errorf("tokens used = %d, want 20", res.TokensUsed)
	}
	l.evictIdle(today.Add(24 * time.Hour))
	if n := mapLen(&l.dailyTokens.m); n != 0 {
		t.Fatalf("%d totals the day after, want 0", n)
	}
	if got := l.dailyTokens.size.Load(); got != 0 {
		t.Errorf("size = %d, want 0", got)
	}
}

// Idle time alone bounds nothing when the names arrive faster than they
// age. Past maxKeys the keys used longest ago go, in use or not.
func TestEviction_BoundOnKeys(t *testing.T) {
	l, c := evictionLimiter(t, rpmPolicy(600))
	l.maxKeys = 100
	// The cleanup goroutine would do the same on its own; it is stopped so
	// that the numbers below are this call's.
	_ = l.Close()

	for i := 0; i < 1000; i++ {
		c.add(time.Millisecond)
		l.Check(fmt.Sprintf("key:%04d", i))
	}
	l.evictIdle(c.now())
	for name, n := range map[string]int{"buckets": mapLen(&l.m), "counters": mapLen(&l.perKeyRequests.m)} {
		if n != 90 {
			t.Errorf("%d %s, want 90 (a tenth below the bound of 100)", n, name)
		}
	}
	// The newest are the ones kept.
	for _, key := range []string{"key:0999", "key:0910"} {
		if _, ok := l.perKeyRequests.m.Load(key); !ok {
			t.Errorf("%s was dropped", key)
		}
		if _, ok := l.m.Load(key); !ok {
			t.Errorf("the bucket of %s was dropped", key)
		}
	}
	if _, ok := l.perKeyRequests.m.Load("key:0909"); ok {
		t.Error("key:0909 was kept: it is older than the 90 kept")
	}
}

// Going over the bound wakes the cleanup at once, not at the next tick (an
// hour away here).
func TestEviction_BoundWakesCleanup(t *testing.T) {
	l, c := evictionLimiter(t, rpmPolicy(600))
	l.maxKeys = 50
	for i := 0; i < 5000; i++ {
		c.add(time.Millisecond)
		l.Check(fmt.Sprintf("key:%04d", i))
	}
	// The goroutine trims after the fact, so the map can be over for a
	// moment; it settles at or under the bound.
	for i := 0; ; i++ {
		l.cleanupNow()
		if mapLen(&l.perKeyRequests.m) <= 50 && mapLen(&l.m) <= 50 {
			break
		}
		if i == 500 {
			t.Fatalf("still %d counters and %d buckets", mapLen(&l.perKeyRequests.m), mapLen(&l.m))
		}
		time.Sleep(time.Millisecond)
	}
}

// Close ends the cleanup goroutine, and a limiter that is closed still
// answers.
func TestEviction_CloseStopsTheGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	var ls []*Limiter
	for i := 0; i < 20; i++ {
		ls = append(ls, NewLimiter(func() *policy.Policy { return rpmPolicy(60) }))
	}
	for _, l := range ls {
		_ = l.Close()
		_ = l.Close()
	}
	for i := 0; runtime.NumGoroutine() > before; i++ {
		if i == 500 {
			t.Fatalf("%d goroutines, %d before the limiters were made", runtime.NumGoroutine(), before)
		}
		time.Sleep(time.Millisecond)
	}
	l := ls[0]
	l.maxKeys = 1
	for i := 0; i < 10; i++ {
		// Over the bound with nobody to wake: must not block.
		if !l.Check(fmt.Sprintf("k%d", i)).Allowed {
			t.Fatal("a closed limiter refused a first request")
		}
	}
}

func TestOldestOver(t *testing.T) {
	keys := func(n int) []keyAge {
		out := make([]keyAge, n)
		for i := range out {
			out[i] = keyAge{key: i, lastUsed: int64(n - i)} // the last is the oldest
		}
		return out
	}
	if got := oldestOver(keys(100), 100); got != nil {
		t.Errorf("at the bound: dropped %d", len(got))
	}
	got := oldestOver(keys(101), 100)
	if len(got) != 11 {
		t.Fatalf("one over a bound of 100: dropped %d, want 11", len(got))
	}
	if got[0] != 100 || got[10] != 90 {
		t.Errorf("dropped %v, want the oldest first", got)
	}
}
