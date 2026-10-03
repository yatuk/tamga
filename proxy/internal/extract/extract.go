package extract

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Role says who a piece of text speaks for. It decides which policy rules
// apply: an instruction in a tool result is an attack, the same words in the
// system prompt are the developer's own.
//
// The role is whatever the request says it is. The caller wrote the body and
// can label any text "system"; a rule that exempts a role trusts the caller.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	// RoleTool is content that came from outside the conversation: tool
	// results, retrieved documents, search results.
	RoleTool Role = "tool"
	// RoleToolDefinition is the description of a tool offered to the model.
	RoleToolDefinition Role = "tool_definition"
)

// Kinds of segment. They refine the role for reporting; rules key on Role.
const (
	KindText       = "text"
	KindToolArgs   = "tool_args"
	KindToolResult = "tool_result"
	KindDocument   = "document"
	KindThinking   = "thinking"
	KindURL        = "url"
	KindDefinition = "definition"
	KindUnknown    = "unknown"
)

// Formats Extract recognises.
const (
	FormatOpenAI    = "openai"
	FormatAnthropic = "anthropic"
	FormatGemini    = "gemini"
)

// Segment is one piece of text the model will read, decoded from the body.
type Segment struct {
	Role Role
	Kind string
	// Path locates the string in the body, e.g. "messages[2].content[0].text".
	// For text inside a JSON document that is itself a string (tool call
	// arguments), the inner location follows a "#": "…arguments#city".
	Path string
	Text string
	// Start and End delimit the JSON string token in the body, quotes
	// included: body[Start:End] decodes to Text, or for a Nested segment to
	// the document that contains it.
	Start, End int
	// Nested marks text inside a JSON-in-a-string document. InnerStart and
	// InnerEnd then delimit its token inside the decoded outer string.
	Nested               bool
	InnerStart, InnerEnd int
	// Encoding is "base64" when Text was decoded from a base64 text
	// attachment. The token then holds the encoded form (after any
	// "data:…;base64," prefix), so Text cannot be written back as it is.
	Encoding string
}

// Skipped is a string under a content root that was deliberately not scanned.
type Skipped struct {
	Path string
	// Reason is "structural" for protocol fields (type, id, role), "binary"
	// for encoded media, "opaque" for signatures and encrypted blobs.
	Reason string
}

// Result is what Extract found in a body.
type Result struct {
	Format   string
	Segments []Segment
	Skipped  []Skipped
	// Unknown lists block types the extractor has no rule for. Their text is
	// still in Segments: an unknown block is scanned generically, not skipped.
	Unknown []string
}

// Extract returns the text segments of an LLM request body, or false when the
// body is not a request shape it knows; the caller then scans the raw bytes.
//
// provider is the route the request arrived on. The shape of the body
// decides the format; the provider only settles the one ambiguous case, a
// "messages" array, which both OpenAI and Anthropic use.
func Extract(provider string, body []byte) (*Result, bool) {
	root, err := parseTree(body)
	if err != nil || root.kind != 'o' {
		return nil, false
	}
	x := &extractor{src: body, res: &Result{}}
	switch {
	case root.get("contents") != nil:
		x.res.Format = FormatGemini
		x.gemini(root)
	case root.get("messages") != nil && isAnthropic(provider, root):
		x.res.Format = FormatAnthropic
		x.anthropic(root)
	case root.get("messages") != nil, root.get("input") != nil, root.get("prompt") != nil:
		x.res.Format = FormatOpenAI
		x.openAI(root)
	default:
		return nil, false
	}
	return x.res, true
}

func isAnthropic(provider string, root *node) bool {
	switch provider {
	case "anthropic", "bedrock":
		return true
	}
	return root.get("anthropic_version") != nil
}

type extractor struct {
	src []byte
	res *Result
}

// ── emitting ────────────────────────────────────────────────────────────

func (x *extractor) emit(n *node, role Role, kind, path string) {
	if n == nil || n.kind != 's' {
		return
	}
	text := n.text(x.src)
	if text == "" {
		return
	}
	x.res.Segments = append(x.res.Segments, Segment{
		Role: role, Kind: kind, Path: path, Text: text, Start: n.start, End: n.end,
	})
}

