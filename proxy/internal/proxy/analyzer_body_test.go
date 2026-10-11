package proxy

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	pb "github.com/yatuk/tamga/proto/analyzer/v1"
)

// stubAnalyzer stands for the analyzer's gRPC client.
type stubAnalyzer struct {
	connected bool
	mu        sync.Mutex
	got       []string
}

func (s *stubAnalyzer) Enabled() bool { return s.connected }

func (s *stubAnalyzer) Analyze(_ context.Context, req *pb.AnalyzeRequest) (*pb.AnalyzeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, req.GetContent())
	return &pb.AnalyzeResponse{}, nil
}

func (s *stubAnalyzer) contents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.got...)
}

// analyzerRig is a proxy whose bus is wired as main wires it: the analyzer
// handler is the content subscriber, and an ordinary subscriber collects
// what the bus hands everyone else. That event has no Body by construction;
// its BodyBytes says how long the body on the published event was, so zero
// means no copy was attached.
type analyzerRig struct {
	*matrixRig
	analyzer *stubAnalyzer
	mu       sync.Mutex
	events   []events.Event
}

func newAnalyzerRig(t *testing.T, connected, sendContent bool) *analyzerRig {
	t.Helper()
	g := &analyzerRig{analyzer: &stubAnalyzer{connected: connected}}
	bus := events.NewBus()
	events.SubscribeAnalyzer(bus, g.analyzer)
	// After the analyzer's handler, so that once this one has an event the
	// analyzer's has already run.
	bus.Subscribe(func(e events.Event) {
		g.mu.Lock()
		g.events = append(g.events, e)
		g.mu.Unlock()
	})
	bus.Start()
	t.Cleanup(bus.Stop)
	g.matrixRig = newMatrixRig(t, mustPolicy(t, eventPolicy), &config.Config{AnalyzerSendContent: sendContent},
		func(hc *HandlerConfig) { hc.Bus = bus })
	return g
}

func (g *analyzerRig) requestEvent(t *testing.T) events.Event {
	t.Helper()
	var out events.Event
	eventually(t, "the request's event", func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		for _, e := range g.events {
			if e.EventType == "request_scanned" || e.EventType == "request_blocked" {
				out = e
				return true
			}
		}
		return false
	})
	return out
}

// A request that is forwarded with a masked address: its risk is inside the
// band the analyzer handler acts on (asserted below, so the cases that
// expect no copy are not passing for the wrong reason).
const inBandText = "write to ayse@example.com today"

// The copy of a forwarded request is made only when the analyzer's handler
// will send it. With no analyzer connected, with content off, or with a risk
// outside the handler's band, the event carries no body at all.
func TestRequestEvent_BodyIsCopiedOnlyWhenTheAnalyzerWillTakeIt(t *testing.T) {
	tests := []struct {
		name                   string
		connected, sendContent bool
		text                   string
		wantCopy               bool
	}{
		{"analyzer connected, content on, risk in band", true, true, inBandText, true},
		{"analyzer not connected, content on", false, true, inBandText, false},
		{"analyzer connected, content off", true, false, inBandText, false},
		{"analyzer not connected, content off", false, false, inBandText, false},
		{"analyzer connected, content on, risk below the band", true, true, "what is the capital of France", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newAnalyzerRig(t, tt.connected, tt.sendContent)
			resp, _ := g.send(t, "application/json", chatWith(tt.text))
			if resp.StatusCode != 200 {
				t.Fatalf("status %d, want the request forwarded", resp.StatusCode)
			}
			e := g.requestEvent(t)
			g.mu.Lock()
			forwarded := g.sent
			g.mu.Unlock()

			inBand := e.InputRisk.Score >= 0.15 && e.InputRisk.Score <= 0.90
			if (tt.text == inBandText) != inBand {
				t.Fatalf("risk %.3f: the case does not test what its name says", e.InputRisk.Score)
			}
			if e.Body != nil {
				t.Fatalf("an ordinary subscriber was given %d bytes of text", len(e.Body))
			}

			got := g.analyzer.contents()
			if !tt.wantCopy {
				if e.BodyBytes != 0 {
					t.Errorf("the event carried a body of %d bytes that nothing would read", e.BodyBytes)
				}
				if len(got) != 0 {
					t.Errorf("the analyzer was sent %d requests", len(got))
				}
				return
			}
			if e.BodyBytes != len(forwarded) {
				t.Errorf("the event carried %d bytes, the provider got %d", e.BodyBytes, len(forwarded))
			}
			if len(got) != 1 {
				t.Fatalf("the analyzer was sent %d requests, want 1", len(got))
			}
			if got[0] != forwarded {
				t.Errorf("the analyzer got %q, the provider got %q", got[0], forwarded)
			}
			if strings.Contains(got[0], "ayse@example.com") || !strings.Contains(got[0], "[email_REDACTED]") {
				t.Errorf("the analyzer did not get the request as it was masked: %s", got[0])
			}
		})
	}
}

