package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/providers"
	"github.com/yatuk/tamga/internal/scanner"
)

// The MCP gateway: Tamga as a Streamable HTTP MCP proxy in front of the MCP
// servers an agent uses. Each server in TAMGA_MCP_CONFIG is served at
// /mcp/<name>; the agent is configured with that URL in place of the
// server's, and every message between the two passes the gateway.
//
// It follows the Streamable HTTP transport of the MCP specification,
// revision 2026-07-28 (the current one, read on 2026-10-18 at
// https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http),
// and the earlier shape of the same transport that revisions 2025-03-26 to
// 2025-11-25 use (https://modelcontextprotocol.io/specification/2025-11-25/basic/transports):
//
//   - POST carries one JSON-RPC message. The answer to a request is one JSON
//     object or an SSE stream of messages ending with the response; both are
//     read message by message (mcp_scan.go). A notification is answered
//     202 by the server, and the answer is passed on.
//   - GET opens the standalone SSE stream of the earlier revisions, and
//     DELETE ends their session. Both are forwarded, and a GET stream is read
//     like a POST stream. A 2026-07-28 server answers them 405, which is
//     passed on.
//   - Mcp-Session-Id, MCP-Protocol-Version, Last-Event-ID (the resumption of
//     the earlier revisions), Mcp-Method, Mcp-Name and Mcp-Param-* are
//     forwarded. Event ids are kept, so a resumed stream resumes.
//   - The initialize handshake of the earlier revisions and the per-request
//     _meta of 2026-07-28 pass as they are: the gateway takes no part in
//     version or capability negotiation.
//
// Authorisation: the agent authenticates to Tamga with its Tamga key, as on
// the proxy path. The server's credential is the operator's, sent from the
// environment (headers_from_env); the agent's own Authorization header is
// not passed on unless forward_authorization says so. See docs/mcp.md.

// mcpForwardHeaders are the request headers passed to the server.
var mcpForwardHeaders = []string{
	"Accept", "Content-Type", "MCP-Protocol-Version", "Mcp-Session-Id", "Last-Event-ID",
	"Mcp-Method", "Mcp-Name", "Traceparent", "Tracestate", "Baggage", "User-Agent",
}

// mcpKeyHeader is a second header the Tamga key is read from.
const mcpKeyHeader = "X-Api-Key"

// mcpReturnHeaders are the response headers passed back to the agent.
var mcpReturnHeaders = []string{"Mcp-Session-Id", "MCP-Protocol-Version", "Retry-After", "Allow"}

// mcpSessionTTL is how long the definitions a session listed are kept after
// its last tools/list, and mcpMaxSessions how many sessions are kept.
const (
	mcpSessionTTL       = 24 * time.Hour
	mcpMaxSessions      = 10000
	mcpMaxToolsRemember = 4096
)

type mcpGateway struct {
	scan    *scanHandler
	servers map[string]*MCPServerConfig
	origins map[string]bool
	client  *http.Client
	memory  *mcpToolMemory
}

