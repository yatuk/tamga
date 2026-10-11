package scanner

import (
	"context"
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
// it hashes. A scanner whose answer depends on the request is never cached.
// One whose answer depends on the text and on a set of patterns that can
// change (the policy's custom entities and competitors, the pattern store)
// is cached under the fingerprint of the set it scanned with: after a change
// the same text asks under another key and is scanned, and nothing has to be
// evicted (see patternSetScanner).
const (
	scanCacheMinBytes = 2 << 10
	scanCacheTTL      = 10 * time.Minute
	scanCacheMaxItems = 16384
)

type scanCacheKey struct {
	scanner string
	// rules is the fingerprint of the pattern set the findings were made
	// with; empty for a scanner whose patterns are built in.
	rules string
	sum   [sha256.Size]byte
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

// patternSetScanner is a scanner whose findings depend on the text and on a
// set of patterns that changes while the proxy runs.
//
// patternSnapshot returns the fingerprint of the set the scanner has
// compiled and a scan that uses exactly that set, taken together under the
// scanner's lock. That pairing is what makes the cache exact: the findings
// stored under a fingerprint were made with the patterns it names, also when
// the policy changes while the scan runs. Two sets with the same fingerprint
// are the same names, patterns, severities and actions. scan is nil when
// there are no patterns, and then there is nothing to scan or to remember.
type patternSetScanner interface {
	Scanner
	patternSnapshot() (fingerprint string, scan func(ctx context.Context, content []byte) ([]Finding, error))
}

// cachedScan runs one scanner over content, answering from the scan cache
// when it can.
func cachedScan(ctx context.Context, s Scanner, content []byte) ([]Finding, error) {
	if len(content) < scanCacheMinBytes {
		return s.Scan(ctx, content)
	}
	key := scanCacheKey{}
	scan := s.Scan
	if ps, ok := s.(patternSetScanner); ok {
		fp, bound := ps.patternSnapshot()
		if bound == nil {
			return nil, nil
		}
		key.scanner, key.rules, scan = s.Name(), fp, bound
	} else if name, pure := pureScannerName(s); pure {
		key.scanner = name
	} else {
		return s.Scan(ctx, content)
	}
	key.sum = contentSum(ctx, content)
	if found, ok := scanCacheGet(key); ok {
		return found, nil
	}
	found, err := scan(ctx, content)
	if err == nil {
		scanCachePut(key, found)
	}
	return found, err
}

// ScanCacheStats returns how many scans were answered from the cache and how
// many were not.
func ScanCacheStats() (hits, misses int64) {
	return scanCache.hits.Load(), scanCache.miss.Load()
}

// ResetScanCache empties the cache. Tests use it; so does a change that
// alters what a pure scanner finds (the phrase automaton being rebuilt). A
// change of the policy's patterns needs no reset: it changes the key.
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
