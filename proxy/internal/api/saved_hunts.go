package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/store"
)

// orgIDFromRequest returns the organisation whose saved hunts a request is
// about: the deployment's own. No credential carries an organisation into
// the management API, so X-Tamga-Org-Id, which the client writes, does not
// choose whose hunts are read or changed.
func (cfg Config) orgIDFromRequest(_ *http.Request) string {
	return cfg.DefaultOrgID
}

// auditHunt records a change to a saved hunt: who, which one and its name,
// not the query.
func (cfg Config) auditHunt(r *http.Request, kind, orgID, id, name string) {
	if cfg.Audit == nil {
		return
	}
	detail := map[string]interface{}{"org_id": orgID}
	if name != "" {
		detail["name"] = name
	}
	cfg.Audit.Append(incidents.AuditEntry{Kind: kind, Target: id, Actor: actorFromRequest(r), Detail: detail})
}

func (cfg Config) savedHuntStore() store.SavedHuntStore {
	if cfg.SavedHunts != nil {
		return cfg.SavedHunts
	}
	return nil
}

func (cfg Config) handleSavedHuntList(w http.ResponseWriter, r *http.Request) {
	st := cfg.savedHuntStore()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved hunts store unavailable"})
		return
	}
	orgID := cfg.orgIDFromRequest(r)
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing org_id"})
		return
	}
	hunts, err := st.List(r.Context(), orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if hunts == nil {
		hunts = []store.SavedHunt{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": hunts, "total": len(hunts)})
}

func (cfg Config) handleSavedHuntCreate(w http.ResponseWriter, r *http.Request) {
	st := cfg.savedHuntStore()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved hunts store unavailable"})
		return
	}
	orgID := cfg.orgIDFromRequest(r)
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing org_id"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Name      string          `json:"name"`
		QueryJSON json.RawMessage `json:"query_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	hunt := &store.SavedHunt{
		OrgID: orgID,
		Name:  name,
		Query: body.QueryJSON,
	}
	if err := st.Create(r.Context(), hunt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cfg.auditHunt(r, "hunt.create", orgID, hunt.ID, hunt.Name)
	writeJSON(w, http.StatusCreated, hunt)
}

func (cfg Config) handleSavedHuntUpdate(w http.ResponseWriter, r *http.Request) {
	st := cfg.savedHuntStore()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved hunts store unavailable"})
		return
	}
	orgID := cfg.orgIDFromRequest(r)
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing org_id"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing id"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Name      string          `json:"name"`
		QueryJSON json.RawMessage `json:"query_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	hunt := &store.SavedHunt{
		ID:    id,
		OrgID: orgID,
		Name:  strings.TrimSpace(body.Name),
		Query: body.QueryJSON,
	}
	if err := st.Update(r.Context(), hunt); err != nil {
		if errors.Is(err, store.ErrSavedHuntNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cfg.auditHunt(r, "hunt.update", orgID, hunt.ID, hunt.Name)
	writeJSON(w, http.StatusOK, hunt)
}

func (cfg Config) handleSavedHuntDelete(w http.ResponseWriter, r *http.Request) {
	st := cfg.savedHuntStore()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved hunts store unavailable"})
		return
	}
	orgID := cfg.orgIDFromRequest(r)
	if orgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing org_id"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing id"})
		return
	}
	if err := st.Delete(r.Context(), orgID, id); err != nil {
		if errors.Is(err, store.ErrSavedHuntNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cfg.auditHunt(r, "hunt.delete", orgID, id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
