package extract

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A streamed answer carries what the model writes in three ways, and the
// stream guard needs each of them:
//
//   - text that arrives in pieces: the answer itself, and the reasoning many
//     clients show next to it (StreamDelta, KindText and KindThinking);
//   - a tool call's arguments, a JSON document that arrives in pieces and is
//     of use to the client only when it is complete (StreamDelta,
//     KindToolArgs);
//   - values that arrive whole inside one event: a Gemini function call, the
//     "done" events of the Responses API, which repeat what the deltas
//     carried, a content block that starts with its content
//     (StreamEvent.Whole).
//
// Signatures and encrypted reasoning are none of these. They are opaque,
// nothing is yielded for them, and they pass as they are.

// StreamDelta is a piece of what the model wrote, carried by one streamed
// event.
type StreamDelta struct {
	// Channel names the run the piece belongs to. One response can stream
	// several at once (choices, content blocks, tool calls); a value split
	// over events is only whole within its own channel. A channel named
	// "x.y" is part of "x": what ends "x" ends it too.
	Channel string
	// Kind is KindText for the answer, KindThinking for reasoning, and
	// KindToolArgs for a piece of a tool call's arguments.
	Kind string
	// Path is where a buffered answer holds what this channel carries
	// ("choices[0].message.tool_calls[1].function.arguments"). It is empty
	// for the answer's own text, the one a schema is held against.
	Path string
	// Sealed marks text the provider signs: the client has to send it back
	// as it was, so it cannot be masked.
	Sealed bool
	// Text is the decoded piece.
	Text string
	// Tool is the name of the tool a KindToolArgs piece is for. The parser
	// leaves it empty (the name comes in StreamEvent.Tools); whoever keeps
	// the channel sets it before ArgumentSegments.
	Tool string
	// Plain marks the pieces of a KindToolArgs channel that make up a text,
	// not a JSON document: a custom tool's input, code to run, one string
	// argument of a Gemini partial function call. ArgumentSegments reads
	// such a text as it stands, as a buffered answer reads it.
	Plain bool
	// start and end delimit the JSON string token in the event's data.
	start, end int
	// objectArgs says the arguments are an object in the buffered shape
	// (Anthropic's input), not a string that holds a document.
	objectArgs bool
	// alt describes an event that carries this piece and nothing else, for
	// shapes in which the event at hand may carry more (an id, a name, a
	// role, other parts): data[:altStart] + altHead + <text> + altTail +
	// data[altEnd:]. altEnd is 0 when the event at hand is such an event.
	altStart, altEnd int
	altHead, altTail string
}

// StreamEvent is what one streamed event's JSON payload means for what the
// model is writing.
type StreamEvent struct {
	// Deltas are the pieces the event adds, in the order they stand in the
	// event.
	Deltas []StreamDelta
	// Ends are the channels the event closes; EndsAll closes every one.
	Ends    []string
	EndsAll bool
	// Whole are the values the event carries complete. Start and End are
	// offsets into the event's data, as Extract gives them for a body.
	Whole []Segment
	// SealedThinking says the KindThinking segments of Whole are signed.
	SealedThinking bool
	// Tools names the tools whose calls the event opens, by the channel
	// that will carry their arguments. A stream names a tool once, in the
	// event that opens its call, and not in the events that carry its
	// arguments.
	Tools []ToolName
	// WholeChannel is the tool-call channel whose arguments the KindToolArgs
	// values of Whole repeat, for an event that closes a call without
	// naming its tool (the Responses API's ".done" events).
	WholeChannel string
	// Calls are the calls of tools the client runs that the event opens or
	// carries whole, as Result.ToolCalls lists them for a body. A call can
	// be named again by a later event that repeats it (the Responses API's
	// ".done" and "completed" events).
	Calls []ToolCall
}

// ToolName says which tool the arguments of a channel are for.
type ToolName struct {
	Channel, Name string
}

