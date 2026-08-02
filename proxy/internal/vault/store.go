package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// RedisClient is the subset of redisx.Client the vault store needs. The proxy's
// redisx.Client satisfies it structurally (in-memory fallback when REDIS_URL is
// unset, so the store degrades gracefully to single-node).
type RedisClient interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// Store persists per-request placeholder->original mappings, encrypted at rest.
// It is the durable/audit/multi-instance copy; the handler also keeps the
// mapping in a request-scoped variable for the common same-process round-trip.
// A Store with a nil client or nil cipher is a no-op (Enabled reports false).
type Store struct {
	client    RedisClient
	cipher    *Cipher
	keyPrefix string
	ttl       time.Duration
}

// NewStore builds a vault store. Pass a nil client or nil cipher to disable it.
func NewStore(client RedisClient, cipher *Cipher, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Store{client: client, cipher: cipher, keyPrefix: "tamga:vault", ttl: ttl}
}

// Enabled reports whether the store can persist (client + cipher present).
func (s *Store) Enabled() bool {
	return s != nil && s.client != nil && s.cipher != nil
}

func (s *Store) key(requestID string) string {
	return fmt.Sprintf("%s:%s", s.keyPrefix, requestID)
}

// Save encrypts every original value and stores the placeholder->ciphertext map
// under the request id with the configured TTL.
func (s *Store) Save(ctx context.Context, requestID string, mapping map[string]string) error {
	if !s.Enabled() || len(mapping) == 0 {
		return nil
	}
	enc := make(map[string]string, len(mapping))
	for token, original := range mapping {
		blob, err := s.cipher.Encrypt(original)
		if err != nil {
			return err
		}
		enc[token] = blob
	}
	data, err := json.Marshal(enc)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.key(requestID), data, s.ttl)
}

// Load decrypts and returns the placeholder->original map for a request, or an
// empty map when absent.
func (s *Store) Load(ctx context.Context, requestID string) (map[string]string, error) {
	if !s.Enabled() {
		return nil, nil
	}
	data, found, err := s.client.Get(ctx, s.key(requestID))
	if err != nil || !found {
		return nil, err
	}
	var enc map[string]string
	if err := json.Unmarshal(data, &enc); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(enc))
	for token, blob := range enc {
		original, derr := s.cipher.Decrypt(blob)
		if derr != nil {
			log.Warn().Err(derr).Str("request_id", requestID).Msg("vault: decrypt failed for token")
			continue
		}
		out[token] = original
	}
	return out, nil
}

// Delete removes a request's vault entry (single-use restore).
func (s *Store) Delete(ctx context.Context, requestID string) {
	if !s.Enabled() {
		return
	}
	_ = s.client.Del(ctx, s.key(requestID))
}