func (x *extractor) skip(path, reason string) {
	x.res.Skipped = append(x.res.Skipped, Skipped{Path: path, Reason: reason})
}

func (x *extractor) unknown(blockType string) {
	if blockType == "" {
		blockType = "(no type)"
	}
	for _, u := range x.res.Unknown {
		if u == blockType {
			return
		}
	}
	x.res.Unknown = append(x.res.Unknown, blockType)
}

// payload scans every string under n. It is for free-form data — tool
// arguments, tool results, schemas — where no key has a protocol meaning, so
// nothing may be skipped: a field called "type" or "data" there is content.
func (x *extractor) payload(n *node, role Role, kind, path string) {
	if n == nil {
		return
	}
	switch n.kind {
	case 's':
		x.emit(n, role, kind, path)
	case 'o':
		for i, k := range n.keys {
			x.payload(n.vals[i], role, kind, path+"."+k)
		}
	case 'a':
		for i, v := range n.vals {
			x.payload(v, role, kind, index(path, i))
		}
	}
}

// structural are protocol fields whose string value is never model input.
var structural = map[string]bool{
	"type": true, "role": true, "id": true, "name": true,
	"tool_call_id": true, "tool_use_id": true, "call_id": true,
	"media_type": true, "mimeType": true, "mime_type": true,
	"detail": true, "format": true, "status": true, "language": true,
	"file_id": true, "outcome": true,
}

// opaque are fields holding signatures or ciphertext: not text, not media.
var opaque = map[string]bool{
	"signature": true, "encrypted_content": true, "thoughtSignature": true, "thought_signature": true,
}

// rest handles the keys of a protocol object that its handler did not
// consume: known protocol fields are recorded as skipped, anything else is
// scanned as payload, so a field added by a provider is never silently unread.
func (x *extractor) rest(obj *node, done map[string]bool, role Role, kind, path string) {
	for i, k := range obj.keys {
		if done[k] {
			continue
		}
		v := obj.vals[i]
		p := path + "." + k
		if k == "cache_control" && v.kind == 'o' {
			// {"type":"ephemeral","ttl":"1h"}: caching hints, never model input.
			for j, ck := range v.keys {
				if v.vals[j].kind == 's' {
					x.skip(p+"."+ck, "structural")
				}
			}
			continue
		}
		if v.kind == 's' {
			switch {
			case structural[k]:
				x.skip(p, "structural")
				continue
			case opaque[k]:
				x.skip(p, "opaque")
				continue
			}
		}
		x.payload(v, role, kind, p)
	}
}

// restUnknown scans a block the extractor has no rule for. Only "type" and
// "id" are taken as protocol; with no knowledge of the block, a field called
// "name" or "format" may be the text the model reads.
func (x *extractor) restUnknown(obj *node, role Role, path string) {
	for i, k := range obj.keys {
		v := obj.vals[i]
		p := path + "." + k
		if v.kind == 's' && (k == "type" || k == "id") {
			x.skip(p, "structural")
			continue
		}
		x.payload(v, role, KindUnknown, p)
	}
}

// media handles an object that carries encoded media or a link to it:
// {media_type|mimeType|format, data}, {url}, {file_data}. Text media
// ("text/plain") is emitted, everything else is recorded as binary.
func (x *extractor) media(obj *node, role Role, kind, path string) {
	if obj == nil || obj.kind != 'o' {
		x.payload(obj, role, kind, path)
		return
	}
	mime := obj.get("media_type").text(x.src) + obj.get("mimeType").text(x.src) + obj.get("mime_type").text(x.src)
	isText := strings.HasPrefix(mime, "text/") || obj.get("type").text(x.src) == "text"
	done := map[string]bool{}
	for i, k := range obj.keys {
		v := obj.vals[i]
		p := path + "." + k
		switch k {
		case "data", "file_data":
			done[k] = true
			switch {
			case v.kind != 's':
				x.payload(v, role, kind, p)
			case obj.get("type").text(x.src) == "text":
				// Anthropic's plain-text source carries the text as it is.
				x.emit(v, role, kind, p)
			default:
				x.attachment(v, isText, role, kind, p)
			}
		case "url", "image_url", "file_url", "fileUri", "file_uri":
			done[k] = true
			if v.kind != 's' {
				x.media(v, role, kind, p)
			} else if isDataURI(v.text(x.src)) {
				x.attachment(v, false, role, kind, p)
			} else {
				x.emit(v, role, KindURL, p)
			}
		}
	}
	x.rest(obj, done, role, kind, path)
}

