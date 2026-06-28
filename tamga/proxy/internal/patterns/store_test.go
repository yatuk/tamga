package patterns

import (
	"testing"
)

func TestCreateValidRegex(t *testing.T) {
	s := NewMemoryStore()
	p, err := s.Create(Pattern{Name: "proj codename", Kind: "regex", Pattern: `(?i)project-\w+`, Severity: "high", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected id")
	}
	if got, ok := s.Get(p.ID); !ok || got.Name != "proj codename" {
		t.Fatalf("get: %+v ok=%v", got, ok)
	}
}

func TestCreateInvalidRegex(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Create(Pattern{Name: "x", Kind: "regex", Pattern: "("}); err == nil {
		t.Fatal("expected regex compile error")
	}
}

func TestUpdateDelete(t *testing.T) {
	s := NewMemoryStore()
	p, err := s.Create(Pattern{Name: "a", Kind: "literal", Pattern: "CASH", Severity: "low", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Update(p.ID, Pattern{Name: "b", Kind: "literal", Pattern: "CASH2", Severity: "medium", Enabled: false}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, ok := s.Get(p.ID)
	if !ok || got.Name != "b" || got.Pattern != "CASH2" {
		t.Fatalf("update mismatch: %+v", got)
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := s.Get(p.ID); ok {
		t.Fatal("expected deleted")
	}
}
