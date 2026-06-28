package events

import (
	"sync"
	"sync/atomic"
)

// Broker fan-outs events to runtime-registered subscribers (SSE clients).
//
// Unlike Bus, subscribers can come and go at any time and slow subscribers
// never block publishers — full channels simply drop the oldest frame.
type Broker struct {
	mu   sync.RWMutex
	next atomic.Int64
	subs map[int64]chan Event
	cap  int
}

// NewBroker constructs a broker with the given per-subscriber buffer size.
func NewBroker(bufPerSub int) *Broker {
	if bufPerSub < 1 {
		bufPerSub = 32
	}
	return &Broker{subs: make(map[int64]chan Event), cap: bufPerSub}
}

// Subscribe returns a channel and an unsubscribe function. The channel is
// closed by the unsubscribe call — it is safe to call multiple times.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, b.cap)
	id := b.next.Add(1)
	b.mu.Lock()
	b.subs[id] = ch
	b.mu.Unlock()
	var once sync.Once
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			if c, ok := b.subs[id]; ok {
				delete(b.subs, id)
				close(c)
			}
			b.mu.Unlock()
		})
	}
	return ch, unsub
}

// Publish dispatches an event to every live subscriber, dropping frames on
// saturated buffers rather than blocking the hot path.
func (b *Broker) Publish(e Event) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
			// drop
		}
	}
}

// Size returns the current subscriber count (useful for metrics / tests).
func (b *Broker) Size() int {
	if b == nil {
		return 0
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
