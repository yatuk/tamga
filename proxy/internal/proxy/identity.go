package proxy

import (
	"net/http"
	"strings"
)

// keyHeader carries a Tamga key on the proxy path. It is separate from
// Authorization and X-API-Key, which belong to the provider.
const keyHeader = "X-Tamga-Key"

// caller is who a proxy request is attributed to.
type caller struct {
	// KeyID is set when the request carried a key that verified. Org and
	// role then come from the key's record, not from anything the request
	// says about itself.
	KeyID string
	Role  string
}

func (c caller) verified() bool { return c.KeyID != "" }

// authenticate resolves the caller and rewrites the request's identity
// headers to match, so every later reader (budget, cache partition, events,
// policy exceptions) sees the same, verified values.
//
// With a valid key: X-Tamga-Org-Id becomes the key's organisation (or the
// deployment default), X-Tamga-Role is dropped in favour of the key's role,
// and X-Tamga-User-Id becomes the key's user when it names one. A key issued
// to a service that acts for many users names none, and then the service's
// own X-Tamga-User-Id is kept as its attribution of the request.
//
// Without a key the request is anonymous: refused when TAMGA_REQUIRE_KEY is
// on, otherwise handled as before, with its headers taken as sent.
//
// A non-empty errType means the request must be answered 401.
func authenticate(r *http.Request, cfg HandlerConfig) (c caller, errType, errMsg string) {
	raw := strings.TrimSpace(r.Header.Get(keyHeader))
	// Never forwarded, never logged.
	r.Header.Del(keyHeader)

	if raw == "" {
		if cfg.Config != nil && cfg.Config.RequireKey {
			return caller{}, "tamga_key_required", "This Tamga proxy requires a key: send it in the X-Tamga-Key header"
		}
		return caller{}, "", ""
	}
	if cfg.Keys == nil {
		return caller{}, "tamga_key_invalid", "The X-Tamga-Key is not valid"
	}
	k, ok := cfg.Keys.Verify(raw)
	if !ok {
		// One message for unknown, revoked and expired: which of them it is
		// is not the caller's to learn.
		return caller{}, "tamga_key_invalid", "The X-Tamga-Key is not valid"
	}

	org := k.OrgID
	if org == "" && cfg.Config != nil {
		org = cfg.Config.DefaultOrgID
	}
	if org != "" {
		r.Header.Set("X-Tamga-Org-Id", org)
	} else {
		r.Header.Del("X-Tamga-Org-Id")
	}
	r.Header.Del("X-Tamga-Role")
	if k.UserID != "" {
		r.Header.Set("X-Tamga-User-Id", k.UserID)
	}
	return caller{KeyID: k.ID, Role: k.Role}, "", ""
}
