package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// ── eventToV2 conversion tests ──────────────────────────────────────────

func TestEventToV2_BasicFields(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 30, 0, 0, time.UTC)
	e := Event{
		RequestID: "req-v2-1",
		OrgID:     "org-1",
		Provider:  "anthropic",
		Model:     "claude-sonnet-4-6",
		Action:    "BLOCK",
		EventType: "request_blocked",
		Endpoint:  "/v1/messages",
		Timestamp: now,
		InputRisk: scanner.RiskScore{Score: 0.7, Level: "high"},
		Findings: []scanner.Finding{
			{Type: "pii", Category: "credit_card", Severity: "high", Match: "4111-..."},
		},
		ScanLatencyMs:  5.2,
		TotalLatencyMs: 120.0,
		CostUSD:        0.003,
		CacheStatus:    "miss",
		TraceContext:   map[string]string{"traceparent": "00-abc123-def456-01"},
	}

	ev := eventToV2(e)

	if ev.ID == "" {
		t.Fatal("expected non-empty ID (UUID)")
	}
	if ev.Type != EventBlockTriggered {
		t.Fatalf("want EventBlockTriggered, got %s", ev.Type)
	}
	if ev.RequestID != "req-v2-1" {
		t.Fatalf("RequestID: %q", ev.RequestID)
	}
	if ev.OrgID != "org-1" {
		t.Fatalf("OrgID: %q", ev.OrgID)
	}
	if !ev.Timestamp.Equal(now) {
		t.Fatalf("Timestamp: %v", ev.Timestamp)
	}
	if ev.Metadata.Source != "tamga-proxy" {
		t.Fatalf("Source: %q", ev.Metadata.Source)
	}
	if ev.Metadata.Version != "2.0" {
		t.Fatalf("Version: %q", ev.Metadata.Version)
	}
	if ev.Metadata.TraceID != "00-abc123-def456-01" {
		t.Fatalf("TraceID: %q", ev.Metadata.TraceID)
	}
	// Payload checks
	if ev.Payload["provider"] != "anthropic" {
		t.Fatalf("payload.provider: %v", ev.Payload["provider"])
	}
	if ev.Payload["action"] != "BLOCK" {
		t.Fatalf("payload.action: %v", ev.Payload["action"])
	}
	if ev.Payload["findings_count"].(int) != 1 {
		t.Fatalf("payload.findings_count: %v", ev.Payload["findings_count"])
	}
	if _, ok := ev.Payload["findings"].(json.RawMessage); !ok {
		t.Fatal("payload.findings should be json.RawMessage")
	}
}

func TestEventToV2_NilTraceContext(t *testing.T) {
	e := Event{
		RequestID:    "req-no-trace",
		EventType:    "request_scanned",
		Action:       "PASS",
		Timestamp:    time.Now(),
		TraceContext: nil,
	}
	ev := eventToV2(e)
	if ev.Metadata.TraceID != "" {
		t.Fatalf("TraceID should be empty with nil TraceContext, got %q", ev.Metadata.TraceID)
	}
}

func TestEventToV2_EmptyTraceContext(t *testing.T) {
	e := Event{
		RequestID:    "req-empty-trace",
		EventType:    "request_scanned",
		Action:       "PASS",
		Timestamp:    time.Now(),
		TraceContext: map[string]string{},
	}
	ev := eventToV2(e)
	if ev.Metadata.TraceID != "" {
		t.Fatalf("TraceID should be empty, got %q", ev.Metadata.TraceID)
	}
}

func TestEventToV2_IncludesOutputFindings(t *testing.T) {
	e := Event{
		RequestID: "req-out",
		EventType: "output_scanned",
		Action:    "REDACT",
		Timestamp: time.Now(),
		OutputFindings: []scanner.Finding{
			{Type: "secret", Category: "api_key", Severity: "critical", Match: "sk-..."},
		},
	}
	ev := eventToV2(e)
	if _, ok := ev.Payload["output_findings"].(json.RawMessage); !ok {
		t.Fatal("payload.output_findings should be json.RawMessage")
	}
	if ev.Type != EventOutputScanned {
		t.Fatalf("want EventOutputScanned, got %s", ev.Type)
	}
}

func TestEventToV2_EventTypes(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		action    string
		wantType  EventType
	}{
		{"scan completed default", "request_scanned", "PASS", EventScanCompleted},
		{"scan completed redact", "request_scanned", "REDACT", EventRedactApplied},
		{"request blocked", "request_blocked", "BLOCK", EventBlockTriggered},
		{"output scanned", "output_scanned", "PASS", EventOutputScanned},
		{"output scanned redact", "output_scanned", "REDACT", EventOutputScanned},
		{"unknown event type", "some_event", "PASS", EventScanCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := Event{
				RequestID: "type-test",
				EventType: tt.eventType,
				Action:    tt.action,
				Timestamp: time.Now(),
			}
			ev := eventToV2(e)
			if ev.Type != tt.wantType {
				t.Fatalf("want %s, got %s", tt.wantType, ev.Type)
			}
		})
	}
}

// ── subjectForEvent tests ─────────────────────────────────────────────

