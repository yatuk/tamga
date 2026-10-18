package policy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	// ActionRoute sends the request, as written, to a pool of model servers
	// the organisation runs itself, and not to the provider the caller
	// addressed. The caller gets an answer and the text does not leave. See
	// route.go.
	ActionRoute  Action = "ROUTE"
	// ActionStrip replaces the whole piece of text a finding is in with a
	// placeholder and forwards the request. It is what an agent session
	// needs when a tool result carries an injection: the model never sees
	// the text, and the session goes on.
	ActionStrip  Action = "STRIP"
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
	// AppliesTo limits the rule to findings in text of these message roles
	// (system, user, assistant, tool, tool_definition, request). Empty means
	// every role. See MessageRoles for what each is.
	//
	// The role is what the request says it is. Whoever writes the body can
	// label any text "system", so leaving a role out of a rule trusts the
	// caller not to do that: safe when your own server builds the request,
	// not when end users reach the proxy directly.
	AppliesTo []string `yaml:"applies_to" json:"applies_to,omitempty"`
	// RouteTo names the internal pool (providers.pools) a rule with the
	// action ROUTE sends the request to. See route.go.
	RouteTo string `yaml:"route_to,omitempty" json:"route_to,omitempty"`
}

// MessageRoles are the roles a rule's applies_to may name.
var MessageRoles = map[string]string{
	"system":          "the system prompt",
	"user":            "user messages",
	"assistant":       "assistant messages, including the arguments of tool calls",
	"tool":            "tool results, attached documents and search results",
	"tool_definition": "the descriptions of the tools offered to the model",
	"request":         "request parameters outside the conversation, such as metadata",
}

// appliesToRole reports whether the rule covers text of the given role. A
// finding without a role comes from a raw scan, where the role is unknown;
// every rule covers it, so an unrecognised request shape cannot be used to
// step around a role-limited rule.
func (r Rule) appliesToRole(role string) bool {
	if len(r.AppliesTo) == 0 || role == "" {
		return true
	}
	for _, a := range r.AppliesTo {
		if strings.EqualFold(strings.TrimSpace(a), role) {
			return true
		}
	}
	return false
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
	// The four below describe an internal pool, the target of the ROUTE
	// action (route.go). They are left out of the JSON form when unset, so a
	// policy without them keeps its digest.
	//
	// Internal marks the pool as model servers the organisation runs itself.
	// Such a pool is named freely, not after a route, and is used only by
	// ROUTE; ROUTE accepts no other kind.
	Internal bool `yaml:"internal,omitempty" json:"internal,omitempty"`
	// Dialect is the API the pool's servers speak: "openai" or "anthropic".
	Dialect string `yaml:"dialect,omitempty" json:"dialect,omitempty"`
	// Model, when set, replaces the model name of a routed request.
	Model string `yaml:"model,omitempty" json:"model,omitempty"`
	// AllowPrivateAddresses lets the pool connect to a private or loopback
	// address. The address must still be one the operator listed in
	// TAMGA_UPSTREAM_ALLOW; this says the policy means to go there.
	AllowPrivateAddresses bool `yaml:"allow_private_addresses,omitempty" json:"allow_private_addresses,omitempty"`
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
	// RouteTo names the internal pool for the action ROUTE (route.go).
	RouteTo string `yaml:"route_to,omitempty" json:"route_to,omitempty"`
}