// ParseStreamEvent reads the data of one streamed event. It returns false
// when the data is not JSON it can read; such an event carries nothing this
// package knows how to find.
//
// Known shapes: OpenAI chat and legacy completions chunks, OpenAI Responses
// events, Anthropic message events, Gemini streamGenerateContent chunks.
func ParseStreamEvent(data []byte) (StreamEvent, bool) {
	root, err := parseTree(data)
	if err != nil || root.kind != 'o' {
		return StreamEvent{}, false
	}
	p := &streamParser{data: data}
	typ := p.str(root.get("type"))
	switch {
	case root.get("choices") != nil && root.get("choices").kind == 'a':
		p.openAIChat(root.get("choices"))
	case strings.HasPrefix(typ, "content_block_") || typ == "message_delta" || typ == "message_stop" || typ == "error":
		p.anthropic(typ, root)
	case strings.HasPrefix(typ, "response."):
		p.responses(typ, root)
	case root.get("candidates") != nil && root.get("candidates").kind == 'a':
		p.gemini(root.get("candidates"))
	}
	if p.x != nil {
		p.ev.Whole = p.whole().res.Segments
		p.ev.Calls = append(p.ev.Calls, p.x.res.ToolCalls...)
	}
	if len(p.ev.Deltas) > 1 {
		sort.SliceStable(p.ev.Deltas, func(i, j int) bool { return p.ev.Deltas[i].start < p.ev.Deltas[j].start })
	}
	return p.ev, true
}

type streamParser struct {
	data []byte
	ev   StreamEvent
	// x collects the values an event carries whole, with the rules the
	// buffered extractor has for them. It is made when one is met: most
	// events carry a piece of text and nothing else.
	x *extractor
}

func (p *streamParser) whole() *extractor {
	if p.x == nil {
		p.x = &extractor{src: p.data, res: &Result{}}
	}
	return p.x
}

// isString reports whether n is a string, which is what a piece is.
func isString(n *node) bool { return n != nil && n.kind == 's' }

func (p *streamParser) str(n *node) string {
	if n == nil || n.kind != 's' {
		return ""
	}
	return n.text(p.data)
}

// num returns a number as it is written, or the fallback.
func (p *streamParser) num(n *node, fallback int) string {
	if n != nil && n.kind == 'x' {
		raw := string(p.data[n.start:n.end])
		if _, err := strconv.Atoi(raw); err == nil {
			return raw
		}
	}
	return strconv.Itoa(fallback)
}

func (p *streamParser) present(n *node) bool {
	return n != nil && string(p.data[n.start:n.end]) != "null"
}

// name records the tool a channel's arguments are for.
func (p *streamParser) name(channel string, n *node) {
	if name := p.str(n); name != "" {
		p.ev.Tools = append(p.ev.Tools, ToolName{Channel: channel, Name: name})
	}
}

// add yields the string n as a piece of channel.
func (p *streamParser) add(n *node, d StreamDelta) {
	if n == nil || n.kind != 's' {
		return
	}
	d.Text = n.text(p.data)
	d.start, d.end = n.start, n.end
	p.ev.Deltas = append(p.ev.Deltas, d)
}

// ── OpenAI chat and legacy completions ──────────────────────────────────

