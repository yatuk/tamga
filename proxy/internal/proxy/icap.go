package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/icap"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/webhooks"
)

// The ICAP route: a company's web proxy (Squid, or the ICAP client of a
// secure web gateway) that already terminates its users' TLS asks Tamga
// about a request on its way to an AI service, and does what the reply
// says: send it on as it was, send it on changed, or answer the user with
// the response Tamga gives in its place. The protocol is internal/icap;
// this is what Tamga decides.
//
// Like the LiteLLM route it is an adapter on the scan path: a request it
// recognises is scanned and judged by scanHandler.evaluateInput, with the
// proxy path's registry, policy, classifier and banned topics, and its
// verdict is recorded as a request on the proxy path is (see record). What
// it does not do is what the proxy path does around the scan: no rate
// limit, no budget or token quota, no cache, no canary, no vault, no
// routing to an internal pool (ROUTE is a BLOCK), and no answer scanning
// (RESPMOD is not implemented). The request goes to the vendor through the
// web proxy, never through Tamga.
//
// Only requests to the endpoints in icapRoutes are read. Everything else,
// including every other request to the same hosts, passes unread with a
// 204: the web proxy sends it on and Tamga never sees its body.
//
// See docs/icap.md.

// icapSlotWait is how long a request waits for a free scan slot before it
// is refused with a 503. A request here is a user waiting, not a client
// that can retry on its own, so it waits before it is refused.
const icapSlotWait = 5 * time.Second

// icapMaxSegments is the most strings a body that is read generically (see
// icapStrings) is scanned as; the rest is scanned as one more segment.
const icapMaxSegments = 4096

// icapMaxDepth is how deep a JSON document written inside a string is
// followed.
const icapMaxDepth = 8

// icapMaxUser is the longest user name kept from the web proxy.
const icapMaxUser = 256

// The ICAP headers the web proxy names its user in. Squid sends
// X-Client-Username (icap_client_username_header) as it is, unless
// icap_client_username_encode is on; X-Authenticated-User is base64 by
// convention (RFC 3507 section 9 mentions it, several gateways send it).
const (
	icapUserHeader     = "X-Client-Username"
	icapAuthUserHeader = "X-Authenticated-User"
)

// icapRoute is an endpoint whose requests are read.
type icapRoute struct {
	// hosts are matched exactly; suffix, when set, matches any host that
	// ends in it.
	hosts  []string
	suffix string
	path   *regexp.Regexp
	// name is the endpoint recorded: the host and the path with the
	// identifiers in it replaced by "*".
	name string
	// provider is the request shape the extractor reads the body as, and
	// the provider recorded.
	provider string
	// web marks a web application's own back-end. Its request format is
	// not documented by the vendor and was not checked against live
	// traffic, so its body is read generically, every string in it, and
	// never rewritten: a REDACT or a STRIP of it is a BLOCK.
	web bool
	// form marks a body that is application/x-www-form-urlencoded with the
	// request in the f.req field, as the Gemini web app is believed to send.
	form bool
}

var icapRoutes = []icapRoute{
	// The vendors' public APIs: the shapes the extractor knows.
	{hosts: []string{"api.openai.com"}, path: regexp.MustCompile(`^/v1/(chat/completions|completions|responses)$`), name: "api.openai.com/v1/*", provider: "openai"},
	{hosts: []string{"api.anthropic.com"}, path: regexp.MustCompile(`^/v1/messages(/count_tokens)?$`), name: "api.anthropic.com/v1/messages", provider: "anthropic"},
	{hosts: []string{"generativelanguage.googleapis.com"}, path: regexp.MustCompile(`^/v1(beta|alpha)?/(models|tunedModels)/[^/:]+:(generateContent|streamGenerateContent|countTokens)$`), name: "generativelanguage.googleapis.com/*/models/*", provider: "gemini"},
	{hosts: []string{"api.mistral.ai"}, path: regexp.MustCompile(`^/v1/(chat/completions|agents/completions)$`), name: "api.mistral.ai/v1/*", provider: "mistral"},
	{suffix: ".openai.azure.com", path: regexp.MustCompile(`^/openai/(deployments/[^/]+/(chat/completions|completions)|v1/(chat/completions|responses)|responses)$`), name: "*.openai.azure.com/openai/*", provider: "azure"},

	// The web applications' back-ends. Not verified: see icapRoute.web.
	{hosts: []string{"chatgpt.com", "chat.openai.com"}, path: regexp.MustCompile(`^/backend-api/(f/)?conversation$`), name: "chatgpt.com/backend-api/conversation", provider: "openai", web: true},
	{hosts: []string{"claude.ai"}, path: regexp.MustCompile(`^/api/organizations/[^/]+/chat_conversations/[^/]+/(completion|retry_completion)$`), name: "claude.ai/api/organizations/*/chat_conversations/*/completion", provider: "anthropic", web: true},
	{hosts: []string{"gemini.google.com"}, path: regexp.MustCompile(`^/_/BardChatUi/data/assistant\.lamda\.BardFrontendService/StreamGenerate$`), name: "gemini.google.com/_/BardChatUi/data/*/StreamGenerate", provider: "gemini", web: true, form: true},
}

