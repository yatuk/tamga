package proxy

import (
	"sort"
	"strconv"

	"github.com/yatuk/tamga/internal/extract"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// agents.provenance (policy/agents.go): a high-risk tool call in the answer
// to a request whose conversation carried tool content the policy does not
// trust is withheld, or recorded.
//
// The taint is worked out per request, from the request's own findings:
// an agent sends the whole conversation each time, so a tool result read
// twenty turns ago is in the request, and scanned, as long as it is in the
// model's context. Nothing is kept between requests. A finding the policy
// blocks never gets this far; what taints is what it let through: a
// warning, a signal, a stripped tool result (the client sends it again
// unstripped, and the rest of what the attacker wrote may still be there),
// a classifier score under the threshold.

// provenanceTaint says what made a request tainted. nil means it is not,
// or the policy does not turn provenance on.
type provenanceTaint struct {
	set policy.ProvenanceSettings
	// sources are the pieces of tool content that taint, one per segment,
	// in the order of the conversation.
	sources []taintSource
}

// taintSource is one piece of tool content that taints the request. It
// names the piece by where it stands in the request and what was found in
// it, never by its text.
type taintSource struct {
	seg            int
	path, role     string
	typ, category  string
	score          float64
	classifierOnly bool
	rank           int
}

// taintRank orders the findings of one piece of tool content: a verdict
// above a signal, then by severity.
func taintRank(f scanner.Finding) int {
	rank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}[f.Severity]
	if !scanner.Signal(f) {
		rank += 10
	}
	return rank
}

// requestTaint works out whether a request is tainted. segs and findings are
// the request's; scores are the classifier's, by segment (classifyOutcome).
//
// A finding counts when it lies in tool content (role tool: tool results,
// attached documents, search results) and the policy's taint_on names it. A
// finding without a role comes from a body that was not read by message:
// where it stood is not known, and it counts as rules count it, for every
// role.
func requestTaint(set policy.ProvenanceSettings, segs []extract.Segment, findings []scanner.Finding, scores map[int]float64) *provenanceTaint {
	bySeg := make(map[int]taintSource)
	for _, f := range findings {
		if f.Role != "" && f.Role != string(extract.RoleTool) {
			continue
		}
		if !set.Taints(f) {
			continue
		}
		seg := f.Seg
		if f.Role == "" {
			seg = -1
		}
		// Of the findings in one piece, the record names the strongest.
		if have, ok := bySeg[seg]; ok && have.rank >= taintRank(f) {
			continue
		}
		bySeg[seg] = taintSource{seg: seg, path: f.Path, role: f.Role, typ: f.Type, category: f.Category, rank: taintRank(f)}
	}
	if set.ClassifierScore > 0 {
		for i, score := range scores {
			if i < 0 || i >= len(segs) || segs[i].Role != extract.RoleTool || score < set.ClassifierScore {
				continue
			}
			if _, have := bySeg[i]; have {
				continue
			}
			bySeg[i] = taintSource{seg: i, path: segs[i].Path, role: string(segs[i].Role), typ: "injection", category: "classifier_score", score: score, classifierOnly: true}
		}
	}
	if len(bySeg) == 0 {
		return nil
	}
	t := &provenanceTaint{set: set}
	for _, s := range bySeg {
		t.sources = append(t.sources, s)
	}
	sort.Slice(t.sources, func(i, j int) bool { return t.sources[i].seg < t.sources[j].seg })
	return t
}

// blocks reports whether a high-risk tool call is withheld.
func (t *provenanceTaint) blocks() bool { return t != nil && t.set.Action == policy.ActionBlock }

// latest is the source the record names: the newest piece of tool content
// that taints, the one closest to the tool call.
func (t *provenanceTaint) latest() taintSource { return t.sources[len(t.sources)-1] }

// finding is the tool_provenance finding for a call of a high-risk tool,
// placed on the call in the answer.
func (t *provenanceTaint) finding(call extract.ToolCall) scanner.Finding {
	src := t.latest()
	meta := map[string]string{
		"source_role":     src.role,
		"source_path":     src.path,
		"source_type":     src.typ,
		"source_category": src.category,
		"sources":         strconv.Itoa(len(t.sources)),
	}
	if call.Name != "" {
		meta["tool"] = call.Name
	}
	if src.classifierOnly {
		meta["source_score"] = strconv.FormatFloat(roundScore(src.score), 'f', -1, 64)
	}
	if src.role == "" {
		// The body was scanned raw: no message to point at.
		meta["source_role"] = "unknown"
		delete(meta, "source_path")
	}
	return scanner.Finding{
		Type:     scanner.ToolProvenanceFindingType,
		Category: scanner.TaintedToolCall,
		Severity: "high",
		// The tool's name, which is all the call is judged by.
		Match:          call.Name,
		Confidence:     1.0,
		Seg:            -1,
		Role:           string(extract.RoleAssistant),
		Path:           call.Path,
		Metadata:       meta,
		ScannerVersion: scanner.ScannerVersions[scanner.ToolProvenanceFindingType],
	}
}

// callFindings returns a finding for each high-risk tool among calls, one
// per tool name, leaving out the names in seen and adding the ones it
// reports to it.
func (t *provenanceTaint) callFindings(calls []extract.ToolCall, seen map[string]bool) []scanner.Finding {
	if t == nil {
		return nil
	}
	var out []scanner.Finding
	for _, c := range calls {
		if seen[c.Name] || !t.set.HighRisk(c.Name) {
			continue
		}
		seen[c.Name] = true
		out = append(out, t.finding(c))
	}
	return out
}

// answerProvenance reads a buffered answer for tool calls held to the
// taint. It returns nothing for an answer in a shape the extractor does not
// know, which carries no call the client would run either.
func answerProvenance(t *provenanceTaint, provider string, raw []byte) []scanner.Finding {
	if t == nil || len(raw) == 0 {
		return nil
	}
	res, ok := extract.ExtractResponse(provider, raw)
	if !ok {
		return nil
	}
	return t.callFindings(res.ToolCalls, map[string]bool{})
}

// provenanceBlockBody is the answer a client gets in place of one whose
// tool call was withheld.
func provenanceBlockBody(requestID string) []byte {
	return []byte(`{"error":{"message":"Response blocked by Tamga: a high-risk tool call after tool content the policy does not trust","type":"output_policy_block","reason":"tool_call_provenance","request_id":"` + requestID + `"}}`)
}
