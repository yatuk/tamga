package proxy

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/api"
	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/classifier"
	"github.com/yatuk/tamga/internal/errkind"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/scanner"
)

// adminKeyHeader carries the admin key, as on the rest of the management API.
const adminKeyHeader = "X-Tamga-Admin-Key"

// scanMaxBodyBytes is the largest request the scan API reads. It is fixed,
// not taken from body_limits, which is set per provider and the scan API has
// none; 256 KiB is a quarter of the proxy's default.
const scanMaxBodyBytes = 256 * 1024

// scanDefaultRPM is the per-key limit when the policy sets none. The proxy
// path is unlimited in that case; a route that any key opens is not.
const scanDefaultRPM = 60

// scanProvider is the request shape a scanned text is wrapped in. The text
// becomes the content of one chat message, so it is one segment with the
// role, kind and path a real request would give it.
const scanProvider = "openai"

// scanExcluded are the scanners the scan API leaves out. operator_state
// hands the text it reads to the deep-analysis publisher, and a scanned text
// must go nowhere.
var scanExcluded = []string{"operator_state"}

// scanRoles are the roles a scan request may give its text.
var scanRoles = map[string]bool{"system": true, "user": true, "assistant": true, "tool": true}

// scanUnplacedTypes are the finding types reported without a position. Where
// exactly a detector matched an attack is of more use to whoever is writing
// the attack than to the caller.
var scanUnplacedTypes = map[string]bool{"injection": true, "jailbreak": true, "content_moderation": true, "classifier": true, "topic": true}

// scanNamedTypes are the finding types whose category is a name the operator
// chose: a custom entity, a competitor, a banned topic. Together those names are the watch
// list. A proxy key is the one handed to applications and browser extensions,
// so it is told the type and not the name; the other scopes already read the
// policy.
var scanNamedTypes = map[string]bool{"custom": true, "competitor": true, "topic": true}

// scanClassifierHeader says the inline classifier did not read all of the
// text, on a verdict that was given all the same.
const scanClassifierHeader = "X-Tamga-Classifier"

// scanTopicsHeader says the same of the banned-topic model.
const scanTopicsHeader = "X-Tamga-Topics"

// The scan API's error messages. They are fixed: nothing from the request,
// a scanner or a decoder is passed on.
const (
	scanErrUnauthorized = "unauthorized"
	scanErrOrigin       = "origin not allowed"
	scanErrRateLimit    = "rate limit exceeded"
	scanErrContentType  = "Content-Type must be application/json"
	scanErrTooLarge     = "request body exceeds 262144 bytes"
	scanErrRead         = "failed to read request body"
	scanErrUTF8         = "request body is not valid UTF-8"
	scanErrJSON         = "invalid JSON body"
	scanErrText         = "text is required"
	scanErrRole         = "role must be one of system, user, assistant, tool"
	scanErrBusy         = "the scan API is busy, try again"
	scanErrUnavailable  = "the scan could not be completed, so there is no verdict"
)

// ScanOptions is what the scan API needs beyond the proxy path's own
// configuration.
type ScanOptions struct {
	// AdminKey opens the route when sent as X-Tamga-Admin-Key.
	AdminKey string
	// Classifier is the inline classifier for scans. It must be a Guard of
	// its own, not HandlerConfig.Classifier: a Guard holds the circuit
	// breaker, and failed scans must not open the one the proxy path uses.
	Classifier *classifier.Guard
	// Topics is the banned-topic model for scans: a TopicGuard of its own,
	// for the same reason.
	Topics *classifier.TopicGuard
	// CORSOrigins is the management API's origin allowlist (comma-separated).
	// A request from a web page of any other origin is refused.
	CORSOrigins string
}

type scanRequest struct {
	Text *string `json:"text"`
	Role *string `json:"role"`
}

