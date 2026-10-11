package events

import (
	"encoding/json"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/operator_state"
)

// MapFindings returns e with fn applied to every list of findings it
// carries: the request's, the response's, and the fast-tier findings an
// operator-state deep-scan event takes to the analyzer. fn must return a new
// slice; nothing e shares with its publisher is written to.
func MapFindings(e Event, fn func([]scanner.Finding) []scanner.Finding) Event {
	if fn == nil {
		return e
	}
	if len(e.Findings) > 0 {
		e.Findings = fn(e.Findings)
	}
	if len(e.OutputFindings) > 0 {
		e.OutputFindings = fn(e.OutputFindings)
	}
	if req, _ := e.Metadata[operatorStateRequestKey].(*operator_state.AnalyzerRequest); req != nil && len(req.FastFindings) > 0 {
		mapped := *req
		mapped.FastFindings = mapFastFindings(req.FastFindings, fn)
		meta := make(map[string]interface{}, len(e.Metadata))
		for k, v := range e.Metadata {
			meta[k] = v
		}
		meta[operatorStateRequestKey] = &mapped
		e.Metadata = meta
	}
	return e
}

// mapFastFindings applies fn to findings held as JSON. One that cannot be
// read is left out: it cannot be passed on unchanged.
func mapFastFindings(raw []json.RawMessage, fn func([]scanner.Finding) []scanner.Finding) []json.RawMessage {
	findings := make([]scanner.Finding, 0, len(raw))
	for _, r := range raw {
		var f scanner.Finding
		if err := json.Unmarshal(r, &f); err == nil {
			findings = append(findings, f)
		}
	}
	out := make([]json.RawMessage, 0, len(findings))
	for _, f := range fn(findings) {
		if b, err := json.Marshal(f); err == nil {
			out = append(out, b)
		}
	}
	return out
}
