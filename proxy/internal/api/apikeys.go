package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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
		Label         string `json:"label"`
		Scope         string `json:"scope"`
		OrgID         string `json:"org_id"`
		Role          string `json:"role"`
		UserID        string `json:"user_id"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Scope = strings.ToLower(strings.TrimSpace(body.Scope))
	if body.Scope == "" {
		body.Scope = apikeys.ScopeRead
	}
	if body.ExpiresInDays < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expires_in_days must not be negative"})
		return
	}
	p := apikeys.Params{Label: body.Label, Scope: body.Scope, OrgID: body.OrgID, Role: body.Role, UserID: body.UserID}
	if body.ExpiresInDays > 0 {
		p.ExpiresAt = time.Now().AddDate(0, 0, body.ExpiresInDays)
	}
	ck, err := cfg.APIKeys.CreateWith(p)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "apikey.create",
			Target: ck.ID,
			Detail: map[string]interface{}{"label": ck.Label, "scope": ck.Scope, "org_id": ck.OrgID, "role": ck.Role},
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
