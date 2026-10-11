package patterns

import (
	"regexp"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
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

// A literal reaches the scanners as an expression that matches its text and
// nothing else, in any case: a pattern the console lets an analyst make and
// try has to be one the proxy scans for.
func TestExpression(t *testing.T) {
	lit := Pattern{Kind: KindLiteral, Pattern: "Project a.b (v2)+"}
	re, err := regexp.Compile(lit.Expression())
	if err != nil {
		t.Fatalf("literal does not compile: %v", err)
	}
	if got := re.FindString("see PROJECT A.B (V2)+ notes"); got != "PROJECT A.B (V2)+" {
		t.Fatalf("literal matched %q", got)
	}
	for _, other := range []string{"project aXb (v2)+", "project a.b v2", "project a.b (v2)"} {
		if re.MatchString(other) {
			t.Fatalf("literal matched %q: a metacharacter was not escaped", other)
		}
	}
	if err := scanner.CheckPattern(lit.Expression()); err != nil {
		t.Fatalf("literal is refused by the pattern limits: %v", err)
	}
	rx := Pattern{Kind: KindRegex, Pattern: `INV-\d{4}`}
	if rx.Expression() != rx.Pattern {
		t.Fatalf("regex was changed: %q", rx.Expression())
	}
}