func (p *streamParser) openAIChat(choices *node) {
	for i, c := range choices.vals {
		if c.kind != 'o' {
			continue
		}
		ci := p.num(c.get("index"), i)
		ch := "c" + ci
		msg := "choices[" + ci + "].message"
		// An event that carries one piece of this choice and nothing else.
		alone := func(d StreamDelta, open, close string) StreamDelta {
			d.altStart, d.altEnd = choices.start, choices.end
			d.altHead = `[{"index":` + ci + `,"delta":{` + open
			d.altTail = close + `},"finish_reason":null}]`
			return d
		}
		if d := c.get("delta"); d != nil && d.kind == 'o' {
			if n := d.get("content"); isString(n) {
				p.add(n, alone(StreamDelta{Channel: ch, Kind: KindText}, `"content":`, ``))
			} else if n != nil && n.kind == 'a' {
				p.contentParts(n, ch, msg+".content", alone)
			}
			if n := d.get("refusal"); isString(n) {
				p.add(n, alone(StreamDelta{Channel: ch + ".refusal", Kind: KindText, Path: msg + ".refusal"}, `"refusal":`, ``))
			}
			// Reasoning as the OpenAI-compatible servers stream it.
			for _, key := range []string{"reasoning_content", "reasoning"} {
				if n := d.get(key); isString(n) {
					p.add(n, alone(StreamDelta{Channel: ch + "." + key, Kind: KindThinking, Path: msg + "." + key}, `"`+key+`":`, ``))
				}
			}
			if calls := d.get("tool_calls"); calls != nil && calls.kind == 'a' {
				for j, tc := range calls.vals {
					if tc.kind != 'o' {
						continue
					}
					ti := p.num(tc.get("index"), j)
					call := StreamDelta{Channel: ch + ".t" + ti, Kind: KindToolArgs}
					path := msg + ".tool_calls[" + ti + "]"
					open := `"tool_calls":[{"index":` + ti + `,`
					name := ""
					for _, key := range []string{"function", "custom"} {
						if fn := tc.get(key); fn != nil && fn.kind == 'o' {
							p.name(call.Channel, fn.get("name"))
							if name == "" {
								name = p.str(fn.get("name"))
							}
						}
					}
					// The chunk that opens a call gives its id and, as a
					// rule, the tool's name.
					if name != "" || p.str(tc.get("id")) != "" {
						p.ev.Calls = append(p.ev.Calls, ToolCall{Name: name, Path: path})
					}
					if fn := tc.get("function"); fn != nil && fn.kind == 'o' {
						call.Path = path + ".function.arguments"
						if args := fn.get("arguments"); args != nil && args.kind != 's' {
							// Arguments sent as an object are there whole.
							p.whole().payload(args, RoleAssistant, KindToolArgs, call.Path)
						}
						p.add(fn.get("arguments"), alone(call, open+`"function":{"arguments":`, `}}]`))
					}
					if custom := tc.get("custom"); custom != nil && custom.kind == 'o' {
						call.Path, call.Plain = path+".custom.input", true
						p.add(custom.get("input"), alone(call, open+`"custom":{"input":`, `}}]`))
					}
				}
			}
			if fc := d.get("function_call"); fc != nil && fc.kind == 'o' {
				call := StreamDelta{Channel: ch + ".fc", Kind: KindToolArgs, Path: msg + ".function_call.arguments"}
				p.name(call.Channel, fc.get("name"))
				if name := p.str(fc.get("name")); name != "" {
					p.ev.Calls = append(p.ev.Calls, ToolCall{Name: name, Path: msg + ".function_call"})
				}
				p.add(fc.get("arguments"), alone(call, `"function_call":{"arguments":`, `}`))
			}
		}
		// Legacy completions put the text on the choice itself.
		if n := c.get("text"); isString(n) {
			legacy := StreamDelta{Channel: ch, Kind: KindText}
			legacy.altStart, legacy.altEnd = choices.start, choices.end
			legacy.altHead, legacy.altTail = `[{"index":`+ci+`,"text":`, `,"finish_reason":null}]`
			p.add(n, legacy)
		}
		if p.present(c.get("finish_reason")) {
			p.ev.Ends = append(p.ev.Ends, ch)
		}
	}
}

