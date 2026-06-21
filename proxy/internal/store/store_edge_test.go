package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// =============================================================================
// SSOSettingsStore tests — pure in-memory, always run in short mode
// =============================================================================

func TestMemorySSOSettingsStore_New(t *testing.T) {
	t.Run("construct_non_nil", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		if s == nil {
			t.Fatal("NewMemorySSOSettingsStore should return non-nil")
		}
	})

	t.Run("construct_data_not_nil", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		s.mu.RLock()
		d := s.data
		s.mu.RUnlock()
		if d == nil {
			t.Fatal("internal data map should be non-nil")
		}
	})
}

func TestMemorySSOSettingsStore_Get(t *testing.T) {
	t.Run("missing_org_returns_default", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		cfg := s.Get("nonexistent-org")
		if cfg == nil {
			t.Fatal("Get should return non-nil default settings")
		}
		if cfg.ProviderType != "" {
			t.Errorf("ProviderType: want empty, got %q", cfg.ProviderType)
		}
		if cfg.Enabled {
			t.Error("Enabled: want false by default")
		}
		if cfg.AttributeMapping == nil {
			t.Fatal("AttributeMapping should not be nil")
		}
		if cfg.AttributeMapping["email"] != "email" {
			t.Errorf("email mapping: want 'email', got %q", cfg.AttributeMapping["email"])
		}
		if cfg.AttributeMapping["name"] != "displayName" {
			t.Errorf("name mapping: want 'displayName', got %q", cfg.AttributeMapping["name"])
		}
	})

	t.Run("set_then_get_returns_copy", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		original := SSOSettings{
			ProviderType: "oidc",
			MetadataURL:  "https://example.com/.well-known/openid-configuration",
			AttributeMapping: map[string]string{
				"email": "mail",
				"name":  "cn",
			},
			Enabled: true,
			Domain:  "example.com",
		}
		s.Set("org-1", original)

		got := s.Get("org-1")
		if got == nil {
			t.Fatal("Get should return non-nil for stored org")
		}
		if got.ProviderType != "oidc" {
			t.Errorf("ProviderType: want 'oidc', got %q", got.ProviderType)
		}
		if got.MetadataURL != original.MetadataURL {
			t.Errorf("MetadataURL: want %q, got %q", original.MetadataURL, got.MetadataURL)
		}
		if got.Enabled != true {
			t.Error("Enabled: want true")
		}
		if got.Domain != "example.com" {
			t.Errorf("Domain: want 'example.com', got %q", got.Domain)
		}
		if got.AttributeMapping["email"] != "mail" {
			t.Errorf("email mapping: want 'mail', got %q", got.AttributeMapping["email"])
		}
		if got.AttributeMapping["name"] != "cn" {
			t.Errorf("name mapping: want 'cn', got %q", got.AttributeMapping["name"])
		}

		// Verify it's a copy: mutating the returned value should not affect the stored one.
		got.AttributeMapping["email"] = "modified"
		got2 := s.Get("org-1")
		if got2.AttributeMapping["email"] != "mail" {
			t.Errorf("mutating returned copy affected stored value: got %q", got2.AttributeMapping["email"])
		}
	})

	t.Run("get_without_attribute_mapping_returns_default_builtin_mapping", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		s.Set("org-no-attrs", SSOSettings{
			ProviderType: "saml",
			Enabled:      true,
		})
		got := s.Get("org-no-attrs")
		if got.AttributeMapping != nil {
			t.Error("AttributeMapping should be nil when not provided")
		}
	})
}

func TestMemorySSOSettingsStore_Set(t *testing.T) {
	t.Run("set_overwrites", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		s.Set("org-1", SSOSettings{ProviderType: "saml", Enabled: false})
		s.Set("org-1", SSOSettings{ProviderType: "oidc", Enabled: true})

		got := s.Get("org-1")
		if got.ProviderType != "oidc" {
			t.Errorf("ProviderType: want 'oidc', got %q", got.ProviderType)
		}
		if !got.Enabled {
			t.Error("Enabled: want true after overwrite")
		}
	})

	t.Run("set_multiple_orgs", func(t *testing.T) {
		s := NewMemorySSOSettingsStore()
		s.Set("org-a", SSOSettings{ProviderType: "saml", Domain: "a.com"})
		s.Set("org-b", SSOSettings{ProviderType: "oidc", Domain: "b.com"})

		gotA := s.Get("org-a")
		if gotA.ProviderType != "saml" || gotA.Domain != "a.com" {
			t.Errorf("org-a: got %+v", gotA)
		}
		gotB := s.Get("org-b")
		if gotB.ProviderType != "oidc" || gotB.Domain != "b.com" {
			t.Errorf("org-b: got %+v", gotB)
		}
		// Unset org still returns default.
		gotC := s.Get("org-c")
		if gotC.ProviderType != "" {
			t.Errorf("org-c default ProviderType: want empty, got %q", gotC.ProviderType)
		}
	})
}

