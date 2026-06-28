package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/patterns"
)

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
	var body patterns.Pattern
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := cfg.Patterns.Create(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "pattern.create",
			Target: out.ID,
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
	var body patterns.Pattern
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	out, err := cfg.Patterns.Update(id, body)
	if err != nil {
		if errors.Is(err, patterns.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "pattern.update",
			Target: out.ID,
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
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "pattern.delete", Target: id})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
