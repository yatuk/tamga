package store

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/operator_state"
)

// A card number that passes Luhn and belongs to nobody, as the scanner lets
// it out, and what the scanner says about it beside the match.
const (
	testCardMasked = "453201******0366"
	testCardBIN    = "453201"
	testCardBank   = "Test Bankasi"
)

func hashingPolicy() *policy.Policy {
	return &policy.Policy{Data: &policy.DataControl{HashFindings: true}}
}

// sensitiveEvent is a request's event with a national id in the request and
// a card in the answer, each with the metadata its scanner adds.
func sensitiveEvent() events.Event {
	return events.Event{
		RequestID: uuid.NewString(),
		OrgID:     evOrg,
		EventType: "request_scanned",
		Action:    "REDACT",
		Findings: []scanner.Finding{{
			Type: "pii", Category: "tc_kimlik", Severity: "critical", Match: testTCKNMasked,
			StartPos: 4, EndPos: 15, Confidence: 0.9, Role: "user", Path: "messages[0].content",
			Metadata: map[string]string{
				"detected_via":    "digit_separators_removed",
				"denylist_match":  "true",
				"denylist_reason": "Mustafa Kemal Atatürk (public record, Law 2587)",
			},
		}},
		OutputFindings: []scanner.Finding{{
			Type: "pii", Category: "credit_card", Severity: "critical", Match: testCardMasked,
			Metadata: map[string]string{
				"card_brand": "visa", "card_type": "credit", "bank_name": testCardBank, "country": "TR", "bin": testCardBIN,
			},
		}},
		OutputAction:  "REDACT",
		OutputScanned: true,
	}
}

// leaks are the strings no sink may hold once data.hash_findings is on.
var leaks = []string{testTCKNMasked, testCardMasked, testCardBIN, testCardBank, "Atatürk", "denylist_reason", `"bin"`, `"bank_name"`, `"country"`}

func assertNoLeak(t *testing.T, sink string, raw []byte) {
	t.Helper()
	for _, leak := range leaks {
		if bytes.Contains(raw, []byte(leak)) {
			t.Errorf("%s holds %q: %s", sink, leak, raw)
		}
	}
}

// sinks is a bus with the subscribers that keep a finding or send it on: the
// ring of recent events, the live feed, the NATS envelope and the request
// log. Publishing returns what each of them got for one event.
type sinks struct {
	bus      *events.Bus
	recent   *events.RecentBuffer
	live     <-chan events.Event
	envelope chan events.EventV2
	rows     *captureStore
}

func newSinks(t *testing.T, log zerolog.Logger, getPolicy func() *policy.Policy) *sinks {
	t.Helper()
	s := &sinks{
		bus:      events.NewBus(),
		recent:   events.NewRecentBuffer(8),
		envelope: make(chan events.EventV2, 8),
		rows:     &captureStore{NoopStore: &NoopStore{}},
	}
	broker := events.NewBroker(8)
	live, unsub := broker.Subscribe()
	s.live = live
	s.bus.SetPrepare(EventFindingHasher(log, getPolicy))
	s.bus.Subscribe(events.RecentBufferHandler(s.recent))
	s.bus.Subscribe(broker.Publish)
	s.bus.Subscribe(func(e events.Event) { s.envelope <- events.ToV2(e) })
	s.bus.Subscribe(DBHandler(log, s.rows, evOrg, getPolicy))
	s.bus.Start()
	t.Cleanup(func() { s.bus.Stop(); unsub() })
	return s
}

