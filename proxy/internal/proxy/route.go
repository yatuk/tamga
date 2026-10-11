package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/upstream"
)

// The ROUTE action on the proxy path. What the policy may say is in
// internal/policy/route.go; this is what happens to the request.
//
//   - It is sent to the internal pool as written. Nothing is masked: the
//     point of the action is that the organisation's own model may read what
//     an outside provider may not. A finding whose own rule says REDACT is
//     therefore left as it is.
//   - A finding whose own rule says STRIP is still stripped. STRIP is for
//     text that is an attack on the model (an injection in a tool result),
//     not for text that is merely confidential, and the internal model is as
//     open to it as any other. If the strip cannot be carried out the
//     request is blocked, as it is without ROUTE.
//   - Nothing of it goes to the provider the caller addressed, in any case.
//     When the pool cannot take the request (it is not there, it speaks
//     another dialect than the caller's route, none of its endpoints
//     answered before the answer began) the request is blocked with
//     tamga_route_unavailable. It is not retried against the provider.
//   - The caller's provider credential is not sent to the pool; the pool's
//     endpoints use their own (upstream.cloneRequestForEndpoint).
//   - The answer is scanned by output_rules like any other, and the response
//     cache keys on the pool (cache.KeyParts.Route).

// routedToHeader names the internal pool that answered, on the response.
const routedToHeader = "X-Tamga-Routed-To"

// routeUnavailableCode is the error code of a request a ROUTE verdict could
// not send to its pool.
const routeUnavailableCode = "tamga_route_unavailable"

// routePoolFor returns the pool a ROUTE verdict sends a request on this
// route to, or the reason there is none (one of upstream.Route*).
func routePoolFor(cfg HandlerConfig, provider, name string) (*upstream.ProviderPool, string) {
	if cfg.UpstreamRegistry == nil || name == "" {
		return nil, upstream.RouteNoPool
	}
	pool := cfg.UpstreamRegistry.Pool(name)
	// Internal is asked again here, whatever validation let through: a pool
	// that is not internal is an outside provider.
	if pool == nil || !pool.Internal() {
		return nil, upstream.RouteNoPool
	}
	if d := policy.RouteDialect(provider); d == "" || d != pool.Dialect() {
		return nil, upstream.RouteDialect
	}
	return pool, ""
}

// routeUnavailableBody is the body of that refusal, in the shape of
// writeScanUnavailable's.
func routeUnavailableBody(requestID, reason string) []byte {
	msg := "The policy routes this request to an internal model, and that model could not be reached. The request was not sent to the provider."
	if reason == upstream.RouteDialect {
		msg = "The policy routes this request to an internal model that does not speak the API of the route it was sent on. The request was not sent to the provider."
	}
	b, _ := json.Marshal(map[string]interface{}{
		"error": map[string]interface{}{
			"code":       routeUnavailableCode,
			"message":    msg,
			"type":       "service_unavailable",
			"reason":     reason,
			"request_id": requestID,
		},
	})
	return append(b, '\n')
}

// writeRouteUnavailable refuses a request whose ROUTE could not be carried
// out. It is a 503, like the other refusals that say nothing is wrong with
// the request itself. Retry-After is set when sending it again can end
// differently: an endpoint may be back, a dialect does not change.
func writeRouteUnavailable(w http.ResponseWriter, requestID, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tamga-Request-Id", requestID)
	if reason == upstream.RouteUpstream {
		w.Header().Set("Retry-After", "5")
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write(routeUnavailableBody(requestID, reason))
}

// stripOnly limits a per-finding action to STRIP: under ROUTE the findings
// to strip are stripped and nothing is masked.
func stripOnly(actionOf func(f scanner.Finding) policy.Action) func(f scanner.Finding) policy.Action {
	return func(f scanner.Finding) policy.Action {
		if actionOf(f) == policy.ActionStrip {
			return policy.ActionStrip
		}
		return policy.ActionPass
	}
}

// rewriteModel replaces the model a request names with the internal pool's.
// Only the value of a top-level "model" member that is a string is changed;
// every other byte of the body is kept as it was sent. It returns the body
// unchanged when it is not a JSON object or names no model: the internal
// server then decides what to do with it.
func rewriteModel(body []byte, model string) []byte {
	if model == "" {
		return body
	}
	repl, err := json.Marshal(model)
	if err != nil {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return body
	}
	type span struct{ from, to int }
	var spans []span
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return body
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return body
		}
		if k, ok := key.(string); ok && k == "model" && len(val) > 0 && val[0] == '"' {
			end := int(dec.InputOffset())
			spans = append(spans, span{end - len(val), end})
		}
	}
	// A body cut short is not rewritten. More reports false at the end of the
	// input as it does at the closing brace, so the brace itself is asked for.
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return body
	}
	if len(spans) == 0 {
		return body
	}
	out := make([]byte, 0, len(body)+len(spans)*len(repl))
	at := 0
	for _, s := range spans {
		// The value's bytes, where the decoder says they are.
		if s.from < at || s.to > len(body) || body[s.from] != '"' {
			return body
		}
		out = append(out, body[at:s.from]...)
		out = append(out, repl...)
		at = s.to
	}
	return append(out, body[at:]...)
}
