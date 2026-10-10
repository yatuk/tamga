package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/patterns"
)

// maxPatternBodyBytes bounds the body of a pattern create or update. A
// pattern is at most scanner.MaxPatternBytes; the rest is a name and three
// short fields.
const maxPatternBodyBytes = 16 << 10

// decodePattern reads a pattern from the request, and answers the request
// itself when it cannot: 413 for a body over the limit, 400 otherwise.
func decodePattern(w http.ResponseWriter, r *http.Request) (patterns.Pattern, bool) {
	var body patterns.Pattern
	r.Body = http.MaxBytesReader(w, r.Body, maxPatternBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body exceeds 16384 bytes"})
			return body, false
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return body, false
	}
	return body, true
}

func (cfg Config) handlePatternList(w http.ResponseWriter, _ *http.Request) {
	if cfg.Patterns == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []patterns.Pattern{}, "total": 0})
		return
	}
	items := cfg.Patterns.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

func (cfg Config) handlePatternCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.Patterns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patterns store unavailable"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	body, ok := decodePattern(w, r)
	if !ok {
		return
	}
	out, err := cfg.Patterns.Create(body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	// Activate the new pattern immediately: the custom scanner caches compiled
	// specs and only recompiles on Refresh, so without this a created pattern
	// would sit inactive until the next policy reload.
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "pattern.create",
			Target: out.ID,
			Actor:  actorFromRequest(r),
			Detail: map[string]interface{}{"name": out.Name, "kind": out.Kind, "severity": out.Severity},
		})
	}
	writeJSON(w, http.StatusCreated, out)
}

func (cfg Config) handlePatternUpdate(w http.ResponseWriter, r *http.Request) {
	if cfg.Patterns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patterns store unavailable"})
		return
	}
	id := r.PathValue("id")
	defer func() { _ = r.Body.Close() }()
	body, ok := decodePattern(w, r)
	if !ok {
		return
	}
	out, err := cfg.Patterns.Update(id, body)
	if err != nil {
		if errors.Is(err, patterns.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeStoreError(w, err)
		return
	}
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "pattern.update",
			Target: out.ID,
			Actor:  actorFromRequest(r),
			Detail: map[string]interface{}{"name": out.Name, "kind": out.Kind, "severity": out.Severity, "enabled": out.Enabled},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (cfg Config) handlePatternDelete(w http.ResponseWriter, r *http.Request) {
	if cfg.Patterns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "patterns store unavailable"})
		return
	}
	id := r.PathValue("id")
	if err := cfg.Patterns.Delete(id); err != nil {
		if errors.Is(err, patterns.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg.CustomScanner != nil {
		cfg.CustomScanner.Refresh()
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "pattern.delete", Target: id, Actor: actorFromRequest(r)})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