// With data.hash_findings on, the match and what the metadata says of the
// value are in none of the places an event is kept in or sent to, and all
// of them hold the same digest.
func TestEventFindingHasher_EverySinkHoldsTheDigest(t *testing.T) {
	h := useTestFindingHasher(t)
	s := newSinks(t, zerolog.Nop(), hashingPolicy)
	e := sensitiveEvent()
	published := sensitiveEvent()
	published.RequestID = e.RequestID
	s.bus.Publish(e)
	s.bus.Stop()

	wantIn, wantOut := h.Hash(testTCKNMasked), h.Hash(testCardMasked)

	// The publisher's own findings are not written to: it may still use them.
	if !reflect.DeepEqual(e.Findings, published.Findings) || !reflect.DeepEqual(e.OutputFindings, published.OutputFindings) {
		t.Errorf("the publisher's findings were changed: %+v", e)
	}

	// The ring behind the console, the event detail and the export when
	// there is no database.
	ring, ok := s.recent.GetByRequestID(e.RequestID)
	if !ok || !ring.FindingsHashed {
		t.Fatalf("recent buffer: found=%v hashed=%v", ok, ring.FindingsHashed)
	}
	ringJSON, _ := json.Marshal(struct {
		API events.EventJSON
		Out []scanner.Finding
	}{events.EventToJSON(ring), ring.OutputFindings})
	assertNoLeak(t, "the recent buffer", ringJSON)
	if ring.Findings[0].Match != wantIn || ring.OutputFindings[0].Match != wantOut {
		t.Errorf("recent buffer matches %q, %q", ring.Findings[0].Match, ring.OutputFindings[0].Match)
	}

	// The live feed sends the same JSON view.
	live := <-s.live
	liveJSON, _ := json.Marshal(events.EventToJSON(live))
	assertNoLeak(t, "the live feed", liveJSON)
	if !strings.Contains(string(liveJSON), wantIn) {
		t.Errorf("the live feed has no digest: %s", liveJSON)
	}

	// The NATS envelope, which the stream keeps on disk.
	env := <-s.envelope
	envJSON, _ := json.Marshal(env)
	assertNoLeak(t, "the NATS envelope", envJSON)
	if !strings.Contains(string(envJSON), wantIn) || !strings.Contains(string(envJSON), wantOut) || !strings.Contains(string(envJSON), `"findings_hashed":true`) {
		t.Errorf("the NATS envelope: %s", envJSON)
	}

	// The request log, by the bus and by NATS: the digest once, not a digest
	// of the digest, and the same row either way.
	if len(s.rows.rows) != 1 {
		t.Fatalf("the bus writer saved %d rows", len(s.rows.rows))
	}
	viaBus := s.rows.rows[0]
	var back events.EventV2
	if err := json.Unmarshal(envJSON, &back); err != nil {
		t.Fatal(err)
	}
	viaNATS, ok := RequestLogFromV2(back, evOrg, hashingPolicy)
	if !ok {
		t.Fatal("the NATS consumer did not take the event")
	}
	for name, row := range map[string]RequestLog{"bus": viaBus, "nats": viaNATS} {
		assertNoLeak(t, "the row written by "+name, append(append([]byte{}, row.Findings...), row.OutputFindings...))
		var in, out []scanner.Finding
		if json.Unmarshal(row.Findings, &in) != nil || json.Unmarshal(row.OutputFindings, &out) != nil || len(in) != 1 || len(out) != 1 {
			t.Fatalf("%s row: %s / %s", name, row.Findings, row.OutputFindings)
		}
		if in[0].Match != wantIn || out[0].Match != wantOut {
			t.Errorf("%s row matches %q, %q; want the digest once", name, in[0].Match, out[0].Match)
		}
		// What describes the detection is still there.
		if in[0].Metadata["detected_via"] != "digit_separators_removed" || in[0].Metadata["denylist_match"] != "true" ||
			in[0].Category != "tc_kimlik" || in[0].Path != "messages[0].content" || in[0].EndPos != 15 {
			t.Errorf("%s row lost the detection: %+v", name, in[0])
		}
		if out[0].Metadata["card_brand"] != "visa" || out[0].Metadata["card_type"] != "credit" || len(out[0].Metadata) != 2 {
			t.Errorf("%s row card metadata: %v", name, out[0].Metadata)
		}
	}
	if !reflect.DeepEqual(viaBus.Findings, viaNATS.Findings) || !reflect.DeepEqual(viaBus.OutputFindings, viaNATS.OutputFindings) {
		t.Errorf("the two writers disagree:\n bus  %s\n nats %s", viaBus.Findings, viaNATS.Findings)
	}
}

