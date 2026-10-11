package events

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// A publisher asks the bus before it copies a request onto an event. With no
// subscriber that takes text, the answer is no, whatever the event.
func TestBus_WantsContent(t *testing.T) {
	e := Event{EventType: "request_scanned", Action: "PASS", InputRisk: scanner.RiskScore{Score: 0.5}}

	var nilBus *Bus
	if nilBus.WantsContent(e) {
		t.Error("a nil bus wants content")
	}

	b := NewBus()
	if b.WantsContent(e) {
		t.Error("a bus with no subscriber wants content")
	}
	b.Subscribe(func(Event) {})
	if b.WantsContent(e) {
		t.Error("a bus whose subscribers get no text wants content")
	}

	b.SubscribeContentWhen(func(Event) {}, func(e Event) bool { return e.Action == "REDACT" })
	if b.WantsContent(e) {
		t.Error("content is wanted for an event the one content subscriber declined")
	}
	e.Action = "REDACT"
	if !b.WantsContent(e) {
		t.Error("content is not wanted for an event the content subscriber takes")
	}

	// A content subscriber registered without a condition takes every event.
	all := NewBus()
	all.SubscribeContent(func(Event) {})
	if !all.WantsContent(Event{}) {
		t.Error("SubscribeContent without a condition does not want content")
	}

	// A stopped bus delivers nothing, so it wants nothing.
	all.Start()
	all.Stop()
	if all.WantsContent(Event{}) {
		t.Error("a stopped bus wants content")
	}
}

// The question is asked on every request, before the copy it is there to
// save: it must not allocate itself.
func TestBus_WantsContentDoesNotAllocate(t *testing.T) {
	b := NewBus()
	b.Subscribe(func(Event) {})
	SubscribeAnalyzer(b, &mockAnalyzerClient{enabled: true})
	e := Event{
		EventType: "request_scanned", Action: "PASS", RequestID: "r1",
		Findings:  []scanner.Finding{{Type: "pii", Category: "email"}},
		InputRisk: scanner.RiskScore{Score: 0.5},
	}
	var wanted bool
	if n := testing.AllocsPerRun(200, func() { wanted = b.WantsContent(e) }); n != 0 {
		t.Errorf("WantsContent allocates %.0f times per call", n)
	}
	if !wanted {
		t.Error("the analyzer handler does not want an event it acts on")
	}
}

// AnalyzerWants and AnalyzerHandler cannot disagree: for every combination of
// analyzer state, event type, action and risk, the handler sends the text
// exactly when AnalyzerWants said it would. Both edges of the band are in
// the table.
func TestAnalyzerWants_IsExactlyWhatTheHandlerSends(t *testing.T) {
	clients := map[string]func() *mockAnalyzerClient{
		"no client":    func() *mockAnalyzerClient { return nil },
		"disconnected": func() *mockAnalyzerClient { return &mockAnalyzerClient{enabled: false} },
		"connected":    func() *mockAnalyzerClient { return &mockAnalyzerClient{enabled: true} },
	}
	risks := []float64{0, 0.01, 0.149, hybridSkipLow, 0.151, 0.3, 0.449, 0.45, 0.6, 0.899, hybridSkipHigh, 0.901, 0.95, 1}
	types := []string{"request_scanned", "request_blocked", "output_scan_hint", EventOperatorStateDeepScan, ""}
	actions := []string{"PASS", "WARN", "REDACT", "STRIP", "BLOCK", ""}

	wanted := 0
	for name, newClient := range clients {
		for _, typ := range types {
			for _, action := range actions {
				for _, risk := range risks {
					mock := newClient()
					var client AnalyzerClient
					if mock != nil {
						client = mock
					}
					e := Event{
						EventType: typ, Action: action, RequestID: "r",
						Body:      []byte("the request as it was forwarded"),
						InputRisk: scanner.RiskScore{Score: risk},
					}
					wants := AnalyzerWants(client)(e)
					AnalyzerHandler(client)(e)
					sent := mock != nil && mock.analyzeCalls == 1
					if wants != sent {
						t.Errorf("%s, %q/%q, risk %.3f: AnalyzerWants = %v, the handler sent = %v", name, typ, action, risk, wants, sent)
					}
					inBand := risk >= hybridSkipLow && risk <= hybridSkipHigh
					want := name == "connected" && typ == "request_scanned" && action != "BLOCK" && inBand
					if wants != want {
						t.Errorf("%s, %q/%q, risk %.3f: AnalyzerWants = %v, want %v", name, typ, action, risk, wants, want)
					}
					if wants {
						wanted++
					}
				}
			}
		}
	}
	// 1 client state x 1 type x 5 actions x 8 risks inside the band.
	if wanted != 40 {
		t.Errorf("%d combinations were wanted, want 40", wanted)
	}
}

