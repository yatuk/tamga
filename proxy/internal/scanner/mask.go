package scanner

import (
	"context"

	"github.com/yatuk/tamga/internal/extract"
)

// maskableTypes are the finding types whose StartPos and EndPos mark the
// matched value in the text as written, so that the value can be cut out and
// a marker put in its place.
//
// The list names what can be masked, not what cannot: a type nobody listed
// (a scanner added later, a remote scanner's own) is not masked on a guess,
// and a REDACT verdict that rests on it blocks.
//
// The other types are verdicts on a piece of text, not values in it:
//
//   - injection (the pattern scanners, the jailbreak heuristics and the
//     inline classifier): the positions, where there are any, may be offsets
//     into a normalised or decoded view of the text;
//   - content_moderation and operator_state: no position at all;
//   - code_leak: the "span" is the whole text;
//   - system_prompt_leak: the canary, which is acted on by canary.block_on_leak;
//   - output_schema: a verdict on the answer as a document.
//
// A malicious_link finding is made on answers only, and its span is the
// link's address.
var maskableTypes = map[string]bool{
	"pii":            true,
	"secret":         true,
	"custom":         true,
	"competitor":     true,
	"malicious_link": true,
	// The address of a place a tool call sends data to: masked, the call
	// goes nowhere.
	ToolDestinationFindingType: true,
}

// MaskableType reports whether findings of this type have a span that REDACT
// can mask.
func MaskableType(typ string) bool { return maskableTypes[typ] }

// Maskable reports whether this finding can be masked: its type has spans,
// and this one has a position in the text as written.
func Maskable(f Finding) bool { return maskableTypes[f.Type] && f.Placed() }

// MaskMarker is what is written in place of a masked finding. It names the
// kind of value and nothing of the value itself.
//
// For most types the kind is the category: "email", "aws_access_key", the
// name of a custom entity (held to letters, digits and underscore by the
// policy's bounds). A competitor's category is the competitor's name, which
// is the very thing being taken out, so its marker names the type.
func MaskMarker(f Finding) string {
	kind := f.Category
	if f.Type == "competitor" || kind == "" {
		kind = f.Type
	}
	return "[" + kind + "_REDACTED]"
}

// ScanRaw scans a body that is not a request shape the extractor knows, as
// one piece of text. The findings are settled the way ScanSegments settles
// those of a segment: a match found only in a text the body decodes to has
// no position, and one place in the body is reported once however many
// readings it was found in. Without that a value was reported twice with
// the same span, and a position in a decoded text was taken for one in the
// body.
//
// The findings carry no role and no path: nothing is known about who the
// text speaks for.
func (r *Registry) ScanRaw(ctx context.Context, content []byte, cfg PipelineConfig) ([]Finding, error) {
	perSeg, err := r.scanUnits(ctx, []extract.Segment{{Text: string(content)}}, cfg)
	return dedupeFindings(perSeg[0]), err
}

// SettleRaw does for findings a remote scanner made on a raw body what
// ScanRaw does for the local ones.
func SettleRaw(findings []Finding, content []byte) []Finding {
	seg := extract.Segment{Text: string(content)}
	for i := range findings {
		placeFinding(&findings[i], 0, seg)
	}
	return dedupeFindings(findings)
}
