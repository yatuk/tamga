package api

import (
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/docstore"
)

// writeStoreError answers a failed settings change. A change the database
// could not take is the server's failure, not the caller's, and its detail
// stays in the log.
func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, docstore.ErrUnavailable) {
		log.Error().Err(err).Msg("settings change could not be stored")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "the change could not be stored, try again"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
