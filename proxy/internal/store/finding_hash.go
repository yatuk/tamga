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

	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/scanner"
)

// With data.hash_findings on, a finding holds a digest of its match in
// place of the match, wherever the finding is kept or sent: the event bus
// replaces it before any subscriber has the event (EventFindingHasher), and
// the request log does the same for an event that reached it another way.
// The match is what the scanner let out: for PII the masked value (the first
// and last two characters), so the digest is of that, never of the whole
// value. What a finding's metadata says about the value goes the same way
// (see findingMetadataKept).
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

// findingHasher is the hasher of this process. Findings are hashed in more
// than one place (the event bus, and the two writers of the request log for
// an event the bus did not hash) by functions that are called from many
// more, so the hasher is set once at startup rather than handed down.
var findingHasher atomic.Pointer[FindingHasher]

// unkeyedMatches counts the matches left out because there was no key.
var unkeyedMatches atomic.Uint64

// unkeyedLogged is set once LogUnkeyedMatches has spoken.
var unkeyedLogged atomic.Bool

// SetFindingHasher sets the hasher of this process; nil means no key.
func SetFindingHasher(h *FindingHasher) {
	findingHasher.Store(h)
	unkeyedLogged.Store(false)
}

// CurrentFindingHasher returns the hasher of this process, nil without a key.
func CurrentFindingHasher() *FindingHasher { return findingHasher.Load() }

// UnkeyedMatches is how many matches were stored as UnkeyedMatch.
func UnkeyedMatches() uint64 { return unkeyedMatches.Load() }

// LogUnkeyedMatches says, at error level, that data.hash_findings is on and
// there is no key to hash with. It speaks once per process, whoever calls
// it and however often: the bus and both writers of the request log do.
func LogUnkeyedMatches(log zerolog.Logger) {
	if !unkeyedLogged.CompareAndSwap(false, true) {
		return
	}
	log.Error().Str("component", "findings").
		Msg("data.hash_findings is on and no finding hash key is set: matches are replaced by \"" + UnkeyedMatch +
			"\" wherever a finding is kept or sent, and cannot be looked up; set " + FindingHashKeyEnv + " or TAMGA_VAULT_KEY")
}

// findingMetadataKept lists the metadata keys of a finding that are kept
// when its match is hashed: the ones that say how the finding was made (the
// way the value was written, a validator's result, the state a rule
// compared) and nothing about what the value is.
//
// Any other key is dropped, a key no scanner emits yet included: a new key
// is not kept until it is listed here. Dropped today:
//
//   - bin, bank_name, country (credit card): the first six or eight digits
//     of the number, and the issuer and country that follow from them.
//   - denylist_reason (national id): every reason belongs to one number, and
//     one of them names its holder.
//   - decision_id, note_id (operator state): the match again, in the clear.
//   - detail, message (operator state): free text that repeats the id, the
//     operator and the audit log's own wording.
var findingMetadataKept = map[string]bool{
	// pii, secret: how the text had to be read before the pattern matched.
	"detected_via": true,
	// pii: the value is on the list of known test numbers.
	"denylist_match": true,
	// credit card: the network and whether it is a credit or a debit card.
	// They say what kind of card, as the category says what kind of value.
	"card_brand": true,
	"card_type":  true,
	// Segment scan: findings merged into this one, segments it spans.
	"variants":       true,
	"spans_segments": true,
	// jailbreak: the encoding the phrase was found under.
	"layer": true,
	// Hidden text and unreadable blocks: what kind, and how much of it.
	"kind":   true,
	"count":  true,
	"blocks": true,
	"chars":  true,
	// classifier: the signal that made a low score reportable.
	"signal":    true,
	"threshold": true,
	// operator state: the states compared and the rule that compared them.
	"current_state":       true,
	"required_state":      true,
	"note_state":          true,
	"rule":                true,
	"action_on_fail":      true,
	"last_verified":       true,
	"ttl_seconds":         true,
	"needs_deep_analysis": true,
	// scanner service: the reasoning of the confidence score, which is
	// built from weights only.
	"confidence_score_total": true,
}

// findingMetadataHashed lists the metadata keys that name a person and are
// kept as a keyed digest, so that two findings of one person can still be
// told to be: operator_id is who the request said was acting (operator
// state). Without a key they are dropped.
var findingMetadataHashed = map[string]bool{
	"operator_id": true,
}

// hashFindingMatches returns a copy of findings with each Match replaced by
// its keyed digest (see FindingHasher) and each Metadata reduced to what
// describes the detection (see findingMetadataKept). Without a key the
// match is replaced by UnkeyedMatch: the finding keeps its type, category
// and severity and nothing derived from the value. The findings given are
// not changed, nor are their metadata maps.
func hashFindingMatches(findings []scanner.Finding) []scanner.Finding {
	h := CurrentFindingHasher()
	out := make([]scanner.Finding, len(findings))
	for i, f := range findings {
		out[i] = f
		out[i].Metadata = hashedFindingMetadata(h, f.Metadata)
		if f.Match == "" {
			continue
		}
		if h == nil {
			out[i].Match = UnkeyedMatch
			unkeyedMatches.Add(1)
			continue
		}
		out[i].Match = h.Hash(f.Match)
	}
	return out
}

// hashedFindingMetadata is the metadata of a finding whose match is hashed.
// It returns nil when nothing is left.
func hashedFindingMetadata(h *FindingHasher, meta map[string]string) map[string]string {
	var out map[string]string
	for k, v := range meta {
		switch {
		case findingMetadataKept[k]:
		case findingMetadataHashed[k] && h != nil && v != "":
			v = h.Hash(v)
		default:
			continue
		}
		if out == nil {
			out = make(map[string]string, len(meta))
		}
		out[k] = v
	}
	return out
}

// errNotStoredDigest is returned for a digest selector that is not 64 hex
// characters. The API refuses such a selector before it gets here.
var errNotStoredDigest = errors.New("not a stored digest: want 64 hex characters, 0-9 and a-f")

// StoredDigestForms returns the stored matches a digest given as its 64
// lower-case hex characters may be: under the keyed scheme and under the
// unkeyed one of earlier versions. It returns nil for anything else, so a
// selector that is not a digest selects nothing.
func StoredDigestForms(hexDigest string) []string {
	if len(hexDigest) != 64 {
		return nil
	}
	for _, c := range hexDigest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil
		}
	}
	return []string{KeyedMatchPrefix + hexDigest, LegacyMatchPrefix + hexDigest}
}

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