// scanFinding is what the scan API says about a finding: what it is, where,
// and what the policy does about it. It is a projection, not the scanner's
// record. The matched text is left out (the caller has it, and the scanners'
// masking keeps part of a value), as are scores and scanner details.
type scanFinding struct {
	Type     string `json:"type"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	// Position says whether the finding has a place in the submitted text:
	// "exact", "normalised_view" (found only in a normalised view of it) or
	// "none" (the detector gives none, or the type is in scanUnplacedTypes).
	Position string `json:"position"`
	// Start and End are byte offsets into the submitted text as UTF-8, and
	// StartUTF16 and EndUTF16 the same span in UTF-16 code units, which is
	// how a browser indexes a string. All four are absent unless Position
	// is "exact".
	Start      *int `json:"start,omitempty"`
	End        *int `json:"end,omitempty"`
	StartUTF16 *int `json:"start_utf16,omitempty"`
	EndUTF16   *int `json:"end_utf16,omitempty"`
	// One text is scanned, with one role: the offsets are into that text,
	// and the role, which the caller chose, is not repeated on a finding.
	Action string `json:"action"`
}

type scanPolicyRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type scanResponse struct {
	Action   string        `json:"action"`
	Findings []scanFinding `json:"findings"`
	// RedactedText is set only when the action is REDACT.
	RedactedText *string `json:"redacted_text,omitempty"`
	// RouteTo is set only when the action is ROUTE: the internal pool the
	// proxy would send the request to. A scan sends nothing anywhere.
	RouteTo       string        `json:"route_to,omitempty"`
	Policy        scanPolicyRef `json:"policy"`
	ScanLatencyMs float64       `json:"scan_latency_ms"`
}

type scanHandler struct {
	cfg     HandlerConfig
	opts    ScanOptions
	origins map[string]bool
	// fallback limits callers when the policy has no rate limit.
	fallback *ratelimit.Limiter
	// slots bounds the scans running at once, so that scan load cannot take
	// the processors the proxy path's own scans need.
	slots chan struct{}
}

// NewScanHandler serves POST /api/v1/scan: the verdict the policy gives a
// text, without a provider being called. It is mounted in the management API
// but does its own authentication, because a proxy key must open it and
// nothing else there.
//
// The text goes nowhere: no event is published, so it reaches neither the
// request store, the audit log nor the live feed; nothing is written to the
// vault or the prompt cache; and nothing logged here contains it.
func NewScanHandler(cfg HandlerConfig, opts ScanOptions) http.Handler {
	limit := &policy.Policy{RateLimit: &policy.RateLimit{MaxRequestsPerMinute: scanDefaultRPM}}
	h := &scanHandler{
		cfg:      cfg,
		opts:     opts,
		origins:  map[string]bool{},
		fallback: ratelimit.NewLimiter(func() *policy.Policy { return limit }),
		slots:    make(chan struct{}, max(1, runtime.GOMAXPROCS(0)/2)),
	}
	for _, o := range strings.Split(opts.CORSOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			h.origins[o] = true
		}
	}
	return h
}

// scanCall is what one call is logged and counted with. None of it is text.
type scanCall struct {
	start    time.Time
	logger   zerolog.Logger
	keyID    string
	org      string
	keyRole  string
	textRole string
	bytes    int
	byType   map[string]int
	// bridge is set by the LiteLLM route (litellm.go), which is counted and
	// logged apart from the scan API.
	bridge *bridgeCall
}

// bridgeCall is what a call over the LiteLLM route adds to its log line.
type bridgeCall struct {
	direction string
	texts     int
	tools     int
	toolCalls int
	callID    string
}

// done writes the call's one log line and counts it.
func (c *scanCall) done(outcome string, refused bool) {
	msg := "scan api call"
	if c.bridge != nil {
		msg = "litellm bridge call"
		api.CountScanAPI("litellm_" + outcome)
	} else {
		api.CountScanAPI(outcome)
	}
	ev := c.logger.Info()
	if refused {
		ev = c.logger.Warn()
	}
	counts := zerolog.Dict()
	total := 0
	for typ, n := range c.byType {
		counts.Int(typ, n)
		total += n
	}
	if c.bridge != nil {
		ev = ev.Str("direction", c.bridge.direction).
			Int("texts", c.bridge.texts).
			Int("tools", c.bridge.tools).
			Int("tool_calls", c.bridge.toolCalls).
			Str("litellm_call_id", c.bridge.callID)
	}
	ev.Str("outcome", outcome).
		Str("key_id", c.keyID).
		Str("org_id", c.org).
		Str("key_role", c.keyRole).
		Str("text_role", c.textRole).
		Int("bytes", c.bytes).
		Int("findings", total).
		Dict("findings_by_type", counts).
		Float64("latency_ms", float64(time.Since(c.start).Microseconds())/1000).
		Msg(msg)
}

func writeScanJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeScanError(w http.ResponseWriter, status int, msg string) {
	writeScanJSON(w, status, map[string]string{"error": msg})
}

// authenticate resolves who is asking. Only the two key headers count here:
// no key in the query string, no session, no development bypass.
//
// A key in X-Tamga-Key is verified as the proxy path verifies it, whatever
// its scope. X-Tamga-Admin-Key takes the admin key. bucket names the
// caller within a route's rate limit.
func (h *scanHandler) authenticate(r *http.Request) (who caller, bucket string, ok bool) {
	if strings.TrimSpace(r.Header.Get(keyHeader)) != "" {
		who, errType, _ := authenticate(r, h.cfg)
		if errType != "" || !who.verified() {
			return caller{}, "", false
		}
		return who, "tk:" + who.KeyID, true
	}
	key := r.Header.Get(adminKeyHeader)
	if key == "" || h.opts.AdminKey == "" {
		return caller{}, "", false
	}
	if subtle.ConstantTimeCompare([]byte(key), []byte(h.opts.AdminKey)) != 1 {
		return caller{}, "", false
	}
	return caller{}, "admin", true
}

// originAllowed refuses a request a web page made, unless the page's origin
// is on the management API's allowlist. A page cannot read the answer
// without CORS, but it could still spend a key's rate limit. A browser
// extension's background worker sends its own scheme or no Origin at all.
func (h *scanHandler) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	lower := strings.ToLower(origin)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return true
	}
	return h.origins[origin]
}

// checkRate counts the call in the caller's bucket for this route, which is
// apart from the one its proxy requests are counted in: a CI job that scans
// in a loop must not use up the application's requests, nor the other way
// round. fallback is the route's limit when the policy sets none.
func (h *scanHandler) checkRate(bucket string, pol *policy.Policy, fallback *ratelimit.Limiter) ratelimit.AllowResult {
	if h.cfg.RateLimit != nil && pol != nil && pol.RateLimit != nil && pol.RateLimit.MaxRequestsPerMinute > 0 {
		return h.cfg.RateLimit.Check(bucket)
	}
	return fallback.Check(bucket)
}

// scanAdmission is what differs between the routes that share the scan path
// (the scan API and the LiteLLM route) before a text is read.
type scanAdmission struct {
	// bucket is the prefix of the route's rate-limit buckets.
	bucket string
	// fallback limits callers when the policy has no rate limit.
	fallback *ratelimit.Limiter
	// maxBody is the largest body read, and tooLarge the message for one
	// that is larger.
	maxBody  int64
	tooLarge string
	// apiKeyHeader, when set, is a second header a Tamga key is read from.
	// X-Tamga-Key wins when both are sent. The admin key is not read from it.
	apiKeyHeader string
}

// admit does what every route on the scan path does before it looks at a
// text: origin, key, rate limit, content type, body limit, and the proxy
// path's own test of a JSON body. A request that fails one is answered
// through refuse, and ok is false.
func (h *scanHandler) admit(w http.ResponseWriter, r *http.Request, call *scanCall, adm scanAdmission,
	refuse func(status int, outcome, msg string)) (who caller, pol *policy.Policy, raw []byte, ok bool) {
	if !h.originAllowed(r) {
		refuse(http.StatusForbidden, "origin_refused", scanErrOrigin)
		return caller{}, nil, nil, false
	}

	if adm.apiKeyHeader != "" {
		if key := strings.TrimSpace(r.Header.Get(adm.apiKeyHeader)); key != "" && strings.TrimSpace(r.Header.Get(keyHeader)) == "" {
			r.Header.Set(keyHeader, key)
		}
		r.Header.Del(adm.apiKeyHeader)
	}
	who, bucket, authed := h.authenticate(r)
	if !authed {
		call.logger = call.logger.With().Str("client_ip", clientIP(r, trustedProxies(h.cfg))).Logger()
		refuse(http.StatusUnauthorized, "unauthorized", scanErrUnauthorized)
		return caller{}, nil, nil, false
	}
	call.keyID, call.keyRole = who.KeyID, who.Role
	if who.verified() {
		call.org = r.Header.Get("X-Tamga-Org-Id")
	}

	pol = h.cfg.GetPolicy()

	// Going over the limit is always refused. rate_limit.action_on_exceed
	// can say WARN on the proxy path, where the request is then forwarded;
	// a verdict has no such second outcome.
	res := h.checkRate(adm.bucket+bucket, pol, adm.fallback)
	if res.Limit > 0 {
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
	}
	if !res.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, res.RetryAfterS)))
		refuse(http.StatusTooManyRequests, "rate_limited", scanErrRateLimit)
		return caller{}, nil, nil, false
	}

	// Only JSON, said to be JSON. A web page can post text/plain to another
	// origin without asking first; it cannot do that with this type.
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		refuse(http.StatusUnsupportedMediaType, "unsupported_media_type", scanErrContentType)
		return caller{}, nil, nil, false
	}

	if r.ContentLength > adm.maxBody {
		refuse(http.StatusRequestEntityTooLarge, "too_large", adm.tooLarge)
		return caller{}, nil, nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, adm.maxBody))
	call.bytes = len(raw)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			refuse(http.StatusRequestEntityTooLarge, "too_large", adm.tooLarge)
			return caller{}, nil, nil, false
		}
		refuse(http.StatusBadRequest, "invalid", scanErrRead)
		return caller{}, nil, nil, false
	}

	// The proxy path's own test of a JSON body: bytes that are not UTF-8,
	// which the decoder would quietly replace, and duplicate keys, which
	// would let the text scanned differ from the text the caller meant.
	if strictErr := extract.CheckStrict(raw); strictErr != nil {
		msg := scanErrJSON
		if strictErr.Reason == extract.ReasonInvalidUTF8 {
			msg = scanErrUTF8
		}
		refuse(http.StatusBadRequest, "invalid", msg)
		return caller{}, nil, nil, false
	}
	return who, pol, raw, true
}

// scanMessage is one text and the role it is scanned in.
type scanMessage struct {
	role string
	text string
}

// scanEval is what the policy does with a set of messages.
type scanEval struct {
	action   policy.Action
	findings []scanner.Finding
	// actionOf is what the policy does about one finding on its own.
	actionOf func(scanner.Finding) policy.Action
	// rewritten is the text of each message as the proxy would forward it,
	// in the order the messages were given. It is set when the action is
	// REDACT or STRIP, and only then.
	rewritten []string
	// tools is the tool definitions as the proxy would forward them, set
	// with rewritten when one of them was changed and nil otherwise.
	tools []json.RawMessage
	// fallback says why a REDACT or a STRIP became a BLOCK, and is empty
	// when the BLOCK is a rule's own.
	fallback string
	// headers are response headers the evaluation asks for.
	headers [][2]string
	// route is the internal pool the policy names for the action ROUTE, set
	// when that was the verdict of the rules.
	route string
}

// scanInput is what evaluate reads: a chat request in OpenAI's shape, built
// from what the caller sent, and its segments as the proxy path would scan
// them. The first texts messages hold one text each; the LiteLLM route adds
// tool calls (one more message, the assistant's) and tool definitions.
type scanInput struct {
	body  []byte
	segs  []extract.Segment
	texts int
}

// scanWireMessage is one message of the request a scanInput is built as.
type scanWireMessage struct {
	Role      string            `json:"role"`
	Content   *string           `json:"content,omitempty"`
	ToolCalls []json.RawMessage `json:"tool_calls,omitempty"`
}

// Why a segment of a scanInput cannot be rewritten. They are reasons a
// REDACT or a STRIP falls back to BLOCK, beside those in redact.go.
const (
	// redactToolCall: the value is in a tool call's arguments, and the
	// LiteLLM contract's reply has no field to return tool calls in.
	redactToolCall = "tool_call"
	// redactToolName: the value is a tool's name. LiteLLM matches the tools
	// it gets back to its own by name.
	redactToolName = "tool_name"
)

// toolNamePath is the path of a tool definition's name.
var toolNamePath = regexp.MustCompile(`^tools\[\d+\](\.function)?\.name$`)

// newScanInput builds the request evaluate reads. toolCalls and tools are
// the caller's own JSON and go in as they came, so the extractor reads them
// as it reads a tool call and a tool definition on the proxy path. ok is
// false when the request does not read back as what it was built from.
func newScanInput(msgs []scanMessage, toolCalls, tools []json.RawMessage) (in scanInput, ok bool) {
	doc := struct {
		Messages []scanWireMessage `json:"messages"`
		Tools    []json.RawMessage `json:"tools,omitempty"`
	}{Messages: make([]scanWireMessage, 0, len(msgs)+1), Tools: tools}
	for i := range msgs {
		doc.Messages = append(doc.Messages, scanWireMessage{Role: msgs[i].role, Content: &msgs[i].text})
	}
	if len(toolCalls) > 0 {
		doc.Messages = append(doc.Messages, scanWireMessage{Role: string(extract.RoleAssistant), ToolCalls: toolCalls})
	}
	// Without HTML escaping: a tool definition that is sent back should
	// differ from the one received only where it was masked.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return in, false
	}
	in.body, in.texts = bytes.TrimSuffix(buf.Bytes(), []byte("\n")), len(msgs)
	res, extracted := extract.Extract(scanProvider, in.body)
	if !extracted || len(res.Segments) < len(msgs) {
		return in, false
	}
	in.segs = res.Segments
	for i, m := range msgs {
		if in.segs[i].Text != m.text || string(in.segs[i].Role) != m.role || in.segs[i].Path != indexed("messages", i)+".content" {
			return in, false
		}
	}
	// Whatever follows the texts is a tool call's or a tool definition's.
	for _, seg := range in.segs[len(msgs):] {
		switch {
		case in.inToolCall(seg) && seg.Role == extract.RoleAssistant:
		case strings.HasPrefix(seg.Path, "tools[") && seg.Role == extract.RoleToolDefinition:
		default:
			return in, false
		}
	}
	return in, true
}

func indexed(path string, i int) string { return path + "[" + strconv.Itoa(i) + "]" }

// inToolCall reports whether seg is part of the tool calls.
func (in scanInput) inToolCall(seg extract.Segment) bool {
	return strings.HasPrefix(seg.Path, indexed("messages", in.texts)+".tool_calls")
}

// fixed says why one of the edited segments may not be rewritten, and "" when
// all of them may. A value there that the policy wants masked or stripped
// blocks the request.
func (in scanInput) fixed(edited []int) string {
	for _, i := range edited {
		if i < in.texts || i >= len(in.segs) {
			continue
		}
		switch seg := in.segs[i]; {
		case in.inToolCall(seg):
			return redactToolCall
		case toolNamePath.MatchString(seg.Path):
			return redactToolName
		}
	}
	return ""
}

// rewritten reads the texts and the tool definitions back out of the body a
// rewrite produced. The tools are returned only when the rewrite touched one.
func (in scanInput) rewritten(body []byte, edited []int) (texts []string, tools []json.RawMessage, err error) {
	if texts, err = messageTexts(body, in.texts); err != nil {
		return nil, nil, err
	}
	changed := false
	for _, i := range edited {
		if i >= 0 && i < len(in.segs) && strings.HasPrefix(in.segs[i].Path, "tools[") {
			changed = true
		}
	}
	if !changed {
		return texts, nil, nil
	}
	var doc struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err = json.Unmarshal(body, &doc); err != nil {
		return nil, nil, err
	}
	return texts, doc.Tools, nil
}

// evaluate is the scan path for texts alone, as the scan API sends them.
func (h *scanHandler) evaluate(ctx context.Context, call *scanCall, requestID string, who caller, pol *policy.Policy, msgs []scanMessage) (ev scanEval, outcome string) {
	in, ok := newScanInput(msgs, nil, nil)
	if !ok {
		return ev, "segment_mismatch"
	}
	return h.evaluateInput(ctx, call, requestID, who, pol, in)
}

// evaluateInput is the scan path: the request is read as the segments the
// proxy path would scan, scanned by the proxy path's registry, and judged by
// the policy and the inline classifier on the proxy path's terms. A
// non-empty outcome means the scan did not run to the end and there is no
// verdict.
func (h *scanHandler) evaluateInput(ctx context.Context, call *scanCall, requestID string, who caller, pol *policy.Policy, in scanInput) (ev scanEval, outcome string) {
	body, segs := in.body, in.segs

	// The proxy path's registry, by segment, as handleProxy scans a chat
	// request. Not through the shared worker pool, and without load
	// shedding: a queue filled by scans would cost real traffic its
	// scanners, and a scan with scanners shed is not a verdict.
	pipelineCfg, strictMode := h.pipeline()
	findings, err := h.cfg.Registry.Without(scanExcluded...).ScanSegments(ctx, segs, pipelineCfg)
	if err != nil {
		// Logged by kind only: a scanner's error can quote what it read.
		call.logger.Error().Bool("degraded", errors.Is(err, scanner.ErrScanDegraded)).
			Str("reason", scanner.IncompleteReason(err)).Str("error_kind", errkind.Of(err)).Msg("scan incomplete")
		return ev, "scan_incomplete"
	}
	api.ObserveScan(float64(time.Since(call.start).Milliseconds()))

	scoreProximityBySegment(segs, findings)

	// Policy exceptions go by the role on the key's record. The admin key
	// has none, and X-Tamga-Role is not read: no gateway vouches for it here.
	userRole := who.Role
	action, _ := pol.EvaluateWithRole(findings, userRole, strictMode)

	// The inline classifier, on the proxy path's terms: asked when the
	// policy turns it on and the rules have not already blocked.
	if set, on := pol.ClassifierSettings(); on && action != policy.ActionBlock {
		if h.opts.Classifier == nil {
			call.logger.Warn().Msg("classifier is enabled in the policy and not configured")
			return ev, "classifier_unavailable"
		}
		res := classifySegments(ctx, h.opts.Classifier, set, requestID, segs, findings)
		// What it did read counts, also when it could not read everything.
		if len(res.findings) > 0 {
			findings = append(findings, res.findings...)
			action, _ = pol.EvaluateWithRole(findings, userRole, strictMode)
		}
		if res.err != nil || res.partial {
			call.logger.Warn().Bool("partial", res.partial).Msg("classifier did not read the text")
			// unread: the classifier read none of what it was to read. For
			// one text that is every partial answer. For several it is not:
			// the classifier takes the newest texts first, up to max_chars,
			// and the proxy path forwards a conversation it read in part.
			unread := res.err != nil || res.asked == 0
			mark := "partial"
			if unread {
				// Text the classifier did not read cannot be called clean,
				// so a verdict that would let the text through as it is, is
				// not given. One that already stops or rewrites it stands:
				// the unread part could only have made it stricter, and the
				// header says it was not read.
				if !scanActs(action) {
					return ev, "classifier_unavailable"
				}
				mark = "incomplete"
			}
			ev.headers = append(ev.headers, [2]string{scanClassifierHeader, mark})
		}
	}

	// Banned topics, on the same terms as the classifier above.
	if set, on := pol.TopicSettings(); on && action != policy.ActionBlock {
		if h.opts.Topics == nil {
			call.logger.Warn().Msg("topics is enabled in the policy and no classifier service is configured")
			return ev, "topics_unavailable"
		}
		res := topicSegments(ctx, h.opts.Topics, set, requestID, segs)
		if len(res.findings) > 0 {
			findings = append(findings, res.findings...)
			action, _ = pol.EvaluateWithRole(findings, userRole, strictMode)
		}
		if res.err != nil || res.partial {
			call.logger.Warn().Bool("partial", res.partial).Msg("topic model did not read the text")
			mark := "partial"
			if res.err != nil || res.asked == 0 {
				if !scanActs(action) {
					return ev, "topics_unavailable"
				}
				mark = "incomplete"
			}
			ev.headers = append(ev.headers, [2]string{scanTopicsHeader, mark})
		}
	}

	// REDACT that cannot be carried out is a BLOCK on the proxy path, so it
	// is one here. The vault is not used: with it on the proxy sends
	// numbered placeholders and keeps the originals to put back, and a scan
	// keeps nothing. The text shows the plain markers instead.
	actionOf := func(f scanner.Finding) policy.Action { return pol.FindingAction(f, userRole, strictMode) }
	if action == policy.ActionRedact {
		// The same findings, on the same terms, as on the proxy path
		// (redact.go).
		targets := redactTargets(findings, actionOf)
		reason := redactBlocker(targets)
		if reason == "" {
			edits := scanner.RedactEdits(targets)
			edited := make([]int, len(edits))
			for i, e := range edits {
				edited[i] = e.Seg
			}
			if len(edits) == 0 {
				reason = redactNothing
			} else if reason = in.fixed(edited); reason != "" {
				// Nothing is rewritten: see scanInput.fixed.
			} else if masked, rwErr := extract.RewriteChecked(scanProvider, body, segs, edits); rwErr != nil {
				reason = redactRewrite
			} else if ev.rewritten, ev.tools, rwErr = in.rewritten(masked, edited); rwErr != nil {
				reason = redactRewrite
			}
		}
		if reason != "" {
			ev.headers = append(ev.headers, [2]string{"X-Tamga-Redact-Fallback", "block:" + reason})
			ev.fallback = "REDACT could not be carried out (" + reason + ")"
			ev.rewritten, ev.tools = nil, nil
			action = policy.ActionBlock
		}
	}

	// The same for STRIP.
	if action == policy.ActionStrip {
		res, stripErr := stripSegments(scanProvider, body, segs, findings, actionOf, false)
		reason := ""
		if stripErr == nil {
			if reason = in.fixed(res.edited); reason == "" {
				ev.rewritten, ev.tools, stripErr = in.rewritten(res.body, res.edited)
			}
		}
		if stripErr != nil {
			reason = stripFailure(stripErr)
		}
		if reason != "" {
			ev.headers = append(ev.headers, [2]string{"X-Tamga-Strip-Fallback", "block:" + reason})
			ev.fallback = "STRIP could not be carried out (" + reason + ")"
			ev.rewritten, ev.tools = nil, nil
			action = policy.ActionBlock
		}
	}

	// ROUTE is reported, with the pool, and not carried out: a scan sends
	// nothing. The proxy path would also strip what a STRIP rule covers on
	// the way to the pool; no text is rewritten here for it.
	if action == policy.ActionRoute {
		ev.route = pol.RoutePool()
	}

	ev.action, ev.findings, ev.actionOf = action, findings, actionOf
	return ev, ""
}

// pipeline is the scanner pipeline a scan runs with: the proxy path's mode
// and timeout, without its worker pool.
func (h *scanHandler) pipeline() (cfg scanner.PipelineConfig, strictMode bool) {
	if h.cfg.Config != nil {
		cfg.Mode = scanner.PipelineMode(h.cfg.Config.ScannerPipelineMode)
		cfg.Timeout = time.Duration(h.cfg.Config.ScannerPipelineTimeoutMs) * time.Millisecond
		strictMode = h.cfg.Config.StrictMode
	}
	return cfg, strictMode
}

func (h *scanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.Must(uuid.NewV7()).String()
	call := &scanCall{
		start:  time.Now(),
		logger: log.With().Str("component", "scan_api").Str("request_id", requestID).Logger(),
	}
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.Header().Set("Cache-Control", "no-store")

	refuse := func(status int, outcome, msg string) {
		call.done(outcome, true)
		writeScanError(w, status, msg)
	}
	// unavailable answers a scan that did not run to the end. The proxy
	// path can be told to forward on what it has (scan.on_error: pass); a
	// verdict reached with part of the scan missing is not given here,
	// whatever that setting says, because a caller would read it as a PASS.
	unavailable := func(outcome string) {
		w.Header().Set("Retry-After", "1")
		refuse(http.StatusServiceUnavailable, outcome, scanErrUnavailable)
	}

	who, pol, raw, ok := h.admit(w, r, call, scanAdmission{
		bucket:   "scan:",
		fallback: h.fallback,
		maxBody:  scanMaxBodyBytes,
		tooLarge: scanErrTooLarge,
	}, refuse)
	if !ok {
		return
	}

	var req scanRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		refuse(http.StatusBadRequest, "invalid", scanErrJSON)
		return
	}
	if req.Text == nil || *req.Text == "" {
		refuse(http.StatusBadRequest, "invalid", scanErrText)
		return
	}
	// A role left out is the user's. One given as "" is a mistake: a
	// finding with no role is covered by every rule, whatever applies_to
	// says, and nobody asks for that by sending an empty string.
	role := string(extract.RoleUser)
	if req.Role != nil {
		role = *req.Role
	}
	if !scanRoles[role] {
		refuse(http.StatusBadRequest, "invalid", scanErrRole)
		return
	}
	text := *req.Text
	call.textRole = role

	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		refuse(http.StatusServiceUnavailable, "busy", scanErrBusy)
		return
	}

	if pol == nil {
		unavailable("policy_unavailable")
		return
	}

	ev, outcome := h.evaluate(r.Context(), call, requestID, who, pol, []scanMessage{{role: role, text: text}})
	if outcome != "" {
		unavailable(outcome)
		return
	}
	for _, hd := range ev.headers {
		w.Header().Set(hd[0], hd[1])
	}

	out := scanResponse{
		Action:        scanVerdict(ev.action),
		Findings:      make([]scanFinding, 0, len(ev.findings)),
		Policy:        scanPolicyRef{Name: pol.Name, Version: pol.Version},
		ScanLatencyMs: float64(time.Since(call.start).Microseconds()) / 1000,
	}
	// The stripped text is not returned here, only the redacted one.
	if ev.action == policy.ActionRedact && len(ev.rewritten) == 1 {
		out.RedactedText = &ev.rewritten[0]
	}
	if ev.action == policy.ActionRoute {
		out.RouteTo = ev.route
	}
	coarse := who.Scope == apikeys.ScopeProxy
	call.byType = map[string]int{}
	for _, f := range ev.findings {
		call.byType[f.Type]++
		category := f.Category
		if coarse && scanNamedTypes[f.Type] {
			category = f.Type
		}
		sf := scanFinding{
			Type:     f.Type,
			Category: category,
			Severity: f.Severity,
			Position: api.PositionNone,
			Action:   scanVerdict(ev.actionOf(f)),
		}
		if !scanUnplacedTypes[f.Type] && !scanUnplacedTypes[f.Category] {
			// One text was scanned, as one segment: the finding's offsets
			// are into it.
			span := api.FindingSpan(text, f)
			sf.Position = span.Position
			sf.Start, sf.End, sf.StartUTF16, sf.EndUTF16 = span.Start, span.End, span.StartUTF16, span.EndUTF16
		}
		out.Findings = append(out.Findings, sf)
	}

	call.done(out.Action, false)
	writeScanJSON(w, http.StatusOK, out)
}

// messageTexts returns the content of the first n messages of a body
// evaluate built, after a rewrite: its texts. A message edited down to
// nothing is an empty text.
func messageTexts(body []byte, n int) ([]string, error) {
	var doc struct {
		Messages []struct {
			Content *string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if len(doc.Messages) < n {
		return nil, errors.New("the rewritten body does not hold the messages it was built from")
	}
	out := make([]string, n)
	for i, m := range doc.Messages[:n] {
		if m.Content == nil {
			return nil, errors.New("a rewritten message has no text content")
		}
		out[i] = *m.Content
	}
	return out, nil
}

// scanActs reports whether a verdict stops the text or rewrites it, as
// opposed to letting it through as it is.
func scanActs(a policy.Action) bool {
	return a == policy.ActionBlock || a == policy.ActionRoute || a == policy.ActionStrip || a == policy.ActionRedact
}

// scanVerdict is an action as the scan API names it. LOG forwards the
// request untouched on the proxy path, which to a caller is a PASS.
func scanVerdict(a policy.Action) string {
	switch a {
	case policy.ActionBlock, policy.ActionRoute, policy.ActionStrip, policy.ActionRedact, policy.ActionWarn:
		return string(a)
	}
	return string(policy.ActionPass)
}
