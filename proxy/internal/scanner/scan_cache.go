package scanner

import (
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"time"
)

// An agent resends its whole history with every request: the same file
// contents and tool output, turn after turn, with a little new text at the
// end. Scanning all of it again each time costs seconds per megabyte. The
// built-in scanners are pure functions of the text, so what they found in a
// piece of text is remembered for a while and reused.
//
// Only text of some length is remembered; a chat message scans faster than
// it hashes. Scanners whose answer depends on anything but the text (the
// request, the policy, patterns edited at runtime) are never cached.
const (
	scanCacheMinBytes = 2 << 10
	scanCacheTTL      = 10 * time.Minute
	scanCacheMaxItems = 16384
)

type scanCacheKey struct {
	scanner string
	sum     [sha256.Size]byte
}

type scanCacheItem struct {
	findings []Finding
	stored   time.Time
}

var scanCache = struct {
	mu    sync.Mutex
	items map[scanCacheKey]scanCacheItem
	hits  atomic.Int64
	miss  atomic.Int64
}{items: make(map[scanCacheKey]scanCacheItem)}

// pureScannerName returns the cache name of a scanner whose findings depend
// on the text alone, and false for every other scanner.
func pureScannerName(s Scanner) (string, bool) {
	switch s.(type) {
	case *PIIScanner, *SecretScanner, *InjectionScanner, *JailbreakScanner, *ContentModerationScanner:
		return s.Name(), true
	}
	return "", false
}

// ScanCacheStats returns how many scans were answered from the cache and how
// many were not.
func ScanCacheStats() (hits, misses int64) {
	return scanCache.hits.Load(), scanCache.miss.Load()
}

// ResetScanCache empties the cache. Tests use it; so does a change that
// alters what a pure scanner finds (the phrase automaton being rebuilt).
func ResetScanCache() {
	scanCache.mu.Lock()
	scanCache.items = make(map[scanCacheKey]scanCacheItem)
	scanCache.mu.Unlock()
}

func scanCacheGet(key scanCacheKey) ([]Finding, bool) {
	scanCache.mu.Lock()
	item, ok := scanCache.items[key]
	scanCache.mu.Unlock()
	if !ok || time.Since(item.stored) > scanCacheTTL {
		scanCache.miss.Add(1)
		return nil, false
	}
	scanCache.hits.Add(1)
	return cloneFindings(item.findings), true
}

func scanCachePut(key scanCacheKey, findings []Finding) {
	stored := cloneFindings(findings)
	now := time.Now()
	scanCache.mu.Lock()
	defer scanCache.mu.Unlock()
	if len(scanCache.items) >= scanCacheMaxItems {
		for k, v := range scanCache.items {
			if now.Sub(v.stored) > scanCacheTTL {
				delete(scanCache.items, k)
			}
		}
		if len(scanCache.items) >= scanCacheMaxItems {
			scanCache.items = make(map[scanCacheKey]scanCacheItem)
		}
	}
	scanCache.items[key] = scanCacheItem{findings: stored, stored: now}
}

// cloneFindings copies findings deeply enough that the caller can place,
// score and annotate them without touching the cached originals.
func cloneFindings(in []Finding) []Finding {
	if in == nil {
		return nil
	}
	out := make([]Finding, len(in))
	copy(out, in)
	for i := range out {
		if out[i].ConfidenceScore != nil {
			score := *out[i].ConfidenceScore
			out[i].ConfidenceScore = &score
		}
		if out[i].Metadata != nil {
			out[i].Metadata = copyMeta(out[i].Metadata)
		}
	}
	return out
}