// icapGeminiModel takes the model out of a Gemini API path.
var icapGeminiModel = regexp.MustCompile(`/(?:models|tunedModels)/([^/:]+):`)

// matchICAP returns the route r is a request to, or nil.
func matchICAP(r *http.Request) *icapRoute {
	if r == nil || r.Method != http.MethodPost || r.URL == nil {
		return nil
	}
	host := r.URL.Host
	if host == "" {
		host = r.Host
	}
	if h, _, ok := strings.Cut(host, ":"); ok && !strings.Contains(h, "]") {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for i := range icapRoutes {
		rt := &icapRoutes[i]
		hit := rt.suffix != "" && strings.HasSuffix(host, rt.suffix) && len(host) > len(rt.suffix)
		for _, h := range rt.hosts {
			hit = hit || host == h
		}
		if hit && rt.path.MatchString(r.URL.Path) {
			return rt
		}
	}
	return nil
}

type icapAdapter struct {
	scan *scanHandler
	// slots bounds the scans running at once. They are the route's own:
	// the scan API cannot take them from the web proxy's users, nor the
	// other way round.
	slots chan struct{}
	wait  time.Duration
}

// NewICAPHandler returns what the ICAP server asks about requests. scan is
// the scan API's handler from NewScanHandlers: the ICAP route scans with
// its configuration and its classifier guards.
func NewICAPHandler(scan http.Handler) (icap.Handler, error) {
	h, ok := scan.(*scanHandler)
	if !ok {
		return nil, errors.New("icap: the scan handler is not the one NewScanHandlers returns")
	}
	return &icapAdapter{scan: h, slots: make(chan struct{}, max(1, runtime.GOMAXPROCS(0))), wait: icapSlotWait}, nil
}

// ISTag changes with the policy: what a request would be told depends on it.
func (a *icapAdapter) ISTag() string {
	pol := a.scan.cfg.GetPolicy()
	if pol == nil {
		return "tamga-none"
	}
	sum := sha256.Sum256([]byte(pol.Name + "\x00" + pol.Version))
	return "tamga-" + hex.EncodeToString(sum[:8])
}

func (a *icapAdapter) maxBody(rt *icapRoute) int {
	configMax := 0
	if a.scan.cfg.Config != nil {
		configMax = a.scan.cfg.Config.MaxBodyBytes
	}
	return a.scan.cfg.GetPolicy().ResolveMaxBodyBytes(rt.provider, configMax)
}

// Inspect wants the requests to a route, and limits their bodies as the
// proxy path limits them: body_limits for the provider, else
// TAMGA_MAX_BODY_BYTES.
func (a *icapAdapter) Inspect(req *icap.Request) (bool, int) {
	rt := matchICAP(req.HTTP)
	if rt == nil {
		return false, 0
	}
	return true, a.maxBody(rt)
}

// icapCall is one request over the route, as it is logged.
type icapCall struct {
	start     time.Time
	requestID string
	route     *icapRoute
	logger    zerolog.Logger
	accept    string
	org       string
	user      string
}

func (a *icapAdapter) Adapt(ctx context.Context, req *icap.Request) icap.Result {
	rt := matchICAP(req.HTTP)
	if rt == nil {
		return icap.Result{Kind: icap.Unmodified}
	}
	call := &icapCall{
		start:     time.Now(),
		requestID: uuid.Must(uuid.NewV7()).String(),
		route:     rt,
		accept:    req.HTTP.Header.Get("Accept"),
		user:      icapUser(req.Header),
	}
	call.logger = log.With().Str("component", "icap").Str("request_id", call.requestID).
		Str("endpoint", rt.name).Str("icap_client", req.RemoteAddr).Logger()
	h := a.scan
	cfg := h.cfg

	refuse := func(status int, outcome, code, msg string) icap.Result {
		call.logger.Warn().Str("outcome", outcome).Int("status", status).Msg("icap request refused")
		return icapRefusal(call, status, code, msg, nil, nil)
	}
	unavailable := func(outcome string) icap.Result {
		return refuse(http.StatusServiceUnavailable, outcome, "tamga_scan_unavailable",
			"The security scan could not be completed and the policy does not allow forwarding without it.")
	}

	pol := cfg.GetPolicy()
	if pol == nil {
		return unavailable("policy_unavailable")
	}

	// A Tamga key, when the web proxy sends one (Squid: adaptation_meta),
	// is verified as on the proxy path and names the organisation. With
	// TAMGA_REQUIRE_KEY on, the web proxy must send one.
	kr := &http.Request{Header: http.Header{}}
	if k := strings.TrimSpace(req.Header.Get(keyHeader)); k != "" {
		kr.Header.Set(keyHeader, k)
	}
	who, errType, _ := authenticate(kr, cfg)
	if errType != "" {
		call.logger.Error().Str("reason", errType).Msg("icap: the web proxy's Tamga key was refused")
		return refuse(http.StatusServiceUnavailable, "unauthorized", "tamga_icap_key", "Tamga refused the web proxy's key, so there is no verdict.")
	}
	if who.verified() {
		call.org = kr.Header.Get("X-Tamga-Org-Id")
	}

	limit := a.maxBody(rt)
	if req.BodyTooLarge {
		return refuse(http.StatusRequestEntityTooLarge, "too_large", "body_too_large",
			"Request body exceeds maximum allowed size of "+strconv.Itoa(limit)+" bytes")
	}
	body, decoded, err := icapDecode(req.HTTP.Header.Get("Content-Encoding"), req.Body, limit)
	switch {
	case errors.Is(err, errICAPTooLarge):
		return refuse(http.StatusRequestEntityTooLarge, "too_large", "body_too_large",
			"Request body exceeds maximum allowed size of "+strconv.Itoa(limit)+" bytes")
	case err != nil:
		// A body Tamga cannot read is not one it lets through unread.
		return refuse(http.StatusUnsupportedMediaType, "unreadable", "tamga_unreadable_body",
			"Tamga cannot read a request body with this Content-Encoding, so it was not forwarded.")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return icap.Result{Kind: icap.Unmodified}
	}

	in, malformed := icapInput(rt, body)
	if malformed != nil && pol.OnMalformedJSON() == policy.MalformedBlock {
		extract.RecordMalformed(malformed.Reason)
		return refuse(http.StatusBadRequest, "invalid", "tamga_invalid_json", "Request body is ambiguous JSON: "+malformed.Error())
	}
	model := icapModel(rt, req.HTTP, body)

	if len(in.segs) == 0 {
		a.record(ctx, call, who, pol, model, scanEval{action: policy.ActionPass}, 0)
		return icap.Result{Kind: icap.Unmodified}
	}

	wait, cancel := context.WithTimeout(ctx, a.wait)
	select {
	case a.slots <- struct{}{}:
		cancel()
		defer func() { <-a.slots }()
	case <-wait.Done():
		cancel()
		return refuse(http.StatusServiceUnavailable, "busy", "tamga_scan_busy", "Tamga is busy, try again.")
	}

	sc := &scanCall{start: call.start, logger: call.logger}
	ev, outcome := h.evaluateInput(ctx, sc, call.requestID, who, pol, in)
	if outcome != "" {
		return unavailable(outcome)
	}
	scanned := time.Since(call.start)
	// The web proxy can send a request on, change it or answer it. It
	// cannot send it to an internal pool.
	if ev.action == policy.ActionRoute {
		ev.action = policy.ActionBlock
		ev.fallback = "the policy routes this request to an internal model, and routing is not possible over ICAP"
		ev.headers = append(ev.headers, [2]string{"X-Tamga-Route-Fallback", "block:icap"})
	}
	if (ev.action == policy.ActionRedact || ev.action == policy.ActionStrip) && ev.body == nil {
		// Defensive: evaluateInput turns a rewrite it cannot carry out into
		// a BLOCK, so a REDACT here always has a body.
		ev.action = policy.ActionBlock
	}

	a.record(ctx, call, who, pol, model, ev, scanned)
	call.logger.Info().Str("action", string(ev.action)).Int("findings", len(ev.findings)).
		Int("bytes", len(body)).Float64("latency_ms", durationMs(time.Since(call.start))).Msg("icap request")

	switch ev.action {
	case policy.ActionBlock:
		msg := "Request blocked by Tamga security policy"
		if ev.fallback != "" {
			msg += ": " + ev.fallback
		}
		return icapRefusal(call, http.StatusForbidden, "tamga_blocked", msg, ev.findings, ev.headers)
	case policy.ActionRedact, policy.ActionStrip:
		res := icap.Result{Kind: icap.Modified, Body: ev.body}
		if decoded {
			res.DropHeaders = []string{"Content-Encoding"}
		}
		return res
	}
	return icap.Result{Kind: icap.Unmodified}
}

var errICAPTooLarge = errors.New("decoded body too large")

// icapDecode returns the body as the scanners read it. Only gzip is
// decoded; decoded says the body is no longer in the encoding it came in.
func icapDecode(encoding string, body []byte, limit int) (out []byte, decoded bool, err error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return body, false, nil
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, false, err
		}
		out, err = io.ReadAll(io.LimitReader(zr, int64(limit)+1))
		if err != nil {
			return nil, false, err
		}
		if len(out) > limit {
			return nil, false, errICAPTooLarge
		}
		return out, true, nil
	}
	return nil, false, errors.New("unsupported content encoding")
}

