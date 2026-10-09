package policy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yatuk/tamga/internal/cache"
	"github.com/yatuk/tamga/internal/scanner"
)

// Bounds on what a policy may set. The analyst role can write the policy, so
// a value here is a value an analyst chose: one that turns a control off
// without saying so, or makes the proxy hold a request in memory without
// limit, does not load. The limits on regular expressions are in
// scanner/pattern_limits.go.
const (
	// MinBodyLimitBytes is the smallest body limit that still lets a chat
	// request through; below it nearly every request is refused with a 413.
	// That is an outage the operator sees at once, not a control switched
	// off, so it is a warning and the policy loads.
	MinBodyLimitBytes = 1024
	// MaxBodyLimitBytes is the largest body limit. A request body is read
	// into memory whole and copied while it is scanned. 64 MiB is twice the
	// 32 MB the shipped policy sets for Anthropic, the largest any provider
	// accepts.
	MaxBodyLimitBytes = 64 << 20

	// MinOutputBufferBytes and MaxOutputBufferBytes bound
	// output_rules.buffer_bytes and streaming.max_buffer_bytes. A response
	// larger than the buffer is forwarded unscanned, so a buffer of a few
	// bytes switches the output scan off; a response within it is held in
	// memory.
	MinOutputBufferBytes = 1024
	MaxOutputBufferBytes = 64 << 20

	// MaxMinimumConfidence is the top of the confidence scale. A minimum
	// above it is one no finding reaches.
	MaxMinimumConfidence = 100

	// MinEndpointTimeout is the shortest timeout a pool endpoint may have.
	// Below it every attempt fails on the deadline, and deadline errors do
	// not open the circuit breaker.
	MinEndpointTimeout = 100 * time.Millisecond

	// MinClassifierTimeoutMs and MinClassifierMaxChars are the smallest
	// values at which the inline classifier can still read anything: a
	// call takes 15 to 30 ms, so a shorter deadline fails every one, and a
	// budget of a few characters leaves every message partly unread.
	MinClassifierTimeoutMs = 10
	MinClassifierMaxChars  = 16

	// MaxEntityNameBytes bounds a custom entity's name.
	MaxEntityNameBytes = 64
)

// entityName is what a custom entity may be called. The name is written
// into forwarded request bodies as "[<name>_REDACTED]", so it is kept to
// characters that cannot close a JSON string or read as an instruction.
var entityName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func boundError(field, rule, format string, args ...interface{}) ValidationIssue {
	return ValidationIssue{Field: field, Rule: rule, Message: fmt.Sprintf(format, args...), Severity: "error"}
}

func boundWarning(field, rule, format string, args ...interface{}) ValidationIssue {
	return ValidationIssue{Field: field, Rule: rule, Message: fmt.Sprintf(format, args...), Severity: "warning"}
}

// boundsIssues reports the values of a policy that are outside their bounds.
// An issue of severity "error" fails the load in LoadFromBytes, so no way of
// installing a policy can bring the value in; ValidateSemantics reports all
// of them to the editor. Warnings do not fail the load.
func boundsIssues(p *Policy) []ValidationIssue {
	if p == nil {
		return nil
	}
	var issues []ValidationIssue
	issues = append(issues, patternIssues(p)...)
	issues = append(issues, ruleBoundIssues(p)...)
	issues = append(issues, limitIssues(p)...)
	issues = append(issues, outputBoundIssues(p)...)
	issues = append(issues, poolTimeoutIssues(p)...)
	issues = append(issues, classifierBoundIssues(p)...)
	issues = append(issues, noEffectIssues(p)...)
	return issues
}

// firstBoundError returns the first load-blocking issue, if any.
func firstBoundError(issues []ValidationIssue) (ValidationIssue, bool) {
	for _, is := range issues {
		if is.Severity == "error" {
			return is, true
		}
	}
	return ValidationIssue{}, false
}

