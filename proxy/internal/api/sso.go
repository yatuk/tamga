package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/yatuk/tamga/internal/store"
)

// ssoSettingsSafe returns the SSO store or a no-op fallback to avoid nil
// pointer panics when the operator hasn't wired an SSO store.
func (cfg Config) ssoSettingsSafe() store.SSOSettingsStore {
	if cfg.SSOSettings != nil {
		return cfg.SSOSettings
	}
	return store.NewNoopSSOSettingsStore()
}

// handleSSOGet returns the SSO configuration for the default organisation.
func (cfg Config) handleSSOGet(w http.ResponseWriter, r *http.Request) {
	orgID := cfg.DefaultOrgID
	if orgID == "" {
		orgID = "default"
	}
	st := cfg.ssoSettingsSafe()
	s := st.Get(orgID)
	writeJSON(w, http.StatusOK, s)
}

// handleSSOPut updates the SSO configuration for the default organisation.
// It validates the request body before persisting.
func (cfg Config) handleSSOPut(w http.ResponseWriter, r *http.Request) {
	orgID := cfg.DefaultOrgID
	if orgID == "" {
		orgID = "default"
	}
	var body store.SSOSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Normalise strings.
	body.ProviderType = strings.ToLower(strings.TrimSpace(body.ProviderType))
	body.MetadataURL = strings.TrimSpace(body.MetadataURL)
	body.Domain = strings.ToLower(strings.TrimSpace(body.Domain))

	// --- Validation ---
	errs := make([]map[string]string, 0)

	validTypes := map[string]bool{"saml": true, "oidc": true, "": true}
	if !validTypes[body.ProviderType] {
		errs = append(errs, map[string]string{
			"field":   "provider_type",
			"message": "must be 'saml' or 'oidc'",
		})
	}

	if body.MetadataURL != "" {
		if u, err := url.Parse(body.MetadataURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, map[string]string{
				"field":   "metadata_url",
				"message": "must be a valid URL with http or https scheme and a host",
			})
		}
	}

	if body.Domain != "" {
		if !strings.Contains(body.Domain, ".") || strings.Contains(body.Domain, "://") {
			errs = append(errs, map[string]string{
				"field":   "domain",
				"message": "must be a valid domain (e.g. example.com) without protocol or path",
			})
		}
	}

	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
			"error":   "validation failed",
			"details": errs,
		})
		return
	}

	// Ensure AttributeMapping is never nil.
	if body.AttributeMapping == nil {
		body.AttributeMapping = map[string]string{"email": "email", "name": "displayName"}
	}

	st := cfg.ssoSettingsSafe()
	st.Set(orgID, body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"config": body,
	})
}