// The same at the one place the copy is made, by what is allocated: with
// nothing to take it, a body at the size limit costs no allocation; with the
// analyzer connected and the risk in band it is copied once, and the copy is
// not the caller's slice.
func TestAnalyzerContent_AllocatesOnlyForAnEventThatIsWanted(t *testing.T) {
	body := bytes.Repeat([]byte("a"), maxEventBodyBytes)
	on, off := &config.Config{AnalyzerSendContent: true}, &config.Config{}
	inBand := events.Event{EventType: "request_scanned", Action: string(policy.ActionRedact), InputRisk: scanner.RiskScore{Score: 0.5}}
	low, high, blocked := inBand, inBand, inBand
	low.InputRisk.Score, high.InputRisk.Score = 0.05, 0.95
	blocked.Action, blocked.EventType = string(policy.ActionBlock), "request_blocked"

	busFor := func(connected bool) *events.Bus {
		b := events.NewBus()
		events.SubscribeAnalyzer(b, &stubAnalyzer{connected: connected})
		b.Subscribe(func(events.Event) {})
		return b
	}
	noContentSubscriber := events.NewBus()
	noContentSubscriber.Subscribe(func(events.Event) {})

	none := []struct {
		name string
		cfg  HandlerConfig
		e    events.Event
	}{
		{"content off", HandlerConfig{Config: off, Bus: busFor(true)}, inBand},
		{"no config", HandlerConfig{Bus: busFor(true)}, inBand},
		{"analyzer not connected", HandlerConfig{Config: on, Bus: busFor(false)}, inBand},
		{"no content subscriber", HandlerConfig{Config: on, Bus: noContentSubscriber}, inBand},
		{"no bus", HandlerConfig{Config: on}, inBand},
		{"risk below the band", HandlerConfig{Config: on, Bus: busFor(true)}, low},
		{"risk above the band", HandlerConfig{Config: on, Bus: busFor(true)}, high},
		{"blocked", HandlerConfig{Config: on, Bus: busFor(true)}, blocked},
	}
	for _, tt := range none {
		var got []byte
		allocs := testing.AllocsPerRun(50, func() { got = analyzerContent(tt.cfg, body, tt.e) })
		if got != nil || allocs != 0 {
			t.Errorf("%s: a body of %d bytes, %.0f allocations per call; want none", tt.name, len(got), allocs)
		}
	}

	wanted := HandlerConfig{Config: on, Bus: busFor(true)}
	var got []byte
	allocs := testing.AllocsPerRun(50, func() { got = analyzerContent(wanted, body, inBand) })
	if !bytes.Equal(got, body) || allocs != 1 {
		t.Fatalf("wanted: %d bytes, %.0f allocations per call; want the body, copied once", len(got), allocs)
	}
	if &got[0] == &body[0] {
		t.Error("the event was given the caller's slice, not a copy")
	}
	// Over the limit nothing is carried, as before.
	if analyzerContent(wanted, append(body, 'a'), inBand) != nil {
		t.Error("a body over the limit was copied")
	}
}
