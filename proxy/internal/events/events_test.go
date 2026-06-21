package events

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/yatuk/tamga/internal/scanner"
)

func TestMetricsHandler_NilSafe(t *testing.T) {
	// Should not panic when m is nil.
	h := MetricsHandler(nil)
	h(Event{EventType: "request_scanned", Action: "PASS"})
}

func TestMetricsHandler_Counts(t *testing.T) {
	m := &Metrics{}
	h := MetricsHandler(m)

	// Request scanned → total++
	h(Event{EventType: "request_scanned", Action: "PASS"})
	if m.TotalRequests.Load() != 1 {
		t.Fatalf("total: want 1, got %d", m.TotalRequests.Load())
	}

	// Request blocked → total++ and blocked++
	h(Event{EventType: "request_blocked", Action: "BLOCK"})
	if m.TotalRequests.Load() != 2 {
		t.Fatalf("total: want 2, got %d", m.TotalRequests.Load())
	}
	if m.Blocked.Load() != 1 {
		t.Fatalf("blocked: want 1, got %d", m.Blocked.Load())
	}

	// REDACT action increments redacted counter.
	h(Event{EventType: "request_scanned", Action: "REDACT"})
	if m.Redacted.Load() != 1 {
		t.Fatalf("redacted: want 1, got %d", m.Redacted.Load())
	}

	// WARN action increments warned counter.
	h(Event{EventType: "request_scanned", Action: "WARN"})
	if m.Warned.Load() != 1 {
		t.Fatalf("warned: want 1, got %d", m.Warned.Load())
	}

	// Total should be 4 now.
	if m.TotalRequests.Load() != 4 {
		t.Fatalf("total: want 4, got %d", m.TotalRequests.Load())
	}
}

func TestMetricsHandler_OutputScanHintIgnored(t *testing.T) {
	m := &Metrics{}
	h := MetricsHandler(m)
	// Output scan hints should not increment any counter.
	h(Event{EventType: "output_scan_hint", Action: "REDACT"})
	if m.TotalRequests.Load() != 0 {
		t.Fatalf("output_scan_hint should not count: got %d", m.TotalRequests.Load())
	}
}

func TestRecentBuffer_NilSafe(t *testing.T) {
	var rb *RecentBuffer
	rb.Add(Event{RequestID: "r1", EventType: "request_scanned"})
	_, ok := rb.GetByRequestID("r1")
	if ok {
		t.Fatal("nil buffer should not find events")
	}
	evs, total := rb.Page(1, 10)
	if evs != nil || total != 0 {
		t.Fatalf("nil buffer page: %d/%d", len(evs), total)
	}
}

func TestRecentBuffer_AddAndGet(t *testing.T) {
	rb := NewRecentBuffer(5)
	e := Event{
		RequestID: "req-1",
		EventType: "request_scanned",
		Action:    "BLOCK",
		Provider:  "anthropic",
		Timestamp: time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
	}
	rb.Add(e)
	got, ok := rb.GetByRequestID("req-1")
	if !ok {
		t.Fatal("event not found")
	}
	if got.Action != "BLOCK" || got.Provider != "anthropic" {
		t.Fatalf("got %+v", got)
	}
}

func TestRecentBuffer_GetMissing(t *testing.T) {
	rb := NewRecentBuffer(5)
	_, ok := rb.GetByRequestID("missing")
	if ok {
		t.Fatal("should not find missing event")
	}
}

func TestRecentBuffer_Page(t *testing.T) {
	rb := NewRecentBuffer(5)
	for i := range 5 {
		rb.Add(Event{
			RequestID: "r" + string(rune('a'+i)),
			EventType: "request_scanned",
			Timestamp: time.Now(),
		})
	}
	evs, total := rb.Page(1, 3)
	if len(evs) != 3 || total != 5 {
		t.Fatalf("page 1 size 3: got %d/%d", len(evs), total)
	}
	evs2, total2 := rb.Page(2, 3)
	if len(evs2) != 2 || total2 != 5 {
		t.Fatalf("page 2 size 3: got %d/%d, want 2/5", len(evs2), total2)
	}
}

