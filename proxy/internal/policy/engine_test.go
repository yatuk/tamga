package policy

import (
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

func TestLoadFromBytesYAML(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test-policy
rules:
  pii_detection:
    action: REDACT
    sensitivity: medium
    types: [email]
providers:
  allowed: [openai]
  blocked: [blocked_provider]
rate_limit:
  max_requests_per_minute: 10
  action_on_exceed: BLOCK
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "test-policy" {
		t.Fatalf("name: got %q", p.Name)
	}
	if p.Rules["pii_detection"].Action != ActionRedact {
		t.Fatalf("rule action: %v", p.Rules["pii_detection"].Action)
	}
	if p.RateLimit.MaxRequestsPerMinute != 10 {
		t.Fatalf("rpm: %d", p.RateLimit.MaxRequestsPerMinute)
	}
	if !p.ProviderAllowed("openai") {
		t.Fatal("openai should be allowed")
	}
	if p.ProviderAllowed("blocked_provider") {
		t.Fatal("blocked_provider should be denied")
	}
}

func TestEvaluateRulePriority_BlockOverRedact(t *testing.T) {
	raw := []byte(`
version: "1.0"
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
    sensitivity: low
    types: [github_token]
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "github_token", Severity: "critical"},
	}
	if got := p.Evaluate(fs); got != ActionBlock {
		t.Fatalf("Evaluate: got %v want BLOCK", got)
	}
}

func TestEvaluateRulePriority_RedactOverWarn(t *testing.T) {
	raw := []byte(`
version: "1.0"
rules:
  pii_detection:
    action: WARN
    sensitivity: low
    types: [email]
  secret_detection:
    action: REDACT
    sensitivity: low
    types: [github_token]
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "github_token", Severity: "critical"},
	}
	if got := p.Evaluate(fs); got != ActionRedact {
		t.Fatalf("Evaluate: got %v want REDACT", got)
	}
}

