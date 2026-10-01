package store

import (
	"testing"
	"time"
)

// The bound text below is what PostgreSQL actually returns from pg_get_expr
// for a partition of a timestamptz-ranged table. The parser used to accept
// only the bare-date form, so every real partition was skipped and retention
// never dropped anything.
func TestParsePartitionUpperBoundTO_PostgresTimestamptzFormats(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		bound string
		want  time.Time
	}{
		{"utc server", `FOR VALUES FROM ('2020-01-01 00:00:00+00') TO ('2020-02-01 00:00:00+00')`,
			time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"istanbul server", `FOR VALUES FROM ('2020-01-01 03:00:00+03') TO ('2020-02-01 03:00:00+03')`,
			time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"half-hour offset", `FOR VALUES FROM ('2020-01-01 05:30:00+05:30') TO ('2020-02-01 05:30:00+05:30')`,
			time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"timestamp without zone", `FOR VALUES FROM ('2020-01-01 00:00:00') TO ('2020-02-01 00:00:00')`,
			time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got, ok := parsePartitionUpperBoundTO(tc.bound)
		if !ok {
			t.Errorf("%s: bound not parsed: %s", tc.name, tc.bound)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

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
