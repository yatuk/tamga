package events

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/yatuk/tamga/internal/scanner"
)

// Event carries telemetry for the Tamga event bus (scan, block, outbound hints).
type Event struct {
	RequestID      string
	OrgID          string
	Provider       string
	Model          string
	ModelFamily    string // coarse family derived from Model (e.g. "claude-4", "gpt-4o")
	EventType      string // "request_scanned", "request_blocked", "output_scan_hint"
	Findings       []scanner.Finding
	OutputFindings []scanner.Finding // findings from response body scan (Faz 1A)
	OutputAction   string            // action taken on response (PASS/REDACT/BLOCK)
	Action         string
	Body           []byte // optional copy for deep analysis (may be nil)
	ContentType    string // e.g. response Content-Type for output_scan_hint
	Endpoint       string
	ScanLatencyMs  float64
	TotalLatencyMs float64
	InputTokens    int
	OutputTokens   int
	CostUSD        float64 // computed from tokens × pricing (Faz 3B)
	CacheStatus    string  // "hit" | "miss" | "bypass"
	UserID         string
	Timestamp      time.Time
	InputRisk      scanner.RiskScore
	OutputRisk     scanner.RiskScore
	// Metadata carries optional structured fields (e.g. provider circuit state).
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// TraceContext propagates W3C trace context to async consumers (analyzer, DB handlers).
	TraceContext map[string]string `json:"-"`
}

const defaultChanCap = 1000

// Bus is a buffered, non-blocking publish / fan-out subscriber bus.
type Bus struct {
	ch       chan Event
	handlers []func(Event)
	mu       sync.RWMutex
	wg       sync.WaitGroup
	closed   atomic.Bool
	dropped  atomic.Int64 // events silently lost due to full buffer
}

// NewBus creates a bus with a 1000-capacity buffer.
func NewBus() *Bus {
	return &Bus{
		ch: make(chan Event, defaultChanCap),
	}
}

// Subscribe registers a handler. Not safe to call after Start (document: call before Start).
func (b *Bus) Subscribe(fn func(Event)) {
	if b == nil || fn == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, fn)
}

// Start begins the dispatcher goroutine. Call once after Subscribe.
func (b *Bus) Start() {
	if b == nil {
		return
	}
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for e := range b.ch {
			b.dispatch(e)
		}
	}()
}

func (b *Bus) dispatch(e Event) {
	b.mu.RLock()
	hs := make([]func(Event), len(b.handlers))
	copy(hs, b.handlers)
	b.mu.RUnlock()
	for _, h := range hs {
		h(e)
	}
}

// Publish enqueues an event without trace propagation (background context).
func (b *Bus) Publish(e Event) {
	b.PublishContext(context.Background(), e)
}

// PublishContext injects the current trace context into the event for downstream spans.
func (b *Bus) PublishContext(ctx context.Context, e Event) {
	if b == nil || b.closed.Load() {
		return
	}
	if ctx != nil {
		carrier := make(propagation.MapCarrier)
		otel.GetTextMapPropagator().Inject(ctx, carrier)
		if len(carrier) > 0 {
			e.TraceContext = map[string]string(carrier)
		}
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	select {
	case b.ch <- e:
	default:
		b.dropped.Add(1) // observable via Prometheus /metrics
	}
}

// Dropped returns the number of events silently dropped due to a full buffer.
// Operators should alert on sustained positive values — it indicates the bus
// is undersized for current throughput or a handler is blocking.
func (b *Bus) Dropped() int64 {
	if b == nil {
		return 0
	}
	return b.dropped.Load()
}

// Stop closes the queue and waits until the dispatcher has drained remaining events.
func (b *Bus) Stop() {
	if b == nil {
		return
	}
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	close(b.ch)
	b.wg.Wait()
}
