package policy

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/yatuk/tamga/internal/scanner"
)

// Action represents what to do when a rule matches.
type Action string

const (
	ActionBlock  Action = "BLOCK"
	ActionRedact Action = "REDACT"
	ActionWarn   Action = "WARN"
	ActionLog    Action = "LOG"
	ActionPass   Action = "PASS"
)

type Notify struct {
	Channel string `yaml:"channel" json:"channel"`
	Webhook string `yaml:"webhook" json:"webhook"`
}

type Rule struct {
	Action            Action   `yaml:"action" json:"action"`
	Sensitivity       string   `yaml:"sensitivity" json:"sensitivity"` // low, medium, high
	Types             []string `yaml:"types" json:"types"`
	Notify            []Notify `yaml:"notify" json:"notify"`
	Mode              string   `yaml:"mode" json:"mode"` // legacy(default) or confidence_based
	MinimumConfidence int      `yaml:"minimum_confidence" json:"minimum_confidence"`
	OverrideAction    *Action  `yaml:"override_action" json:"override_action"`
}

type RateLimit struct {
	MaxRequestsPerMinute int    `yaml:"max_requests_per_minute" json:"max_requests_per_minute"`
	MaxTokensPerDay      int    `yaml:"max_tokens_per_day" json:"max_tokens_per_day"`
	ActionOnExceed       Action `yaml:"action_on_exceed" json:"action_on_exceed"`
}

type Providers struct {
	Allowed []string `yaml:"allowed" json:"allowed"`
	Blocked []string `yaml:"blocked" json:"blocked"`
	// Pools defines optional per-route upstream chains (FAZ2 — provider fallback).
	// When set for a route key (e.g. "anthropic"), the proxy uses these endpoints
	// instead of the built-in single-host mapping.
	Pools map[string]ProviderUpstreamPool `yaml:"pools,omitempty" json:"pools,omitempty"`
}

// ProviderUpstreamPool is a named strategy with an ordered list of endpoints.
type ProviderUpstreamPool struct {
	Strategy  string                     `yaml:"strategy" json:"strategy"` // fallback_chain | round_robin
	Endpoints []ProviderUpstreamEndpoint `yaml:"providers" json:"providers"`
}

// BreakerConfig overrides gobreaker defaults for an endpoint.
type BreakerConfig struct {
	FailureThreshold *float64 `yaml:"failure_threshold,omitempty" json:"failure_threshold,omitempty"`
	MinimumRequests  *int     `yaml:"minimum_requests,omitempty" json:"minimum_requests,omitempty"`
}

// ProviderUpstreamEndpoint is one hop in a provider pool (distinct base URL / credentials).
type ProviderUpstreamEndpoint struct {
	Name      string         `yaml:"name" json:"name"`
	BaseURL   string         `yaml:"base_url" json:"base_url"`
	APIKeyEnv string         `yaml:"api_key_env,omitempty" json:"api_key_env,omitempty"`
	Priority  int            `yaml:"priority" json:"priority"`
	Timeout   string         `yaml:"timeout,omitempty" json:"timeout,omitempty"` // e.g. 30s
	Breaker   *BreakerConfig `yaml:"breaker,omitempty" json:"breaker,omitempty"`
}

// CustomEntity is a customer-defined regex pattern (PwC-style "Custom Entity").
type CustomEntity struct {
	Name        string  `yaml:"name" json:"name"`
	Pattern     string  `yaml:"pattern" json:"pattern"`
	Description string  `yaml:"description" json:"description"`
	Severity    string  `yaml:"severity" json:"severity"`
	Action      string  `yaml:"action" json:"action"`
	Confidence  float64 `yaml:"confidence" json:"confidence"`
}

