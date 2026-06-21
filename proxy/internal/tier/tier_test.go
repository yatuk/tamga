package tier

import (
	"testing"

	"github.com/yatuk/tamga/internal/policy"
)

func TestEnforcer_NoPolicy(t *testing.T) {
	e := New(nil)
	e.Refresh()

	if e.SSOAllowed() {
		t.Error("SSO should not be allowed with nil policy")
	}
	if !e.CustomEntitiesAllowed() {
		t.Error("custom entities should be allowed by default")
	}
	if e.MaxRequestsPerMonth() != 0 {
		t.Errorf("expected 0 max requests, got %d", e.MaxRequestsPerMonth())
	}
	if e.TierName() != "community" {
		t.Errorf("expected 'community', got %q", e.TierName())
	}
}

func TestEnforcer_NilPricing(t *testing.T) {
	pol := &policy.Policy{Name: "test", Pricing: nil}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if e.SSOAllowed() {
		t.Error("SSO should not be allowed when pricing is nil")
	}
	if e.TierName() != "community" {
		t.Errorf("expected fallback 'community', got %q", e.TierName())
	}
}

func TestEnforcer_CommunityTier(t *testing.T) {
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "community",
			Tiers: []policy.PricingTier{
				{
					Name:            "community",
					MaxRequestsMo:   0, // unlimited for community self-hosted
					SSOEnabled:      false,
					CustomEntities:  true,
					AirGapped:       true,
					RetentionDays:   7,
					SupportSLAHours: 0,
				},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if e.SSOAllowed() {
		t.Error("community tier should not have SSO")
	}
	if !e.CustomEntitiesAllowed() {
		t.Error("community tier should allow custom entities")
	}
	if !e.AirGappedAllowed() {
		t.Error("community tier should allow air-gapped")
	}
	if e.TierName() != "community" {
		t.Errorf("expected 'community', got %q", e.TierName())
	}
}

func TestEnforcer_BusinessTier(t *testing.T) {
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "business",
			Tiers: []policy.PricingTier{
				{
					Name:           "community",
					SSOEnabled:     false,
					CustomEntities: true,
				},
				{
					Name:            "team",
					MaxRequestsMo:   10_000_000,
					SSOEnabled:      false,
					CustomEntities:  true,
					RetentionDays:   14,
					SupportSLAHours: 24,
				},
				{
					Name:            "business",
					MaxRequestsMo:   0, // unlimited
					SSOEnabled:      true,
					CustomEntities:  true,
					AirGapped:       false,
					RetentionDays:   90,
					SupportSLAHours: 4,
				},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if !e.SSOAllowed() {
		t.Error("business tier should have SSO")
	}
	if !e.CustomEntitiesAllowed() {
		t.Error("business tier should allow custom entities")
	}
	if e.AirGappedAllowed() {
		t.Error("business tier should not allow air-gapped")
	}
	if e.MaxRequestsPerMonth() != 0 {
		t.Errorf("expected unlimited (0) requests, got %d", e.MaxRequestsPerMonth())
	}
	if e.TierName() != "business" {
		t.Errorf("expected 'business', got %q", e.TierName())
	}
}

func TestEnforcer_TeamTierWithLimit(t *testing.T) {
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "team",
			Tiers: []policy.PricingTier{
				{
					Name:          "team",
					MaxRequestsMo: 10_000_000,
				},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if e.MaxRequestsPerMonth() != 10_000_000 {
		t.Errorf("expected 10M max requests, got %d", e.MaxRequestsPerMonth())
	}
}

func TestEnforcer_HotReload(t *testing.T) {
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "community",
			Tiers: []policy.PricingTier{
				{Name: "community", SSOEnabled: false},
				{Name: "business", SSOEnabled: true},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if e.SSOAllowed() {
		t.Error("community should not have SSO before reload")
	}

	// Hot-reload: switch to business
	pol.Pricing.ActiveTier = "business"
	e.Refresh()

	if !e.SSOAllowed() {
		t.Error("business should have SSO after reload")
	}
	if e.TierName() != "business" {
		t.Errorf("expected 'business', got %q", e.TierName())
	}
}

func TestEnforcer_UnknownTier(t *testing.T) {
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "enterprise",
			Tiers: []policy.PricingTier{
				{Name: "community", SSOEnabled: false},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if e.Active() != nil {
		t.Error("unknown tier should return nil active")
	}
	// Should fall back to safe defaults
	if e.SSOAllowed() {
		t.Error("unknown tier should not have SSO")
	}
}

func TestEnforcer_RecordRequest(t *testing.T) {
	e := New(func() *policy.Policy { return nil })
	e.Refresh()

	if e.MonthlyRequests() != 0 {
		t.Errorf("expected 0, got %d", e.MonthlyRequests())
	}

	e.RecordRequest()
	e.RecordRequest()
	e.RecordRequest()

	if e.MonthlyRequests() != 3 {
		t.Errorf("expected 3, got %d", e.MonthlyRequests())
	}
}

func TestEnforcer_CheckMonthlyLimit(t *testing.T) {
	// Unlimited tier (0 limit)
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "business",
			Tiers: []policy.PricingTier{
				{Name: "business", MaxRequestsMo: 0},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	for i := 0; i < 100; i++ {
		e.RecordRequest()
	}
	if e.CheckMonthlyLimit() {
		t.Error("unlimited tier should never hit limit")
	}

	// Limited tier (10 requests)
	pol2 := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "team",
			Tiers: []policy.PricingTier{
				{Name: "team", MaxRequestsMo: 10},
			},
		},
	}
	e2 := New(func() *policy.Policy { return pol2 })
	e2.Refresh()

	for i := 0; i < 9; i++ {
		e2.RecordRequest()
	}
	if e2.CheckMonthlyLimit() {
		t.Error("should not be at limit after 9 requests")
	}

	e2.RecordRequest()
	if !e2.CheckMonthlyLimit() {
		t.Error("should be at limit after 10 requests")
	}
}

func TestEnforcer_AirGappedAllowed(t *testing.T) {
	pol := &policy.Policy{
		Name: "test",
		Pricing: &policy.Pricing{
			ActiveTier: "community",
			Tiers: []policy.PricingTier{
				{Name: "community", AirGapped: true},
			},
		},
	}
	e := New(func() *policy.Policy { return pol })
	e.Refresh()

	if !e.AirGappedAllowed() {
		t.Error("community tier should allow air-gapped")
	}

	// Business without air-gapped
	pol.Pricing.ActiveTier = "" // triggers nil active
	e.Refresh()
	if !e.AirGappedAllowed() {
		t.Error("nil tier should allow air-gapped (community default)")
	}
}