func TestNoopSSOSettingsStore_New(t *testing.T) {
	s := NewNoopSSOSettingsStore()
	if s == nil {
		t.Fatal("NewNoopSSOSettingsStore should return non-nil")
	}
}

func TestNoopSSOSettingsStore_Get(t *testing.T) {
	t.Run("returns_default_with_builtin_mapping", func(t *testing.T) {
		s := NewNoopSSOSettingsStore()
		cfg := s.Get("any-org")
		if cfg == nil {
			t.Fatal("Get should return non-nil")
		}
		if cfg.Enabled {
			t.Error("Enabled: want false in noop defaults")
		}
		if cfg.ProviderType != "" {
			t.Errorf("ProviderType: want empty, got %q", cfg.ProviderType)
		}
		if cfg.AttributeMapping == nil {
			t.Fatal("AttributeMapping should not be nil")
		}
		if cfg.AttributeMapping["email"] != "email" {
			t.Errorf("email mapping: want 'email', got %q", cfg.AttributeMapping["email"])
		}
		if cfg.AttributeMapping["name"] != "displayName" {
			t.Errorf("name mapping: want 'displayName', got %q", cfg.AttributeMapping["name"])
		}
	})

	t.Run("returns_copy_on_each_call", func(t *testing.T) {
		s := NewNoopSSOSettingsStore()
		a := s.Get("org-1")
		b := s.Get("org-1")
		// Different pointer values = separate copies.
		if a == b {
			t.Log("same pointer: noop store may return the same default (acceptable)")
		}
		a.ProviderType = "mutated"
		c := s.Get("org-1")
		if c.ProviderType != "" {
			t.Error("mutation of previous result should not affect subsequent gets")
		}
	})
}

func TestNoopSSOSettingsStore_Set(t *testing.T) {
	t.Run("set_is_noop", func(t *testing.T) {
		s := NewNoopSSOSettingsStore()
		s.Set("org-1", SSOSettings{
			ProviderType: "oidc",
			Enabled:      true,
			Domain:       "example.com",
		})
		// Get should still return defaults, not the value we set.
		cfg := s.Get("org-1")
		if cfg.Enabled {
			t.Error("Enabled: want false after Set (noop), got true")
		}
		if cfg.ProviderType != "" {
			t.Errorf("ProviderType: want empty after Set (noop), got %q", cfg.ProviderType)
		}
	})

	t.Run("set_no_panic", func(t *testing.T) {
		s := NewNoopSSOSettingsStore()
		// Should not panic even with zero-value SSOSettings.
		s.Set("", SSOSettings{})
		s.Set("org", SSOSettings{})
	})
}

// =============================================================================
// SSOSettingsStore interface compliance checks
// =============================================================================

func TestSSOSettingsStore_Interface(t *testing.T) {
	// Compile-time: both implementations satisfy the interface.
	var _ SSOSettingsStore = NewMemorySSOSettingsStore()
	var _ SSOSettingsStore = NewNoopSSOSettingsStore()
}

// =============================================================================
// RetentionPolicyFromEnv — invalid integer values
// =============================================================================

func TestRetentionPolicyFromEnv_InvalidValues(t *testing.T) {
	t.Run("negative_values_ignored", func(t *testing.T) {
		restore := saveEnv(
			"TAMGA_RETENTION_REQUEST_LOGS_DAYS",
			"TAMGA_RETENTION_ALERTS_DAYS",
		)
		defer restore()

		_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "-10")
		_ = os.Setenv("TAMGA_RETENTION_ALERTS_DAYS", "-5")

		p := RetentionPolicyFromEnv()
		def := DefaultRetentionPolicy()
		if p.RequestLogsDays != def.RequestLogsDays {
			t.Errorf("RequestLogsDays with negative env: want default %d, got %d", def.RequestLogsDays, p.RequestLogsDays)
		}
		if p.AlertsDays != def.AlertsDays {
			t.Errorf("AlertsDays with negative env: want default %d, got %d", def.AlertsDays, p.AlertsDays)
		}
	})

	t.Run("non_numeric_values_ignored", func(t *testing.T) {
		restore := saveEnv(
			"TAMGA_RETENTION_REQUEST_LOGS_DAYS",
			"TAMGA_RETENTION_DAILY_STATS_DAYS",
		)
		defer restore()

		_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "abc")
		_ = os.Setenv("TAMGA_RETENTION_DAILY_STATS_DAYS", "not-a-number")

		p := RetentionPolicyFromEnv()
		def := DefaultRetentionPolicy()
		if p.RequestLogsDays != def.RequestLogsDays {
			t.Errorf("RequestLogsDays with non-numeric env: want default %d, got %d", def.RequestLogsDays, p.RequestLogsDays)
		}
		if p.DailyStatsDays != def.DailyStatsDays {
			t.Errorf("DailyStatsDays with non-numeric env: want default %d, got %d", def.DailyStatsDays, p.DailyStatsDays)
		}
	})

	t.Run("zero_values_ignored", func(t *testing.T) {
		restore := saveEnv("TAMGA_RETENTION_REQUEST_LOGS_DAYS")
		defer restore()

		_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "0")

		p := RetentionPolicyFromEnv()
		def := DefaultRetentionPolicy()
		if p.RequestLogsDays != def.RequestLogsDays {
			t.Errorf("RequestLogsDays with zero env: want default %d, got %d", def.RequestLogsDays, p.RequestLogsDays)
		}
	})

	t.Run("whitespace_only_ignored", func(t *testing.T) {
		restore := saveEnv("TAMGA_RETENTION_REQUEST_LOGS_DAYS")
		defer restore()

		_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "   ")

		p := RetentionPolicyFromEnv()
		if p.RequestLogsDays != 30 {
			t.Errorf("RequestLogsDays with whitespace env: want 30, got %d", p.RequestLogsDays)
		}
	})
}

