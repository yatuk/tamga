package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/yatuk/tamga/internal/scim"
)

// We store the list response schema URN.
var scimListSchemaURN = "urn:ietf:params:scim:api:messages:2.0:ListResponse"

// handleScimListUsers returns all SCIM users (GET /scim/v2/Users).
func (cfg Config) handleScimListUsers(w http.ResponseWriter, r *http.Request) {
	if cfg.Scim == nil {
		writeJSON(w, http.StatusOK, scim.ListResponse{
			Schemas:      []string{scimListSchemaURN},
			TotalResults: 0,
			Resources:    []scim.User{},
		})
		return
	}
	users := cfg.Scim.List()
	writeJSON(w, http.StatusOK, scim.ListResponse{
		Schemas:      []string{scimListSchemaURN},
		TotalResults: len(users),
		Resources:    users,
	})
}

// handleScimCreateUser creates a new SCIM user (POST /scim/v2/Users).
func (cfg Config) handleScimCreateUser(w http.ResponseWriter, r *http.Request) {
	if cfg.Scim == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SCIM store unavailable"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body scim.User
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	out, err := cfg.Scim.Create(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleScimGetUser returns a single SCIM user by id (GET /scim/v2/Users/{id}).
func (cfg Config) handleScimGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if cfg.Scim == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	user, ok := cfg.Scim.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// handleScimPatchUser applies a SCIM PATCH to a user (PATCH /scim/v2/Users/{id}).
func (cfg Config) handleScimPatchUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if cfg.Scim == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SCIM store unavailable"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var patch map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	out, err := cfg.Scim.Patch(id, patch)
	if err != nil {
		if errors.Is(err, scim.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleScimDeleteUser removes a SCIM user (DELETE /scim/v2/Users/{id}).
func (cfg Config) handleScimDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if cfg.Scim == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SCIM store unavailable"})
		return
	}
	if err := cfg.Scim.Delete(id); err != nil {
		if errors.Is(err, scim.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
