package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/incidents"
)

func (cfg Config) handleAPIKeyList(w http.ResponseWriter, _ *http.Request) {
	if cfg.APIKeys == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []apikeys.Key{}, "total": 0})
		return
	}
	items := cfg.APIKeys.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

func (cfg Config) handleAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.APIKeys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "api keys store unavailable"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Label string `json:"label"`
		Scope string `json:"scope"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Scope = strings.ToLower(strings.TrimSpace(body.Scope))
	if body.Scope == "" {
		body.Scope = apikeys.ScopeRead
	}
	ck, err := cfg.APIKeys.Create(body.Label, body.Scope)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "apikey.create",
			Target: ck.ID,
			Detail: map[string]interface{}{"label": ck.Label, "scope": ck.Scope},
		})
	}
	writeJSON(w, http.StatusCreated, ck)
}

func (cfg Config) handleAPIKeyDelete(w http.ResponseWriter, r *http.Request) {
	if cfg.APIKeys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "api keys store unavailable"})
		return
	}
	id := r.PathValue("id")
	if err := cfg.APIKeys.Delete(id); err != nil {
		if errors.Is(err, apikeys.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "apikey.revoke", Target: id})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