// icapInput is what a request is scanned as. A body of a known API shape is
// read by the extractor and may be rewritten as a whole. Anything else (a
// web application's body, an API body the extractor does not know, a JSON
// body that two parsers could read differently) is read generically and
// may only pass or be blocked. malformed is set for the last of those.
func icapInput(rt *icapRoute, body []byte) (in scanInput, malformed *extract.StrictError) {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if !rt.form && len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		malformed = extract.CheckStrict(body)
	}
	if !rt.web && malformed == nil {
		if res, ok := extract.Extract(rt.provider, body); ok && len(res.Segments) > 0 {
			for _, blockType := range res.Unknown {
				extract.RecordUnknownBlock(blockType)
			}
			return scanInput{body: body, segs: res.Segments, provider: rt.provider, whole: true, files: res.Attachments}, nil
		}
	}
	in = scanInput{body: body, provider: rt.provider, locked: "unverified_format"}
	switch {
	case malformed != nil:
		in.locked = "malformed_json"
	case !rt.web:
		in.locked = "unknown_shape"
	}
	if rt.form {
		in.segs = icapForm(body)
	} else {
		in.segs = icapStrings(body, "")
	}
	if malformed != nil && utf8.Valid(body) {
		// The decoder keeps the last of two values for one key; the bytes
		// as they came are scanned as well, so that the first is read too.
		in.segs = append(in.segs, icapSegment("(raw)", string(body)))
	}
	return in, malformed
}