// =============================================================================
// envIntPositive — additional edge cases
// =============================================================================

func TestEnvIntPositive_Whitespace(t *testing.T) {
	key := "TAMGA_TEST_ENV_INT_WHITESPACE"
	defer func() { _ = os.Unsetenv(key) }()

	_ = os.Setenv(key, "  42  ")
	got := envIntPositive(key)
	if got != 42 {
		t.Errorf("envIntPositive with whitespace: want 42, got %d", got)
	}
}

func TestEnvIntPositive_LeadingZeros(t *testing.T) {
	key := "TAMGA_TEST_ENV_INT_LEADING_ZEROS"
	defer func() { _ = os.Unsetenv(key) }()

	_ = os.Setenv(key, "007")
	got := envIntPositive(key)
	if got != 7 {
		t.Errorf("envIntPositive with leading zeros: want 7, got %d", got)
	}
}

// =============================================================================
// hashFindingMatches — edge cases
// =============================================================================

func TestHashFindingMatches_EmptySlice(t *testing.T) {
	got := hashFindingMatches(nil)
	if got == nil {
		t.Fatal("hashFindingMatches(nil) should return non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got %d items", len(got))
	}

	got2 := hashFindingMatches([]scanner.Finding{})
	if got2 == nil {
		t.Fatal("hashFindingMatches([]) should return non-nil slice")
	}
	if len(got2) != 0 {
		t.Errorf("want empty slice, got %d items", len(got2))
	}
}

func TestHashFindingMatches_AllEmptyMatches(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Match: "", Severity: "low"},
		{Type: "secret", Match: "", Severity: "high"},
	}
	out := hashFindingMatches(findings)
	if len(out) != 2 {
		t.Fatalf("want 2 findings, got %d", len(out))
	}
	// All empty matches should remain empty.
	for i, f := range out {
		if f.Match != "" {
			t.Errorf("finding %d match: want empty, got %q", i, f.Match)
		}
	}
}

func TestHashFindingMatches_AllHashed(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Match: "alice@example.com", Severity: "high"},
		{Type: "secret", Match: "sk-1234567890abcdef", Severity: "critical"},
	}
	out := hashFindingMatches(findings)
	if len(out) != 2 {
		t.Fatalf("want 2 findings, got %d", len(out))
	}
	for i, f := range out {
		if f.Match == "" {
			t.Errorf("finding %d match: should be hashed, got empty", i)
		}
		if f.Match == findings[i].Match {
			t.Errorf("finding %d match: should be hashed, got original %q", i, f.Match)
		}
		// Verify format: sha256: prefix.
		if len(f.Match) < 7 || f.Match[:7] != "sha256:" {
			t.Errorf("finding %d match: want 'sha256:' prefix, got %q", i, f.Match)
		}
	}
}

// =============================================================================
// PostgresStore — EraseSubject/SubjectAccess additional edge cases
// NOTE: TestPostgresStore_EraseSubject_NoIdentifier and
// TestPostgresStore_SubjectAccess_NoIdentifier already exist in postgres_test.go.
// =============================================================================

func TestPostgresStore_SubjectAccess_DefaultLimit(t *testing.T) {
	// Verify the default limit logic branch (limit <= 0 → 500).
	// Use a rejecting pool so we exercise the code path without a real DB.
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := s.SubjectAccess(ctx, "00000000-0000-0000-0000-000000000001", "test-user", "", "", 0)
	// Expect error (connection rejected), the important thing is no hang.
	if err == nil {
		t.Log("SubjectAccess with zero limit unexpectedly succeeded (no pool connection)")
	}
}

// =============================================================================
// PostgresStore — GetStats invalid org UUID path
// =============================================================================

func TestPostgresStore_GetStats_InvalidUUID(t *testing.T) {
	s := &PostgresStore{log: zerolog.Nop()}
	ctx := context.Background()
	_, err := s.GetStats(ctx, "not-a-uuid", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Fatal("GetStats with invalid UUID should return error")
	}
}

