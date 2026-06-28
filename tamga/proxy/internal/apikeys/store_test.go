package apikeys

import (
	"strings"
	"testing"
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
