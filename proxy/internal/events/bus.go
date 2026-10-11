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
	// Body is the request as it was forwarded, for the analyzer's deep
	// analysis. It is nil unless TAMGA_ANALYZER_SEND_CONTENT is on and a
	// content subscriber will take it (see Bus.WantsContent), and always
	// nil for a request that was blocked. It is the one field of an
	// event that holds request text, and only a handler registered with
	// SubscribeContent is given it: every other subscriber gets the event
	// with Body nil and BodyBytes saying how long it was.
	Body           []byte
	BodyBytes      int
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

	// CostKnown says CostUSD is what the request was charged. It is false
	// when no cost is known (a model with no price, a stream, a request that
	// reached no provider), and CostUSD is then not a cost of zero.
	// InputTokens and OutputTokens need no such flag: a count is never zero
	// on both sides, so both zero means the counts are not known.
	CostKnown bool
	// On a request's own event: the name and version of the policy the
	// request was decided with, and whether its response was scanned (the
	// output fields above then describe that scan).
	PolicyName    string
	PolicyVersion string
	OutputScanned bool
	// OrgVerified says OrgID came from a Tamga key. Otherwise it is what the
	// caller sent in X-Tamga-Org-Id, or empty.
	OrgVerified bool
	// Direction is "output" on a request's own event when what was judged
	// was an answer and nothing else: a call LiteLLM made for a response
	// (the LiteLLM route). Action and Findings are then the answer's. Empty
	// everywhere else.
	Direction string
	// FindingsHashed says the findings on this event hold a digest in place
	// of their match (data.hash_findings was on when it was published, see
	// Bus.SetPrepare). Whoever stores or sends them takes them as they are.
	FindingsHashed bool
}

const defaultChanCap = 1000

// WithoutContent returns e without the request text it carries (see Body).
// Nothing else on an event is request or response text: findings hold masked
// matches or digests, and Metadata holds identifiers and states.
func (e Event) WithoutContent() Event {
	if e.Body != nil {
		e.BodyBytes = len(e.Body)
		e.Body = nil
	}
	return e
}

// subscriber is a handler and whether it is given an event's request text.
// wants narrows a content subscriber to the events whose text it reads; nil
// means every event.
type subscriber struct {
	fn      func(Event)
	content bool
	wants   func(Event) bool
}

// takes reports whether the subscriber is given e's request text.
func (s subscriber) takes(e Event) bool {
	return s.content && (s.wants == nil || s.wants(e))
}

// Bus is a buffered, non-blocking publish / fan-out subscriber bus.
type Bus struct {
	ch       chan Event
	handlers []subscriber
	prepare  func(Event) Event
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
// The handler gets every event without its request text (see
// Event.WithoutContent), so one that keeps events, as the ring of recent
// events and the live feed do, keeps no text.
func (b *Bus) Subscribe(fn func(Event)) {
	b.subscribe(fn, false, nil)
}

// SubscribeContent registers a handler that is given the event as it was
// published, request text included. It is for a handler that needs the text
// at delivery and does not keep the event: the analyzer's. Like Subscribe,
// call it before Start.
func (b *Bus) SubscribeContent(fn func(Event)) {
	b.subscribe(fn, true, nil)
}

// SubscribeContentWhen is SubscribeContent for a handler that reads the text
// of some events only. wants says which: it is asked when an event is about
// to be published (WantsContent), so that no copy of a request is made for a
// handler that would not read it, and again at delivery, where an event it
// does not want arrives without its text. wants is called on the publisher's
// goroutine and must not block; nil means every event.
func (b *Bus) SubscribeContentWhen(fn func(Event), wants func(Event) bool) {
	b.subscribe(fn, true, wants)
}

func (b *Bus) subscribe(fn func(Event), content bool, wants func(Event) bool) {
	if b == nil || fn == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, subscriber{fn: fn, content: content, wants: wants})
}

// WantsContent reports whether some subscriber would be given, and read, the
// request text of e. A publisher asks before it copies a request onto an
// event: with no content subscriber, or none that wants this event, the copy
// would be made, queued and thrown away. e is the event as it will be
// published, without its Body.
func (b *Bus) WantsContent(e Event) bool {
	if b == nil || b.closed.Load() {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, h := range b.handlers {
		if h.takes(e) {
			return true
		}
	}
	return false
}

// SetPrepare sets a function every event goes through when it is published,
// before it is queued: what it returns is what every subscriber gets. It is
// where data.hash_findings is applied, so that the ring of recent events,
// the live feed, the NATS envelope and the request log all hold the same
// thing. Like Subscribe, call it before Start.
func (b *Bus) SetPrepare(fn func(Event) Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prepare = fn
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
	hs := make([]subscriber, len(b.handlers))
	copy(hs, b.handlers)
	b.mu.RUnlock()
	bare := e.WithoutContent()
	for _, h := range hs {
		if e.Body != nil && h.takes(e) {
			h.fn(e)
		} else {
			h.fn(bare)
		}
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
	b.mu.RLock()
	prepare := b.prepare
	b.mu.RUnlock()
	if prepare != nil {
		e = prepare(e)
	}
	// The queue holds an event, and its text, until it is dispatched and no
	// longer: a receive clears the slot. An event dropped here was never
	// queued, so nothing holds its text once this returns.
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