// NewMCPGateway returns the handler for /mcp/<name>.
func NewMCPGateway(cfg HandlerConfig, gw *MCPGatewayConfig) http.Handler {
	g := &mcpGateway{
		// The scan path with the proxy path's own classifier guards: this is
		// real traffic, as on the proxy path.
		scan:    NewScanHandler(cfg, ScanOptions{Classifier: cfg.Classifier, Topics: cfg.Topics}).(*scanHandler),
		servers: map[string]*MCPServerConfig{},
		origins: map[string]bool{},
		client: &http.Client{
			Transport: upstreamTransportOrDefault(cfg),
			// A redirect would take the request, and the operator's
			// credential, somewhere the configuration does not name.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		memory: newMCPToolMemory(mcpMaxSessions, mcpSessionTTL),
	}
	if gw != nil {
		for i := range gw.Servers {
			g.servers[gw.Servers[i].Name] = &gw.Servers[i]
		}
		for _, o := range gw.AllowedOrigins {
			g.origins[o] = true
		}
	}
	return g
}

// RegisterMCPGateway mounts the gateway at /mcp/, behind the IP allowlist
// the proxy routes have.
func RegisterMCPGateway(mux *http.ServeMux, cfg HandlerConfig, gw *MCPGatewayConfig) {
	var h http.Handler = NewMCPGateway(cfg, gw)
	if cfg.Config != nil && cfg.Config.IPAllowlist != "" {
		h = NewIPAllowlistMiddleware(cfg.Config.IPAllowlist, cfg.Config.TrustedProxies)(h)
	}
	mux.Handle("/mcp/", h)
}

// mcpCall is one exchange through the gateway: a POST and what answered it,
// or one message of a GET stream. It is what the event is made from.
type mcpCall struct {
	start     time.Time
	requestID string
	server    *MCPServerConfig
	who       caller
	r         *http.Request
	session   string
	method    string
	tool      string
	logger    zerolog.Logger

	findings []scanner.Finding
	action   policy.Action
	fallback string
	// rewritten counts the texts STRIP or REDACT changed in what the
	// server sent.
	rewritten int
	scanned   time.Duration
}

// mcpActionRank orders the actions by how much they did to the traffic.
func mcpActionRank(a policy.Action) int {
	switch a {
	case policy.ActionBlock:
		return 5
	case policy.ActionStrip:
		return 4
	case policy.ActionRedact:
		return 3
	case policy.ActionWarn:
		return 2
	case policy.ActionLog:
		return 1
	}
	return 0
}

// add records findings and the action taken on them.
func (c *mcpCall) add(findings []scanner.Finding, action policy.Action, fallback string) {
	c.findings = append(c.findings, findings...)
	if mcpActionRank(action) > mcpActionRank(c.action) {
		c.action = action
	}
	if fallback != "" && c.fallback == "" {
		c.fallback = fallback
	}
	c.scanned = time.Since(c.start)
}

// errorData is the data of a refusal: the request id to look it up by, and
// the kinds of finding behind it.
func (c *mcpCall) errorData(v *mcpVerdict) map[string]any {
	data := map[string]any{"tamga_request_id": c.requestID}
	if v != nil {
		if kinds := mcpKinds(v.findings, c.who.Scope == apikeys.ScopeProxy); len(kinds) > 0 {
			data["kinds"] = kinds
		}
		if v.fallback != "" {
			data["reason"] = v.fallback
		}
	} else if c.fallback != "" {
		data["reason"] = c.fallback
	}
	return data
}

func (g *mcpGateway) newCall(r *http.Request, srv *MCPServerConfig, who caller) *mcpCall {
	id := uuid.Must(uuid.NewV7()).String()
	ident := "ip:" + clientIP(r, trustedProxies(g.scan.cfg))
	if who.verified() {
		ident = "key:" + who.KeyID
	}
	return &mcpCall{
		start:     time.Now(),
		requestID: id,
		server:    srv,
		who:       who,
		r:         r,
		session:   srv.Name + "\x00" + ident + "\x00" + r.Header.Get("Mcp-Session-Id"),
		action:    policy.ActionPass,
		logger:    log.With().Str("component", "mcp_gateway").Str("request_id", id).Str("mcp_server", srv.Name).Logger(),
	}
}

func writeMCPJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (g *mcpGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/mcp/"), "/")
	srv := g.servers[name]
	if srv == nil || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	// The transport requires a server to check Origin against DNS rebinding.
	if o := strings.ToLower(r.Header.Get("Origin")); (strings.HasPrefix(o, "http://") || strings.HasPrefix(o, "https://")) &&
		!g.origins[r.Header.Get("Origin")] {
		writeMCPJSON(w, http.StatusForbidden, mcpErrorBody(nil, mcpCodeUnauthorized, scanErrOrigin, nil))
		return
	}

	// A client that offers only a few header names (Claude's custom
	// connectors list authorization, x-api-key and x-auth-token) sends the
	// Tamga key in X-Api-Key. It is read as X-Tamga-Key is, which wins when
	// both are sent, and like it is never passed on.
	if k := strings.TrimSpace(r.Header.Get(mcpKeyHeader)); k != "" && strings.TrimSpace(r.Header.Get(keyHeader)) == "" {
		r.Header.Set(keyHeader, k)
	}
	r.Header.Del(mcpKeyHeader)
	who, errType, errMsg := authenticate(r, g.scan.cfg)
	if errType != "" {
		writeMCPJSON(w, http.StatusUnauthorized, mcpErrorBody(nil, mcpCodeUnauthorized, errMsg, map[string]any{"type": errType}))
		return
	}

	switch r.Method {
	case http.MethodPost:
		g.post(w, r, srv, who)
	case http.MethodGet, http.MethodDelete:
		g.passOn(w, r, srv, who)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// upstreamRequest builds the request to the server from the agent's.
func (g *mcpGateway) upstreamRequest(ctx context.Context, r *http.Request, srv *MCPServerConfig, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, r.Method, srv.target.String(), body)
	if err != nil {
		return nil, err
	}
	for _, h := range mcpForwardHeaders {
		if v := r.Header.Values(h); len(v) > 0 {
			req.Header[http.CanonicalHeaderKey(h)] = append([]string(nil), v...)
		}
	}
	for k, v := range r.Header {
		if strings.HasPrefix(strings.ToLower(k), "mcp-param-") {
			req.Header[k] = append([]string(nil), v...)
		}
	}
	if srv.ForwardAuthorization {
		if v := r.Header.Get("Authorization"); v != "" {
			req.Header.Set("Authorization", v)
		}
	}
	for k, v := range srv.headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// returnHeaders copies the headers of the server's answer the agent needs.
func returnHeaders(w http.ResponseWriter, resp *http.Response, srv *MCPServerConfig) {
	for _, h := range mcpReturnHeaders {
		if v := resp.Header.Values(h); len(v) > 0 {
			w.Header()[http.CanonicalHeaderKey(h)] = append([]string(nil), v...)
		}
	}
	if srv.ForwardAuthorization {
		if v := resp.Header.Values("WWW-Authenticate"); len(v) > 0 {
			w.Header()["Www-Authenticate"] = append([]string(nil), v...)
		}
	}
}

// post handles one JSON-RPC message from the agent.
func (g *mcpGateway) post(w http.ResponseWriter, r *http.Request, srv *MCPServerConfig, who caller) {
	c := g.newCall(r, srv, who)
	w.Header().Set("X-Tamga-Request-Id", c.requestID)
	pol := g.scan.cfg.GetPolicy()

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(srv.MaxMessageBytes)))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeMCPJSON(w, http.StatusRequestEntityTooLarge, mcpErrorBody(nil, mcpCodeTooLarge, mcpMsgTooLarge, nil))
			return
		}
		writeMCPJSON(w, http.StatusBadRequest, mcpErrorBody(nil, mcpCodeParse, mcpMsgParse, nil))
		return
	}
	// Bytes that are not UTF-8 and duplicate keys would let what Tamga reads
	// differ from what the server reads, as on the proxy path.
	if extract.CheckStrict(raw) != nil {
		writeMCPJSON(w, http.StatusBadRequest, mcpErrorBody(nil, mcpCodeParse, mcpMsgParse, nil))
		return
	}
	if t := bytes.TrimLeft(raw, " \t\r\n"); len(t) > 0 && t[0] == '[' {
		// Batches were part of 2025-03-26 only. Each message of one would
		// have to be judged and answered apart; the gateway refuses them.
		writeMCPJSON(w, http.StatusBadRequest, mcpErrorBody(nil, mcpCodeInvalidRequest, mcpMsgBatch, nil))
		return
	}
	v, ok := mcpDecode(raw)
	msg, isObj := v.(map[string]any)
	if !ok || !isObj {
		writeMCPJSON(w, http.StatusBadRequest, mcpErrorBody(nil, mcpCodeParse, mcpMsgParse, nil))
		return
	}
	id, hasID := msg["id"]
	c.method, _ = msg["method"].(string)
	if !mcpHeadersMatch(r.Header, msg) {
		writeMCPJSON(w, http.StatusBadRequest, mcpErrorBody(id, mcpCodeHeaderMismatch, mcpMsgHeaderMismatch, nil))
		return
	}

	body := raw
	if c.method == "tools/call" && !hasID {
		// A call sent as a notification would reach a lax server unjudged.
		writeMCPJSON(w, http.StatusBadRequest, mcpErrorBody(nil, mcpCodeInvalidRequest, mcpMsgCallNoID, nil))
		return
	}
	if c.method == "tools/call" {
		refusal, changed := g.judgeCall(r.Context(), c, pol, r.Header, msg)
		if refusal != nil {
			writeMCPJSON(w, http.StatusOK, mcpErrorBody(id, refusal.code, refusal.message, refusal.data))
			g.record(r.Context(), c, pol)
			return
		}
		if changed {
			body = mcpEncode(msg)
		}
	}

	req, err := g.upstreamRequest(r.Context(), r, srv, bytes.NewReader(body))
	if err == nil {
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/json")
	}
	var resp *http.Response
	if err == nil {
		resp, err = g.client.Do(req)
	}
	if err != nil {
		c.logger.Warn().Str("error_kind", "upstream").Msg("mcp server could not be reached")
		writeMCPJSON(w, http.StatusBadGateway, mcpErrorBody(id, mcpCodeUpstream, mcpMsgUpstream, map[string]any{"tamga_request_id": c.requestID}))
		g.record(r.Context(), c, pol)
		return
	}
	defer resp.Body.Close()
	g.relay(w, r, resp, c, pol, id, false)
	g.record(r.Context(), c, pol)
}