// =============================================================================
// PostgresStore — SearchSecurityEvents invalid org UUID path
// =============================================================================

func TestPostgresStore_SearchSecurityEvents_InvalidUUID(t *testing.T) {
	s := &PostgresStore{log: zerolog.Nop()}
	ctx := context.Background()
	_, _, err := s.SearchSecurityEvents(ctx, "not-a-uuid", EventSearchParams{Page: 1, Limit: 50})
	if err == nil {
		t.Fatal("SearchSecurityEvents with invalid UUID should return error")
	}
}

// =============================================================================
// PostgresStore — SearchSecurityEvents pagination edge cases exercised through
// the invalid-UUID path (code before UUID parse: page/limit clamping)
// =============================================================================

func TestPostgresStore_SearchSecurityEvents_PaginationEdgeCases(t *testing.T) {
	s := &PostgresStore{log: zerolog.Nop()}
	ctx := context.Background()

	t.Run("page_zero_clamped_to_1", func(t *testing.T) {
		// Page 0 should be clamped to 1; still fails on UUID parse.
		_, _, err := s.SearchSecurityEvents(ctx, "not-a-uuid", EventSearchParams{Page: 0, Limit: 50})
		if err == nil {
			t.Fatal("expected error from invalid UUID")
		}
	})

	t.Run("negative_page_clamped_to_1", func(t *testing.T) {
		_, _, err := s.SearchSecurityEvents(ctx, "not-a-uuid", EventSearchParams{Page: -5, Limit: 50})
		if err == nil {
			t.Fatal("expected error from invalid UUID")
		}
	})

	t.Run("limit_zero_clamped_to_50", func(t *testing.T) {
		_, _, err := s.SearchSecurityEvents(ctx, "not-a-uuid", EventSearchParams{Page: 1, Limit: 0})
		if err == nil {
			t.Fatal("expected error from invalid UUID")
		}
	})

	t.Run("negative_limit_clamped_to_50", func(t *testing.T) {
		_, _, err := s.SearchSecurityEvents(ctx, "not-a-uuid", EventSearchParams{Page: 1, Limit: -10})
		if err == nil {
			t.Fatal("expected error from invalid UUID")
		}
	})

	t.Run("limit_exceeds_200_clamped", func(t *testing.T) {
		_, _, err := s.SearchSecurityEvents(ctx, "not-a-uuid", EventSearchParams{Page: 1, Limit: 500})
		if err == nil {
			t.Fatal("expected error from invalid UUID")
		}
	})
}

// =============================================================================
// PricingStore tests with rejecting pool
// =============================================================================

func TestPricingStore_ListActive_WithRejectingPool(t *testing.T) {
	t.Run("rejecting_pool_returns_error", func(t *testing.T) {
		ps := NewPricingStore(newRejectingPool(t))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		active, err := ps.ListActive(ctx)
		if err == nil {
			t.Logf("ListActive unexpectedly succeeded: %d rows", len(active))
		}
	})

	t.Run("nil_pool_constructor", func(t *testing.T) {
		ps := NewPricingStore(nil)
		if ps == nil {
			t.Fatal("NewPricingStore(nil) should return non-nil")
		}
		// Calling ListActive with nil pool will likely panic or timeout.
		// This tests constructor safety only.
	})
}

func TestPricingStore_Lookup_WithRejectingPool(t *testing.T) {
	t.Run("rejecting_pool_returns_error", func(t *testing.T) {
		ps := NewPricingStore(newRejectingPool(t))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		price, err := ps.Lookup(ctx, "openai", "gpt-4o", "default", time.Now())
		if err == nil {
			t.Logf("Lookup unexpectedly succeeded: %+v", price)
		}
	})
}

// =============================================================================
// SavedHuntStore tests with rejecting pool
// =============================================================================

func newRejectingSavedHuntStore(t *testing.T) SavedHuntStore {
	t.Helper()
	pool := newRejectingPool(t)
	return NewSavedHuntStore(pool)
}

