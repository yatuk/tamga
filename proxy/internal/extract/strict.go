// Package extract reads LLM request bodies the way the provider will.
//
// The proxy decides on the text it scans; the provider acts on the text it
// parses. Wherever the two can read one body differently, an attacker can
// show the proxy something harmless and the model something else. This
// package exists to close that gap.
package extract

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// Reasons a body is refused by CheckStrict. They are the "reason" label of
// tamga_malformed_json_total and the "reason" field of the 400 response.
const (
	ReasonInvalidUTF8    = "invalid_utf8"
	ReasonNotJSON        = "not_json"
	ReasonDuplicateKey   = "duplicate_key"
	ReasonSurrogateInKey = "surrogate_in_key"
	ReasonTooDeep        = "too_deep"
)

// maxDepth bounds nesting. Chat bodies nest a handful of levels; tool-call
// arguments a few more. Past this the body is not one a provider accepts.
const maxDepth = 128

// StrictError says why a body is ambiguous and where.
type StrictError struct {
	Reason string
	// Offset is the byte position in the body, -1 when not known.
	Offset int
	// Key is the offending object key, when the reason concerns one.
	Key string
}

func (e *StrictError) Error() string {
	switch e.Reason {
	case ReasonDuplicateKey:
		return fmt.Sprintf("duplicate key %q at byte %d", e.Key, e.Offset)
	case ReasonSurrogateInKey:
		return fmt.Sprintf("unpaired surrogate in an object key at byte %d", e.Offset)
	case ReasonInvalidUTF8:
		return fmt.Sprintf("invalid UTF-8 at byte %d", e.Offset)
	case ReasonTooDeep:
		return fmt.Sprintf("nesting deeper than %d levels", maxDepth)
	default:
		return "body is not valid JSON"
	}
}

// CheckStrict reports whether body is JSON that every parser reads the same
// way. It refuses:
//
//   - a key that appears twice in one object. Parsers disagree on which
//     value wins (Go and Python take the last, others the first), so
//     {"messages":[benign],"messages":[attack]} can be scanned as one and
//     executed as the other;
//   - an unpaired surrogate escape in a key. Some parsers drop it, turning
//     two different keys into the same one;
//   - bytes that are not UTF-8, which Go would silently replace;
//   - anything that is not RFC 8259 JSON (comments, trailing commas), since
//     a lenient parser behind the proxy would see keys a strict one does not.
//
// An unpaired surrogate inside a string value is allowed: clients that cut
// text in the middle of an emoji produce them, and they cannot be used to
// swap one value for another.
//
// A nil return means the body is unambiguous, not that it is a valid request.
func CheckStrict(body []byte) *StrictError {
	if !utf8.Valid(body) {
		return &StrictError{Reason: ReasonInvalidUTF8, Offset: firstInvalidUTF8(body)}
	}
	if !json.Valid(body) {
		return &StrictError{Reason: ReasonNotJSON, Offset: -1}
	}

	// json.Valid has established the grammar, so the walk below only has to
	// tell keys from values and remember the keys of each open object.
	type frame struct {
		object    bool
		expectKey bool
		keys      map[string]struct{}
	}
	var stack []frame

	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '{':
			if len(stack) >= maxDepth {
				return &StrictError{Reason: ReasonTooDeep, Offset: i}
			}
			stack = append(stack, frame{object: true, expectKey: true})
		case '[':
			if len(stack) >= maxDepth {
				return &StrictError{Reason: ReasonTooDeep, Offset: i}
			}
			stack = append(stack, frame{})
		case '}', ']':
			stack = stack[:len(stack)-1]
		case ',':
			if n := len(stack); n > 0 && stack[n-1].object {
				stack[n-1].expectKey = true
			}
		case '"':
			end := stringEnd(body, i)
			raw := body[i : end+1]
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].expectKey {
				top := &stack[n-1]
				top.expectKey = false
				if hasUnpairedSurrogate(raw) {
					return &StrictError{Reason: ReasonSurrogateInKey, Offset: i}
				}
				key := decodeKey(raw)
				if top.keys == nil {
					top.keys = make(map[string]struct{}, 8)
				}
				if _, seen := top.keys[key]; seen {
					return &StrictError{Reason: ReasonDuplicateKey, Offset: i, Key: key}
				}
				top.keys[key] = struct{}{}
			}
			i = end
		}
	}
	return nil
}

