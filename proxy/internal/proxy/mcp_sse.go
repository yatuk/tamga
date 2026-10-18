package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
)

// A server-sent event stream as the MCP gateway reads and writes it
// (https://html.spec.whatwg.org/multipage/server-sent-events.html). The
// gateway reads an upstream stream event by event, judges each JSON-RPC
// message, and writes the event on with its id, type and retry as they came.
// Comment lines (the keep-alives) are passed on as they arrive.

// mcpSSEEvent is one event of a stream.
type mcpSSEEvent struct {
	id    string
	hasID bool
	event string
	retry string
	data  []byte
	// hasData says a data field was present, even an empty one. An event
	// without one is not dispatched by a reader, and is written as it came.
	hasData bool
	// other holds lines with a field name the format does not define, which
	// a reader ignores. They are not passed on.
	other int
}

// message reports whether the event carries a JSON-RPC message: its type is
// "message", the default.
func (e *mcpSSEEvent) message() bool { return e.event == "" || e.event == "message" }

// errMCPSSELarge is a line longer than the reader's limit.
var errMCPSSELarge = errors.New("sse: event larger than the limit")

// mcpSSEReader reads events from a stream.
type mcpSSEReader struct {
	r     *bufio.Reader
	limit int
	// comment is called with each comment line, without its newline.
	comment func(line []byte) error
}

func newMCPSSEReader(r io.Reader, limit int, comment func([]byte) error) *mcpSSEReader {
	return &mcpSSEReader{r: bufio.NewReaderSize(r, 64<<10), limit: limit, comment: comment}
}

// line returns the next line without its end (CRLF, LF or CR).
func (s *mcpSSEReader) line() ([]byte, error) {
	var buf []byte
	for {
		chunk, err := s.r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > s.limit {
			return nil, errMCPSSELarge
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			if len(buf) > 0 && errors.Is(err, io.EOF) {
				return trimEOL(buf), nil
			}
			return nil, err
		}
		break
	}
	// A lone CR also ends a line; next splits on it.
	return trimEOL(buf), nil
}

func trimEOL(b []byte) []byte {
	b = bytes.TrimSuffix(b, []byte("\n"))
	return bytes.TrimSuffix(b, []byte("\r"))
}

// next returns the next event. io.EOF means the stream ended between events.
func (s *mcpSSEReader) next() (*mcpSSEEvent, error) {
	ev := &mcpSSEEvent{}
	started := false
	size := 0
	for {
		raw, err := s.line()
		if err != nil {
			if errors.Is(err, io.EOF) && started {
				// The stream ended inside an event: a reader drops it.
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		// Lines split on lone CRs, for a stream that ends lines that way.
		for _, ln := range bytes.Split(raw, []byte("\r")) {
			if len(ln) == 0 {
				if started {
					return ev, nil
				}
				continue
			}
			size += len(ln)
			if size > s.limit {
				return nil, errMCPSSELarge
			}
			if ln[0] == ':' {
				if s.comment != nil {
					if err := s.comment(ln); err != nil {
						return nil, err
					}
				}
				continue
			}
			started = true
			field, value := ln, []byte(nil)
			if i := bytes.IndexByte(ln, ':'); i >= 0 {
				field, value = ln[:i], ln[i+1:]
				value = bytes.TrimPrefix(value, []byte(" "))
			}
			switch string(field) {
			case "data":
				if ev.hasData {
					ev.data = append(ev.data, '\n')
				}
				ev.data = append(ev.data, value...)
				ev.hasData = true
			case "id":
				if bytes.IndexByte(value, 0) < 0 {
					ev.id, ev.hasID = string(value), true
				}
			case "event":
				ev.event = string(value)
			case "retry":
				ev.retry = string(value)
			default:
				ev.other++
			}
		}
	}
}

// writeMCPSSEEvent writes an event with data in place of its own. A data that
// holds line breaks is written as one data line per line, which a reader
// joins back with "\n".
func writeMCPSSEEvent(w io.Writer, ev *mcpSSEEvent, data []byte) error {
	var b strings.Builder
	if ev.hasID {
		b.WriteString("id: ")
		b.WriteString(ev.id)
		b.WriteByte('\n')
	}
	if ev.event != "" {
		b.WriteString("event: ")
		b.WriteString(ev.event)
		b.WriteByte('\n')
	}
	if ev.retry != "" {
		b.WriteString("retry: ")
		b.WriteString(ev.retry)
		b.WriteByte('\n')
	}
	if ev.hasData {
		for _, ln := range bytes.Split(data, []byte("\n")) {
			b.WriteString("data: ")
			b.Write(ln)
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err
}