// Competitor defines a competitor brand or product to detect in LLM prompts.
// When a user mentions a competitor in their prompt, the competitor scanner
// fires a finding so the SOC team can track competitive intelligence signals.
type Competitor struct {
	Name        string   `yaml:"name" json:"name"`               // e.g. "Lakera Guard"
	Patterns    []string `yaml:"patterns" json:"patterns"`       // regex patterns to match (case-insensitive)
	Severity    string   `yaml:"severity" json:"severity"`       // "low" | "medium" | "high"
	Action      string   `yaml:"action" json:"action"`           // "log" | "warn" | "block"
	Description string   `yaml:"description" json:"description"` // human-readable note
	Enabled     bool     `yaml:"enabled" json:"enabled"`         // per-competitor toggle
}

// OutputStreamScan configures sliding-window scanning of SSE / NDJSON streams (FAZ1 Prompt 2).
// When Enabled is true, outbound chunks are buffered up to MaxBufferBytes and scanned;
// on BLOCK the stream is terminated with an SSE error event (fail-close).
type OutputStreamScan struct {
	Enabled        bool `yaml:"enabled" json:"enabled"`
	MaxBufferBytes int  `yaml:"max_buffer_bytes" json:"max_buffer_bytes"` // rolling buffer cap (default 8192)
	OverlapBytes   int  `yaml:"overlap_bytes" json:"overlap_bytes"`       // reserved for future overlap tuning
}

// OutputRules configures response scanning (Sprint 6+ Faz 1A).
// When Enabled is true the proxy buffers response bodies up to BufferBytes
// (non-stream) or MaxStreamBytes (stream) and runs the registered scanners.
type OutputRules struct {
	Enabled           bool              `yaml:"enabled" json:"enabled"`
	BufferBytes       int               `yaml:"buffer_bytes" json:"buffer_bytes"`
	MaxStreamBytes    int               `yaml:"max_stream_bytes" json:"max_stream_bytes"`
	ScanWindowMs      int               `yaml:"scan_window_ms" json:"scan_window_ms"`
	BlockOn           []string          `yaml:"block_on" json:"block_on"`   // finding types/categories that trigger BLOCK
	RedactOn          []string          `yaml:"redact_on" json:"redact_on"` // finding types/categories that trigger REDACT
	MinimumConfidence int               `yaml:"minimum_confidence" json:"minimum_confidence"`
	FailOpen          bool              `yaml:"fail_open" json:"fail_open"` // scan timeout → pass raw
	Streaming         *OutputStreamScan `yaml:"streaming" json:"streaming"`
}

// Governance controls dual-control and approval requirements.
type Governance struct {
	RequireDualControl bool `yaml:"require_dual_control" json:"require_dual_control"`
}

// DataControl contains KVKK/GDPR retention + hashing knobs.
type DataControl struct {
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
	HashFindings  bool   `yaml:"hash_findings" json:"hash_findings"`
	Residency     string `yaml:"residency" json:"residency"` // eu|tr|us
}

// PricingTier defines limits and feature flags for a subscription tier.
// Mirrors the public pricing page: Community (free), Team, Business, Enterprise.
type PricingTier struct {
	Name            string `yaml:"name" json:"name"`                                     // community | team | business | enterprise
	MaxRequestsMo   int    `yaml:"max_requests_per_month" json:"max_requests_per_month"` // 0 = unlimited
	SSOEnabled      bool   `yaml:"sso_enabled" json:"sso_enabled"`                       // SAML/OIDC
	RetentionDays   int    `yaml:"retention_days" json:"retention_days"`                 // audit log retention
	SupportSLAHours int    `yaml:"support_sla_hours" json:"support_sla_hours"`           // 0 = community/best-effort
	CustomEntities  bool   `yaml:"custom_entities" json:"custom_entities"`               // custom regex patterns
	AirGapped       bool   `yaml:"air_gapped" json:"air_gapped"`                         // on-prem / self-hosted
}

// Pricing holds the active tier configuration for enforcement.
type Pricing struct {
	ActiveTier string        `yaml:"active_tier" json:"active_tier"` // tier name to enforce
	Tiers      []PricingTier `yaml:"tiers" json:"tiers"`
}

