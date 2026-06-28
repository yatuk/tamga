package webhooks

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// CorrelationEngine tracks sliding-window event counts to suppress duplicate
// webhook firings within a correlation window. It is safe for concurrent use.
//
// The engine maps (webhookID + correlationKey) to a sliding window of
// timestamps. When ShouldFire returns true, the window's count is included
// in the webhook payload as "correlated_count" so SIEM receivers get
// situational awareness.
type CorrelationEngine struct {
	mu sync.Mutex

	// entries maps "<webhookID>/<correlationKey>" to a sliding window of event
	// timestamps. Timestamps older than the oldest active window are pruned
	// on ShouldFire calls.
	entries map[string]*correlationEntry

	// maxKeys caps the number of tracked keys. When exceeded, the LRU entry
	// is evicted and a WARN is logged.
	maxKeys int

	log *slog.Logger
}

// correlationEntry holds a sliding window of event timestamps and the last
// fire time for cooldown enforcement.
type correlationEntry struct {
	// timestamps is an ordered ring of recent event times within the window.
	timestamps []time.Time

	// lastFire records when this key last caused a webhook to fire.
	lastFire time.Time

	// lastAccess supports LRU eviction.
	lastAccess time.Time
}

// defaultMaxKeys is the maximum number of correlation keys tracked in memory.
const defaultMaxKeys = 10000

// NewCorrelationEngine creates a correlation engine with maxKeys tracked
// correlation entries. Pass 0 for the default (10000).
func NewCorrelationEngine(maxKeys int) *CorrelationEngine {
	if maxKeys <= 0 {
		maxKeys = defaultMaxKeys
	}
	return &CorrelationEngine{
		entries: make(map[string]*correlationEntry),
		maxKeys: maxKeys,
		log:     slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
}

// ShouldFire determines whether a webhook should fire for the given
// correlation key.
//
// Parameters:
//   - webhookID: the webhook's unique identifier
//   - correlationKey: a caller-defined grouping key (e.g. finding type + severity)
//   - thresholdCount: minimum events within the window before firing (0 = fire immediately)
//   - thresholdWindowSecs: observation window in seconds
//   - cooldownSecs: minimum seconds between re-fires for the same key
//
// Returns:
//   - true + correlatedCount when the threshold is met and cooldown has passed
//   - false + 0 when suppressed (threshold not met or cooldown active)
func (ce *CorrelationEngine) ShouldFire(webhookID, correlationKey string, thresholdCount int, thresholdWindowSecs int, cooldownSecs int) (bool, int) {
	if ce == nil {
		// No engine configured: always fire (backward-compatible).
		return true, 1
	}

	// Zero threshold means fire immediately — no correlation.
	if thresholdCount <= 0 {
		return true, 1
	}

	now := time.Now().UTC()
	fullKey := fmt.Sprintf("%s/%s", webhookID, correlationKey)

	ce.mu.Lock()
	defer ce.mu.Unlock()

	entry, ok := ce.entries[fullKey]
	if !ok {
		entry = &correlationEntry{
			timestamps: make([]time.Time, 0, thresholdCount+1),
		}
		// Evict LRU entry if at capacity.
		if len(ce.entries) >= ce.maxKeys {
			ce.evictLRU()
		}
		ce.entries[fullKey] = entry
	}

	entry.lastAccess = now

	// Prune timestamps outside the observation window.
	windowStart := now.Add(-time.Duration(thresholdWindowSecs) * time.Second)
	pruned := entry.timestamps[:0]
	for _, ts := range entry.timestamps {
		if ts.After(windowStart) {
			pruned = append(pruned, ts)
		}
	}
	entry.timestamps = pruned

	// Record this event.
	entry.timestamps = append(entry.timestamps, now)

	correlatedCount := len(entry.timestamps)

	// Not enough events yet.
	if correlatedCount < thresholdCount {
		return false, 0
	}

	// Check cooldown.
	if cooldownSecs > 0 && !entry.lastFire.IsZero() {
		cooldownUntil := entry.lastFire.Add(time.Duration(cooldownSecs) * time.Second)
		if now.Before(cooldownUntil) {
			return false, 0
		}
	}

	// Fire.
	entry.lastFire = now
	// Reset the window after firing so we start counting fresh.
	entry.timestamps = entry.timestamps[:0]

	return true, correlatedCount
}

// evictLRU removes the least-recently-accessed entry from the map. Caller
// must hold ce.mu.
func (ce *CorrelationEngine) evictLRU() {
	var oldestKey string
	var oldestAccess time.Time
	first := true
	for k, e := range ce.entries {
		if first || e.lastAccess.Before(oldestAccess) {
			oldestKey = k
			oldestAccess = e.lastAccess
			first = false
		}
	}
	if oldestKey != "" {
		delete(ce.entries, oldestKey)
		ce.log.Warn("correlation engine LRU eviction", "evicted_key", oldestKey)
	}
}

// Expire removes entries whose lastAccess is older than age. Call this
// periodically to prevent unbounded memory growth for stale keys.
func (ce *CorrelationEngine) Expire(age time.Duration) {
	if ce == nil {
		return
	}
	ce.mu.Lock()
	defer ce.mu.Unlock()
	cutoff := time.Now().UTC().Add(-age)
	for k, e := range ce.entries {
		if e.lastAccess.Before(cutoff) {
			delete(ce.entries, k)
		}
	}
}

// Size returns the current number of tracked correlation entries.
func (ce *CorrelationEngine) Size() int {
	if ce == nil {
		return 0
	}
	ce.mu.Lock()
	defer ce.mu.Unlock()
	return len(ce.entries)
}