// patternIssues covers the regular expressions of the policy and how many
// of them there are.
func patternIssues(p *Policy) []ValidationIssue {
	var issues []ValidationIssue
	if n := len(p.CustomEntities); n > scanner.MaxCustomEntities {
		issues = append(issues, boundError("custom_entities", "too_many", "%d custom entities, the limit is %d", n, scanner.MaxCustomEntities))
	}
	for i, ce := range p.CustomEntities {
		prefix := fmt.Sprintf("custom_entities[%d]", i)
		if ce.Name != "" {
			switch {
			case len(ce.Name) > MaxEntityNameBytes:
				issues = append(issues, boundError(prefix+".name", "name_too_long", "name is %d bytes, the limit is %d", len(ce.Name), MaxEntityNameBytes))
			case !entityName.MatchString(ce.Name):
				issues = append(issues, boundError(prefix+".name", "name_charset", "name may only contain letters, digits and underscore (A-Z, a-z, 0-9, _): it is written into redacted text"))
			}
		}
		if ce.Pattern != "" {
			if err := scanner.CheckPattern(ce.Pattern); err != nil {
				issues = append(issues, boundError(prefix+".pattern", "pattern_limit", "%v", err))
			}
		}
	}

	if n := len(p.Competitors); n > scanner.MaxCompetitors {
		issues = append(issues, boundError("competitors", "too_many", "%d competitors, the limit is %d", n, scanner.MaxCompetitors))
	}
	for i, c := range p.Competitors {
		prefix := fmt.Sprintf("competitors[%d].patterns", i)
		if n := len(c.Patterns); n > scanner.MaxPatternsPerCompetitor {
			issues = append(issues, boundError(prefix, "too_many", "%d patterns for one competitor, the limit is %d", n, scanner.MaxPatternsPerCompetitor))
		}
		for j, pat := range c.Patterns {
			if strings.TrimSpace(pat) == "" {
				continue
			}
			if err := scanner.CheckCompetitorPattern(pat); err != nil {
				issues = append(issues, boundError(fmt.Sprintf("%s[%d]", prefix, j), "pattern_limit", "%v", err))
			}
		}
	}

	if os := p.OperatorState; os != nil {
		if n := len(os.Assertions); n > scanner.MaxOperatorStatePatterns {
			issues = append(issues, boundError("operator_state.assertions", "too_many", "%d assertions, the limit is %d", n, scanner.MaxOperatorStatePatterns))
		}
		if n := len(os.Authorization); n > scanner.MaxOperatorStatePatterns {
			issues = append(issues, boundError("operator_state.authorization", "too_many", "%d authorization rules, the limit is %d", n, scanner.MaxOperatorStatePatterns))
		}
		for i, a := range os.Assertions {
			if strings.TrimSpace(a.DecisionPattern) == "" {
				continue
			}
			if err := scanner.CheckMatcherPattern(a.DecisionPattern); err != nil {
				issues = append(issues, boundError(fmt.Sprintf("operator_state.assertions[%d].decision_pattern", i), "pattern_limit", "%v", err))
			}
		}
		for i, a := range os.Authorization {
			if strings.TrimSpace(a.DecisionPattern) == "" {
				continue
			}
			if err := scanner.CheckMatcherPattern(a.DecisionPattern); err != nil {
				issues = append(issues, boundError(fmt.Sprintf("operator_state.authorization[%d].decision_pattern", i), "pattern_limit", "%v", err))
			}
		}
	}
	return issues
}

// sortedRuleNames makes the order of issues, and so the one error a failed
// load reports, the same on every run.
func sortedRuleNames(rules map[string]Rule) []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// confidenceIssue refuses a minimum no finding can reach. A negative one is
// no minimum at all, which turns nothing off; ValidateSemantics reports it
// and the load lets it be.
func confidenceIssue(field string, v int) (ValidationIssue, bool) {
	if v > MaxMinimumConfidence {
		return boundError(field, "out_of_range", "minimum_confidence is %d and confidence goes from 0 to %d: no finding would reach it", v, MaxMinimumConfidence), true
	}
	return ValidationIssue{}, false
}

func ruleBoundIssues(p *Policy) []ValidationIssue {
	var issues []ValidationIssue
	for _, name := range sortedRuleNames(p.Rules) {
		rule := p.Rules[name]
		if is, bad := confidenceIssue("rules."+name+".minimum_confidence", rule.MinimumConfidence); bad {
			issues = append(issues, is)
		}
		// The engine compares the action as written. "block" or "BLOK" is
		// none of the names it knows, and a rule with such an action lets
		// everything through while reading as if it stopped it. An absent
		// action is left to the rule's own default, as before.
		if rule.Action != "" {
			switch rule.Action {
			case ActionBlock, ActionStrip, ActionRedact, ActionWarn, ActionLog, ActionPass:
			default:
				issues = append(issues, boundError("rules."+name+".action", "enum",
					"unknown action %q (use BLOCK, STRIP, REDACT, WARN, LOG or PASS, in upper case)", printableValue(string(rule.Action))))
			}
		}
		if rule.OverrideAction != nil {
			// The engine compares the value as written, so anything but
			// one of these exact names acts as PASS.
			switch *rule.OverrideAction {
			case ActionBlock, ActionStrip, ActionRedact, ActionWarn, ActionLog, ActionPass:
			default:
				issues = append(issues, boundError("rules."+name+".override_action", "enum",
					"unknown override_action %q (use BLOCK, STRIP, REDACT, WARN, LOG or PASS, in upper case)", printableValue(string(*rule.OverrideAction))))
			}
		}
	}
	return issues
}