// Competitor defines a competitor brand or product to detect in LLM prompts.
// When a user mentions a competitor in their prompt, the competitor scanner
// fires a finding so the SOC team can track competitive intelligence signals.
type Competitor struct {
	Name        string   `yaml:"name" json:"name"`               // e.g. "Lakera Guard"
	Patterns    []string `yaml:"patterns" json:"patterns"`       // regex patterns to match (case-insensitive)
	Severity    string   `yaml:"severity" json:"severity"`       // "low" | "medium" | "high"
	Action      string   `yaml:"action" json:"action"`           // BLOCK, ROUTE, STRIP, REDACT, WARN, LOG or PASS; see entry_action.go
	Description string   `yaml:"description" json:"description"` // human-readable note
	Enabled     bool     `yaml:"enabled" json:"enabled"`         // per-competitor toggle
	// RouteTo names the internal pool for the action ROUTE (route.go).
	RouteTo string `yaml:"route_to,omitempty" json:"route_to,omitempty"`
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
	// Links and JSONSchema are described in output_content.go. They are
	// left out of the JSON form when unset, so that a policy without them
	// keeps the digest, and with it the cache key, it had before they existed.
	Links      *OutputLinks      `yaml:"links" json:"links,omitempty"`
	JSONSchema *OutputJSONSchema `yaml:"json_schema" json:"json_schema,omitempty"`
	// ToolDestinations are the places tools may send data to, by tool
	// (tool_destinations.go). Left out of the JSON form when unset, for the
	// same reason.
	ToolDestinations []ToolDestination `yaml:"tool_destinations" json:"tool_destinations,omitempty"`
	// PromptLeak looks for the system prompt, or the operator's canaries,
	// in answers (prompt_leak.go). Left out of the JSON form when unset.
	PromptLeak *OutputPromptLeak `yaml:"prompt_leak" json:"prompt_leak,omitempty"`
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
		// A custom entity's or a competitor's own action; the rules below
		// apply to it as well, and the stronger wins (entry_action.go).
		if a, ok := p.entryAction(f); ok && actionSeverity(a) > actionSeverity(maxAction) {
			maxAction = a
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
			if a, ok := evalRuleAction(rule, f); ok && actionSeverity(a) > actionSeverity(maxAction) {
				maxAction = a
			}
		}
		if a, ok := p.unruledAction(f); ok && actionSeverity(a) > actionSeverity(maxAction) {
			maxAction = a
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
	Version        string               `yaml:"version" json:"version"`
	Name           string               `yaml:"name" json:"name"`
	Rules          map[string]Rule      `yaml:"rules" json:"rules"`
	OutputRules    *OutputRules         `yaml:"output_rules" json:"output_rules"`
	RateLimit      *RateLimit           `yaml:"rate_limit" json:"rate_limit"`
	Providers      *Providers           `yaml:"providers" json:"providers"`
	CustomEntities []CustomEntity       `yaml:"custom_entities" json:"custom_entities"`
	Competitors    []Competitor         `yaml:"competitors" json:"competitors"`
	Governance     *Governance          `yaml:"governance" json:"governance"`
	Data           *DataControl         `yaml:"data" json:"data"`
	Cost           *CostControl         `yaml:"cost" json:"cost"`
	Cache          *CacheConfig         `yaml:"cache" json:"cache"`
	BodyLimits     *BodyLimitsConfig    `yaml:"body_limits" json:"body_limits"`
	Exceptions     []Exception          `yaml:"exceptions" json:"exceptions"`
	OperatorState  *OperatorStateConfig `yaml:"operator_state" json:"operator_state"`
	Vault          *VaultConfig         `yaml:"vault" json:"vault"`
	Canary         *CanaryConfig        `yaml:"canary" json:"canary"`
	Scan           *ScanConfig          `yaml:"scan" json:"scan"`
	// Language is the languages a request may be written in (language.go).
	// It is left out of the JSON form when absent, so that a policy without
	// it keeps its digest.
	Language *LanguageConfig `yaml:"language" json:"language,omitempty"`
	// Topics is left out of the JSON form when unset, so that a policy
	// without it keeps its digest (see OutputRules.Links).
	Topics *TopicsConfig `yaml:"topics" json:"topics,omitempty"`
	// ProviderAccounts are the provider accounts a request may use
	// (provider_accounts.go). Left out of the JSON form when unset, for the
	// same reason.
	ProviderAccounts *ProviderAccountsConfig `yaml:"provider_accounts" json:"provider_accounts,omitempty"`
	// Agents is described in agents.go. Left out of the JSON form when
	// unset, for the same reason.
	Agents *AgentsConfig `yaml:"agents" json:"agents,omitempty"`
	// Attachments is described in attachments.go. Left out of the JSON
	// form when unset, for the same reason.
	Attachments *AttachmentsConfig `yaml:"attachments" json:"attachments,omitempty"`
}

// Values of scan.on_malformed.
const (
	// MalformedBlock answers 400 to a body that parsers can read differently.
	MalformedBlock = "block"
	// MalformedRawScan scans such a body as plain bytes and forwards it.
	MalformedRawScan = "raw_scan"
)

// ScanConfig holds settings for the scan stage as a whole, as opposed to the
// per-finding rules under "rules".
type ScanConfig struct {
	// OnMalformed decides what happens to a JSON body that is ambiguous:
	// duplicate keys, invalid UTF-8, or not JSON at all. Empty means block.
	OnMalformed string `yaml:"on_malformed" json:"on_malformed"`
	// OnError decides what happens when the scan itself fails: a scanner
	// errors or panics, the scan times out, or a scanner is shed under load.
	// "block" answers 503; "pass" forwards the request on the findings of the
	// scanners that did run, except when those findings call for masking or
	// stripping: text is not sent masked in part. Empty means pass for a
	// plain chat request and block for one that carries tool results or tool
	// definitions (BlockOnScanError).
	OnError string `yaml:"on_error" json:"on_error"`
	// DefaultAction is the action for a finding whose type no rule is
	// written for (BLOCK, WARN, LOG or PASS). Empty means PASS. It does not
	// touch findings a rule deliberately leaves alone — below its
	// sensitivity, outside its types or its roles.
	DefaultAction string `yaml:"default_action" json:"default_action"`
	// Classifier configures the inline prompt-injection classifier.
	Classifier *ClassifierConfig `yaml:"classifier" json:"classifier,omitempty"`
	// StripScope is how much of a piece of text STRIP takes out: "text"
	// (the default) replaces the whole piece, "field" replaces, in a tool
	// result that is JSON, only the strings that hold the instruction
	// (StripFields). It is left out of the JSON form when unset, so that a
	// policy without it keeps its digest.
	StripScope string `yaml:"strip_scope" json:"strip_scope,omitempty"`
}

// Values of scan.strip_scope.
const (
	StripScopeText  = "text"
	StripScopeField = "field"
)

// StripFields reports whether STRIP takes out only the strings of a JSON
// tool result that hold what it strips, and leaves the rest of the tool
// result as it was. Off unless scan.strip_scope is "field".
func (p *Policy) StripFields() bool {
	return p != nil && p.Scan != nil && strings.EqualFold(strings.TrimSpace(p.Scan.StripScope), StripScopeField)
}

// Values of scan.on_error.
const (
	ScanErrorBlock = "block"
	ScanErrorPass  = "pass"
)

// BlockOnScanError reports whether a failed scan must stop the request.
// carriesTools says the request holds tool results or tool definitions
// (agent traffic). Where on_error is not set, such a request fails closed and
// a plain chat request fails open, as before the setting existed: a crashed
// check must not let an injected tool result through. Setting on_error to
// pass or block applies to both.
func (p *Policy) BlockOnScanError(carriesTools bool) bool {
	if p == nil {
		return false
	}
	if p.Scan == nil || strings.TrimSpace(p.Scan.OnError) == "" {
		return carriesTools
	}
	return strings.EqualFold(strings.TrimSpace(p.Scan.OnError), ScanErrorBlock)
}

// unruledAction returns scan.default_action for a finding whose type has no
// rule at all, and false otherwise. A scanner can be registered — content
// moderation, competitor mentions — without the policy saying what to do with
// what it finds; this is the answer for those.
func (p *Policy) unruledAction(f scanner.Finding) (Action, bool) {
	if p == nil || p.Scan == nil || strings.TrimSpace(p.Scan.DefaultAction) == "" || f.Type == "custom" {
		return ActionPass, false
	}
	// A signal is acted on only by a rule that names it (evalRuleAction).
	if scanner.Signal(f) {
		return ActionPass, false
	}
	for _, key := range []string{f.Type + "_detection", f.Type} {
		if _, ok := p.Rules[key]; ok {
			return ActionPass, false
		}
	}
	// A competitor entry with an action of its own has been decided about.
	if _, ok := p.entryAction(f); ok {
		return ActionPass, false
	}
	return ParseAction(p.Scan.DefaultAction), true
}

// validate checks the values of the scan section. A misspelled value would
// otherwise fall back to a default nobody chose.
func (c *ScanConfig) validate() error {
	if c == nil {
		return nil
	}
	switch c.OnMalformed {
	case "", MalformedBlock, MalformedRawScan:
	default:
		return fmt.Errorf("scan.on_malformed: unknown value %q (block or raw_scan)", c.OnMalformed)
	}
	switch strings.ToLower(strings.TrimSpace(c.OnError)) {
	case "", ScanErrorBlock, ScanErrorPass:
	default:
		return fmt.Errorf("scan.on_error: unknown value %q (block or pass)", c.OnError)
	}
	switch Action(strings.ToUpper(strings.TrimSpace(c.DefaultAction))) {
	case "", ActionBlock, ActionWarn, ActionLog, ActionPass:
	default:
		return fmt.Errorf("scan.default_action: unknown value %q (BLOCK, WARN, LOG or PASS)", c.DefaultAction)
	}
	switch strings.ToLower(strings.TrimSpace(c.StripScope)) {
	case "", StripScopeText, StripScopeField:
	default:
		return fmt.Errorf("scan.strip_scope: unknown value %q (text or field)", c.StripScope)
	}
	return c.Classifier.validate()
}

// OnMalformedJSON returns the action for an ambiguous JSON body. It blocks
// unless the policy asks for raw_scan: a body the proxy and the provider can
// read differently is one the proxy cannot vouch for.
func (p *Policy) OnMalformedJSON() string {
	if p != nil && p.Scan != nil && p.Scan.OnMalformed == MalformedRawScan {
		return MalformedRawScan
	}
	return MalformedBlock
}

// VaultConfig enables reversible PII tokenization. When enabled, PII that would
// be REDACTed is instead replaced with numbered placeholders on the way to the
// provider and restored to the original values in the response (see
// internal/vault). The at-rest encryption key comes from TAMGA_VAULT_KEY and
// the store TTL from TAMGA_VAULT_TTL_SECONDS (proxy config), so this toggle is
// just the hot-reloadable on/off switch.
type VaultConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// CanaryConfig enables system-prompt-leak detection via canary tokens. When
// enabled, a unique token is injected into the outgoing system prompt; if it
// appears in the response, a system_prompt_leak finding is raised (and the
// response blocked when BlockOnLeak is set). Providers limits the feature to
// specific providers (empty = openai + anthropic).
type CanaryConfig struct {
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	BlockOnLeak bool     `yaml:"block_on_leak" json:"block_on_leak"`
	Providers   []string `yaml:"providers" json:"providers"`
}

// CanaryAppliesTo reports whether the canary should run for the given provider.
func (c *CanaryConfig) CanaryAppliesTo(provider string) bool {
	if c == nil || !c.Enabled {
		return false
	}
	if len(c.Providers) == 0 {
		return provider == "openai" || provider == "anthropic"
	}
	for _, p := range c.Providers {
		if strings.EqualFold(strings.TrimSpace(p), provider) {
			return true
		}
	}
	return false
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
		if containsOutputKey(p.OutputRules.BlockOn, outputType(f), f.Category) {
			if actionSeverity(ActionBlock) > actionSeverity(maxAction) {
				maxAction = ActionBlock
			}
			continue
		}
		if containsOutputKey(p.OutputRules.RedactOn, outputType(f), f.Category) {
			if actionSeverity(ActionRedact) > actionSeverity(maxAction) {
				maxAction = ActionRedact
			}
			continue
		}
		_ = key
	}
	return maxAction
}