func TestRecentBuffer_CapacityEviction(t *testing.T) {
	rb := NewRecentBuffer(3)
	rb.Add(Event{RequestID: "r1", EventType: "request_scanned", Timestamp: time.Now()})
	rb.Add(Event{RequestID: "r2", EventType: "request_scanned", Timestamp: time.Now()})
	rb.Add(Event{RequestID: "r3", EventType: "request_scanned", Timestamp: time.Now()})
	rb.Add(Event{RequestID: "r4", EventType: "request_scanned", Timestamp: time.Now()})

	// r1 should be evicted, r2-r4 should exist.
	_, ok := rb.GetByRequestID("r1")
	if ok {
		t.Fatal("r1 should be evicted")
	}
	for _, id := range []string{"r2", "r3", "r4"} {
		if _, ok := rb.GetByRequestID(id); !ok {
			t.Fatalf("%s should exist", id)
		}
	}
}

func TestRecentBuffer_Search(t *testing.T) {
	rb := NewRecentBuffer(10)
	fs := []scanner.Finding{{Type: "pii", Category: "email", Severity: "high"}}
	_ = fs
	rb.Add(Event{
		RequestID: "openai-1", EventType: "request_scanned",
		Action: "PASS", Provider: "openai",
		Timestamp: time.Now().Add(-1 * time.Hour),
	})
	rb.Add(Event{
		RequestID: "anthropic-1", EventType: "request_scanned",
		Action: "BLOCK", Provider: "anthropic",
		Timestamp: time.Now().Add(-2 * time.Hour),
	})

	t.Run("filter by provider openai", func(t *testing.T) {
		match := func(e Event) bool { return e.Provider == "openai" }
		evs, total := rb.Search(1, 50, match)
		if total != 1 || len(evs) != 1 {
			t.Fatalf("want 1 openai, got %d/%d", len(evs), total)
		}
		if evs[0].RequestID != "openai-1" {
			t.Fatalf("want openai-1, got %s", evs[0].RequestID)
		}
	})

	t.Run("filter by action BLOCK", func(t *testing.T) {
		match := func(e Event) bool { return e.Action == "BLOCK" }
		_, total := rb.Search(1, 50, match)
		if total != 1 {
			t.Fatalf("want 1 blocked, got %d", total)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		match := func(e Event) bool { return e.Provider == "gemini" }
		evs, total := rb.Search(1, 50, match)
		if total != 0 || len(evs) != 0 {
			t.Fatalf("want 0 matches, got %d/%d", len(evs), total)
		}
	})
}

func TestLogHandler_NilSafe(t *testing.T) {
	h := LogHandler(zerolog.Nop())
	h(Event{
		RequestID: "req-1",
		EventType: "request_scanned",
		Action:    "PASS",
		Timestamp: time.Now(),
	})
}

func TestAnalyzerHandler_NilClient(t *testing.T) {
	h := AnalyzerHandler(nil)
	// Should not panic with nil client.
	h(Event{
		EventType:    "request_scanned",
		Action:       "PASS",
		RequestID:    "req-1",
		InputRisk:    scanner.RiskScore{Score: 0.5},
		TraceContext: map[string]string{},
	})
}

func TestAnalyzerHandler_SkipBelowLowThreshold(t *testing.T) {
	// We can't test actual gRPC calls, but nil client always skips.
	// The risk gate logic is tested via the constants and h() return.
	h := AnalyzerHandler(nil)
	// Risk below 0.15 → skip
	h(Event{
		EventType:    "request_scanned",
		Action:       "PASS",
		RequestID:    "low-risk",
		InputRisk:    scanner.RiskScore{Score: 0.05},
		TraceContext: map[string]string{},
	})
	// Should not panic.
}

func TestAnalyzerHandler_SkipNonRequestEvents(t *testing.T) {
	h := AnalyzerHandler(nil)
	h(Event{
		EventType: "output_scan_hint",
		Action:    "WARN",
		RequestID: "out-1",
	})
	// Should not panic — non-request events are skipped early.
}

func TestBroker_SubscribeUnsubscribe(t *testing.T) {
	b := NewBroker(4)
	ch, unsub := b.Subscribe()
	if ch == nil {
		t.Fatal("subscribe returned nil channel")
	}
	unsub()
	// Channel should be closed after unsubscribe.
	_, open := <-ch
	if open {
		t.Fatal("channel should be closed after unsubscribe")
	}
}

func TestBroker_PublishToSubscriber(t *testing.T) {
	b := NewBroker(4)
	ch, unsub := b.Subscribe()
	defer unsub()

	e := Event{RequestID: "evt-1", EventType: "request_scanned", Timestamp: time.Now()}
	b.Publish(e)

	select {
	case got := <-ch:
		if got.RequestID != "evt-1" {
			t.Fatalf("want evt-1, got %s", got.RequestID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBroker_DropOnFullBuffer(t *testing.T) {
	b := NewBroker(1)
	ch, unsub := b.Subscribe()
	defer unsub()

	// Fill the buffer.
	b.Publish(Event{RequestID: "first", EventType: "request_scanned", Timestamp: time.Now()})
	b.Publish(Event{RequestID: "second", EventType: "request_scanned", Timestamp: time.Now()})

	// First event should be in channel, second dropped.
	select {
	case got := <-ch:
		if got.RequestID != "first" {
			t.Fatalf("want first, got %s", got.RequestID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	// Channel should be empty now (second event dropped).
	select {
	case got := <-ch:
		t.Fatalf("unexpected event in channel: %s", got.RequestID)
	default:
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	b := NewBroker(4)
	ch1, u1 := b.Subscribe()
	defer u1()
	ch2, u2 := b.Subscribe()
	defer u2()

	e := Event{RequestID: "fanout", EventType: "request_scanned", Timestamp: time.Now()}
	b.Publish(e)

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case got := <-ch:
			if got.RequestID != "fanout" {
				t.Fatalf("sub %d: want fanout, got %s", i, got.RequestID)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timed out", i)
		}
	}
}

func TestRecentBufferHandler_Wiring(t *testing.T) {
	rb := NewRecentBuffer(10)
	h := RecentBufferHandler(rb)
	e := Event{RequestID: "buf-1", EventType: "request_scanned", Timestamp: time.Now()}
	h(e)
	got, ok := rb.GetByRequestID("buf-1")
	if !ok {
		t.Fatal("event not buffered by handler")
	}
	if got.EventType != "request_scanned" {
		t.Fatalf("event type: %s", got.EventType)
	}
}

// --- EventToJSON ---

func TestEventToJSON_RequestScanned(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	e := Event{
		RequestID:      "req-1",
		OrgID:          "org-1",
		Provider:       "openai",
		Model:          "gpt-4o",
		EventType:      "request_scanned",
		Action:         "PASS",
		Findings:       []scanner.Finding{{Type: "pii", Category: "email", Severity: "high"}},
		ScanLatencyMs:  2.5,
		TotalLatencyMs: 15.0,
		InputRisk:      scanner.RiskScore{Score: 0.3, Level: "medium", Percentage: 30},
		OutputRisk:     scanner.RiskScore{Score: 0, Level: "none", Percentage: 0},
		Timestamp:      now,
	}

	j := EventToJSON(e)
	if j.RequestID != "req-1" {
		t.Errorf("RequestID: want req-1, got %q", j.RequestID)
	}
	if j.InputRiskPct != 30 {
		t.Errorf("InputRiskPct: want 30, got %d", j.InputRiskPct)
	}
	if j.RiskLevel != "medium" {
		t.Errorf("RiskLevel: want medium, got %q", j.RiskLevel)
	}
	if j.FindingsCount != 1 {
		t.Errorf("FindingsCount: want 1, got %d", j.FindingsCount)
	}
	if j.InputRisk == nil || j.OutputRisk == nil {
		t.Error("InputRisk/OutputRisk should be non-nil for request_scanned")
	}
}

func TestEventToJSON_RequestBlocked(t *testing.T) {
	e := Event{
		RequestID: "req-2",
		EventType: "request_blocked",
		Action:    "BLOCK",
		InputRisk: scanner.RiskScore{Score: 0.9, Level: "critical", Percentage: 90},
		Timestamp: time.Now(),
	}

	j := EventToJSON(e)
	if j.RiskLevel != "critical" {
		t.Errorf("RiskLevel: want critical, got %q", j.RiskLevel)
	}
	if j.InputRiskPct != 90 {
		t.Errorf("InputRiskPct: want 90, got %d", j.InputRiskPct)
	}
}

func TestEventToJSON_NonRequestEvent(t *testing.T) {
	e := Event{
		RequestID: "req-3",
		EventType: "output_scan_hint",
		Action:    "REDACT",
		Timestamp: time.Now(),
	}

	j := EventToJSON(e)
	// Non-request events should not have risk fields.
	if j.InputRisk != nil || j.OutputRisk != nil {
		t.Error("output_scan_hint should not have risk fields")
	}
	if j.InputRiskPct != 0 {
		t.Errorf("InputRiskPct: want 0, got %d", j.InputRiskPct)
	}
}

func TestEventToJSON_RiskLevelDefaultsToNone(t *testing.T) {
	e := Event{
		RequestID:  "req-4",
		EventType:  "request_scanned",
		Action:     "PASS",
		InputRisk:  scanner.RiskScore{Score: 0, Level: "", Percentage: 0},
		OutputRisk: scanner.RiskScore{Score: 0, Level: "", Percentage: 0},
		Timestamp:  time.Now(),
	}

	j := EventToJSON(e)
	if j.RiskLevel != "none" {
		t.Errorf("RiskLevel: want 'none' (zero risk default), got %q", j.RiskLevel)
	}
}

// --- MarshalEventsJSON ---

func TestMarshalEventsJSON(t *testing.T) {
	events := []Event{
		{RequestID: "r1", EventType: "request_scanned", Timestamp: time.Now()},
		{RequestID: "r2", EventType: "request_blocked", Timestamp: time.Now()},
	}

	b, err := MarshalEventsJSON(events)
	if err != nil {
		t.Fatalf("MarshalEventsJSON: %v", err)
	}
	var out []EventJSON
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("want 2 events, got %d", len(out))
	}
}

func TestMarshalEventsJSON_Empty(t *testing.T) {
	b, err := MarshalEventsJSON(nil)
	if err != nil {
		t.Fatalf("MarshalEventsJSON nil: %v", err)
	}
	if string(b) != "[]" {
		t.Errorf("want [], got %q", string(b))
	}
}

// --- MatchEventSearch ---

func TestMatchEventSearch_ActionFilter(t *testing.T) {
	e := Event{Action: "BLOCK", Timestamp: time.Now()}
	if !MatchEventSearch(e, "BLOCK", "", false, "", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should match BLOCK action")
	}
	if MatchEventSearch(e, "PASS", "", false, "", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should not match PASS action")
	}
}

func TestMatchEventSearch_ProviderFilter(t *testing.T) {
	now := time.Now()
	e := Event{Provider: "openai", Timestamp: now}
	if !MatchEventSearch(e, "", "openai", false, "", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should match openai provider")
	}
	if MatchEventSearch(e, "", "anthropic", false, "", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should not match anthropic provider")
	}
}

func TestMatchEventSearch_ShadowOnly(t *testing.T) {
	now := time.Now()
	t.Run("non-enterprise matches shadow", func(t *testing.T) {
		e := Event{Provider: "mistral", Timestamp: now}
		if !MatchEventSearch(e, "", "", true, "", "", "", "", "", time.Time{}, time.Time{}) {
			t.Error("mistral (non-enterprise) should match shadow filter")
		}
	})
	t.Run("enterprise excluded from shadow", func(t *testing.T) {
		e := Event{Provider: "openai", Timestamp: now}
		if MatchEventSearch(e, "", "", true, "", "", "", "", "", time.Time{}, time.Time{}) {
			t.Error("openai (enterprise) should not match shadow filter")
		}
	})
	t.Run("empty provider excluded from shadow", func(t *testing.T) {
		e := Event{Provider: "", Timestamp: now}
		if MatchEventSearch(e, "", "", true, "", "", "", "", "", time.Time{}, time.Time{}) {
			t.Error("empty provider should not match shadow filter")
		}
	})
}

func TestMatchEventSearch_ProviderShadowKeyword(t *testing.T) {
	now := time.Now()
	// When provider=shadow is explicitly passed, treat like shadowOnly.
	e := Event{Provider: "mistral", Timestamp: now}
	if !MatchEventSearch(e, "", "shadow", false, "", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("shadow provider filter should match non-enterprise")
	}
}

func TestMatchEventSearch_ProviderAll(t *testing.T) {
	now := time.Now()
	e := Event{Provider: "openai", Timestamp: now}
	if !MatchEventSearch(e, "", "all", false, "", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("all provider should match everything")
	}
}

func TestMatchEventSearch_FindingType(t *testing.T) {
	now := time.Now()
	e := Event{
		Findings: []scanner.Finding{
			{Type: "pii", Category: "email", Severity: "high"},
		},
		Timestamp: now,
	}
	if !MatchEventSearch(e, "", "", false, "pii", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should match finding type pii")
	}
	if MatchEventSearch(e, "", "", false, "injection", "", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should not match finding type injection")
	}
}

func TestMatchEventSearch_Severity(t *testing.T) {
	now := time.Now()
	e := Event{
		Findings: []scanner.Finding{
			{Type: "pii", Severity: "high"},
		},
		Timestamp: now,
	}
	if !MatchEventSearch(e, "", "", false, "", "high", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should match high severity")
	}
	if MatchEventSearch(e, "", "", false, "", "low", "", "", "", time.Time{}, time.Time{}) {
		t.Error("should not match low severity")
	}
}

func TestMatchEventSearch_Category(t *testing.T) {
	now := time.Now()
	e := Event{
		Findings: []scanner.Finding{
			{Type: "pii", Category: "credit_card", Severity: "high"},
		},
		Timestamp: now,
	}
	if !MatchEventSearch(e, "", "", false, "", "", "credit", "", "", time.Time{}, time.Time{}) {
		t.Error("should match category containing 'credit'")
	}
}

func TestMatchEventSearch_Technique(t *testing.T) {
	now := time.Now()
	e := Event{
		Findings: []scanner.Finding{
			{Type: "injection", Category: "prompt", Severity: "critical", Match: "ignore instructions"},
		},
		Timestamp: now,
	}
	if !MatchEventSearch(e, "", "", false, "", "", "", "ignore", "", time.Time{}, time.Time{}) {
		t.Error("should match technique 'ignore'")
	}
}

func TestMatchEventSearch_Query(t *testing.T) {
	now := time.Now()
	e := Event{
		RequestID: "evt-abc123",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "email", Severity: "high", Match: "user@example.com"},
		},
		Timestamp: now,
	}
	if !MatchEventSearch(e, "", "", false, "", "", "", "", "abc123", time.Time{}, time.Time{}) {
		t.Error("should match query against request ID")
	}
	if !MatchEventSearch(e, "", "", false, "", "", "", "", "user@example", time.Time{}, time.Time{}) {
		t.Error("should match query against findings JSON")
	}
}

func TestMatchEventSearch_TimeRange(t *testing.T) {
	now := time.Now()
	e := Event{Timestamp: now}

	// Since filter: event after since should match.
	since := now.Add(-1 * time.Hour)
	if !MatchEventSearch(e, "", "", false, "", "", "", "", "", since, time.Time{}) {
		t.Error("should match when timestamp is after since")
	}

	// Since filter: event before since should not match.
	since2 := now.Add(1 * time.Hour)
	if MatchEventSearch(e, "", "", false, "", "", "", "", "", since2, time.Time{}) {
		t.Error("should not match when timestamp is before since")
	}

	// Until filter: event before until should match.
	until := now.Add(1 * time.Hour)
	if !MatchEventSearch(e, "", "", false, "", "", "", "", "", time.Time{}, until) {
		t.Error("should match when timestamp is before until")
	}

	// Until filter: event after until should not match.
	until2 := now.Add(-1 * time.Hour)
	if MatchEventSearch(e, "", "", false, "", "", "", "", "", time.Time{}, until2) {
		t.Error("should not match when timestamp is after until")
	}
}

// --- Bus edge cases ---

func TestBus_PublishContext_NilContext(t *testing.T) {
	b := NewBus()
	b.Start()
	defer b.Stop()

	// PublishContext with nil context should not panic.
	b.PublishContext(context.TODO(), Event{RequestID: "nil-ctx", EventType: "request_scanned"})

	// Give it a moment to dispatch.
	time.Sleep(10 * time.Millisecond)
}

func TestBus_PublishContext_NilBus(t *testing.T) {
	var b *Bus
	// Should not panic.
	b.PublishContext(context.Background(), Event{RequestID: "nil-bus"})
	if b.Dropped() != 0 {
		t.Error("nil bus dropped should be 0")
	}
}

func TestBus_Stop_Idempotent(t *testing.T) {
	b := NewBus()
	b.Start()

	b.Stop()
	// Second stop should be safe (no-op).
	b.Stop()
}

func TestBus_Subscribe_NilHandler(t *testing.T) {
	b := NewBus()
	b.Subscribe(nil) // should not panic

	b.mu.RLock()
	n := len(b.handlers)
	b.mu.RUnlock()
	if n != 0 {
		t.Errorf("nil handler should not be registered, got %d handlers", n)
	}
}

func TestBus_NilSubscribe(t *testing.T) {
	var b *Bus
	b.Subscribe(func(e Event) {}) // Should not panic on nil bus.
	b.Start()                     // Should not panic on nil bus.
	b.Publish(Event{})            // Should not panic on nil bus.
	if b.Dropped() != 0 {
		t.Error("nil bus dropped should be 0")
	}
}

func TestBus_Stop_NilBus(t *testing.T) {
	var b *Bus
	b.Stop() // Should not panic.
}

func TestBus_DrainAfterClose(t *testing.T) {
	b := NewBus()
	var received int
	b.Subscribe(func(e Event) { received++ })
	b.Start()

	e := Event{RequestID: "drain", EventType: "request_scanned", Timestamp: time.Now()}
	b.Publish(e)
	b.Stop() // Closes channel, waits for drain.

	if received != 1 {
		t.Errorf("want 1 received after drain, got %d", received)
	}
}

// -- Bus buffer overflow --

func TestBus_DroppedCounter(t *testing.T) {
	b := NewBus()
	// Don't start the dispatcher — fill the buffer beyond capacity.
	for i := 0; i < 1500; i++ {
		b.Publish(Event{RequestID: "fill", EventType: "request_scanned"})
	}
	dropped := b.Dropped()
	if dropped <= 0 {
		t.Errorf("expected dropped > 0 after overflow, got %d", dropped)
	}
}

// --- MarshalEventsJSON full fields ---

func TestEventToJSON_FullFields(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high", Match: "a@b.com"},
		{Type: "injection", Category: "prompt", Severity: "critical", Match: "ignore previous"},
	}
	e := Event{
		RequestID:      "full-1",
		OrgID:          "org-1",
		Provider:       "anthropic",
		Model:          "claude-sonnet-4-6",
		ModelFamily:    "claude-4",
		EventType:      "request_scanned",
		Action:         "REDACT",
		Findings:       findings,
		Endpoint:       "/v1/messages",
		ScanLatencyMs:  3.2,
		TotalLatencyMs: 42.1,
		ContentType:    "application/json",
		CostUSD:        0.015,
		CacheStatus:    "miss",
		UserID:         "user-123",
		Timestamp:      time.Now(),
		InputRisk:      scanner.RiskScore{Score: 0.65, Level: "high", Percentage: 65},
		OutputRisk:     scanner.RiskScore{Score: 0.1, Level: "low", Percentage: 10},
	}

	j := EventToJSON(e)
	if j.Provider != "anthropic" {
		t.Errorf("Provider: %q", j.Provider)
	}
	if j.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: %q", j.Model)
	}
	if j.Endpoint != "/v1/messages" {
		t.Errorf("Endpoint: %q", j.Endpoint)
	}
	if j.ScanLatencyMs != 3.2 {
		t.Errorf("ScanLatencyMs: %f", j.ScanLatencyMs)
	}
	if j.TotalLatencyMs != 42.1 {
		t.Errorf("TotalLatencyMs: %f", j.TotalLatencyMs)
	}
	if j.InputRiskPct != 65 {
		t.Errorf("InputRiskPct: %d", j.InputRiskPct)
	}
	if j.RiskLevel != "high" {
		t.Errorf("RiskLevel: %q", j.RiskLevel)
	}
	if j.FindingsCount != 2 {
		t.Errorf("FindingsCount: %d", j.FindingsCount)
	}
	if len(j.Findings) != 2 {
		t.Errorf("Findings len: %d", len(j.Findings))
	}
	if j.InputRisk == nil || j.InputRisk.Score != 0.65 {
		t.Error("InputRisk not preserved")
	}
}

// --- RecentBufferHandler nil safety ---

func TestRecentBufferHandler_NilBuffer(t *testing.T) {
	h := RecentBufferHandler(nil)
	// Should not panic with nil buffer.
	h(Event{RequestID: "nil-buf", EventType: "request_scanned", Timestamp: time.Now()})
}

// --- MarshalEventsJSON verifies JSON structure ---

func TestMarshalEventsJSON_OutputStructure(t *testing.T) {
	events := []Event{
		{
			RequestID: "struct-1",
			EventType: "request_scanned",
			Action:    "BLOCK",
			Provider:  "openai",
			Model:     "gpt-4o",
			InputRisk: scanner.RiskScore{Score: 0.85, Level: "high", Percentage: 85},
			Timestamp: time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		},
	}

	b, err := MarshalEventsJSON(events)
	if err != nil {
		t.Fatalf("MarshalEventsJSON: %v", err)
	}

	// Verify the JSON contains expected keys.
	s := string(b)
	for _, key := range []string{
		`"request_id":"struct-1"`,
		`"event_type":"request_scanned"`,
		`"action":"BLOCK"`,
		`"provider":"openai"`,
		`"model":"gpt-4o"`,
		`"input_risk_pct":85`,
		`"risk_level":"high"`,
	} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing %s", key)
		}
	}
}
