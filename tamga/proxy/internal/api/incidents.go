package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
)

func (cfg Config) handleIncidentGet(w http.ResponseWriter, r *http.Request) {
	if cfg.Incidents == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "incidents store unavailable"})
		return
	}
	id := r.PathValue("request_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_id"})
		return
	}
	st, err := cfg.Incidents.Get(id)
	if err != nil {
		if errors.Is(err, incidents.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "incident not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (cfg Config) handleIncidentList(w http.ResponseWriter, r *http.Request) {
	if cfg.Incidents == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "incidents store unavailable"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items := cfg.Incidents.List(limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

func (cfg Config) handleIncidentPatch(w http.ResponseWriter, r *http.Request) {
	if cfg.Incidents == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "incidents store unavailable"})
		return
	}
	id := r.PathValue("request_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_id"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var patch incidents.Patch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	// Basic validation for known enum-ish fields.
	if patch.Status != nil {
		switch *patch.Status {
		case incidents.StatusOpen, incidents.StatusInProgress, incidents.StatusClosed, incidents.StatusFalsePositive:
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
	}
	st, err := cfg.Incidents.Apply(id, patch)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		detail := map[string]interface{}{}
		if patch.Status != nil {
			detail["status"] = *patch.Status
		}
		if patch.Assignee != nil {
			detail["assignee"] = *patch.Assignee
		}
		if patch.Tags != nil {
			detail["tags"] = patch.Tags
		}
		if patch.AddComment != nil {
			detail["comment"] = strings.TrimSpace(patch.AddComment.Text)
		}
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "incident.patch",
			Target: id,
			Detail: detail,
		})
	}
	writeJSON(w, http.StatusOK, st)
}

func (cfg Config) handleAuditList(w http.ResponseWriter, r *http.Request) {
	if cfg.Audit == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []incidents.AuditEntry{}, "total": 0})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items := cfg.Audit.List(limit)
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

// handleIncidentTriage processes POST /api/v1/incidents/{request_id}/triage
func (cfg Config) handleIncidentTriage(w http.ResponseWriter, r *http.Request) {
	if cfg.IncidentLifecycle == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle store unavailable"})
		return
	}
	id := r.PathValue("request_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_id"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Assignee string `json:"assignee"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if err := cfg.IncidentLifecycle.Triage(r.Context(), id, body.Assignee); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Fetch updated state to return.
	st, err := cfg.Incidents.Get(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "request_id": id})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "incident.triage",
			Target: id,
			Detail: map[string]interface{}{"assignee": body.Assignee},
		})
	}
	writeJSON(w, http.StatusOK, st)
}

// handleIncidentResolve processes POST /api/v1/incidents/{request_id}/resolve
func (cfg Config) handleIncidentResolve(w http.ResponseWriter, r *http.Request) {
	if cfg.IncidentLifecycle == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle store unavailable"})
		return
	}
	id := r.PathValue("request_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_id"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Resolution string `json:"resolution"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}
	if body.Resolution == "" {
		body.Resolution = "true_positive"
	}
	resolvedBy := actorFromRequest(r)
	if err := cfg.IncidentLifecycle.Resolve(r.Context(), id, body.Resolution, body.Notes, resolvedBy); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	st, err := cfg.Incidents.Get(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "request_id": id})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "incident.resolve",
			Target: id,
			Detail: map[string]interface{}{"resolution": body.Resolution, "resolved_by": resolvedBy},
		})
	}
	writeJSON(w, http.StatusOK, st)
}

// handleIncidentReopen processes POST /api/v1/incidents/{request_id}/reopen
func (cfg Config) handleIncidentReopen(w http.ResponseWriter, r *http.Request) {
	if cfg.IncidentLifecycle == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle store unavailable"})
		return
	}
	id := r.PathValue("request_id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_id"})
		return
	}
	if err := cfg.IncidentLifecycle.Reopen(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	st, err := cfg.Incidents.Get(id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "request_id": id})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "incident.reopen",
			Target: id,
		})
	}
	writeJSON(w, http.StatusOK, st)
}

// handleMTTR processes GET /api/v1/mttr?range=7d&org_id=...
func (cfg Config) handleMTTR(w http.ResponseWriter, r *http.Request) {
	if cfg.IncidentLifecycle == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "lifecycle store unavailable"})
		return
	}
	orgID := r.URL.Query().Get("org_id")
	rng := strings.ToLower(r.URL.Query().Get("range"))
	if rng == "" {
		rng = "7d"
	}
	to := time.Now().UTC()
	from := to.Add(time.Duration(-rangeMillis(rng)) * time.Millisecond)
	stats, err := cfg.IncidentLifecycle.CalculateMTTR(r.Context(), orgID, from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