// OutputRedacts reports whether output_rules asks for this finding to be
// redacted from a response: it is named in redact_on, not in block_on, and
// clears the minimum confidence.
func (p *Policy) OutputRedacts(f scanner.Finding) bool {
	if p == nil || p.OutputRules == nil || !p.OutputRules.Enabled {
		return false
	}
	confScore := 0
	if f.ConfidenceScore != nil {
		confScore = f.ConfidenceScore.Total
	} else if f.Confidence > 0 {
		confScore = int(f.Confidence * 100)
	}
	if p.OutputRules.MinimumConfidence > 0 && confScore < p.OutputRules.MinimumConfidence {
		return false
	}
	return !containsOutputKey(p.OutputRules.BlockOn, outputType(f), f.Category) &&
		containsOutputKey(p.OutputRules.RedactOn, outputType(f), f.Category)
}

// outputType is the type a finding is matched by in block_on and redact_on.
// A signal has none: as with the rules for requests, it is acted on only
// when its category is named ("unreadable_block"), not because the list
// names "injection".
func outputType(f scanner.Finding) string {
	if scanner.Signal(f) {
		return ""
	}
	return f.Type
}

func containsOutputKey(list []string, typ, cat string) bool {
	for _, k := range list {
		if k == "" {
			continue
		}
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
	case ActionBlock, ActionRoute, ActionStrip, ActionRedact, ActionWarn, ActionLog, ActionPass:
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

	return LoadFromBytes(data)
}

// LoadFromBytes parses policy YAML from memory (tests and embedded configs).
//
// Parsing is strict: a key that does not map to a known policy field is an
// error. A mis-indented block (e.g. rules nested under an unrelated section)
// would otherwise be dropped silently and the policy would load without the
// enforcement the operator wrote down.
func LoadFromBytes(data []byte) (*Policy, error) {
	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// io.EOF means an empty document, which yaml.Unmarshal also accepted.
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		// The section was removed with the plans it described. It is refused
		// like any other unknown key, with the reason, so that a request cap
		// somebody relied on does not stop applying without a word.
		if strings.Contains(err.Error(), "field pricing not found") {
			return nil, fmt.Errorf("parsing policy YAML: the \"pricing\" section is no longer part of the policy: Tamga has no plans or tiers. Remove it; request and spend limits are set under rate_limit and cost: %w", err)
		}
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}
	// Only the first document is the policy. A second one would be dropped
	// without a word, and with it whatever the operator wrote there.
	for {
		var extra yaml.Node
		err := dec.Decode(&extra)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing policy YAML: %w", err)
		}
		if !emptyDocument(&extra) {
			return nil, fmt.Errorf("parsing policy YAML: the file holds more than one YAML document (a second \"---\"); a policy is one document")
		}
	}

	if p.Version == "" {
		p.Version = "1.0"
	}

	if err := p.Scan.validate(); err != nil {
		return nil, fmt.Errorf("parsing policy YAML: %w", err)
	}

	// A misspelled role would make the rule cover nothing, silently.
	for name, rule := range p.Rules {
		for _, role := range rule.AppliesTo {
			if _, ok := MessageRoles[strings.ToLower(strings.TrimSpace(role))]; !ok {
				return nil, fmt.Errorf("parsing policy YAML: rules.%s.applies_to: unknown role %q (known: system, user, assistant, tool, tool_definition, request)", name, role)
			}
		}
	}

	// A pool endpoint decides where callers' traffic and a key from the
	// proxy's environment are sent. One the rules refuse fails the load, so
	// no way of installing a policy can bring it in.
	//
	// A value outside its bounds fails the load for the same reason: a
	// pattern that costs seconds per request, or a number that turns a
	// control off, must not get in through any door. See bounds.go.
	//
	// Both come back as an IssuesError, one issue per field.
	if issues := loadBlockingIssues(&p); len(issues) > 0 {
		return nil, &IssuesError{Issues: issues}
	}

	return &p, nil
}