// icapForm reads a form body: the f.req field when there is one, and every
// field when there is not. Other fields of the Gemini web app carry tokens,
// which the secret scanners would take for keys.
func icapForm(body []byte) []extract.Segment {
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		return icapStrings(body, "")
	}
	keys := []string{"f.req"}
	if _, ok := vals["f.req"]; !ok {
		keys = keys[:0]
		for k := range vals {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	var segs []extract.Segment
	for _, k := range keys {
		for i, v := range vals[k] {
			segs = append(segs, icapStrings([]byte(v), k+"["+strconv.Itoa(i)+"]")...)
		}
	}
	return icapCap(segs)
}

// icapStrings reads every string in a JSON body as text the user wrote, a
// JSON document written inside a string included. A body that is not JSON
// is one text.
func icapStrings(body []byte, prefix string) []extract.Segment {
	var v interface{}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		if s := strings.TrimSpace(string(body)); s != "" && utf8.ValidString(s) {
			return []extract.Segment{icapSegment(prefix, s)}
		}
		return nil
	}
	var segs []extract.Segment
	icapWalk(v, prefix, 0, &segs)
	return icapCap(segs)
}

func icapSegment(path, text string) extract.Segment {
	if path == "" {
		path = "$"
	}
	return extract.Segment{Role: extract.RoleUser, Kind: extract.KindUnknown, Path: path, Text: text}
}

