package webhooks

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is the real clock moved forward by hand, so a cooldown can pass
// without the test waiting for it.
type fakeClock struct{ ahead atomic.Int64 }

func (c *fakeClock) now() time.Time          { return time.Now().Add(time.Duration(c.ahead.Load())) }
func (c *fakeClock) advance(d time.Duration) { c.ahead.Add(int64(d)) }

const (
	answers int32 = iota
	hangs
	errors500
)

// moodyReceiver answers, hangs or fails, as the test says.
type moodyReceiver struct {
	*httptest.Server
	mood    atomic.Int32
	hits    atomic.Int32
	release chan struct{}
}

func newMoodyReceiver(t *testing.T, mood int32) *moodyReceiver {
	t.Helper()
	r := &moodyReceiver{release: make(chan struct{})}
	r.mood.Store(mood)
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		r.hits.Add(1)
		switch r.mood.Load() {
		case hangs:
			select {
			case <-r.release:
			case <-req.Context().Done():
			}
		case errors500:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(func() {
		close(r.release)
		r.Close()
	})
	return r
}

func destKnown(d *Dispatcher, key string) bool {
	d.destMu.Lock()
	defer d.destMu.Unlock()
	_, ok := d.dests[key]
	return ok
}

// One destination hangs and one answers. Alerts to the one that answers all
// arrive before the hanging one has finished a single delivery; the hanging
// one's breaker opens and its alerts are skipped; after the cooldown exactly
// one attempt is let through; and when it recovers, its state is let go.
func TestDispatcher_HangingDestinationDoesNotStarveOthers(t *testing.T) {
	const attempt = 500 * time.Millisecond
	// Shorter than destStateTTL, or the state would be forgotten instead.
	const cooldown = 5 * time.Minute
	clk := &fakeClock{}
	bad := newMoodyReceiver(t, hangs)
	good := newMoodyReceiver(t, answers)
	store := loopbackStore()
	wBad := mustCreate(t, store, Webhook{Kind: KindSlack, URL: bad.URL, Enabled: true})
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: good.URL, Enabled: true})
	d := NewDispatcher(store, DispatcherConfig{
		DefaultOrg:      "default",
		AttemptTimeout:  attempt,
		MaxAttempts:     2,
		Backoff:         5 * time.Millisecond,
		Guard:           loopbackGuard(),
		BreakerCooldown: cooldown,
		now:             clk.now,
	})
	defer d.Stop(50 * time.Millisecond)
	key := "dest:" + wBad.ID

	const n = 12
	start := time.Now()
	for i := 0; i < n; i++ {
		d.HandleEvent(blockEvent(fmt.Sprintf("req-%d", i)))
	}
	waitFor(t, "the healthy destination to get every alert", func() bool { return statsOf(d, KindGeneric).Sent == n })
	if took := time.Since(start); took > attempt {
		t.Fatalf("the healthy destination waited %v behind the hanging one", took)
	}
	if st := statsOf(d, KindSlack); st.Failed != 0 || st.Skipped != 0 {
		t.Fatalf("the hanging destination had already finished a delivery: %+v", st)
	}
	// It holds two of the four workers and no more.
	d.destMu.Lock()
	inflight := d.dests[key].inflight
	d.destMu.Unlock()
	if inflight != 2 {
		t.Errorf("the hanging destination holds %d workers, want 2", inflight)
	}

	// Two deliveries fail, then two more; the third failure opens the
	// breaker and everything still waiting is skipped.
	waitFor(t, "the breaker to open", func() bool {
		st := statsOf(d, KindSlack)
		return st.Failed+st.Skipped == n
	})
	st := statsOf(d, KindSlack)
	if st.Failed < 3 || st.Failed > 4 || st.Skipped != n-st.Failed || st.Sent != 0 || st.Dropped != 0 {
		t.Fatalf("after the burst: %+v, want 3 or 4 failed and the rest skipped", st)
	}
	if h := bad.hits.Load(); h > int32(st.Failed)*2 {
		t.Errorf("the hanging receiver was tried %d times for %d failed deliveries", h, st.Failed)
	}

	// While it is open, nothing is attempted.
	hits := bad.hits.Load()
	d.HandleEvent(blockEvent("req-open"))
	waitFor(t, "the skip", func() bool { return statsOf(d, KindSlack).Skipped == st.Skipped+1 })
	if bad.hits.Load() != hits {
		t.Fatal("an alert was attempted while the breaker was open")
	}
	waitFor(t, "the healthy destination's next alert", func() bool { return statsOf(d, KindGeneric).Sent == n+1 })

	// After the cooldown one attempt goes through, once, and fails.
	clk.advance(cooldown + time.Second)
	d.HandleEvent(blockEvent("req-trial"))
	d.HandleEvent(blockEvent("req-behind-the-trial"))
	waitFor(t, "the trial", func() bool {
		now := statsOf(d, KindSlack)
		return now.Failed == st.Failed+1 && now.Skipped == st.Skipped+2
	})
	if got := bad.hits.Load(); got != hits+1 {
		t.Fatalf("the trial made %d attempts, want 1", got-hits)
	}
	if !destKnown(d, key) {
		t.Fatal("the breaker's state is gone while it is open")
	}

	// It recovers: the next trial closes the breaker and the state goes.
	bad.mood.Store(answers)
	clk.advance(cooldown + time.Second)
	d.HandleEvent(blockEvent("req-recovered"))
	waitFor(t, "the recovery", func() bool { return statsOf(d, KindSlack).Sent == 1 })
	waitFor(t, "the state to be released", func() bool { return !destKnown(d, key) })
	d.HandleEvent(blockEvent("req-after"))
	waitFor(t, "normal service", func() bool { return statsOf(d, KindSlack).Sent == 2 })
}

