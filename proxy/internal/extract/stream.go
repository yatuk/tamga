package extract

import "strconv"

// StreamDelta is a piece of model text carried by one streamed event.
type StreamDelta struct {
	// Channel names the run of text the piece belongs to. One response can
	// stream several at once (choices, content blocks); a value split over
	// events is only whole within its own channel.
	Channel string
	// Text is the decoded piece.
	Text string
	// start and end delimit the JSON string token in the event's data.
	start, end int
}

// StreamEvent is what one streamed event's JSON payload means for the text
// being streamed.
type StreamEvent struct {
	// Deltas are the pieces of text the event adds.
	Deltas []StreamDelta
	// Ends are the channels the event closes; EndsAll closes every one.
	Ends    []string
	EndsAll bool
}

// ParseStreamEvent reads the data of one streamed event. It returns false
// when the data is not JSON it can read; such an event carries no text this
// package knows how to find.
//
// Known shapes: OpenAI chat and legacy completions chunks, OpenAI Responses
// events, Anthropic message events, Gemini streamGenerateContent chunks.
func ParseStreamEvent(data []byte) (StreamEvent, bool) {
	root, err := parseTree(data)
	if err != nil || root.kind != 'o' {
		return StreamEvent{}, false
	}
	var ev StreamEvent
	str := func(n *node) (string, bool) {
		if n == nil || n.kind != 's' {
			return "", false
		}
		return n.text(data), true
	}
	add := func(channel string, n *node) {
		if text, ok := str(n); ok {
			ev.Deltas = append(ev.Deltas, StreamDelta{Channel: channel, Text: text, start: n.start, end: n.end})
		}
	}
	index := func(n *node, fallback int) string {
		if n != nil && n.kind == 'x' {
			return string(data[n.start:n.end])
		}
		return strconv.Itoa(fallback)
	}
	present := func(n *node) bool { return n != nil && string(data[n.start:n.end]) != "null" }

	typ, _ := str(root.get("type"))
	switch {
	case root.get("choices") != nil && root.get("choices").kind == 'a':
		// OpenAI chat chunk or legacy completion chunk.
		for i, c := range root.get("choices").vals {
			if c.kind != 'o' {
				continue
			}
			ch := "c" + index(c.get("index"), i)
			if d := c.get("delta"); d != nil && d.kind == 'o' {
				add(ch, d.get("content"))
			}
			add(ch, c.get("text"))
			if present(c.get("finish_reason")) {
				ev.Ends = append(ev.Ends, ch)
			}
		}
	case typ == "content_block_delta":
		// Anthropic. Only text is model prose; tool input and thinking
		// arrive as other delta types.
		if d := root.get("delta"); d != nil && d.kind == 'o' {
			if dt, _ := str(d.get("type")); dt == "text_delta" {
				add("b"+index(root.get("index"), 0), d.get("text"))
			}
		}
	case typ == "content_block_stop":
		ev.Ends = append(ev.Ends, "b"+index(root.get("index"), 0))
	case typ == "message_delta" || typ == "message_stop" || typ == "error":
		ev.EndsAll = true
	case typ == "response.output_text.delta":
		// OpenAI Responses API.
		add("r"+index(root.get("output_index"), 0)+"."+index(root.get("content_index"), 0), root.get("delta"))
	case len(typ) > 9 && typ[:9] == "response." && (hasSuffix(typ, ".done") || typ == "response.completed" || typ == "response.failed" || typ == "response.incomplete"):
		ev.EndsAll = true
	case root.get("candidates") != nil && root.get("candidates").kind == 'a':
		// Gemini.
		for i, c := range root.get("candidates").vals {
			if c.kind != 'o' {
				continue
			}
			ch := "g" + index(c.get("index"), i)
			if content := c.get("content"); content != nil && content.kind == 'o' {
				if parts := content.get("parts"); parts != nil && parts.kind == 'a' {
					for _, p := range parts.vals {
						if p.kind == 'o' {
							add(ch, p.get("text"))
						}
					}
				}
			}
			if present(c.get("finishReason")) {
				ev.Ends = append(ev.Ends, ch)
			}
		}
	}
	return ev, true
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
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