// contentParts reads content sent as a list of parts in a chat chunk, as
// some OpenAI-compatible servers stream it. Mistral's reasoning models send
// the reasoning that way, a "thinking" part that holds text parts, then the
// first text of the answer as a "text" part, then the rest as a string
// (https://docs.mistral.ai/capabilities/reasoning/). The text of a text
// part is a piece of the answer and the text parts of a thinking part are
// pieces of reasoning; any other part, and what else a part holds, is read
// whole.
func (p *streamParser) contentParts(parts *node, ch, path string, alone func(StreamDelta, string, string) StreamDelta) {
	for k, part := range parts.vals {
		pp := index(path, k)
		if part.kind != 'o' {
			p.whole().payload(part, RoleAssistant, KindText, pp)
			continue
		}
		switch t := p.str(part.get("type")); {
		case t == "text" && isString(part.get("text")):
			p.add(part.get("text"), alone(StreamDelta{Channel: ch, Kind: KindText}, `"content":[{"type":"text","text":`, `}]`))
			p.whole().rest(part, map[string]bool{"text": true}, RoleAssistant, KindText, pp)
		case t == "thinking" && part.get("thinking") != nil && part.get("thinking").kind == 'a':
			for m, sub := range part.get("thinking").vals {
				sp := index(pp+".thinking", m)
				if sub.kind == 'o' && p.str(sub.get("type")) == "text" && isString(sub.get("text")) {
					p.add(sub.get("text"), alone(StreamDelta{Channel: ch + ".thinking", Kind: KindThinking, Path: path + ".thinking"},
						`"content":[{"type":"thinking","thinking":[{"type":"text","text":`, `}]}]`))
					p.whole().rest(sub, map[string]bool{"text": true}, RoleAssistant, KindThinking, sp)
					continue
				}
				p.whole().openAIPart(sub, RoleAssistant, KindThinking, sp)
			}
			p.whole().rest(part, map[string]bool{"thinking": true}, RoleAssistant, KindThinking, pp)
		default:
			p.whole().openAIPart(part, RoleAssistant, KindText, pp)
		}
	}
}

// ── Anthropic messages ──────────────────────────────────────────────────

func (p *streamParser) anthropic(typ string, root *node) {
	bi := p.num(root.get("index"), 0)
	ch := "b" + bi
	switch typ {
	case "content_block_start":
		// A block that starts with content has it whole: a server tool's
		// result, a tool call whose input is not streamed. Empty strings,
		// which is what a streamed block starts with, yield nothing.
		if b := root.get("content_block"); b != nil && b.kind == 'o' {
			p.whole().anthropicBlock(b, RoleAssistant, KindText, "content_block")
			p.ev.SealedThinking = true
			if strings.HasSuffix(p.str(b.get("type")), "tool_use") {
				p.name(ch, b.get("name"))
			}
		}
	case "content_block_delta":
		d := root.get("delta")
		if d == nil || d.kind != 'o' {
			return
		}
		switch p.str(d.get("type")) {
		case "text_delta":
			p.add(d.get("text"), StreamDelta{Channel: ch, Kind: KindText})
		case "thinking_delta":
			// The block ends with a signature over this text.
			p.add(d.get("thinking"), StreamDelta{Channel: ch, Kind: KindThinking, Path: "content[" + bi + "].thinking", Sealed: true})
		case "input_json_delta":
			p.add(d.get("partial_json"), StreamDelta{Channel: ch, Kind: KindToolArgs, Path: "content[" + bi + "].input", objectArgs: true})
		case "citations_delta":
			// One citation to add to the text block's list, whole: the text
			// it quotes from a document and the document's title
			// (https://platform.claude.com/docs/en/build-with-claude/citations#streaming-support).
			// A buffered answer has the same fields in the block's
			// "citations".
			if c := d.get("citation"); c != nil && c.kind == 'o' {
				p.whole().rest(c, nil, RoleAssistant, KindText, "content["+bi+"].citations")
			}
		}
		// signature_delta and anything newer: opaque, or not known.
	case "content_block_stop":
		p.ev.Ends = append(p.ev.Ends, ch)
	case "message_delta", "message_stop", "error":
		p.ev.EndsAll = true
	}
}

