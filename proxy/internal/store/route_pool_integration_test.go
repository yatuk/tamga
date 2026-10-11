package store

import (
	"context"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

// The pool a ROUTE verdict named is stored with the request, by both writers,
// read back, and counted in the timeseries. On a table from before migration
// 020 the row is written without it, and the proxy adds the column itself.
func TestTC_RoutePool(t *testing.T) {
	s := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour).Add(-20 * time.Minute)
	since := now.Add(-24 * time.Hour)

	if !recColumns(t, s)["route_pool"] {
		t.Fatal("column route_pool is missing after the migrations")
	}
	execAll(t, s, migrationSQL(t, "020_route_pool.up.sql")) // can be run again

	routed := events.Event{
		RequestID: "route-bus", OrgID: testOrgID, Provider: "openai", Model: "internal-model-1",
		EventType: "request_scanned", Action: "ROUTE", RoutePool: "internal-llm", Timestamp: now,
	}
	busRow, _ := RequestLogFromEvent(routed, testOrgID, nil)
	viaNATS := routed
	viaNATS.RequestID = "route-nats"
	natsRow, ok := RequestLogFromV2(overNATS(t, viaNATS), testOrgID, nil)
	if !ok {
		t.Fatal("the NATS consumer did not take the event")
	}
	failed := routed
	failed.RequestID, failed.Action, failed.EventType = "route-failed", "BLOCK", "request_blocked"
	failedRow, _ := RequestLogFromEvent(failed, testOrgID, nil)
	plain := events.Event{RequestID: "route-none", OrgID: testOrgID, Provider: "openai", EventType: "request_scanned", Action: "PASS", Timestamp: now}
	plainRow, _ := RequestLogFromEvent(plain, testOrgID, nil)
	if err := s.insertBatch(ctx, []RequestLog{busRow, natsRow, failedRow, plainRow}); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string][2]string{
		"route-bus":    {"route", "internal-llm"},
		"route-nats":   {"route", "internal-llm"},
		"route-failed": {"block", "internal-llm"},
		"route-none":   {"pass", ""},
	} {
		rec, err := s.GetRequestRecord(ctx, testOrgID, id, since)
		if err != nil || rec == nil {
			t.Fatalf("%s: %v, %v", id, rec, err)
		}
		if rec.ActionTaken != want[0] || rec.RoutePool != want[1] {
			t.Errorf("%s: action %q, pool %q; want %q, %q", id, rec.ActionTaken, rec.RoutePool, want[0], want[1])
		}
	}
	if n := countRows(t, s, "request_id = 'route-none' AND route_pool IS NULL"); n != 1 {
		t.Error("a request that was not routed holds a pool")
	}

	buckets, err := s.RequestTimeseries(ctx, testOrgID, since, now.Add(time.Hour), "day")
	if err != nil {
		t.Fatal(err)
	}
	var total, routedCount, blocked int64
	for _, b := range buckets {
		total, routedCount, blocked = total+b.Total, routedCount+b.Routed, blocked+b.Blocked
	}
	if total != 4 || routedCount != 2 || blocked != 1 {
		t.Errorf("timeseries: %d requests, %d routed, %d blocked; want 4, 2, 1", total, routedCount, blocked)
	}

	// The table as it was before migration 020.
	execAll(t, s, migrationSQL(t, "020_route_pool.down.sql"))
	if recColumns(t, s)["route_pool"] {
		t.Fatal("the down migration left its column")
	}
	older := routed
	older.RequestID = "route-older-table"
	row, _ := RequestLogFromEvent(older, testOrgID, nil)
	if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
		t.Fatalf("write on the older table: %v", err)
	}
	s.schema.Store(nil)
	rec, err := s.GetRequestRecord(ctx, testOrgID, "route-older-table", since)
	if err != nil || rec == nil || rec.ActionTaken != "route" || rec.RoutePool != "" {
		t.Fatalf("read on the older table: %+v, %v", rec, err)
	}
	if err := s.EnsureRequestLogColumns(ctx); err != nil {
		t.Fatal(err)
	}
	if !recColumns(t, s)["route_pool"] {
		t.Fatal("EnsureRequestLogColumns did not add route_pool")
	}
	after := routed
	after.RequestID = "route-after-ensure"
	row, _ = RequestLogFromEvent(after, testOrgID, nil)
	if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
		t.Fatal(err)
	}
	if rec, err := s.GetRequestRecord(ctx, testOrgID, "route-after-ensure", since); err != nil || rec == nil || rec.RoutePool != "internal-llm" {
		t.Fatalf("after EnsureRequestLogColumns: %+v, %v", rec, err)
	}
}
