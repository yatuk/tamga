package operator_state

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Ensure env vars are unset.
	for _, key := range []string{
		"TAMGA_OPERATOR_STATE_DECISIONS_PATH",
		"TAMGA_OPERATOR_STATE_NOTES_PATH",
		"TAMGA_OPERATOR_STATE_POLL_INTERVAL_MS",
	} {
		os.Unsetenv(key)
	}

	cfg := LoadConfig()

	if cfg.Enabled {
		t.Error("config should be disabled when no paths are set")
	}
	if cfg.DecisionsPath != "" {
		t.Errorf("DecisionsPath = %q, want empty", cfg.DecisionsPath)
	}
	if cfg.NotesPath != "" {
		t.Errorf("NotesPath = %q, want empty", cfg.NotesPath)
	}
	if cfg.PollInterval != 1*time.Second {
		t.Errorf("PollInterval = %v, want 1s", cfg.PollInterval)
	}
}

func TestLoadConfig_WithPaths(t *testing.T) {
	os.Setenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH", "/var/jugeni/decisions.jsonl")
	os.Setenv("TAMGA_OPERATOR_STATE_NOTES_PATH", "/var/jugeni/notes.jsonl")
	defer func() {
		os.Unsetenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH")
		os.Unsetenv("TAMGA_OPERATOR_STATE_NOTES_PATH")
	}()

	cfg := LoadConfig()

	if !cfg.Enabled {
		t.Error("config should be enabled when paths are set")
	}
	if cfg.DecisionsPath != "/var/jugeni/decisions.jsonl" {
		t.Errorf("DecisionsPath = %q", cfg.DecisionsPath)
	}
	if cfg.NotesPath != "/var/jugeni/notes.jsonl" {
		t.Errorf("NotesPath = %q", cfg.NotesPath)
	}
}

func TestLoadConfig_CustomPollInterval(t *testing.T) {
	os.Setenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH", "/tmp/test.jsonl")
	os.Setenv("TAMGA_OPERATOR_STATE_POLL_INTERVAL_MS", "500")
	defer func() {
		os.Unsetenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH")
		os.Unsetenv("TAMGA_OPERATOR_STATE_POLL_INTERVAL_MS")
	}()

	cfg := LoadConfig()

	if cfg.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 500ms", cfg.PollInterval)
	}
}

func TestLoadConfig_EnabledWithOnlyDecisions(t *testing.T) {
	os.Setenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH", "/tmp/d.jsonl")
	os.Unsetenv("TAMGA_OPERATOR_STATE_NOTES_PATH")
	defer os.Unsetenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH")

	cfg := LoadConfig()

	if !cfg.Enabled {
		t.Error("config should be enabled when decisions path is set")
	}
}

func TestLoadConfig_EnabledWithOnlyNotes(t *testing.T) {
	os.Unsetenv("TAMGA_OPERATOR_STATE_DECISIONS_PATH")
	os.Setenv("TAMGA_OPERATOR_STATE_NOTES_PATH", "/tmp/n.jsonl")
	defer os.Unsetenv("TAMGA_OPERATOR_STATE_NOTES_PATH")

	cfg := LoadConfig()

	if !cfg.Enabled {
		t.Error("config should be enabled when notes path is set")
	}
}