// A nil *client inside the interface is what main hands over when the
// analyzer's address could not be dialled. It wants nothing.
func TestAnalyzerWants_NilClientInsideTheInterface(t *testing.T) {
	var client AnalyzerClient = (*offClient)(nil)
	e := Event{EventType: "request_scanned", Action: "PASS", InputRisk: scanner.RiskScore{Score: 0.5}}
	if AnalyzerWants(client)(e) {
		t.Error("a client that is not there wants content")
	}
	b := NewBus()
	SubscribeAnalyzer(b, client)
	if b.WantsContent(e) {
		t.Error("the bus wants content for an analyzer that is not there")
	}
}

type offClient struct{ mockAnalyzerClient }

func (c *offClient) Enabled() bool { return c != nil && c.enabled }

// At delivery the same condition holds: an event that came with text all the
// same reaches a content subscriber that declined it without the text, and
// one it takes with the text as published.
func TestBus_ContentSubscriberGetsTextOnlyForEventsItTakes(t *testing.T) {
	b := NewBus()
	client := &mockAnalyzerClient{enabled: true}
	var declinedBody, declinedSaid atomic.Int64
	seen := make(chan Event, 2)
	b.SubscribeContentWhen(func(e Event) {
		if e.RequestID == "low" {
			declinedBody.Store(int64(len(e.Body)))
			declinedSaid.Store(int64(e.BodyBytes))
		}
	}, AnalyzerWants(client))
	SubscribeAnalyzer(b, client)
	b.Subscribe(func(e Event) { seen <- e })
	b.Start()
	defer b.Stop()

	forwarded := `{"messages":[{"role":"user","content":"write to [email_REDACTED]"}]}`
	b.Publish(Event{EventType: "request_scanned", Action: "PASS", RequestID: "low", Body: []byte(forwarded), InputRisk: scanner.RiskScore{Score: 0.05}})
	b.Publish(Event{EventType: "request_scanned", Action: "REDACT", RequestID: "mid", Body: []byte(forwarded), InputRisk: scanner.RiskScore{Score: 0.5}})
	for i := 0; i < 2; i++ {
		select {
		case e := <-seen:
			if e.Body != nil {
				t.Errorf("%s: an ordinary subscriber got %d bytes of text", e.RequestID, len(e.Body))
			}
		case <-time.After(2 * time.Second):
			t.Fatal("the events were not delivered")
		}
	}

	if declinedBody.Load() != 0 || declinedSaid.Load() != int64(len(forwarded)) {
		t.Errorf("a declined event arrived with %d bytes of text (BodyBytes %d)", declinedBody.Load(), declinedSaid.Load())
	}
	if client.analyzeCalls != 1 || client.lastReq.GetRequestId() != "mid" {
		t.Fatalf("the analyzer was called %d times, last for %q; want once, for mid", client.analyzeCalls, client.lastReq.GetRequestId())
	}
	if got := client.lastReq.GetContent(); got != forwarded {
		t.Errorf("the analyzer got %q, want the request as forwarded", got)
	}
}
