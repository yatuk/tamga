package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/scanner"
)

// The LiteLLM route: LiteLLM's Generic Guardrail API, served over the scan
// path. A LiteLLM proxy configured with
//
//	guardrail: generic_guardrail_api
//	api_base:  https://<tamga>/api/v1
//
// posts the texts of a request (input_type "request") or of an answer
// (input_type "response") to <api_base>/beta/litellm_basic_guardrail_api and
// does what the reply says: stop, go on, or go on with other texts.
//
// It is an adapter and decides nothing of its own. A request's texts go
// through scanHandler.evaluate, the path POST /api/v1/scan takes. An answer's
// texts go through scanResponseBody, the path the proxy takes for an answer,
// so they are judged by output_rules and by nothing else.
//
// The contract is the one in LiteLLM's
// litellm/proxy/guardrails/guardrail_hooks/generic_guardrail_api and
// litellm/types/proxy/guardrails/guardrail_hooks/generic_guardrail_api.py at
// v1.104.2, read on 2026-10-10. See docs/litellm.md.

// litellmMaxBodyBytes is the largest body the route reads. It is the proxy
// path's default and four times the scan API's: LiteLLM sends a conversation
// as texts and again as structured_messages, with the tool definitions.
const litellmMaxBodyBytes = 1024 * 1024

// litellmMaxTexts is the most texts one call may carry.
const litellmMaxTexts = 256

// litellmDefaultRPM is the per-key limit when the policy sets none. LiteLLM
// calls once for every request and once more for every answer, so the scan
// API's 60 a minute would refuse a deployment at one request a second.
const litellmDefaultRPM = 600

// litellmKeyHeader is where LiteLLM puts the api_key of a guardrail. On this
// route it is read as X-Tamga-Key is.
const litellmKeyHeader = "X-Api-Key"

// litellmActionHeader names the policy's own action on a reply. The contract
// has three actions; the policy has more (WARN and PASS are both NONE).
const litellmActionHeader = "X-Tamga-Action"

// The actions of the contract.
const (
	litellmNone       = "NONE"
	litellmBlocked    = "BLOCKED"
	litellmIntervened = "GUARDRAIL_INTERVENED"
)

// The route's error messages. Like the scan API's they are fixed.
const (
	litellmErrTooLarge    = "request body exceeds 1048576 bytes"
	litellmErrTextsLarge  = "texts exceed 262144 bytes in total"
	litellmErrTooMany     = "more than 256 texts"
	litellmErrInputType   = "input_type must be request or response"
	litellmErrUnavailable = "The security scan could not be completed, so there is no verdict."
)

// litellmRequest is what the route reads of a call. The rest of the contract
// (images, tools, tool_calls, request_data, request_headers, model,
// additional_provider_specific_params) is accepted and not looked at.
type litellmRequest struct {
	Texts     []string `json:"texts"`
	InputType *string  `json:"input_type"`
	// StructuredMessages is used for one thing: the role of each text.
	StructuredMessages []json.RawMessage `json:"structured_messages"`
	CallID             string            `json:"litellm_call_id"`
}