// ── OpenAI Responses ────────────────────────────────────────────────────

// item scans one output item with the rules of a buffered response.
func (p *streamParser) item(n *node, path string) {
	if n == nil || n.kind != 'o' {
		return
	}
	p.whole().openAIInput(&node{kind: 'a', vals: []*node{n}}, path)
}

func (p *streamParser) responses(typ string, root *node) {
	oi := p.num(root.get("output_index"), 0)
	item := "r" + oi
	delta := root.get("delta")
	// The commonest event by far carries a piece of the answer's text.
	if typ == "response.output_text.delta" {
		p.add(delta, StreamDelta{Channel: item + "." + p.num(root.get("content_index"), 0), Kind: KindText})
		return
	}
	out := "output[" + oi + "]"
	part := item + "." + p.num(root.get("content_index"), 0)
	summary := item + ".s" + p.num(root.get("summary_index"), 0)
	switch typ {
	case "response.output_text.done":
		p.ev.Ends = append(p.ev.Ends, part)
		p.whole().emit(root.get("text"), RoleAssistant, KindText, "text")
	case "response.refusal.delta":
		p.add(delta, StreamDelta{Channel: part + ".refusal", Kind: KindText, Path: out + ".content.refusal"})
	case "response.refusal.done":
		p.ev.Ends = append(p.ev.Ends, part+".refusal")
		p.whole().emit(root.get("refusal"), RoleAssistant, KindText, "refusal")
	case "response.reasoning_summary_text.delta":
		p.add(delta, StreamDelta{Channel: summary, Kind: KindThinking, Path: out + ".summary"})
	case "response.reasoning_summary_text.done":
		p.ev.Ends = append(p.ev.Ends, summary)
		p.whole().emit(root.get("text"), RoleAssistant, KindThinking, "text")
	case "response.reasoning_text.delta":
		p.add(delta, StreamDelta{Channel: part + ".reasoning", Kind: KindThinking, Path: out + ".content"})
	case "response.reasoning_text.done":
		p.ev.Ends = append(p.ev.Ends, part+".reasoning")
		p.whole().emit(root.get("text"), RoleAssistant, KindThinking, "text")
	case "response.function_call_arguments.delta", "response.mcp_call_arguments.delta":
		p.add(delta, StreamDelta{Channel: item + ".args", Kind: KindToolArgs, Path: out + ".arguments"})
	case "response.function_call_arguments.done", "response.mcp_call_arguments.done":
		p.ev.Ends = append(p.ev.Ends, item+".args")
		p.ev.WholeChannel = item + ".args"
		p.whole().nestedJSON(root.get("arguments"), RoleAssistant, KindToolArgs, "arguments")
	case "response.custom_tool_call_input.delta":
		p.add(delta, StreamDelta{Channel: item + ".args", Kind: KindToolArgs, Path: out + ".input", Plain: true})
	case "response.custom_tool_call_input.done":
		p.ev.Ends = append(p.ev.Ends, item+".args")
		p.ev.WholeChannel = item + ".args"
		p.whole().emit(root.get("input"), RoleAssistant, KindToolArgs, "input")
	case "response.code_interpreter_call_code.delta":
		p.add(delta, StreamDelta{Channel: item + ".code", Kind: KindToolArgs, Path: out + ".code", Plain: true})
	case "response.code_interpreter_call_code.done":
		p.ev.Ends = append(p.ev.Ends, item+".code")
		p.whole().emit(root.get("code"), RoleAssistant, KindToolArgs, "code")
	case "response.content_part.added", "response.content_part.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		kind := KindText
		if strings.HasPrefix(typ, "response.reasoning_summary_part") {
			kind = KindThinking
		}
		if typ == "response.content_part.done" {
			p.ev.Ends = append(p.ev.Ends, part)
		}
		if typ == "response.reasoning_summary_part.done" {
			p.ev.Ends = append(p.ev.Ends, summary)
		}
		if pt := root.get("part"); pt != nil && pt.kind == 'o' {
			p.whole().openAIContent(&node{kind: 'a', vals: []*node{pt}}, RoleAssistant, kind, "part")
		}
	case "response.output_item.added":
		p.item(root.get("item"), "item")
		if it := root.get("item"); it != nil && it.kind == 'o' {
			p.name(item+".args", it.get("name"))
		}
	case "response.output_item.done":
		p.ev.Ends = append(p.ev.Ends, item)
		p.item(root.get("item"), "item")
	case "response.completed", "response.failed", "response.incomplete":
		p.ev.EndsAll = true
		// The response object repeats everything the stream carried, and a
		// client's final state is taken from it.
		if resp := root.get("response"); resp != nil && resp.kind == 'o' {
			if output := resp.get("output"); output != nil && output.kind == 'a' {
				p.whole().openAIInput(output, "response.output")
			}
		}
	}
}

