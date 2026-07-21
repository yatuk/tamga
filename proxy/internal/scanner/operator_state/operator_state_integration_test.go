package operator_state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/scanner"
)

// fixturePath resolves a bundled jugeni-contracts fixture.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "testdata", "operator_state", name)
}

// copyFixture copies a bundled fixture into dir so the test can append to it.
func copyFixture(t *testing.T, name, dir string) string {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
	return dst
}

// TestIntegration_FixtureReplayToScan drives the full path end-to-end:
// bundled jugeni-contracts fixtures → Watcher replay → Projection →
// scanner with a RequestContext → findings, then a live tail append and
// re-scan, asserting the deep-analysis flag and publisher hook.
func TestIntegration_FixtureReplayToScan(t *testing.T) {
	for _, forcePoll := range []bool{false, true} {
		name := "platform_default"
		if forcePoll {
			name = "forced_polling"
		}
		t.Run(name, func(t *testing.T) {
			if forcePoll {
				t.Setenv("TAMGA_OPERATOR_STATE_FORCE_POLL", "1")
			}

			dir := t.TempDir()
			decisionsPath := copyFixture(t, "decisions.jsonl", dir)
			notesPath := copyFixture(t, "notes.jsonl", dir)

			cfg := Config{
				DecisionsPath: decisionsPath,
				NotesPath:     notesPath,
				PollInterval:  50 * time.Millisecond,
				Enabled:       true,
			}

			projection := NewProjection()
			var mu sync.Mutex
			applied := 0
			w, err := NewWatcher(cfg,
				func(ev DecisionEvent) {
					mu.Lock()
					applied++
					mu.Unlock()
					projection.ApplyDecision(ev)
				},
				func(ev NoteEvent) { projection.ApplyNote(ev) })
			if err != nil {
				t.Fatalf("NewWatcher: %v", err)
			}
			if err := w.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer w.Stop()
			if err := w.WaitInitial(); err != nil {
				t.Fatalf("WaitInitial: %v", err)
			}

			// Fixture ground truth: 13 decision events across D-001..D-004,
			// 7 note events; D-001 ends superseded, D-004 ends locked.
			decisions, notes := projection.Stats()
			if decisions != 4 {
				t.Fatalf("projected decisions = %d, want 4", decisions)
			}
			if notes != 6 {
				t.Fatalf("projected notes = %d, want 6", notes)
			}
			if rec := projection.GetDecision("D-2026-06-23-004"); rec == nil || rec.State != StateLocked {
				t.Fatalf("D-004 state = %+v, want locked", rec)
			}
			if rec := projection.GetDecision("D-2026-06-23-001"); rec == nil || rec.State != StateSuperseded {
				t.Fatalf("D-001 state = %+v, want superseded", rec)
			}

			// Scanner with the shipped default rule (all decisions locked).
			rule := AssertionRule{DecisionPattern: "D-.*", RequiredState: "locked", ActionOnFail: "block", Severity: "critical", Description: "decisions must be locked"}
			if err := rule.Compile(); err != nil {
				t.Fatalf("compile: %v", err)
			}
			s := NewOperatorStateScannerWithRules(projection, &ScannerRules{Assertions: []AssertionRule{rule}})

			var published *AnalyzerRequest
			s.SetDeepAnalysisPublisher(func(req *AnalyzerRequest) { published = req })

			// D-001 is superseded: assertion fails, the finding is flagged for
			// deep analysis, and the analyzer request is published.
			reqCtx := &scanner.RequestContext{RequestId: "it-1", OperatorId: "mike", ActiveDecisionIds: []string{"D-2026-06-23-001"}}
			findings, err := s.ScanWithContext(context.Background(), []byte("proceed per D-2026-06-23-001"), reqCtx)
			if err != nil {
				t.Fatalf("ScanWithContext: %v", err)
			}
			assertHasCategory(t, findings, "state_assertion_failed")
			assertHasCategory(t, findings, "inactive_decision_in_active_set")
			if findings[0].Metadata["needs_deep_analysis"] != "true" {
				t.Error("findings not flagged for deep analysis")
			}
			if published == nil || published.RequestID != "it-1" {
				t.Fatalf("analyzer request not published: %+v", published)
			}

			// D-004 is locked: clean scan.
			findings, err = s.ScanWithContext(context.Background(), []byte("proceed per D-2026-06-23-004"), &scanner.RequestContext{RequestId: "it-2"})
			if err != nil {
				t.Fatalf("ScanWithContext: %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("expected clean scan for locked D-004, got %+v", findings)
			}

			// Live tail: lock a new decision and re-scan once projected.
			mu.Lock()
			before := applied
			mu.Unlock()
			appendEvent := func(line string) {
				f, err := os.OpenFile(decisionsPath, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatalf("open append: %v", err)
				}
				defer f.Close()
				if _, err := fmt.Fprintln(f, line); err != nil {
					t.Fatalf("append: %v", err)
				}
			}
			now := time.Now().Format(time.RFC3339)
			appendEvent(`{"ts": "` + now + `", "action": "propose", "decision": "D-2026-07-21-001", "detail": "tail test"}`)
			appendEvent(`{"ts": "` + now + `", "action": "lock", "decision": "D-2026-07-21-001", "detail": ""}`)

			deadline := time.Now().Add(5 * time.Second)
			for {
				mu.Lock()
				got := applied
				mu.Unlock()
				if got >= before+2 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("tail events not applied within deadline (applied=%d, want >=%d)", got, before+2)
				}
				time.Sleep(cfg.PollInterval)
			}

			findings, err = s.ScanWithContext(context.Background(), []byte("apply D-2026-07-21-001"), &scanner.RequestContext{RequestId: "it-3"})
			if err != nil {
				t.Fatalf("ScanWithContext: %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("tailed locked decision should scan clean, got %+v", findings)
			}
		})
	}
}

func assertHasCategory(t *testing.T, findings []scanner.Finding, category string) {
	t.Helper()
	for _, f := range findings {
		if f.Category == category {
			return
		}
	}
	t.Errorf("no finding with category %q in %+v", category, findings)
}