func TestSubjectForEvent(t *testing.T) {
	tests := []struct {
		eventType   string
		action      string
		wantSubject string
	}{
		{"request_scanned", "PASS", "scan.completed"},
		{"request_scanned", "REDACT", "redact.applied"},
		{"request_blocked", "BLOCK", "block.triggered"},
		{"request_blocked", "PASS", "block.triggered"},
		{"output_scanned", "PASS", "output.scanned"},
		{"output_scanned", "REDACT", "output.scanned"},
		{"unknown_type", "PASS", "scan.completed"},
		{"", "", "scan.completed"},
	}
	for _, tt := range tests {
		t.Run(tt.wantSubject+"_"+tt.eventType+"_"+tt.action, func(t *testing.T) {
			got := subjectForEvent(tt.eventType, tt.action)
			if got != tt.wantSubject {
				t.Fatalf("subjectForEvent(%q, %q) = %q, want %q", tt.eventType, tt.action, got, tt.wantSubject)
			}
		})
	}
}

// ── subjectToEventType tests ──────────────────────────────────────────

func TestSubjectToEventType(t *testing.T) {
	tests := []struct {
		eventType string
		action    string
		wantType  EventType
	}{
		{"request_scanned", "PASS", EventScanCompleted},
		{"request_scanned", "REDACT", EventRedactApplied},
		{"request_blocked", "BLOCK", EventBlockTriggered},
		{"request_blocked", "PASS", EventBlockTriggered},
		{"output_scanned", "PASS", EventOutputScanned},
		{"output_scanned", "REDACT", EventOutputScanned},
		{"", "", EventScanCompleted},
	}
	for _, tt := range tests {
		t.Run(string(tt.wantType), func(t *testing.T) {
			got := subjectToEventType(tt.eventType, tt.action)
			if got != tt.wantType {
				t.Fatalf("subjectToEventType(%q, %q) = %s, want %s", tt.eventType, tt.action, got, tt.wantType)
			}
		})
	}
}

// ── EventV2 JSON roundtrip tests ──────────────────────────────────────

func TestEventV2_MarshalRoundtrip(t *testing.T) {
	now := time.Date(2026, 6, 14, 15, 0, 0, 0, time.UTC)
	ev := EventV2{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Type:      EventScanCompleted,
		Timestamp: now,
		RequestID: "req-json-1",
		OrgID:     "org-json",
		Payload: map[string]any{
			"provider":        "openai",
			"model":           "gpt-4o",
			"action":          "PASS",
			"scan_latency_ms": 3.5,
		},
		Metadata: EventMetadata{
			Source:  "tamga-proxy",
			Version: "2.0",
			TraceID: "00-trace-me-01",
		},
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

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
	if decoded.OrgID != ev.OrgID {
		t.Fatalf("OrgID: %q != %q", decoded.OrgID, ev.OrgID)
	}
	if decoded.Metadata.Source != ev.Metadata.Source {
		t.Fatalf("Source: %q != %q", decoded.Metadata.Source, ev.Metadata.Source)
	}
	if decoded.Metadata.Version != ev.Metadata.Version {
		t.Fatalf("Version: %q != %q", decoded.Metadata.Version, ev.Metadata.Version)
	}
	if decoded.Metadata.TraceID != ev.Metadata.TraceID {
		t.Fatalf("TraceID: %q != %q", decoded.Metadata.TraceID, ev.Metadata.TraceID)
	}
	if !decoded.Timestamp.Equal(ev.Timestamp) {
		t.Fatalf("Timestamp mismatch: %v != %v", decoded.Timestamp, ev.Timestamp)
	}
	// Payload key check
	if decoded.Payload["provider"] != "openai" {
		t.Fatalf("payload.provider: %v", decoded.Payload["provider"])
	}
}

func TestEventV2_EmptyPayload(t *testing.T) {
	ev := EventV2{
		ID:        "empty-payload-id",
		Type:      EventScanCompleted,
		Timestamp: time.Now(),
		RequestID: "empty-req",
		Payload:   nil,
		Metadata:  EventMetadata{Source: "tamga-proxy", Version: "2.0"},
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded EventV2
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RequestID != "empty-req" {
		t.Fatalf("RequestID: %q", decoded.RequestID)
	}
}

func TestEventV2_AllEventTypes(t *testing.T) {
	types := []EventType{
		EventScanCompleted,
		EventBlockTriggered,
		EventRedactApplied,
		EventPolicyViolation,
		EventOutputScanned,
	}
	for _, typ := range types {
		t.Run(string(typ), func(t *testing.T) {
			ev := EventV2{
				ID:        "type-test-id",
				Type:      typ,
				Timestamp: time.Now(),
				RequestID: "type-req",
				Payload:   map[string]any{},
				Metadata:  EventMetadata{Source: "test", Version: "2.0"},
			}
			data, err := json.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal %s: %v", typ, err)
			}
			var decoded EventV2
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal %s: %v", typ, err)
			}
			if decoded.Type != typ {
				t.Fatalf("Type roundtrip: %s != %s", decoded.Type, typ)
			}
		})
	}
}