// What the dispatcher remembers about a destination does not outlive it.
func TestDispatcher_DestinationStateIsReleased(t *testing.T) {
	clk := &fakeClock{}
	bad := newMoodyReceiver(t, errors500)
	badRule := newMoodyReceiver(t, errors500)
	good := newMoodyReceiver(t, answers)
	store := loopbackStore()
	wBad := mustCreate(t, store, Webhook{Kind: KindSlack, URL: bad.URL, Enabled: true})
	mustCreate(t, store, Webhook{Kind: KindGeneric, URL: good.URL, Enabled: true})
	d := NewDispatcher(store, DispatcherConfig{
		DefaultOrg:       "default",
		AttemptTimeout:   2 * time.Second,
		MaxAttempts:      1,
		Guard:            loopbackGuard(),
		BreakerThreshold: 1,
		BreakerCooldown:  time.Hour,
		now:              clk.now,
	})
	defer d.Stop(time.Second)
	destKey, ruleKey := "dest:"+wBad.ID, "rule:"+badRule.URL

	d.HandleEvent(blockEvent("req-1"))
	d.NotifyURLs([]string{badRule.URL}, Alert{RequestID: "req-1", OrgID: "acme", Action: "BLOCK"})
	waitFor(t, "both to fail", func() bool {
		return statsOf(d, KindSlack).Failed == 1 && statsOf(d, KindRuleNotify).Failed == 1 && statsOf(d, KindGeneric).Sent == 1
	})
	if !destKnown(d, destKey) || !destKnown(d, ruleKey) {
		t.Fatal("a failing destination should be remembered")
	}
	d.destMu.Lock()
	remembered := len(d.dests)
	d.destMu.Unlock()
	if remembered != 2 {
		t.Fatalf("%d destinations are remembered, want the two that fail: the healthy one leaves nothing behind", remembered)
	}

	// The saved destination is deleted: the next sweep lets its state go.
	// The rule's URL is still within its idle time and stays.
	if err := store.Delete(wBad.ID); err != nil {
		t.Fatal(err)
	}
	clk.advance(destSweepEvery + time.Second)
	d.HandleEvent(blockEvent("req-2"))
	waitFor(t, "the deleted destination's state to go", func() bool { return !destKnown(d, destKey) })
	if !destKnown(d, ruleKey) {
		t.Fatal("the rule URL's state went early")
	}

	// A rule URL that is not used again is forgotten after the idle time.
	clk.advance(destStateTTL + time.Second)
	d.HandleEvent(blockEvent("req-3"))
	waitFor(t, "the idle rule URL's state to go", func() bool { return !destKnown(d, ruleKey) })
}