func icapWalk(v interface{}, path string, depth int, segs *[]extract.Segment) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return
		}
		if depth < icapMaxDepth && (s[0] == '[' || s[0] == '{') {
			var inner interface{}
			dec := json.NewDecoder(strings.NewReader(s))
			dec.UseNumber()
			if dec.Decode(&inner) == nil {
				icapWalk(inner, path+"#", depth+1, segs)
				return
			}
		}
		*segs = append(*segs, icapSegment(path, t))
	case []interface{}:
		for i, e := range t {
			icapWalk(e, path+"["+strconv.Itoa(i)+"]", depth, segs)
		}
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if path != "" {
				p = path + "." + k
			}
			icapWalk(t[k], p, depth, segs)
		}
	}
}

// icapCap keeps the first icapMaxSegments segments and joins the rest into
// one, so that nothing is left unscanned.
func icapCap(segs []extract.Segment) []extract.Segment {
	if len(segs) <= icapMaxSegments {
		return segs
	}
	var rest strings.Builder
	for _, s := range segs[icapMaxSegments-1:] {
		rest.WriteString(s.Text)
		rest.WriteString("\n")
	}
	out := append(segs[:icapMaxSegments-1:icapMaxSegments-1], icapSegment("(rest)", rest.String()))
	return out
}

// icapModel is the model a request names, for the record.
func icapModel(rt *icapRoute, r *http.Request, body []byte) string {
	if rt.provider == "gemini" && !rt.web {
		if m := icapGeminiModel.FindStringSubmatch(r.URL.Path); m != nil {
			return litellmModel(m[1])
		}
	}
	if rt.form {
		return ""
	}
	return litellmModel(extractModelFromBody(body))
}