// emptyDocument reports whether a YAML document holds nothing: what a
// trailing "---" or a comment after one leaves behind.
func emptyDocument(n *yaml.Node) bool {
	if n.Kind == 0 {
		return true
	}
	if n.Kind == yaml.DocumentNode {
		return len(n.Content) == 0 || (len(n.Content) == 1 && emptyDocument(n.Content[0]))
	}
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
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
	LogCoverageGaps(newP)
	return nil
}

// Evaluate determines the action to take for a set of findings.
// When multiple rules apply, the highest-severity action wins:
// BLOCK > ROUTE > STRIP > REDACT > WARN > LOG > PASS.
func (p *Policy) Evaluate(findings []scanner.Finding) Action {
	if len(findings) == 0 {
		return ActionPass
	}

	maxAction := ActionPass

	for _, f := range findings {
		// A custom entity's or a competitor's own action; the rules below
		// apply to it as well, and the stronger wins (entry_action.go).
		if a, ok := p.entryAction(f); ok && actionSeverity(a) > actionSeverity(maxAction) {
			maxAction = a
		}

		// A finding of type "pii" is scored against both the "pii_detection"
		// and bare "pii" rule keys when both exist, and the strongest action
		// wins. This lets one rule REDACT contact PII (email, phone) while a
		// second BLOCKs critical identifiers (TC Kimlik, credit cards) by
		// category, without over-blocking every finding of the type.
		for _, key := range []string{f.Type + "_detection", f.Type} {
			rule, ok := p.Rules[key]
			if !ok {
				continue
			}
			if a, ok := evalRuleAction(rule, f); ok && actionSeverity(a) > actionSeverity(maxAction) {
				maxAction = a
			}
		}
		if a, ok := p.unruledAction(f); ok && actionSeverity(a) > actionSeverity(maxAction) {
			maxAction = a
		}
	}

	return maxAction
}

