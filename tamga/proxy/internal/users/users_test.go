package users

import (
	"testing"
	"time"
)

func TestIsValidRole(t *testing.T) {
	for _, r := range []string{"admin", "analyst", "viewer"} {
		if !IsValidRole(r) {
			t.Errorf("%q should be valid", r)
		}
	}
	// Case insensitive
	for _, r := range []string{"Admin", "ANALYST", "Viewer", "ADMIN"} {
		if !IsValidRole(r) {
			t.Errorf("%q should be valid (case-insensitive)", r)
		}
	}
}

func TestIsValidRole_Invalid(t *testing.T) {
	for _, r := range []string{"superadmin", "owner", "user", "guest", "", " "} {
		if IsValidRole(r) {
			t.Errorf("%q should be invalid", r)
		}
	}
}

func TestMemoryStore_SetAndRole(t *testing.T) {
	s := NewMemoryStore()

	// Set a role
	m, err := s.Set("user-1", "admin")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if m.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", m.UserID)
	}
	if m.Role != "admin" {
		t.Errorf("expected admin, got %q", m.Role)
	}
	if m.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}

	// Role lookup
	role, ok := s.Role("user-1")
	if !ok {
		t.Fatal("expected user-1 to exist")
	}
	if role != "admin" {
		t.Errorf("expected admin, got %q", role)
	}

	// Non-existent
	_, ok = s.Role("nonexistent")
	if ok {
		t.Error("expected nonexistent to return false")
	}
}

func TestMemoryStore_Set_InvalidRole(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Set("user-1", "superadmin")
	if err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestMemoryStore_Set_EmptyUserID(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Set("", "admin")
	if err == nil {
		t.Error("expected error for empty user_id")
	}
	_, err = s.Set("  ", "admin")
	if err == nil {
		t.Error("expected error for whitespace user_id")
	}
}

func TestMemoryStore_Set_CaseNormalization(t *testing.T) {
	s := NewMemoryStore()
	m, err := s.Set("user-1", "ADMIN")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if m.Role != "admin" {
		t.Errorf("expected role 'admin' (lowercase), got %q", m.Role)
	}
}

func TestMemoryStore_UpdateExisting(t *testing.T) {
	s := NewMemoryStore()
	_, _ = s.Set("user-1", "viewer")

	// Update role — capture time before the update to compare.
	beforeUpdate := time.Now().UTC()
	m, err := s.Set("user-1", "analyst")
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if m.Role != "analyst" {
		t.Errorf("expected analyst, got %q", m.Role)
	}
	// UpdatedAt should be at or after the beforeUpdate time.
	if m.UpdatedAt.Before(beforeUpdate) {
		t.Error("UpdatedAt should be at or after beforeUpdate")
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	_, _ = s.Set("user-1", "viewer")
	_, _ = s.Set("user-2", "admin")

	s.Delete("user-1")

	_, ok := s.Role("user-1")
	if ok {
		t.Error("user-1 should be deleted")
	}
	_, ok = s.Role("user-2")
	if !ok {
		t.Error("user-2 should still exist")
	}
}

func TestMemoryStore_DeleteNonExistent(t *testing.T) {
	s := NewMemoryStore()
	// Should not panic
	s.Delete("nonexistent")
}

func TestMemoryStore_List(t *testing.T) {
	s := NewMemoryStore()
	_, _ = s.Set("user-1", "viewer")
	_, _ = s.Set("user-2", "admin")
	_, _ = s.Set("user-3", "analyst")

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 members, got %d", len(list))
	}

	// Verify list is sorted by UpdatedAt descending
	for i := 1; i < len(list); i++ {
		if list[i-1].UpdatedAt.Before(list[i].UpdatedAt) {
			t.Error("list should be sorted by UpdatedAt descending")
		}
	}
}

func TestMemoryStore_ListEmpty(t *testing.T) {
	s := NewMemoryStore()
	list := s.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestClerkUser_Name(t *testing.T) {
	tests := []struct {
		name      string
		firstName string
		lastName  string
		want      string
	}{
		{"full name", "Alice", "Smith", "Alice Smith"},
		{"first only", "Bob", "", "Bob"},
		{"empty", "", "", ""},
		{"last only", "", "Jones", "Jones"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &ClerkUser{FirstName: tt.firstName, LastName: tt.lastName}
			if got := u.Name(); got != tt.want {
				t.Errorf("Name(): want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestClerkUser_Email(t *testing.T) {
	t.Run("primary email", func(t *testing.T) {
		u := &ClerkUser{PrimaryEmail: "a@b.com"}
		if got := u.Email(); got != "a@b.com" {
			t.Errorf("expected 'a@b.com', got %q", got)
		}
	})
	t.Run("fallback to first email", func(t *testing.T) {
		u := &ClerkUser{
			EmailAddresses: []struct {
				EmailAddress string `json:"email_address"`
			}{
				{EmailAddress: "first@test.com"},
				{EmailAddress: "second@test.com"},
			},
		}
		if got := u.Email(); got != "first@test.com" {
			t.Errorf("expected 'first@test.com', got %q", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		u := &ClerkUser{}
		if got := u.Email(); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
	t.Run("primary takes precedence", func(t *testing.T) {
		u := &ClerkUser{
			PrimaryEmail: "primary@test.com",
			EmailAddresses: []struct {
				EmailAddress string `json:"email_address"`
			}{
				{EmailAddress: "fallback@test.com"},
			},
		}
		if got := u.Email(); got != "primary@test.com" {
			t.Errorf("expected 'primary@test.com', got %q", got)
		}
	})
}

func TestNewClerkClient(t *testing.T) {
	c := NewClerkClient("")
	if c != nil {
		t.Error("expected nil for empty secret")
	}

	c = NewClerkClient("sk_test_123")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.Secret != "sk_test_123" {
		t.Errorf("expected secret, got %q", c.Secret)
	}
	if c.HTTP == nil {
		t.Error("expected non-nil HTTP client")
	}
}
