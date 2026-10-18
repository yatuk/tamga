package proxy

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// keyPathPrefix starts a path that carries a Tamga key: /k/<key>/v1/...
const keyPathPrefix = "/k/"

// maxPathKeyLen bounds the segment that is taken for a key. Tamga keys are 51
// bytes; this only keeps an absurd segment out of a hash.
const maxPathKeyLen = 256

// keyPathRoutes are the provider routes a keyed path may lead to, with the
// provider and the prefix handleProxy strips. Nothing else is reachable
// through /k/: not the management API, not the scan API.
var keyPathRoutes = []struct{ prefix, provider, strip string }{
	{"/v1", "openai", ""},
	{"/openai", "openai", "/openai"},
	{"/anthropic", "anthropic", "/anthropic"},
	{"/gemini", "gemini", "/gemini"},
	{"/azure", "azure", "/azure"},
	{"/bedrock", "bedrock", "/bedrock"},
	{"/mistral", "mistral", "/mistral"},
	{"/local", "local", "/local"},
}

// handleKeyInPath serves /k/<key>/<route>/... for clients that can set a base
// URL and an API key but no header (Cline, Roo Code, JetBrains AI Assistant,
// Zed). The key is cut out of the path and moved to the X-Tamga-Key header,
// and the request is handed to the same handler as the unkeyed route, so no
// later reader sees the key in the path: not the span, the cache key, the
// event's endpoint, the upstream URL or a log line. The original request
// (and its RequestURI) is dropped here and never logged.
//
// A request that also sends X-Tamga-Key with another value is refused: it
// would be a client with two identities.
func handleKeyInPath(w http.ResponseWriter, r *http.Request, cfg HandlerConfig) {
	key, rest, ok := splitKeyPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var provider, strip string
	for _, route := range keyPathRoutes {
		if rest == route.prefix || strings.HasPrefix(rest, route.prefix+"/") {
			provider, strip = route.provider, route.strip
			break
		}
	}
	if provider == "" {
		http.NotFound(w, r)
		return
	}

	if h := strings.TrimSpace(r.Header.Get(keyHeader)); h != "" &&
		subtle.ConstantTimeCompare([]byte(h), []byte(key)) != 1 {
		requestID := uuid.Must(uuid.NewV7()).String()
		log.Warn().Str("component", "proxy").Str("request_id", requestID).
			Str("event", "tamga_key_conflict").
			Str("client_ip", clientIP(r, trustedProxies(cfg))).
			Msg("proxy request refused: key in path and in header differ")
		writePolicyError(w, requestID, http.StatusUnauthorized, "tamga_key_conflict",
			"The Tamga key in the path and the X-Tamga-Key header differ")
		return
	}

	r2 := r.Clone(r.Context())
	r2.URL.Path = rest
	r2.URL.RawPath = ""
	r2.RequestURI = rest
	if r2.URL.RawQuery != "" {
		r2.RequestURI += "?" + r2.URL.RawQuery
	}
	r2.Header.Set(keyHeader, key)
	handleProxy(w, r2, provider, strip, cfg)
}

// splitKeyPath cuts "/k/<key>/<rest>" into the key and "/<rest>". The key is
// one non-empty segment.
func splitKeyPath(p string) (key, rest string, ok bool) {
	p, found := strings.CutPrefix(p, keyPathPrefix)
	if !found {
		return "", "", false
	}
	i := strings.IndexByte(p, '/')
	if i <= 0 || i > maxPathKeyLen {
		return "", "", false
	}
	return p[:i], p[i:], true
}