// printableValue shortens a value from the policy for an error message.
func printableValue(v string) string {
	if len(v) > 32 {
		return v[:32] + "..."
	}
	return v
}

// limitIssues covers the rate limit, the cost caps, the body limits and the
// cache lifetime.
func limitIssues(p *Policy) []ValidationIssue {
	var issues []ValidationIssue
	if rl := p.RateLimit; rl != nil {
		switch {
		case rl.MaxRequestsPerMinute < 0:
			issues = append(issues, boundError("rate_limit.max_requests_per_minute", "negative", "max_requests_per_minute must not be negative; leave it out or set 0 for no limit per minute"))
		case rl.MaxRequestsPerMinute == 0:
			issues = append(issues, boundWarning("rate_limit.max_requests_per_minute", "unlimited", "max_requests_per_minute is 0 or not set: requests per minute are not limited"))
		}
		if rl.MaxTokensPerDay < 0 {
			issues = append(issues, boundError("rate_limit.max_tokens_per_day", "negative", "max_tokens_per_day must be non-negative"))
		}
	}
	if c := p.Cost; c != nil {
		if c.MaxTokensPerDay < 0 {
			issues = append(issues, boundError("cost.max_tokens_per_day", "negative", "max_tokens_per_day must be non-negative"))
		}
		if c.MaxCostUSDPerDay < 0 {
			issues = append(issues, boundError("cost.max_cost_usd_per_day", "negative", "max_cost_usd_per_day must be non-negative"))
		}
		if c.MaxTokensPerDay == 0 {
			issues = append(issues, boundWarning("cost.max_tokens_per_day", "unlimited", "max_tokens_per_day is 0 or not set: there is no daily token cap"))
		}
		if c.MaxCostUSDPerDay == 0 {
			issues = append(issues, boundWarning("cost.max_cost_usd_per_day", "unlimited", "max_cost_usd_per_day is 0 or not set: there is no daily cost cap"))
		}
	}
	if bl := p.BodyLimits; bl != nil {
		// Zero or less means "not set here" and falls back to the default
		// or to TAMGA_MAX_BODY_BYTES, so it is not judged here.
		if is, bad := bodyLimitIssue("body_limits.default.max_bytes", bl.Default.MaxBytes); bad {
			issues = append(issues, is)
		}
		provs := make([]string, 0, len(bl.PerProvider))
		for prov := range bl.PerProvider {
			provs = append(provs, prov)
		}
		sort.Strings(provs)
		for _, prov := range provs {
			if is, bad := bodyLimitIssue("body_limits.per_provider."+prov+".max_bytes", bl.PerProvider[prov].MaxBytes); bad {
				issues = append(issues, is)
			}
		}
	}
	return issues
}

func bodyLimitIssue(field string, v int) (ValidationIssue, bool) {
	switch {
	case v > 0 && v < MinBodyLimitBytes:
		return boundWarning(field, "too_small", "max_bytes is %d: a limit below %d refuses nearly every request", v, MinBodyLimitBytes), true
	case v > MaxBodyLimitBytes:
		return boundError(field, "too_large", "max_bytes is %d, the maximum is %d: a request body is held in memory whole", v, MaxBodyLimitBytes), true
	}
	return ValidationIssue{}, false
}

func outputBoundIssues(p *Policy) []ValidationIssue {
	or := p.OutputRules
	if or == nil || !or.Enabled {
		return nil
	}
	var issues []ValidationIssue
	// Zero or less falls back to the default of 1 MiB.
	switch {
	case or.BufferBytes > 0 && or.BufferBytes < MinOutputBufferBytes:
		issues = append(issues, boundError("output_rules.buffer_bytes", "too_small", "buffer_bytes is %d, the minimum is %d: a response larger than the buffer is forwarded unscanned", or.BufferBytes, MinOutputBufferBytes))
	case or.BufferBytes > MaxOutputBufferBytes:
		issues = append(issues, boundError("output_rules.buffer_bytes", "too_large", "buffer_bytes is %d, the maximum is %d: a response within the buffer is held in memory", or.BufferBytes, MaxOutputBufferBytes))
	}
	if is, bad := confidenceIssue("output_rules.minimum_confidence", or.MinimumConfidence); bad {
		issues = append(issues, is)
	}
	if st := or.Streaming; st != nil && st.Enabled && st.MaxBufferBytes > MaxOutputBufferBytes {
		issues = append(issues, boundError("output_rules.streaming.max_buffer_bytes", "too_large", "max_buffer_bytes is %d, the maximum is %d", st.MaxBufferBytes, MaxOutputBufferBytes))
	}
	return issues
}

