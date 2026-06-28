package scanner

import (
	"context"
	"testing"
)

func makeSpecs(specs []CompetitorSpec) func() []CompetitorSpec {
	return func() []CompetitorSpec { return specs }
}

func TestCompetitorScanner_Name(t *testing.T) {
	s := NewCompetitorScanner(nil)
	if s.Name() != "competitor" {
		t.Fatalf("expected name 'competitor', got %q", s.Name())
	}
}

func TestCompetitorScanner_ExactMatch(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "Lakera Guard",
			Patterns: []string{`Lakera\s*(Guard|AI)?`},
			Severity: "medium",
			Action:   "warn",
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("Should we switch from Lakera Guard to something else?"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Type != "competitor" {
		t.Errorf("expected type 'competitor', got %q", f.Type)
	}
	if f.Category != "Lakera Guard" {
		t.Errorf("expected category 'Lakera Guard', got %q", f.Category)
	}
	if f.Severity != "medium" {
		t.Errorf("expected severity 'medium', got %q", f.Severity)
	}
	if f.ActionTaken != "warn" {
		t.Errorf("expected action 'warn', got %q", f.ActionTaken)
	}
	if f.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", f.Confidence)
	}
}

func TestCompetitorScanner_CaseInsensitive(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "PromptArmor",
			Patterns: []string{`PromptArmor`},
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	cases := []string{
		"promptarmor is better",
		"PROMPTARMOR",
		"PromptArmor pricing",
		"promptArmor",
	}
	for _, input := range cases {
		findings, err := s.Scan(context.Background(), []byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 {
			t.Errorf("input %q: expected 1 finding, got %d", input, len(findings))
		}
	}
}

func TestCompetitorScanner_MultipleCompetitors(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "Lakera",
			Patterns: []string{`Lakera`},
			Enabled:  true,
		},
		{
			Name:     "NeMo",
			Patterns: []string{`NeMo\s*Guardrails?`},
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("Comparing Lakera vs NeMo Guardrails for our use case"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
	names := map[string]bool{}
	for _, f := range findings {
		names[f.Category] = true
	}
	if !names["Lakera"] || !names["NeMo"] {
		t.Errorf("expected both Lakera and NeMo, got %v", names)
	}
}

func TestCompetitorScanner_DisabledCompetitor(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "Lakera",
			Patterns: []string{`Lakera`},
			Enabled:  false,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("what about Lakera?"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for disabled competitor, got %d", len(findings))
	}
}

func TestCompetitorScanner_EmptyPatterns(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "EmptyCo",
			Patterns: []string{},
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("EmptyCo is here"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty patterns, got %d", len(findings))
	}
}

func TestCompetitorScanner_NilGetter(t *testing.T) {
	s := NewCompetitorScanner(nil)
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("Lakera Guard"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for nil getter, got %d", len(findings))
	}
}

func TestCompetitorScanner_EmptySpecs(t *testing.T) {
	s := NewCompetitorScanner(makeSpecs(nil))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("Lakera Guard"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty specs, got %d", len(findings))
	}
}

func TestCompetitorScanner_NoMatch(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "Lakera",
			Patterns: []string{`Lakera`},
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("Hello, world! No competitor names here."))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestCompetitorScanner_DefaultSeverityAndAction(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "DefaultCo",
			Patterns: []string{`DefaultCo`},
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("DefaultCo is here"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != "low" {
		t.Errorf("expected default severity 'low', got %q", findings[0].Severity)
	}
	if findings[0].ActionTaken != "log" {
		t.Errorf("expected default action 'log', got %q", findings[0].ActionTaken)
	}
}

func TestCompetitorScanner_HotReload(t *testing.T) {
	specs1 := []CompetitorSpec{
		{Name: "V1", Patterns: []string{`VersionOne`}, Enabled: true},
	}
	getter := func() []CompetitorSpec { return specs1 }
	s := NewCompetitorScanner(getter)
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("VersionOne"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("V1: expected 1 finding, got %d", len(findings))
	}

	// Hot-reload: swap the underlying slice
	specs1[0] = CompetitorSpec{Name: "V2", Patterns: []string{`VersionTwo`}, Enabled: true}
	s.Refresh()

	findings, err = s.Scan(context.Background(), []byte("VersionTwo is the new thing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("V2: expected 1 finding, got %d", len(findings))
	}
	if findings[0].Category != "V2" {
		t.Errorf("expected category 'V2', got %q", findings[0].Category)
	}

	// V1 should no longer match
	findings, err = s.Scan(context.Background(), []byte("VersionOne is gone"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("after reload: expected 0 findings for V1, got %d", len(findings))
	}
}

func TestCompetitorScanner_MultipleMatchesSameCompetitor(t *testing.T) {
	specs := []CompetitorSpec{
		{
			Name:     "Lakera",
			Patterns: []string{`Lakera`},
			Enabled:  true,
		},
	}
	s := NewCompetitorScanner(makeSpecs(specs))
	s.Refresh()

	findings, err := s.Scan(context.Background(), []byte("Lakera vs Lakera again. Lakera once more."))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(findings))
	}
}
