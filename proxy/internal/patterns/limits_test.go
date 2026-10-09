package patterns

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
)

func TestStoreRefusesCostlyPatterns(t *testing.T) {
	s := NewMemoryStore()
	kept, err := s.Create(Pattern{Name: "kept", Kind: "regex", Pattern: `MN-\d{8}`, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, pattern, limit string
	}{
		{"twenty thousand alternations", strings.Repeat("(?:a|b)", 20000), "the limit is 1024"},
		{"bounded dot before a word", `(?s).{0,1000}secret`, "the limit is 500"},
		{"a thousand letters", `\p{L}{1000}`, "the limit is 500"},
		{"star", `x*`, "matches the empty string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Pattern{Name: "costly", Kind: "regex", Pattern: tc.pattern, Enabled: true}
			for op, err := range map[string]error{
				"create": func() error { _, err := s.Create(p); return err }(),
				"update": func() error { _, err := s.Update(kept.ID, p); return err }(),
			} {
				if err == nil {
					t.Fatalf("%s accepted it", op)
				}
				if !strings.Contains(err.Error(), tc.limit) || len(err.Error()) > 300 {
					t.Errorf("%s: %q does not name the limit %q, or carries the pattern", op, err, tc.limit)
				}
			}
		})
	}
	if got := s.List(); len(got) != 1 || got[0].Pattern != `MN-\d{8}` {
		t.Errorf("the store changed: %+v", got)
	}
}

func TestStoreLiteralLengthLimit(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Create(Pattern{Name: "at", Kind: "literal", Pattern: strings.Repeat("a", scanner.MaxPatternBytes)}); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	if _, err := s.Create(Pattern{Name: "over", Kind: "literal", Pattern: strings.Repeat("a", scanner.MaxPatternBytes+1)}); err == nil {
		t.Error("a literal over the length limit was accepted")
	}
}

func TestStoreCountLimit(t *testing.T) {
	s := NewMemoryStore()
	var last Pattern
	for i := 0; i < scanner.MaxStoredPatterns; i++ {
		p, err := s.Create(Pattern{Name: fmt.Sprintf("p%d", i), Kind: "regex", Pattern: fmt.Sprintf(`ID%d-\d{4}`, i)})
		if err != nil {
			t.Fatalf("pattern %d of %d: %v", i+1, scanner.MaxStoredPatterns, err)
		}
		last = p
	}
	if _, err := s.Create(Pattern{Name: "one more", Kind: "regex", Pattern: `ONE-\d{4}`}); !errors.Is(err, ErrTooMany) {
		t.Errorf("one over the limit: err = %v, want ErrTooMany", err)
	}
	// Room is made by deleting, and a full store can still be edited.
	if _, err := s.Update(last.ID, Pattern{Name: "edited", Kind: "regex", Pattern: `EDIT-\d{4}`}); err != nil {
		t.Errorf("update at the limit: %v", err)
	}
	if err := s.Delete(last.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(Pattern{Name: "one more", Kind: "regex", Pattern: `ONE-\d{4}`}); err != nil {
		t.Errorf("create after a delete: %v", err)
	}
}
