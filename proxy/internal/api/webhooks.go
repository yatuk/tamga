package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/yatuk/tamga/internal/incidents"
	"github.com/yatuk/tamga/internal/webhooks"
)

// redactedValue stands where a list response leaves a credential out.
const redactedValue = "[redacted]"

// listedWebhook is a destination as the list shows it: what it is and where
// it goes, without what would let someone else post to it. The token is
// left out, header values are replaced, and the URL is cut to its scheme and
// host, because the path or query of a webhook URL is usually the secret.
type listedWebhook struct {
	webhooks.Webhook
	HasAuthToken bool `json:"has_auth_token"`
	URLRedacted  bool `json:"url_redacted"`
}

func listWebhook(wh webhooks.Webhook) listedWebhook {
	out := listedWebhook{Webhook: wh, HasAuthToken: wh.AuthToken != ""}
	out.AuthToken = ""
	if len(wh.Headers) > 0 {
		out.Headers = make(map[string]string, len(wh.Headers))
		for name := range wh.Headers {
			out.Headers[name] = redactedValue
		}
	}
	out.URL, out.URLRedacted = redactWebhookURL(wh.URL)
	return out
}

// redactWebhookURL keeps the scheme and host of a destination. A URL that
// does not parse is not shown at all.
func redactWebhookURL(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return redactedValue, true
	}
	if (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.User == nil && u.Fragment == "" {
		return raw, false
	}
	return u.Scheme + "://" + u.Host + "/" + redactedValue, true
}

func (cfg Config) handleWebhookList(w http.ResponseWriter, _ *http.Request) {
	items := []listedWebhook{}
	if cfg.Webhooks != nil {
		for _, wh := range cfg.Webhooks.List() {
			items = append(items, listWebhook(wh))
		}
	}
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
		writeStoreError(w, err)
		return
	}
	created, err := cfg.Webhooks.Create(body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if cfg.Audit != nil {
		cfg.Audit.Append(incidents.AuditEntry{
			Kind:   "webhook.create",
			Target: created.ID,
			Actor:  actorFromRequest(r),
			Detail: map[string]interface{}{"kind": string(created.Kind), "label": created.Label},
		})
	}
	writeJSON(w, http.StatusCreated, created)
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
		// The guard's refusal is the destination's fault, not the
		// receiver's, and it is one fixed sentence: see MemoryStore.Test.
		if errors.Is(err, webhooks.ErrDestinationNotAllowed) || errors.Is(err, webhooks.ErrInsecureScheme) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
		cfg.Audit.Append(incidents.AuditEntry{Kind: "webhook.delete", Target: id, Actor: actorFromRequest(r)})
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