// attachment handles an encoded file: base64, bare or as a data URI. The
// model reads a text file's contents, so those are decoded and scanned.
// Images, audio and PDFs are recorded as binary: what they say to the model
// is not visible here, and that is a limit of this proxy, not a judgement
// that they are safe.
func (x *extractor) attachment(v *node, textMime bool, role Role, kind, path string) {
	raw := v.text(x.src)
	payload := raw
	if isDataURI(raw) {
		head, rest, _ := strings.Cut(raw, ";base64,")
		payload = rest
		textMime = strings.HasPrefix(head, "data:text/")
	}
	if !textMime {
		x.skip(path, "binary")
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "="))
	}
	if err != nil || !utf8.Valid(decoded) || len(decoded) == 0 {
		// Declared as text but not base64 text: scan it as written.
		x.emit(v, role, kind, path)
		return
	}
	x.res.Segments = append(x.res.Segments, Segment{
		Role: role, Kind: KindDocument, Path: path, Text: string(decoded),
		Start: v.start, End: v.end, Encoding: "base64",
	})
}

func isDataURI(s string) bool {
	return strings.HasPrefix(s, "data:") && strings.Contains(s[:min(len(s), 128)], ";base64,")
}

// nestedJSON handles a string that holds a JSON document, as OpenAI tool
// call arguments do. The model reads the inner strings, so those are the
// segments; a string that is not JSON is one segment as it stands.
func (x *extractor) nestedJSON(n *node, role Role, kind, path string) {
	if n == nil || n.kind != 's' {
		x.payload(n, role, kind, path)
		return
	}
	inner := n.text(x.src)
	tree, err := parseTree([]byte(inner))
	if err != nil || (tree.kind != 'o' && tree.kind != 'a') {
		x.emit(n, role, kind, path)
		return
	}
	sub := &extractor{src: []byte(inner), res: &Result{}}
	sub.payload(tree, role, kind, path+"#")
	for _, s := range sub.res.Segments {
		s.Path = strings.Replace(s.Path, "#.", "#", 1)
		s.Nested = true
		s.InnerStart, s.InnerEnd = s.Start, s.End
		s.Start, s.End = n.start, n.end
		x.res.Segments = append(x.res.Segments, s)
	}
}

func index(path string, i int) string {
	return path + "[" + strconv.Itoa(i) + "]"
}

// ── OpenAI: chat completions, responses, legacy completions ─────────────

func openAIRole(r string) Role {
	switch r {
	case "system", "developer":
		return RoleSystem
	case "assistant":
		return RoleAssistant
	case "tool", "function":
		return RoleTool
	default:
		return RoleUser
	}
}

func (x *extractor) openAI(root *node) {
	for i, k := range root.keys {
		v := root.vals[i]
		switch k {
		case "messages":
			for j, m := range x.list(v, RoleUser, "messages") {
				x.openAIMessage(m, index("messages", j))
			}
		case "input":
			x.openAIInput(v, "input")
		case "instructions":
			x.payload(v, RoleSystem, KindText, "instructions")
		case "prompt":
			x.payload(v, RoleUser, KindText, "prompt")
		case "tools", "functions":
			x.toolDefinitions(v, k)
		}
	}
}

func (x *extractor) openAIMessage(m *node, path string) {
	if m.kind != 'o' {
		x.payload(m, RoleUser, KindUnknown, path)
		return
	}
	role := openAIRole(m.get("role").text(x.src))
	kind := KindText
	if role == RoleTool {
		kind = KindToolResult
	}
	done := map[string]bool{}
	for i, k := range m.keys {
		v := m.vals[i]
		p := path + "." + k
		switch k {
		case "content":
			done[k] = true
			x.openAIContent(v, role, kind, p)
		case "tool_calls":
			done[k] = true
			for j, tc := range x.list(v, RoleAssistant, p) {
				x.openAIToolCall(tc, index(p, j))
			}
		case "function_call":
			done[k] = true
			x.openAIToolCall(v, p)
		}
	}
	x.rest(m, done, role, kind, path)
}