// activeTier returns the PricingTier matching the active_tier name, or nil.
func (p *Pricing) ActiveTierDef() *PricingTier {
	if p == nil || p.ActiveTier == "" {
		return nil
	}
	for i := range p.Tiers {
		if p.Tiers[i].Name == p.ActiveTier {
			return &p.Tiers[i]
		}
	}
	return nil
}

// CostControl — daily token / USD caps (Faz 3B).
type CostControl struct {
	MaxTokensPerDay  int     `yaml:"max_tokens_per_day" json:"max_tokens_per_day"`
	MaxCostUSDPerDay float64 `yaml:"max_cost_usd_per_day" json:"max_cost_usd_per_day"`
}

// BodyLimitsConfig caps incoming LLM request bodies. Per-provider overrides win over default;
// when unset, ResolveMaxBodyBytes falls back to config/env (see proxy handler).
type BodyLimitsConfig struct {
	Default     BodyLimitRule            `yaml:"default" json:"default"`
	PerProvider map[string]BodyLimitRule `yaml:"per_provider" json:"per_provider"`
}

// BodyLimitRule holds a single byte limit for requests.
type BodyLimitRule struct {
	MaxBytes int `yaml:"max_bytes" json:"max_bytes"`
}

// CacheConfig — semantic/exact cache (Faz 3C).
type CacheConfig struct {
	Enabled    bool `yaml:"enabled" json:"enabled"`
	TTLSeconds int  `yaml:"ttl_seconds" json:"ttl_seconds"`
	SemanticOn bool `yaml:"semantic" json:"semantic"`
}

// Exception defines a policy rule bypass for specific roles.
// When a user with a matching role hits a rule covered by an exception,
// that rule is skipped entirely — BLOCK becomes allow, REDACT returns raw content.
type Exception struct {
	Rule      string   `yaml:"rule" json:"rule"`
	Roles     []string `yaml:"roles" json:"roles"`
	Reason    string   `yaml:"reason" json:"reason"`
	ExpiresAt string   `yaml:"expires_at" json:"expires_at"` // optional RFC3339; empty means permanent
}

// ExceptionMatch records an applied exception for audit logging.
type ExceptionMatch struct {
	Rule   string `json:"rule"`
	Role   string `json:"role"`
	Reason string `json:"reason"`
}

// ValidRoles is the set of known RBAC roles that can be used in exceptions.
// Custom roles beyond this set can be registered by the platform operator.
var ValidRoles = map[string]struct{}{
	"admin":         {},
	"analyst":       {},
	"viewer":        {},
	"security_lead": {},
}

// IsValidRole reports whether the given role name is a known valid role.
func IsValidRole(role string) bool {
	_, ok := ValidRoles[strings.ToLower(strings.TrimSpace(role))]
	return ok
}

// isExceptionExpired checks if an exception's expires_at date has passed.
// An empty expires_at means permanent (never expires).
func isExceptionExpired(expiresAt string) bool {
	if expiresAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		// Malformed expiry: treat as expired (silently ignored per spec).
		return true
	}
	return time.Now().UTC().After(t)
}

// IsRuleExempted checks whether the given role has a non-expired exception
// for the named rule. Returns (true, ExceptionMatch) if the rule is exempted.
func (p *Policy) IsRuleExempted(ruleName, role string) (bool, ExceptionMatch) {
	if p == nil || len(p.Exceptions) == 0 || role == "" {
		return false, ExceptionMatch{}
	}
	roleNormalized := strings.ToLower(strings.TrimSpace(role))
	for _, exc := range p.Exceptions {
		if !strings.EqualFold(exc.Rule, ruleName) {
			continue
		}
		if isExceptionExpired(exc.ExpiresAt) {
			continue
		}
		for _, r := range exc.Roles {
			if strings.EqualFold(strings.TrimSpace(r), roleNormalized) {
				return true, ExceptionMatch{
					Rule:   exc.Rule,
					Role:   roleNormalized,
					Reason: exc.Reason,
				}
			}
		}
	}
	return false, ExceptionMatch{}
}