// Rule notify URLs come from a policy someone can edit, so the number of
// destinations remembered has a ceiling, and the one used longest ago makes
// room.
func TestDispatcher_DestinationStateIsBounded(t *testing.T) {
	clk := &fakeClock{}
	d := NewDispatcher(nil, DispatcherConfig{now: clk.now})
	defer d.Stop(time.Second)

	base := clk.now()
	d.destMu.Lock()
	d.lastSweep = base
	for i := 0; i < maxDestStates; i++ {
		d.dests[fmt.Sprintf("rule:https://hooks.example.com/%d", i)] = &destState{fails: 1, openUntil: base.Add(time.Hour), lastUsed: base.Add(-time.Duration(maxDestStates-i) * time.Second)}
	}
	for i := 0; i < 500; i++ {
		d.state(fmt.Sprintf("rule:https://attacker.example/%d", i))
	}
	size := len(d.dests)
	_, oldest := d.dests["rule:https://hooks.example.com/0"]
	_, newest := d.dests["rule:https://attacker.example/499"]
	d.destMu.Unlock()
	if size > maxDestStates {
		t.Fatalf("%d destinations are remembered, the ceiling is %d", size, maxDestStates)
	}
	if oldest || !newest {
		t.Errorf("eviction kept the oldest (%v) or lost the newest (%v)", oldest, !newest)
	}
}

// Alerts waiting for a busy destination are bounded too: beyond a queue's
// worth they are dropped and counted.
func TestDispatcher_WaitingAlertsAreBounded(t *testing.T) {
	bad := newMoodyReceiver(t, hangs)
	store := loopbackStore()
	mustCreate(t, store, Webhook{Kind: KindSlack, URL: bad.URL, Enabled: true})
	d := NewDispatcher(store, DispatcherConfig{
		DefaultOrg: "default", QueueSize: 4, Workers: 4, AttemptTimeout: 30 * time.Second, MaxAttempts: 1, Guard: loopbackGuard(),
	})
	defer d.Stop(10 * time.Millisecond)

	for i := 0; i < 200; i++ {
		d.HandleEvent(blockEvent("req-n"))
		if i%10 == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	waitFor(t, "the overflow to be dropped", func() bool { return statsOf(d, KindSlack).Dropped > 0 })
	d.destMu.Lock()
	parked, inflight := d.parked, 0
	for _, st := range d.dests {
		inflight += st.inflight
	}
	d.destMu.Unlock()
	if parked > 4 || inflight > 2 {
		t.Fatalf("%d alerts wait and %d are in flight; the limits are 4 and 2", parked, inflight)
	}
	if h := bad.hits.Load(); h > 2 {
		t.Errorf("the hanging receiver has %d requests open, want at most 2", h)
	}
}

// The probe against something that is not an HTTP server reports a class.
// What the peer sent is in the transport's error text and must not be.
func TestStore_ProbeAgainstNonHTTPListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9.9 SECRETBANNER\r\n"))
			_ = c.Close()
		}
	}()
	s := loopbackStore()
	w := mustCreate(t, s, Webhook{Kind: KindGeneric, URL: "http://" + ln.Addr().String() + "/x"})
	status, err := s.Test(context.Background(), w.ID)
	if status != 0 || err == nil || err.Error() != "test failed: network" {
		t.Fatalf("probe: %d %v, want a fixed class", status, err)
	}
	for _, bad := range []string{"SECRET", "SSH", "127.0.0.1", "malformed"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("the probe's error carries %q: %v", bad, err)
		}
	}

	// The same listener behind the strict guard: refused before any
	// connection, the same way whatever is listening.
	strict := NewMemoryStore()
	storeUnchecked(strict, Webhook{ID: "x", Kind: KindGeneric, URL: "http://" + ln.Addr().String() + "/x"})
	if _, err := strict.Test(context.Background(), "x"); err != ErrDestinationNotAllowed {
		t.Fatalf("strict probe: %v", err)
	}
}
