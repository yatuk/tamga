package webhooks

import (
	"testing"
	"time"
)

func TestCorrelationEngine_SingleEventFiresImmediately(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// No threshold: zero threshold means fire immediately.
	fired, count := ce.ShouldFire("wh-1", "key-a", 0, 300, 0)
	if !fired {
		t.Fatal("expected immediate fire with zero threshold")
	}
	if count != 1 {
		t.Errorf("expected correlated_count=1, got %d", count)
	}
}

func TestCorrelationEngine_ThresholdNotMet(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// 4 events with threshold=5 → all suppressed.
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 300, 0)
		if fired {
			t.Fatalf("event %d: expected suppressed (threshold not met)", i+1)
		}
	}

	if s := ce.Size(); s != 1 {
		t.Errorf("expected 1 entry tracked, got %d", s)
	}
}

func TestCorrelationEngine_ThresholdMet(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// First 4 events below threshold.
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 300, 0)
		if fired {
			t.Fatalf("event %d: expected suppressed", i+1)
		}
	}

	// 5th event meets threshold → fires with correlated_count=5.
	fired, count := ce.ShouldFire("wh-1", "key-a", 5, 300, 0)
	if !fired {
		t.Fatal("expected fire when threshold met")
	}
	if count != 5 {
		t.Errorf("expected correlated_count=5, got %d", count)
	}
}

func TestCorrelationEngine_CooldownPreventsDuplicate(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Fire 5 events to trigger the first burst.
	for i := 0; i < 5; i++ {
		ce.ShouldFire("wh-1", "key-a", 5, 300, 60)
	}

	// Now the window was reset after the fire. Add 5 more events.
	// These should fire again because the threshold is met.
	for i := 0; i < 5; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 300, 60)
		if fired {
			t.Fatalf("event %d in second burst: expected cooldown suppression (fired too soon)", i+1)
		}
	}

	if s := ce.Size(); s != 1 {
		t.Errorf("expected 1 entry tracked, got %d", s)
	}
}

func TestCorrelationEngine_CooldownExpires(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Fire 5 events to trigger the first burst.
	for i := 0; i < 5; i++ {
		ce.ShouldFire("wh-1", "key-a", 5, 300, 1)
	}

	// Wait for cooldown to expire.
	time.Sleep(1100 * time.Millisecond)

	// Now another 5 events should fire again.
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 300, 1)
		if fired {
			t.Fatalf("event %d: expected suppressed (need 5th)", i+1)
		}
	}
	fired, count := ce.ShouldFire("wh-1", "key-a", 5, 300, 1)
	if !fired {
		t.Fatal("expected fire after cooldown expired")
	}
	if count != 5 {
		t.Errorf("expected correlated_count=5, got %d", count)
	}
}

func TestCorrelationEngine_SlidingWindowExpiry(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Send 4 events with a very short window.
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 1, 0)
		if fired {
			t.Fatalf("event %d: expected suppressed", i+1)
		}
	}

	// Wait for the window to expire.
	time.Sleep(1100 * time.Millisecond)

	// First 4 events are pruned (outside the 1s window).
	// Send 4 new events — all suppressed (below threshold 5).
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 1, 0)
		if fired {
			t.Fatalf("event %d after sleep: expected suppressed, only %d in window", i+1, i+1)
		}
	}

	// 5th event after sleep → fires with correlated_count=5.
	fired, count := ce.ShouldFire("wh-1", "key-a", 5, 1, 0)
	if !fired {
		t.Fatal("expected fire after window expiry and new accumulation (5 new events)")
	}
	if count != 5 {
		t.Errorf("expected correlated_count=5, got %d", count)
	}
}

func TestCorrelationEngine_MaxKeysEviction(t *testing.T) {
	// Tiny max so we trigger eviction.
	ce := NewCorrelationEngine(3)

	// Fill with 3 entries.
	for i := 0; i < 3; i++ {
		ce.ShouldFire("wh-1", "key-"+string(rune('a'+i)), 5, 300, 0)
	}
	if s := ce.Size(); s != 3 {
		t.Fatalf("expected 3 entries, got %d", s)
	}

	// Adding a 4th entry should evict the LRU.
	ce.ShouldFire("wh-1", "key-d", 5, 300, 0)
	if s := ce.Size(); s != 3 {
		t.Fatalf("expected 3 entries after eviction, got %d", s)
	}
}

