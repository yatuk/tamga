package events

import (
	"sync/atomic"
	"testing"
	"time"
)

func testBus(capacity int) *Bus {
	return &Bus{
		ch: make(chan Event, capacity),
	}
}

func TestBus_PublishSubscribe(t *testing.T) {
	b := NewBus()
	var n atomic.Int32
	done := make(chan struct{}, 1)
	b.Subscribe(func(e Event) {
		if e.EventType == "request_scanned" {
			n.Add(1)
		}
		select {
		case done <- struct{}{}:
		default:
		}
	})
	b.Start()
	b.Publish(Event{EventType: "request_scanned", Timestamp: time.Now()})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not run")
	}
	if n.Load() != 1 {
		t.Fatalf("want 1 delivery, got %d", n.Load())
	}
	b.Stop()
}

func TestBus_DropWhenFullDoesNotBlock(t *testing.T) {
	b := testBus(2)
	// No Start — consumer not draining; Publish must never block.
	for i := 0; i < 3; i++ {
		b.Publish(Event{EventType: "request_scanned", Timestamp: time.Now()})
	}
	if len(b.ch) != 2 {
		t.Fatalf("want 2 events retained (3rd dropped), got len=%d", len(b.ch))
	}
}

func TestBus_StopDrainsPending(t *testing.T) {
	b := NewBus()
	var n atomic.Int32
	b.Subscribe(func(Event) { n.Add(1) })
	b.Start()
	b.Publish(Event{EventType: "request_scanned", Timestamp: time.Now()})
	b.Stop()
	if n.Load() != 1 {
		t.Fatalf("want 1 after drain, got %d", n.Load())
	}
}