// evalRuleAction returns the action a single rule assigns to a finding, and
// whether the rule applies at all (category + sensitivity/confidence gates).
func evalRuleAction(rule Rule, f scanner.Finding) (Action, bool) {
	// Category filter: an empty Types list matches every category.
	if len(rule.Types) > 0 && !containsType(rule.Types, f.Category) {
		return ActionPass, false
	}
	if !rule.appliesToRole(f.Role) {
		return ActionPass, false
	}
	// A signal (an unreadable block, a run of invisible characters) says a
	// text is unusual, not that it is an attack. A rule written for
	// attacks does not mean it, whatever its sensitivity: only a rule that
	// names the category in its types acts on it, and then at any
	// sensitivity, since naming it is the decision.
	signal := scanner.Signal(f)
	if signal && len(rule.Types) == 0 {
		return ActionPass, false
	}

	if strings.EqualFold(strings.TrimSpace(rule.Mode), "confidence_based") {
		confScore := 0
		if f.ConfidenceScore != nil {
			confScore = f.ConfidenceScore.Total
		} else if f.Confidence > 0 {
			confScore = int(f.Confidence * 100)
		}
		if rule.MinimumConfidence > 0 && confScore < rule.MinimumConfidence {
			return ActionPass, false
		}
		if rule.OverrideAction != nil {
			return *rule.OverrideAction, true
		}
		return actionFromConfidenceFinding(f), true
	}

	if !signal && !meetsThreshold(f.Severity, rule.Sensitivity) {
		return ActionPass, false
	}
	return rule.Action, true
}