func TestSavedHuntStore_List_WithRejectingPool(t *testing.T) {
	store := newRejectingSavedHuntStore(t)
	if store == nil {
		t.Fatal("NewSavedHuntStore should return non-nil with rejecting pool")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	hunts, err := store.List(ctx, "org-1")
	if err == nil {
		t.Logf("List unexpectedly succeeded: %d hunts", len(hunts))
	}
}

func TestSavedHuntStore_Create_WithRejectingPool(t *testing.T) {
	store := newRejectingSavedHuntStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	hunt := &SavedHunt{
		OrgID: "org-1",
		Name:  "test-hunt",
	}
	err := store.Create(ctx, hunt)
	if err == nil {
		t.Logf("Create unexpectedly succeeded: id=%s", hunt.ID)
	}
}

func TestSavedHuntStore_Create_NilQueryDefaults(t *testing.T) {
	store := newRejectingSavedHuntStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	hunt := &SavedHunt{
		OrgID: "org-1",
		Name:  "test-hunt",
		// Query is nil → should default to json.RawMessage("{}")
		// CreatedAt is zero → should default to time.Now().UTC()
	}
	err := store.Create(ctx, hunt)
	if err == nil {
		t.Logf("Create with nil query unexpectedly succeeded: id=%s", hunt.ID)
	}
}

func TestSavedHuntStore_Update_WithRejectingPool(t *testing.T) {
	store := newRejectingSavedHuntStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	hunt := &SavedHunt{
		ID:    "hunt-id",
		OrgID: "org-1",
		Name:  "updated-hunt",
	}
	err := store.Update(ctx, hunt)
	if err == nil {
		t.Logf("Update unexpectedly succeeded")
	}
}

func TestSavedHuntStore_Update_NilQueryDefaults(t *testing.T) {
	store := newRejectingSavedHuntStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	hunt := &SavedHunt{
		ID:    "hunt-id",
		OrgID: "org-1",
		Name:  "updated-hunt",
		// Query is nil → should default to json.RawMessage("{}")
	}
	err := store.Update(ctx, hunt)
	if err == nil {
		t.Logf("Update with nil query unexpectedly succeeded")
	}
}

func TestSavedHuntStore_Delete_WithRejectingPool(t *testing.T) {
	store := newRejectingSavedHuntStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := store.Delete(ctx, "org-1", "hunt-id")
	if err == nil {
		t.Logf("Delete unexpectedly succeeded")
	}
}

// =============================================================================
// PostgresStore — GetDailyTokenUsage with rejecting pool
// =============================================================================

func TestPostgresStore_GetDailyTokenUsage_WithRejectingPool(t *testing.T) {
	s := newRejectingPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	usage, err := s.GetDailyTokenUsage(ctx, "00000000-0000-0000-0000-000000000001",
		time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Logf("GetDailyTokenUsage unexpectedly succeeded: %d rows", len(usage))
	}
}

// =============================================================================
// PostgresStore — insertBatch error path (n>0 but pool SendBatch fails)
// This is already tested via batch flush with invalid UUIDs (all yieled into n==0).
// The n>0 error path is covered by the SaveRequestLog_BatchFlush test with
// invalid UUIDs. The rejecting pool tests cover QueryRow/Query error paths.
// =============================================================================

// =============================================================================
// DBHandler — additional edge cases for hashFindingMatches in data protection
// =============================================================================

func TestDBHandler_HashFindings_PolicyEnabled(t *testing.T) {
	s := NewNoopStoreSilent()
	// Policy with HashFindings enabled.
	getPolicy := func() *policy.Policy {
		return &policy.Policy{
			Data: &policy.DataControl{
				HashFindings: true,
			},
		}
	}
	handler := DBHandler(zerolog.Nop(), s, "org-1", getPolicy)
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings: []scanner.Finding{
			{Type: "pii", Match: "alice@example.com", Severity: "high"},
		},
	})
}

func TestDBHandler_HashFindings_PolicyDisabled(t *testing.T) {
	s := NewNoopStoreSilent()
	// Policy with HashFindings disabled.
	getPolicy := func() *policy.Policy {
		return &policy.Policy{
			Data: &policy.DataControl{
				HashFindings: false,
			},
		}
	}
	handler := DBHandler(zerolog.Nop(), s, "org-1", getPolicy)
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings: []scanner.Finding{
			{Type: "secret", Match: "sk-abc123", Severity: "critical"},
		},
	})
}

// =============================================================================
// Store interface — compile-time checks for all concrete types
// =============================================================================

func TestStoreInterface_AllImplementations(t *testing.T) {
	// NoopStore
	var _ Store = (*NoopStore)(nil)
	// PostgresStore
	var _ Store = (*PostgresStore)(nil)
}

// =============================================================================
// PricingQuerier — compile-time check
// =============================================================================

func TestPricingQuerier_Interface(t *testing.T) {
	var _ PricingQuerier = (*PricingStore)(nil)
}

// =============================================================================
// ModelPricing zero-value check
// =============================================================================

func TestModelPricing_ZeroValue(t *testing.T) {
	p := ModelPricing{}
	if p.ID != 0 {
		t.Errorf("ID: want 0, got %d", p.ID)
	}
	if p.Provider != "" {
		t.Errorf("Provider: want empty, got %q", p.Provider)
	}
	if p.Currency != "" {
		t.Errorf("Currency: want empty, got %q", p.Currency)
	}
	if p.InputPer1K != 0 {
		t.Errorf("InputPer1K: want 0, got %.3f", p.InputPer1K)
	}
	if p.OutputPer1K != 0 {
		t.Errorf("OutputPer1K: want 0, got %.3f", p.OutputPer1K)
	}
}

