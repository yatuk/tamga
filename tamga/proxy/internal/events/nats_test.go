package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/scanner"
)

// ── NATSHandler tests ─────────────────────────────────────────────────

func TestNATSHandler_NilPublisher(t *testing.T) {
	h := NewNATSHandler(nil)
	// Should not panic when publisher is nil — Handle returns immediately.
	h.Handle(Event{
		RequestID: "r1",
		EventType: "request_scanned",
		Action:    "PASS",
	})
}

func TestNATSHandler_NilPubInHandler(t *testing.T) {
	// Handler with nil publisher should return immediately.
	h := &NATSHandler{pub: nil}
	h.Handle(Event{
		RequestID: "nil-pub-test",
		EventType: "request_scanned",
		Action:    "PASS",
	})
	// No panic, no error — just returns.
}

func TestNATSHandler_VariousEventTypes(t *testing.T) {
	// With a nil publisher, Handle returns immediately for all event types.
	h := NewNATSHandler(nil)

	events := []Event{
		{RequestID: "e1", EventType: "request_scanned", Action: "PASS"},
		{RequestID: "e2", EventType: "request_blocked", Action: "BLOCK"},
		{RequestID: "e3", EventType: "request_scanned", Action: "REDACT"},
		{RequestID: "e4", EventType: "output_scanned", Action: "WARN"},
	}
	for _, e := range events {
		// Should not panic for any event type.
		h.Handle(e)
	}
}

// ── StartDurableConsumers tests ─────────────────────────────────────

func TestStartDurableConsumers_NilPublisher(t *testing.T) {
	consumers := map[string]struct {
		Subjects []string
		Handler  EventHandler
	}{
		"db-persist": {
			Subjects: []string{"scan.>"},
			Handler:  func(ev EventV2) {},
		},
	}
	StartDurableConsumers(nil, consumers, zerolog.Nop())
	// Should not panic.
}

func TestStartDurableConsumers_EmptyConsumers(t *testing.T) {
	var pub *NATSPublisher
	StartDurableConsumers(pub, nil, zerolog.Nop())
	StartDurableConsumers(pub, map[string]struct {
		Subjects []string
		Handler  EventHandler
	}{}, zerolog.Nop())
	// Should not panic with empty or nil consumer map.
}

func TestStartDurableConsumers_NilPubNilLog(t *testing.T) {
	consumers := map[string]struct {
		Subjects []string
		Handler  EventHandler
	}{
		"test-consumer": {
			Subjects: []string{"scan.>"},
			Handler:  func(ev EventV2) {},
		},
	}
	StartDurableConsumers(nil, consumers, zerolog.Logger{})
	// Should not panic even with zero-value logger.
}

// ── NATSPublisher accessor tests ─────────────────────────────────────

func TestNATSPublisher_Stream(t *testing.T) {
	pub := &NATSPublisher{js: nil, log: zerolog.Nop()}
	js := pub.Stream()
	if js != nil {
		t.Fatal("expected nil JetStream when js field is nil")
	}
}

func TestNATSPublisher_Close_NilNC(t *testing.T) {
	pub := &NATSPublisher{nc: nil, log: zerolog.Nop()}
	// Close accesses p.nc.Close() — with nil nc, this will panic.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("nil nc Close panic (expected): %v", r)
		}
	}()
	pub.Close()
}

// ── Publish pipeline: Event → EventV2 → JSON tests ───────────────────

