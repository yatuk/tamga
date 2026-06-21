package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/users"
)

func (cfg Config) handleTeamList(w http.ResponseWriter, r *http.Request) {
	if cfg.Users == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []users.Member{}, "clerk": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stored := cfg.Users.List()
	roleByID := make(map[string]users.Member, len(stored))
	for _, m := range stored {
		roleByID[m.UserID] = m
	}

	items := make([]users.Member, 0, len(stored))
	clerkOK := false
	if cfg.Clerk != nil {
		clerkUsers, err := cfg.Clerk.ListUsers(ctx, 200)
		if err == nil {
			clerkOK = true
			seen := map[string]bool{}
			for _, cu := range clerkUsers {
				m := users.Member{
					UserID:   cu.ID,
					Email:    cu.Email(),
					Name:     cu.Name(),
					ImageURL: cu.ImageURL,
					Role:     users.RoleViewer,
				}
				if existing, ok := roleByID[cu.ID]; ok {
					m.Role = existing.Role
					m.UpdatedAt = existing.UpdatedAt
				}
				items = append(items, m)
				seen[cu.ID] = true
			}
			for _, m := range stored {
				if !seen[m.UserID] {
					items = append(items, m)
				}
			}
		}
	}
	if !clerkOK {
		items = stored
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"total": len(items),
		"clerk": clerkOK,
	})
}

func (cfg Config) handleTeamRolePut(w http.ResponseWriter, r *http.Request) {
	if cfg.Users == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "users store unavailable"})
		return
	}
	id := r.PathValue("id")
	defer func() { _ = r.Body.Close() }()
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	m, err := cfg.Users.Set(id, body.Role)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "team.role",
			Target: m.UserID,
			Detail: map[string]interface{}{"role": m.Role},
		})
	}
	writeJSON(w, http.StatusOK, m)
}
