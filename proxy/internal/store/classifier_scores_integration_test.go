package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
)

// The shadow comparison is stored with the request, by both writers, and
// read back. On a table from before migration 021 the row is written
// without it, and the proxy adds the column itself.
func TestTC_ClassifierScores(t *testing.T) {
	s := NewTestPostgresStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour).Add(-20 * time.Minute)
	since := now.Add(-24 * time.Hour)

	if !recColumns(t, s)["classifier_scores"] {
		t.Fatal("column classifier_scores is missing after the migrations")
	}
	execAll(t, s, migrationSQL(t, "021_classifier_scores.up.sql")) // can be run again

	scored := events.Event{
		RequestID: "shadow-bus", OrgID: testOrgID, Provider: "openai", Model: "m",
		EventType: "request_scanned", Action: "PASS", Timestamp: now, Classifier: shadowRecord(),
	}
	busRow, _ := RequestLogFromEvent(scored, testOrgID, nil)
	viaNATS := scored
	viaNATS.RequestID = "shadow-nats"
	natsRow, ok := RequestLogFromV2(overNATS(t, viaNATS), testOrgID, nil)
	if !ok {
		t.Fatal("the NATS consumer did not take the event")
	}
	plain := events.Event{RequestID: "shadow-none", OrgID: testOrgID, Provider: "openai", EventType: "request_scanned", Action: "PASS", Timestamp: now}
	plainRow, _ := RequestLogFromEvent(plain, testOrgID, nil)
	if err := s.insertBatch(ctx, []RequestLog{busRow, natsRow, plainRow}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"shadow-bus", "shadow-nats"} {
		rec, err := s.GetRequestRecord(ctx, testOrgID, id, since)
		if err != nil || rec == nil {
			t.Fatalf("%s: %v, %v", id, rec, err)
		}
		var got events.ClassifierRecord
		if err := json.Unmarshal(rec.ClassifierScores, &got); err != nil || got.ShadowModel != "tamga-tr-candidate" || len(got.Texts) != 2 || got.Texts[0].Shadow == nil {
			t.Errorf("%s: stored %s (%v)", id, rec.ClassifierScores, err)
		}
	}
	if rec, err := s.GetRequestRecord(ctx, testOrgID, "shadow-none", since); err != nil || rec == nil || rec.ClassifierScores != nil {
		t.Fatalf("a request without a shadow: %+v, %v", rec, err)
	}

	// The table as it was before migration 021.
	execAll(t, s, migrationSQL(t, "021_classifier_scores.down.sql"))
	if recColumns(t, s)["classifier_scores"] {
		t.Fatal("the down migration left its column")
	}
	older := scored
	older.RequestID = "shadow-older-table"
	row, _ := RequestLogFromEvent(older, testOrgID, nil)
	if err := s.insertBatch(ctx, []RequestLog{row}); err != nil {
		t.Fatalf("write on the older table: %v", err)
	}
	s.schema.Store(nil)
	if rec, err := s.GetRequestRecord(ctx, testOrgID, "shadow-older-table", since); err != nil || rec == nil || rec.ClassifierScores != nil {
		t.Fatalf("read on the older table: %+v, %v", rec, err)
	}
	if err := s.EnsureRequestLogColumns(ctx); err != nil {
		t.Fatal(err)
	}
	if !recColumns(t, s)["classifier_scores"] {
		t.Fatal("EnsureRequestLogColumns did not add classifier_scores")
	}
}
