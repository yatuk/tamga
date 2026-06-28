package operator_state

import (
	"testing"
)

func TestProjection_DecisionLifecycle(t *testing.T) {
	p := NewProjection()

	// Propose → state should be proposed.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"})
	rec := p.GetDecision("D-2026-06-23-001")
	if rec == nil {
		t.Fatal("decision not found after propose")
	}
	if rec.State != StateProposed {
		t.Errorf("state = %q, want proposed", rec.State)
	}

	// Accept → state should be accepted.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:15:30+02:00", Action: DecisionAccept, Decision: "D-2026-06-23-001"})
	rec = p.GetDecision("D-2026-06-23-001")
	if rec.State != StateAccepted {
		t.Errorf("state = %q, want accepted", rec.State)
	}

	// Lock → state should be locked.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:18:22+02:00", Action: DecisionLock, Decision: "D-2026-06-23-001"})
	rec = p.GetDecision("D-2026-06-23-001")
	if rec.State != StateLocked {
		t.Errorf("state = %q, want locked", rec.State)
	}

	// Supersede → state should be superseded.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T11:33:42+02:00", Action: DecisionSupersede, Decision: "D-2026-06-23-001"})
	rec = p.GetDecision("D-2026-06-23-001")
	if rec.State != StateSuperseded {
		t.Errorf("state = %q, want superseded", rec.State)
	}

	// History should contain all 4 events.
	if len(rec.History) != 4 {
		t.Errorf("history length = %d, want 4", len(rec.History))
	}
}

func TestProjection_RejectAndReopen(t *testing.T) {
	p := NewProjection()

	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-002"})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T10:05:11+02:00", Action: DecisionReject, Decision: "D-2026-06-23-002"})

	rec := p.GetDecision("D-2026-06-23-002")
	if rec.State != StateRejected {
		t.Errorf("state = %q, want rejected", rec.State)
	}

	// Reopen → goes back to proposed.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T12:45:00+02:00", Action: DecisionReopen, Decision: "D-2026-06-23-002"})
	rec = p.GetDecision("D-2026-06-23-002")
	if rec.State != StateProposed {
		t.Errorf("state after reopen = %q, want proposed", rec.State)
	}

	// Accept after reopen.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T12:46:30+02:00", Action: DecisionAccept, Decision: "D-2026-06-23-002"})
	rec = p.GetDecision("D-2026-06-23-002")
	if rec.State != StateAccepted {
		t.Errorf("state after reopen+accept = %q, want accepted", rec.State)
	}

	if len(rec.History) != 4 {
		t.Errorf("history length = %d, want 4", len(rec.History))
	}
}

func TestProjection_IdempotentReplay(t *testing.T) {
	p := NewProjection()

	ev := DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"}

	// First apply should succeed.
	if !p.ApplyDecision(ev) {
		t.Error("first ApplyDecision should return true")
	}

	// Second apply of same event should be a no-op.
	if p.ApplyDecision(ev) {
		t.Error("second ApplyDecision should return false (duplicate)")
	}

	rec := p.GetDecision("D-2026-06-23-001")
	if len(rec.History) != 1 {
		t.Errorf("history length = %d, want 1 (duplicate should not append)", len(rec.History))
	}
}

func TestProjection_NoteLifecycle(t *testing.T) {
	p := NewProjection()

	p.ApplyNote(NoteEvent{TS: "2026-06-23T08:00:00+02:00", Action: NoteAdd, Note: "N-2026-06-23-001", Detail: "architecture"})
	rec := p.GetNote("N-2026-06-23-001")
	if rec == nil {
		t.Fatal("note not found after add")
	}
	if rec.State != StateNoteActive {
		t.Errorf("state = %q, want active", rec.State)
	}
	if rec.Detail != "architecture" {
		t.Errorf("detail = %q, want architecture", rec.Detail)
	}

	// Archive.
	p.ApplyNote(NoteEvent{TS: "2026-06-23T11:00:33+02:00", Action: NoteArchive, Note: "N-2026-06-23-001", Detail: "superseded"})
	rec = p.GetNote("N-2026-06-23-001")
	if rec.State != StateNoteArchived {
		t.Errorf("state after archive = %q, want archived", rec.State)
	}
	if rec.Detail != "superseded" {
		t.Errorf("detail after archive = %q, want superseded", rec.Detail)
	}
}

