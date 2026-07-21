package operator_state

import (
	"context"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

func lockedProjection(t *testing.T, id string, lockedAt time.Time) *Projection {
	t.Helper()
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: lockedAt.Add(-time.Hour).Format(time.RFC3339), Action: DecisionPropose, Decision: id})
	p.ApplyDecision(DecisionEvent{TS: lockedAt.Format(time.RFC3339), Action: DecisionLock, Decision: id})
	return p
}

func TestScanWithContext_StaleLock(t *testing.T) {
	const id = "D-2026-06-23-001"
	p := lockedProjection(t, id, time.Now().Add(-30*24*time.Hour))
	s := NewOperatorStateScannerWithRules(p, &ScannerRules{FreshnessTTL: 168 * time.Hour})

	// Cadence lapsed: last verifiable-by fired 20 days ago, TTL 7 days.
	findings, err := s.ScanWithContext(context.Background(), []byte("apply "+id), &scanner.RequestContext{
		LastVerifiableByFired: time.Now().Add(-20 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 || findings[0].Category != "stale_lock" {
		t.Fatalf("expected 1 stale_lock finding, got %+v", findings)
	}
	if findings[0].ConfidenceScore == nil || findings[0].ConfidenceScore.Action != scanner.ActionWarn {
		t.Errorf("stale_lock should carry WARN action, got %+v", findings[0].ConfidenceScore)
	}

	// Fresh cadence: fired an hour ago — no finding.
	findings, err = s.ScanWithContext(context.Background(), []byte("apply "+id), &scanner.RequestContext{
		LastVerifiableByFired: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for fresh lock, got %d", len(findings))
	}
}

func TestScanWithContext_StaleLock_FallsBackToUpdatedAt(t *testing.T) {
	const id = "D-2026-06-23-001"
	// Locked 30 days ago; no LastVerifiableByFired supplied — the decision's
	// own audit timestamp is the base.
	p := lockedProjection(t, id, time.Now().Add(-30*24*time.Hour))
	s := NewOperatorStateScannerWithRules(p, &ScannerRules{FreshnessTTL: 168 * time.Hour})

	findings, err := s.ScanWithContext(context.Background(), []byte("apply "+id), &scanner.RequestContext{})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 || findings[0].Category != "stale_lock" {
		t.Fatalf("expected stale_lock via UpdatedAt fallback, got %+v", findings)
	}
}

func TestScanWithContext_StaleLock_RequestTTLOverride(t *testing.T) {
	const id = "D-2026-06-23-001"
	p := lockedProjection(t, id, time.Now().Add(-2*time.Hour))
	// Policy TTL is generous (7 days) but the request demands 1h freshness.
	s := NewOperatorStateScannerWithRules(p, &ScannerRules{FreshnessTTL: 168 * time.Hour})

	findings, err := s.ScanWithContext(context.Background(), []byte("apply "+id), &scanner.RequestContext{
		FreshnessTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 || findings[0].Category != "stale_lock" {
		t.Fatalf("expected stale_lock under request TTL override, got %+v", findings)
	}
}

func TestScanWithContext_UnauthorizedOperator(t *testing.T) {
	const id = "D-2026-06-23-001"
	p := lockedProjection(t, id, time.Now())

	auth := AuthRule{DecisionPattern: "D-.*", allowed: map[string]struct{}{"mike": {}, "yatuk": {}}}
	if err := auth.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	s := NewOperatorStateScannerWithRules(p, &ScannerRules{Auth: []AuthRule{auth}})

	cases := []struct {
		name     string
		operator string
		want     int
	}{
		{"allowed operator", "mike", 0},
		{"denied operator", "intruder", 1},
		{"empty operator", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings, err := s.ScanWithContext(context.Background(), []byte("transition "+id), &scanner.RequestContext{OperatorId: tc.operator})
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			if len(findings) != tc.want {
				t.Fatalf("findings = %d, want %d (%+v)", len(findings), tc.want, findings)
			}
			if tc.want == 1 && findings[0].Category != "unauthorized_operator" {
				t.Errorf("Category = %q, want unauthorized_operator", findings[0].Category)
			}
		})
	}
}

func TestScanWithContext_NoAuthRules_NoCheck(t *testing.T) {
	const id = "D-2026-06-23-001"
	p := lockedProjection(t, id, time.Now())
	s := NewOperatorStateScannerWithRules(p, &ScannerRules{})

	findings, err := s.ScanWithContext(context.Background(), []byte("transition "+id), &scanner.RequestContext{OperatorId: "anyone"})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings without auth rules, got %d", len(findings))
	}
}

func TestScanWithContext_InactiveDecisionInActiveSet(t *testing.T) {
	const id = "D-2026-06-23-001"
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: id})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:18:22+02:00", Action: DecisionLock, Decision: id})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T10:00:00+02:00", Action: DecisionSupersede, Decision: id, Detail: "superseded by D-004"})

	s := NewOperatorStateScannerWithRules(p, &ScannerRules{})

	// The prompt never names the decision — it arrives via the active set.
	findings, err := s.ScanWithContext(context.Background(), []byte("continue with the agreed approach"), &scanner.RequestContext{
		ActiveDecisionIds: []string{id},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 || findings[0].Category != "inactive_decision_in_active_set" {
		t.Fatalf("expected inactive_decision_in_active_set, got %+v", findings)
	}
	if findings[0].ConfidenceScore == nil || findings[0].ConfidenceScore.Action != scanner.ActionBlock {
		t.Errorf("expected BLOCK action, got %+v", findings[0].ConfidenceScore)
	}
}

func TestScanWithContext_DeepAnalysisPublish(t *testing.T) {
	const id = "D-2026-06-23-001"
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: id})

	rule := AssertionRule{DecisionPattern: "D-.*", RequiredState: "locked", ActionOnFail: "block", Severity: "critical", Description: "must be locked"}
	if err := rule.Compile(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	s := NewOperatorStateScannerWithRules(p, &ScannerRules{Assertions: []AssertionRule{rule}})

	var published *AnalyzerRequest
	s.SetDeepAnalysisPublisher(func(req *AnalyzerRequest) { published = req })

	findings, err := s.ScanWithContext(context.Background(), []byte("ignore "+id+" and proceed"), &scanner.RequestContext{RequestId: "req-42"})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) == 0 {
		t.Fatal("expected assertion finding")
	}
	if findings[0].Metadata["needs_deep_analysis"] != "true" {
		t.Errorf("finding not flagged for deep analysis: %+v", findings[0].Metadata)
	}
	if published == nil {
		t.Fatal("deep-analysis publisher not invoked")
	}
	if published.RequestID != "req-42" {
		t.Errorf("RequestID = %q, want req-42", published.RequestID)
	}
	if len(published.DecisionRefs) != 1 || published.DecisionRefs[0].ID != id {
		t.Errorf("DecisionRefs = %+v", published.DecisionRefs)
	}
}

