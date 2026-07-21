package operator_state

import "encoding/json"

// HashChainVerifier validates the SHA-256 hash chain for v2 audit log entries.
// In v1 this is a no-op placeholder. When jugeni-contracts v2 ships with
// prev_hash and entry_hash fields, the implementation is filled in here
// without changing any call sites.
type HashChainVerifier struct{}

// NewHashChainVerifier returns a no-op verifier for v1.
func NewHashChainVerifier() *HashChainVerifier {
	return &HashChainVerifier{}
}

// Verify checks that prev_hash and entry_hash form a valid chain link.
// Currently a no-op: always returns nil. In v2 this will verify:
//   - prev_hash == SHA-256(canonical JSON of previous entry)
//   - entry_hash == SHA-256(canonical JSON of this entry)
func (v *HashChainVerifier) Verify(prevHash, entryHash string) error {
	// v1: no-op — hash chain not yet shipped in jugeni-contracts.
	// v2: implement SHA-256 canonical JSON verification per the contract spec.
	_ = prevHash
	_ = entryHash
	return nil
}

// Canonicalize produces the canonical JSON representation of an entry
// for hash computation: UTF-8, no whitespace, keys sorted lexicographically.
// Currently returns the raw message re-marshalled. In v2 this will enforce
// key ordering and whitespace stripping per the contract spec.
func Canonicalize(raw json.RawMessage) ([]byte, error) {
	// Re-marshal to strip whitespace; encoding/json sorts keys by default.
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}
