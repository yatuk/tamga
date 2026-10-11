package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/errkind"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/providers"
	"github.com/yatuk/tamga/internal/ratelimit"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/webhooks"
)

// The LiteLLM route: LiteLLM's Generic Guardrail API, served over the scan
// path. A LiteLLM proxy configured with
//
//	guardrail: generic_guardrail_api
//	api_base:  https://<tamga>/api/v1
//
// posts the texts of a request (input_type "request") or of an answer
// (input_type "response") to <api_base>/beta/litellm_basic_guardrail_api,
// with the tool definitions and the tool calls that go with them, and does
// what the reply says: stop, go on, or go on with other texts and tools.
//
// It is an adapter and decides nothing of its own. A request goes through
// scanHandler.evaluateInput, the path POST /api/v1/scan takes: its texts in
// their roles, its tool definitions as tool_definition, and the arguments of
// its tool calls as the assistant's, which is how the proxy path reads the
// same request. An answer goes through scanResponseBody, the path the proxy
// takes for an answer, so it is judged by output_rules and by nothing else.
//
// Unlike the scan API, a call here is real traffic, and it is recorded as a
// request on the proxy path is: see record.
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

// litellmMaxTools is the most tool definitions one call may carry, and the
// most tool calls.
const litellmMaxTools = 256

// litellmAnswerBudget is how long the scan of one call's answer may take, all
// its texts and tool calls together. It is a bound, not a target.
//
// output_rules.scan_window_ms is not it. On the proxy path that window is
// how long a buffered answer may be held back from a client that is waiting
// for it, and output_rules.fail_open says what becomes of the answer when
// the window runs out. Here LiteLLM already has the answer and is waiting
// for a verdict, a call without one is refused, and there is no fail_open:
// 200 ms refused every answer the scanners needed longer for, which a long
// one is. What bounds the work here is the size limit (scanMaxBodyBytes of
// text in a call); the budget is several times what the scanners need for
// that much, so that it ends a scan that is stuck and not one that is long.
// A scan that reaches it is still answered with a 503.
const litellmAnswerBudget = 10 * time.Second

// litellmMaxModel is how much of the model's name is kept.
const litellmMaxModel = 128

// litellmEndpoint is the endpoint a recorded call names, before its
// direction: the route as LiteLLM calls it.
const litellmEndpoint = "/api/v1/beta/litellm_basic_guardrail_api"

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
	litellmErrTextsLarge  = "texts, tools and tool_calls exceed 262144 bytes in total"
	litellmErrTooMany     = "more than 256 texts"
	litellmErrTooManyTool = "more than 256 tools or tool_calls"
	litellmErrInputType   = "input_type must be request or response"
	litellmErrUnavailable = "The security scan could not be completed, so there is no verdict."
)

// litellmRequest is what the route reads of a call. The rest of the contract
// (images, request_data, request_headers, litellm_trace_id,
// additional_provider_specific_params) is accepted and not looked at.
type litellmRequest struct {
	Texts     []string `json:"texts"`
	InputType *string  `json:"input_type"`
	// StructuredMessages is used for one thing: the role of each text.
	StructuredMessages []json.RawMessage `json:"structured_messages"`
	// Tools are the tool definitions of a request, in OpenAI's shape. They
	// are scanned on a request and not looked at on an answer, which has
	// no rule for them.
	Tools []json.RawMessage `json:"tools"`
	// ToolCalls are the tool calls of the conversation (a request) or of
	// the model (an answer), in OpenAI's shape. Their arguments are scanned.
	ToolCalls []json.RawMessage `json:"tool_calls"`
	CallID    string            `json:"litellm_call_id"`
	// Model is recorded with the call. It decides nothing.
	Model string `json:"model"`
}

