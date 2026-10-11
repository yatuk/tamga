package store

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/yatuk/tamga/internal/scanner"
)

// With data.hash_findings on, a stored finding holds a digest of its match
// in place of the match. The match is what the scanner let out: for PII the
// masked value (the first and last two characters), so the digest is of
// that, never of the whole value.
//
// The digest is keyed. A plain SHA-256 of a value with few possibilities is
// undone by hashing the possibilities, so it kept what it was meant not to
// keep. HMAC-SHA-256 under a secret of the deployment still gives the same
// digest for the same value, which is what the digest is for (telling that
// two findings are of one value, and finding the rows of an identifier), and
// cannot be computed or enumerated without the key.
const (
	// KeyedMatchPrefix marks a match stored as HMAC-SHA-256.
	KeyedMatchPrefix = "hmac-sha256:"
	// LegacyMatchPrefix marks a match stored by earlier versions: an
	// unkeyed SHA-256. Such rows are not rewritten; they are still found.
	LegacyMatchPrefix = "sha256:"
	// UnkeyedMatch is stored in place of the match when hashing is asked
	// for and the deployment has no key. Nothing derived from the value is
	// kept: an unkeyed digest would be the reversible one again.
	UnkeyedMatch = "[not stored: no finding hash key]"

	// findingHashInfo separates the key derived from TAMGA_VAULT_KEY from
	// every other use of that key.
	findingHashInfo = "tamga finding match hash v1"

	// FindingHashKeyEnv is the variable of the dedicated key.
	FindingHashKeyEnv = "TAMGA_FINDING_HASH_KEY"
)

// FindingHasher computes the stored digest of a match.
type FindingHasher struct {
	key    []byte
	source string
}

// NewFindingHasher builds the hasher from the deployment's secrets, both in
// standard base64. The dedicated key (TAMGA_FINDING_HASH_KEY, 32 bytes or
// more) is used when set. Otherwise a key is derived from TAMGA_VAULT_KEY
// with HKDF-SHA-256, so a deployment that already has the vault key needs
// nothing new and the vault's own key is never used for a second purpose.
//
// It returns nil and no error when neither is set, and nil and an error
// when the one that is set is unusable. A dedicated key that is set and
// unusable is not replaced by the vault key: the digests would silently be
// under another key than the operator chose.
func NewFindingHasher(dedicatedB64, vaultKeyB64 string) (*FindingHasher, error) {
	if s := strings.TrimSpace(dedicatedB64); s != "" {
		key, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, errors.New(FindingHashKeyEnv + " is not standard base64")
		}
		if len(key) < 32 {
			return nil, fmt.Errorf("%s must decode to at least 32 bytes, got %d", FindingHashKeyEnv, len(key))
		}
		return &FindingHasher{key: key, source: FindingHashKeyEnv}, nil
	}
	if s := strings.TrimSpace(vaultKeyB64); s != "" {
		secret, err := base64.StdEncoding.DecodeString(s)
		if err != nil || len(secret) != 32 {
			return nil, errors.New("TAMGA_VAULT_KEY is not a base64 32-byte key, so no finding hash key can be derived from it")
		}
		key, err := hkdf.Key(sha256.New, secret, nil, findingHashInfo, 32)
		if err != nil {
			return nil, fmt.Errorf("derive finding hash key: %w", err)
		}
		return &FindingHasher{key: key, source: "TAMGA_VAULT_KEY (derived)"}, nil
	}
	return nil, nil
}

// Source names where the key came from. It never holds key material.
func (h *FindingHasher) Source() string {
	if h == nil {
		return ""
	}
	return h.source
}

// Hash returns the stored form of a match: the prefix and the HMAC in hex.
func (h *FindingHasher) Hash(match string) string {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(match))
	return KeyedMatchPrefix + hex.EncodeToString(mac.Sum(nil))
}

// legacyMatchHash is the stored form earlier versions wrote for a match.
func legacyMatchHash(match string) string {
	sum := sha256.Sum256([]byte(match))
	return LegacyMatchPrefix + hex.EncodeToString(sum[:])
}

// findingHasher is the hasher of this process. Rows are built in two places
// (the event bus and the NATS consumer) by functions that are called from
// many more, so the hasher is set once at startup rather than handed down.
var findingHasher atomic.Pointer[FindingHasher]

// unkeyedMatches counts the matches left out because there was no key.
var unkeyedMatches atomic.Uint64

// SetFindingHasher sets the hasher of this process; nil means no key.
func SetFindingHasher(h *FindingHasher) { findingHasher.Store(h) }

// CurrentFindingHasher returns the hasher of this process, nil without a key.
func CurrentFindingHasher() *FindingHasher { return findingHasher.Load() }

// UnkeyedMatches is how many matches were stored as UnkeyedMatch.
func UnkeyedMatches() uint64 { return unkeyedMatches.Load() }

// SubjectMatchForms returns every form in which the request log may hold a
// finding of the given identifier (a national id, as the person wrote it):
// the masked match (rows written with data.hash_findings off), its keyed
// digest when this process has a key, and the unkeyed digest of earlier
// versions. The identifier itself is in none of them.
//
// The scanner masks before a match leaves it, so the rows are found by the
// masked form. Two identifiers with the same first and last two characters
// have the same forms.
func SubjectMatchForms(identifier string) []string {
	masked := scanner.MaskMatch(identifier)
	forms := []string{masked, legacyMatchHash(masked)}
	if h := CurrentFindingHasher(); h != nil {
		forms = append(forms, h.Hash(masked))
	}
	return forms
}

// matchContainmentDocs turns stored match values into the JSON documents a
// findings column is tested against with @>: one finding with that match.
func matchContainmentDocs(forms []string) []string {
	docs := make([]string, 0, len(forms))
	for _, f := range forms {
		if f == "" {
			continue
		}
		b, err := json.Marshal([]map[string]string{{"match": f}})
		if err != nil {
			continue
		}
		docs = append(docs, string(b))
	}
	return docs
}
