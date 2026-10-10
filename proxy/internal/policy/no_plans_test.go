package policy

import (
	"strings"
	"testing"
)

// The "pricing" section selected a plan and its monthly cap. It is gone with
// the plans. A policy that still has it is refused, like any unknown key, and
// the error says why and where limits are set, so a cap somebody relied on
// does not stop applying silently.
func TestPricingSectionIsRefusedWithTheReason(t *testing.T) {
	_, err := LoadFromBytes([]byte("name: t\npricing:\n  active_tier: team\n  tiers:\n    - name: team\n      max_requests_per_month: 1000\n"))
	if err == nil {
		t.Fatal("a policy with a pricing section loaded")
	}
	for _, want := range []string{"pricing", "no plans or tiers", "rate_limit", "cost"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

// The limits a policy does set are untouched: they are the operator's own.
func TestPolicyLimitsStillLoad(t *testing.T) {
	p, err := LoadFromBytes([]byte("name: t\nrate_limit:\n  max_requests_per_minute: 10\ncost:\n  max_tokens_per_day: 1000\n  max_cost_usd_per_day: 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.RateLimit == nil || p.Cost == nil || p.Cost.MaxTokensPerDay != 1000 || p.Cost.MaxCostUSDPerDay != 5 {
		t.Fatalf("limits were not read: rate_limit=%+v cost=%+v", p.RateLimit, p.Cost)
	}
}