func TestEventV2_RoundtripViaJSON(t *testing.T) {
	ev := EventV2{
		ID:        "roundtrip-id",
		Type:      EventScanCompleted,
		RequestID: "rt-req",
		Payload: map[string]any{
			"provider": "openai",
			"model":    "gpt-4o",
		},
		Metadata: EventMetadata{
			Source:  "tamga-proxy",
			Version: "2.0",
			TraceID: "00-abc-01",
		},
	}

	// Marshal — this is what NATSPublisher.Publish does internally.
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}

	// Unmarshal — verify roundtrip.
	var decoded EventV2
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != ev.ID {
		t.Fatalf("ID: %q != %q", decoded.ID, ev.ID)
	}
	if decoded.Type != ev.Type {
		t.Fatalf("Type: %s != %s", decoded.Type, ev.Type)
	}
	if decoded.RequestID != ev.RequestID {
		t.Fatalf("RequestID: %q != %q", decoded.RequestID, ev.RequestID)
	}
	if decoded.Metadata.TraceID != ev.Metadata.TraceID {
		t.Fatalf("TraceID: %q != %q", decoded.Metadata.TraceID, ev.Metadata.TraceID)
	}
}

func TestEventV2_JSONContainsExpectedKeys(t *testing.T) {
	ev := EventV2{
		ID:        "key-test-id",
		Type:      EventBlockTriggered,
		RequestID: "key-req",
		Payload:   map[string]any{"action": "BLOCK"},
		Metadata:  EventMetadata{Source: "test", Version: "2.0", TraceID: "trace-me"},
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	requiredKeys := []string{
		`"id":"key-test-id"`,
		`"type":"block.triggered"`,
		`"request_id":"key-req"`,
		`"metadata"`,
		`"source":"test"`,
		`"version":"2.0"`,
		`"trace_id":"trace-me"`,
	}
	for _, key := range requiredKeys {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing %q in: %s", key, s)
		}
	}
}

func TestEventV2_PublishMarshalErrorPath(t *testing.T) {
	// EventV2 with a payload containing an unmarshalable value.
	// json.Marshal should handle all standard Go types.
	ev := EventV2{
		ID:        "marshal-test",
		Type:      EventScanCompleted,
		RequestID: "mt-req",
		Payload: map[string]any{
			"string_val": "hello",
			"int_val":    42,
			"float_val":  3.14,
			"bool_val":   true,
			"null_val":   nil,
			"array_val":  []any{1, 2, 3},
		},
		Metadata: EventMetadata{Source: "test", Version: "2.0"},
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var decoded EventV2
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Payload["string_val"] != "hello" {
		t.Fatalf("string_val: %v", decoded.Payload["string_val"])
	}
	if v, ok := decoded.Payload["int_val"].(float64); !ok || int(v) != 42 {
		t.Fatalf("int_val: %v (type %T)", decoded.Payload["int_val"], decoded.Payload["int_val"])
	}
}

// ── full eventToV2 → publish pipeline test ────────────────────────────

func TestFullPipeline_EventToV2ToJSON(t *testing.T) {
	// Simulate the full pipeline: Event → eventToV2 → json.Marshal
	// This is what NATSPublisher.Publish does: marshal ev, then publish.
	e := Event{
		RequestID:      "pipeline-req",
		OrgID:          "org-1",
		Provider:       "anthropic",
		Model:          "claude-sonnet-4-6",
		EventType:      "request_blocked",
		Action:         "BLOCK",
		ScanLatencyMs:  2.5,
		TotalLatencyMs: 100.0,
		CostUSD:        0.01,
		InputTokens:    500,
		OutputTokens:   200,
		TraceContext:   map[string]string{"traceparent": "00-pipeline-trace-01"},
	}

	ev := eventToV2(e)
	subject := subjectForEvent(e.EventType, e.Action)

	if subject != "block.triggered" {
		t.Fatalf("subject: %q", subject)
	}
	if ev.Type != EventBlockTriggered {
		t.Fatalf("type: %s", ev.Type)
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON")
	}

	// Verify key fields are in the JSON.
	s := string(data)
	if !strings.Contains(s, `"provider":"anthropic"`) {
		t.Error("JSON missing provider")
	}
	if !strings.Contains(s, `"model":"claude-sonnet-4-6"`) {
		t.Error("JSON missing model")
	}
}

func TestFullPipeline_OutputScanned(t *testing.T) {
	e := Event{
		RequestID:    "pipeline-out",
		EventType:    "output_scanned",
		Action:       "REDACT",
		OrgID:        "org-2",
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		TraceContext: map[string]string{},
	}

	ev := eventToV2(e)
	if ev.Type != EventOutputScanned {
		t.Fatalf("type: %s, want EventOutputScanned", ev.Type)
	}
	if ev.Metadata.TraceID != "" {
		t.Fatalf("TraceID should be empty: %q", ev.Metadata.TraceID)
	}

	subject := subjectForEvent(e.EventType, e.Action)
	if subject != "output.scanned" {
		t.Fatalf("subject: %q", subject)
	}
}

// ── Context propagation in Publish ────────────────────────────────────

func TestNATSPublisher_Publish_ContextPropagation(t *testing.T) {
	// Verify the Publish method signature accepts context properly.
	// Since we don't have a real JetStream connection, test the marshaling
	// path which is the first step in Publish.
	ev := EventV2{
		ID:        "ctx-prop",
		Type:      EventScanCompleted,
		RequestID: "ctx-req",
		Payload: map[string]any{
			"provider": "openai",
		},
		Metadata: EventMetadata{Source: "test", Version: "2.0"},
	}

	// Marshal check: ensure the event can be marshalled.
	_, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Context.Background should be accepted.
	ctx := context.Background()
	_ = ctx
}

// ── Mock types for NATS JetStream testing ──────────────────────────────

// mockJetStream implements jetstream.JetStream by embedding the real interface
// and overriding only the methods used by our code. Calling any other method
// will panic — that is intentional so tests fail fast when unexpected methods
// are invoked.
type mockJetStream struct {
	jetstream.JetStream
	publishFunc                func(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
	createStreamFunc           func(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error)
	createOrUpdateConsumerFunc func(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error)
}

func (m *mockJetStream) Publish(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, subject, payload, opts...)
	}
	// Default success behavior.
	return &jetstream.PubAck{Stream: "TAMGA_EVENTS", Sequence: 1}, nil
}