// icapUser is the user the web proxy names, cut to length and to printable
// characters. It is the web proxy's word, which Tamga cannot check.
func icapUser(h http.Header) string {
	u := strings.TrimSpace(h.Get(icapUserHeader))
	if u == "" {
		if raw := strings.TrimSpace(h.Get(icapAuthUserHeader)); raw != "" {
			u = raw
			if dec, err := base64.StdEncoding.DecodeString(raw); err == nil && utf8.Valid(dec) {
				u = string(dec)
			}
		}
	}
	if len(u) > icapMaxUser || !utf8.ValidString(u) {
		return ""
	}
	for _, r := range u {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	return u
}

// icapRefusal is the HTTP response the user gets in place of a request
// that is not sent on: JSON, in the shape of the proxy path's errors, or a
// short page for a client that asks for HTML.
func icapRefusal(call *icapCall, status int, code, msg string, findings []scanner.Finding, extra [][2]string) icap.Result {
	hdr := http.Header{}
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Tamga-Request-Id", call.requestID)
	for _, h := range extra {
		hdr.Set(h[0], h[1])
	}
	// The types, not the categories: a category can be a name on the
	// operator's watch list, which the user is not told (scanNamedTypes).
	types := []string{}
	for _, f := range findings {
		if !slices.Contains(types, f.Type) {
			types = append(types, f.Type)
		}
	}
	if code == "tamga_blocked" {
		hdr.Set("X-Tamga-Action", string(policy.ActionBlock))
	}
	if status == http.StatusServiceUnavailable {
		hdr.Set("Retry-After", "1")
	}
	accept := strings.ToLower(call.accept)
	if strings.Contains(accept, "text/html") && !strings.Contains(accept, "json") && !strings.Contains(accept, "event-stream") {
		hdr.Set("Content-Type", "text/html; charset=utf-8")
		page := "<!doctype html><html><head><meta charset=\"utf-8\"><title>Stopped by Tamga</title></head><body>" +
			"<h1>Stopped by Tamga</h1><p>" + html.EscapeString(msg) + "</p>" +
			"<p>Request ID: <code>" + html.EscapeString(call.requestID) + "</code></p></body></html>\n"
		return icap.Result{Kind: icap.Respond, Status: status, ResponseHeader: hdr, ResponseBody: []byte(page)}
	}
	hdr.Set("Content-Type", "application/json")
	e := map[string]interface{}{
		"code":       code,
		"message":    msg,
		"request_id": call.requestID,
	}
	if code == "tamga_blocked" {
		e["type"] = "security_violation"
		e["findings_count"] = len(findings)
		e["finding_types"] = types
	}
	b, _ := json.Marshal(map[string]interface{}{"error": e})
	return icap.Result{Kind: icap.Respond, Status: status, ResponseHeader: hdr, ResponseBody: append(b, '\n')}
}

// record publishes the request as a request's own event, as the proxy path
// publishes one when a request is over, so a request seen over ICAP is in
// the request log, the live feed and the alerts like any other. The
// provider is the vendor the request was going to, and the endpoint is
// "icap " and the route's name, which is how a row says it came this way.
// No text is on the event, and no body for the analyzer.
func (a *icapAdapter) record(ctx context.Context, call *icapCall, who caller, pol *policy.Policy, model string, ev scanEval, scanned time.Duration) {
	cfg := a.scan.cfg
	if cfg.Bus == nil || pol == nil {
		return
	}
	findings := append([]scanner.Finding(nil), ev.findings...)
	applyFindingActionTaken(findings, ev.action)
	risk := scanner.CalculateRisk(findings)
	e := events.Event{
		RequestID:      call.requestID,
		OrgID:          call.org,
		OrgVerified:    who.verified(),
		Provider:       call.route.provider,
		Model:          model,
		ModelFamily:    extractModelFamily(model),
		EventType:      "request_scanned",
		Findings:       findings,
		Action:         string(ev.action),
		Endpoint:       "icap " + call.route.name,
		ScanLatencyMs:  durationMs(scanned),
		TotalLatencyMs: durationMs(time.Since(call.start)),
		UserID:         call.user,
		Timestamp:      time.Now().UTC(),
		InputRisk:      risk,
		OutputRisk:     scanner.RiskScore{Level: "none", Breakdown: map[string]float64{}},
		PolicyName:     pol.Name,
		PolicyVersion:  pol.Version,
		RoutePool:      ev.route,
		Classifier:     ev.classifier,
	}
	if ev.action == policy.ActionBlock {
		e.EventType = "request_blocked"
	}
	cfg.Bus.PublishContext(ctx, e)

	if cfg.Alerts == nil || ev.action == policy.ActionPass {
		return
	}
	urls := pol.NotifyURLsForFindings(findings)
	if len(urls) == 0 {
		return
	}
	org := call.org
	if org == "" {
		org = "default"
		if cfg.Config != nil && cfg.Config.DefaultOrgID != "" {
			org = cfg.Config.DefaultOrgID
		}
	}
	cfg.Alerts.NotifyURLs(urls, webhooks.Alert{
		RequestID: call.requestID,
		Timestamp: e.Timestamp,
		OrgID:     org,
		Provider:  call.route.provider,
		Model:     model,
		Action:    string(ev.action),
		Direction: "input",
		EventType: "rule_notify",
		InputRisk: risk.Percentage,
		Findings:  webhooks.AlertFindings(findings),
	})
}