// stringEnd returns the index of the quote that closes the string opening at
// start. The body is valid JSON, so the string is known to close.
func stringEnd(body []byte, start int) int {
	for i := start + 1; i < len(body); i++ {
		switch body[i] {
		case '\\':
			i++ // the escaped character is not a terminator
		case '"':
			return i
		}
	}
	return len(body) - 1
}

// decodeKey returns the key a parser sees: "a" and "a" are one key.
func decodeKey(raw []byte) string {
	inner := raw[1 : len(raw)-1]
	for _, c := range inner {
		if c == '\\' {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				return s
			}
			break
		}
	}
	return string(inner)
}

// hasUnpairedSurrogate reports a \uD800–\uDFFF escape that is not half of a
// high-then-low pair.
func hasUnpairedSurrogate(raw []byte) bool {
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		if raw[i+1] != 'u' {
			i++ // skip the escaped character, which may itself be a backslash
			continue
		}
		cp, ok := hex4(raw, i+2)
		if !ok {
			return false // json.Valid would have refused it
		}
		switch {
		case cp >= 0xD800 && cp <= 0xDBFF:
			low, ok := uint32(0), false
			if i+11 < len(raw) && raw[i+6] == '\\' && raw[i+7] == 'u' {
				low, ok = hex4(raw, i+8)
			}
			if !ok || low < 0xDC00 || low > 0xDFFF {
				return true
			}
			i += 11
		case cp >= 0xDC00 && cp <= 0xDFFF:
			return true
		default:
			i += 5
		}
	}
	return false
}

func hex4(b []byte, at int) (uint32, bool) {
	if at+4 > len(b) {
		return 0, false
	}
	var v uint32
	for _, c := range b[at : at+4] {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint32(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= uint32(c-'A') + 10
		default:
			return 0, false
		}
	}
	return v, true
}

func firstInvalidUTF8(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// malformedCounts tracks refused or flagged bodies, keyed by reason.
var malformedCounts sync.Map // map[string]*int64

// RecordMalformed counts one body that CheckStrict found ambiguous.
func RecordMalformed(reason string) {
	val, _ := malformedCounts.LoadOrStore(reason, new(int64))
	atomic.AddInt64(val.(*int64), 1)
}

// MalformedStats returns a snapshot of ambiguous-body counts by reason.
func MalformedStats() map[string]int64 {
	out := make(map[string]int64)
	malformedCounts.Range(func(key, value any) bool {
		out[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	return out
}

// scanModeCounts tracks how request bodies were scanned: "segments" when the
// body was a recognised request, "raw" when it was scanned as plain bytes.
var scanModeCounts sync.Map // map[string]*int64

// RecordScanMode counts one scanned request body.
func RecordScanMode(mode string) {
	val, _ := scanModeCounts.LoadOrStore(mode, new(int64))
	atomic.AddInt64(val.(*int64), 1)
}

// ScanModeStats returns a snapshot of scan counts by mode.
func ScanModeStats() map[string]int64 { return snapshot(&scanModeCounts) }

// unknownBlockCounts tracks content blocks the extractor has no rule for.
// They are scanned generically; a count here says a provider has shipped
// something worth writing a rule for.
var unknownBlockCounts sync.Map // map[string]*int64

// RecordUnknownBlock counts one unknown block type seen in a request.
func RecordUnknownBlock(blockType string) {
	val, _ := unknownBlockCounts.LoadOrStore(blockType, new(int64))
	atomic.AddInt64(val.(*int64), 1)
}

// UnknownBlockStats returns a snapshot of unknown block counts by type.
func UnknownBlockStats() map[string]int64 { return snapshot(&unknownBlockCounts) }

func snapshot(m *sync.Map) map[string]int64 {
	out := make(map[string]int64)
	m.Range(func(key, value any) bool {
		out[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	return out
}
