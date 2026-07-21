package operator_state

import "time"

// Advisory verdicts returned by the semantic (LLM-as-judge) tier.
const (
	VerdictContradictionConfirmed = "contradiction_confirmed"
	VerdictNoContradiction        = "no_contradiction"
	VerdictInconclusive           = "inconclusive"
)

// Advisory is the semantic tier's judgement on a paraphrased-contradiction
// candidate. It is advisory, not enforcing: the operator decides. Provenance
// ties the verdict back to the audit-log state it was judged against.
type Advisory struct {
	RequestID  string     `json:"request_id"`
	Verdict    string     `json:"verdict"` // contradiction_confirmed | no_contradiction | inconclusive
	Confidence float64    `json:"confidence"`
	Provenance Provenance `json:"provenance"`
}

// Provenance records what state the advisory was based on.
type Provenance struct {
	// DecisionIDs the analyzer compared the prompt against.
	DecisionIDs []string `json:"decision_ids"`
	// LogTimestamps are the audit-log timestamps of the last event backing
	// each decision's projected state, index-aligned with DecisionIDs.
	LogTimestamps []string `json:"log_timestamps"`
	// AnalyzerModel identifies the judge (empty until the Python-side
	// operator-state endpoint ships).
	AnalyzerModel string    `json:"analyzer_model,omitempty"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
}
