package classifier

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeScorer struct {
	mu    sync.Mutex
	calls [][]string
	err   error
	delay time.Duration
}

func (f *fakeScorer) Score(ctx context.Context, _ string, texts []string) ([]Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), texts...))
	err, delay := f.err, f.delay
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]Result, len(texts))
	for i, t := range texts {
		if t == "attack" {
			out[i].Score = 0.99
		}
		if t == "huge" {
			out[i].Truncated = true
		}
	}
	return out, nil
}

func (f *fakeScorer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestGuard_OnlyNewTextIsScored(t *testing.T) {
	f := &fakeScorer{}
	g := NewGuard(f)
	ctx := context.Background()

	// Turn one of a conversation.
	res, err := g.Score(ctx, "r1", []string{"hello", "attack"}, time.Second)
	if err != nil || res[0].Score != 0 || res[1].Score != 0.99 {
		t.Fatalf("turn 1: %+v %v", res, err)
	}
	// Turn two resends both and adds one.
	res, err = g.Score(ctx, "r2", []string{"hello", "attack", "new message"}, time.Second)
	if err != nil || res[1].Score != 0.99 {
		t.Fatalf("turn 2: %+v %v", res, err)
	}
	if len(f.calls) != 2 || len(f.calls[1]) != 1 || f.calls[1][0] != "new message" {
		t.Fatalf("calls = %v, want the second call to carry only the new message", f.calls)
	}
	// Turn three is all cached: no call at all.
	if _, err := g.Score(ctx, "r3", []string{"hello", "attack", "new message"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if f.callCount() != 2 {
		t.Fatalf("calls = %d, want 2", f.callCount())
	}
	if s := g.Stats(); s.Calls != 2 || s.CachedTexts != 5 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestGuard_CacheExpires(t *testing.T) {
	f := &fakeScorer{}
	g := NewGuard(f)
	now := time.Now()
	g.now = func() time.Time { return now }
	_, _ = g.Score(context.Background(), "r", []string{"hello"}, time.Second)
	now = now.Add(cacheTTL + time.Second)
	_, _ = g.Score(context.Background(), "r", []string{"hello"}, time.Second)
	if f.callCount() != 2 {
		t.Fatalf("calls = %d, want the expired entry scored again", f.callCount())
	}
}

// A truncated result says nothing about the part that was not read.
func TestGuard_TruncatedResultIsNotCached(t *testing.T) {
	f := &fakeScorer{}
	g := NewGuard(f)
	for i := 0; i < 2; i++ {
		res, err := g.Score(context.Background(), "r", []string{"huge"}, time.Second)
		if err != nil || !res[0].Truncated {
			t.Fatalf("%+v %v", res, err)
		}
	}
	if f.callCount() != 2 {
		t.Fatalf("calls = %d, want 2", f.callCount())
	}
}

func TestGuard_DeadlineIsEnforced(t *testing.T) {
	f := &fakeScorer{delay: 500 * time.Millisecond}
	g := NewGuard(f)
	start := time.Now()
	_, err := g.Score(context.Background(), "r", []string{"slow"}, 30*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if took := time.Since(start); took > 250*time.Millisecond {
		t.Fatalf("waited %v for a 30 ms deadline", took)
	}
}

// A service that is down must stop costing its timeout on every request.
func TestGuard_BreakerOpensAndRecovers(t *testing.T) {
	f := &fakeScorer{err: errors.New("connection refused")}
	g := NewGuard(f)
	now := time.Now()
	g.now = func() time.Time { return now }
	ctx := context.Background()

	for i := 0; i < breakerThreshold; i++ {
		if _, err := g.Score(ctx, "r", []string{"t"}, time.Second); err == nil {
			t.Fatal("want an error")
		}
	}
	if _, err := g.Score(ctx, "r", []string{"t"}, time.Second); !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
	if f.callCount() != breakerThreshold {
		t.Fatalf("calls = %d: the open breaker still called the service", f.callCount())
	}
	// Cached text is still answered while the breaker is open.
	g.cache[keyOf("known")] = cacheEntry{res: Result{Score: 0.7}, seen: now}
	if res, err := g.Score(ctx, "r", []string{"known"}, time.Second); err != nil || res[0].Score != 0.7 {
		t.Fatalf("cached while open: %+v %v", res, err)
	}

	// After the cooldown one request is let through; success closes it.
	now = now.Add(breakerCooldown + time.Second)
	f.mu.Lock()
	f.err = nil
	f.mu.Unlock()
	if _, err := g.Score(ctx, "r", []string{"t"}, time.Second); err != nil {
		t.Fatalf("after cooldown: %v", err)
	}
	if _, err := g.Score(ctx, "r", []string{"t2"}, time.Second); err != nil {
		t.Fatalf("closed again: %v", err)
	}
	if s := g.Stats(); s.Errors != breakerThreshold || s.ShortCircuited != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestNewClient_EmptyAddrMeansNoClassifier(t *testing.T) {
	c, err := NewClient("")
	if c != nil || err != nil {
		t.Fatalf("c=%v err=%v", c, err)
	}
	if NewGuard(nil) != nil {
		t.Fatal("a nil scorer must give a nil guard")
	}
}

func keyOf(s string) [32]byte { return sha256.Sum256([]byte(s)) }