// ── Gemini streamGenerateContent ────────────────────────────────────────

func (p *streamParser) gemini(candidates *node) {
	for i, c := range candidates.vals {
		if c.kind != 'o' {
			continue
		}
		ci := p.num(c.get("index"), i)
		ch := "g" + ci
		content := c.get("content")
		if content != nil && content.kind == 'o' {
			if parts := content.get("parts"); parts != nil && parts.kind == 'a' {
				for j, part := range parts.vals {
					if part.kind != 'o' {
						continue
					}
					path := "candidates[" + ci + "].content.parts[" + strconv.Itoa(j) + "]"
					if n := part.get("text"); isString(n) {
						d := StreamDelta{Channel: ch, Kind: KindText}
						d.altStart, d.altEnd = candidates.start, candidates.end
						d.altHead = `[{"content":{"role":"model","parts":[{"text":`
						d.altTail = `}]},"index":` + ci + `}]`
						if t := part.get("thought"); t != nil && string(p.data[t.start:t.end]) == "true" {
							d.Channel, d.Kind, d.Path = ch+".thought", KindThinking, path+".text"
							d.altTail = `,"thought":true` + d.altTail
							// A signature on the part belongs to this text.
							d.Sealed = p.str(part.get("thoughtSignature")) != "" || p.str(part.get("thought_signature")) != ""
						}
						p.add(n, d)
						if len(part.keys) == 1 {
							continue
						}
					}
					partial := false
					for _, key := range []string{"functionCall", "function_call"} {
						if fc := part.get(key); fc != nil && fc.kind == 'o' && fc.get("args") == nil {
							p.geminiPartialCall(fc, ch, ci, path, candidates)
							partial = true
						}
					}
					// Everything else a part can hold arrives whole: a
					// function call, code to run, its result.
					rest := &node{kind: 'o'}
					for k, key := range part.keys {
						if (key != "text" || !isString(part.vals[k])) && !(partial && (key == "functionCall" || key == "function_call")) {
							rest.keys = append(rest.keys, key)
							rest.vals = append(rest.vals, part.vals[k])
						}
					}
					p.whole().geminiPart(rest, RoleAssistant, path)
				}
			}
		}
		if p.present(c.get("finishReason")) || p.present(c.get("finish_reason")) {
			p.ev.Ends = append(p.ev.Ends, ch)
		}
	}
}