// litellmResponse is the reply. It has the contract's fields and no others.
type litellmResponse struct {
	Action        string `json:"action"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	// Texts is set only with GUARDRAIL_INTERVENED: one text for each one
	// received, in the same order.
	Texts []string `json:"texts,omitempty"`
}

type litellmHandler struct {
	scan *scanHandler
	// fallback limits callers when the policy has no rate limit.
	fallback *ratelimit.Limiter
}

// NewScanHandlers returns the handler for POST /api/v1/scan and the one for
// LiteLLM's guardrail calls. They are one scan path: the same scanners,
// policy evaluation and classifier guard, and the same slots, so the two
// together take no more processors than the scan API did alone.
func NewScanHandlers(cfg HandlerConfig, opts ScanOptions) (scan, litellm http.Handler) {
	s := NewScanHandler(cfg, opts).(*scanHandler)
	limit := &policy.Policy{RateLimit: &policy.RateLimit{MaxRequestsPerMinute: litellmDefaultRPM}}
	return s, &litellmHandler{
		scan:     s,
		fallback: ratelimit.NewLimiter(func() *policy.Policy { return limit }),
	}
}

func writeLiteLLMError(w http.ResponseWriter, status int, code, msg, requestID string) {
	writeScanJSON(w, status, map[string]interface{}{
		"error": map[string]string{"code": code, "message": msg, "request_id": requestID},
	})
}

func (b *litellmHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := b.scan
	requestID := uuid.Must(uuid.NewV7()).String()
	call := &scanCall{
		start:  time.Now(),
		logger: log.With().Str("component", "litellm_bridge").Str("request_id", requestID).Logger(),
		bridge: &bridgeCall{},
	}
	w.Header().Set("X-Tamga-Request-Id", requestID)
	w.Header().Set("Cache-Control", "no-store")

	fail := func(status int, outcome, code, msg string) {
		call.done(outcome, true)
		writeLiteLLMError(w, status, code, msg, requestID)
	}
	refuse := func(status int, outcome, msg string) { fail(status, outcome, "tamga_"+outcome, msg) }
	// unavailable answers a scan that did not run to the end. It is never a
	// reply with an action: NONE would let the text through unread, and
	// BLOCKED would call a failure a finding. LiteLLM counts a 503 as its
	// guardrail being unreachable and, unless it was told to fail open,
	// stops the request.
	unavailable := func(outcome string) {
		w.Header().Set("Retry-After", "1")
		fail(http.StatusServiceUnavailable, outcome, "tamga_scan_unavailable", litellmErrUnavailable)
	}

	who, pol, raw, ok := h.admit(w, r, call, scanAdmission{
		bucket:       "litellm:",
		fallback:     b.fallback,
		maxBody:      litellmMaxBodyBytes,
		tooLarge:     litellmErrTooLarge,
		apiKeyHeader: litellmKeyHeader,
	}, refuse)
	if !ok {
		return
	}

	var req litellmRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		refuse(http.StatusBadRequest, "invalid", scanErrJSON)
		return
	}
	if req.InputType == nil || (*req.InputType != "request" && *req.InputType != "response") {
		refuse(http.StatusBadRequest, "invalid", litellmErrInputType)
		return
	}
	direction := *req.InputType
	call.bridge.direction = direction
	call.bridge.texts = len(req.Texts)
	call.bridge.callID = clipID(req.CallID)

	if len(req.Texts) > litellmMaxTexts {
		refuse(http.StatusRequestEntityTooLarge, "too_large", litellmErrTooMany)
		return
	}
	// What is scanned is held to the scan API's limit, whatever else the
	// body carries.
	total := 0
	var present []int // the texts that are not empty, by their place in the call
	for i, t := range req.Texts {
		total += len(t)
		if t != "" {
			present = append(present, i)
		}
	}
	if total > scanMaxBodyBytes {
		refuse(http.StatusRequestEntityTooLarge, "too_large", litellmErrTextsLarge)
		return
	}

	reply := func(ev scanEval, out litellmResponse) {
		for _, hd := range ev.headers {
			w.Header().Set(hd[0], hd[1])
		}
		w.Header().Set(litellmActionHeader, scanVerdict(ev.action))
		call.done(scanVerdict(ev.action), false)
		writeScanJSON(w, http.StatusOK, out)
	}

	// A call with no text: an answer that is only a tool call, a request
	// that is only an image. There is nothing here the scanners read.
	if len(present) == 0 {
		reply(scanEval{action: policy.ActionPass}, litellmResponse{Action: litellmNone})
		return
	}

	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		fail(http.StatusServiceUnavailable, "busy", "tamga_scan_busy", scanErrBusy)
		return
	}

	if pol == nil {
		unavailable("policy_unavailable")
		return
	}

	var ev scanEval
	var outcome string
	if direction == "request" {
		roles := litellmRoles(req.Texts, req.StructuredMessages)
		msgs := make([]scanMessage, len(present))
		for j, i := range present {
			msgs[j] = scanMessage{role: roles[i], text: req.Texts[i]}
		}
		ev, outcome = h.evaluate(r.Context(), call, requestID, who, pol, msgs)
	} else {
		texts := make([]string, len(present))
		for j, i := range present {
			texts[j] = req.Texts[i]
		}
		ev, outcome = b.evaluateOutput(r.Context(), call, pol, texts)
	}
	if outcome != "" {
		unavailable(outcome)
		return
	}

	call.byType = map[string]int{}
	for _, f := range ev.findings {
		call.byType[f.Type]++
	}

	out := litellmResponse{Action: litellmNone}
	switch ev.action {
	case policy.ActionBlock:
		out.Action = litellmBlocked
		out.BlockedReason = litellmReason(direction, ev, who.Scope == apikeys.ScopeProxy, requestID)
	case policy.ActionRedact, policy.ActionStrip:
		if len(ev.rewritten) != len(present) {
			unavailable("rewrite_mismatch")
			return
		}
		// One text back for each one received: LiteLLM puts them where it
		// took them from, by position.
		out.Action = litellmIntervened
		out.Texts = append([]string(nil), req.Texts...)
		for j, i := range present {
			out.Texts[i] = ev.rewritten[j]
		}
	}
	reply(ev, out)
}

// evaluateOutput is the answer path: each text is read as the content of one
// assistant message and handed to scanResponseBody, which scans it with the
// proxy's registry and its output-only registry and applies output_rules.
//
// The proxy reads an answer only when output_rules is enabled, so with it off
// every answer is a PASS here too. Where the proxy lets output_rules.fail_open
// decide about an answer whose scan was cut short, this gives no verdict:
// whether to fail open is LiteLLM's setting on this side of the bridge.
func (b *litellmHandler) evaluateOutput(ctx context.Context, call *scanCall, pol *policy.Policy, texts []string) (ev scanEval, outcome string) {
	h := b.scan
	ev.action = policy.ActionPass
	ev.actionOf = func(f scanner.Finding) policy.Action { return pol.EvaluateOutput([]scanner.Finding{f}) }
	if pol.OutputRules == nil || !pol.OutputRules.Enabled {
		return ev, ""
	}
	winMs := pol.OutputRules.ScanWindowMs
	if winMs <= 0 {
		winMs = 200
	}
	pipelineCfg, _ := h.pipeline()
	registry := h.cfg.Registry.Without(scanExcluded...)
	provider := ProviderFor(scanProvider)

	rewritten := append([]string(nil), texts...)
	redacted := false
	for i, text := range texts {
		raw, err := json.Marshal(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"index":   0,
				"message": map[string]string{"role": string(extract.RoleAssistant), "content": text},
			}},
		})
		if err != nil {
			return ev, "segment_mismatch"
		}
		if got, ok := answerText(raw); !ok || got != text {
			return ev, "segment_mismatch"
		}
		res, err := scanResponseBody(ctx, registry, h.cfg.OutputOnlyRegistry, pol, provider, raw, winMs, pipelineCfg)
		if res.incomplete != "" {
			// It was to be masked and the scan behind the masking was cut
			// short. The proxy withholds such an answer with a 503.
			call.logger.Error().Str("reason", res.incomplete).Msg("answer scan incomplete; not masked")
			return ev, "redaction_incomplete"
		}
		if err != nil && res.action != policy.ActionBlock {
			// Logged by kind only: a scanner's error can quote what it read.
			call.logger.Error().Bool("degraded", true).Msg("answer scan incomplete")
			return ev, "scan_incomplete"
		}
		ev.findings = append(ev.findings, res.findings...)

		switch res.action {
		case policy.ActionBlock:
			ev.action = policy.ActionBlock
			// scanResponseBody blocks an answer it could not mask. Nothing
			// in block_on was found then.
			own := false
			for _, f := range res.findings {
				if ev.actionOf(f) == policy.ActionBlock {
					own = true
					break
				}
			}
			if !own {
				ev.fallback = "REDACT could not be carried out"
			}
		case policy.ActionRedact:
			masked, ok := answerText(res.redacted)
			if !ok {
				ev.action = policy.ActionBlock
				ev.fallback = "REDACT could not be carried out"
				continue
			}
			rewritten[i] = masked
			redacted = true
		}
	}
	if ev.action != policy.ActionBlock && redacted {
		ev.action = policy.ActionRedact
		ev.rewritten = rewritten
	}
	return ev, ""
}

// answerText returns the text of the one message of an answer evaluateOutput
// built, as the extractor reads it. A message masked down to nothing reads
// as no text at all.
func answerText(raw []byte) (string, bool) {
	res, ok := extract.ExtractResponse(scanProvider, raw)
	if !ok || len(res.Segments) > 1 {
		return "", false
	}
	if len(res.Segments) == 0 {
		return "", true
	}
	return res.Segments[0].Text, true
}

// litellmRole maps a chat role to one of the four the policy knows.
func litellmRole(role string) string {
	switch role {
	case "system", "developer":
		return string(extract.RoleSystem)
	case "assistant":
		return string(extract.RoleAssistant)
	case "tool", "function":
		return string(extract.RoleTool)
	}
	return string(extract.RoleUser)
}

// litellmRoles returns the role each text is scanned in.
//
// The contract's texts carry no role. On the chat routes LiteLLM also sends
// structured_messages, the conversation in OpenAI's shape. When the texts of
// those messages, taken in order, are exactly the texts of the call, each
// text takes the role of the message it came from. In every other case (no
// structured_messages, or a call whose texts were put together some other
// way) every text is the user's, which is what the scan API assumes of a
// text sent without a role.
func litellmRoles(texts []string, structured []json.RawMessage) []string {
	roles := make([]string, len(texts))
	for i := range roles {
		roles[i] = string(extract.RoleUser)
	}
	if len(structured) == 0 {
		return roles
	}

	var flatText, flatRole []string
	add := func(text, role string) {
		if text != "" {
			flatText = append(flatText, text)
			flatRole = append(flatRole, role)
		}
	}
	for _, rawMsg := range structured {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(rawMsg, &m); err != nil {
			return roles
		}
		if len(m.Content) == 0 {
			continue
		}
		role := litellmRole(m.Role)
		var whole string
		if err := json.Unmarshal(m.Content, &whole); err == nil {
			add(whole, role)
			continue
		}
		var parts []struct {
			Text *string `json:"text"`
		}
		if err := json.Unmarshal(m.Content, &parts); err != nil {
			return roles
		}
		for _, p := range parts {
			if p.Text != nil {
				add(*p.Text, role)
			}
		}
	}

	var present []int
	for i, t := range texts {
		if t != "" {
			present = append(present, i)
		}
	}
	if len(present) != len(flatText) {
		return roles
	}
	for j, i := range present {
		if texts[i] != flatText[j] {
			return roles
		}
	}
	for j, i := range present {
		roles[i] = flatRole[j]
	}
	return roles
}

// litellmMaxKinds is how many kinds of finding a blocked_reason names.
const litellmMaxKinds = 5

// litellmReason is the blocked_reason of a BLOCKED reply. LiteLLM hands it
// to its own caller as the error message, so it names the kinds of finding
// the policy blocks on and never what was matched. A competitor's name is
// what was matched, and a custom entity's name is the operator's watch list,
// which a proxy key is not told (see scanNamedTypes).
func litellmReason(direction string, ev scanEval, coarse bool, requestID string) string {
	seen := map[string]bool{}
	var kinds []string
	for _, f := range ev.findings {
		if ev.actionOf == nil || ev.actionOf(f) != policy.ActionBlock {
			continue
		}
		kind := f.Type
		named := f.Type == "competitor" || (coarse && scanNamedTypes[f.Type])
		if f.Category != "" && f.Category != f.Type && !named {
			kind += "/" + f.Category
		}
		if !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	more := ""
	if len(kinds) > litellmMaxKinds {
		kinds, more = kinds[:litellmMaxKinds], " and others"
	}

	why := "the policy's rules"
	switch {
	case len(kinds) > 0:
		why = strings.Join(kinds, ", ") + more
	case ev.fallback != "":
		why = ev.fallback
	}
	what := "Request blocked by Tamga policy: "
	if direction == "response" {
		what = "Response blocked by Tamga output policy: "
	}
	return what + why + ". Tamga request id " + requestID + "."
}

// clipID shortens an identifier LiteLLM sent, for the log line.
func clipID(id string) string {
	const limit = 64
	if len(id) > limit {
		id = strings.ToValidUTF8(id[:limit], "")
	}
	return id
}
