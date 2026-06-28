package operator_state

import (
	"os"
	"strconv"
	"time"
)

// Config holds the operator-state audit log consumer configuration.
type Config struct {
	// DecisionsPath is the filesystem path to the decisions JSONL file.
	// Empty means the decisions stream is not consumed.
	DecisionsPath string

	// NotesPath is the filesystem path to the notes JSONL file.
	// Empty means the notes stream is not consumed.
	NotesPath string

	// PollInterval is the interval between stat() polls when fsnotify
	// is unavailable (Windows, or when TAMGA_OPERATOR_STATE_FORCE_POLL is set).
	// Default 1s.
	PollInterval time.Duration

	// RedisEnabled reports whether Redis is available for state persistence.
	RedisEnabled bool

	// Enabled reports whether at least one audit log path is configured.
	Enabled bool
}

// LoadConfig reads operator-state configuration from environment variables.
func LoadConfig() Config {
	cfg := Config{
		DecisionsPath: envOrDefault("TAMGA_OPERATOR_STATE_DECISIONS_PATH", ""),
		NotesPath:     envOrDefault("TAMGA_OPERATOR_STATE_NOTES_PATH", ""),
		PollInterval:  time.Duration(envOrDefaultInt("TAMGA_OPERATOR_STATE_POLL_INTERVAL_MS", 1000)) * time.Millisecond,
	}
	cfg.Enabled = cfg.DecisionsPath != "" || cfg.NotesPath != ""
	return cfg
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