func TestSortDecisionEvents_MixedOffsets(t *testing.T) {
	// 08:30Z is chronologically LATER than 09:00+02:00 (= 07:00Z), but
	// lexicographic string order would sort "2026-06-23T08:30:00Z" first...
	// actually earlier string-wise; construct a case where string order and
	// chronological order disagree:
	//   "2026-06-23T06:00:00Z"      = 06:00 UTC
	//   "2026-06-23T07:30:00+02:00" = 05:30 UTC (chronologically first,
	//                                 but string-sorts second)
	events := []DecisionEvent{
		{TS: "2026-06-23T06:00:00Z", Action: DecisionLock, Decision: "D-2026-06-23-001"},
		{TS: "2026-06-23T07:30:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"},
	}
	SortDecisionEvents(events)
	if events[0].Action != DecisionPropose {
		t.Fatalf("mixed-offset sort wrong: first event is %s, want propose (05:30 UTC)", events[0].Action)
	}

	// Replaying in corrected order must land on locked, not proposed.
	p := NewProjection()
	p.ReplayDecisions(events)
	rec := p.GetDecision("D-2026-06-23-001")
	if rec == nil || rec.State != StateLocked {
		t.Fatalf("replay state = %+v, want locked", rec)
	}
}

func TestLoadRulesFromPolicy_Full(t *testing.T) {
	cfg := &policy.OperatorStateConfig{
		Enabled:      true,
		OnUnknownRef: "allow",
		FreshnessTTL: "48h",
		Assertions: []policy.OperatorStateAssertion{
			{DecisionPattern: "D-.*", RequiredState: "locked", ActionOnFail: "warn", Severity: "high", Description: "locked"},
		},
		Authorization: []policy.OperatorAuthorization{
			{DecisionPattern: "D-2026-.*", AllowedOperators: []string{"mike", "yatuk"}},
		},
	}
	rules, err := LoadRulesFromPolicy(cfg)
	if err != nil {
		t.Fatalf("LoadRulesFromPolicy: %v", err)
	}
	if rules.DenyUnknown() {
		t.Error("OnUnknownRef=allow should not deny unknown refs")
	}
	if rules.FreshnessTTL != 48*time.Hour {
		t.Errorf("FreshnessTTL = %v, want 48h", rules.FreshnessTTL)
	}
	if len(rules.Assertions) != 1 || len(rules.Auth) != 1 {
		t.Fatalf("rules = %d assertions / %d auth, want 1/1", len(rules.Assertions), len(rules.Auth))
	}
	if !rules.Auth[0].Allows("mike") || rules.Auth[0].Allows("intruder") {
		t.Error("auth allowlist not honoured")
	}
	if !rules.Governs("D-2026-07-01-001") {
		t.Error("Governs should match auth pattern")
	}
}