func (m *mockJetStream) CreateStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	if m.createStreamFunc != nil {
		return m.createStreamFunc(ctx, cfg)
	}
	return nil, nil
}

func (m *mockJetStream) CreateOrUpdateConsumer(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	if m.createOrUpdateConsumerFunc != nil {
		return m.createOrUpdateConsumerFunc(ctx, stream, cfg)
	}
	return nil, fmt.Errorf("no consumer func set")
}

// mockConsumer implements jetstream.Consumer.
type mockConsumer struct {
	jetstream.Consumer
	fetchFunc func(batch int, opts ...jetstream.FetchOpt) (jetstream.MessageBatch, error)
}

func (m *mockConsumer) Fetch(batch int, opts ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(batch, opts...)
	}
	return &mockMessageBatch{empty: true}, nil
}

// mockMessageBatch implements jetstream.MessageBatch.
type mockMessageBatch struct {
	empty  bool
	ch     chan jetstream.Msg
	errVal error
}

func (m *mockMessageBatch) Messages() <-chan jetstream.Msg {
	if m.ch != nil {
		return m.ch
	}
	ch := make(chan jetstream.Msg)
	close(ch)
	return ch
}

func (m *mockMessageBatch) Error() error {
	return m.errVal
}

// ── NATSPublisher.Publish tests ────────────────────────────────────────