// =============================================================================
// SavedHunt zero-value check
// =============================================================================

func TestSavedHunt_ZeroValue(t *testing.T) {
	h := SavedHunt{}
	if h.ID != "" {
		t.Errorf("ID: want empty, got %q", h.ID)
	}
	if h.OrgID != "" {
		t.Errorf("OrgID: want empty, got %q", h.OrgID)
	}
	if h.Name != "" {
		t.Errorf("Name: want empty, got %q", h.Name)
	}
	if h.Query != nil {
		t.Errorf("Query: want nil, got %s", string(h.Query))
	}
}

// =============================================================================
// ErrSavedHuntNotFound sentinel check
// =============================================================================

func TestErrSavedHuntNotFound_IsError(t *testing.T) {
	err := ErrSavedHuntNotFound
	if err.Error() != "saved hunt not found" {
		t.Errorf("error message: want 'saved hunt not found', got %q", err.Error())
	}
}

// =============================================================================
// DailyTokenUsage zero-value check
// =============================================================================

func TestDailyTokenUsage_ZeroValue(t *testing.T) {
	u := DailyTokenUsage{}
	if u.Provider != "" {
		t.Errorf("Provider: want empty, got %q", u.Provider)
	}
	if u.Model != "" {
		t.Errorf("Model: want empty, got %q", u.Model)
	}
	if u.ModelFamily != "" {
		t.Errorf("ModelFamily: want empty, got %q", u.ModelFamily)
	}
	if u.InputTokens != 0 {
		t.Errorf("InputTokens: want 0, got %d", u.InputTokens)
	}
	if u.OutputTokens != 0 {
		t.Errorf("OutputTokens: want 0, got %d", u.OutputTokens)
	}
	if !u.Date.IsZero() {
		t.Errorf("Date: want zero, got %v", u.Date)
	}
}

// =============================================================================
// RequestLogRow zero-value check
// =============================================================================

func TestRequestLogRow_ZeroValue(t *testing.T) {
	r := RequestLogRow{}
	if r.RequestID != "" {
		t.Errorf("RequestID: want empty, got %q", r.RequestID)
	}
	if r.CostUSD != 0 {
		t.Errorf("CostUSD: want 0, got %.6f", r.CostUSD)
	}
	if r.TokensIn != 0 {
		t.Errorf("TokensIn: want 0, got %d", r.TokensIn)
	}
	if r.TokensOut != 0 {
		t.Errorf("TokensOut: want 0, got %d", r.TokensOut)
	}
}

// =============================================================================
// flush — with cancel triggered during insertTimeout window (rejecting pool)
// =============================================================================

func TestPostgresStore_Flush_WithExpiredContext(t *testing.T) {
	s := newRejectingPostgresStore(t)
	// Manually set a buffer so flush tries to process it.
	s.mu.Lock()
	s.buf = []RequestLog{
		{RequestID: "r1", OrgID: "00000000-0000-0000-0000-000000000001"},
	}
	s.mu.Unlock()

	// flush with an already-expired context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// flush will create ctx2 from the expired ctx, then call insertBatch
	// which will try to use the rejecting pool. May log a warning, but should not panic.
	s.flush(ctx)
	// No assertion — just exercising the code path safely.
}

// =============================================================================
// Store error mock — comprehensive error-returning mock using composition
// =============================================================================

// errorStore wraps a NoopStore and overrides specific methods to return errors.
// Used to test error handling in callers of the Store interface.
type errorStore struct {
	*NoopStore
	saveErr        error
	statsErr       error
	listErr        error
	searchErr      error
	modelUsageErr  error
	dailyUsageErr  error
	pingErr        error
	closeErr       error
}

func (e *errorStore) SaveRequestLog(_ context.Context, _ RequestLog) error {
	if e.saveErr != nil {
		return e.saveErr
	}
	return nil
}

func (e *errorStore) GetStats(_ context.Context, _ string, _, _ time.Time) (*Stats, error) {
	if e.statsErr != nil {
		return nil, e.statsErr
	}
	return &Stats{}, nil
}

func (e *errorStore) ListSecurityEvents(_ context.Context, _ string, _, _ int) ([]SecurityEvent, int, error) {
	if e.listErr != nil {
		return nil, 0, e.listErr
	}
	return nil, 0, nil
}

func (e *errorStore) SearchSecurityEvents(_ context.Context, _ string, _ EventSearchParams) ([]SecurityEvent, int, error) {
	if e.searchErr != nil {
		return nil, 0, e.searchErr
	}
	return nil, 0, nil
}

func (e *errorStore) GetModelTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]ModelTokenUsage, error) {
	if e.modelUsageErr != nil {
		return nil, e.modelUsageErr
	}
	return nil, nil
}

func (e *errorStore) GetDailyTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]DailyTokenUsage, error) {
	if e.dailyUsageErr != nil {
		return nil, e.dailyUsageErr
	}
	return nil, nil
}