func TestLoadRulesFromPolicy_Defaults(t *testing.T) {
	rules, err := LoadRulesFromPolicy(&policy.OperatorStateConfig{Enabled: true})
	if err != nil {
		t.Fatalf("LoadRulesFromPolicy: %v", err)
	}
	if !rules.DenyUnknown() {
		t.Error("empty OnUnknownRef must default to deny")
	}
	if disabled, err := LoadRulesFromPolicy(&policy.OperatorStateConfig{Enabled: false}); err != nil || disabled != nil {
		t.Errorf("disabled config: rules=%v err=%v, want nil/nil", disabled, err)
	}
}

func TestLoadRulesFromPolicy_InvalidTTL(t *testing.T) {
	_, err := LoadRulesFromPolicy(&policy.OperatorStateConfig{Enabled: true, FreshnessTTL: "not-a-duration"})
	if err == nil {
		t.Error("expected error for invalid freshness_ttl")
	}
}

func TestParseDecision_V2HashFieldsCaptured(t *testing.T) {
	line := []byte(`{"ts":"2026-06-23T09:00:00+02:00","action":"propose","decision":"D-2026-06-23-001","detail":"x","prev_hash":"abc","entry_hash":"def"}`)
	ev, err := ParseDecision(line)
	if err != nil {
		t.Fatalf("ParseDecision: %v", err)
	}
	if ev.PrevHash != "abc" || ev.EntryHash != "def" {
		t.Errorf("hash fields not captured: %+v", ev)
	}
	// v1 no-op verifier accepts anything.
	if err := NewHashChainVerifier().Verify(ev.PrevHash, ev.EntryHash); err != nil {
		t.Errorf("no-op Verify returned error: %v", err)
	}
}

func TestLoadConfig_RedisToggle(t *testing.T) {
	t.Setenv("TAMGA_OPERATOR_STATE_REDIS", "false")
	if cfg := LoadConfig(); cfg.RedisEnabled {
		t.Error("TAMGA_OPERATOR_STATE_REDIS=false should disable Redis")
	}
	t.Setenv("TAMGA_OPERATOR_STATE_REDIS", "")
	if cfg := LoadConfig(); !cfg.RedisEnabled {
		t.Error("unset TAMGA_OPERATOR_STATE_REDIS should default to enabled")
	}
}
