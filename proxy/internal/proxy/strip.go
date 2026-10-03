package proxy

import (
	"errors"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/vault"
)

// stripPlaceholder is what the model reads in place of a stripped piece of
// text. It says that something was taken out, so the model does not treat
// an empty tool result as the tool's answer.
const stripPlaceholder = "[Content removed by Tamga security policy.]"

// stripResult is a request body with the stripped segments replaced.
type stripResult struct {
	body     []byte
	stripped int
	redacted int
	mapping  map[string]string
}

var (
	errStripRaw      = errors.New("raw")      // the body was not read by segment
	errStripUnplaced = errors.New("unplaced") // a value to redact has no position
)

// stripSegments applies STRIP: every segment that holds a finding whose own
// action is STRIP is replaced whole by the placeholder.
//
// STRIP being the verdict does not excuse the rest of the request. A
// finding elsewhere whose rule says REDACT is still redacted, and if that
// cannot be done the caller blocks, as it would have without the strip.
func stripSegments(provider string, body []byte, segs []extract.Segment, findings []scanner.Finding,
	actionOf func(scanner.Finding) policy.Action, vaultOn bool) (stripResult, error) {
	var out stripResult
	if segs == nil {
		return out, errStripRaw
	}

	strip := make(map[int]bool)
	for _, f := range findings {
		if actionOf(f) == policy.ActionStrip && f.Seg >= 0 && f.Seg < len(segs) {
			strip[f.Seg] = true
		}
	}

	// What is left to redact: findings outside the stripped segments, when
	// any of them is under a REDACT rule.
	var rest []scanner.Finding
	needRedact := false
	for _, f := range findings {
		if strip[f.Seg] {
			continue
		}
		rest = append(rest, f)
		if actionOf(f) == policy.ActionRedact {
			needRedact = true
		}
	}

	var edits []extract.Edit
	for seg := range strip {
		edits = append(edits, extract.Edit{Seg: seg, From: 0, To: len(segs[seg].Text), Replacement: stripPlaceholder})
	}
	if needRedact {
		if scanner.UnplacedRedactions(rest) > 0 {
			return out, errStripUnplaced
		}
		if vaultOn {
			var more []extract.Edit
			more, out.mapping = vault.TokenizeEdits(segs, rest)
			edits = append(edits, more...)
		} else {
			edits = append(edits, scanner.RedactEdits(rest)...)
		}
		out.redacted = scanner.PlacedRedactions(rest)
	}

	rewritten, err := extract.RewriteChecked(provider, body, segs, edits)
	if err != nil {
		return stripResult{}, err
	}
	out.body = rewritten
	out.stripped = len(strip)
	return out, nil
}