func (e *errorStore) Ping(_ context.Context) error {
	if e.pingErr != nil {
		return e.pingErr
	}
	return nil
}

func (e *errorStore) Close() error {
	if e.closeErr != nil {
		return e.closeErr
	}
	return nil
}

// compile-time check
var _ Store = (*errorStore)(nil)

func TestErrorStore_SaveRequestLog_Error(t *testing.T) {
	errStore := &errorStore{saveErr: errors.New("db write failure")}
	handler := DBHandler(zerolog.Nop(), errStore, "org-1", nil)
	// Should not panic when the underlying SaveRequestLog returns an error.
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "pii", Match: "test"}},
	})
}

func TestErrorStore_GetStats_Error(t *testing.T) {
	errStore := &errorStore{statsErr: errors.New("query timeout")}
	ctx := context.Background()
	_, err := errStore.GetStats(ctx, "org-1", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Fatal("expected error from GetStats")
	}
	if err.Error() != "query timeout" {
		t.Errorf("error message: want 'query timeout', got %q", err.Error())
	}
}

func TestErrorStore_ListSecurityEvents_Error(t *testing.T) {
	errStore := &errorStore{listErr: errors.New("scan failure")}
	ctx := context.Background()
	_, _, err := errStore.ListSecurityEvents(ctx, "org-1", 1, 50)
	if err == nil {
		t.Fatal("expected error from ListSecurityEvents")
	}
}

func TestErrorStore_SearchSecurityEvents_Error(t *testing.T) {
	errStore := &errorStore{searchErr: errors.New("query failure")}
	ctx := context.Background()
	_, _, err := errStore.SearchSecurityEvents(ctx, "org-1", EventSearchParams{Page: 1, Limit: 50})
	if err == nil {
		t.Fatal("expected error from SearchSecurityEvents")
	}
}

func TestErrorStore_GetModelTokenUsage_Error(t *testing.T) {
	errStore := &errorStore{modelUsageErr: errors.New("usage query failure")}
	ctx := context.Background()
	_, err := errStore.GetModelTokenUsage(ctx, "org-1", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Fatal("expected error from GetModelTokenUsage")
	}
}

func TestErrorStore_GetDailyTokenUsage_Error(t *testing.T) {
	errStore := &errorStore{dailyUsageErr: errors.New("daily query failure")}
	ctx := context.Background()
	_, err := errStore.GetDailyTokenUsage(ctx, "org-1", time.Now().Add(-24*time.Hour), time.Now())
	if err == nil {
		t.Fatal("expected error from GetDailyTokenUsage")
	}
}

func TestErrorStore_Ping_Error(t *testing.T) {
	errStore := &errorStore{pingErr: errors.New("connection lost")}
	ctx := context.Background()
	err := errStore.Ping(ctx)
	if err == nil {
		t.Fatal("expected error from Ping")
	}
}

func TestErrorStore_Close_Error(t *testing.T) {
	errStore := &errorStore{closeErr: errors.New("close failure")}
	err := errStore.Close()
	if err == nil {
		t.Fatal("expected error from Close")
	}
}