// openAIToolCall handles {id, type, function:{name, arguments}} and the
// flattened {name, arguments} of the legacy and responses shapes.
func (x *extractor) openAIToolCall(tc *node, path string) {
	if tc.kind != 'o' {
		x.payload(tc, RoleAssistant, KindToolArgs, path)
		return
	}
	done := map[string]bool{}
	if fn := tc.get("function"); fn != nil && fn.kind == 'o' {
		done["function"] = true
		x.nestedJSON(fn.get("arguments"), RoleAssistant, KindToolArgs, path+".function.arguments")
		x.rest(fn, map[string]bool{"arguments": true}, RoleAssistant, KindToolArgs, path+".function")
	}
	if args := tc.get("arguments"); args != nil {
		done["arguments"] = true
		x.nestedJSON(args, RoleAssistant, KindToolArgs, path+".arguments")
	}
	x.rest(tc, done, RoleAssistant, KindToolArgs, path)
}

// openAIContent handles a message's content: a string, or a list of parts.
func (x *extractor) openAIContent(v *node, role Role, kind, path string) {
	if v.kind != 'a' {
		x.payload(v, role, kind, path)
		return
	}
	for j, part := range v.vals {
		p := index(path, j)
		if part.kind != 'o' {
			x.payload(part, role, kind, p)
			continue
		}
		t := part.get("type").text(x.src)
		switch t {
		case "text", "input_text", "output_text", "refusal", "summary_text", "reasoning_text":
			done := map[string]bool{"text": true, "refusal": true}
			x.emit(part.get("text"), role, kind, p+".text")
			x.emit(part.get("refusal"), role, kind, p+".refusal")
			x.rest(part, done, role, kind, p)
		case "image_url", "input_image", "input_audio", "file", "input_file", "audio":
			done := map[string]bool{}
			for _, key := range []string{"image_url", "input_audio", "file", "audio"} {
				if sub := part.get(key); sub != nil && sub.kind == 'o' {
					done[key] = true
					x.media(sub, role, kind, p+"."+key)
				}
			}
			// The responses shape puts image_url, file_data and file_url on
			// the part itself.
			flat := &node{kind: 'o'}
			for i, k := range part.keys {
				if !done[k] && (k == "image_url" || k == "file_data" || k == "file_url") {
					flat.keys = append(flat.keys, k)
					flat.vals = append(flat.vals, part.vals[i])
					done[k] = true
				}
			}
			x.media(flat, role, kind, p)
			x.rest(part, done, role, kind, p)
		default:
			// OpenAI-compatible gateways in front of other models pass their
			// block types through; the Anthropic rules know several of them.
			x.anthropicBlock(part, role, kind, p)
		}
	}
}

// openAIInput handles the responses API "input": a string, or a list of
// messages and tool items. It also covers the embeddings "input".
func (x *extractor) openAIInput(v *node, path string) {
	if v.kind != 'a' {
		x.payload(v, RoleUser, KindText, path)
		return
	}
	for j, item := range v.vals {
		p := index(path, j)
		if item.kind != 'o' {
			x.payload(item, RoleUser, KindText, p)
			continue
		}
		t := item.get("type").text(x.src)
		switch {
		case t == "" || t == "message":
			x.openAIMessage(item, p)
		case t == "function_call" || t == "custom_tool_call":
			x.openAIToolCall(item, p)
		case strings.HasSuffix(t, "_output"):
			done := map[string]bool{"output": true}
			if out := item.get("output"); out != nil {
				x.openAIContent(out, RoleTool, KindToolResult, p+".output")
			}
			x.rest(item, done, RoleTool, KindToolResult, p)
		case t == "reasoning":
			done := map[string]bool{"summary": true, "content": true}
			for _, key := range []string{"summary", "content"} {
				if sub := item.get(key); sub != nil {
					x.openAIContent(sub, RoleAssistant, KindThinking, p+"."+key)
				}
			}
			x.rest(item, done, RoleAssistant, KindThinking, p)
		default:
			x.unknown(t)
			x.restUnknown(item, RoleAssistant, p)
		}
	}
}