// geminiPartialCall reads a function call whose arguments are streamed
// (functionCallingConfig.streamFunctionCallArguments, Gemini 3 on Vertex
// AI). The first event of a call names it; each event after it carries
// partialArgs, values each at a jsonPath of the arguments, a string in
// pieces that share their jsonPath; an event whose functionCall is empty,
// without willContinue, ends the call
// (https://docs.cloud.google.com/vertex-ai/generative-ai/docs/multimodal/function-calling,
// "Streaming function call arguments").
//
// A string is a channel of its own, "g0.f.<jsonPath>", whose pieces are put
// together and read when the call ends, as the arguments of a call that is
// streamed another way are. A number, a boolean or null comes whole. A
// jsonPath that is not made of field names and indexes is read as a key of
// the arguments, and the string under it is read piece by piece where it
// stands: held, it would go out again with the jsonPath as it came.
func (p *streamParser) geminiPartialCall(fc *node, ch, ci, path string, candidates *node) {
	call := ch + ".f"
	p.name(call, fc.get("name"))
	args := path + ".functionCall.args"
	for _, key := range []string{"partialArgs", "partial_args"} {
		list := fc.get(key)
		if list == nil || list.kind != 'a' {
			continue
		}
		p.ev.WholeChannel = call
		for _, a := range list.vals {
			if a.kind != 'o' {
				continue
			}
			jpNode := a.get("jsonPath")
			if jpNode == nil {
				jpNode = a.get("json_path")
			}
			jp := p.str(jpNode)
			at := args + strings.TrimPrefix(jp, "$")
			plain := plainJSONPath(jp)
			if !plain && isString(jpNode) {
				x := p.whole()
				x.res.Segments = append(x.res.Segments, Segment{
					Role: RoleAssistant, Kind: KindToolArgs, Path: args, Text: jp, Start: jpNode.start, End: jpNode.end, Literal: LiteralKey,
				})
			}
			for i, k := range a.keys {
				v := a.vals[i]
				switch k {
				case "stringValue", "string_value":
					if !plain {
						p.whole().emit(v, RoleAssistant, KindToolArgs, at)
						continue
					}
					d := StreamDelta{Channel: call + "." + jp, Kind: KindToolArgs, Path: at, Plain: true}
					d.altStart, d.altEnd = candidates.start, candidates.end
					d.altHead = `[{"content":{"role":"model","parts":[{"functionCall":{"partialArgs":[{"jsonPath":` + string(encodeString(jp)) + `,"stringValue":`
					d.altTail = `,"willContinue":true}],"willContinue":true}}]},"index":` + ci + `}]`
					p.add(v, d)
				case "numberValue", "number_value":
					x := p.whole()
					n := len(x.res.Segments)
					x.literalNumber(v, RoleAssistant, at)
					if len(x.res.Segments) > n && i < len(a.keyNodes) {
						// A mask is a string, and the member holds a number.
						s := &x.res.Segments[n]
						s.Start, s.Literal = a.keyNodes[i].start, LiteralNumberValue
					}
				}
			}
		}
	}
	more := fc.get("willContinue")
	if more == nil {
		more = fc.get("will_continue")
	}
	if more == nil || string(p.data[more.start:more.end]) != "true" {
		p.ev.Ends = append(p.ev.Ends, call)
	}
}

// plainJSONPath reports whether a JSONPath names a place in the arguments
// with field names and short indexes alone ("$.items[0].title"), so that
// it holds no value of its own.
func plainJSONPath(jp string) bool {
	rest, ok := strings.CutPrefix(jp, "$")
	if !ok {
		return false
	}
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == '.' || r == '[' || r == ']' }) {
		digits := strings.Trim(part, "0123456789") == ""
		if digits && len(part) < minLiteralDigits || !digits && fieldName(part) {
			continue
		}
		return false
	}
	return true
}

// ReplaceStreamText returns data with each delta's text replaced by the
// matching entry of texts. Nothing else in the event changes.
func ReplaceStreamText(data []byte, deltas []StreamDelta, texts []string) []byte {
	out := make([]byte, 0, len(data))
	pos := 0
	for i, d := range deltas {
		out = append(out, data[pos:d.start]...)
		out = append(out, encodeString(texts[i])...)
		pos = d.end
	}
	return append(out, data[pos:]...)
}

