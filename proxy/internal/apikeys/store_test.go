package apikeys

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMemoryStore_Create(t *testing.T) {
	s := NewMemoryStore()
	created, err := s.Create("test-key", ScopeRead)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(created.RawKey, "tk_") {
		t.Fatalf("raw key should have tk_ prefix: %q", created.RawKey)
	}
	if created.Label != "test-key" {
		t.Fatalf("label: got %q want test-key", created.Label)
	}
	if created.Scope != ScopeRead {
		t.Fatalf("scope: got %q want read", created.Scope)
	}
	if created.Prefix == "" {
		t.Fatal("prefix should not be empty")
	}
	if len(created.Prefix) != 8 {
		t.Fatalf("prefix length: got %d want 8", len(created.Prefix))
	}
	if created.ID == "" {
		t.Fatal("id should not be empty")
	}
}

func TestMemoryStore_ListSorted(t *testing.T) {
	s := NewMemoryStore()
	c1, _ := s.Create("key-a", ScopeRead)
	c2, _ := s.Create("key-b", ScopeWrite)
	c3, _ := s.Create("key-c", ScopeAdmin)

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("list length: got %d want 3", len(list))
	}
	// All created IDs should be present.
	ids := make(map[string]bool)
	for _, k := range list {
		ids[k.ID] = true
	}
	if !ids[c1.ID] || !ids[c2.ID] || !ids[c3.ID] {
		t.Fatalf("not all IDs present: %v", ids)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	created, _ := s.Create("delete-me", ScopeRead)

	err := s.Delete(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second delete should return ErrNotFound.
	err = s.Delete(created.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_DeleteNonExistent(t *testing.T) {
	s := NewMemoryStore()
	err := s.Delete("nonexistent-id")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Verify(t *testing.T) {
	s := NewMemoryStore()
	created, _ := s.Create("verify-key", ScopeWrite)

	key, ok := s.Verify(created.RawKey)
	if !ok {
		t.Fatal("expected verification to succeed")
	}
	if key.ID != created.ID {
		t.Fatalf("id mismatch: got %s want %s", key.ID, created.ID)
	}
	if key.LastUsed.IsZero() {
		t.Fatal("LastUsed should be set after Verify")
	}
}

func TestMemoryStore_VerifyEmptyString(t *testing.T) {
	s := NewMemoryStore()
	_, ok := s.Verify("")
	if ok {
		t.Fatal("expected empty string verification to fail")
	}
}

func TestMemoryStore_VerifyUnknown(t *testing.T) {
	s := NewMemoryStore()
	_, _ = s.Create("some-key", ScopeRead)

	_, ok := s.Verify("tk_ffffffffffffffffffffffffffffffffffffffffffffffffff")
	if ok {
		t.Fatal("expected unknown key verification to fail")
	}
}

func TestMemoryStore_CreateInvalidScope(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Create("bad-scope", "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestIsValidScope(t *testing.T) {
	if !IsValidScope(ScopeRead) {
		t.Fatal("read should be valid")
	}
	if !IsValidScope(ScopeWrite) {
		t.Fatal("write should be valid")
	}
	if !IsValidScope(ScopeAdmin) {
		t.Fatal("admin should be valid")
	}
	if IsValidScope("") {
		t.Fatal("empty string should be invalid")
	}
	if IsValidScope("superadmin") {
		t.Fatal("superadmin should be invalid")
	}
}

func TestMemoryStore_IdentityAndExpiry(t *testing.T) {
	s := NewMemoryStore()
	ck, err := s.CreateWith(Params{Label: "svc", Scope: ScopeProxy, OrgID: "acme", Role: "analyst", UserID: "svc-1", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	k, ok := s.Verify(ck.RawKey)
	if !ok || k.OrgID != "acme" || k.Role != "analyst" || k.UserID != "svc-1" || k.Scope != ScopeProxy {
		t.Fatalf("verify: ok=%v key=%+v", ok, k)
	}
	if _, err := s.CreateWith(Params{Scope: ScopeProxy, ExpiresAt: time.Now().Add(-time.Second)}); err == nil {
		t.Fatal("created a key that is already expired")
	}
	if !(Key{ExpiresAt: time.Now().Add(-time.Second)}).Expired(time.Now()) {
		t.Fatal("a past expiry must count as expired")
	}
	if (Key{}).Expired(time.Now()) {
		t.Fatal("a key with no expiry does not expire")
	}
	if err := s.Delete(ck.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Verify(ck.RawKey); ok {
		t.Fatal("a revoked key verified")
	}
}

// A key that was never used and never expires must not serialise the zero
// time: the dashboard would show it as a date two thousand years ago.
func TestKey_JSONOmitsUnsetTimes(t *testing.T) {
	b, err := json.Marshal(Key{ID: "a", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "last_used") || strings.Contains(string(b), "expires_at") {
		t.Fatalf("unset times were serialised: %s", b)
	}
}

// The id names the key in audit entries, log lines and the console. It must
// hold none of the key.
func TestNewKey_IDIsIndependentOfKey(t *testing.T) {
	s := NewMemoryStore()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		ck, err := s.Create("k", ScopeRead)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(ck.ID, idPrefix) {
			t.Fatalf("id %q does not start with %q", ck.ID, idPrefix)
		}
		random := strings.TrimPrefix(ck.ID, idPrefix)
		if len(random) != 16 {
			t.Fatalf("id %q: random part is %d characters, want 16", ck.ID, len(random))
		}
		secret := strings.TrimPrefix(ck.RawKey, "tk_")
		if len(secret) != 48 {
			t.Fatalf("key body is %d characters, want 48", len(secret))
		}
		// Any 8 characters in common would be 32 bits of the key.
		for j := 0; j+8 <= len(random); j++ {
			if strings.Contains(secret, random[j:j+8]) {
				t.Fatalf("id %q shares %q with the key", ck.ID, random[j:j+8])
			}
		}
		if seen[ck.ID] {
			t.Fatalf("id %q drawn twice", ck.ID)
		}
		seen[ck.ID] = true
		if ck.Prefix != ck.RawKey[:prefixLen] || len(ck.Prefix) != 8 {
			t.Fatalf("prefix %q: want the first %d characters of the key", ck.Prefix, prefixLen)
		}
	}
}

// A key made before ids were random has the start of the key as its id. It
// must go on verifying under that id, and so must a key whose id is anything
// else: the lookup is by hash.
func TestMemoryStore_VerifyDoesNotDependOnID(t *testing.T) {
	const raw = "tk_00112233445566778899aabbccddeeff0011223344556677"
	for _, id := range []string{"001122334455", "key_ffffffffffffffff", "anything"} {
		s := NewMemoryStore()
		s.data[id] = &memKey{Key: Key{ID: id, Scope: ScopeRead, Prefix: raw[:prefixLen]}, hash: hashKey(raw)}
		s.byHash[hashKey(raw)] = id
		k, ok := s.Verify(raw)
		if !ok || k.ID != id {
			t.Fatalf("id %q: ok=%v key=%+v", id, ok, k)
		}
		// A key that merely starts like the id opens nothing.
		if _, ok := s.Verify("tk_001122334455ffffffffffffffffffffffffffffffffffff"); ok {
			t.Fatalf("id %q: a different key verified", id)
		}
		if err := s.Delete(id); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Verify(raw); ok || len(s.byHash) != 0 {
			t.Fatalf("id %q: revoked key still known (ok=%v, hashes=%d)", id, ok, len(s.byHash))
		}
	}
}

// The index only finds a candidate; the stored hash decides.
func TestMemoryStore_VerifyComparesTheStoredHash(t *testing.T) {
	s := NewMemoryStore()
	a, _ := s.Create("a", ScopeRead)
	b, _ := s.Create("b", ScopeAdmin)
	// An index entry that points a's hash at b's record must not open b.
	s.byHash[hashKey(a.RawKey)] = b.ID
	if k, ok := s.Verify(a.RawKey); ok {
		t.Fatalf("verified as %+v through a wrong index entry", k)
	}
	if k, ok := s.Verify(b.RawKey); !ok || k.ID != b.ID {
		t.Fatalf("b: ok=%v key=%+v", ok, k)
	}
}

func TestSameHash(t *testing.T) {
	h := hashKey("tk_a")
	if !sameHash(h, hashKey("tk_a")) || sameHash(h, hashKey("tk_b")) || sameHash(h, "") || sameHash(h, h[:10]) {
		t.Fatal("sameHash")
	}
}