// toolDefinitions scans what the model is told about its tools. The text is
// written by whoever supplied the tool, which is not always the developer.
func (x *extractor) toolDefinitions(v *node, path string) {
	for j, tool := range x.list(v, RoleToolDefinition, path) {
		p := index(path, j)
		if tool.kind != 'o' {
			x.payload(tool, RoleToolDefinition, KindDefinition, p)
			continue
		}
		for i, k := range tool.keys {
			if k == "type" && tool.vals[i].kind == 's' {
				x.skip(p+".type", "structural")
				continue
			}
			x.payload(tool.vals[i], RoleToolDefinition, KindDefinition, p+"."+k)
		}
	}
}

// ── Anthropic messages ──────────────────────────────────────────────────

func (x *extractor) anthropic(root *node) {
	for i, k := range root.keys {
		v := root.vals[i]
		switch k {
		case "system":
			x.anthropicContent(v, RoleSystem, KindText, "system")
		case "messages":
			for j, m := range x.list(v, RoleUser, "messages") {
				p := index("messages", j)
				if m.kind != 'o' {
					x.payload(m, RoleUser, KindUnknown, p)
					continue
				}
				role := RoleUser
				if m.get("role").text(x.src) == "assistant" {
					role = RoleAssistant
				}
				done := map[string]bool{}
				if c := m.get("content"); c != nil {
					done["content"] = true
					x.anthropicContent(c, role, KindText, p+".content")
				}
				x.rest(m, done, role, KindText, p)
			}
		case "tools":
			x.toolDefinitions(v, "tools")
		}
	}
}

// anthropicContent handles a string or a list of content blocks.
func (x *extractor) anthropicContent(v *node, role Role, kind, path string) {
	if v.kind != 'a' {
		x.payload(v, role, kind, path)
		return
	}
	for j, b := range v.vals {
		p := index(path, j)
		if b.kind != 'o' {
			x.payload(b, role, kind, p)
			continue
		}
		x.anthropicBlock(b, role, kind, p)
	}
}

// anthropicBlock handles one content block; role and kind are those of the
// message or block that contains it.
func (x *extractor) anthropicBlock(b *node, role Role, kind, p string) {
	t := b.get("type").text(x.src)
	switch {
	case t == "text":
		x.emit(b.get("text"), role, kind, p+".text")
		x.rest(b, map[string]bool{"text": true}, role, kind, p)
	case t == "tool_use" || t == "server_tool_use" || t == "mcp_tool_use":
		x.payload(b.get("input"), RoleAssistant, KindToolArgs, p+".input")
		x.rest(b, map[string]bool{"input": true}, RoleAssistant, KindToolArgs, p)
	case strings.HasSuffix(t, "tool_result"):
		if c := b.get("content"); c != nil {
			x.anthropicContent(c, RoleTool, KindToolResult, p+".content")
		}
		x.rest(b, map[string]bool{"content": true}, RoleTool, KindToolResult, p)
	case t == "image":
		done := map[string]bool{}
		if src := b.get("source"); src != nil {
			done["source"] = true
			x.media(src, role, kind, p+".source")
		}
		x.rest(b, done, role, kind, p)
	case t == "document" || t == "search_result":
		done := map[string]bool{}
		if src := b.get("source"); src != nil {
			done["source"] = true
			if src.kind == 'o' && src.get("content") != nil {
				x.anthropicContent(src.get("content"), RoleTool, KindDocument, p+".source.content")
				x.rest(src, map[string]bool{"content": true}, RoleTool, KindDocument, p+".source")
			} else {
				x.media(src, RoleTool, KindDocument, p+".source")
			}
		}
		if c := b.get("content"); c != nil {
			done["content"] = true
			x.anthropicContent(c, RoleTool, KindDocument, p+".content")
		}
		x.rest(b, done, RoleTool, KindDocument, p)
	case t == "thinking":
		x.emit(b.get("thinking"), RoleAssistant, KindThinking, p+".thinking")
		x.rest(b, map[string]bool{"thinking": true}, RoleAssistant, KindThinking, p)
	case t == "redacted_thinking":
		if d := b.get("data"); d != nil && d.kind == 's' {
			x.skip(p+".data", "opaque")
		}
		x.rest(b, map[string]bool{"data": true}, RoleAssistant, KindThinking, p)
	default:
		x.unknown(t)
		r := role
		if strings.HasSuffix(t, "_result") {
			r = RoleTool
		}
		x.restUnknown(b, r, p)
	}
}