// With the option off an event goes through as it was published.
func TestEventFindingHasher_OffChangesNothing(t *testing.T) {
	useTestFindingHasher(t)
	for name, getPolicy := range map[string]func() *policy.Policy{
		"no policy source": nil,
		"no policy":        func() *policy.Policy { return nil },
		"no data section":  func() *policy.Policy { return &policy.Policy{} },
		"option off":       func() *policy.Policy { return &policy.Policy{Data: &policy.DataControl{}} },
	} {
		e := sensitiveEvent()
		got := EventFindingHasher(zerolog.Nop(), getPolicy)(e)
		if got.FindingsHashed || !reflect.DeepEqual(got, e) {
			t.Errorf("%s: the event was changed: %+v", name, got)
		}
	}
}

// An event that is already hashed is not hashed again, by the bus or by the
// request log, and stays hashed when the option has been turned off since.
func TestEventFindingHasher_HashesOnce(t *testing.T) {
	h := useTestFindingHasher(t)
	prepare := EventFindingHasher(zerolog.Nop(), hashingPolicy)
	once := prepare(sensitiveEvent())
	twice := prepare(once)
	if !reflect.DeepEqual(once, twice) || once.Findings[0].Match != h.Hash(testTCKNMasked) {
		t.Errorf("hashed twice: %q then %q", once.Findings[0].Match, twice.Findings[0].Match)
	}
	for name, getPolicy := range map[string]func() *policy.Policy{"option on": hashingPolicy, "option off since": nil} {
		row, ok := RequestLogFromEvent(once, evOrg, getPolicy)
		if !ok || !strings.Contains(string(row.Findings), `"match":"`+h.Hash(testTCKNMasked)+`"`) {
			t.Errorf("%s: stored %s", name, row.Findings)
		}
	}
}

// Without a key every sink holds the placeholder and nothing of the value,
// and the proxy says so once, however many sinks and events there are.
func TestEventFindingHasher_WithoutKey(t *testing.T) {
	noFindingHasher(t)
	var logged bytes.Buffer
	log := zerolog.New(&logged)
	s := newSinks(t, log, hashingPolicy)
	before := UnkeyedMatches()
	var last events.Event
	for i := 0; i < 3; i++ {
		last = sensitiveEvent()
		// An operator id is hashed with the key; without one it is dropped.
		last.Findings[0].Metadata["operator_id"] = "op-ayse"
		s.bus.Publish(last)
	}
	s.bus.Stop()

	ring, _ := s.recent.GetByRequestID(last.RequestID)
	ringJSON, _ := json.Marshal(ring)
	envJSON, _ := json.Marshal(<-s.envelope)
	liveJSON, _ := json.Marshal(events.EventToJSON(<-s.live))
	if len(s.rows.rows) != 3 {
		t.Fatalf("saved %d rows, want 3", len(s.rows.rows))
	}
	row := s.rows.rows[2]
	for sink, raw := range map[string][]byte{
		"the recent buffer": ringJSON, "the NATS envelope": envJSON, "the live feed": liveJSON,
		"the row": append(append([]byte{}, row.Findings...), row.OutputFindings...),
	} {
		assertNoLeak(t, sink, raw)
		if !bytes.Contains(raw, []byte(UnkeyedMatch)) {
			t.Errorf("%s does not hold the placeholder: %s", sink, raw)
		}
		for _, derived := range []string{KeyedMatchPrefix, LegacyMatchPrefix, "op-ayse", "operator_id"} {
			if bytes.Contains(raw, []byte(derived)) {
				t.Errorf("without a key %s holds %q: %s", sink, derived, raw)
			}
		}
	}
	// Two findings an event, hashed once each: the row is not counted again.
	if got := UnkeyedMatches() - before; got != 6 {
		t.Errorf("counted %d matches left out, want 6", got)
	}
	if n := strings.Count(logged.String(), `"level":"error"`); n != 1 {
		t.Errorf("logged %d errors for three events and four sinks, want one: %s", n, logged.String())
	}
	if !strings.Contains(logged.String(), FindingHashKeyEnv) {
		t.Errorf("the log line does not name the variable: %s", logged.String())
	}

	// A writer that meets an unhashed event later says nothing more.
	DBHandler(log, s.rows, evOrg, hashingPolicy)(sensitiveEvent())
	if n := strings.Count(logged.String(), `"level":"error"`); n != 1 {
		t.Errorf("a second sink logged again: %s", logged.String())
	}
}

