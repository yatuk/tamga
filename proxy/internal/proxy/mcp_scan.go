package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// What the MCP gateway reads in the messages it passes, and what it does with
// them. Everything is judged on the scan path (scanHandler.evaluateInput),
// with the proxy path's registry, policy and classifier:
//
//   - towards the agent, the text of a tools/list result (each tool as a tool
//     definition), and that of a tools/call result, a resources/read result,
//     a prompts/get result and the instructions of an initialize result, each
//     as a tool result (role "tool"), so a JSON tool result is also read
//     decoded and STRIP and REDACT apply as they do to a tool result in a
//     model request;
//   - towards the server, the arguments of a tools/call request, as the
//     arguments of a tool call (role "assistant"), and held to
//     output_rules.tool_destinations.
//
// A message is recognised by its shape, not by the request it answers: a
// response that arrives on a resumed stream (2025-11-25 and earlier) is read
// the same way as one that answers the POST it came on.

// The JSON-RPC error codes of the gateway. -32000 to -32019 are left to
// implementations by the MCP specification (2026-07-28, basic/index, error
// codes); these do not change.
const (
	mcpCodeBlocked         = -32010
	mcpCodeToolNotAllowed  = -32011
	mcpCodeToolChanged     = -32012
	mcpCodeScanUnavailable = -32013
	mcpCodeTooLarge        = -32014
	mcpCodeUpstream        = -32015
	mcpCodeUnauthorized    = -32016

	// JSON-RPC's own, and the specification's HeaderMismatch.
	mcpCodeParse          = -32700
	mcpCodeInvalidRequest = -32600
	mcpCodeHeaderMismatch = -32020
)

// The messages that go with the codes. They are fixed: nothing the agent,
// the server or a scanner said is in them.
const (
	mcpMsgBlocked         = "Blocked by Tamga security policy"
	mcpMsgToolNotAllowed  = "Tool not allowed by the Tamga MCP gateway"
	mcpMsgToolChanged     = "Tool definition changed during the session"
	mcpMsgScanUnavailable = "Tamga could not complete the security scan"
	mcpMsgTooLarge        = "Message larger than the Tamga MCP gateway accepts"
	mcpMsgUpstream        = "The MCP server could not be reached or sent a message Tamga cannot read"
	mcpMsgParse           = "Parse error"
	mcpMsgBatch           = "JSON-RPC batches are not accepted by the Tamga MCP gateway"
	mcpMsgCallNoID        = "tools/call must be a request with an id"
	mcpMsgHeaderMismatch  = "Header mismatch: Mcp-Method or Mcp-Name does not match the body"
)

// mcpToolChangeType is the finding of a tool whose definition changed after
// it was first listed in a session (a "rug pull").
const (
	mcpToolChangeType     = "mcp_tool_change"
	mcpToolChangeCategory = "tool_definition_changed"
)

// Why a request was refused although no finding asked for a BLOCK.
const (
	mcpFallbackRoute    = "the policy routes this content to an internal model, which the MCP gateway cannot do"
	mcpFallbackStrip    = "a tool call's arguments cannot be stripped"
	mcpFallbackParam    = "masked arguments are mirrored in Mcp-Param headers"
	mcpFallbackRewrite  = "the arguments could not be masked"
	mcpFallbackNotAllow = "the tool is not on the server's tools list"
	mcpFallbackResult   = "the result could not be rewritten"
)

// mcpRefusal is a JSON-RPC error the gateway answers in place of the server.
type mcpRefusal struct {
	code    int
	message string
	data    map[string]any
}

// mcpErrorBody is a JSON-RPC error response. id is nil for none.
func mcpErrorBody(id any, code int, message string, data map[string]any) []byte {
	type rpcError struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data,omitempty"`
	}
	return mcpEncode(struct {
		JSONRPC string   `json:"jsonrpc"`
		ID      any      `json:"id"`
		Error   rpcError `json:"error"`
	}{"2.0", id, rpcError{code, message, data}})
}