// litellmResponse is the reply. It has fields of the contract and no others.
type litellmResponse struct {
	Action        string `json:"action"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	// Texts is set only with GUARDRAIL_INTERVENED: one text for each one
	// received, in the same order.
	Texts []string `json:"texts,omitempty"`
	// Tools is set only with GUARDRAIL_INTERVENED, and only when a tool
	// definition was changed: every tool received, in the same order.
	// LiteLLM keeps its own tools when the field is absent.
	Tools []json.RawMessage `json:"tools,omitempty"`
}

type litellmHandler struct {
	scan *scanHandler
	// fallback limits callers when the policy has no rate limit.
	fallback *ratelimit.Limiter
	// answerBudget bounds the scan of one answer: litellmAnswerBudget.
	answerBudget time.Duration
}

// NewScanHandlers returns the handler for POST /api/v1/scan and the one for
// LiteLLM's guardrail calls. They are one scan path: the same scanners,
// policy evaluation and classifier guard, and the same slots, so the two
// together take no more processors than the scan API did alone.
func NewScanHandlers(cfg HandlerConfig, opts ScanOptions) (scan, litellm http.Handler) {
	s := NewScanHandler(cfg, opts).(*scanHandler)
	limit := &policy.Policy{RateLimit: &policy.RateLimit{MaxRequestsPerMinute: litellmDefaultRPM}}
	return s, &litellmHandler{
		scan:         s,
		fallback:     ratelimit.NewLimiter(func() *policy.Policy { return limit }),
		answerBudget: litellmAnswerBudget,
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
	answer := direction == "response"
	tools, toolCalls := litellmObjects(req.Tools), litellmObjects(req.ToolCalls)
	if answer {
		tools = nil
	}
	call.bridge.direction = direction
	call.bridge.texts = len(req.Texts)
	call.bridge.tools = len(tools)
	call.bridge.toolCalls = len(toolCalls)
	call.bridge.callID = clipID(req.CallID)
	model := litellmModel(req.Model)

	if len(req.Texts) > litellmMaxTexts {
		refuse(http.StatusRequestEntityTooLarge, "too_large", litellmErrTooMany)
		return
	}
	if len(tools) > litellmMaxTools || len(toolCalls) > litellmMaxTools {
		refuse(http.StatusRequestEntityTooLarge, "too_large", litellmErrTooManyTool)
		return
	}
	// What is scanned is held to the scan API's limit, whatever else the
	// body carries: the texts, and the tool definitions and tool calls as
	// they were sent.
	total := 0
	var present []int // the texts that are not empty, by their place in the call
	for i, t := range req.Texts {
		total += len(t)
		if t != "" {
			present = append(present, i)
		}
	}
	for _, raw := range tools {
		total += len(raw)
	}
	for _, raw := range toolCalls {
		total += len(raw)
	}
	if total > scanMaxBodyBytes {
		refuse(http.StatusRequestEntityTooLarge, "too_large", litellmErrTextsLarge)
		return
	}

	var scanned time.Duration
	reply := func(ev scanEval, out litellmResponse) {
		for _, hd := range ev.headers {
			w.Header().Set(hd[0], hd[1])
		}
		w.Header().Set(litellmActionHeader, scanVerdict(ev.action))
		call.done(scanVerdict(ev.action), false)
		writeScanJSON(w, http.StatusOK, out)
		// After the reply: the record waits for nothing and holds nothing up.
		b.record(r.Context(), call, requestID, who, pol, direction, model, ev, scanned)
	}

	// A call with nothing the scanners read: a request that is only an
	// image, an answer with neither text nor a tool call.
	if len(present) == 0 && len(tools) == 0 && len(toolCalls) == 0 {
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
	if !answer {
		roles := litellmRoles(req.Texts, req.StructuredMessages)
		msgs := make([]scanMessage, len(present))
		for j, i := range present {
			msgs[j] = scanMessage{role: roles[i], text: req.Texts[i]}
		}
		in, built := newScanInput(msgs, toolCalls, tools)
		if !built {
			unavailable("segment_mismatch")
			return
		}
		ev, outcome = h.evaluateInput(r.Context(), call, requestID, who, pol, in)
		// A finding names its place in what LiteLLM sent, not in the
		// request the scan was run on.
		for i := range ev.findings {
			ev.findings[i].Path = litellmFindingPath(ev.findings[i].Path, present)
		}
	} else {
		texts := make([]string, len(present))
		for j, i := range present {
			texts[j] = req.Texts[i]
		}
		ev, outcome = b.evaluateOutput(r.Context(), call, pol, texts, present, toolCalls)
	}
	if outcome != "" {
		unavailable(outcome)
		return
	}
	scanned = time.Since(call.start)

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
		// The tools go back only when one was changed, and then all of
		// them: LiteLLM puts the list it gets in place of its own.
		if ev.tools != nil {
			if len(ev.tools) != len(tools) || len(tools) != len(req.Tools) {
				unavailable("rewrite_mismatch")
				return
			}
			out.Tools = ev.tools
		}
	}
	reply(ev, out)
}

// litellmObjects returns the entries of a list LiteLLM sent, without the
// nulls: a tool or a tool call that is null is none.
func litellmObjects(list []json.RawMessage) []json.RawMessage {
	out := list[:0:0]
	for _, raw := range list {
		if len(raw) > 0 && string(raw) != "null" {
			out = append(out, raw)
		}
	}
	return out
}

// litellmModel is the model name as it is recorded: cut to length, and
// dropped unless it is made of the characters model names are made of. It
// is the caller's text and goes into the request log.
func litellmModel(model string) string {
	if len(model) > litellmMaxModel {
		return ""
	}
	for _, r := range model {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("._:/@+-", r):
		default:
			return ""
		}
	}
	return model
}

// litellmFindingPath turns the path of a finding in the request evaluate read into
// its place in the call: "messages[1].content" is the second text that was
// not empty, the tool calls are the message after the texts.
func litellmFindingPath(path string, present []int) string {
	if rest, ok := strings.CutPrefix(path, indexed("messages", len(present))+".tool_calls"); ok {
		return "tool_calls" + rest
	}
	for j, i := range present {
		if path == indexed("messages", j)+".content" {
			return indexed("texts", i)
		}
	}
	return path
}

// record publishes the call as a request's own event, the one the proxy path
// publishes when a request is over (publishEvent). The request log, the
// console's live feed and the alerts take it from there, so a block over the
// bridge is stored and alerted on like a block on the proxy path.
//
// What the event holds: the verdict, the findings with the matches the
// scanners masked, the Tamga key's organisation, the policy, the latency,
// and the model LiteLLM named. The provider is providers.LiteLLMBridge and
// the endpoint is the route and the direction; that is how a row says it
// came this way. No text is on the event, and no body for the analyzer.
//
// Every request call is an event, as every request on the proxy path is. An
// answer call is one only when something was found or done: LiteLLM calls
// again and again for one streamed answer, and the answer of a request that
// is already a row would otherwise be a row for every few chunks.
//
// An answer's event carries the verdict as the action and the findings as
// the findings, like a request's, so the console shows what stopped it; the
// output fields say the same, and Direction tells the alerts it was an
// answer.
//
// Publishing does not block: the bus drops when its buffer is full, and the
// alert queue does the same.
func (b *litellmHandler) record(ctx context.Context, call *scanCall, requestID string, who caller, pol *policy.Policy,
	direction, model string, ev scanEval, scanned time.Duration) {
	cfg := b.scan.cfg
	answer := direction == "response"
	if cfg.Bus == nil || pol == nil || (answer && len(ev.findings) == 0 && !scanActs(ev.action)) {
		return
	}
	findings := append([]scanner.Finding(nil), ev.findings...)
	applyFindingActionTaken(findings, ev.action)
	risk := scanner.CalculateRisk(findings)
	none := scanner.RiskScore{Level: "none", Breakdown: map[string]float64{}}

	e := events.Event{
		RequestID:      requestID,
		OrgID:          call.org,
		OrgVerified:    who.verified(),
		Provider:       providers.LiteLLMBridge,
		Model:          model,
		ModelFamily:    extractModelFamily(model),
		EventType:      "request_scanned",
		Findings:       findings,
		Action:         string(ev.action),
		Endpoint:       litellmEndpoint + " (" + direction + ")",
		ScanLatencyMs:  durationMs(scanned),
		TotalLatencyMs: durationMs(time.Since(call.start)),
		Timestamp:      time.Now().UTC(),
		InputRisk:      risk,
		OutputRisk:     none,
		PolicyName:     pol.Name,
		PolicyVersion:  pol.Version,
	}
	if ev.action == policy.ActionBlock {
		e.EventType = "request_blocked"
	}
	if answer {
		e.Direction = "output"
		e.InputRisk, e.OutputRisk = none, risk
		e.OutputFindings, e.OutputAction, e.OutputScanned = findings, string(ev.action), true
	}
	cfg.Bus.PublishContext(ctx, e)

	// The rules that matched are told, as on the proxy path: for a request,
	// whatever was decided short of a PASS.
	if answer || cfg.Alerts == nil || ev.action == policy.ActionPass {
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
		RequestID: requestID,
		Timestamp: e.Timestamp,
		OrgID:     org,
		Provider:  providers.LiteLLMBridge,
		Model:     model,
		Action:    string(ev.action),
		Direction: "input",
		EventType: "rule_notify",
		InputRisk: risk.Percentage,
		Findings:  webhooks.AlertFindings(findings),
	})
}

// evaluateOutput is the answer path: each text is read as the content of one
// assistant message, and the tool calls as those of one more, and handed to
// scanResponseBody, which scans it with the proxy's registry and its
// output-only registry and applies output_rules. present is the place of
// each text in the call, for the findings' paths.
//
// A tool call's arguments are scanned and never rewritten: the contract's
// reply has no field to return them in. A value there that output_rules
// would mask blocks the answer, and the reason says so.
//
// The proxy reads an answer only when output_rules is enabled, so with it off
// every answer is a PASS here too. Where the proxy lets output_rules.fail_open
// decide about an answer whose scan was cut short, this gives no verdict:
// whether to fail open is LiteLLM's setting on this side of the bridge.
//
// The whole answer is scanned within one budget (see litellmAnswerBudget),
// not each part within output_rules.scan_window_ms. A window set longer than
// the budget is kept: whoever set it meant to wait that long.
func (b *litellmHandler) evaluateOutput(ctx context.Context, call *scanCall, pol *policy.Policy, texts []string, present []int, toolCalls []json.RawMessage) (ev scanEval, outcome string) {
	h := b.scan
	ev.action = policy.ActionPass
	ev.actionOf = func(f scanner.Finding) policy.Action { return pol.EvaluateOutput([]scanner.Finding{f}) }
	if pol.OutputRules == nil || !pol.OutputRules.Enabled {
		return ev, ""
	}
	budget := b.answerBudget
	if win := time.Duration(pol.OutputRules.ScanWindowMs) * time.Millisecond; win > budget {
		budget = win
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	pipelineCfg, _ := h.pipeline()
	registry := h.cfg.Registry.Without(scanExcluded...)
	provider := ProviderFor(scanProvider)

	rewritten := append([]string(nil), texts...)
	redacted := false
	// The texts, and after them the tool calls as one more message.
	parts := len(texts)
	if len(toolCalls) > 0 {
		parts++
	}
	for i := 0; i < parts; i++ {
		isCalls := i == len(texts)
		message := scanWireMessage{Role: string(extract.RoleAssistant)}
		if isCalls {
			message.ToolCalls = toolCalls
		} else {
			message.Content = &texts[i]
		}
		raw, err := json.Marshal(map[string]interface{}{
			"choices": []map[string]interface{}{{"index": 0, "message": message}},
		})
		if err != nil {
			return ev, "segment_mismatch"
		}
		if isCalls {
			// Everything read here has to be a tool call's.
			read, ok := extract.ExtractResponse(scanProvider, raw)
			if !ok {
				return ev, "segment_mismatch"
			}
			for _, seg := range read.Segments {
				if !strings.HasPrefix(seg.Path, answerPath+".tool_calls") {
					return ev, "segment_mismatch"
				}
			}
		} else if got, ok := answerText(raw); !ok || got != texts[i] {
			return ev, "segment_mismatch"
		}
		// No window of its own: ctx carries the budget of the whole answer.
		res, err := scanResponseBody(ctx, registry, h.cfg.OutputOnlyRegistry, pol, provider, raw, 0, pipelineCfg)
		if res.incomplete != "" {
			// It was to be masked and the scan behind the masking was cut
			// short. The proxy withholds such an answer with a 503.
			call.logger.Error().Str("reason", res.incomplete).Msg("answer scan incomplete; not masked")
			return ev, "redaction_incomplete"
		}
		if err != nil && res.action != policy.ActionBlock {
			// Logged by kind only: a scanner's error can quote what it read.
			call.logger.Error().Bool("degraded", true).
				Str("reason", scanner.IncompleteReason(err)).Str("error_kind", errkind.Of(err)).Msg("answer scan incomplete")
			return ev, "scan_incomplete"
		}
		for _, f := range res.findings {
			if rest, ok := strings.CutPrefix(f.Path, answerPath+".tool_calls"); ok {
				f.Path = "tool_calls" + rest
			} else if !isCalls {
				f.Path = "texts[" + strconv.Itoa(present[i]) + "]"
			}
			ev.findings = append(ev.findings, f)
		}

		if isCalls && res.action == policy.ActionRedact {
			ev.action = policy.ActionBlock
			ev.fallback = "REDACT could not be carried out (" + redactToolCall + ")"
			ev.headers = append(ev.headers, [2]string{"X-Tamga-Redact-Fallback", "block:" + redactToolCall})
			continue
		}
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
			if !own && ev.fallback == "" {
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

// answerPath is where the extractor finds the message of an answer
// evaluateOutput built.
const answerPath = "choices[0].message"

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
