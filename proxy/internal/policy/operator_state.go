package policy

// OperatorStateAssertion is a single operator-state assertion rule: decisions
// matching DecisionPattern must be in RequiredState, otherwise ActionOnFail
// is applied to the request.
type OperatorStateAssertion struct {
	// DecisionPattern is a regex matching decision IDs (e.g. "D-2026-06-.*").
	DecisionPattern string `yaml:"decision_pattern" json:"decision_pattern"`

	// RequiredState is the state the decision must be in
	// (proposed | accepted | locked | rejected | superseded).
	RequiredState string `yaml:"required_state" json:"required_state"`

	// ActionOnFail is the action taken when the assertion fails (block | warn | log).
	ActionOnFail string `yaml:"action_on_fail" json:"action_on_fail"`

	// Severity is the finding severity (critical | high | medium | low).
	Severity string `yaml:"severity" json:"severity"`

	// Description is a human-readable explanation of the assertion.
	Description string `yaml:"description" json:"description"`
}

// OperatorAuthorization allowlists operators for decisions matching a pattern.
// Requests referencing a matching decision with an operator id outside the
// allowlist produce an unauthorized_operator finding.
type OperatorAuthorization struct {
	DecisionPattern  string   `yaml:"decision_pattern" json:"decision_pattern"`
	AllowedOperators []string `yaml:"allowed_operators" json:"allowed_operators"`
}

// OperatorStateConfig configures the operator_state scanner
// (jugeni-contracts v1 audit-log integration).
type OperatorStateConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`

	// OnUnknownRef controls what happens when a prompt references a decision id
	// that is not present in the audit-log projection: "deny" (default) emits a
	// blocking unknown_decision_ref finding; "allow" passes silently.
	OnUnknownRef string `yaml:"on_unknown_ref" json:"on_unknown_ref"`

	// FreshnessTTL is the default verification cadence for locked decisions as a
	// Go duration string (e.g. "168h"). A locked decision whose last verifiable-by
	// diagnostic fired longer ago than this is reported as a stale_lock.
	// RequestContext.FreshnessTTL overrides it per request when set.
	FreshnessTTL string `yaml:"freshness_ttl" json:"freshness_ttl"`

	Assertions    []OperatorStateAssertion `yaml:"assertions" json:"assertions"`
	Authorization []OperatorAuthorization  `yaml:"authorization" json:"authorization"`
}