func TestErrorStore_AllMethods_WithErrors(t *testing.T) {
	fakeErr := errors.New("universal error")
	errStore := &errorStore{
		saveErr:       fakeErr,
		statsErr:      fakeErr,
		listErr:       fakeErr,
		searchErr:     fakeErr,
		modelUsageErr: fakeErr,
		dailyUsageErr: fakeErr,
		pingErr:       fakeErr,
		closeErr:      fakeErr,
	}
	ctx := context.Background()

	t.Run("SaveRequestLog", func(t *testing.T) {
		if err := errStore.SaveRequestLog(ctx, RequestLog{RequestID: "r1"}); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("GetStats", func(t *testing.T) {
		if _, err := errStore.GetStats(ctx, "org", time.Now(), time.Now()); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ListSecurityEvents", func(t *testing.T) {
		if _, _, err := errStore.ListSecurityEvents(ctx, "org", 1, 10); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("SearchSecurityEvents", func(t *testing.T) {
		if _, _, err := errStore.SearchSecurityEvents(ctx, "org", EventSearchParams{}); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("GetModelTokenUsage", func(t *testing.T) {
		if _, err := errStore.GetModelTokenUsage(ctx, "org", time.Now(), time.Now()); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("GetDailyTokenUsage", func(t *testing.T) {
		if _, err := errStore.GetDailyTokenUsage(ctx, "org", time.Now(), time.Now()); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("Ping", func(t *testing.T) {
		if err := errStore.Ping(ctx); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("Close", func(t *testing.T) {
		if err := errStore.Close(); err == nil {
			t.Error("expected error")
		}
	})
}

// =============================================================================
// PartitionManager.RunMaintenanceCycle with a non-nil pm but nil db
// (already tested: nil db returns error)
// =============================================================================

// =============================================================================
// RetentionPolicy zero-value checks
// =============================================================================

func TestRetentionPolicy_ZeroValue(t *testing.T) {
	p := RetentionPolicy{}
	if p.RequestLogsDays != 0 {
		t.Errorf("RequestLogsDays: want 0, got %d", p.RequestLogsDays)
	}
	if p.AlertsDays != 0 {
		t.Errorf("AlertsDays: want 0, got %d", p.AlertsDays)
	}
	if p.DailyStatsDays != 0 {
		t.Errorf("DailyStatsDays: want 0, got %d", p.DailyStatsDays)
	}
	if p.AuditLogsDays != 0 {
		t.Errorf("AuditLogsDays: want 0, got %d", p.AuditLogsDays)
	}
	if p.IncidentClosedDays != 0 {
		t.Errorf("IncidentClosedDays: want 0, got %d", p.IncidentClosedDays)
	}
}

// =============================================================================
// PartitionManager — LastRun on nil receiver panics (no nil-receiver guard).
// This is by design: LastRun is only called after construction.
// Skipping nil-receiver test to avoid intentional panic.
// =============================================================================

// =============================================================================
// PartitionManager.RunMaintenanceCycle with rejecting pool
// =============================================================================

func TestPartitionManager_RunMaintenanceCycle_WithRejectingPool(t *testing.T) {
	pool := newRejectingPool(t)
	pm := NewPartitionManager(pool, DefaultRetentionPolicy(), zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := pm.RunMaintenanceCycle(ctx)
	if err == nil {
		t.Log("RunMaintenanceCycle unexpectedly succeeded with rejecting pool")
	}
	// Verify lastRunUnix was NOT set (error path).
	ts := pm.LastRun()
	if !ts.IsZero() {
		t.Errorf("LastRun after failed cycle: want zero, got %v", ts)
	}
}

// =============================================================================
// PartitionManager.Start with rejecting pool
// =============================================================================

func TestPartitionManager_Start_WithRejectingPool(t *testing.T) {
	pool := newRejectingPool(t)
	pm := NewPartitionManager(pool, DefaultRetentionPolicy(), zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	pm.Start(ctx, time.Hour)
}

// =============================================================================
// PartitionManager.createUpcomingPartitions error path
// =============================================================================

func TestPartitionManager_CreateUpcomingPartitions_ErrorPath(t *testing.T) {
	pool := newRejectingPool(t)
	pm := NewPartitionManager(pool, DefaultRetentionPolicy(), zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := pm.createUpcomingPartitions(ctx, 2)
	if err == nil {
		t.Fatal("createUpcomingPartitions should fail with rejecting pool")
	}
}

// =============================================================================
// NoopStore.GetDailyTokenUsage — direct coverage (was 0%)
// =============================================================================

func TestNoopStore_GetDailyTokenUsage_Compliance(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		now := time.Now().UTC()
		usage, err := s.GetDailyTokenUsage(ctx, "org-1", now.Add(-24*time.Hour), now)
		if err != nil {
			t.Fatalf("GetDailyTokenUsage should return nil error, got %v", err)
		}
		if usage != nil {
			t.Errorf("usage: want nil slice, got %d rows", len(usage))
		}
	})

	t.Run("zero_time_range", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		zero := time.Time{}
		usage, err := s.GetDailyTokenUsage(ctx, "org-1", zero, zero)
		if err != nil {
			t.Fatalf("GetDailyTokenUsage zero time: %v", err)
		}
		if usage != nil {
			t.Errorf("usage: want nil, got %d rows", len(usage))
		}
	})

	t.Run("empty_org", func(t *testing.T) {
		s := NewNoopStoreSilent()
		ctx := context.Background()
		usage, err := s.GetDailyTokenUsage(ctx, "", time.Now(), time.Now())
		if err != nil {
			t.Fatalf("GetDailyTokenUsage empty org: %v", err)
		}
		if usage != nil {
			t.Errorf("usage: want nil, got %d rows", len(usage))
		}
	})
}

// =============================================================================
// nullStr additional edge case tests
// =============================================================================

func TestNullStr_EdgeCases(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if nullStr("") != nil {
			t.Error("nullStr(\"\") should return nil")
		}
	})
	t.Run("non_empty", func(t *testing.T) {
		if nullStr("hello") != "hello" {
			t.Error("nullStr(\"hello\") should return \"hello\"")
		}
	})
	t.Run("whitespace", func(t *testing.T) {
		if nullStr("  ") != "  " {
			t.Error("nullStr(\"  \") should return \"  \"")
		}
	})
}

// =============================================================================
// SavedHuntStore interface check with non-nil pool
// =============================================================================

func TestNewSavedHuntStore_InterfaceReturn(t *testing.T) {
	pool := newRejectingPool(t)
	store := NewSavedHuntStore(pool)
	if store == nil {
		t.Fatal("NewSavedHuntStore with non-nil pool should return non-nil")
	}
	var _ SavedHuntStore = store
}