func actionFromConfidenceFinding(f scanner.Finding) Action {
	if f.ConfidenceScore != nil {
		switch strings.ToUpper(strings.TrimSpace(f.ConfidenceScore.Action)) {
		case scanner.ActionBlock:
			return ActionBlock
		case scanner.ActionRedact:
			return ActionRedact
		case scanner.ActionWarn:
			return ActionWarn
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
		return 6
	case ActionRoute:
		return 5
	case ActionStrip:
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
	keys := []string{f.Type + "_detection", f.Type}
	for _, key := range keys {
		rule, ok := p.Rules[key]
		if !ok {
			continue
		}
		if len(rule.Types) > 0 && !containsType(rule.Types, f.Category) {
			continue
		}
		if !rule.appliesToRole(f.Role) {
			continue
		}
		if scanner.Signal(f) {
			// As in evalRuleAction: only a rule that names it.
			if len(rule.Types) == 0 {
				continue
			}
		} else if !meetsThreshold(f.Severity, rule.Sensitivity) {
			continue
		}
		return rule, true
	}
	// No rule covers it: a custom entity or a competitor may still set an
	// action of its own.
	if a, ok := p.entryAction(f); ok {
		return Rule{Action: a, Sensitivity: "low"}, true
	}
	if f.Type == "custom" && p.CustomEntityByName(f.Category) != nil {
		// An entity of the policy without an action: known, and let through.
		return Rule{Action: ActionPass, Sensitivity: "low"}, true
	}
	if a, ok := p.unruledAction(f); ok {
		return Rule{Action: a, Sensitivity: "low"}, true
	}
	return Rule{}, false
}

// NotifyURLsForFindings returns the unique notify webhook URLs of the rules
// that match the given findings, whatever each rule's action: a rule that
// blocks or redacts is as entitled to tell someone as one that warns.
func (p *Policy) NotifyURLsForFindings(findings []scanner.Finding) []string {
	var urls []string
	seen := map[string]struct{}{}
	for _, f := range findings {
		rule, ok := p.MatchedRule(f)
		if !ok {
			continue
		}
		for _, n := range rule.Notify {
			u := notifyURL(n.Webhook)
			if u == "" {
				continue
			}
			if _, dup := seen[u]; dup {
				continue
			}
			seen[u] = struct{}{}
			urls = append(urls, u)
		}
	}
	return urls
}

// notifyEnv reads the environment for notifyURL; tests replace it.
var notifyEnv = os.LookupEnv

// notifyURL resolves a rule's webhook value. A webhook URL is a credential,
// so the shipped policy names an environment variable, "${SLACK_WEBHOOK_URL}",
// in place of the URL. Only a value that is exactly one such reference is
// looked up: nothing is substituted inside a longer string, so a policy
// cannot be used to build a URL out of other variables. An unset variable
// gives no URL, and so does a name the rules do not let a policy read (see
// UpstreamRules.NotifyEnvAllowed).
func notifyURL(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "${") || !strings.HasSuffix(v, "}") {
		return v
	}
	name := v[2 : len(v)-1]
	if name == "" || strings.ContainsAny(name, "${} ") {
		return ""
	}
	// Any name at all would let a policy post to whatever URL some other
	// variable holds, and tell an outsider which variables hold one.
	if !ActiveUpstreamRules().NotifyEnvAllowed(name) {
		return ""
	}
	val, _ := notifyEnv(name)
	return strings.TrimSpace(val)
}

// ProviderAllowed returns false if the provider is blocked or not on the allowlist (when allowlist is non-empty).
func (p *Policy) ProviderAllowed(provider string) bool {
	if p.Providers == nil {
		return true
	}
	want := CanonicalProvider(provider)
	for _, b := range p.Providers.Blocked {
		if CanonicalProvider(b) == want {
			return false
		}
	}
	if len(p.Providers.Allowed) == 0 {
		return true
	}
	for _, a := range p.Providers.Allowed {
		if CanonicalProvider(a) == want {
			return true
		}
	}
	return false
}

// providerAliases maps the vendor-style names used in older policies and docs
// to the route names the proxy actually dispatches on (see proxy.RegisterRoutes).
var providerAliases = map[string]string{
	"azure_openai":  "azure",
	"azure-openai":  "azure",
	"google_vertex": "gemini",
	"google-vertex": "gemini",
	"vertex":        "gemini",
	"google":        "gemini",
	"aws_bedrock":   "bedrock",
	"aws-bedrock":   "bedrock",
	"ollama":        "local",
}

// CanonicalProvider normalizes a provider name from policy YAML or a route to
// the proxy's route name, so "azure_openai" in an allowlist matches the
// "/azure/" route.
func CanonicalProvider(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if canon, ok := providerAliases[n]; ok {
		return canon
	}
	return n
}

// FindingAction returns the action the rules assign to one finding, for a
// caller with the given role. It is the per-finding view of what
// EvaluateWithRole reduces to a single verdict: STRIP and REDACT act on
// particular findings, so the caller needs to know which.
func (p *Policy) FindingAction(f scanner.Finding, role string, strictMode bool) Action {
	if p == nil {
		return ActionPass
	}
	exempt := func(key string) bool {
		if strictMode || role == "" {
			return false
		}
		for _, exc := range p.Exceptions {
			if !strings.EqualFold(exc.Rule, key) || isExceptionExpired(exc.ExpiresAt) {
				continue
			}
			for _, r := range exc.Roles {
				if strings.EqualFold(strings.TrimSpace(r), strings.TrimSpace(role)) {
					return true
				}
			}
		}
		return false
	}
	// An entry's own action first; an exception waives rules, not that.
	max, _ := p.entryAction(f)
	for _, key := range []string{f.Type + "_detection", f.Type} {
		rule, ok := p.Rules[key]
		if !ok || exempt(key) {
			continue
		}
		if a, ok := evalRuleAction(rule, f); ok && actionSeverity(a) > actionSeverity(max) {
			max = a
		}
	}
	if a, ok := p.unruledAction(f); ok && actionSeverity(a) > actionSeverity(max) {
		max = a
	}
	return max
}