func TestEvaluateRulePriority_WarnOverLog(t *testing.T) {
	raw := []byte(`
version: "1.0"
rules:
  pii_detection:
    action: LOG
    sensitivity: low
    types: [email]
  secret_detection:
    action: WARN
    sensitivity: low
    types: [github_token]
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "github_token", Severity: "critical"},
	}
	if got := p.Evaluate(fs); got != ActionWarn {
		t.Fatalf("Evaluate: got %v want WARN", got)
	}
}

func TestProviderAllowlistEmptyMeansAllow(t *testing.T) {
	raw := []byte(`version: "1.0"`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !p.ProviderAllowed("anything") {
		t.Fatal("empty allowlist should allow all (unless blocked)")
	}
}

func TestEvaluateCustomEntity(t *testing.T) {
	raw := []byte(`
version: "1.0"
custom_entities:
  - name: "musteri_no"
    action: WARN
    severity: high
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{{Type: "custom", Category: "musteri_no", Severity: "high"}}
	if got := p.Evaluate(fs); got != ActionWarn {
		t.Fatalf("Evaluate: got %v want WARN", got)
	}
	rule, ok := p.MatchedRule(fs[0])
	if !ok || rule.Action != ActionWarn {
		t.Fatalf("MatchedRule: ok=%v rule=%+v", ok, rule)
	}
}

func TestEvaluateConfidenceBasedRule(t *testing.T) {
	raw := []byte(`
version: "1.0"
rules:
  pii_detection:
    mode: confidence_based
    minimum_confidence: 70
    types: [credit_card]
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{
			Type:     "pii",
			Category: "credit_card",
			ConfidenceScore: &scanner.ConfidenceScore{
				Total:  80,
				Action: scanner.ActionRedact,
			},
		},
	}
	if got := p.Evaluate(fs); got != ActionRedact {
		t.Fatalf("Evaluate: got %v want REDACT", got)
	}
}

func TestEvaluateConfidenceBasedRuleBelowMinimum(t *testing.T) {
	raw := []byte(`
version: "1.0"
rules:
  pii_detection:
    mode: confidence_based
    minimum_confidence: 70
    types: [credit_card]
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{
			Type:     "pii",
			Category: "credit_card",
			ConfidenceScore: &scanner.ConfidenceScore{
				Total:  60,
				Action: scanner.ActionPassLog,
			},
		},
	}
	if got := p.Evaluate(fs); got != ActionPass {
		t.Fatalf("Evaluate: got %v want PASS", got)
	}
}

func TestResolveMaxBodyBytes(t *testing.T) {
	t.Run("policy per_provider", func(t *testing.T) {
		raw := []byte(`
version: "1.0"
body_limits:
  default:
    max_bytes: 100
  per_provider:
    openai:
      max_bytes: 2048
providers:
  allowed: [openai]
`)
		p, err := LoadFromBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.ResolveMaxBodyBytes("openai", 999999); got != 2048 {
			t.Fatalf("openai: got %d want 2048", got)
		}
		if got := p.ResolveMaxBodyBytes("anthropic", 500); got != 100 {
			t.Fatalf("anthropic falls back to default: got %d want 100", got)
		}
	})
	t.Run("config default when policy empty", func(t *testing.T) {
		raw := []byte(`
version: "1.0"
providers:
  allowed: [openai]
`)
		p, err := LoadFromBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.ResolveMaxBodyBytes("openai", 512000); got != 512000 {
			t.Fatalf("got %d want 512000", got)
		}
	})
	t.Run("implicit 1MiB", func(t *testing.T) {
		p := &Policy{}
		if got := p.ResolveMaxBodyBytes("openai", 0); got != 1048576 {
			t.Fatalf("got %d want 1048576", got)
		}
	})
}

func TestResolveMaxBodyBytes_NilPolicy(t *testing.T) {
	var p *Policy
	if got := p.ResolveMaxBodyBytes("openai", 512000); got != 512000 {
		t.Fatalf("nil policy: got %d want configDefault 512000", got)
	}
	if got := p.ResolveMaxBodyBytes("openai", 0); got != 1048576 {
		t.Fatalf("nil policy with zero default: got %d want implicit 1MiB", got)
	}
	if got := p.ResolveMaxBodyBytes("openai", -1); got != 1048576 {
		t.Fatalf("nil policy with negative default: got %d want implicit 1MiB", got)
	}
}

func TestResolveMaxBodyBytes_NilBodyLimits(t *testing.T) {
	p := &Policy{
		Version:    "1.0",
		Name:       "test",
		Rules:      map[string]Rule{},
		BodyLimits: nil,
	}
	if got := p.ResolveMaxBodyBytes("openai", 2048); got != 2048 {
		t.Fatalf("nil body_limits: got %d want configDefault 2048", got)
	}
	if got := p.ResolveMaxBodyBytes("openai", 0); got != 1048576 {
		t.Fatalf("nil body_limits with zero default: got %d want 1048576", got)
	}
}

func TestResolveMaxBodyBytes_EmptyPerProvider(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default:     BodyLimitRule{MaxBytes: 8192},
			PerProvider: map[string]BodyLimitRule{},
		},
	}
	if got := p.ResolveMaxBodyBytes("openai", 0); got != 8192 {
		t.Fatalf("empty per_provider should fall back to default: got %d want 8192", got)
	}
}

func TestResolveMaxBodyBytes_ZeroDefaultZeroPerProvider(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules:   map[string]Rule{},
		BodyLimits: &BodyLimitsConfig{
			Default:     BodyLimitRule{MaxBytes: 0},
			PerProvider: map[string]BodyLimitRule{},
		},
	}
	if got := p.ResolveMaxBodyBytes("openai", 0); got != 1048576 {
		t.Fatalf("zero default + empty per_provider: got %d want implicit 1MiB", got)
	}
}

// --- EvaluateOutput tests ---

func TestEvaluateOutput_NilPolicy(t *testing.T) {
	var p *Policy
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Severity: "high"},
	}); got != ActionPass {
		t.Fatalf("nil policy should return PASS, got %v", got)
	}
}

func TestEvaluateOutput_OutputRulesNil(t *testing.T) {
	p := &Policy{Version: "1.0", Name: "test"}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Severity: "high"},
	}); got != ActionPass {
		t.Fatalf("nil output_rules should return PASS, got %v", got)
	}
}

func TestEvaluateOutput_OutputRulesDisabled(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled: false,
			BlockOn: []string{"pii"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Severity: "high"},
	}); got != ActionPass {
		t.Fatalf("disabled output_rules should return PASS, got %v", got)
	}
}