// passOn forwards a GET or a DELETE. A GET answered with a stream is read
// like a POST's stream, each message of it recorded on its own.
func (g *mcpGateway) passOn(w http.ResponseWriter, r *http.Request, srv *MCPServerConfig, who caller) {
	c := g.newCall(r, srv, who)
	pol := g.scan.cfg.GetPolicy()
	req, err := g.upstreamRequest(r.Context(), r, srv, http.NoBody)
	var resp *http.Response
	if err == nil {
		resp, err = g.client.Do(req)
	}
	if err != nil {
		writeMCPJSON(w, http.StatusBadGateway, mcpErrorBody(nil, mcpCodeUpstream, mcpMsgUpstream, nil))
		return
	}
	defer resp.Body.Close()
	g.relay(w, r, resp, c, pol, nil, true)
}

// relay passes the server's answer to the agent. perMessage records each
// message of a stream as its own exchange (a GET stream, which answers no
// one request).
func (g *mcpGateway) relay(w http.ResponseWriter, r *http.Request, resp *http.Response, c *mcpCall, pol *policy.Policy, id any, perMessage bool) {
	returnHeaders(w, resp, c.server)
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	limit := c.server.MaxMessageBytes

	switch mt {
	case "text/event-stream":
		g.relayStream(w, r, resp, c, pol, perMessage)
	case "application/json":
		raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
		switch {
		case err != nil:
			writeMCPJSON(w, http.StatusBadGateway, mcpErrorBody(id, mcpCodeUpstream, mcpMsgUpstream, nil))
		case len(raw) > limit:
			writeMCPJSON(w, http.StatusBadGateway, mcpErrorBody(id, mcpCodeTooLarge, mcpMsgTooLarge, nil))
		case resp.StatusCode >= 400 && !mcpIsObject(raw):
			// An error status with an empty body, or one that is not a
			// message: a client tells the revisions apart by such answers
			// (2026-07-28, transports/streamable-http, backward
			// compatibility), so it is passed on as it is.
			writeMCPJSON(w, resp.StatusCode, raw)
		default:
			out, keep := g.judgeInbound(r.Context(), c, pol, raw)
			if !keep {
				writeMCPJSON(w, http.StatusBadGateway, mcpErrorBody(id, mcpCodeUpstream, mcpMsgUpstream, nil))
				return
			}
			writeMCPJSON(w, resp.StatusCode, out)
		}
	default:
		// A 202 for a notification, a 405 for a GET, an error page. A body
		// that is not a message is passed on only when it is small: it is
		// not read by anything that hands it to a model.
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(resp.Body, 64<<10))
	}
}

