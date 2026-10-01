package scanner

import (
	"context"
	"errors"
	"testing"
)

type degStub struct {
	name string
	fn   func() ([]Finding, error)
}

func (s degStub) Name() string { return s.name }
func (s degStub) Scan(context.Context, []byte) ([]Finding, error) {
	return s.fn()
}

func okScanner(name string) degStub {
	return degStub{name: name, fn: func() ([]Finding, error) {
		return []Finding{{Type: "pii", Category: name}}, nil
	}}
}

func panicScanner(name string) degStub {
	return degStub{name: name, fn: func() ([]Finding, error) { panic("boom") }}
}

func errScanner(name string) degStub {
	return degStub{name: name, fn: func() ([]Finding, error) { return nil, errors.New("backend down") }}
}

// A scanner that panics or errors must not hide the findings of the scanners
// that did run, and the loss of coverage must be reported, not swallowed.
func TestPipeline_DegradedScanIsReported(t *testing.T) {
	modes := []PipelineMode{ModeAdaptive, ModeAsync}
	failing := map[string]func(string) degStub{
		DegradedPanic: panicScanner,
		DegradedError: errScanner,
	}

	for _, mode := range modes {
		for reason, mk := range failing {
			for _, speed := range []ScannerSpeed{SpeedFast, SpeedSlow} {
				before := ScanDegradedStats()[reason]

				p := NewPipelineWithConfig([]ScannerEntry{
					{Scanner: okScanner("healthy"), Speed: SpeedFast},
					{Scanner: mk("broken"), Speed: speed},
				}, PipelineConfig{Mode: mode})

				findings, err := p.Scan(context.Background(), []byte("x"))

				if !errors.Is(err, ErrScanDegraded) {
					t.Errorf("mode=%s reason=%s speed=%d: err = %v, want ErrScanDegraded", mode, reason, speed, err)
				}
				if len(findings) != 1 || findings[0].Category != "healthy" {
					t.Errorf("mode=%s reason=%s speed=%d: healthy scanner's finding was lost: %+v", mode, reason, speed, findings)
				}
				if got := ScanDegradedStats()[reason]; got != before+1 {
					t.Errorf("mode=%s reason=%s speed=%d: counter went %d -> %d, want +1", mode, reason, speed, before, got)
				}
			}
		}
	}
}

func TestPipeline_HealthyScanIsNotDegraded(t *testing.T) {
	p := NewPipelineWithConfig([]ScannerEntry{
		{Scanner: okScanner("a"), Speed: SpeedFast},
		{Scanner: okScanner("b"), Speed: SpeedSlow},
	}, PipelineConfig{Mode: ModeAdaptive})

	findings, err := p.Scan(context.Background(), []byte("x"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("want 2 findings, got %d", len(findings))
	}
}