func TestEvaluateOutput_EmptyFindings(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled: true,
			BlockOn: []string{"pii"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{}); got != ActionPass {
		t.Fatalf("empty findings should return PASS, got %v", got)
	}
	if got := p.EvaluateOutput(nil); got != ActionPass {
		t.Fatalf("nil findings should return PASS, got %v", got)
	}
}

func TestEvaluateOutput_BlockOnMatch(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled: true,
			BlockOn: []string{"pii"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}); got != ActionBlock {
		t.Fatalf("pii should be blocked, got %v", got)
	}
}

func TestEvaluateOutput_RedactOnMatch(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled:  true,
			RedactOn: []string{"pii"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}); got != ActionRedact {
		t.Fatalf("pii should be redacted, got %v", got)
	}
}

func TestEvaluateOutput_BlockOverRedact(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled:  true,
			BlockOn:  []string{"secret"},
			RedactOn: []string{"pii"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "github_token", Severity: "critical"},
	}); got != ActionBlock {
		t.Fatalf("block should win over redact, got %v", got)
	}
}

func TestEvaluateOutput_NoMatchingRules(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled:  true,
			BlockOn:  []string{"secret"},
			RedactOn: []string{"secret"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}); got != ActionPass {
		t.Fatalf("unmatched type should return PASS, got %v", got)
	}
}

func TestEvaluateOutput_CategoryMatch(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled: true,
			BlockOn: []string{"credit_card"},
		},
	}
	if got := p.EvaluateOutput([]scanner.Finding{
		{Type: "pii", Category: "credit_card", Severity: "high"},
	}); got != ActionBlock {
		t.Fatalf("category match should trigger block, got %v", got)
	}
}

func TestEvaluateOutput_ConfidenceFiltering(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		OutputRules: &OutputRules{
			Enabled:           true,
			BlockOn:           []string{"pii"},
			MinimumConfidence: 80,
		},
	}
	t.Run("below threshold", func(t *testing.T) {
		if got := p.EvaluateOutput([]scanner.Finding{
			{Type: "pii", Category: "email", Severity: "high", Confidence: 0.5},
		}); got != ActionPass {
			t.Fatalf("low confidence should be filtered, got %v", got)
		}
	})
	t.Run("above threshold", func(t *testing.T) {
		if got := p.EvaluateOutput([]scanner.Finding{
			{Type: "pii", Category: "email", Severity: "high", Confidence: 0.95},
		}); got != ActionBlock {
			t.Fatalf("high confidence should trigger block, got %v", got)
		}
	})
	t.Run("with ConfidenceScore", func(t *testing.T) {
		if got := p.EvaluateOutput([]scanner.Finding{
			{
				Type:            "pii",
				Category:        "email",
				Severity:        "high",
				ConfidenceScore: &scanner.ConfidenceScore{Total: 85},
			},
		}); got != ActionBlock {
			t.Fatalf("high confidence score should trigger block, got %v", got)
		}
	})
}

// --- CustomEntityByName tests ---

func TestCustomEntityByName_NilPolicy(t *testing.T) {
	var p *Policy
	if ce := p.CustomEntityByName("test-entity"); ce != nil {
		t.Fatalf("nil policy should return nil, got %+v", ce)
	}
}

func TestCustomEntityByName_NotFound(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "entity1", Pattern: `\d+`, Action: "BLOCK", Severity: "high"},
		},
	}
	if ce := p.CustomEntityByName("nonexistent"); ce != nil {
		t.Fatalf("unknown name should return nil, got %+v", ce)
	}
}

func TestCustomEntityByName_CaseInsensitive(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "MyEntity", Pattern: `\d+`, Action: "BLOCK", Severity: "high"},
		},
	}
	if ce := p.CustomEntityByName("myentity"); ce == nil {
		t.Fatal("case-insensitive lookup should find entity")
	} else if ce.Name != "MyEntity" {
		t.Fatalf("expected Name 'MyEntity', got %q", ce.Name)
	}
}

