package events

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/operator_state"
)

// starred replaces every match by a marker, as a new slice.
func starred(fs []scanner.Finding) []scanner.Finding {
	out := make([]scanner.Finding, len(fs))
	for i, f := range fs {
		out[i] = f
		out[i].Match = "changed"
	}
	return out
}

// What SetPrepare returns is what every subscriber gets, and it is applied
// when the event is published, before the first subscriber.
func TestBus_SetPrepare(t *testing.T) {
	b := NewBus()
	var got [2]Event
	b.Subscribe(func(e Event) { got[0] = e })
	b.Subscribe(func(e Event) { got[1] = e })
	b.SetPrepare(func(e Event) Event {
		e = MapFindings(e, starred)
		e.FindingsHashed = true
		return e
	})
	b.Start()
	published := Event{EventType: "request_scanned", Findings: []scanner.Finding{{Type: "pii", Match: "10*******46"}}}
	b.Publish(published)
	b.Stop()

	for i, e := range got {
		if len(e.Findings) != 1 || e.Findings[0].Match != "changed" || !e.FindingsHashed || e.Timestamp.IsZero() {
			t.Errorf("subscriber %d got %+v", i, e)
		}
	}
	if published.Findings[0].Match != "10*******46" {
		t.Error("the publisher's findings were written to")
	}

	// Without one the event goes through as it is.
	plain := NewBus()
	var seen Event
	plain.Subscribe(func(e Event) { seen = e })
	plain.Start()
	plain.Publish(published)
	plain.Stop()
	if seen.Findings[0].Match != "10*******46" || seen.FindingsHashed {
		t.Errorf("without SetPrepare: %+v", seen)
	}
	var none *Bus
	none.SetPrepare(func(e Event) Event { return e })
}

// MapFindings reaches every list of findings an event carries.
func TestMapFindings(t *testing.T) {
	fast, _ := json.Marshal(scanner.Finding{Type: "operator_state", Match: "DEC-1042"})
	req := &operator_state.AnalyzerRequest{RequestID: "r-1", FastFindings: []json.RawMessage{fast, json.RawMessage(`{"match":`)}}
	meta := map[string]interface{}{operatorStateRequestKey: req}
	e := Event{
		Findings:       []scanner.Finding{{Match: "in"}},
		OutputFindings: []scanner.Finding{{Match: "out"}},
		Metadata:       meta,
	}
	got := MapFindings(e, starred)
	if got.Findings[0].Match != "changed" || got.OutputFindings[0].Match != "changed" {
		t.Errorf("findings %+v, output findings %+v", got.Findings, got.OutputFindings)
	}
	sent, _ := got.Metadata[operatorStateRequestKey].(*operator_state.AnalyzerRequest)
	// The entry that could not be read is left out, not passed on as it was.
	if sent == nil || len(sent.FastFindings) != 1 || !strings.Contains(string(sent.FastFindings[0]), `"match":"changed"`) {
		t.Fatalf("fast findings: %+v", sent)
	}
	if e.Findings[0].Match != "in" || e.OutputFindings[0].Match != "out" || meta[operatorStateRequestKey] != req ||
		len(req.FastFindings) != 2 || string(req.FastFindings[0]) != string(fast) {
		t.Error("the event given was written to")
	}

	// An event without findings, and no function, are left alone.
	empty := MapFindings(Event{EventType: "output_scan_hint"}, starred)
	if empty.Findings != nil || empty.OutputFindings != nil || empty.Metadata != nil {
		t.Errorf("an empty event: %+v", empty)
	}
	if same := MapFindings(e, nil); same.Findings[0].Match != "in" {
		t.Errorf("no function: %+v", same.Findings)
	}
}

// The envelope says when its findings are digests, and only then.
func TestEventToV2_FindingsHashed(t *testing.T) {
	e := Event{EventType: "request_scanned", Findings: []scanner.Finding{{Match: "hmac-sha256:00"}}, FindingsHashed: true}
	if v, _ := ToV2(e).Payload["findings_hashed"].(bool); !v {
		t.Error("a hashed event's envelope does not say so")
	}
	e.FindingsHashed = false
	if _, ok := ToV2(e).Payload["findings_hashed"]; ok {
		t.Error("an unhashed event's envelope says findings_hashed")
	}
}
