package policy

import (
	"strings"
	"testing"
)

func TestValidateSemantics_RequiresName(t *testing.T) {
	p := &Policy{Version: "1.0", Rules: map[string]Rule{}}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty policy name")
	}
}

func TestValidateSemantics_CustomEntityDuplicate(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "x", Pattern: `\d+`, Action: "BLOCK", Severity: "high", Confidence: 0.9},
			{Name: "x", Pattern: `[a-z]+`, Action: "BLOCK", Severity: "high", Confidence: 0.9},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected duplicate error")
	}
}

func TestValidateSemantics_InvalidRegex(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "bad", Pattern: `(`, Action: "BLOCK", Severity: "high", Confidence: 0.9},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected regexp error")
	}
}

func TestValidateSemantics_InvalidRuleAction(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"r1": {Action: "DELETE"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected action error")
	}
}

func TestValidateSemantics_RedosWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "risky", Pattern: `(a+)+`, Action: "BLOCK", Severity: "high", Confidence: 0.9},
		},
	}
	issues := ValidateSemantics(p)
	var warn bool
	for _, i := range issues {
		if i.Rule == "redos_risk" {
			warn = true
		}
	}
	if !warn {
		t.Fatal("expected redos warning")
	}
	if HasValidationErrors(issues) {
		t.Fatal("redos should be warning only")
	}
}

// --- New validation tests ---

func TestValidateSemantics_BodyLimitsDefaultZeroMaxBytes(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default: BodyLimitRule{MaxBytes: 0},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for zero max_bytes in body_limits.default")
	}
	found := false
	for _, i := range issues {
		if i.Field == "body_limits.default.max_bytes" && i.Rule == "non_positive" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected non_positive error on body_limits.default.max_bytes, got: %+v", issues)
	}
}

func TestValidateSemantics_BodyLimitsDefaultNegativeMaxBytes(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default: BodyLimitRule{MaxBytes: -1},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative max_bytes")
	}
}

func TestValidateSemantics_BodyLimitsPerProviderZeroMaxBytes(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			PerProvider: map[string]BodyLimitRule{
				"openai": {MaxBytes: 0},
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for zero max_bytes in per_provider")
	}
}

func TestValidateSemantics_BodyLimitsLargeWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default: BodyLimitRule{MaxBytes: 20 * 1024 * 1024},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("large body limit should be warning, not error")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "large_body" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected large_body warning, got: %+v", issues)
	}
}

func TestValidateSemantics_StreamingMaxBufferBytesZero(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled: true,
			Streaming: &OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 0,
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for zero max_buffer_bytes when streaming enabled")
	}
}

func TestValidateSemantics_StreamingMaxBufferBytesNegative(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled: true,
			Streaming: &OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: -100,
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative max_buffer_bytes when streaming enabled")
	}
}

func TestValidateSemantics_OutputRulesNegativeScanWindowMs(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled:      true,
			ScanWindowMs: -1,
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative scan_window_ms")
	}
}

func TestValidateSemantics_OutputRulesNegativeBufferBytes(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled:     true,
			BufferBytes: -1,
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative buffer_bytes")
	}
}

func TestValidateSemantics_OutputRulesNegativeMinimumConfidence(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled:           true,
			MinimumConfidence: -1,
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative output_rules minimum_confidence")
	}
}

func TestValidateSemantics_RuleMinimumConfidenceNegative(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK", MinimumConfidence: -5},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative rule minimum_confidence")
	}
}

func TestValidateSemantics_RuleMinimumConfidenceZeroValid(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK", MinimumConfidence: 0},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("zero minimum_confidence should be valid, got errors: %+v", issues)
	}
}

func TestValidateSemantics_UnknownRuleKeyWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"unknown_scanner": {Action: "BLOCK"},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("unknown rule key should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "unknown_scanner" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unknown_scanner warning, got: %+v", issues)
	}
}

func TestValidateSemantics_KnownRuleKeysNoWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection":       {Action: "REDACT"},
			"secret_detection":    {Action: "BLOCK"},
			"injection_detection": {Action: "BLOCK"},
			"content_moderation":  {Action: "WARN"},
			"competitor":          {Action: "LOG"},
			"custom":              {Action: "PASS"},
		},
	}
	issues := ValidateSemantics(p)
	for _, i := range issues {
		if i.Rule == "unknown_scanner" {
			t.Errorf("known rule key should not trigger unknown_scanner warning: %+v", i)
		}
	}
}