func TestCustomEntityByName_EmptyName(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "", Pattern: `\d+`, Action: "BLOCK", Severity: "high"},
		},
	}
	// NOTE: strings.EqualFold("", "") returns true, so empty name lookup matches an entity with an empty name.
	if ce := p.CustomEntityByName(""); ce != nil {
		if ce.Name != "" {
			t.Fatalf("expected empty name entity, got %q", ce.Name)
		}
	}
}

// --- WebhookURLsForFindings tests ---

func TestWebhookURLsForFindings_EmptyFindings(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionWarn, Notify: []Notify{{Webhook: "https://hooks.example.com"}}},
		},
	}
	urls := p.WebhookURLsForFindings(nil, ActionWarn)
	if len(urls) != 0 {
		t.Fatalf("nil findings should return empty, got %d urls", len(urls))
	}
	urls = p.WebhookURLsForFindings([]scanner.Finding{}, ActionWarn)
	if len(urls) != 0 {
		t.Fatalf("empty findings should return empty, got %d urls", len(urls))
	}
}

func TestWebhookURLsForFindings_NoMatch(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionWarn, Notify: []Notify{{Webhook: "https://hooks.example.com"}}},
		},
	}
	urls := p.WebhookURLsForFindings([]scanner.Finding{
		{Type: "secret", Category: "github_token", Severity: "high"},
	}, ActionWarn)
	if len(urls) != 0 {
		t.Fatalf("unmatched type should return empty, got %d urls", len(urls))
	}
}

func TestWebhookURLsForFindings_WrongAction(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionBlock, Notify: []Notify{{Webhook: "https://hooks.example.com"}}},
		},
	}
	urls := p.WebhookURLsForFindings([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}, ActionWarn)
	if len(urls) != 0 {
		t.Fatalf("wrong action filter should return empty, got %d urls", len(urls))
	}
}

func TestWebhookURLsForFindings_MatchingWebhooks(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {
				Action: ActionWarn,
				Notify: []Notify{
					{Webhook: "https://hooks.example.com/pii"},
					{Webhook: "https://hooks.example.com/shared"},
				},
			},
		},
	}
	urls := p.WebhookURLsForFindings([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}, ActionWarn)
	if len(urls) != 2 {
		t.Fatalf("expected 2 webhooks, got %d: %v", len(urls), urls)
	}
}

func TestWebhookURLsForFindings_Deduplicates(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {
				Action: ActionWarn,
				Notify: []Notify{{Webhook: "https://hooks.example.com/shared"}},
			},
			"secret_detection": {
				Action: ActionWarn,
				Notify: []Notify{{Webhook: "https://hooks.example.com/shared"}},
			},
		},
	}
	urls := p.WebhookURLsForFindings([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "aws_key", Severity: "high"},
	}, ActionWarn)
	if len(urls) != 1 {
		t.Fatalf("expected 1 deduplicated webhook, got %d: %v", len(urls), urls)
	}
}

func TestWebhookURLsForFindings_EmptyWebhookURL(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {
				Action: ActionWarn,
				Notify: []Notify{{Channel: "slack", Webhook: ""}},
			},
		},
	}
	urls := p.WebhookURLsForFindings([]scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}, ActionWarn)
	if len(urls) != 0 {
		t.Fatalf("empty webhook should be skipped, got %d urls", len(urls))
	}
}

// --- MatchedRule additional tests ---

func TestMatchedRule_UnknownType(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionRedact},
		},
	}
	_, ok := p.MatchedRule(scanner.Finding{Type: "unknown_type", Category: "x"})
	if ok {
		t.Fatal("unknown type should not match any rule")
	}
}

func TestMatchedRule_CustomCategoryNotFound(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		CustomEntities: []CustomEntity{
			{Name: "known", Pattern: `\d+`, Action: "BLOCK", Severity: "high"},
		},
	}
	_, ok := p.MatchedRule(scanner.Finding{Type: "custom", Category: "unknown"})
	if ok {
		t.Fatal("unknown custom category should not match")
	}
}

func TestMatchedRule_TypeFiltering(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionBlock, Types: []string{"credit_card"}},
		},
	}
	_, ok := p.MatchedRule(scanner.Finding{Type: "pii", Category: "email", Severity: "high"})
	if ok {
		t.Fatal("email category should not match credit_card-only rule")
	}
	rule, ok := p.MatchedRule(scanner.Finding{Type: "pii", Category: "credit_card", Severity: "high"})
	if !ok || rule.Action != ActionBlock {
		t.Fatalf("credit_card should match, got ok=%v rule=%+v", ok, rule)
	}
}

