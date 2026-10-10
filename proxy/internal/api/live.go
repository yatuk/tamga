package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

// defaultLiveHeartbeat is how often the live stream sends a comment when it
// has nothing else to say. A stream that carries no traffic looks, to the
// client and to every proxy in between, exactly like one whose other end is
// gone; the comment lets the client tell them apart and keeps idle timeouts
// away.
const defaultLiveHeartbeat = 15 * time.Second

func (cfg Config) liveHeartbeat() time.Duration {
	if cfg.LiveHeartbeat > 0 {
		return cfg.LiveHeartbeat
	}
	return defaultLiveHeartbeat
}

// ownEvent reports whether an event held in memory belongs to the
// deployment's organisation, which is the one the database is read for. An
// event that names no organisation is the deployment's own.
func (cfg Config) ownEvent(e events.Event) bool {
	return e.OrgID == "" || cfg.DefaultOrgID == "" || e.OrgID == cfg.DefaultOrgID
}

// handleLiveEvents streams scanned/blocked events to the client using
// Server-Sent Events. The connection stays open until the client disconnects
// or the broker unsubscribes on shutdown.
func (cfg Config) handleLiveEvents(w http.ResponseWriter, r *http.Request) {
	if cfg.Broker == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "live stream unavailable"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	// Initial comment so browsers commit headers + establish the stream.
	_, _ = fmt.Fprintf(w, ": tamga-live\n\n")
	flusher.Flush()

	ch, unsub := cfg.Broker.Subscribe()
	defer unsub()
	ctx := r.Context()
	beat := time.NewTicker(cfg.liveHeartbeat())
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-beat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case e, open := <-ch:
			if !open {
				return
			}
			if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
				continue
			}
			if !cfg.ownEvent(e) {
				continue
			}
			payload, err := json.Marshal(events.EventToJSON(e))
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\n", e.EventType)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