func TestValidateSemantics_CustomEntityInvalidSeverity(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "ce1", Pattern: `\d+`, Severity: "super_high", Action: "BLOCK", Confidence: 0.9},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("invalid severity should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "enum" && i.Field == "custom_entities[0].severity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected severity enum warning, got: %+v", issues)
	}
}

func TestValidateSemantics_CustomEntityValidSeverity(t *testing.T) {
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		p := &Policy{
			Version: "1.0",
			Name:    "test",
			CustomEntities: []CustomEntity{
				{Name: "ce1", Pattern: `\d+`, Severity: sev, Action: "BLOCK", Confidence: 0.9},
			},
		}
		issues := ValidateSemantics(p)
		for _, i := range issues {
			if i.Rule == "enum" && i.Field == "custom_entities[0].severity" {
				t.Errorf("valid severity %q should not trigger warning", sev)
			}
		}
	}
}

func TestValidateSemantics_CompetitorInvalidSeverity(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Competitors: []Competitor{
			{Name: "CompA", Patterns: []string{`comp`}, Severity: "unknown", Action: "LOG", Enabled: true},
		},
	}
	issues := ValidateSemantics(p)
	found := false
	for _, i := range issues {
		if i.Rule == "enum" && i.Field == "competitors[0].severity" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected competitor severity warning, got: %+v", issues)
	}
}

func TestValidateSemantics_CompetitorInvalidAction(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Competitors: []Competitor{
			{Name: "CompA", Patterns: []string{`comp`}, Severity: "high", Action: "DELETE", Enabled: true},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for invalid competitor action")
	}
}

func TestValidateSemantics_RateLimitNegativeMaxTokensPerDay(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		RateLimit: &RateLimit{
			MaxTokensPerDay: -1,
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative max_tokens_per_day")
	}
}

func TestValidateSemantics_CostNegativeMaxTokensPerDay(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Cost: &CostControl{
			MaxTokensPerDay: -100,
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative cost max_tokens_per_day")
	}
}

func TestValidateSemantics_NilPolicy(t *testing.T) {
	issues := ValidateSemantics(nil)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for nil policy")
	}
}

func TestValidateSemantics_ValidPolicyNoErrors(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "valid-policy",
		Rules: map[string]Rule{
			"pii_detection":    {Action: "REDACT", Sensitivity: "medium", Types: []string{"email"}, MinimumConfidence: 50},
			"secret_detection": {Action: "BLOCK", Sensitivity: "high"},
		},
		BodyLimits: &BodyLimitsConfig{
			Default: BodyLimitRule{MaxBytes: 2 * 1024 * 1024},
		},
		OutputRules: &OutputRules{
			Enabled:     true,
			BufferBytes: 4096,
			Streaming: &OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 8192,
			},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("valid policy should have no errors, got: %+v", issues)
	}
}

func TestHasValidationErrors_Empty(t *testing.T) {
	if HasValidationErrors(nil) {
		t.Fatal("nil should return false")
	}
	if HasValidationErrors([]ValidationIssue{}) {
		t.Fatal("empty should return false")
	}
}

func TestHasValidationErrors_WarningsOnly(t *testing.T) {
	issues := []ValidationIssue{
		{Severity: "warning", Rule: "large_body"},
		{Severity: "warning", Rule: "redos_risk"},
	}
	if HasValidationErrors(issues) {
		t.Fatal("warnings only should return false")
	}
}

func TestHasValidationErrors_Mixed(t *testing.T) {
	issues := []ValidationIssue{
		{Severity: "warning", Rule: "large_body"},
		{Severity: "error", Rule: "required"},
	}
	if !HasValidationErrors(issues) {
		t.Fatal("mixed should return true")
	}
}

// --- Provider pools validation tests ---