func TestMatchedRule_SensitivityThreshold(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionBlock, Sensitivity: "high"},
		},
	}
	_, ok := p.MatchedRule(scanner.Finding{Type: "pii", Category: "email", Severity: "low"})
	if ok {
		t.Fatal("low severity should not match high sensitivity rule")
	}
	rule, ok := p.MatchedRule(scanner.Finding{Type: "pii", Category: "email", Severity: "high"})
	if !ok || rule.Action != ActionBlock {
		t.Fatalf("high severity should match high sensitivity rule, got ok=%v rule=%+v", ok, rule)
	}
}

func TestMatchedRule_NoSuffixKey(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"secret": {Action: ActionBlock},
		},
	}
	rule, ok := p.MatchedRule(scanner.Finding{Type: "secret", Category: "aws_key", Severity: "high"})
	if !ok || rule.Action != ActionBlock {
		t.Fatalf("bare key 'secret' should match type 'secret', got ok=%v rule=%+v", ok, rule)
	}
}

// --- RBAC Exception Framework Tests ---

func TestExceptionBypassesBlock(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
  secret_detection:
    action: BLOCK
    sensitivity: low
    types: [github_token]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admins inspecting production data need raw view"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// Without exception, BLOCK would fire.
	if got := p.Evaluate(fs); got != ActionBlock {
		t.Fatalf("Evaluate without exception: got %v want BLOCK", got)
	}
	// With admin role, pii rule should be bypassed (no findings match secret rule).
	action, matches := p.EvaluateWithRole(fs, "admin", false)
	if action != ActionPass {
		t.Fatalf("EvaluateWithRole: got %v want PASS (exception bypasses BLOCK)", action)
	}
	if len(matches) != 1 || matches[0].Rule != "pii_detection" {
		t.Fatalf("expected pii_detection exception match, got %+v", matches)
	}
	if matches[0].Role != "admin" {
		t.Fatalf("expected role admin in match, got %q", matches[0].Role)
	}
}

func TestExceptionBypassesRedact(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: REDACT
    sensitivity: low
    types: [email]
exceptions:
  - rule: "pii_detection"
    roles: ["security_lead"]
    reason: "Security staff auditing raw PII content"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// Without exception, REDACT would fire.
	if got := p.Evaluate(fs); got != ActionRedact {
		t.Fatalf("Evaluate without exception: got %v want REDACT", got)
	}
	// With security_lead role, pii rule should be bypassed → PASS.
	action, matches := p.EvaluateWithRole(fs, "security_lead", false)
	if action != ActionPass {
		t.Fatalf("EvaluateWithRole: got %v want PASS (exception bypasses REDACT)", action)
	}
	if len(matches) != 1 || matches[0].Rule != "pii_detection" {
		t.Fatalf("expected pii_detection exception match, got %+v", matches)
	}
}

func TestExpiredExceptionIsIgnored(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Historical exception"
    expires_at: "2020-01-01T00:00:00Z"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// Expired exception should not apply → BLOCK still fires.
	action, matches := p.EvaluateWithRole(fs, "admin", false)
	if action != ActionBlock {
		t.Fatalf("EvaluateWithRole with expired exception: got %v want BLOCK", action)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no exception matches for expired exception, got %+v", matches)
	}
}

func TestStrictModeDisablesAllExceptions(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admins inspecting production data need raw view"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// strictMode=true should disable exceptions → BLOCK still fires.
	action, matches := p.EvaluateWithRole(fs, "admin", true)
	if action != ActionBlock {
		t.Fatalf("EvaluateWithRole with strictMode: got %v want BLOCK", action)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no exception matches in strictMode, got %+v", matches)
	}
}

func TestExceptionMultipleRules(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email, credit_card]
  secret_detection:
    action: REDACT
    sensitivity: high
    types: [github_token]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admin PII bypass"
  - rule: "secret_detection"
    roles: ["admin"]
    reason: "Admin secret bypass"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "github_token", Severity: "critical"},
	}
	// Both rules bypassed → PASS.
	action, matches := p.EvaluateWithRole(fs, "admin", false)
	if action != ActionPass {
		t.Fatalf("EvaluateWithRole for multiple exceptions: got %v want PASS", action)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 exception matches, got %d: %+v", len(matches), matches)
	}
}

