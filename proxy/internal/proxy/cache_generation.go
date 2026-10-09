package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// A cache hit is served without the answer being scanned again, so an entry
// is only good under the rules it was scanned with. Those rules go into the
// cache key (cache.KeyParts.Policy and Scan): after a change, the same
// request asks under another key and misses, and nothing has to be evicted.
// Both values are made from content alone, so replicas that share Redis share
// entries exactly when they run the same rules.

// policyGeneration pairs a loaded policy with the digest of its content.
type policyGeneration struct {
	// pol is held, not just compared: while it is here the address cannot be
	// given to another policy.
	pol    *policy.Policy
	digest string
}

// policyDigests remembers the digest of the policy last asked about. A loaded
// policy is never changed in place (a reload installs a new one), so the
// digest is worked out once per load and not once per request.
var policyDigests struct {
	mu   sync.Mutex
	last atomic.Pointer[policyGeneration]
	// computed counts how often a digest was worked out.
	computed atomic.Int64
}

// policyDigest names the content of pol. It returns "" when the policy cannot
// be named, and a request without a name for its policy does not use the
// cache.
//
// The digest is taken over the decoded policy, not the file: comments,
// spacing and the order of keys do not change it. The order of a list does
// (custom_entities, block_on); a policy with a list reordered is another
// policy here, and the cost of that is one miss per entry.
func policyDigest(pol *policy.Policy) string {
	if pol == nil {
		return ""
	}
	if g := policyDigests.last.Load(); g != nil && g.pol == pol {
		return g.digest
	}
	policyDigests.mu.Lock()
	defer policyDigests.mu.Unlock()
	if g := policyDigests.last.Load(); g != nil && g.pol == pol {
		return g.digest
	}
	g := &policyGeneration{pol: pol, digest: computePolicyDigest(pol)}
	policyDigests.computed.Add(1)
	policyDigests.last.Store(g)
	return g.digest
}

// computePolicyDigest hashes the policy as JSON, which writes struct fields
// in a fixed order and map keys sorted.
func computePolicyDigest(pol *policy.Policy) string {
	raw, err := json.Marshal(pol)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// cacheScanState names what the scanners work from besides the policy: the
// scanner build, the card-issuer dataset built into it, and whatever the
// deployment reports through HandlerConfig.ScanState.
func cacheScanState(cfg HandlerConfig) string {
	state := scanner.ScannerVersion + "/" + scanner.BINDatasetVersion
	if cfg.ScanState != nil {
		state += "/" + cfg.ScanState()
	}
	return state
}

// cacheRulesUnchanged reports whether the rules a cache key was made under
// are still the ones in force: the same loaded policy and the same scan
// state. The scanners read some of their rules live, so an answer that came
// back after a change may have been scanned under either set.
func cacheRulesUnchanged(cfg HandlerConfig, pol *policy.Policy, scanState string) bool {
	return cfg.GetPolicy() == pol && cacheScanState(cfg) == scanState
}

// CustomSpecsDigest names a set of custom patterns by content, whatever order
// they are listed in. A HandlerConfig.ScanState can be built from it; the
// proxy binary reports what its scanners have compiled instead.
func CustomSpecsDigest(specs []scanner.CustomEntitySpec) string {
	rows := make([]string, 0, len(specs))
	for _, sp := range specs {
		rows = append(rows, strconv.Quote(sp.Name)+" "+strconv.Quote(sp.Pattern)+" "+strconv.Quote(sp.Severity)+" "+
			strconv.FormatFloat(sp.Confidence, 'g', -1, 64))
	}
	sort.Strings(rows)
	sum := sha256.Sum256([]byte(strings.Join(rows, "\n")))
	return hex.EncodeToString(sum[:])
}