func TestValidateSemantics_ProviderPoolUnknownStrategy(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"anthropic": {
					Strategy: "random_choice",
					Endpoints: []ProviderUpstreamEndpoint{
						{Name: "ep1", BaseURL: "https://api.anthropic.com"},
					},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("unknown strategy should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "enum" && i.Field == "providers.pools.anthropic.strategy" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unknown strategy warning, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderPoolEmptyEndpoints(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"openai": {
					Strategy:  "fallback_chain",
					Endpoints: []ProviderUpstreamEndpoint{},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty endpoints")
	}
	found := false
	for _, i := range issues {
		if i.Field == "providers.pools.openai.providers" && i.Rule == "required" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected required error for empty endpoints, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderEndpointEmptyName(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"openai": {
					Strategy: "round_robin",
					Endpoints: []ProviderUpstreamEndpoint{
						{Name: "", BaseURL: "https://api.openai.com"},
					},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty endpoint name")
	}
	found := false
	for _, i := range issues {
		if i.Field == "providers.pools.openai.providers[0].name" && i.Rule == "required" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected required error for endpoint name, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderEndpointEmptyBaseURL(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"openai": {
					Strategy: "round_robin",
					Endpoints: []ProviderUpstreamEndpoint{
						{Name: "ep1", BaseURL: ""},
					},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty base_url")
	}
	found := false
	for _, i := range issues {
		if i.Field == "providers.pools.openai.providers[0].base_url" && i.Rule == "required" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected required error for base_url, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderEndpointInvalidURL(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"openai": {
					Strategy: "fallback_chain",
					Endpoints: []ProviderUpstreamEndpoint{
						{Name: "ep1", BaseURL: "not-a-valid-url-%"},
					},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for invalid base_url")
	}
	found := false
	for _, i := range issues {
		if i.Field == "providers.pools.openai.providers[0].base_url" && i.Rule == "url" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected url error for base_url, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderEndpointInvalidTimeout(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"openai": {
					Strategy: "fallback_chain",
					Endpoints: []ProviderUpstreamEndpoint{
						{Name: "ep1", BaseURL: "https://api.openai.com", Timeout: "xyz"},
					},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for invalid timeout")
	}
	found := false
	for _, i := range issues {
		if i.Field == "providers.pools.openai.providers[0].timeout" && i.Rule == "duration" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected duration error for timeout, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderEndpointValidTimeout(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Providers: &Providers{
			Pools: map[string]ProviderUpstreamPool{
				"openai": {
					Strategy: "fallback_chain",
					Endpoints: []ProviderUpstreamEndpoint{
						{Name: "ep1", BaseURL: "https://api.openai.com", Timeout: "30s"},
					},
				},
			},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("valid timeout should have no errors, got: %+v", issues)
	}
}

func TestValidateSemantics_RateLimitTooHighWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		RateLimit: &RateLimit{
			MaxRequestsPerMinute: 200000,
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("high rate limit should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "unreasonable" && i.Field == "rate_limit.max_requests_per_minute" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unreasonable warning for high rate limit, got: %+v", issues)
	}
}

func TestValidateSemantics_StreamingBufferTooSmallWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled: true,
			Streaming: &OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 256,
			},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("small streaming buffer should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "too_small" && i.Field == "output_rules.streaming.max_buffer_bytes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected too_small warning for streaming buffer, got: %+v", issues)
	}
}

func TestValidateSemantics_StreamingBufferTooLargeWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled: true,
			Streaming: &OutputStreamScan{
				Enabled:        true,
				MaxBufferBytes: 1024 * 1024,
			},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("large streaming buffer should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "large_buffer" && i.Field == "output_rules.streaming.max_buffer_bytes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected large_buffer warning, got: %+v", issues)
	}
}

func TestValidateSemantics_OutputBufferBytesTooSmallWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		OutputRules: &OutputRules{
			Enabled:     true,
			BufferBytes: 512,
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("small buffer_bytes should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "too_small" && i.Field == "output_rules.buffer_bytes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected too_small warning for buffer_bytes, got: %+v", issues)
	}
}

func TestValidateSemantics_BodyLimitsPerProviderLargeWarning(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default: BodyLimitRule{MaxBytes: 1 * 1024 * 1024},
			PerProvider: map[string]BodyLimitRule{
				"anthropic": {MaxBytes: 20 * 1024 * 1024},
			},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatal("large per-provider body limit should be warning only")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "large_body" && i.Field == "body_limits.per_provider.anthropic.max_bytes" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected large_body warning for per_provider, got: %+v", issues)
	}
}

func TestValidateSemantics_DefaultBodyLimitNegativeMaxBytes(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default: BodyLimitRule{MaxBytes: -5},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for negative max_bytes in body_limits.default")
	}
	found := false
	for _, i := range issues {
		if i.Field == "body_limits.default.max_bytes" && i.Rule == "non_positive" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected non_positive error on body_limits.default.max_bytes, got: %+v", issues)
	}
}

func TestValidateSemantics_ProviderPoolValidStrategy(t *testing.T) {
	for _, strat := range []string{"fallback_chain", "FALLBACK_CHAIN", "round_robin", "ROUND_ROBIN"} {
		p := &Policy{
			Version: "1.0",
			Name:    "test",
			Rules:   map[string]Rule{},
			Providers: &Providers{
				Pools: map[string]ProviderUpstreamPool{
					"openai": {
						Strategy: strat,
						Endpoints: []ProviderUpstreamEndpoint{
							{Name: "ep1", BaseURL: "https://api.openai.com"},
						},
					},
				},
			},
		}
		issues := ValidateSemantics(p)
		for _, i := range issues {
			if i.Rule == "enum" && strings.Contains(i.Field, "strategy") {
				t.Errorf("strategy %q should be valid, got warning: %+v", strat, i)
			}
		}
	}
}

// --- RBAC Exception Validation Tests ---

func TestValidateSemantics_ExceptionUnknownRole(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{"superhero"}, Reason: "test"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for unknown role in exception")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "unknown_role" && i.Field == "exceptions[0].roles[0]" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unknown_role error, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionMissingRuleReference(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		Exceptions: []Exception{
			{Rule: "nonexistent_rule", Roles: []string{"admin"}, Reason: "test"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for missing rule reference in exception")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "unknown_rule" && i.Field == "exceptions[0].rule" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unknown_rule error, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionEmptyRuleName(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "", Roles: []string{"admin"}, Reason: "test"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty rule name in exception")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "required" && i.Field == "exceptions[0].rule" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected required error for empty rule, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionEmptyRoles(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{}, Reason: "test"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty roles list in exception")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "required" && i.Field == "exceptions[0].roles" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected required error for empty roles, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionEmptyRoleName(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{""}, Reason: "test"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for empty role name in exception")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "required" && i.Field == "exceptions[0].roles[0]" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected required error for empty role name, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionInvalidExpiresAt(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{"admin"}, Reason: "test", ExpiresAt: "not-a-date"},
		},
	}
	issues := ValidateSemantics(p)
	if !HasValidationErrors(issues) {
		t.Fatal("expected error for invalid expires_at in exception")
	}
	found := false
	for _, i := range issues {
		if i.Rule == "rfc3339" && i.Field == "exceptions[0].expires_at" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected rfc3339 error for invalid expires_at, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionValidExpiresAt(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{"admin"}, Reason: "test", ExpiresAt: "2027-12-31T00:00:00Z"},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("valid exception with expires_at should have no errors, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionValidAllRoles(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{"admin", "analyst", "viewer", "security_lead"}, Reason: "all built-in roles"},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("valid exception with all built-in roles should have no errors, got: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionRuleReferenceWithSuffix(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii", Roles: []string{"admin"}, Reason: "bare type"},
		},
	}
	// "pii" without _detection suffix should be checked with suffix too.
	// Since we check ruleWithSuffix ("pii_detection") which exists, this should pass.
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("exception referencing bare 'pii' should resolve via _detection suffix, got errors: %+v", issues)
	}
}

func TestValidateSemantics_ExceptionValidNoErrors(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "valid-policy",
		Rules: map[string]Rule{
			"pii_detection":    {Action: "REDACT"},
			"secret_detection": {Action: "BLOCK"},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{"admin", "security_lead"}, Reason: "Admins inspecting production data need raw view", ExpiresAt: "2027-12-31T00:00:00Z"},
		},
	}
	issues := ValidateSemantics(p)
	if HasValidationErrors(issues) {
		t.Fatalf("valid exception should have no errors, got: %+v", issues)
	}
}