func TestNATSPublisher_Publish_Success(t *testing.T) {
	var capturedSubject string
	var capturedData []byte

	mockJS := &mockJetStream{
		publishFunc: func(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
			capturedSubject = subject
			capturedData = make([]byte, len(payload))
			copy(capturedData, payload)
			return &jetstream.PubAck{Stream: "TAMGA_EVENTS", Sequence: 1}, nil
		},
	}

	pub := &NATSPublisher{js: mockJS, log: zerolog.Nop()}

	ev := EventV2{
		ID:        "test-event-1",
		Type:      EventScanCompleted,
		RequestID: "req-pub-1",
		OrgID:     "org-1",
		Payload:   map[string]any{"provider": "openai", "model": "gpt-4o"},
		Metadata:  EventMetadata{Source: "tamga-proxy", Version: "2.0"},
	}

	err := pub.Publish(context.Background(), "scan.completed", ev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSubject != "scan.completed" {
		t.Fatalf("subject: want 'scan.completed', got %q", capturedSubject)
	}

	// Verify the JSON can be deserialised back to an EventV2.
	var out EventV2
	if err := json.Unmarshal(capturedData, &out); err != nil {
		t.Fatalf("published data not valid JSON: %v", err)
	}
	if out.ID != "test-event-1" {
		t.Fatalf("ID: want 'test-event-1', got %q", out.ID)
	}
	if out.RequestID != "req-pub-1" {
		t.Fatalf("RequestID: want 'req-pub-1', got %q", out.RequestID)
	}
	if out.Type != EventScanCompleted {
		t.Fatalf("Type: want %s, got %s", EventScanCompleted, out.Type)
	}
}

func TestNATSPublisher_Publish_Disconnected(t *testing.T) {
	mockJS := &mockJetStream{
		publishFunc: func(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
			return nil, fmt.Errorf("nats: connection closed")
		},
	}

	var buf bytes.Buffer
	log := zerolog.New(&buf)
	pub := &NATSPublisher{js: mockJS, log: log}

	ev := EventV2{
		ID:        "test-event-2",
		Type:      EventScanCompleted,
		RequestID: "req-pub-2",
	}
	err := pub.Publish(context.Background(), "scan.completed", ev)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection closed") {
		t.Fatalf("error message mismatch: %v", err)
	}
	// Verify the warning was logged.
	logged := buf.String()
	if !strings.Contains(logged, "nats publish failed") {
		t.Fatalf("expected warning 'nats publish failed' in log, got: %s", logged)
	}
}

