package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// retentionMarker stands for request text: it is in the body of the events
// these tests publish and must be nowhere once they are delivered.
const retentionMarker = "RETAINED-REQUEST-TEXT-7f3a"

// holdsText reports whether a string or a byte slice reachable from v holds
// marker. It follows pointers, interfaces, slices, arrays, maps and struct
// fields, unexported ones included; a channel's buffer is not reachable this
// way and has to be drained by the caller.
func holdsText(v reflect.Value, marker string, depth int) bool {
	if depth > 32 || !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(v.String(), marker)
	case reflect.Pointer, reflect.Interface:
		return !v.IsNil() && holdsText(v.Elem(), marker, depth+1)
	case reflect.Slice:
		if v.IsNil() {
			return false
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return bytes.Contains(v.Bytes(), []byte(marker))
		}
		fallthrough
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if holdsText(v.Index(i), marker, depth+1) {
				return true
			}
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if holdsText(it.Key(), marker, depth+1) || holdsText(it.Value(), marker, depth+1) {
				return true
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if holdsText(v.Field(i), marker, depth+1) {
				return true
			}
		}
	}
	return false
}

// The walker itself: it has to see text behind an unexported field, a
// pointer in a map of interfaces and a byte slice, or the tests below prove
// nothing.
func TestHoldsText_FindsTextWhereAnEventCanCarryIt(t *testing.T) {
	type inner struct{ hidden []byte }
	for name, v := range map[string]any{
		"body":     Event{Body: []byte("x " + retentionMarker)},
		"metadata": Event{Metadata: map[string]interface{}{"k": &inner{hidden: []byte(retentionMarker)}}},
		"finding":  Event{Findings: []scanner.Finding{{Match: retentionMarker}}},
		"ring":     &RecentBuffer{byID: map[string]Event{"r": {Body: []byte(retentionMarker)}}},
	} {
		if !holdsText(reflect.ValueOf(v), retentionMarker, 0) {
			t.Errorf("%s: the text was not found", name)
		}
	}
	if holdsText(reflect.ValueOf(Event{RequestID: "r", Body: []byte("other")}), retentionMarker, 0) {
		t.Error("found text that is not there")
	}
}

// publishWithBody publishes a forwarded request's event whose body holds the
// marker, and returns a flag that is set once that body has been collected.
// It is its own function so that the caller's frame holds no reference to
// the body.
//
//go:noinline
func publishWithBody(b *Bus, requestID string) *atomic.Bool {
	body := bytes.Repeat([]byte(retentionMarker+" "), 512)
	freed := new(atomic.Bool)
	runtime.SetFinalizer(&body[0], func(*byte) { freed.Store(true) })
	b.Publish(Event{
		EventType: "request_scanned",
		Action:    "PASS",
		RequestID: requestID,
		OrgID:     "org-1",
		Body:      body,
		Findings:  []scanner.Finding{{Type: "pii", Category: "email", Match: "a***@example.com"}},
		InputRisk: scanner.RiskScore{Score: 0.5, Level: "medium", Percentage: 50},
	})
	return freed
}

