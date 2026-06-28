package scanner

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestCustomScanner_FindsMusteriNo(t *testing.T) {
	s := NewCustomScanner(func() []CustomEntitySpec {
		return []CustomEntitySpec{
			{Name: "musteri_no", Pattern: `MN-\d{8}`, Severity: "high", Confidence: 0.9},
		}
	})
	fs, err := s.Scan(context.Background(), []byte(`Müşteri MN-12345678 kaydı`))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("findings: got %d want 1", len(fs))
	}
	if fs[0].Type != "custom" || fs[0].Category != "musteri_no" {
		t.Fatalf("finding: %+v", fs[0])
	}
}

func TestCustomScanner_FindsSicilNo(t *testing.T) {
	s := NewCustomScanner(func() []CustomEntitySpec {
		return []CustomEntitySpec{
			{Name: "sicil_no", Pattern: `SN\d{6}`, Severity: "high", Confidence: 0.9},
		}
	})
	fs, err := s.Scan(context.Background(), []byte(`Sicil: SN123456`))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("findings: got %d want 1", len(fs))
	}
	if fs[0].Category != "sicil_no" {
		t.Fatalf("category: %q", fs[0].Category)
	}
}

func TestCustomScanner_InvalidRegexSkipped(t *testing.T) {
	log.Logger = zerolog.Nop()
	s := NewCustomScanner(func() []CustomEntitySpec {
		return []CustomEntitySpec{{Name: "bad_ent", Pattern: "("}}
	})
	fs, err := s.Scan(context.Background(), []byte("anything"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Fatalf("expected no findings, got %d", len(fs))
	}
}

func TestCustomScanner_EmptySpecs(t *testing.T) {
	s := NewCustomScanner(func() []CustomEntitySpec { return nil })
	fs, err := s.Scan(context.Background(), []byte(`MN-12345678`))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Fatalf("expected no findings, got %d", len(fs))
	}
}

func TestCustomScanner_Name(t *testing.T) {
	s := NewCustomScanner(nil)
	if s.Name() != "custom" {
		t.Fatalf("name: %q", s.Name())
	}
}

func TestCustomScanner_ConcurrentScans(t *testing.T) {
	s := NewCustomScanner(func() []CustomEntitySpec {
		return []CustomEntitySpec{
			{Name: "musteri_no", Pattern: `MN-\d{8}`, Severity: "high", Confidence: 0.9},
			{Name: "sicil_no", Pattern: `SN\d{6}`, Severity: "high", Confidence: 0.9},
		}
	})
	// Trigger initial compilation.
	_, _ = s.Scan(context.Background(), []byte("MN-12345678"))

	ctx := context.Background()
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			findings, err := s.Scan(ctx, []byte("Müşteri MN-12345678 kaydı ve SN123456 sicil"))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(findings) < 1 {
				t.Errorf("expected at least 1 finding, got %d", len(findings))
			}
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestCustomScanner_Refresh(t *testing.T) {
	calls := 0
	s := NewCustomScanner(func() []CustomEntitySpec {
		calls++
		return []CustomEntitySpec{
			{Name: "test", Pattern: `TEST-\d+`, Severity: "high", Confidence: 0.9},
		}
	})
	// Initial scan compiles.
	fs, err := s.Scan(context.Background(), []byte("TEST-123"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fs))
	}
	compileCount := calls

	// Refresh recompiles even with same specs (fingerprint matches but still safe).
	s.Refresh()

	// Scan still works after refresh.
	fs, err = s.Scan(context.Background(), []byte("TEST-456"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("expected 1 finding after refresh, got %d", len(fs))
	}
	_ = compileCount
}