// Every metadata key a scanner emits is either kept, hashed or dropped on
// purpose. A key that is not listed is dropped.
func TestHashFindingMatches_Metadata(t *testing.T) {
	h := useTestFindingHasher(t)
	kept := map[string]string{
		"detected_via": "reversed_digits", "denylist_match": "true", "card_brand": "visa", "card_type": "debit",
		"variants": "2", "spans_segments": "3", "layer": "base64/hex", "kind": "zero_width", "count": "12",
		"blocks": "2", "chars": "900", "signal": "hidden_text", "threshold": "0.50",
		"current_state": "rejected", "required_state": "locked", "note_state": "archived", "rule": "payments need a locked decision",
		"action_on_fail": "block", "last_verified": "2026-01-02T03:04:05Z", "ttl_seconds": "3600", "needs_deep_analysis": "true",
		"confidence_score_total": "score=90 action=BLOCK factors=[format match (+40)]",
	}
	dropped := map[string]string{
		"bin": testCardBIN, "bank_name": testCardBank, "country": "TR",
		"denylist_reason": "Sequential pattern (all 1s with valid checksum)",
		"decision_id":     "DEC-1042", "note_id": "NOTE-7",
		"detail": "rejected by the risk committee", "message": "decision DEC-1042 is rejected",
		"a_key_added_later": "4532015112830366",
	}
	meta := map[string]string{"operator_id": "op-ayse"}
	for k, v := range kept {
		meta[k] = v
	}
	for k, v := range dropped {
		meta[k] = v
	}
	in := []scanner.Finding{{Type: "operator_state", Category: "unauthorized_operator", Match: "DEC-1042", Metadata: meta}}
	out := hashFindingMatches(in)

	got := out[0].Metadata
	for k, v := range kept {
		if got[k] != v {
			t.Errorf("kept key %q = %q, want %q", k, got[k], v)
		}
	}
	for k := range dropped {
		if _, ok := got[k]; ok {
			t.Errorf("key %q was kept: %q", k, got[k])
		}
	}
	if got["operator_id"] != h.Hash("op-ayse") {
		t.Errorf("operator_id = %q, want its keyed digest", got["operator_id"])
	}
	if len(got) != len(kept)+1 {
		t.Errorf("%d keys left, want %d: %v", len(got), len(kept)+1, got)
	}
	if out[0].Match != h.Hash("DEC-1042") {
		t.Errorf("match = %q", out[0].Match)
	}
	// The finding given keeps its own map.
	if len(in[0].Metadata) != len(meta) || in[0].Metadata["bin"] != testCardBIN || in[0].Match != "DEC-1042" {
		t.Errorf("the input was changed: %+v", in[0])
	}

	// Nothing left is no metadata at all, and none stays none.
	bare := hashFindingMatches([]scanner.Finding{{Match: "x", Metadata: map[string]string{"bin": testCardBIN}}, {Match: "y"}})
	if bare[0].Metadata != nil || bare[1].Metadata != nil {
		t.Errorf("metadata %v, %v; want none", bare[0].Metadata, bare[1].Metadata)
	}
	raw, _ := json.Marshal(bare)
	if strings.Contains(string(raw), "metadata") {
		t.Errorf("an empty metadata is written: %s", raw)
	}
}