// EvaluateWithRole evaluates findings with RBAC exception awareness.
// When strictMode is true, all exceptions are ignored.
// Applied exceptions are returned for audit logging purposes.
func (p *Policy) EvaluateWithRole(findings []scanner.Finding, role string, strictMode bool) (Action, []ExceptionMatch) {
	if len(findings) == 0 {
		return ActionPass, nil
	}

	var appliedExceptions []ExceptionMatch

	// Build a set of exempted rule keys for this role (only if strictMode is off and we have a role).
	ruleExempted := make(map[string]ExceptionMatch)
	if !strictMode && role != "" && p != nil && len(p.Exceptions) > 0 {
		for _, exc := range p.Exceptions {
			if isExceptionExpired(exc.ExpiresAt) {
				continue
			}
			roleNormalized := strings.ToLower(strings.TrimSpace(role))
			for _, r := range exc.Roles {
				if strings.EqualFold(strings.TrimSpace(r), roleNormalized) {
					ruleExempted[strings.ToLower(exc.Rule)] = ExceptionMatch{
						Rule:   exc.Rule,
						Role:   roleNormalized,
						Reason: exc.Reason,
					}
					break
				}
			}
		}
	}

	// If no exceptions apply to this role, fall through to standard evaluation.
	if len(ruleExempted) == 0 {
		return p.Evaluate(findings), nil
	}

	maxAction := ActionPass

	for _, f := range findings {
		if f.Type == "custom" {
			if a, ok := p.ActionForCustomCategory(f.Category); ok {
				if actionSeverity(a) > actionSeverity(maxAction) {
					maxAction = a
				}
			}
			continue
		}

		ruleKeys := []string{f.Type + "_detection", f.Type}
		for _, key := range ruleKeys {
			// Check if this rule is exempted for the user's role.
			if match, exempted := ruleExempted[strings.ToLower(key)]; exempted {
				appliedExceptions = append(appliedExceptions, match)
				continue
			}

			rule, ok := p.Rules[key]
			if !ok {
				continue
			}

			// Check if this specific category is covered by the rule's types
			if len(rule.Types) > 0 && !containsType(rule.Types, f.Category) {
				continue
			}

			if strings.EqualFold(strings.TrimSpace(rule.Mode), "confidence_based") {
				confScore := 0
				if f.ConfidenceScore != nil {
					confScore = f.ConfidenceScore.Total
				} else if f.Confidence > 0 {
					confScore = int(f.Confidence * 100)
				}
				if rule.MinimumConfidence > 0 && confScore < rule.MinimumConfidence {
					continue
				}
				nextAction := actionFromConfidenceFinding(f)
				if rule.OverrideAction != nil {
					nextAction = *rule.OverrideAction
				}
				if actionSeverity(nextAction) > actionSeverity(maxAction) {
					maxAction = nextAction
				}
				continue
			}

			// Check sensitivity threshold
			if !meetsThreshold(f.Severity, rule.Sensitivity) {
				continue
			}

			if actionSeverity(rule.Action) > actionSeverity(maxAction) {
				maxAction = rule.Action
			}
		}
	}

	// Deduplicate applied exceptions (same rule can be matched by multiple findings).
	seen := make(map[string]struct{})
	deduped := make([]ExceptionMatch, 0, len(appliedExceptions))
	for _, m := range appliedExceptions {
		if _, dup := seen[m.Rule]; dup {
			continue
		}
		seen[m.Rule] = struct{}{}
		deduped = append(deduped, m)
	}

	return maxAction, deduped
}