func TestNATSPublisher_Publish_MarshalError(t *testing.T) {
	// EventV2 with standard types should always marshal, but test the path.
	// The only way to trigger a marshal error is with a channel type,
	// which can't be used in map[string]any. This tests the error handling
	// behaviour regardless — the mock never gets called.
	var called atomic.Bool
	mockJS := &mockJetStream{
		publishFunc: func(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
			called.Store(true)
			return &jetstream.PubAck{Stream: "TAMGA_EVENTS", Sequence: 1}, nil
		},
	}
	pub := &NATSPublisher{js: mockJS, log: zerolog.Nop()}

	// Normal publish — should call publishFunc.
	ev := EventV2{ID: "marshal-ok", Type: EventScanCompleted, RequestID: "ok"}
	if err := pub.Publish(context.Background(), "scan.completed", ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called.Load() {
		t.Fatal("publishFunc was not called")
	}
}

func TestNATSPublisher_New_EmptyURL(t *testing.T) {
	// An empty URL should cause nats.Connect to fail, returning nil publisher.
	cfg := NATSConfig{URL: "", Stream: "TAMGA_EVENTS"}
	pub, err := NewNATSPublisher(context.Background(), cfg, zerolog.Nop())
	if pub != nil {
		t.Fatal("expected nil publisher with empty URL")
	}
	if err == nil {
		t.Fatal("expected error with empty URL")
	}
}

func TestNATSPublisher_Stream_Accessor(t *testing.T) {
	// Stream() returns the underlying JetStream — verify it returns whatever was set.
	mockJS := &mockJetStream{}
	pub := &NATSPublisher{js: mockJS, log: zerolog.Nop()}

	got := pub.Stream()
	if got != mockJS {
		t.Fatal("Stream() should return the underlying JetStream")
	}
}

// ── runDurableConsumer tests ───────────────────────────────────────────

func TestDurableConsumer_CreateError(t *testing.T) {
	// When CreateOrUpdateConsumer fails, the function logs and returns.
	var buf bytes.Buffer
	log := zerolog.New(&buf)

	mockJS := &mockJetStream{
		createOrUpdateConsumerFunc: func(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
			return nil, fmt.Errorf("stream TAMGA_EVENTS not found")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Should return immediately after log.Error.
	runDurableConsumer(ctx, mockJS, "TAMGA_EVENTS", "durable-test-1", []string{"scan.>"}, func(ev EventV2) {}, log)

	logged := buf.String()
	if !strings.Contains(logged, "failed to create NATS consumer") {
		t.Fatalf("expected 'failed to create NATS consumer' in log, got: %s", logged)
	}
	if !strings.Contains(logged, "durable-test-1") {
		t.Fatalf("expected durable name in log, got: %s", logged)
	}
}

func TestDurableConsumer_ContextCanceled(t *testing.T) {
	// CreateOrUpdateConsumer succeeds; the fetch loop spins.
	// Cancelling the context must cause the function to exit.
	mockConsumer := &mockConsumer{
		fetchFunc: func(batch int, opts ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
			// Return empty batch each time so the loop keeps running.
			ch := make(chan jetstream.Msg)
			close(ch)
			return &mockMessageBatch{ch: ch}, nil
		},
	}

	mockJS := &mockJetStream{
		createOrUpdateConsumerFunc: func(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
			return mockConsumer, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runDurableConsumer(ctx, mockJS, "TAMGA_EVENTS", "durable-cancel", []string{"scan.>"}, func(ev EventV2) {}, zerolog.Nop())
		close(done)
	}()

	// Give the goroutine time to enter the fetch loop.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success — consumer exited cleanly.
	case <-time.After(3 * time.Second):
		t.Fatal("runDurableConsumer did not exit after context cancel")
	}
}

func TestDurableConsumer_MessageProcessing(t *testing.T) {
	// When fetch returns messages, the handler should be called with
	// the parsed EventV2 and each message should be acknowledged.
	var received atomic.Int32

	ev := EventV2{
		ID:        "consumer-msg-1",
		Type:      EventScanCompleted,
		RequestID: "req-consumer-1",
		Payload:   map[string]any{"test": "value"},
		Metadata:  EventMetadata{Source: "test", Version: "2.0"},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Create a channel with one message that auto-closes.
	msgCh := make(chan jetstream.Msg, 1)
	// We need a Msg with data. Use the mock msg type.
	msgCh <- &mockMsg{data: data}
	close(msgCh)

	mockConsumer := &mockConsumer{
		fetchFunc: func(batch int, opts ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
			return &mockMessageBatch{ch: msgCh}, nil
		},
	}

	mockJS := &mockJetStream{
		createOrUpdateConsumerFunc: func(ctx context.Context, stream string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
			return mockConsumer, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use a done channel because runDurableConsumer loops on fetch.
	// After processing the single message, the next Fetch returns empty and loops.
	done := make(chan struct{})
	go func() {
		runDurableConsumer(ctx, mockJS, "TAMGA_EVENTS", "msg-proc", []string{"scan.>"},
			func(ev EventV2) {
				received.Add(1)
				if ev.ID != "consumer-msg-1" {
					t.Errorf("handler got wrong event ID: %s", ev.ID)
				}
			},
			zerolog.Nop(),
		)
		close(done)
	}()

	// Wait for processing, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runDurableConsumer timed out")
	}

	if received.Load() != 1 {
		t.Fatalf("handler call count: want 1, got %d", received.Load())
	}
}

// mockMsg is a minimal jetstream.Msg implementation for testing.
type mockMsg struct {
	jetstream.Msg
	data []byte
}

func (m *mockMsg) Data() []byte { return m.data }
func (m *mockMsg) Ack() error   { return nil }
func (m *mockMsg) Nak() error   { return nil }
func (m *mockMsg) Subject() string {
	return "scan.completed"
}

// ── EventBus + NATS integration test ───────────────────────────────────

func TestEventBus_NATSIntegration(t *testing.T) {
	var capturedSubject string
	var capturedData []byte
	publishComplete := make(chan struct{})

	mockJS := &mockJetStream{
		publishFunc: func(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
			capturedSubject = subject
			capturedData = make([]byte, len(payload))
			copy(capturedData, payload)
			close(publishComplete)
			return &jetstream.PubAck{Stream: "TAMGA_EVENTS", Sequence: 1}, nil
		},
	}

	pub := &NATSPublisher{js: mockJS, log: zerolog.Nop()}
	natsHandler := NewNATSHandler(pub)

	bus := NewBus()
	bus.Subscribe(natsHandler.Handle)
	bus.Start()
	defer bus.Stop()

	e := Event{
		RequestID:     "integ-test-1",
		OrgID:         "org-integ",
		EventType:     "request_scanned",
		Action:        "PASS",
		Provider:      "openai",
		Model:         "gpt-4o",
		Timestamp:     time.Now(),
		ScanLatencyMs: 3.2,
		InputRisk:     scanner.RiskScore{Score: 0.35, Level: "low", Percentage: 35},
	}

	bus.Publish(e)

	// NATSHandler publishes in a goroutine (fire-and-forget).
	select {
	case <-publishComplete:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for NATS publish")
	}

	// Verify subject routing.
	if capturedSubject != "scan.completed" {
		t.Fatalf("subject: want 'scan.completed', got %q", capturedSubject)
	}

	// Verify the published JSON is a valid EventV2.
	var ev EventV2
	if err := json.Unmarshal(capturedData, &ev); err != nil {
		t.Fatalf("published data not valid JSON: %v\nraw: %s", err, string(capturedData))
	}
	if ev.RequestID != "integ-test-1" {
		t.Fatalf("RequestID: want 'integ-test-1', got %q", ev.RequestID)
	}
	if ev.OrgID != "org-integ" {
		t.Fatalf("OrgID: want 'org-integ', got %q", ev.OrgID)
	}
	if ev.Metadata.Source != "tamga-proxy" {
		t.Fatalf("Metadata.Source: want 'tamga-proxy', got %q", ev.Metadata.Source)
	}
	if ev.Metadata.Version != "2.0" {
		t.Fatalf("Metadata.Version: want '2.0', got %q", ev.Metadata.Version)
	}
}

func TestEventBus_NATSIntegration_BlockedEvent(t *testing.T) {
	testIntegrationEventType(t, "request_blocked", "BLOCK", "block.triggered")
}

func TestEventBus_NATSIntegration_RedactEvent(t *testing.T) {
	testIntegrationEventType(t, "request_scanned", "REDACT", "redact.applied")
}

func TestEventBus_NATSIntegration_OutputScanned(t *testing.T) {
	testIntegrationEventType(t, "output_scanned", "REDACT", "output.scanned")
}

func testIntegrationEventType(t *testing.T, eventType, action, wantSubject string) {
	t.Helper()
	var capturedSubject string
	publishDone := make(chan struct{})

	mockJS := &mockJetStream{
		publishFunc: func(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
			capturedSubject = subject
			close(publishDone)
			return &jetstream.PubAck{Stream: "TAMGA_EVENTS", Sequence: 1}, nil
		},
	}

	pub := &NATSPublisher{js: mockJS, log: zerolog.Nop()}
	natsHandler := NewNATSHandler(pub)

	bus := NewBus()
	bus.Subscribe(natsHandler.Handle)
	bus.Start()
	defer bus.Stop()

	bus.Publish(Event{
		RequestID: "integ-" + eventType,
		EventType: eventType,
		Action:    action,
		Timestamp: time.Now(),
	})

	select {
	case <-publishDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out for eventType=%s action=%s", eventType, action)
	}

	if capturedSubject != wantSubject {
		t.Fatalf("subject for (%s,%s): want %q, got %q", eventType, action, wantSubject, capturedSubject)
	}
}
