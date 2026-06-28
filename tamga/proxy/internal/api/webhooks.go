package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/webhooks"
)

func (cfg Config) handleWebhookList(w http.ResponseWriter, _ *http.Request) {
	if cfg.Webhooks == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []webhooks.Webhook{}, "total": 0})
		return
	}
	items := cfg.Webhooks.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

func (cfg Config) handleWebhookCreate(w http.ResponseWriter, r *http.Request) {
	if cfg.Webhooks == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "webhooks unavailable"})
		return
	}
	defer func() { _ = r.Body.Close() }()
	var body webhooks.Webhook
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := cfg.Webhooks.Create(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "webhook.create",
			Target: created.ID,
			Detail: map[string]interface{}{"kind": string(created.Kind), "label": created.Label},
		})
	}
	writeJSON(w, http.StatusCreated, created)
}

func (cfg Config) handleWebhookUpdate(w http.ResponseWriter, r *http.Request) {
	if cfg.Webhooks == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "webhooks unavailable"})
		return
	}
	id := r.PathValue("id")
	defer func() { _ = r.Body.Close() }()
	var body webhooks.Webhook
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := cfg.Webhooks.Update(id, body)
	if err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "webhook.update",
			Target: updated.ID,
			Detail: map[string]interface{}{"kind": string(updated.Kind), "label": updated.Label},
		})
	}
	writeJSON(w, http.StatusOK, updated)
}

func (cfg Config) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if cfg.Webhooks == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "webhooks unavailable"})
		return
	}
	id := r.PathValue("id")
	status, err := cfg.Webhooks.Test(r.Context(), id)
	if err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status_code": status, "ok": status >= 200 && status < 300})
}

func (cfg Config) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	if cfg.Webhooks == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "webhooks unavailable"})
		return
	}
	id := r.PathValue("id")
	if err := cfg.Webhooks.Delete(id); err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{Kind: "webhook.delete", Target: id})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
