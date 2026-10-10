package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

const evOrg = "00000000-0000-0000-0000-0000000000aa"

// captureStore keeps the rows it is asked to save.
type captureStore struct {
	*NoopStore
	rows []RequestLog
}

func (c *captureStore) SaveRequestLog(_ context.Context, rl RequestLog) error {
	c.rows = append(c.rows, rl)
	return nil
}

// overNATS is the envelope a consumer receives for e: what the bridge
// publishes, through JSON.
func overNATS(t *testing.T, e events.Event) events.EventV2 {
	t.Helper()
	b, err := json.Marshal(events.ToV2(e))
	if err != nil {
		t.Fatal(err)
	}
	var ev events.EventV2
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func fullRequestEvent() events.Event {
	outFindings := []scanner.Finding{{Type: "pii", Category: "phone_tr", Severity: "high", Match: "05** *** ** 23", Confidence: 0.5}}
	return events.Event{
		RequestID:   "req-both-ways",
		OrgID:       evOrg,
		OrgVerified: true,
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		ModelFamily: "gpt-4o",
		EventType:   "request_scanned",
		Action:      "REDACT",
		Endpoint:    "/v1/chat/completions",
		UserID:      "alice",
		Findings: []scanner.Finding{{Type: "pii", Category: "email", Severity: "high", Match: "ay****@example.com",
			StartPos: 3, EndPos: 21, Confidence: 0.9, Metadata: map[string]string{"b": "2", "a": "1"}, Role: "user", Path: "messages[0].content"}},
		ScanLatencyMs:  0.347,
		TotalLatencyMs: 812.5,
		InputTokens:    1200,
		OutputTokens:   300,
		CostUSD:        0.00036,
		CostKnown:      true,
		CacheStatus:    "miss",
		Timestamp:      time.Date(2026, 10, 9, 12, 30, 15, 123456000, time.UTC),
		OutputFindings: outFindings,
		OutputAction:   "REDACT",
		OutputScanned:  true,
		OutputRisk:     scanner.CalculateRisk(outFindings),
		PolicyName:     "bank-policy",
		PolicyVersion:  "7",
	}
}

func TestRequestLog_BothWritersBuildTheSameRow(t *testing.T) {
	hashing := func() *policy.Policy { return &policy.Policy{Data: &policy.DataControl{HashFindings: true}} }
	blocked := fullRequestEvent()
	blocked.EventType, blocked.Action = "request_blocked", "BLOCK"
	blocked.InputTokens, blocked.OutputTokens, blocked.CostKnown, blocked.CostUSD = 0, 0, false, 0
	blocked.OutputFindings, blocked.OutputAction, blocked.OutputScanned, blocked.OutputRisk = nil, "", false, scanner.RiskScore{}
	anonymous := fullRequestEvent()
	anonymous.OrgID, anonymous.OrgVerified = "", false

	for name, tc := range map[string]struct {
		e         events.Event
		getPolicy func() *policy.Policy
	}{
		"forwarded":       {fullRequestEvent(), nil},
		"hashed findings": {fullRequestEvent(), hashing},
		"blocked":         {blocked, nil},
		"no organisation": {anonymous, nil},
	} {
		t.Run(name, func(t *testing.T) {
			cap := &captureStore{NoopStore: &NoopStore{}}
			DBHandler(zerolog.Nop(), cap, evOrg, tc.getPolicy)(tc.e)
			if len(cap.rows) != 1 {
				t.Fatalf("the bus handler saved %d rows, want 1", len(cap.rows))
			}
			viaBus := cap.rows[0]
			viaNATS, ok := RequestLogFromV2(overNATS(t, tc.e), evOrg, tc.getPolicy)
			if !ok {
				t.Fatal("the NATS consumer did not take the request's event")
			}
			if !reflect.DeepEqual(viaBus, viaNATS) {
				t.Errorf("the two writers disagree:\n bus  %s\n nats %s", describeRow(viaBus), describeRow(viaNATS))
			}
			if viaBus.ActionTaken != strings.ToLower(tc.e.Action) {
				t.Errorf("action_taken = %q, want lower case from both", viaBus.ActionTaken)
			}
			if tc.getPolicy != nil {
				for _, raw := range [][]byte{viaNATS.Findings, viaNATS.OutputFindings} {
					if !strings.Contains(string(raw), `"match":"sha256:`) || strings.Contains(string(raw), "example.com") || strings.Contains(string(raw), "05**") {
						t.Errorf("data.hash_findings is on and the NATS writer stored %s", raw)
					}
				}
			}
		})
	}
}

// describeRow prints a row with its JSON columns and its cost readable.
func describeRow(r RequestLog) string {
	cost := "unknown"
	if r.CostUSD != nil {
		cost = fmt.Sprint(*r.CostUSD)
	}
	r.CostUSD = nil
	return fmt.Sprintf("%+v findings=%s output_findings=%s output_risk=%s cost=%s", r, r.Findings, r.OutputFindings, r.OutputRisk, cost)
}

func TestRequestLogFromEvent_Fields(t *testing.T) {
	rl, ok := RequestLogFromEvent(fullRequestEvent(), "", nil)
	if !ok {
		t.Fatal("a request's event is a row")
	}
	if rl.InputTokens != 1200 || rl.OutputTokens != 300 || rl.CostUSD == nil || *rl.CostUSD != 0.00036 {
		t.Errorf("tokens %d/%d, cost %v", rl.InputTokens, rl.OutputTokens, rl.CostUSD)
	}
	if rl.CacheStatus != "miss" || rl.OutputAction != "redact" || rl.PolicyName != "bank-policy" || rl.PolicyVersion != "7" {
		t.Errorf("row = %+v", rl)
	}
	if !strings.Contains(string(rl.OutputFindings), `"05** *** ** 23"`) || !strings.Contains(string(rl.OutputRisk), `"level"`) {
		t.Errorf("output findings %s, output risk %s", rl.OutputFindings, rl.OutputRisk)
	}
	if rl.ScanLatencyMs != 0.347 {
		t.Errorf("scan latency = %v, want the fraction kept", rl.ScanLatencyMs)
	}

	// Nothing known about the response, the tokens or the cost: nothing said.
	e := fullRequestEvent()
	e.InputTokens, e.OutputTokens, e.CostKnown = 0, 0, false
	e.OutputFindings, e.OutputAction, e.OutputScanned = nil, "", false
	rl, _ = RequestLogFromEvent(e, "", nil)
	if rl.CostUSD != nil || rl.OutputFindings != nil || rl.OutputRisk != nil || rl.OutputAction != "" {
		t.Errorf("unknown values were given a value: %+v", rl)
	}

	// A response that was scanned and clean is an empty list, not nothing.
	e.OutputScanned, e.OutputAction, e.OutputRisk = true, "PASS", scanner.CalculateRisk(nil)
	rl, _ = RequestLogFromEvent(e, "", nil)
	if string(rl.OutputFindings) != "[]" || rl.OutputAction != "pass" || !strings.Contains(string(rl.OutputRisk), `"none"`) {
		t.Errorf("clean response: findings %s, action %q, risk %s", rl.OutputFindings, rl.OutputAction, rl.OutputRisk)
	}
}

func TestRequestLogFromEvent_Organisation(t *testing.T) {
	row := func(org string, verified bool, def string) (RequestLog, bool) {
		e := fullRequestEvent()
		e.OrgID, e.OrgVerified = org, verified
		return RequestLogFromEvent(e, def, nil)
	}
	// What a caller without a key sent, when it cannot be an organisation
	// of the record: filed under the default, not dropped.
	if rl, ok := row("made-up", false, evOrg); !ok || rl.OrgID != evOrg {
		t.Errorf("unverified, not a UUID: org %q (row %v), want the default organisation", rl.OrgID, ok)
	}
	// A UUID a caller sent is still taken, as before.
	other := "00000000-0000-0000-0000-0000000000bb"
	if rl, _ := row(other, false, evOrg); rl.OrgID != other {
		t.Errorf("unverified UUID: org %q, want it kept", rl.OrgID)
	}
	// A key's organisation is not replaced by another one.
	if rl, _ := row("acme", true, evOrg); rl.OrgID != "acme" {
		t.Errorf("verified: org %q, want the key's", rl.OrgID)
	}
	if _, ok := row("", false, ""); ok {
		t.Error("no organisation and no default: there is no row to write")
	}
}

func TestRequestLogFromV2_OnlyARequestsOwnEvent(t *testing.T) {
	for _, e := range []events.Event{
		{EventType: "output_scan_hint", RequestID: "r1", Provider: "openai", ContentType: "application/json"},
		{EventType: "provider_state_change", Metadata: map[string]interface{}{"provider": "openai"}},
		{EventType: "output_scanned", RequestID: "r1", OutputAction: "BLOCK"},
	} {
		e.Timestamp = time.Now().UTC()
		// These travel on scan.completed, the subject of a passed request.
		if _, ok := RequestLogFromV2(overNATS(t, e), evOrg, nil); ok {
			t.Errorf("a %s event became a request_logs row", e.EventType)
		}
	}
}

// Envelopes published by the version before this one are still in the stream
// (it keeps a week). They do not name their event.
func TestRequestLogFromV2_OlderEnvelope(t *testing.T) {
	old := func(typ events.EventType, id string, payload map[string]any) events.EventV2 {
		return events.EventV2{ID: "x", Type: typ, RequestID: id, OrgID: evOrg, Timestamp: time.Now().UTC(), Payload: payload}
	}
	rl, ok := RequestLogFromV2(old(events.EventScanCompleted, "r-old", map[string]any{
		"provider": "openai", "model": "gpt-4o", "action": "PASS", "cost_usd": 0.0, "input_tokens": 0.0,
		"scan_latency_ms": 2.0, "total_latency_ms": 40.0,
	}), evOrg, nil)
	if !ok || rl.ActionTaken != "pass" || rl.TotalLatencyMs != 40 {
		t.Fatalf("an older request event: row %v, %+v", ok, rl)
	}
	// Its cost_usd: 0 was written for every event and says nothing.
	if rl.CostUSD != nil {
		t.Errorf("cost = %v, want unknown", *rl.CostUSD)
	}
	if _, ok := RequestLogFromV2(old(events.EventBlockTriggered, "r-old-b", map[string]any{"action": "BLOCK"}), evOrg, nil); !ok {
		t.Error("an older blocked request is still a row")
	}
	// An older output hint: same subject as a passed request, no action.
	if _, ok := RequestLogFromV2(old(events.EventScanCompleted, "r-old", map[string]any{"provider": "openai", "action": ""}), evOrg, nil); ok {
		t.Error("an older output hint became a row")
	}
}

func TestUnrecorded_CountedAndListed(t *testing.T) {
	before := map[string]int64{}
	for _, u := range UnrecordedRequests() {
		before[u.Reason] = u.Count
	}
	if _, ok := before[UnrecordedOrgNotUUID]; !ok {
		t.Fatalf("every reason is listed before its first count: %v", before)
	}
	s := &PostgresStore{log: zerolog.Nop()}
	if err := s.insertBatch(context.Background(), []RequestLog{{RequestID: "r1", OrgID: "acme"}, {RequestID: "r2", OrgID: ""}}); err != nil {
		t.Fatal(err)
	}
	for _, u := range UnrecordedRequests() {
		if u.Reason == UnrecordedOrgNotUUID && u.Count != before[u.Reason]+2 {
			t.Errorf("%s = %d, want %d: two requests could not be stored", u.Reason, u.Count, before[u.Reason]+2)
		}
	}
}
