package store

import (
	"context"
	"os"
	"testing"

	"github.com/rs/zerolog"
)

func TestDefaultRetentionPolicy_AllFields(t *testing.T) {
	p := DefaultRetentionPolicy()
	if p.RequestLogsDays != 30 {
		t.Errorf("expected 30 request_logs days, got %d", p.RequestLogsDays)
	}
	if p.AlertsDays != 90 {
		t.Errorf("expected 90 alerts days, got %d", p.AlertsDays)
	}
	if p.DailyStatsDays != 365 {
		t.Errorf("expected 365 daily_stats days, got %d", p.DailyStatsDays)
	}
	if p.AuditLogsDays != 365 {
		t.Errorf("expected 365 audit_logs days, got %d", p.AuditLogsDays)
	}
	if p.IncidentClosedDays != 0 {
		t.Errorf("expected 0 closed incident days, got %d", p.IncidentClosedDays)
	}
}

func TestRetentionPolicyFromEnv_PartialOverride(t *testing.T) {
	_ = os.Setenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS", "14")
	_ = os.Setenv("TAMGA_RETENTION_ALERTS_DAYS", "45")
	defer func() {
		_ = os.Unsetenv("TAMGA_RETENTION_REQUEST_LOGS_DAYS")
		_ = os.Unsetenv("TAMGA_RETENTION_ALERTS_DAYS")
	}()

	p := RetentionPolicyFromEnv()
	if p.RequestLogsDays != 14 {
		t.Errorf("expected 14, got %d", p.RequestLogsDays)
	}
	if p.AlertsDays != 45 {
		t.Errorf("expected 45, got %d", p.AlertsDays)
	}
	// Unset values should keep defaults
	if p.DailyStatsDays != 365 {
		t.Errorf("expected 365 default, got %d", p.DailyStatsDays)
	}
}

func TestRetentionPolicyFromEnv_AllDefaultsWhenUnset(t *testing.T) {
	for _, k := range []string{
		"TAMGA_RETENTION_REQUEST_LOGS_DAYS",
		"TAMGA_RETENTION_ALERTS_DAYS",
		"TAMGA_RETENTION_DAILY_STATS_DAYS",
		"TAMGA_RETENTION_AUDIT_DAYS",
	} {
		_ = os.Unsetenv(k)
	}
	p := RetentionPolicyFromEnv()
	def := DefaultRetentionPolicy()
	if p != def {
		t.Errorf("expected defaults when no env set:\n  got  %+v\n  want %+v", p, def)
	}
}

func TestParsePartitionUpperBoundTO_InvalidCases(t *testing.T) {
	cases := []string{
		"",
		"garbage",
		"FOR VALUES FROM ('2026-01-01')",
	}
	for _, c := range cases {
		_, ok := parsePartitionUpperBoundTO(c)
		if ok {
			t.Errorf("expected !ok for %q", c)
		}
	}
}

func TestParsePartitionUpperBoundTO_MidMonth(t *testing.T) {
	bound := `FOR VALUES FROM ('2025-12-01') TO ('2026-06-15')`
	ts, ok := parsePartitionUpperBoundTO(bound)
	if !ok {
		t.Fatal("expected ok")
	}
	if ts.Year() != 2026 || ts.Month() != 6 || ts.Day() != 15 {
		t.Errorf("got %v", ts)
	}
}

func TestNullStr_EmptyAndNonEmpty(t *testing.T) {
	if nullStr("") != nil {
		t.Error("expected nil for empty string")
	}
	if nullStr("hello") != "hello" {
		t.Error("expected 'hello' for non-empty string")
	}
	if nullStr(" ") != " " {
		t.Error("expected ' ' for whitespace string")
	}
}

func TestPartitionManager_NewWithPolicy(t *testing.T) {
	// PartitionManager requires a nil-safe nil db pointer for constructor
	pm := &PartitionManager{}
	pm.lastRunUnix.Store(0)
	if !pm.LastRun().IsZero() {
		t.Error("fresh PartitionManager should have zero last run time")
	}
}

func TestNoopStoreSilent_Basic(t *testing.T) {
	ctx := context.Background()
	s := NewNoopStoreSilent()
	if err := s.Ping(ctx); err != nil {
		t.Errorf("ping should succeed: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("close should succeed: %v", err)
	}
}

func TestNoopStoreNormal_Close(t *testing.T) {
	s := NewNoopStore(zerolog.Nop())
	if err := s.Close(); err != nil {
		t.Errorf("close should succeed: %v", err)
	}
}

func TestNoopStoreSilent_SaveRequestLog(t *testing.T) {
	ctx := context.Background()
	s := NewNoopStoreSilent()
	log := RequestLog{
		OrgID:     "org-1",
		RequestID: "req-1",
		Provider:  "openai",
	}
	if err := s.SaveRequestLog(ctx, log); err != nil {
		t.Errorf("SaveRequestLog should succeed: %v", err)
	}
}
