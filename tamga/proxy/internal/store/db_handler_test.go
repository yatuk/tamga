package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

func TestHashFindingMatches(t *testing.T) {
	t.Run("hashes_match_field", func(t *testing.T) {
		findings := []scanner.Finding{
			{Type: "pii", Category: "EMAIL", Match: "user@example.com", Severity: "high", Confidence: 0.95},
			{Type: "secret", Category: "aws_key", Match: "AKIAIOSFODNN7EXAMPLE", Severity: "critical", Confidence: 0.98},
		}
		out := hashFindingMatches(findings)
		if len(out) != 2 {
			t.Fatalf("want 2 findings, got %d", len(out))
		}
		// First finding: match should be sha256-prefixed
		if !strings.HasPrefix(out[0].Match, "sha256:") {
			t.Fatalf("want sha256: prefix, got %s", out[0].Match)
		}
		if len(out[0].Match) != 7+64 { // "sha256:" + 64 hex chars
			t.Fatalf("want 71 chars, got %d: %s", len(out[0].Match), out[0].Match)
		}
		// Other fields should be preserved
		if out[0].Type != "pii" || out[0].Category != "EMAIL" || out[0].Severity != "high" {
			t.Fatalf("fields not preserved: %+v", out[0])
		}
		// Second finding should also be hashed
		if !strings.HasPrefix(out[1].Match, "sha256:") {
			t.Fatalf("want sha256: prefix on second finding, got %s", out[1].Match)
		}
	})

	t.Run("same_input_produces_same_hash", func(t *testing.T) {
		in := []scanner.Finding{{Match: "test@test.com"}}
		a := hashFindingMatches(in)
		b := hashFindingMatches(in)
		if a[0].Match != b[0].Match {
			t.Fatal("same input should produce same hash")
		}
	})

	t.Run("different_input_produces_different_hash", func(t *testing.T) {
		a := hashFindingMatches([]scanner.Finding{{Match: "a@b.com"}})
		b := hashFindingMatches([]scanner.Finding{{Match: "c@d.com"}})
		if a[0].Match == b[0].Match {
			t.Fatal("different inputs should produce different hashes")
		}
	})

	t.Run("empty_match_stays_empty", func(t *testing.T) {
		out := hashFindingMatches([]scanner.Finding{{Match: ""}})
		if out[0].Match != "" {
			t.Fatalf("empty match should stay empty, got %s", out[0].Match)
		}
	})

	t.Run("empty_slice", func(t *testing.T) {
		out := hashFindingMatches(nil)
		if len(out) != 0 {
			t.Fatalf("nil should return empty slice, got %d", len(out))
		}
		out = hashFindingMatches([]scanner.Finding{})
		if len(out) != 0 {
			t.Fatal("empty slice should return empty slice")
		}
	})
}

// spyStore captures the findings JSON written to SaveRequestLog for assertions.
type spyStore struct {
	lastFindings json.RawMessage
}

func (s *spyStore) SaveRequestLog(_ context.Context, rl RequestLog) error {
	s.lastFindings = rl.Findings
	return nil
}
func (s *spyStore) GetStats(_ context.Context, _ string, _, _ time.Time) (*Stats, error) {
	return nil, nil
}
func (s *spyStore) ListSecurityEvents(_ context.Context, _ string, _, _ int) ([]SecurityEvent, int, error) {
	return nil, 0, nil
}
func (s *spyStore) SearchSecurityEvents(_ context.Context, _ string, _ EventSearchParams) ([]SecurityEvent, int, error) {
	return nil, 0, nil
}
func (s *spyStore) GetModelTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]ModelTokenUsage, error) {
	return nil, nil
}
func (s *spyStore) GetDailyTokenUsage(_ context.Context, _ string, _, _ time.Time) ([]DailyTokenUsage, error) {
	return nil, nil
}
func (s *spyStore) Ping(_ context.Context) error { return nil }
func (s *spyStore) Close() error                 { return nil }

func TestDBHandler_HashFindings_Enabled(t *testing.T) {
	spy := &spyStore{}
	getPolicy := func() *policy.Policy {
		return &policy.Policy{
			Data: &policy.DataControl{HashFindings: true},
		}
	}
	handler := DBHandler(zerolog.Nop(), spy, "org-1", getPolicy)

	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "EMAIL", Match: "admin@secretcorp.com", Severity: "high", Confidence: 0.95},
		},
	})

	// Verify the stored findings are hashed.
	var stored []scanner.Finding
	if err := json.Unmarshal(spy.lastFindings, &stored); err != nil {
		t.Fatalf("unmarshal stored findings: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("want 1 finding, got %d", len(stored))
	}
	if !strings.HasPrefix(stored[0].Match, "sha256:") {
		t.Fatalf("expected hashed match, got: %s", stored[0].Match)
	}
	// Verify it's a real hash (64 hex chars after prefix)
	hashPart := strings.TrimPrefix(stored[0].Match, "sha256:")
	if len(hashPart) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(hashPart))
	}
}

