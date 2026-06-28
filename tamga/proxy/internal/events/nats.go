package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

// NATSConfig holds connection parameters for the NATS JetStream publisher.
type NATSConfig struct {
	URL    string
	Stream string // stream name, e.g. "TAMGA_EVENTS"
}

// NATSPublisher sends versioned events to NATS JetStream.
type NATSPublisher struct {
	js  jetstream.JetStream
	nc  *nats.Conn
	log zerolog.Logger
}

// NewNATSPublisher connects to NATS and ensures the event stream exists.
// The stream is created idempotently — safe to call at every startup.
func NewNATSPublisher(ctx context.Context, cfg NATSConfig, log zerolog.Logger) (*NATSPublisher, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.Timeout(5*time.Second),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
		nats.Name("tamga-proxy"),
	)
	if err != nil {
		return nil, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}

	// Create stream once; JetStream treats this as idempotent.
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     cfg.Stream,
		Subjects: []string{"scan.>", "block.>", "redact.>", "output.>"},
		Storage:  jetstream.FileStorage,
		MaxAge:   7 * 24 * time.Hour,
	})
	if err != nil {
		log.Warn().Err(err).Str("component", "events").Str("stream", cfg.Stream).Msg("creating NATS stream (may already exist)")
	}

	return &NATSPublisher{js: js, nc: nc, log: log}, nil
}

// Publish sends a versioned event to the given NATS subject.
func (p *NATSPublisher) Publish(ctx context.Context, subject string, ev EventV2) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = p.js.Publish(ctx, subject, data)
	if err != nil {
		p.log.Warn().Err(err).Str("component", "events").Str("subject", subject).Str("event_id", ev.ID).Msg("nats publish failed")
	}
	return err
}

// Stream returns the underlying JetStream context for consumer setup.
func (p *NATSPublisher) Stream() jetstream.JetStream {
	return p.js
}

// Close drains and closes the NATS connection.
func (p *NATSPublisher) Close() {
	p.nc.Close()
}