// The findings an operator-state deep-scan event takes to the analyzer are
// hashed like any other, and the scanner's own request is left as it was.
func TestEventFindingHasher_OperatorStateDeepScan(t *testing.T) {
	h := useTestFindingHasher(t)
	fast, _ := json.Marshal(scanner.Finding{
		Type: "operator_state", Category: "state_assertion_failed", Severity: "high", Match: "DEC-1042",
		Metadata: map[string]string{"decision_id": "DEC-1042", "current_state": "rejected", "message": "decision DEC-1042 is rejected"},
	})
	req := &operator_state.AnalyzerRequest{RequestID: "r-1", ScanType: "operator_state_semantic", FastFindings: []json.RawMessage{fast}}
	meta := map[string]interface{}{"operator_state_request": req, "other": 1}
	e := events.Event{EventType: events.EventOperatorStateDeepScan, RequestID: "r-1", Metadata: meta}

	got := EventFindingHasher(zerolog.Nop(), hashingPolicy)(e)
	sent, _ := got.Metadata["operator_state_request"].(*operator_state.AnalyzerRequest)
	if sent == nil || sent == req || len(sent.FastFindings) != 1 || got.Metadata["other"] != 1 {
		t.Fatalf("the event's request: %+v", got.Metadata)
	}
	if strings.Contains(string(sent.FastFindings[0]), "DEC-1042") || !strings.Contains(string(sent.FastFindings[0]), h.Hash("DEC-1042")) ||
		!strings.Contains(string(sent.FastFindings[0]), `"current_state":"rejected"`) {
		t.Errorf("the analyzer would get %s", sent.FastFindings[0])
	}
	if sent.RequestID != "r-1" || sent.ScanType != "operator_state_semantic" {
		t.Errorf("the request lost its fields: %+v", sent)
	}
	if !bytes.Equal(req.FastFindings[0], fast) || meta["operator_state_request"] != req {
		t.Error("the scanner's own request was changed")
	}
}

func TestStoredDigestForms(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	forms := StoredDigestForms(digest)
	if len(forms) != 2 || forms[0] != KeyedMatchPrefix+digest || forms[1] != LegacyMatchPrefix+digest {
		t.Fatalf("forms %v", forms)
	}
	for _, bad := range []string{"", "%", "_", "a", digest[:63], digest + "a", strings.ToUpper(digest), digest[:63] + "%", digest[:63] + "_",
		KeyedMatchPrefix + digest, digest[:62] + `"}`} {
		if got := StoredDigestForms(bad); got != nil {
			t.Errorf("StoredDigestForms(%q) = %v, want none", bad, got)
		}
	}
}