// mcpDecode reads one JSON value, keeping numbers as they were written.
func mcpDecode(raw []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}

// mcpEncode writes a value without HTML escaping, so that a message is
// changed only where the gateway changed it.
func mcpEncode(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// mcpPiece is one text the gateway reads in a message, and how to put a
// rewritten one back.
type mcpPiece struct {
	path string
	text string
	set  func(string)
}

// mcpPieces collects the texts of a result that reach the model: tool call
// content and structured content, resource contents, prompt messages, and
// a server's instructions. Images, audio and blobs are not read.
func mcpPieces(result map[string]any) []mcpPiece {
	var out []mcpPiece
	str := func(m map[string]any, key, path string) {
		if s, ok := m[key].(string); ok && s != "" {
			out = append(out, mcpPiece{path: path, text: s, set: func(v string) { m[key] = v }})
		}
	}
	var block func(b any, path string)
	block = func(b any, path string) {
		m, ok := b.(map[string]any)
		if !ok {
			return
		}
		switch m["type"] {
		case "resource":
			if res, ok := m["resource"].(map[string]any); ok {
				str(res, "text", path+".resource.text")
			}
		case "resource_link":
			str(m, "title", path+".title")
			str(m, "description", path+".description")
		case "image", "audio":
		default:
			// "text", and a type this revision does not define.
			str(m, "text", path+".text")
		}
	}

	str(result, "instructions", "result.instructions")
	if list, ok := result["content"].([]any); ok {
		for i, b := range list {
			block(b, "result.content["+strconv.Itoa(i)+"]")
		}
	}
	if sc, ok := result["structuredContent"]; ok && sc != nil {
		if raw := mcpEncode(sc); len(raw) > 0 {
			out = append(out, mcpPiece{path: "result.structuredContent", text: string(raw), set: func(v string) {
				// Masked or stripped in place, it is still JSON. Stripped
				// whole, it is the placeholder, which is not; the field goes
				// then, and the placeholder stands in the text content.
				if back, ok := mcpDecode([]byte(v)); ok {
					result["structuredContent"] = back
				} else {
					delete(result, "structuredContent")
				}
			}})
		}
	}
	if list, ok := result["contents"].([]any); ok {
		for i, c := range list {
			if m, ok := c.(map[string]any); ok {
				str(m, "text", "result.contents["+strconv.Itoa(i)+"].text")
			}
		}
	}
	if list, ok := result["messages"].([]any); ok {
		str(result, "description", "result.description")
		for i, msg := range list {
			m, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			p := "result.messages[" + strconv.Itoa(i) + "].content"
			switch c := m["content"].(type) {
			case []any:
				for j, b := range c {
					block(b, p+"["+strconv.Itoa(j)+"]")
				}
			case string:
				str(m, "content", p)
			default:
				block(c, p)
			}
		}
	}
	return out
}

// mcpVerdict is what the scan made of a message.
type mcpVerdict struct {
	ev       scanEval
	findings []scanner.Finding
	action   policy.Action
	fallback string
	// unavailable is set when the scan did not run to the end.
	unavailable bool
}

// judgeInbound judges one message the server sent towards the agent and
// returns what is passed on: the message as it came, the message with its
// text stripped or masked, or a JSON-RPC error in its place. keep is false
// for a message that is not passed on at all.
func (g *mcpGateway) judgeInbound(ctx context.Context, c *mcpCall, pol *policy.Policy, raw []byte) (out []byte, keep bool) {
	v, ok := mcpDecode(raw)
	msg, isObj := v.(map[string]any)
	if !ok || !isObj {
		c.logger.Warn().Msg("mcp server sent a message that is not a JSON-RPC object; not passed on")
		return nil, false
	}
	result, isResult := msg["result"].(map[string]any)
	if !isResult {
		// Errors, notifications and the server's own requests go as they are.
		return raw, true
	}
	id := msg["id"]
	changed := false

	var toolRaws []json.RawMessage
	var list []any
	if l, ok := result["tools"].([]any); ok {
		kept, n := g.screenTools(c, l)
		if n > 0 {
			changed = true
			result["tools"] = kept
		}
		list = kept
		for _, t := range kept {
			toolRaws = append(toolRaws, mcpEncode(t))
		}
	}
	pieces := mcpPieces(result)
	if len(pieces) == 0 && len(toolRaws) == 0 {
		if changed {
			return mcpEncode(msg), true
		}
		return raw, true
	}

	msgs := make([]scanMessage, len(pieces))
	for i, p := range pieces {
		msgs[i] = scanMessage{role: string(extract.RoleTool), text: p.text}
	}
	verdict := g.evaluate(ctx, c, pol, msgs, nil, toolRaws, false)
	for i := range verdict.findings {
		verdict.findings[i].Path = mcpInboundPath(verdict.findings[i].Path, pieces)
	}
	c.add(verdict.findings, verdict.action, verdict.fallback)
	if verdict.unavailable {
		if pol == nil || pol.BlockOnScanError(true) {
			c.add(nil, policy.ActionBlock, "")
			return mcpErrorBody(id, mcpCodeScanUnavailable, mcpMsgScanUnavailable, c.errorData(nil)), true
		}
		c.logger.Warn().Msg("mcp message passed on unscanned: scan.on_error is pass")
	} else {
		switch verdict.action {
		case policy.ActionBlock:
			return mcpErrorBody(id, mcpCodeBlocked, mcpMsgBlocked, c.errorData(&verdict)), true
		case policy.ActionStrip, policy.ActionRedact:
			if !applyRewrite(verdict.ev, pieces, result, len(list), c) {
				verdict.fallback = mcpFallbackResult
				c.add(nil, policy.ActionBlock, mcpFallbackResult)
				return mcpErrorBody(id, mcpCodeBlocked, mcpMsgBlocked, c.errorData(&verdict)), true
			}
			changed = true
		}
	}
	if changed {
		return mcpEncode(msg), true
	}
	return raw, true
}

// applyRewrite puts the texts and tool definitions a STRIP or a REDACT
// rewrote back into the result, and counts them on the call. It reports
// false, having changed nothing, when they do not fit what was scanned.
func applyRewrite(ev scanEval, pieces []mcpPiece, result map[string]any, tools int, c *mcpCall) bool {
	if len(ev.rewritten) != len(pieces) || (ev.tools != nil && len(ev.tools) != tools) {
		return false
	}
	var defs []any
	if ev.tools != nil {
		defs = make([]any, len(ev.tools))
		for i, t := range ev.tools {
			back, ok := mcpDecode(t)
			if !ok {
				return false
			}
			defs[i] = back
		}
	}
	for i, p := range pieces {
		if ev.rewritten[i] != p.text {
			c.rewritten++
		}
		p.set(ev.rewritten[i])
	}
	if defs != nil {
		result["tools"] = defs
		c.rewritten++
	}
	return true
}

// evaluate runs the scan path on texts, tool calls and tool definitions, and
// settles the gateway's own verdict: ROUTE cannot be carried out here and is
// a BLOCK, and so is a STRIP of a tool call's arguments.
func (g *mcpGateway) evaluate(ctx context.Context, c *mcpCall, pol *policy.Policy, msgs []scanMessage,
	toolCalls, tools []json.RawMessage, calls bool) mcpVerdict {
	if pol == nil {
		return mcpVerdict{unavailable: true}
	}
	in, ok := newScanInput(msgs, toolCalls, tools)
	if !ok {
		c.logger.Error().Msg("mcp message could not be read as the scan path reads a request")
		return mcpVerdict{unavailable: true}
	}
	in.callsWritable = calls
	return g.evaluateInput(ctx, c, pol, in)
}

func (g *mcpGateway) evaluateInput(ctx context.Context, c *mcpCall, pol *policy.Policy, in scanInput) mcpVerdict {
	sc := &scanCall{start: time.Now(), logger: c.logger}
	ev, outcome := g.scan.evaluateInput(ctx, sc, c.requestID, c.who, pol, in)
	if outcome != "" {
		c.logger.Warn().Str("outcome", outcome).Msg("mcp scan incomplete")
		return mcpVerdict{unavailable: true}
	}
	v := mcpVerdict{ev: ev, findings: ev.findings, action: ev.action, fallback: ev.fallback}
	if v.action == policy.ActionRoute {
		v.action, v.fallback = policy.ActionBlock, mcpFallbackRoute
	}
	return v
}

// mcpInboundPath turns a finding's path in the request the scan read into
// its place in the message.
func mcpInboundPath(path string, pieces []mcpPiece) string {
	if rest, ok := strings.CutPrefix(path, "tools["); ok {
		return "result.tools[" + rest
	}
	for i, p := range pieces {
		if path == indexed("messages", i)+".content" {
			return p.path
		}
	}
	return path
}

// screenTools applies the server's tools list and the check for changed
// definitions to a tools/list result. It returns the tools the agent is
// shown and how many were taken out.
func (g *mcpGateway) screenTools(c *mcpCall, list []any) (kept []any, removed int) {
	kept = make([]any, 0, len(list))
	for i, t := range list {
		m, ok := t.(map[string]any)
		name, _ := m["name"].(string)
		if !ok || name == "" {
			kept = append(kept, t)
			continue
		}
		if !c.server.toolAllowed(name) {
			removed++
			continue
		}
		if c.server.ToolChanges != mcpToolChangesOff && g.memory.observe(c.session, name, mcpToolPrint(m)) {
			f := mcpToolChangeFinding(c.server.Name, name, "result.tools["+strconv.Itoa(i)+"]")
			if c.server.ToolChanges == mcpToolChangesBlock {
				c.add([]scanner.Finding{f}, policy.ActionStrip, "")
				removed++
				continue
			}
			c.add([]scanner.Finding{f}, policy.ActionWarn, "")
		}
		kept = append(kept, t)
	}
	return kept, removed
}

// mcpToolPrint is what a tool's definition is compared by: everything the
// model or the client reads of it except its name, icons and _meta.
func mcpToolPrint(tool map[string]any) [32]byte {
	part := map[string]any{}
	for _, k := range []string{"title", "description", "inputSchema", "outputSchema", "annotations"} {
		if v, ok := tool[k]; ok {
			part[k] = v
		}
	}
	return sha256.Sum256(mcpEncode(part))
}

func mcpToolChangeFinding(server, tool, path string) scanner.Finding {
	return scanner.Finding{
		Type:       mcpToolChangeType,
		Category:   mcpToolChangeCategory,
		Severity:   "high",
		Match:      tool,
		Confidence: 1.0,
		Seg:        -1,
		Role:       string(extract.RoleToolDefinition),
		Path:       path,
		Metadata:   map[string]string{"server": server, "tool": tool},
	}
}

// judgeCall judges a tools/call request on its way to the server. It
// returns a refusal to answer in place of the server, or nil; changed says
// the request's arguments were masked and the message must be written anew.
func (g *mcpGateway) judgeCall(ctx context.Context, c *mcpCall, pol *policy.Policy, header http.Header, msg map[string]any) (refusal *mcpRefusal, changed bool) {
	params, _ := msg["params"].(map[string]any)
	name, _ := params["name"].(string)
	c.tool = name
	if !c.server.toolAllowed(name) {
		c.add(nil, policy.ActionBlock, mcpFallbackNotAllow)
		return &mcpRefusal{mcpCodeToolNotAllowed, mcpMsgToolNotAllowed, c.errorData(nil)}, false
	}
	if c.server.ToolChanges == mcpToolChangesBlock && g.memory.changed(c.session, name) {
		c.add([]scanner.Finding{mcpToolChangeFinding(c.server.Name, name, "params.name")}, policy.ActionBlock, "")
		return &mcpRefusal{mcpCodeToolChanged, mcpMsgToolChanged, c.errorData(nil)}, false
	}
	args, present := params["arguments"]
	if !present || args == nil {
		return nil, false
	}
	argsJSON := mcpEncode(args)
	if pol == nil {
		return g.unavailable(c, pol), false
	}

	// Where the arguments send data, first: a masked address is then what
	// the rules read.
	masked := false
	in, ok := mcpCallInput(name, argsJSON)
	if !ok {
		return g.unavailable(c, pol), false
	}
	dest := mcpDestinations(pol, c.server.Name, name, in.segs)
	for i := range dest {
		dest[i].Path = mcpCallPath(dest[i].Path)
	}
	switch pol.EvaluateOutput(dest) {
	case policy.ActionBlock:
		c.add(dest, policy.ActionBlock, "")
		return &mcpRefusal{mcpCodeBlocked, mcpMsgBlocked, c.errorData(&mcpVerdict{findings: dest})}, false
	case policy.ActionRedact:
		var targets []scanner.Finding
		for _, f := range dest {
			if pol.OutputRedacts(f) {
				targets = append(targets, f)
			}
		}
		edits := scanner.RedactEdits(targets)
		var body []byte
		var err error
		if len(edits) == len(targets) {
			body, err = extract.RewriteChecked(scanProvider, in.body, in.segs, edits)
		}
		newArgs, newName, readOK := mcpArgsFrom(body)
		if len(edits) != len(targets) || err != nil || !readOK || newName != name {
			c.add(dest, policy.ActionBlock, mcpFallbackRewrite)
			return &mcpRefusal{mcpCodeBlocked, mcpMsgBlocked, c.errorData(&mcpVerdict{findings: dest, fallback: mcpFallbackRewrite})}, false
		}
		c.add(dest, policy.ActionRedact, "")
		argsJSON, masked = newArgs, true
		if in, ok = mcpCallInput(name, argsJSON); !ok {
			return g.unavailable(c, pol), false
		}
	default:
		c.add(dest, policy.ActionPass, "")
	}

	verdict := g.evaluateInput(ctx, c, pol, in)
	for i := range verdict.findings {
		verdict.findings[i].Path = mcpCallPath(verdict.findings[i].Path)
	}
	if verdict.unavailable {
		if pol.BlockOnScanError(true) {
			return &mcpRefusal{mcpCodeScanUnavailable, mcpMsgScanUnavailable, c.errorData(nil)}, false
		}
		c.logger.Warn().Msg("mcp tool call passed on unscanned: scan.on_error is pass")
	} else {
		if verdict.action == policy.ActionStrip {
			verdict.action, verdict.fallback = policy.ActionBlock, mcpFallbackStrip
		}
		c.add(verdict.findings, verdict.action, verdict.fallback)
		switch verdict.action {
		case policy.ActionBlock:
			return &mcpRefusal{mcpCodeBlocked, mcpMsgBlocked, c.errorData(&verdict)}, false
		case policy.ActionRedact:
			newArgs, newName, ok := mcpArgsFrom(verdict.ev.body)
			if !ok || newName != name {
				c.add(nil, policy.ActionBlock, mcpFallbackRewrite)
				return &mcpRefusal{mcpCodeBlocked, mcpMsgBlocked, c.errorData(&mcpVerdict{findings: verdict.findings, fallback: mcpFallbackRewrite})}, false
			}
			argsJSON, masked = newArgs, true
		}
	}
	if !masked {
		return nil, false
	}
	// The server checks Mcp-Param headers against the body (2026-07-28,
	// transports/streamable-http, custom headers), and the header carries
	// the value Tamga masked.
	for k := range header {
		if strings.HasPrefix(strings.ToLower(k), "mcp-param-") {
			c.add(nil, policy.ActionBlock, mcpFallbackParam)
			return &mcpRefusal{mcpCodeBlocked, mcpMsgBlocked, c.errorData(&mcpVerdict{fallback: mcpFallbackParam})}, false
		}
	}
	back, ok := mcpDecode(argsJSON)
	if !ok {
		c.add(nil, policy.ActionBlock, mcpFallbackRewrite)
		return &mcpRefusal{mcpCodeBlocked, mcpMsgBlocked, c.errorData(&mcpVerdict{fallback: mcpFallbackRewrite})}, false
	}
	params["arguments"] = back
	return nil, true
}

func (g *mcpGateway) unavailable(c *mcpCall, pol *policy.Policy) *mcpRefusal {
	if pol == nil || pol.BlockOnScanError(true) {
		c.add(nil, policy.ActionBlock, "")
		return &mcpRefusal{mcpCodeScanUnavailable, mcpMsgScanUnavailable, c.errorData(nil)}
	}
	c.logger.Warn().Msg("mcp tool call passed on unscanned: scan.on_error is pass")
	return nil
}

// mcpCallInput is a tools/call's arguments as the arguments of one tool call
// of the assistant, which is how the proxy path reads them in a model
// request.
func mcpCallInput(name string, args []byte) (scanInput, bool) {
	tc := mcpEncode(map[string]any{
		"id":       "mcp_call",
		"type":     "function",
		"function": map[string]any{"name": name, "arguments": string(args)},
	})
	in, ok := newScanInput(nil, []json.RawMessage{tc}, nil)
	in.callsWritable = true
	return in, ok
}

// mcpArgsFrom reads the arguments and the name of the tool call back out of
// a body mcpCallInput built and a rewrite changed.
func mcpArgsFrom(body []byte) (args []byte, name string, ok bool) {
	var doc struct {
		Messages []struct {
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if len(body) == 0 || json.Unmarshal(body, &doc) != nil || len(doc.Messages) != 1 || len(doc.Messages[0].ToolCalls) != 1 {
		return nil, "", false
	}
	fn := doc.Messages[0].ToolCalls[0].Function
	if _, ok := mcpDecode([]byte(fn.Arguments)); !ok {
		return nil, "", false
	}
	return []byte(fn.Arguments), fn.Name, true
}

// mcpCallPath turns a finding's path in the tool call the scan read into its
// place in the tools/call request.
func mcpCallPath(path string) string {
	const call = "messages[0].tool_calls[0].function."
	if rest, ok := strings.CutPrefix(path, call+"arguments"); ok {
		return "params.arguments" + rest
	}
	if rest, ok := strings.CutPrefix(path, call); ok {
		return "params." + rest
	}
	return path
}

// mcpDestinations holds a tools/call to output_rules.tool_destinations. An
// entry may name the tool as the server does ("send_email") or as Claude
// Code names an MCP tool to the model ("mcp__mail__send_email"), so that one
// entry covers the call on both paths.
func mcpDestinations(pol *policy.Policy, server, tool string, segs []extract.Segment) []scanner.Finding {
	if pol.ToolDestinationRules() == nil {
		return nil
	}
	out := toolDestinationFindings(pol, segs)
	alias := append([]extract.Segment(nil), segs...)
	for i := range alias {
		if alias[i].Kind == extract.KindToolArgs {
			alias[i].Tool = "mcp__" + server + "__" + tool
		}
	}
	type at struct{ seg, start, end int }
	seen := map[at]bool{}
	for _, f := range out {
		seen[at{f.Seg, f.StartPos, f.EndPos}] = true
	}
	for _, f := range toolDestinationFindings(pol, alias) {
		if k := (at{f.Seg, f.StartPos, f.EndPos}); !seen[k] {
			seen[k] = true
			out = append(out, f)
		}
	}
	return out
}

// mcpKinds names the kinds of finding in a refusal: types and categories,
// never what was matched. A proxy key is told the type of a named finding
// (a custom entity, a competitor, a banned topic) and not its name, as the
// scan API does (scanNamedTypes).
func mcpKinds(findings []scanner.Finding, coarse bool) []string {
	seen := map[string]bool{}
	var kinds []string
	for _, f := range findings {
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
	if len(kinds) > litellmMaxKinds {
		kinds = kinds[:litellmMaxKinds]
	}
	return kinds
}
