package proxy

import (
	"encoding/json"
	"errors"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/vault"
)

// What REDACT does to a request, in one place, for the proxy path, the
// redaction that rides along with STRIP, and the scan API.
//
// The verdict is one action for the whole request; masking is per finding.
// A finding is masked when its own rule (or its entry's own action) says
// REDACT, whatever its type: an e-mail address, a key, a customer number, a
// competitor's name. PII and custom-entity findings are masked as well once
// anything is, whatever their own action: that is what a REDACT verdict has
// always done, and a request that is being rewritten anyway does not go out
// with an identifier left in it.
//
// The outcome is one of two things, never a third: every such finding is
// masked in the body that is forwarded, or the request is blocked.
//
// Masking is by position, and a finding is one place in the text: a value
// written twice is two findings, and both are masked. A value written so
// that only a normalised reading shows it (fullwidth digits, a zero-width
// space inside it, entities, number words, digits with dots between them)
// has the position of the characters it was read from and is masked like
// any other. The request is blocked when a finding to mask has no position
// (it was found only in a text the request decodes to: base64, hex, hidden
// characters), is of a type without spans (scanner.MaskableType), or the
// rewritten body cannot be confirmed. The response then says so in
// X-Tamga-Redact-Fallback.

// Reasons a redaction cannot be carried out. They are the suffix of the
// X-Tamga-Redact-Fallback header ("block:<reason>") and of the degraded
// counter ("redact_<reason>").
const (
	redactUnplaced   = "unplaced"       // a value to mask was found only in a decoded payload and has no position in the text
	redactUnmaskable = "unmaskable"     // a finding to mask is a verdict on the text, not a span in it
	redactNothing    = "nothing_masked" // REDACT was the verdict and nothing was there to mask
	redactRewrite    = "rewrite_failed" // the rewritten body could not be confirmed
)

// redactTargets returns the findings a REDACT masks: see above.
func redactTargets(findings []scanner.Finding, actionOf func(scanner.Finding) policy.Action) []scanner.Finding {
	var out []scanner.Finding
	for _, f := range findings {
		if f.Type == "pii" || f.Type == "custom" || actionOf(f) == policy.ActionRedact {
			out = append(out, f)
		}
	}
	return out
}

// redactBlocker returns why the targets cannot all be masked, and "" when
// they can.
func redactBlocker(targets []scanner.Finding) string {
	for _, f := range targets {
		if !scanner.MaskableType(f.Type) {
			return redactUnmaskable
		}
	}
	if scanner.UnplacedRedactions(targets) > 0 {
		return redactUnplaced
	}
	return ""
}

// maskEdits returns the edits that mask the targets, and the vault mapping
// when the vault is on.
//
// With the vault on, a finding the vault takes (vault.Tokenizable: PII and
// custom entities) gets a numbered placeholder that is put back in the
// answer. Everything else gets the fixed marker and is not put back: a
// secret that reappeared in the answer would have been masked for nothing.
// A finding the vault would take, lying across one it does not, gets the
// fixed marker too, so that no part of the other value is kept for restoring.
func maskEdits(segs []extract.Segment, targets []scanner.Finding, vaultOn bool) ([]extract.Edit, map[string]string) {
	if !vaultOn {
		return scanner.RedactEdits(targets), nil
	}
	var tokens, fixed []scanner.Finding
	for _, f := range targets {
		if vault.Tokenizable(f) && !overlapsFixed(f, targets) {
			tokens = append(tokens, f)
		} else {
			fixed = append(fixed, f)
		}
	}
	edits, mapping := vault.TokenizeEdits(segs, tokens)
	return append(edits, scanner.RedactEdits(fixed)...), mapping
}

// overlapsFixed reports whether f shares text with a target the vault does
// not take.
func overlapsFixed(f scanner.Finding, targets []scanner.Finding) bool {
	for _, g := range targets {
		if vault.Tokenizable(g) || !g.Placed() || g.Seg != f.Seg {
			continue
		}
		if g.StartPos < f.EndPos && f.StartPos < g.EndPos {
			return true
		}
	}
	return false
}

var errNothingMasked = errors.New(redactNothing)

// redactSegments masks the targets in a body that was read by segment. The
// rewrite is confirmed by reading the new body back (extract.RewriteChecked).
func redactSegments(provider string, body []byte, segs []extract.Segment, targets []scanner.Finding, vaultOn bool) ([]byte, map[string]string, error) {
	edits, mapping := maskEdits(segs, targets, vaultOn)
	if len(edits) == 0 {
		return nil, nil, errNothingMasked
	}
	out, err := extract.RewriteChecked(provider, body, segs, edits)
	if err != nil {
		return nil, nil, err
	}
	return out, mapping, nil
}

// redactRaw masks the targets in a body that was scanned as it came: not a
// request shape the extractor knows, or not JSON. The findings' positions
// are offsets into the body itself (scanner.ScanRaw), so the spans are cut
// out of the bytes.
//
// There is no extractor to read the result back with. What can be checked
// is: a body that was JSON must still be JSON, since a marker that swallowed
// a quote would hand the provider a different document.
func redactRaw(body []byte, targets []scanner.Finding, vaultOn bool) ([]byte, map[string]string, error) {
	whole := []extract.Segment{{Text: string(body)}}
	for _, f := range targets {
		if f.Seg != 0 || f.EndPos > len(body) {
			return nil, nil, errors.New("finding outside the body")
		}
	}
	edits, mapping := maskEdits(whole, targets, vaultOn)
	if len(edits) == 0 {
		return nil, nil, errNothingMasked
	}
	out := []byte(extract.ApplyEdits(whole[0].Text, edits))
	if json.Valid(body) && !json.Valid(out) {
		return nil, nil, errors.New("the masked body is no longer JSON")
	}
	return out, mapping, nil
}

// redactRequest carries out a REDACT verdict on a request. reason is empty
// when body is the masked request; otherwise it says why the request has to
// be blocked.
func redactRequest(provider string, body []byte, segs []extract.Segment, findings []scanner.Finding,
	actionOf func(scanner.Finding) policy.Action, vaultOn bool) (masked []byte, mapping map[string]string, targets []scanner.Finding, reason string, err error) {
	targets = redactTargets(findings, actionOf)
	if reason = redactBlocker(targets); reason != "" {
		return nil, nil, targets, reason, nil
	}
	if segs != nil {
		masked, mapping, err = redactSegments(provider, body, segs, targets, vaultOn)
	} else {
		masked, mapping, err = redactRaw(body, targets, vaultOn)
	}
	switch {
	case errors.Is(err, errNothingMasked):
		return nil, nil, targets, redactNothing, nil
	case err != nil:
		return nil, nil, targets, redactRewrite, err
	}
	return masked, mapping, targets, "", nil
}
