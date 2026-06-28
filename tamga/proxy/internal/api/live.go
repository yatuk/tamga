package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yatuk/tamga/internal/events"
)

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
	for {
		select {
		case <-ctx.Done():
			return
		case e, open := <-ch:
			if !open {
				return
			}
			if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
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
