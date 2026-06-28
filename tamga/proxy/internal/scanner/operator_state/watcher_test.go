package operator_state

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/policy"
)

func TestWatcher_ReplayDecisions(t *testing.T) {
	// Create a temp file with sample decisions.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "decisions.jsonl")

	lines := []string{
		`{"ts": "2026-06-23T09:00:00+02:00", "action": "propose", "decision": "D-2026-06-23-001", "detail": ""}`,
		`{"ts": "2026-06-23T09:15:30+02:00", "action": "accept", "decision": "D-2026-06-23-001", "detail": "verified"}`,
		`{"ts": "2026-06-23T09:18:22+02:00", "action": "lock", "decision": "D-2026-06-23-001", "detail": ""}`,
	}
	writeLines(t, path, lines)

	var mu sync.Mutex
	var events []DecisionEvent

	cfg := Config{
		DecisionsPath: path,
		PollInterval:  100 * time.Millisecond,
		Enabled:       true,
	}

	w, err := NewWatcher(cfg, func(ev DecisionEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}, nil)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Wait for initial replay.
	if err := w.WaitInitial(); err != nil {
		t.Fatalf("WaitInitial: %v", err)
	}

	mu.Lock()
	count := len(events)
	mu.Unlock()

	if count != 3 {
		t.Errorf("expected 3 events from initial replay, got %d", count)
	}
}

func TestWatcher_TailDecisions(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "decisions.jsonl")

	// Start with one line.
	writeLines(t, path, []string{
		`{"ts": "2026-06-23T09:00:00+02:00", "action": "propose", "decision": "D-2026-06-23-001", "detail": ""}`,
	})

	var mu sync.Mutex
	var events []DecisionEvent

	cfg := Config{
		DecisionsPath: path,
		PollInterval:  50 * time.Millisecond, // fast polling for test
		Enabled:       true,
	}

	w, err := NewWatcher(cfg, func(ev DecisionEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}, nil)
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

	// Append another line.
	appendLine(t, path, `{"ts": "2026-06-23T09:15:30+02:00", "action": "accept", "decision": "D-2026-06-23-001", "detail": "verified"}`)

	// Wait for polling to pick it up.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(events)
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 events (1 replay + 1 tail), got %d", count)
	}
}

func TestWatcher_ReplayNotes(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "notes.jsonl")

	writeLines(t, path, []string{
		`{"ts": "2026-06-23T08:00:00+02:00", "action": "add", "note": "N-2026-06-23-001", "detail": "architecture"}`,
		`{"ts": "2026-06-23T08:30:14+02:00", "action": "add", "note": "N-2026-06-23-002", "detail": "pattern"}`,
	})

	var mu sync.Mutex
	var events []NoteEvent

	cfg := Config{
		NotesPath:    path,
		PollInterval: 100 * time.Millisecond,
		Enabled:      true,
	}

	w, err := NewWatcher(cfg, nil, func(ev NoteEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})
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

	mu.Lock()
	count := len(events)
	mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 events from replay, got %d", count)
	}
}

func TestWatcher_FileDoesNotExist(t *testing.T) {
	cfg := Config{
		DecisionsPath: "/nonexistent/path/decisions.jsonl",
		PollInterval:  100 * time.Millisecond,
		Enabled:       true,
	}

	w, err := NewWatcher(cfg, func(ev DecisionEvent) {}, nil)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}

	// Start should not error on missing file — it logs a warning and continues.
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start should succeed even with missing file: %v", err)
	}
	defer w.Stop()

	// WaitInitial should also not error.
	if err := w.WaitInitial(); err != nil {
		t.Errorf("WaitInitial should not error for missing file: %v", err)
	}
}

func TestWatcher_MalformedLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "decisions.jsonl")

	writeLines(t, path, []string{
		`{"ts": "2026-06-23T09:00:00+02:00", "action": "propose", "decision": "D-2026-06-23-001", "detail": ""}`,
		`{this is not valid json`,
		`{"ts": "2026-06-23T09:15:30+02:00", "action": "accept", "decision": "D-2026-06-23-001", "detail": "verified"}`,
	})

	var mu sync.Mutex
	var events []DecisionEvent

	cfg := Config{
		DecisionsPath: path,
		PollInterval:  100 * time.Millisecond,
		Enabled:       true,
	}

	w, err := NewWatcher(cfg, func(ev DecisionEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}, nil)
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

	mu.Lock()
	count := len(events)
	mu.Unlock()

	// Should get 2 valid events, malformed line skipped.
	if count != 2 {
		t.Errorf("expected 2 valid events (malformed skipped), got %d", count)
	}
}

func TestLoadAssertionsFromPolicy_NilConfig(t *testing.T) {
	rules, err := LoadAssertionsFromPolicy(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules != nil {
		t.Errorf("expected nil rules for nil config, got %d", len(rules))
	}
}

func TestLoadAssertionsFromPolicy_Disabled(t *testing.T) {
	cfg := &policy.OperatorStateConfig{Enabled: false, Assertions: []policy.OperatorStateAssertion{
		{DecisionPattern: "D-.*", RequiredState: "locked"},
	}}
	rules, err := LoadAssertionsFromPolicy(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules != nil {
		t.Errorf("expected nil rules when disabled, got %d", len(rules))
	}
}

func TestLoadAssertionsFromPolicy_Valid(t *testing.T) {
	cfg := &policy.OperatorStateConfig{Enabled: true, Assertions: []policy.OperatorStateAssertion{
		{DecisionPattern: "D-2026-06-.*", RequiredState: "locked", ActionOnFail: "block", Severity: "critical", Description: "June decisions locked"},
		{DecisionPattern: "D-.*", RequiredState: "accepted", ActionOnFail: "warn", Severity: "medium", Description: "Must be accepted"},
	}}
	rules, err := LoadAssertionsFromPolicy(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].compiledPattern == nil {
		t.Error("rule 0 pattern not compiled")
	}
	if rules[1].compiledPattern == nil {
		t.Error("rule 1 pattern not compiled")
	}
}

func TestLoadAssertionsFromPolicy_InvalidPattern(t *testing.T) {
	cfg := &policy.OperatorStateConfig{Enabled: true, Assertions: []policy.OperatorStateAssertion{
		{DecisionPattern: "[invalid", RequiredState: "locked"},
	}}
	_, err := LoadAssertionsFromPolicy(cfg)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

// helpers

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			f.Close()
			t.Fatalf("write line: %v", err)
		}
	}
	f.Close()
}

func appendLine(t *testing.T, path string, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append line: %v", err)
	}
}
