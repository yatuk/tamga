package proxy

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
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
var scanUnplacedTypes = map[string]bool{"injection": true, "jailbreak": true, "content_moderation": true, "classifier": true}

// scanNamedTypes are the finding types whose category is a name the operator
// chose: a custom entity, a competitor. Together those names are the watch
// list. A proxy key is the one handed to applications and browser extensions,
// so it is told the type and not the name; the other scopes already read the
// policy.
var scanNamedTypes = map[string]bool{"custom": true, "competitor": true}

// scanClassifierHeader says the inline classifier did not read all of the
// text, on a verdict that was given all the same.
const scanClassifierHeader = "X-Tamga-Classifier"

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
	// Start and End are byte offsets into the submitted text as UTF-8. They
	// are absent for a finding with no position, and for the types in
	// scanUnplacedTypes.
	Start  *int   `json:"start,omitempty"`
	End    *int   `json:"end,omitempty"`
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
	RedactedText  *string       `json:"redacted_text,omitempty"`
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
}

// done writes the call's one log line and counts it.
func (c *scanCall) done(outcome string, refused bool) {
	api.CountScanAPI(outcome)
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
	ev.Str("outcome", outcome).
		Str("key_id", c.keyID).
		Str("org_id", c.org).
		Str("key_role", c.keyRole).
		Str("text_role", c.textRole).
		Int("bytes", c.bytes).
		Int("findings", total).
		Dict("findings_by_type", counts).
		Float64("latency_ms", float64(time.Since(c.start).Microseconds())/1000).
		Msg("scan api call")
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
// caller's rate limit.
func (h *scanHandler) authenticate(r *http.Request) (who caller, bucket string, ok bool) {
	if strings.TrimSpace(r.Header.Get(keyHeader)) != "" {
		who, errType, _ := authenticate(r, h.cfg)
		if errType != "" || !who.verified() {
			return caller{}, "", false
		}
		return who, "scan:tk:" + who.KeyID, true
	}
	key := r.Header.Get(adminKeyHeader)
	if key == "" || h.opts.AdminKey == "" {
		return caller{}, "", false
	}
	if subtle.ConstantTimeCompare([]byte(key), []byte(h.opts.AdminKey)) != 1 {
		return caller{}, "", false
	}
	return caller{}, "scan:admin", true
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

// checkRate counts the call in the caller's scan bucket, which is apart from
// the one its proxy requests are counted in: a CI job that scans in a loop
// must not use up the application's requests, nor the other way round.
func (h *scanHandler) checkRate(bucket string, pol *policy.Policy) ratelimit.AllowResult {
	if h.cfg.RateLimit != nil && pol != nil && pol.RateLimit != nil && pol.RateLimit.MaxRequestsPerMinute > 0 {
		return h.cfg.RateLimit.Check(bucket)
	}
	return h.fallback.Check(bucket)
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

	if !h.originAllowed(r) {
		refuse(http.StatusForbidden, "origin_refused", scanErrOrigin)
		return
	}

	who, bucket, ok := h.authenticate(r)
	if !ok {
		call.logger = call.logger.With().Str("client_ip", clientIP(r, trustedProxies(h.cfg))).Logger()
		refuse(http.StatusUnauthorized, "unauthorized", scanErrUnauthorized)
		return
	}
	call.keyID, call.keyRole = who.KeyID, who.Role
	if who.verified() {
		call.org = r.Header.Get("X-Tamga-Org-Id")
	}

	pol := h.cfg.GetPolicy()

	// Going over the limit is always refused. rate_limit.action_on_exceed
	// can say WARN on the proxy path, where the request is then forwarded;
	// a verdict has no such second outcome.
	res := h.checkRate(bucket, pol)
	if res.Limit > 0 {
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
	}
	if !res.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, res.RetryAfterS)))
		refuse(http.StatusTooManyRequests, "rate_limited", scanErrRateLimit)
		return
	}

	// Only JSON, said to be JSON. A web page can post text/plain to another
	// origin without asking first; it cannot do that with this type.
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		refuse(http.StatusUnsupportedMediaType, "unsupported_media_type", scanErrContentType)
		return
	}

	if r.ContentLength > scanMaxBodyBytes {
		refuse(http.StatusRequestEntityTooLarge, "too_large", scanErrTooLarge)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, scanMaxBodyBytes))
	call.bytes = len(raw)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			refuse(http.StatusRequestEntityTooLarge, "too_large", scanErrTooLarge)
			return
		}
		refuse(http.StatusBadRequest, "invalid", scanErrRead)
		return
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

	// One chat message holding the text, read back as the one segment the
	// proxy path would scan.
	body, err := json.Marshal(map[string]interface{}{
		"messages": []map[string]string{{"role": role, "content": text}},
	})
	var segs []extract.Segment
	if err == nil {
		if res, ok := extract.Extract(scanProvider, body); ok {
			segs = res.Segments
		}
	}
	if len(segs) != 1 || segs[0].Text != text || string(segs[0].Role) != role {
		unavailable("segment_mismatch")
		return
	}

	// The proxy path's registry, by segment, as handleProxy scans a chat
	// request. Not through the shared worker pool, and without load
	// shedding: a queue filled by scans would cost real traffic its
	// scanners, and a scan with scanners shed is not a verdict.
	ctx := r.Context()
	pipelineCfg := scanner.PipelineConfig{}
	strictMode := false
	if h.cfg.Config != nil {
		pipelineCfg.Mode = scanner.PipelineMode(h.cfg.Config.ScannerPipelineMode)
		pipelineCfg.Timeout = time.Duration(h.cfg.Config.ScannerPipelineTimeoutMs) * time.Millisecond
		strictMode = h.cfg.Config.StrictMode
	}
	findings, err := h.cfg.Registry.Without(scanExcluded...).ScanSegments(ctx, segs, pipelineCfg)
	if err != nil {
		// Logged by kind only: a scanner's error can quote what it read.
		call.logger.Error().Bool("degraded", errors.Is(err, scanner.ErrScanDegraded)).Msg("scan incomplete")
		unavailable("scan_incomplete")
		return
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
			unavailable("classifier_unavailable")
			return
		}
		res := classifySegments(ctx, h.opts.Classifier, set, requestID, segs, findings)
		// What it did read counts, also when it could not read everything.
		if len(res.findings) > 0 {
			findings = append(findings, res.findings...)
			action, _ = pol.EvaluateWithRole(findings, userRole, strictMode)
		}
		if res.err != nil || res.partial {
			call.logger.Warn().Bool("partial", res.partial).Msg("classifier did not read the text")
			// Text the classifier did not read cannot be called clean, so
			// a verdict that would let the text through as it is, is not
			// given. One that already stops or rewrites it stands: the
			// unread part could only have made it stricter, and the
			// header says it was not read.
			if !scanActs(action) {
				unavailable("classifier_unavailable")
				return
			}
			w.Header().Set(scanClassifierHeader, "incomplete")
		}
	}

	// REDACT that cannot be carried out is a BLOCK on the proxy path, so it
	// is one here. The vault is not used: with it on the proxy sends
	// numbered placeholders and keeps the originals to put back, and a scan
	// keeps nothing. The text shows the plain markers instead.
	var redactedText *string
	if action == policy.ActionRedact {
		reason := ""
		if n := scanner.UnplacedRedactions(findings); n > 0 {
			reason = "unplaced"
		} else if redacted, rwErr := rewrittenText(body, segs, scanner.RedactEdits(findings)); rwErr != nil {
			reason = "rewrite_failed"
		} else {
			redactedText = &redacted
		}
		if reason != "" {
			w.Header().Set("X-Tamga-Redact-Fallback", "block:"+reason)
			action = policy.ActionBlock
		}
	}

	// The same for STRIP. The stripped body itself is not returned.
	if action == policy.ActionStrip {
		actionOf := func(f scanner.Finding) policy.Action { return pol.FindingAction(f, userRole, strictMode) }
		if _, stripErr := stripSegments(scanProvider, body, segs, findings, actionOf, false); stripErr != nil {
			reason := "rewrite_failed"
			if errors.Is(stripErr, errStripRaw) || errors.Is(stripErr, errStripUnplaced) {
				reason = stripErr.Error()
			}
			w.Header().Set("X-Tamga-Strip-Fallback", "block:"+reason)
			action = policy.ActionBlock
		}
	}

	out := scanResponse{
		Action:        scanVerdict(action),
		Findings:      make([]scanFinding, 0, len(findings)),
		RedactedText:  redactedText,
		Policy:        scanPolicyRef{Name: pol.Name, Version: pol.Version},
		ScanLatencyMs: float64(time.Since(call.start).Microseconds()) / 1000,
	}
	coarse := who.Scope == apikeys.ScopeProxy
	call.byType = map[string]int{}
	for _, f := range findings {
		call.byType[f.Type]++
		category := f.Category
		if coarse && scanNamedTypes[f.Type] {
			category = f.Type
		}
		sf := scanFinding{
			Type:     f.Type,
			Category: category,
			Severity: f.Severity,
			Action:   scanVerdict(pol.FindingAction(f, userRole, strictMode)),
		}
		if f.Placed() && !scanUnplacedTypes[f.Type] && !scanUnplacedTypes[f.Category] {
			start, end := f.StartPos, f.EndPos
			sf.Start, sf.End = &start, &end
		}
		out.Findings = append(out.Findings, sf)
	}

	call.done(out.Action, false)
	writeScanJSON(w, http.StatusOK, out)
}

// rewrittenText applies edits to the one message of body as the proxy path
// would before forwarding it, and returns the text that message then holds.
func rewrittenText(body []byte, segs []extract.Segment, edits []extract.Edit) (string, error) {
	rewritten, err := extract.RewriteChecked(scanProvider, body, segs, edits)
	if err != nil {
		return "", err
	}
	res, ok := extract.Extract(scanProvider, rewritten)
	if !ok || len(res.Segments) > 1 {
		return "", errors.New("rewritten text did not read back as one message")
	}
	if len(res.Segments) == 0 {
		// A message edited down to nothing is no longer a segment.
		return "", nil
	}
	return res.Segments[0].Text, nil
}

// scanActs reports whether a verdict stops the text or rewrites it, as
// opposed to letting it through as it is.
func scanActs(a policy.Action) bool {
	return a == policy.ActionBlock || a == policy.ActionStrip || a == policy.ActionRedact
}

// scanVerdict is an action as the scan API names it. LOG forwards the
// request untouched on the proxy path, which to a caller is a PASS.
func scanVerdict(a policy.Action) string {
	switch a {
	case policy.ActionBlock, policy.ActionStrip, policy.ActionRedact, policy.ActionWarn:
		return string(a)
	}
	return string(policy.ActionPass)
}