func TestProjection_Snapshot(t *testing.T) {
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-2026-06-23-001"})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:15:30+02:00", Action: DecisionAccept, Decision: "D-2026-06-23-001"})
	p.ApplyNote(NoteEvent{TS: "2026-06-23T08:00:00+02:00", Action: NoteAdd, Note: "N-2026-06-23-001"})

	snap := p.Snapshot()

	if len(snap.Decisions) != 1 {
		t.Errorf("snapshot decisions = %d, want 1", len(snap.Decisions))
	}
	if len(snap.Notes) != 1 {
		t.Errorf("snapshot notes = %d, want 1", len(snap.Notes))
	}

	// Snapshot should be independent of live state.
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:18:22+02:00", Action: DecisionLock, Decision: "D-2026-06-23-001"})

	if snap.Decisions["D-2026-06-23-001"].State != StateAccepted {
		t.Errorf("snapshot should still be accepted after live lock, got %q", snap.Decisions["D-2026-06-23-001"].State)
	}

	liveRec := p.GetDecision("D-2026-06-23-001")
	if liveRec.State != StateLocked {
		t.Errorf("live state should be locked, got %q", liveRec.State)
	}
}

func TestProjection_DedupBounded(t *testing.T) {
	p := NewProjection()
	p.dedupMax = 5 // small for testing

	// Insert more than dedupMax entries.
	for i := 0; i < 10; i++ {
		p.ApplyDecision(DecisionEvent{
			TS:       "2026-06-23T09:00:00+02:00",
			Action:   DecisionPropose,
			Decision: "D-2026-06-23-001",
			Detail:   string(rune('0' + i)), // make each event unique via detail
		})
	}

	p.mu.RLock()
	dedupSize := len(p.dedup)
	p.mu.RUnlock()

	if dedupSize > p.dedupMax {
		t.Errorf("dedup size = %d, should be <= %d", dedupSize, p.dedupMax)
	}

	// The decision record should still only have 1 history entry per unique (id,ts,action).
	// Each event has same id+ts+action but different detail, so dedup key is same.
	rec := p.GetDecision("D-2026-06-23-001")
	if len(rec.History) > 1 {
		t.Errorf("history should only have 1 entry (others deduped), got %d", len(rec.History))
	}
}

func TestProjection_Stats(t *testing.T) {
	p := NewProjection()
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-001"})
	p.ApplyDecision(DecisionEvent{TS: "2026-06-23T09:01:00+02:00", Action: DecisionPropose, Decision: "D-002"})
	p.ApplyNote(NoteEvent{TS: "2026-06-23T08:00:00+02:00", Action: NoteAdd, Note: "N-001"})

	d, n := p.Stats()
	if d != 2 {
		t.Errorf("decisions = %d, want 2", d)
	}
	if n != 1 {
		t.Errorf("notes = %d, want 1", n)
	}
}

func TestProjection_GetUnknownDecision(t *testing.T) {
	p := NewProjection()
	rec := p.GetDecision("D-nonexistent")
	if rec != nil {
		t.Error("expected nil for unknown decision")
	}
}

func TestProjection_ReplayDecisions(t *testing.T) {
	p := NewProjection()
	events := []DecisionEvent{
		{TS: "2026-06-23T09:00:00+02:00", Action: DecisionPropose, Decision: "D-001"},
		{TS: "2026-06-23T09:15:30+02:00", Action: DecisionAccept, Decision: "D-001"},
		{TS: "2026-06-23T09:18:22+02:00", Action: DecisionLock, Decision: "D-001"},
	}
	SortDecisionEvents(events)
	p.ReplayDecisions(events)

	rec := p.GetDecision("D-001")
	if rec == nil {
		t.Fatal("decision not found after replay")
	}
	if rec.State != StateLocked {
		t.Errorf("state = %q, want locked", rec.State)
	}
	if len(rec.History) != 3 {
		t.Errorf("history length = %d, want 3", len(rec.History))
	}
}