// TestPG_SubjectByStoredDigest looks rows up by a digest copied from one:
// under either scheme, in the request's findings and in the response's, and
// by the whole match only. Needs TAMGA_TEST_DB_URL.
func TestPG_SubjectByStoredDigest(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()
	h := useTestFindingHasher(t)

	keyed := h.Hash(testTCKNMasked)
	digest := strings.TrimPrefix(keyed, KeyedMatchPrefix)
	otherDigest := strings.TrimPrefix(h.Hash("20*******46"), KeyedMatchPrefix)
	finding := func(f scanner.Finding) []byte {
		f.Type, f.Severity = "pii", "critical"
		if f.Category == "" {
			f.Category = "tc_kimlik"
		}
		b, _ := json.Marshal([]scanner.Finding{f})
		return b
	}
	rows := map[string]RequestLog{
		"keyed":           {Findings: finding(scanner.Finding{Match: keyed})},
		"keyed-in-answer": {Findings: []byte(`[]`), OutputFindings: finding(scanner.Finding{Match: keyed})},
		"legacy":          {Findings: finding(scanner.Finding{Match: LegacyMatchPrefix + digest})},
		"other-keyed":     {Findings: finding(scanner.Finding{Match: KeyedMatchPrefix + otherDigest})},
		// The digest as a part of a match, bare, or in another field of the
		// finding is not that finding's match. A substring search took all
		// four.
		"superstring":     {Findings: finding(scanner.Finding{Match: keyed + "0"})},
		"bare":            {Findings: finding(scanner.Finding{Match: digest})},
		"in-category":     {Findings: finding(scanner.Finding{Match: testTCKNMasked, Category: keyed})},
		"in-metadata":     {Findings: finding(scanner.Finding{Match: testTCKNMasked, Metadata: map[string]string{"match": keyed}})},
		"masked":          {Findings: finding(scanner.Finding{Match: testTCKNMasked})},
		"unkeyed":         {Findings: finding(scanner.Finding{Match: UnkeyedMatch})},
		"none":            {Findings: []byte(`[]`)},
		"other-in-answer": {Findings: []byte(`[]`), OutputFindings: finding(scanner.Finding{Match: KeyedMatchPrefix + otherDigest})},
	}
	for user, rl := range rows {
		rl.RequestID, rl.OrgID, rl.Provider, rl.Model, rl.ActionTaken, rl.UserID = uuid.NewString(), testOrgID, "openai", "gpt-4o", "block", user
		rl.FindingsCount = 1
		pgInsertLog(t, s, rl)
		// pgInsertLog writes the columns of the first migration.
		if rl.OutputFindings != nil {
			if _, err := s.pool.Exec(ctx, "UPDATE request_logs SET output_findings = $1::jsonb WHERE user_identifier = $2", string(rl.OutputFindings), user); err != nil {
				t.Fatalf("set output_findings: %v", err)
			}
		}
	}
	count := func() int {
		var n int
		if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM request_logs WHERE org_id = $1", testOrgID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	got, err := s.SubjectAccess(ctx, testOrgID, "", "", digest, 0)
	if err != nil {
		t.Fatalf("SubjectAccess: %v", err)
	}
	found := map[string]bool{}
	for _, r := range got {
		found[r.UserID] = true
	}
	want := []string{"keyed", "keyed-in-answer", "legacy"}
	if len(found) != len(want) {
		t.Errorf("found %v, want %v", found, want)
	}
	for _, u := range want {
		if !found[u] {
			t.Errorf("row %q was not found", u)
		}
	}

	// A selector that is not a digest reaches no query: it is an error, and
	// on erase nothing is deleted. The API refuses these before the store.
	for _, bad := range []string{"%", "_", "a", strings.Repeat("_", 64), strings.Repeat("%", 64), digest[:63] + "%",
		strings.ToUpper(digest), keyed, digest[:32], `x"}]' OR '1'='1`} {
		if rows, err := s.SubjectAccess(ctx, testOrgID, "", "", bad, 0); err == nil || len(rows) != 0 {
			t.Errorf("SubjectAccess(%q) = %d rows, err %v; want an error and no rows", bad, len(rows), err)
		}
		if n, err := s.EraseSubject(ctx, testOrgID, "", "", bad); err == nil || n != 0 {
			t.Errorf("EraseSubject(%q) deleted %d, err %v; want an error and nothing deleted", bad, n, err)
		}
	}
	if left := count(); left != len(rows) {
		t.Fatalf("%d rows left after the refused selectors, want all %d", left, len(rows))
	}

	deleted, err := s.EraseSubject(ctx, testOrgID, "", "", digest)
	if err != nil || deleted != len(want) {
		t.Fatalf("EraseSubject deleted %d, err %v; want %d", deleted, err, len(want))
	}
	if left := count(); left != len(rows)-len(want) {
		t.Errorf("%d rows left, want %d", left, len(rows)-len(want))
	}
	// The other value's rows are there, in either column.
	rest, err := s.SubjectAccess(ctx, testOrgID, "", "", otherDigest, 0)
	if err != nil || len(rest) != 2 {
		t.Errorf("the other digest: %d rows, err %v; want 2", len(rest), err)
	}
}