// ── Gemini generateContent ──────────────────────────────────────────────

func (x *extractor) gemini(root *node) {
	for i, k := range root.keys {
		v := root.vals[i]
		switch k {
		case "systemInstruction", "system_instruction":
			x.geminiContent(v, RoleSystem, k)
		case "contents":
			if v.kind != 'a' {
				x.geminiContent(v, RoleUser, k)
				continue
			}
			for j, c := range v.vals {
				x.geminiContent(c, RoleUser, index(k, j))
			}
		case "tools":
			x.toolDefinitions(v, "tools")
		}
	}
}

// geminiContent handles {role, parts}; fallback is the role when none is given.
func (x *extractor) geminiContent(c *node, fallback Role, path string) {
	if c.kind != 'o' {
		x.payload(c, fallback, KindText, path)
		return
	}
	role := fallback
	switch c.get("role").text(x.src) {
	case "user":
		role = RoleUser
	case "model":
		role = RoleAssistant
	case "function", "tool":
		role = RoleTool
	}
	done := map[string]bool{}
	if parts := c.get("parts"); parts != nil {
		done["parts"] = true
		for j, part := range x.list(parts, role, path+".parts") {
			x.geminiPart(part, role, index(path+".parts", j))
		}
	}
	x.rest(c, done, role, KindText, path)
}

func (x *extractor) geminiPart(part *node, role Role, path string) {
	if part.kind != 'o' {
		x.payload(part, role, KindText, path)
		return
	}
	done := map[string]bool{}
	for i, k := range part.keys {
		v := part.vals[i]
		p := path + "." + k
		switch k {
		case "text":
			done[k] = true
			x.emit(v, role, KindText, p)
		case "inlineData", "inline_data", "fileData", "file_data":
			done[k] = true
			x.media(v, role, KindText, p)
		case "functionCall", "function_call":
			done[k] = true
			x.payloadOf(v, "args", RoleAssistant, KindToolArgs, p)
		case "functionResponse", "function_response":
			done[k] = true
			x.payloadOf(v, "response", RoleTool, KindToolResult, p)
		case "executableCode", "executable_code":
			done[k] = true
			x.payloadOf(v, "code", RoleAssistant, KindToolArgs, p)
		case "codeExecutionResult", "code_execution_result":
			done[k] = true
			x.payloadOf(v, "output", RoleTool, KindToolResult, p)
		case "thought":
			done[k] = true // a boolean flag
		}
	}
	for i, k := range part.keys {
		if done[k] {
			continue
		}
		v := part.vals[i]
		if v.kind == 's' && opaque[k] {
			x.skip(path+"."+k, "opaque")
			continue
		}
		x.unknown("gemini:" + k)
		x.payload(v, role, KindUnknown, path+"."+k)
	}
}

// payloadOf scans obj[member] as payload and the other keys as protocol.
func (x *extractor) payloadOf(obj *node, member string, role Role, kind, path string) {
	if obj == nil || obj.kind != 'o' {
		x.payload(obj, role, kind, path)
		return
	}
	x.payload(obj.get(member), role, kind, path+"."+member)
	x.rest(obj, map[string]bool{member: true}, role, kind, path)
}

// list returns the elements of an array node. Anything else found where a
// list belongs is scanned whole as payload, so its text is not lost and its
// path stays the one in the body.
func (x *extractor) list(n *node, role Role, path string) []*node {
	if n == nil {
		return nil
	}
	if n.kind == 'a' {
		return n.vals
	}
	x.payload(n, role, KindUnknown, path)
	return nil
}