type Policy struct {
	Version        string            `yaml:"version" json:"version"`
	Name           string            `yaml:"name" json:"name"`
	Rules          map[string]Rule   `yaml:"rules" json:"rules"`
	OutputRules    *OutputRules      `yaml:"output_rules" json:"output_rules"`
	RateLimit      *RateLimit        `yaml:"rate_limit" json:"rate_limit"`
	Providers      *Providers        `yaml:"providers" json:"providers"`
	CustomEntities []CustomEntity    `yaml:"custom_entities" json:"custom_entities"`
	Competitors    []Competitor      `yaml:"competitors" json:"competitors"`
	Governance     *Governance       `yaml:"governance" json:"governance"`
	Data           *DataControl      `yaml:"data" json:"data"`
	Pricing        *Pricing          `yaml:"pricing" json:"pricing"`
	Cost           *CostControl      `yaml:"cost" json:"cost"`
	Cache          *CacheConfig      `yaml:"cache" json:"cache"`
	BodyLimits     *BodyLimitsConfig `yaml:"body_limits" json:"body_limits"`
	Exceptions     []Exception           `yaml:"exceptions" json:"exceptions"`
	OperatorState  *OperatorStateConfig  `yaml:"operator_state" json:"operator_state"`
}

// ResolveMaxBodyBytes returns the maximum incoming request body size in bytes for provider.
// configDefault is usually env TAMGA_MAX_BODY_BYTES; when <= 0 the implicit base is 1 MiB
// if policy does not define body_limits.
func (p *Policy) ResolveMaxBodyBytes(provider string, configDefault int) int {
	base := configDefault
	if base <= 0 {
		base = 1024 * 1024
	}
	if p == nil || p.BodyLimits == nil {
		return base
	}
	bl := p.BodyLimits
	if bl.PerProvider != nil {
		if r, ok := bl.PerProvider[provider]; ok && r.MaxBytes > 0 {
			return r.MaxBytes
		}
	}
	if bl.Default.MaxBytes > 0 {
		return bl.Default.MaxBytes
	}
	return base
}

// EvaluateOutput decides the action for response findings based on policy.output_rules.
// It mirrors Evaluate but keys off the output-specific lists.
func (p *Policy) EvaluateOutput(findings []scanner.Finding) Action {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled {
		return ActionPass
	}
	if len(findings) == 0 {
		return ActionPass
	}
	maxAction := ActionPass
	for _, f := range findings {
		confScore := 0
		if f.ConfidenceScore != nil {
			confScore = f.ConfidenceScore.Total
		} else if f.Confidence > 0 {
			confScore = int(f.Confidence * 100)
		}
		if p.OutputRules.MinimumConfidence > 0 && confScore < p.OutputRules.MinimumConfidence {
			continue
		}
		key := f.Type
		if f.Category != "" {
			key = f.Category
		}
		if containsOutputKey(p.OutputRules.BlockOn, f.Type, f.Category) {
			if actionSeverity(ActionBlock) > actionSeverity(maxAction) {
				maxAction = ActionBlock
			}
			continue
		}
		if containsOutputKey(p.OutputRules.RedactOn, f.Type, f.Category) {
			if actionSeverity(ActionRedact) > actionSeverity(maxAction) {
				maxAction = ActionRedact
			}
			continue
		}
		_ = key
	}
	return maxAction
}

func containsOutputKey(list []string, typ, cat string) bool {
	for _, k := range list {
		if strings.EqualFold(k, typ) || strings.EqualFold(k, cat) {
			return true
		}
	}
	return false
}

// ParseAction normalizes a YAML action string to Action (unknown → PASS).
func ParseAction(s string) Action {
	a := Action(strings.ToUpper(strings.TrimSpace(s)))
	switch a {
	case ActionBlock, ActionRedact, ActionWarn, ActionLog, ActionPass:
		return a
	default:
		return ActionPass
	}
}

// CustomEntityByName returns a pointer to the matching custom entity, if any.
func (p *Policy) CustomEntityByName(name string) *CustomEntity {
	if p == nil {
		return nil
	}
	for i := range p.CustomEntities {
		if strings.EqualFold(p.CustomEntities[i].Name, name) {
			return &p.CustomEntities[i]
		}
	}
	return nil
}

