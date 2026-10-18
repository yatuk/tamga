package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/policy"
)

// provider_accounts on the proxy path (policy/provider_accounts.go): the
// check of the account a request uses, and the replacement of the caller's
// credential with the organisation's own key.
//
// Replacement takes the caller's credentials off the request before anything
// reads it, so the response cache names the request by its verified Tamga
// key and not by the organisation's key, which every caller shares
// (cache.Identity). The organisation's key is put on in the Director, on the
// request that goes out. An internal pool (ROUTE) still takes it off again:
// what goes to the organisation's own servers carries no provider key.

// providerAccountCode is the error code of a request the check refused.
const providerAccountCode = "provider_account_not_allowed"

// accountCredentialHeaders are the request headers a provider reads a key
// from, as cache.credentialHeaders lists them.
var accountCredentialHeaders = []string{"Authorization", "X-Api-Key", "Api-Key", "X-Goog-Api-Key"}

// accountKeyQueryParam carries the key on Gemini-style URLs.
const accountKeyQueryParam = "key"

// accountRequestOf collects what the check reads from a request.
func accountRequestOf(r *http.Request) policy.AccountRequest {
	var req policy.AccountRequest
	for _, name := range accountCredentialHeaders {
		for _, v := range r.Header.Values(name) {
			v = strings.TrimSpace(v)
			if name == "Authorization" && len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
				v = strings.TrimSpace(v[7:])
			}
			if v != "" {
				req.Credentials = append(req.Credentials, v)
			}
		}
	}
	if r.URL != nil {
		for _, v := range r.URL.Query()[accountKeyQueryParam] {
			if v = strings.TrimSpace(v); v != "" {
				req.Credentials = append(req.Credentials, v)
			}
		}
	}
	req.Organizations = r.Header.Values(policy.OpenAIOrganizationHeader)
	req.Projects = r.Header.Values(policy.OpenAIProjectHeader)
	req.Workspaces = r.Header.Values(policy.AnthropicWorkspaceHeader)
	return req
}

// dropCallerCredentials takes every provider credential off the request,
// the "key" query parameter included.
func dropCallerCredentials(r *http.Request) {
	for _, name := range accountCredentialHeaders {
		r.Header.Del(name)
	}
	if r.URL != nil && r.URL.RawQuery != "" {
		q := r.URL.Query()
		if _, ok := q[accountKeyQueryParam]; ok {
			q.Del(accountKeyQueryParam)
			r.URL.RawQuery = q.Encode()
		}
	}
}

// setProviderCredential puts the organisation's key on an outgoing request,
// in the header the route's provider reads it from.
func setProviderCredential(req *http.Request, provider, key string) {
	for _, name := range accountCredentialHeaders {
		req.Header.Del(name)
	}
	switch provider {
	case "anthropic":
		req.Header.Set("X-Api-Key", key)
	case "gemini":
		req.Header.Set("X-Goog-Api-Key", key)
	case "azure":
		req.Header.Set("Api-Key", key)
	default: // openai, mistral, local
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

// applyProviderAccounts runs the route's provider_accounts entry on a
// request. It returns the key to send in place of the caller's (empty when
// the route does not replace it), and, when it has answered the request
// itself, the error code it answered with.
func applyProviderAccounts(w http.ResponseWriter, r *http.Request, pol *policy.Policy, provider string, who caller, requestID string, logger zerolog.Logger) (key, refused string) {
	rule := pol.ProviderAccountRule(provider)
	if rule == nil {
		return "", ""
	}
	if why := rule.Check(accountRequestOf(r)); why != "" {
		ev := logger.Warn().Str("event", providerAccountCode).Str("reason", why)
		if rule.Log {
			ev.Msg("provider account not on the list: forwarded (LOG)")
		} else {
			ev.Msg("provider account not on the list: blocked")
			writeProviderAccountRefusal(w, requestID, why)
			return "", providerAccountCode
		}
	}
	if !rule.Replaces() {
		return "", ""
	}
	if !who.verified() {
		logger.Warn().Str("event", "tamga_key_required").Msg("route sends the organisation's provider key: request without a Tamga key refused")
		writePolicyError(w, requestID, http.StatusUnauthorized, "tamga_key_required",
			"This route sends the organisation's provider key: send a Tamga key in the X-Tamga-Key header")
		return "", "tamga_key_required"
	}
	// The variable is judged again here, as a pool endpoint's is.
	if policy.ActiveUpstreamRules().KeyEnvAllowed(rule.ReplaceKeyEnv) {
		key = strings.TrimSpace(os.Getenv(rule.ReplaceKeyEnv))
	}
	if key == "" {
		// The caller's own key is not a fallback.
		logger.Error().Str("event", "provider_key_unavailable").Str("env", rule.ReplaceKeyEnv).Msg("organisation's provider key is not set")
		writePolicyError(w, requestID, http.StatusServiceUnavailable, "provider_key_unavailable",
			"The organisation's provider key for this route is not configured on the proxy. The request was not sent.")
		return "", "provider_key_unavailable"
	}
	dropCallerCredentials(r)
	return key, ""
}

// writeProviderAccountRefusal answers a request whose account is not on the
// list: 403, with the reason.
func writeProviderAccountRefusal(w http.ResponseWriter, requestID, reason string) {
	msg := "The provider account this request uses is not one the organisation allows. Use the organisation's key and account."
	switch reason {
	case policy.AccountOrganizationMissing:
		msg = "This route requires the OpenAI-Organization header, set to one of the organisation's allowed organizations."
	case policy.AccountProjectMissing:
		msg = "This route requires the OpenAI-Project header, set to one of the organisation's allowed projects."
	case policy.AccountWorkspaceMissing:
		msg = "This route requires the anthropic-workspace-id header, set to one of the organisation's allowed workspaces."
	case policy.AccountKeyMissing:
		msg = "This route requires a provider key the organisation allows."
	}
	// The shape of writePolicyError's, with the reason added.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":    msg,
			"type":       providerAccountCode,
			"reason":     reason,
			"request_id": requestID,
		},
	})
}