// collected waits for the garbage collector to have freed what freed stands
// for.
func collected(freed *atomic.Bool) bool {
	for i := 0; i < 200; i++ {
		runtime.GC()
		if freed.Load() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// Request text is on an event only for the analyzer (Event.Body). It reaches
// the handler registered for it and no other, and once the event has been
// delivered nothing holds it: not the ring of recent events, not a live
// subscriber's backlog, not what either serialises, and not the bus, which
// is still running here.
func TestRequestText_ReachesTheAnalyzerOnlyAndIsNotKept(t *testing.T) {
	bus := NewBus()
	ring := NewRecentBuffer(10)
	broker := NewBroker(8)
	live, unsub := broker.Subscribe() // a client that reads nothing yet
	defer unsub()

	var contentBytes, plainBytes, plainSaid atomic.Int64
	delivered := make(chan struct{}, 1)
	bus.Subscribe(func(e Event) {
		plainBytes.Add(int64(len(e.Body)))
		plainSaid.Store(int64(e.BodyBytes))
	})
	bus.SubscribeContent(func(e Event) {
		if bytes.Contains(e.Body, []byte(retentionMarker)) {
			contentBytes.Add(int64(len(e.Body)))
		}
	})
	bus.Subscribe(RecentBufferHandler(ring))
	bus.Subscribe(broker.Publish)
	bus.Subscribe(func(Event) { delivered <- struct{}{} })
	bus.Start()
	defer bus.Stop()

	freed := publishWithBody(bus, "req-text-1")
	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("the event was not delivered")
	}

	want := int64(512 * (len(retentionMarker) + 1))
	if contentBytes.Load() != want {
		t.Errorf("the content subscriber got %d bytes of text, want %d", contentBytes.Load(), want)
	}
	if plainBytes.Load() != 0 {
		t.Errorf("an ordinary subscriber got %d bytes of text", plainBytes.Load())
	}
	if plainSaid.Load() != want {
		t.Errorf("BodyBytes = %d for an ordinary subscriber, want %d", plainSaid.Load(), want)
	}

	// In memory: the ring as a whole, its index included.
	if holdsText(reflect.ValueOf(ring), retentionMarker, 0) {
		t.Error("the ring of recent events holds the request text")
	}
	kept, ok := ring.GetByRequestID("req-text-1")
	if !ok || kept.Body != nil {
		t.Errorf("ring event: found=%v, body of %d bytes", ok, len(kept.Body))
	}
	// Serialised: what the events API and the NATS bridge make of it.
	if raw, err := MarshalEventsJSON(ring.Latest(0)); err != nil || bytes.Contains(raw, []byte(retentionMarker)) {
		t.Errorf("the events API form holds the text (err=%v)", err)
	}
	if raw, err := json.Marshal(ToV2(kept)); err != nil || bytes.Contains(raw, []byte(retentionMarker)) {
		t.Errorf("the NATS envelope holds the text (err=%v)", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", kept), retentionMarker) {
		t.Error("the printed event holds the text")
	}

	// The body itself is gone while the ring, the unread live backlog and
	// the running bus are all still alive.
	if !collected(freed) {
		t.Error("the request body is still reachable after delivery")
	}

	select {
	case e := <-live:
		if holdsText(reflect.ValueOf(e), retentionMarker, 0) {
			t.Error("the live feed's backlog holds the request text")
		}
		if raw, _ := json.Marshal(EventToJSON(e)); bytes.Contains(raw, []byte(retentionMarker)) {
			t.Error("the live feed's frame holds the text")
		}
	default:
		t.Fatal("the live subscriber got no event")
	}
	runtime.KeepAlive(ring)
	runtime.KeepAlive(broker)
}

// The ring and the live broker drop the text themselves, whoever hands them
// an event.
func TestRingAndBroker_DropTextGivenDirectly(t *testing.T) {
	e := Event{EventType: "request_scanned", RequestID: "r1", Body: []byte(retentionMarker)}
	ring := NewRecentBuffer(2)
	ring.Add(e)
	if holdsText(reflect.ValueOf(ring), retentionMarker, 0) {
		t.Error("RecentBuffer.Add kept the text")
	}
	broker := NewBroker(2)
	live, unsub := broker.Subscribe()
	defer unsub()
	broker.Publish(e)
	if got := <-live; got.Body != nil || got.BodyBytes != len(retentionMarker) {
		t.Errorf("Broker.Publish passed on a body of %d bytes (BodyBytes %d)", len(got.Body), got.BodyBytes)
	}
	if string(e.Body) != retentionMarker {
		t.Error("the caller's event was changed")
	}
}

// An event that finds the queue full is dropped, and its text with it: the
// bus keeps nothing of an event it did not queue. One that was queued is
// held until it is dispatched and released then.
func TestBus_DroppedEventReleasesItsText(t *testing.T) {
	b := testBus(1)
	queued := publishWithBody(b, "queued")
	dropped := publishWithBody(b, "dropped")
	if b.Dropped() != 1 {
		t.Fatalf("dropped = %d, want 1", b.Dropped())
	}
	if !collected(dropped) {
		t.Error("the dropped event's body is still reachable")
	}
	if queued.Load() {
		t.Fatal("the queued event's body was collected before delivery")
	}
	var got atomic.Int64
	b.SubscribeContent(func(e Event) { got.Add(int64(len(e.Body))) })
	b.Start()
	b.Stop()
	if got.Load() == 0 {
		t.Error("the queued event was not delivered with its text")
	}
	if !collected(queued) {
		t.Error("the queued event's body is still reachable after the bus drained")
	}
}