func TestCorrelationEngine_DifferentKeysIndependent(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Key A: 4 events (below threshold 5).
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-a", 5, 300, 0)
		if fired {
			t.Fatal("key-a: expected suppressed")
		}
	}

	// Key B: 5 events (meets threshold 5).
	for i := 0; i < 4; i++ {
		fired, _ := ce.ShouldFire("wh-1", "key-b", 5, 300, 0)
		if fired {
			t.Fatal("key-b: expected suppressed at event", i+1)
		}
	}
	fired, count := ce.ShouldFire("wh-1", "key-b", 5, 300, 0)
	if !fired {
		t.Fatal("key-b: expected fire on 5th event")
	}
	if count != 5 {
		t.Errorf("key-b: expected correlated_count=5, got %d", count)
	}

	if s := ce.Size(); s != 2 {
		t.Errorf("expected 2 entries (both keys tracked), got %d", s)
	}
}

func TestCorrelationEngine_ZeroThreshold(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Zero threshold = fire immediately regardless of event count.
	fired, count := ce.ShouldFire("wh-1", "key-a", 0, 300, 0)
	if !fired {
		t.Fatal("expected immediate fire with zero threshold")
	}
	if count != 1 {
		t.Errorf("expected correlated_count=1, got %d", count)
	}
}

func TestCorrelationEngine_DifferentWebhookIDs(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Same correlation key, different webhook IDs → tracked independently.
	for i := 0; i < 3; i++ {
		fired, _ := ce.ShouldFire("wh-1", "shared-key", 5, 300, 0)
		if fired {
			t.Fatal("wh-1: expected suppressed")
		}
	}

	for i := 0; i < 5; i++ {
		fired, _ := ce.ShouldFire("wh-2", "shared-key", 5, 300, 0)
		if i < 4 && fired {
			t.Fatalf("wh-2 event %d: expected suppressed", i+1)
		}
	}

	// wh-2 should fire on the 5th event. wh-1 is independent.
	// We already sent 5 events for wh-2, let me check the last one.
	// Actually we sent 5 in the loop, the last one fires.
	// Let me verify explicitly.
	fired, count := ce.ShouldFire("wh-2", "shared-key", 5, 300, 0)
	if fired {
		t.Fatal("wh-2: window should have been reset after 5th event fire")
	}
	_ = count

	if s := ce.Size(); s != 2 {
		t.Errorf("expected 2 entries (both webhook IDs), got %d", s)
	}
}

func TestCorrelationEngine_NilEngineAlwaysFires(t *testing.T) {
	var ce *CorrelationEngine

	fired, count := ce.ShouldFire("wh-1", "key-a", 5, 300, 60)
	if !fired {
		t.Fatal("nil engine should always fire (backward compat)")
	}
	if count != 1 {
		t.Errorf("nil engine: expected correlated_count=1, got %d", count)
	}
}

func TestCorrelationEngine_ExpireCleansUp(t *testing.T) {
	ce := NewCorrelationEngine(100)

	// Add some entries.
	for i := 0; i < 3; i++ {
		ce.ShouldFire("wh-1", "key-"+string(rune('a'+i)), 5, 300, 0)
	}
	if s := ce.Size(); s != 3 {
		t.Fatalf("expected 3 entries, got %d", s)
	}

	// Expire with immediate cutoff clears all.
	time.Sleep(10 * time.Millisecond)
	ce.Expire(0)

	if s := ce.Size(); s != 0 {
		t.Errorf("expected 0 entries after expiry, got %d", s)
	}
}

func TestCorrelationEngine_ExpireNilSafe(t *testing.T) {
	var ce *CorrelationEngine
	// Should not panic.
	ce.Expire(0)
}

func TestCorrelationEngine_SizeNilSafe(t *testing.T) {
	var ce *CorrelationEngine
	if s := ce.Size(); s != 0 {
		t.Errorf("nil engine size should be 0, got %d", s)
	}
}
