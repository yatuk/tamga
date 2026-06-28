package store

import "testing"

func TestParsePartitionUpperBoundTO(t *testing.T) {
	t.Parallel()
	bound := `FOR VALUES FROM ('2026-04-01') TO ('2026-05-01')`
	ts, ok := parsePartitionUpperBoundTO(bound)
	if !ok {
		t.Fatal("expected ok")
	}
	if ts.Year() != 2026 || ts.Month() != 5 || ts.Day() != 1 {
		t.Fatalf("got %v", ts)
	}
}