func poolTimeoutIssues(p *Policy) []ValidationIssue {
	if p.Providers == nil {
		return nil
	}
	routes := make([]string, 0, len(p.Providers.Pools))
	for route := range p.Providers.Pools {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	var issues []ValidationIssue
	for _, route := range routes {
		for i, ep := range p.Providers.Pools[route].Endpoints {
			if ep.Timeout == "" {
				continue
			}
			d, err := time.ParseDuration(ep.Timeout)
			if err != nil {
				// Reported by ValidateSemantics; the pool falls back to
				// its default.
				continue
			}
			if d < MinEndpointTimeout {
				issues = append(issues, boundError(fmt.Sprintf("providers.pools.%s.providers[%d].timeout", route, i), "too_short",
					"timeout is %s, the minimum is %s: leave it out for the default of 30s", d, MinEndpointTimeout))
			}
		}
	}
	return issues
}

func classifierBoundIssues(p *Policy) []ValidationIssue {
	if p.Scan == nil || p.Scan.Classifier == nil {
		return nil
	}
	c := p.Scan.Classifier
	var issues []ValidationIssue
	if c.TimeoutMs > 0 && c.TimeoutMs < MinClassifierTimeoutMs {
		issues = append(issues, boundError("scan.classifier.timeout_ms", "too_short", "timeout_ms is %d, the minimum is %d: no classification finishes sooner, so every one would fail", c.TimeoutMs, MinClassifierTimeoutMs))
	}
	if c.MaxChars > 0 && c.MaxChars < MinClassifierMaxChars {
		issues = append(issues, boundError("scan.classifier.max_chars", "too_small", "max_chars is %d, the minimum is %d: the classifier would read next to nothing", c.MaxChars, MinClassifierMaxChars))
	}
	return issues
}

// noEffectIssues warns about fields the proxy accepts and does not act on.
// They stay in the schema so that existing files keep loading; the warning
// keeps them from reading as an assurance.
func noEffectIssues(p *Policy) []ValidationIssue {
	var issues []ValidationIssue
	if d := p.Data; d != nil {
		if d.RetentionDays != 0 {
			issues = append(issues, boundWarning("data.retention_days", "no_effect",
				"retention_days has no effect: retention is set in the environment (TAMGA_RETENTION_ENABLED and the TAMGA_RETENTION_*_DAYS variables)"))
		}
		if strings.TrimSpace(d.Residency) != "" {
			issues = append(issues, boundWarning("data.residency", "no_effect",
				"residency restricts nothing: it is compared with TAMGA_REGION once at startup and a mismatch is logged. Where data goes is decided by the providers and upstream hosts the deployment allows"))
		}
	}
	if c := p.Cache; c != nil {
		if c.SemanticOn {
			issues = append(issues, boundWarning("cache.semantic", "no_effect",
				"semantic has no effect: similarity caching is not implemented, the cache matches a request exactly (cache.enabled)"))
		}
		// The cache holds the TTL to its own ceiling where an entry is
		// stored (cache.EffectiveTTL); this says so to whoever wrote more.
		if used, clamped := cache.EffectiveTTL(c.TTLSeconds); clamped {
			issues = append(issues, boundWarning("cache.ttl_seconds", "clamped", "ttl_seconds is %d, above the ceiling: entries are kept for %d seconds", c.TTLSeconds, int(used.Seconds())))
		} else if c.TTLSeconds < 0 {
			issues = append(issues, boundWarning("cache.ttl_seconds", "clamped", "ttl_seconds is negative: the default of %d seconds is used", int(used.Seconds())))
		}
	}
	if or := p.OutputRules; or != nil {
		if or.MaxStreamBytes != 0 {
			issues = append(issues, boundWarning("output_rules.max_stream_bytes", "no_effect",
				"max_stream_bytes has no effect: a streamed response is scanned within output_rules.streaming.max_buffer_bytes"))
		}
		if or.Streaming != nil && or.Streaming.OverlapBytes != 0 {
			issues = append(issues, boundWarning("output_rules.streaming.overlap_bytes", "no_effect", "overlap_bytes has no effect: it is reserved and not read"))
		}
	}
	return issues
}