// ActionForCustomCategory returns the configured action for a custom finding category (entity name).
func (p *Policy) ActionForCustomCategory(category string) (Action, bool) {
	if ce := p.CustomEntityByName(category); ce != nil {
		return ParseAction(ce.Action), true
	}
	return ActionPass, false
}

// LoadFromFile reads and parses a YAML policy file.
func LoadFromFile(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy file: %w", err)
	}

	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	if p.Version == "" {
		p.Version = "1.0"
	}

	return &p, nil
}

// LoadFromBytes parses policy YAML from memory (tests and embedded configs).
func LoadFromBytes(data []byte) (*Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	if p.Version == "" {
		p.Version = "1.0"
	}

	return &p, nil
}

// OperatorStateConfig is the top-level operator_state block in the policy YAML.
// It defines assertions that check jugeni-decision states before LLM calls.
type OperatorStateConfig struct {
	Enabled    bool                     `yaml:"enabled" json:"enabled"`
	Assertions []OperatorStateAssertion `yaml:"assertions" json:"assertions"`
}

// OperatorStateAssertion is a single operator-state assertion rule.
type OperatorStateAssertion struct {
	DecisionPattern string `yaml:"decision_pattern" json:"decision_pattern"`
	RequiredState   string `yaml:"required_state" json:"required_state"`
	ActionOnFail    string `yaml:"action_on_fail" json:"action_on_fail"`
	Severity        string `yaml:"severity" json:"severity"`
	Description     string `yaml:"description" json:"description"`
}

// PolicyStore holds the active policy with atomic swap on reload (thread-safe reads).
type PolicyStore struct {
	p atomic.Pointer[Policy]
}

// NewPolicyStore wraps an initial policy (may be nil; callers should load before serving).
func NewPolicyStore(initial *Policy) *PolicyStore {
	s := &PolicyStore{}
	if initial != nil {
		s.p.Store(initial)
	}
	return s
}

// GetPolicy returns the current policy. Safe for concurrent use with Reload / watcher.
func (s *PolicyStore) GetPolicy() *Policy {
	if s == nil {
		return nil
	}
	return s.p.Load()
}

// Reload reads and parses path, then atomically replaces the active policy on success.
// On error the previous policy is unchanged.
func (s *PolicyStore) Reload(path string) error {
	if s == nil {
		return fmt.Errorf("policy store is nil")
	}
	newP, err := LoadFromFile(path)
	if err != nil {
		return err
	}
	s.p.Store(newP)
	return nil
}

// Evaluate determines the action to take for a set of findings.
// When multiple rules apply, the highest-severity action wins:
// BLOCK > REDACT > WARN > LOG > PASS.
func (p *Policy) Evaluate(findings []scanner.Finding) Action {
	if len(findings) == 0 {
		return ActionPass
	}

	maxAction := ActionPass

	for _, f := range findings {
		if f.Type == "custom" {
			if a, ok := p.ActionForCustomCategory(f.Category); ok {
				if actionSeverity(a) > actionSeverity(maxAction) {
					maxAction = a
				}
			}
			continue
		}

		rule, ok := p.Rules[f.Type+"_detection"]
		if !ok {
			// Try without _detection suffix
			rule, ok = p.Rules[f.Type]
		}
		if !ok {
			continue
		}

		// Check if this specific category is covered by the rule's types
		if len(rule.Types) > 0 && !containsType(rule.Types, f.Category) {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(rule.Mode), "confidence_based") {
			confScore := 0
			if f.ConfidenceScore != nil {
				confScore = f.ConfidenceScore.Total
			} else if f.Confidence > 0 {
				confScore = int(f.Confidence * 100)
			}
			if rule.MinimumConfidence > 0 && confScore < rule.MinimumConfidence {
				continue
			}
			nextAction := actionFromConfidenceFinding(f)
			if rule.OverrideAction != nil {
				nextAction = *rule.OverrideAction
			}
			if actionSeverity(nextAction) > actionSeverity(maxAction) {
				maxAction = nextAction
			}
			continue
		}

		// Check sensitivity threshold
		if !meetsThreshold(f.Severity, rule.Sensitivity) {
			continue
		}

		if actionSeverity(rule.Action) > actionSeverity(maxAction) {
			maxAction = rule.Action
		}
	}

	return maxAction
}