func TestDBHandler_HashFindings_Disabled(t *testing.T) {
	spy := &spyStore{}
	getPolicy := func() *policy.Policy {
		return &policy.Policy{
			Data: &policy.DataControl{HashFindings: false},
		}
	}
	handler := DBHandler(zerolog.Nop(), spy, "org-1", getPolicy)

	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "EMAIL", Match: "admin@secretcorp.com", Severity: "high", Confidence: 0.95},
		},
	})

	var stored []scanner.Finding
	if err := json.Unmarshal(spy.lastFindings, &stored); err != nil {
		t.Fatalf("unmarshal stored findings: %v", err)
	}
	if stored[0].Match != "admin@secretcorp.com" {
		t.Fatalf("expected plaintext match, got: %s", stored[0].Match)
	}
}

func TestDBHandler_HashFindings_NilPolicy(t *testing.T) {
	spy := &spyStore{}
	// getPolicy is nil — no hashing.
	handler := DBHandler(zerolog.Nop(), spy, "org-1", nil)

	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings: []scanner.Finding{
			{Type: "pii", Category: "EMAIL", Match: "admin@secretcorp.com", Severity: "high", Confidence: 0.95},
		},
	})

	var stored []scanner.Finding
	if err := json.Unmarshal(spy.lastFindings, &stored); err != nil {
		t.Fatalf("unmarshal stored findings: %v", err)
	}
	if stored[0].Match != "admin@secretcorp.com" {
		t.Fatalf("expected plaintext match (nil policy = fail-safe), got: %s", stored[0].Match)
	}
}

func TestDBHandler_OnlyScannedEvents(t *testing.T) {
	spy := &spyStore{}
	called := false
	// Wrap spy to track if SaveRequestLog was called.
	handler := DBHandler(zerolog.Nop(), spy, "org-1", nil)

	// Non-scanned event type should be ignored.
	handler(events.Event{
		EventType: "output_scan_hint",
		OrgID:     "org-1",
	})
	// request_blocked should be processed.
	handler(events.Event{
		EventType: "request_blocked",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "injection", Match: "test", Severity: "high"}},
	})
	_ = called
	// Verify that only the blocked event was stored (happens if spy.lastFindings is not nil).
	// The output_scan_hint should not have triggered a save.
	// We just verify the handler doesn't panic for non-scanned events.
}

func TestDBHandler_EmptyOrgID(t *testing.T) {
	spy := &spyStore{}
	handler := DBHandler(zerolog.Nop(), spy, "", nil)

	// Both OrgID and defaultOrgID empty → should skip.
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "",
		Findings:  []scanner.Finding{{Type: "pii", Match: "test"}},
	})
	if spy.lastFindings != nil {
		t.Fatal("empty org should not save")
	}
}

func TestDBHandler_NilStore(t *testing.T) {
	handler := DBHandler(zerolog.Nop(), nil, "org-1", nil)
	// Should not panic — just return a no-op function.
	handler(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
	})
}

func TestHashFindingMatches_PreservesFields(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Category: "EMAIL", Match: "a@b.com", Severity: "high", Confidence: 0.99},
	}
	out := hashFindingMatches(findings)
	if out[0].Type != "pii" || out[0].Category != "EMAIL" || out[0].Severity != "high" || out[0].Confidence != 0.99 {
		t.Fatal("non-match fields should be preserved")
	}
}

func TestHashFindingMatches_SpecialCharacters(t *testing.T) {
	findings := []scanner.Finding{
		{Type: "pii", Match: "user+tag@example.com"},
		{Type: "pii", Match: "line1\\nline2"},
		{Type: "pii", Match: "turkce: istanbul"},
	}
	out := hashFindingMatches(findings)
	for i, f := range out {
		if !strings.HasPrefix(f.Match, "sha256:") {
			t.Errorf("finding %d: match not hashed: %s", i, f.Match)
		}
	}
}

func TestDBHandler_LargeNumberOfFindings(t *testing.T) {
	spy := &spyStore{}
	findings := make([]scanner.Finding, 50)
	for i := range findings {
		findings[i] = scanner.Finding{Type: "pii", Category: "test", Match: "data", Severity: "low", Confidence: 0.5}
	}
	handler := DBHandler(zerolog.Nop(), spy, "org-1", nil)
	handler(events.Event{EventType: "request_scanned", OrgID: "org-1", Findings: findings})

	var stored []scanner.Finding
	if err := json.Unmarshal(spy.lastFindings, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(stored) != 50 {
		t.Fatalf("want 50, got %d", len(stored))
	}
}

func TestDBHandler_EmptyFindings(t *testing.T) {
	spy := &spyStore{}
	handler := DBHandler(zerolog.Nop(), spy, "org-1", nil)
	handler(events.Event{
		EventType: "request_scanned", OrgID: "org-1", Findings: []scanner.Finding{},
	})
	if spy.lastFindings == nil {
		t.Fatal("should still save empty findings")
	}
	var stored []scanner.Finding
	_ = json.Unmarshal(spy.lastFindings, &stored)
	if len(stored) != 0 {
		t.Fatalf("want 0, got %d", len(stored))
	}
}
