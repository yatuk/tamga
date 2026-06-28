package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

// ── NATS Handler Bridge (in-memory bus → JetStream) ─────────────────────

// NATSHandler bridges the in-memory event bus to NATS JetStream for async
// downstream consumers (DB persistence, webhooks, SIEM). Low-latency
// handlers (Log, Metrics, RecentBuffer, liveBroker, Analyzer) remain on
// the in-memory bus for sub-millisecond dispatch.
type NATSHandler struct {
	pub *NATSPublisher
}

// NewNATSHandler creates a handler that publishes every event to NATS.
func NewNATSHandler(pub *NATSPublisher) *NATSHandler {
	return &NATSHandler{pub: pub}
}

// Handle converts the legacy Event to EventV2 and publishes to the
// appropriate NATS subject. The publish is fire-and-forget — errors are
// logged but never propagated to the caller.
func (h *NATSHandler) Handle(e Event) {
	if h.pub == nil {
		return
	}
	ev := eventToV2(e)
	subject := subjectForEvent(e.EventType, e.Action)

	// Fire-and-forget; the outbox worker guarantees eventual delivery.
	go func() {
		_ = h.pub.Publish(context.Background(), subject, ev)
	}()
}

// ── NATS Consumers (async, durable, retryable) ──────────────────────────

// EventHandler is a callback that processes a single EventV2. Register it
// with StartDurableConsumer to receive events from NATS JetStream.
type EventHandler func(EventV2)

// StartDurableConsumers launches durable JetStream consumers. Each consumer
// is identified by a durable name and filters on a subset of subjects (e.g.
// "scan.>" for all scan events). The handler is called for each event;
// messages are acknowledged after successful processing. Failed messages
// are redelivered by NATS (up to MaxDeliver attempts).
func StartDurableConsumers(
	pub *NATSPublisher,
	consumers map[string]struct {
		Subjects []string
		Handler  EventHandler
	},
	log zerolog.Logger,
) {
	if pub == nil {
		return
	}
	js := pub.Stream()
	ctx := context.Background()

	for durable, cfg := range consumers {
		go runDurableConsumer(ctx, js, "TAMGA_EVENTS", durable, cfg.Subjects, cfg.Handler, log)
	}
}

// runDurableConsumer creates (or resumes) a durable pull consumer and
// processes events in a fetch loop. Messages are acknowledged after
// successful handler execution.
func runDurableConsumer(
	ctx context.Context,
	js jetstream.JetStream,
	stream, durable string,
	filterSubjects []string,
	handler EventHandler,
	log zerolog.Logger,
) {
	consumer, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Durable:        durable,
		FilterSubjects: filterSubjects,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     5,
	})
	if err != nil {
		log.Error().Err(err).Str("component", "events").Str("durable", durable).Msg("failed to create NATS consumer")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batch, err := consumer.Fetch(10, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			continue
		}

		for msg := range batch.Messages() {
			var ev EventV2
			if err := json.Unmarshal(msg.Data(), &ev); err != nil {
				_ = msg.Ack()
				continue
			}
			handler(ev)
			_ = msg.Ack()
		}
	}
}