// mcpIsObject reports whether raw is one JSON object.
func mcpIsObject(raw []byte) bool {
	v, ok := mcpDecode(raw)
	_, obj := v.(map[string]any)
	return ok && obj
}

// relayStream passes an SSE stream on event by event.
func (g *mcpGateway) relayStream(w http.ResponseWriter, r *http.Request, resp *http.Response, c *mcpCall, pol *policy.Policy, perMessage bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	// A stream outlives the server's write timeout, which is set for
	// answers that come in one piece.
	_ = rc.SetWriteDeadline(time.Time{})
	w.WriteHeader(resp.StatusCode)
	_ = rc.Flush()

	reader := newMCPSSEReader(resp.Body, c.server.MaxMessageBytes, func(line []byte) error {
		if _, err := w.Write(append(append([]byte(nil), line...), '\n', '\n')); err != nil {
			return err
		}
		return rc.Flush()
	})
	for {
		ev, err := reader.next()
		if err != nil {
			if !errors.Is(err, io.EOF) && r.Context().Err() == nil {
				c.logger.Warn().Bool("too_large", errors.Is(err, errMCPSSELarge)).Msg("mcp stream ended early")
			}
			return
		}
		data := ev.data
		if ev.message() && ev.hasData && len(bytes.TrimSpace(ev.data)) > 0 {
			mc := c
			if perMessage {
				mc = g.newCall(r, c.server, c.who)
			}
			out, keep := g.judgeInbound(r.Context(), mc, pol, ev.data)
			if perMessage && (len(mc.findings) > 0 || mc.action != policy.ActionPass) {
				g.record(r.Context(), mc, pol)
			}
			if !keep {
				continue
			}
			data = out
		}
		if err := writeMCPSSEEvent(w, ev, data); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

// mcpHeadersMatch checks Mcp-Method and Mcp-Name against the body, as a
// server that reads the body must (2026-07-28, transports/streamable-http,
// server validation). The gateway decides on the body; a header that says
// something else would let another hop decide on something else.
func mcpHeadersMatch(h http.Header, msg map[string]any) bool {
	method, _ := msg["method"].(string)
	if v := h.Get("Mcp-Method"); v != "" && v != method {
		return false
	}
	v := h.Get("Mcp-Name")
	if v == "" {
		return true
	}
	if inner, ok := strings.CutPrefix(v, "=?base64?"); ok && strings.HasSuffix(inner, "?=") {
		dec, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(inner, "?="))
		if err != nil {
			return false
		}
		v = string(dec)
	}
	params, _ := msg["params"].(map[string]any)
	name, _ := params["name"].(string)
	if name == "" {
		name, _ = params["uri"].(string)
	}
	return v == name
}

// record publishes the exchange as a request's own event, as the LiteLLM
// route does (litellmHandler.record): the request log, the live feed and the
// alerts take it from there. No text is on it: the findings carry the
// matches the scanners masked, the endpoint names the server, the method
// and the tool. A tools/call is always recorded; any other exchange only
// when something was found or done.
func (g *mcpGateway) record(ctx context.Context, c *mcpCall, pol *policy.Policy) {
	// The agent may be gone by now; what it was sent is still recorded.
	ctx = context.WithoutCancel(ctx)
	ev := c.logger.Info()
	if c.action == policy.ActionBlock {
		ev = c.logger.Warn()
	}
	ev.Str("method", litellmModel(c.method)).Str("tool", litellmModel(c.tool)).Str("action", string(c.action)).
		Int("findings", len(c.findings)).Int("rewritten", c.rewritten).Str("key_id", c.who.KeyID).
		Float64("latency_ms", durationMs(time.Since(c.start))).Msg("mcp gateway exchange")

	cfg := g.scan.cfg
	if cfg.Bus == nil || (c.method != "tools/call" && len(c.findings) == 0 && c.action == policy.ActionPass) {
		return
	}
	findings := append([]scanner.Finding(nil), c.findings...)
	applyFindingActionTaken(findings, c.action)
	risk := scanner.CalculateRisk(findings)
	none := scanner.RiskScore{Level: "none", Breakdown: map[string]float64{}}
	endpoint := "/mcp/" + c.server.Name
	if m := litellmModel(c.method); m != "" {
		endpoint += " " + m
	}
	if t := litellmModel(c.tool); t != "" {
		endpoint += " " + t
	}
	e := events.Event{
		RequestID:      c.requestID,
		OrgID:          c.r.Header.Get("X-Tamga-Org-Id"),
		OrgVerified:    c.who.verified(),
		Provider:       providers.MCPGateway,
		EventType:      "request_scanned",
		Findings:       findings,
		Action:         string(c.action),
		Endpoint:       endpoint,
		UserID:         c.r.Header.Get("X-Tamga-User-Id"),
		ScanLatencyMs:  durationMs(c.scanned),
		TotalLatencyMs: durationMs(time.Since(c.start)),
		Timestamp:      time.Now().UTC(),
		InputRisk:      risk,
		OutputRisk:     none,
	}
	if pol != nil {
		e.PolicyName, e.PolicyVersion = pol.Name, pol.Version
	}
	if c.action == policy.ActionBlock {
		e.EventType = "request_blocked"
	}
	cfg.Bus.PublishContext(ctx, e)
	if c.action != policy.ActionPass {
		notifyRuleWebhooks(cfg, pol, c.r, c.requestID, providers.MCPGateway, "", findings, c.action, risk)
	}
}

// mcpToolMemory keeps, for each session, the definition each tool had when
// it was first listed. A session is the server, the caller (its Tamga key,
// or its address without one) and the Mcp-Session-Id of the earlier
// revisions; a 2026-07-28 client has no session id, and its session is the
// caller's use of the server.
type mcpToolMemory struct {
	mu       sync.Mutex
	sessions map[string]*mcpSessionTools
	max      int
	ttl      time.Duration
	now      func() time.Time
}

type mcpSessionTools struct {
	tools map[string]*mcpToolSeen
	used  time.Time
}

type mcpToolSeen struct {
	print   [32]byte
	changed bool
}

func newMCPToolMemory(max int, ttl time.Duration) *mcpToolMemory {
	return &mcpToolMemory{sessions: map[string]*mcpSessionTools{}, max: max, ttl: ttl, now: time.Now}
}

// observe records a listed definition and reports whether it differs from
// the one first listed under that name in the session. The first stays the
// reference: a definition that changes and changes back is flagged once.
func (m *mcpToolMemory) observe(session, tool string, print [32]byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	s := m.sessions[session]
	if s == nil || now.Sub(s.used) > m.ttl {
		m.makeRoom(now)
		s = &mcpSessionTools{tools: map[string]*mcpToolSeen{}}
		m.sessions[session] = s
	}
	s.used = now
	seen := s.tools[tool]
	if seen == nil {
		if len(s.tools) < mcpMaxToolsRemember {
			s.tools[tool] = &mcpToolSeen{print: print}
		}
		return false
	}
	if seen.print != print {
		seen.changed = true
		return true
	}
	return false
}

// changed reports whether the tool's definition changed in the session.
func (m *mcpToolMemory) changed(session, tool string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[session]
	if s == nil || m.now().Sub(s.used) > m.ttl {
		return false
	}
	seen := s.tools[tool]
	return seen != nil && seen.changed
}

// makeRoom drops expired sessions when the memory is full, and then the one
// used longest ago.
func (m *mcpToolMemory) makeRoom(now time.Time) {
	if len(m.sessions) < m.max {
		return
	}
	oldest, oldestKey := now, ""
	for k, s := range m.sessions {
		if now.Sub(s.used) > m.ttl {
			delete(m.sessions, k)
			continue
		}
		if s.used.Before(oldest) {
			oldest, oldestKey = s.used, k
		}
	}
	if len(m.sessions) >= m.max && oldestKey != "" {
		delete(m.sessions, oldestKey)
	}
}