// RewriteStreamEvent returns data with each delta's text replaced by the
// matching entry of texts and with edits applied to the event's Whole
// segments (Edit.Seg is an index into ev.Whole). Nothing else changes. It
// fails, changing nothing, when an edit does not fit or the result is not
// JSON.
func RewriteStreamEvent(data []byte, ev StreamEvent, texts []string, edits []Edit) ([]byte, error) {
	if len(edits) == 0 {
		return ReplaceStreamText(data, ev.Deltas, texts), nil
	}
	reps, err := tokenReplacements(data, ev.Whole, edits)
	if err != nil {
		return nil, err
	}
	for i, d := range ev.Deltas {
		reps = append(reps, replacement{d.start, d.end, encodeString(texts[i])})
	}
	out, err := applyReplacements(data, reps)
	if err != nil {
		return nil, err
	}
	if !json.Valid(out) {
		return nil, fmt.Errorf("extract: rewritten event is not JSON")
	}
	return out, nil
}

// StandaloneStreamEvent returns the data of an event that carries text as
// the piece deltas[i] and no other piece: what a client adds to the same
// place as the piece itself. deltas are those of the event data was read
// from.
//
// Where the event at hand can carry more than its pieces (a chat chunk has
// the tool call's id and name in it, and a client that is sent them twice
// writes them twice), the event is rebuilt around the piece alone, keeping
// what identifies the response. Elsewhere it is the event with every other
// piece empty.
func StandaloneStreamEvent(data []byte, deltas []StreamDelta, i int, text string) []byte {
	d := deltas[i]
	if d.altEnd > 0 {
		out := make([]byte, 0, len(data)+len(text))
		out = append(out, data[:d.altStart]...)
		out = append(out, d.altHead...)
		out = append(out, encodeString(text)...)
		out = append(out, d.altTail...)
		return append(out, data[d.altEnd:]...)
	}
	texts := make([]string, len(deltas))
	texts[i] = text
	return ReplaceStreamText(data, deltas, texts)
}

// ArgumentSegments returns what is scanned of a tool call's arguments, put
// together from the pieces of a KindToolArgs channel: the strings inside the
// document when it is JSON, as a buffered answer's arguments are read, and
// the text as it stands when it is not (a document the stream cut short) or
// is not meant to be (d.Plain: a custom tool's input). doc is what
// RewriteArguments takes back.
func ArgumentSegments(d StreamDelta, arguments string) (doc []byte, segs []Segment) {
	doc = encodeString(arguments)
	x := &extractor{src: doc, res: &Result{}}
	if d.Plain {
		x.emit(&node{kind: 's', start: 0, end: len(doc)}, RoleAssistant, KindToolArgs, d.Path)
		x.nameTool(0, d.Tool)
		return doc, x.res.Segments
	}
	x.nestedJSON(&node{kind: 's', start: 0, end: len(doc)}, RoleAssistant, KindToolArgs, d.Path)
	x.nameTool(0, d.Tool)
	if d.objectArgs {
		// Where the buffered answer has an object, its members are reached
		// with a dot: content[1].input.city.
		for i := range x.res.Segments {
			s := &x.res.Segments[i]
			if rest, ok := strings.CutPrefix(s.Path, d.Path+"#"); ok {
				// A key of the top object is in the object itself.
				if rest != "" && !strings.HasPrefix(rest, "[") {
					rest = "." + rest
				}
				s.Path = d.Path + rest
			}
		}
	}
	return doc, x.res.Segments
}

// RewriteArguments applies edits to the segments ArgumentSegments returned
// and gives the arguments back. Strings are re-encoded where they stand, so
// a document that was JSON still is.
func RewriteArguments(doc []byte, segs []Segment, edits []Edit) (string, error) {
	out, err := Rewrite(doc, segs, edits)
	if err != nil {
		return "", err
	}
	var arguments string
	if err := json.Unmarshal(out, &arguments); err != nil {
		return "", fmt.Errorf("extract: rewritten arguments are not a string: %w", err)
	}
	return arguments, nil
}