func TestExceptionPermanentNoExpiresAt(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Permanent admin bypass"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// No expires_at means permanent exception → bypass works.
	action, matches := p.EvaluateWithRole(fs, "admin", false)
	if action != ActionPass {
		t.Fatalf("EvaluateWithRole for permanent exception: got %v want PASS", action)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 exception match, got %d: %+v", len(matches), matches)
	}
}

func TestExceptionRoleNotMatching(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admin bypass only"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// Viewer role does not have an exception → BLOCK fires.
	action, matches := p.EvaluateWithRole(fs, "viewer", false)
	if action != ActionBlock {
		t.Fatalf("EvaluateWithRole for non-matching role: got %v want BLOCK", action)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no matches for non-matching role, got %+v", matches)
	}
}

func TestExceptionEmptyRole(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admin bypass"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
	}
	// Empty role → no exception applies → BLOCK fires.
	action, matches := p.EvaluateWithRole(fs, "", false)
	if action != ActionBlock {
		t.Fatalf("EvaluateWithRole with empty role: got %v want BLOCK", action)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no matches for empty role, got %+v", matches)
	}
}

func TestIsRuleExempted_CaseInsensitive(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionBlock},
		},
		Exceptions: []Exception{
			{Rule: "pii_detection", Roles: []string{"Admin"}, Reason: "test"},
		},
	}
	exempted, match := p.IsRuleExempted("pii_detection", "admin")
	if !exempted {
		t.Fatal("case-insensitive role admin should match exception")
	}
	if match.Role != "admin" {
		t.Fatalf("role should be normalized to lowercase, got %q", match.Role)
	}
	exempted, _ = p.IsRuleExempted("PII_DETECTION", "ADMIN")
	if !exempted {
		t.Fatal("case-insensitive rule name should match exception")
	}
}

func TestIsRuleExempted_NilPolicy(t *testing.T) {
	var p *Policy
	exempted, _ := p.IsRuleExempted("pii_detection", "admin")
	if exempted {
		t.Fatal("nil policy should not have exceptions")
	}
}

func TestIsRuleExempted_NoExceptions(t *testing.T) {
	p := &Policy{
		Version: "1.0",
		Name:    "test",
		Rules: map[string]Rule{
			"pii_detection": {Action: ActionBlock},
		},
	}
	exempted, _ := p.IsRuleExempted("pii_detection", "admin")
	if exempted {
		t.Fatal("no exceptions configured, should not be exempted")
	}
}

func TestEvaluateWithRole_NonExemptedRuleStillApplies(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email]
  secret_detection:
    action: REDACT
    sensitivity: low
    types: [github_token]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admin PII bypass"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "secret", Category: "github_token", Severity: "critical"},
	}
	// pii is exempted but secret is not → REDACT should fire (from secret rule).
	action, matches := p.EvaluateWithRole(fs, "admin", false)
	if action != ActionRedact {
		t.Fatalf("EvaluateWithRole for partially exempted: got %v want REDACT (secret still applies)", action)
	}
	if len(matches) != 1 || matches[0].Rule != "pii_detection" {
		t.Fatalf("expected pii_detection exception only, got %+v", matches)
	}
}

func TestEvaluateWithRole_DeduplicatesExceptionMatches(t *testing.T) {
	raw := []byte(`
version: "1.0"
name: test
rules:
  pii_detection:
    action: BLOCK
    sensitivity: low
    types: [email, credit_card]
exceptions:
  - rule: "pii_detection"
    roles: ["admin"]
    reason: "Admin PII bypass"
`)
	p, err := LoadFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Two findings both match the same pii rule → exception should be reported once.
	fs := []scanner.Finding{
		{Type: "pii", Category: "email", Severity: "high"},
		{Type: "pii", Category: "credit_card", Severity: "high"},
	}
	action, matches := p.EvaluateWithRole(fs, "admin", false)
	if action != ActionPass {
		t.Fatalf("EvaluateWithRole: got %v want PASS", action)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 deduplicated exception match, got %d: %+v", len(matches), matches)
	}
}