func actionFromConfidenceFinding(f scanner.Finding) Action {
	if f.ConfidenceScore != nil {
		switch strings.ToUpper(strings.TrimSpace(f.ConfidenceScore.Action)) {
		case scanner.ActionBlock:
			return ActionBlock
		case scanner.ActionRedact:
			return ActionRedact
		case scanner.ActionPassLog:
			return ActionLog
		default:
			return ActionPass
		}
	}
	switch {
	case f.Confidence >= 0.90:
		return ActionBlock
	case f.Confidence >= 0.70:
		return ActionRedact
	case f.Confidence >= 0.40:
		return ActionLog
	default:
		return ActionPass
	}
}

func containsType(types []string, category string) bool {
	for _, t := range types {
		if strings.EqualFold(t, category) {
			return true
		}
	}
	return false
}

func meetsThreshold(findingSeverity, ruleSensitivity string) bool {
	severityMap := map[string]int{
		"low": 1, "medium": 2, "high": 3, "critical": 4,
	}
	sensitivityThreshold := map[string]int{
		"low": 1, "medium": 2, "high": 3,
	}

	fSev := severityMap[findingSeverity]
	threshold := sensitivityThreshold[ruleSensitivity]

	if threshold == 0 {
		threshold = 2 // default: medium
	}

	return fSev >= threshold
}

func actionSeverity(a Action) int {
	switch a {
	case ActionBlock:
		return 4
	case ActionRedact:
		return 3
	case ActionWarn:
		return 2
	case ActionLog:
		return 1
	default:
		return 0
	}
}

// MatchedRule returns the policy rule that applies to a finding, if any.
// Lookup order matches Evaluate: "<type>_detection" then "<type>".
func (p *Policy) MatchedRule(f scanner.Finding) (Rule, bool) {
	if f.Type == "custom" {
		if ce := p.CustomEntityByName(f.Category); ce != nil {
			return Rule{
				Action:      ParseAction(ce.Action),
				Sensitivity: "low",
			}, true
		}
		return Rule{}, false
	}

	keys := []string{f.Type + "_detection", f.Type}
	for _, key := range keys {
		rule, ok := p.Rules[key]
		if !ok {
			continue
		}
		if len(rule.Types) > 0 && !containsType(rule.Types, f.Category) {
			continue
		}
		if !meetsThreshold(f.Severity, rule.Sensitivity) {
			continue
		}
		return rule, true
	}
	return Rule{}, false
}

// WebhookURLsForFindings returns unique notify webhook URLs from rules that match the given findings
// and whose configured rule action equals wantRuleAction (e.g. only WARN rules when notifying on WARN).
func (p *Policy) WebhookURLsForFindings(findings []scanner.Finding, wantRuleAction Action) []string {
	var urls []string
	seen := map[string]struct{}{}
	for _, f := range findings {
		rule, ok := p.MatchedRule(f)
		if !ok || rule.Action != wantRuleAction {
			continue
		}
		for _, n := range rule.Notify {
			if n.Webhook == "" {
				continue
			}
			if _, dup := seen[n.Webhook]; dup {
				continue
			}
			seen[n.Webhook] = struct{}{}
			urls = append(urls, n.Webhook)
		}
	}
	return urls
}

// ProviderAllowed returns false if the provider is blocked or not on the allowlist (when allowlist is non-empty).
func (p *Policy) ProviderAllowed(provider string) bool {
	if p.Providers == nil {
		return true
	}
	for _, b := range p.Providers.Blocked {
		if strings.EqualFold(b, provider) {
			return false
		}
	}
	if len(p.Providers.Allowed) == 0 {
		return true
	}
	for _, a := range p.Providers.Allowed {
		if strings.EqualFold(a, provider) {
			return true
		}
	}
	return false
}
