package policy

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var reDosNestedQuant = regexp.MustCompile(`\([^)]*[+*]\)[+*]`)

// ValidationIssue is a single semantic policy problem (error blocks apply; warning is advisory).
type ValidationIssue struct {
	Field    string `json:"field"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "error" | "warning"
}

// knownScannerTypes lists finding types produced by registered scanners.
// Rule keys are matched as "<type>_detection" or "<type>".
var knownScannerTypes = map[string]struct{}{
	"pii":                {},
	"secret":             {},
	"injection":          {},
	"content_moderation": {},
	"competitor":         {},
	"custom":             {},
	"operator_state":     {},
}

// isValidSeverity returns true for the four canonical severity levels.
func isValidSeverity(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "high", "medium", "low":
		return true
	}
	return false
}

// isKnownRuleKey returns true if the rule key references a known scanner.
// Keys like "pii_detection", "secret_detection" or bare "pii", "secret" are valid.
func isKnownRuleKey(key string) bool {
	base := strings.TrimSuffix(key, "_detection")
	if base == key {
		// No _detection suffix; check bare.
		_, ok := knownScannerTypes[strings.ToLower(base)]
		return ok
	}
	_, ok := knownScannerTypes[strings.ToLower(base)]
	return ok
}

// ValidateSemantics runs checks beyond YAML/JSON parse: duplicate names, regexp validity, coarse ReDoS hints.
func ValidateSemantics(p *Policy) []ValidationIssue {
	if p == nil {
		return []ValidationIssue{{Field: "", Rule: "policy_nil", Message: "policy is nil", Severity: "error"}}
	}
	var issues []ValidationIssue
	if strings.TrimSpace(p.Name) == "" {
		issues = append(issues, ValidationIssue{
			Field:    "name",
			Rule:     "required",
			Message:  "policy name is required",
			Severity: "error",
		})
	}
	seen := make(map[string]struct{})

	for i, ce := range p.CustomEntities {
		prefix := fmt.Sprintf("custom_entities[%d]", i)
		name := strings.TrimSpace(ce.Name)
		if name == "" {
			issues = append(issues, ValidationIssue{Field: prefix + ".name", Rule: "required", Message: "custom entity name is empty", Severity: "error"})
			continue
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			issues = append(issues, ValidationIssue{
				Field:    prefix + ".name",
				Rule:     "duplicate",
				Message:  fmt.Sprintf("duplicate custom entity name %q", name),
				Severity: "error",
			})
		} else {
			seen[key] = struct{}{}
		}
		if ce.Pattern == "" {
			issues = append(issues, ValidationIssue{Field: prefix + ".pattern", Rule: "required", Message: "pattern is empty", Severity: "error"})
			continue
		}
		if _, err := regexp.Compile(ce.Pattern); err != nil {
			issues = append(issues, ValidationIssue{
				Field:    prefix + ".pattern",
				Rule:     "regexp",
				Message:  err.Error(),
				Severity: "error",
			})
			continue
		}
		if hint := reDosHeuristic(ce.Pattern); hint != "" {
			issues = append(issues, ValidationIssue{
				Field:    prefix + ".pattern",
				Rule:     "redos_risk",
				Message:  hint,
				Severity: "warning",
			})
		}
	}

	for name, rule := range p.Rules {
		if rule.Action != "" {
			a := Action(strings.ToUpper(strings.TrimSpace(string(rule.Action))))
			switch a {
			case ActionBlock, ActionRedact, ActionWarn, ActionLog, ActionPass:
			default:
				issues = append(issues, ValidationIssue{
					Field:    "rules." + name + ".action",
					Rule:     "enum",
					Message:  fmt.Sprintf("unknown action %q (use BLOCK, REDACT, WARN, LOG, PASS)", rule.Action),
					Severity: "error",
				})
			}
		}
	}

	if p.RateLimit != nil && p.RateLimit.MaxRequestsPerMinute > 100000 {
		issues = append(issues, ValidationIssue{
			Field:    "rate_limit.max_requests_per_minute",
			Rule:     "unreasonable",
			Message:  "values above 100000/min may be impractical to enforce",
			Severity: "warning",
		})
	}

	if p.BodyLimits != nil {
		if p.BodyLimits.Default.MaxBytes <= 0 {
			issues = append(issues, ValidationIssue{
				Field:    "body_limits.default.max_bytes",
				Rule:     "non_positive",
				Message:  "max_bytes must be greater than 0",
				Severity: "error",
			})
		}
		if p.BodyLimits.Default.MaxBytes > 10*1024*1024 {
			issues = append(issues, ValidationIssue{
				Field:    "body_limits.default.max_bytes",
				Rule:     "large_body",
				Message:  "values above 10MB may cause memory pressure on the proxy",
				Severity: "warning",
			})
		}
		for prov, rule := range p.BodyLimits.PerProvider {
			prefix := fmt.Sprintf("body_limits.per_provider.%s", prov)
			if rule.MaxBytes <= 0 {
				issues = append(issues, ValidationIssue{
					Field:    prefix + ".max_bytes",
					Rule:     "non_positive",
					Message:  "max_bytes must be greater than 0",
					Severity: "error",
				})
			}
			if rule.MaxBytes > 10*1024*1024 {
				issues = append(issues, ValidationIssue{
					Field:    prefix + ".max_bytes",
					Rule:     "large_body",
					Message:  "values above 10MB may cause memory pressure on the proxy",
					Severity: "warning",
				})
			}
		}
	}

	if p.OutputRules != nil && p.OutputRules.Enabled {
		if p.OutputRules.BufferBytes < 0 {
			issues = append(issues, ValidationIssue{
				Field:    "output_rules.buffer_bytes",
				Rule:     "negative",
				Message:  "buffer_bytes must be non-negative",
				Severity: "error",
			})
		}
		if p.OutputRules.BufferBytes > 0 && p.OutputRules.BufferBytes < 1024 {
			issues = append(issues, ValidationIssue{
				Field:    "output_rules.buffer_bytes",
				Rule:     "too_small",
				Message:  "buffer_bytes below 1024 may miss findings in non-streaming scans",
				Severity: "warning",
			})
		}
		if p.OutputRules.ScanWindowMs < 0 {
			issues = append(issues, ValidationIssue{
				Field:    "output_rules.scan_window_ms",
				Rule:     "negative",
				Message:  "scan_window_ms must be non-negative",
				Severity: "error",
			})
		}
		if p.OutputRules.MinimumConfidence < 0 {
			issues = append(issues, ValidationIssue{
				Field:    "output_rules.minimum_confidence",
				Rule:     "negative",
				Message:  "minimum_confidence must be non-negative",
				Severity: "error",
			})
		}
		if st := p.OutputRules.Streaming; st != nil && st.Enabled {
			if st.MaxBufferBytes <= 0 {
				issues = append(issues, ValidationIssue{
					Field:    "output_rules.streaming.max_buffer_bytes",
					Rule:     "non_positive",
					Message:  "max_buffer_bytes must be greater than 0 when streaming is enabled",
					Severity: "error",
				})
			}
			if st.MaxBufferBytes > 512*1024 {
				issues = append(issues, ValidationIssue{
					Field:    "output_rules.streaming.max_buffer_bytes",
					Rule:     "large_buffer",
					Message:  "streaming scan buffer above 512KB may increase latency and memory use",
					Severity: "warning",
				})
			}
			if st.MaxBufferBytes > 0 && st.MaxBufferBytes < 512 {
				issues = append(issues, ValidationIssue{
					Field:    "output_rules.streaming.max_buffer_bytes",
					Rule:     "too_small",
					Message:  "max_buffer_bytes should be at least 512 for useful stream scanning",
					Severity: "warning",
				})
			}
		}
	}

	if p.RateLimit != nil && p.RateLimit.MaxTokensPerDay < 0 {
		issues = append(issues, ValidationIssue{
			Field:    "rate_limit.max_tokens_per_day",
			Rule:     "negative",
			Message:  "max_tokens_per_day must be non-negative",
			Severity: "error",
		})
	}

	if p.Cost != nil && p.Cost.MaxTokensPerDay < 0 {
		issues = append(issues, ValidationIssue{
			Field:    "cost.max_tokens_per_day",
			Rule:     "negative",
			Message:  "max_tokens_per_day must be non-negative",
			Severity: "error",
		})
	}

	// Validate rule-level minimum_confidence.
	for name, rule := range p.Rules {
		if rule.MinimumConfidence < 0 {
			issues = append(issues, ValidationIssue{
				Field:    "rules." + name + ".minimum_confidence",
				Rule:     "negative",
				Message:  "minimum_confidence must be non-negative",
				Severity: "error",
			})
		}
	}

	// Warn for rule keys that do not reference known scanner finding types.
	for name := range p.Rules {
		if !isKnownRuleKey(name) {
			issues = append(issues, ValidationIssue{
				Field:    "rules." + name,
				Rule:     "unknown_scanner",
				Message:  fmt.Sprintf("rule key %q does not match any known scanner type; it will never fire", name),
				Severity: "warning",
			})
		}
	}

	// Validate custom entity severity values.
	for i, ce := range p.CustomEntities {
		if ce.Severity != "" && !isValidSeverity(ce.Severity) {
			issues = append(issues, ValidationIssue{
				Field:    fmt.Sprintf("custom_entities[%d].severity", i),
				Rule:     "enum",
				Message:  fmt.Sprintf("unknown severity %q (use critical, high, medium, or low)", ce.Severity),
				Severity: "warning",
			})
		}
	}

	// Validate competitor severity and action values.
	for i, c := range p.Competitors {
		if c.Severity != "" && !isValidSeverity(c.Severity) {
			issues = append(issues, ValidationIssue{
				Field:    fmt.Sprintf("competitors[%d].severity", i),
				Rule:     "enum",
				Message:  fmt.Sprintf("unknown severity %q (use critical, high, medium, or low)", c.Severity),
				Severity: "warning",
			})
		}
		if c.Action != "" {
			a := Action(strings.ToUpper(strings.TrimSpace(c.Action)))
			switch a {
			case ActionBlock, ActionRedact, ActionWarn, ActionLog, ActionPass:
			default:
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("competitors[%d].action", i),
					Rule:     "enum",
					Message:  fmt.Sprintf("unknown action %q (use BLOCK, REDACT, WARN, LOG, PASS)", c.Action),
					Severity: "error",
				})
			}
		}
	}

	// Validate exception blocks.
	for i, exc := range p.Exceptions {
		prefix := fmt.Sprintf("exceptions[%d]", i)

		// Rule reference is required.
		ruleName := strings.TrimSpace(exc.Rule)
		if ruleName == "" {
			issues = append(issues, ValidationIssue{
				Field:    prefix + ".rule",
				Rule:     "required",
				Message:  "exception rule name is empty",
				Severity: "error",
			})
			continue
		}

		// Referenced rule name must exist in the policy rules.
		if _, ok := p.Rules[ruleName]; !ok {
			// Also check with/without _detection suffix variants.
			ruleWithSuffix := ruleName + "_detection"
			_, okSuffix := p.Rules[ruleWithSuffix]
			_, okNoSuffix := p.Rules[strings.TrimSuffix(ruleName, "_detection")]
			if !okSuffix && !okNoSuffix {
				issues = append(issues, ValidationIssue{
					Field:    prefix + ".rule",
					Rule:     "unknown_rule",
					Message:  fmt.Sprintf("exception references unknown rule %q", ruleName),
					Severity: "error",
				})
			}
		}

		// Roles must not be empty.
		if len(exc.Roles) == 0 {
			issues = append(issues, ValidationIssue{
				Field:    prefix + ".roles",
				Rule:     "required",
				Message:  "exception roles list is empty",
				Severity: "error",
			})
			continue
		}

		// Every role must be valid.
		for j, r := range exc.Roles {
			roleName := strings.TrimSpace(r)
			if roleName == "" {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("%s.roles[%d]", prefix, j),
					Rule:     "required",
					Message:  "exception role name is empty",
					Severity: "error",
				})
				continue
			}
			if !IsValidRole(roleName) {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("%s.roles[%d]", prefix, j),
					Rule:     "unknown_role",
					Message:  fmt.Sprintf("unknown role %q in exception", roleName),
					Severity: "error",
				})
			}
		}

		// Validate expires_at format if present.
		if exc.ExpiresAt != "" {
			if _, err := time.Parse(time.RFC3339, exc.ExpiresAt); err != nil {
				issues = append(issues, ValidationIssue{
					Field:    prefix + ".expires_at",
					Rule:     "rfc3339",
					Message:  fmt.Sprintf("invalid expires_at %q: %v", exc.ExpiresAt, err),
					Severity: "error",
				})
			}
		}
	}

	if p.Providers != nil && len(p.Providers.Pools) > 0 {
		for routeKey, pool := range p.Providers.Pools {
			prefix := "providers.pools." + routeKey
			st := strings.TrimSpace(pool.Strategy)
			if st != "" && !strings.EqualFold(st, "fallback_chain") && !strings.EqualFold(st, "round_robin") {
				issues = append(issues, ValidationIssue{
					Field:    prefix + ".strategy",
					Rule:     "enum",
					Message:  fmt.Sprintf("unknown strategy %q (use fallback_chain or round_robin)", pool.Strategy),
					Severity: "warning",
				})
			}
			if len(pool.Endpoints) == 0 {
				issues = append(issues, ValidationIssue{
					Field:    prefix + ".providers",
					Rule:     "required",
					Message:  "provider pool must list at least one endpoint",
					Severity: "error",
				})
				continue
			}
			for i, ep := range pool.Endpoints {
				eprefix := fmt.Sprintf("%s.providers[%d]", prefix, i)
				if strings.TrimSpace(ep.Name) == "" {
					issues = append(issues, ValidationIssue{
						Field:    eprefix + ".name",
						Rule:     "required",
						Message:  "endpoint name is required",
						Severity: "error",
					})
				}
				if strings.TrimSpace(ep.BaseURL) == "" {
					issues = append(issues, ValidationIssue{
						Field:    eprefix + ".base_url",
						Rule:     "required",
						Message:  "base_url is required",
						Severity: "error",
					})
					continue
				}
				if u, err := url.Parse(ep.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
					issues = append(issues, ValidationIssue{
						Field:    eprefix + ".base_url",
						Rule:     "url",
						Message:  "base_url must be an absolute URL with scheme and host",
						Severity: "error",
					})
				}
				if ep.Timeout != "" {
					if _, err := time.ParseDuration(ep.Timeout); err != nil {
						issues = append(issues, ValidationIssue{
							Field:    eprefix + ".timeout",
							Rule:     "duration",
							Message:  err.Error(),
							Severity: "error",
						})
					}
				}
			}
		}
	}

	return issues
}

func reDosHeuristic(pattern string) string {
	if reDosNestedQuant.MatchString(pattern) {
		return "nested quantifiers may cause exponential backtracking; simplify the pattern if possible"
	}
	return ""
}

// HasValidationErrors returns true if any issue has severity "error".
func HasValidationErrors(issues []ValidationIssue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}
