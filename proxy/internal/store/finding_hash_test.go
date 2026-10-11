package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// testTCKN is a checksum-valid national id that belongs to nobody, and what
// the scanner lets out of it.
const (
	testTCKN       = "10000000146"
	testTCKNMasked = "10*******46"
)

// testKeyB64 returns a base64 key of n bytes, all of one value.
func testKeyB64(n int, fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, n))
}

// useTestFindingHasher gives the process a finding hash key for one test.
func useTestFindingHasher(t *testing.T) *FindingHasher {
	t.Helper()
	h, err := NewFindingHasher(testKeyB64(32, 0x41), "")
	if err != nil || h == nil {
		t.Fatalf("test hasher: %v", err)
	}
	prev := CurrentFindingHasher()
	SetFindingHasher(h)
	t.Cleanup(func() { SetFindingHasher(prev) })
	return h
}

// noFindingHasher takes the key away for one test.
func noFindingHasher(t *testing.T) {
	t.Helper()
	prev := CurrentFindingHasher()
	SetFindingHasher(nil)
	t.Cleanup(func() { SetFindingHasher(prev) })
}

func plainSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestNewFindingHasher_KeySource(t *testing.T) {
	dedicated, vault := testKeyB64(32, 0x41), testKeyB64(32, 0x42)

	if h, err := NewFindingHasher("", ""); h != nil || err != nil {
		t.Fatalf("no key set: got hasher %v, err %v; want neither", h, err)
	}

	fromDedicated, err := NewFindingHasher(dedicated, vault)
	if err != nil || fromDedicated.Source() != FindingHashKeyEnv {
		t.Fatalf("dedicated key: source %q, err %v", fromDedicated.Source(), err)
	}
	fromVault, err := NewFindingHasher("", vault)
	if err != nil || !strings.Contains(fromVault.Source(), "TAMGA_VAULT_KEY") {
		t.Fatalf("vault key: source %q, err %v", fromVault.Source(), err)
	}
	if fromDedicated.Hash("x") == fromVault.Hash("x") {
		t.Error("the dedicated key and the derived one give the same digest")
	}

	// The vault key is not the HMAC key: one is derived from it.
	usedDirectly, _ := NewFindingHasher(vault, "")
	if fromVault.Hash("x") == usedDirectly.Hash("x") {
		t.Error("the vault key itself is used as the HMAC key")
	}
	// The same secret gives the same digests on every replica and restart.
	again, _ := NewFindingHasher("", vault)
	if fromVault.Hash("x") != again.Hash("x") {
		t.Error("the derived key differs between two starts")
	}

	for name, tc := range map[string]struct{ dedicated, vault string }{
		"dedicated not base64":            {"not base64!", ""},
		"dedicated too short":             {testKeyB64(16, 1), ""},
		"dedicated bad, vault good":       {testKeyB64(16, 1), vault},
		"vault not base64":                {"", "not base64!"},
		"vault of the wrong length":       {"", testKeyB64(16, 1)},
		"dedicated not base64, vault set": {"***", vault},
	} {
		h, err := NewFindingHasher(tc.dedicated, tc.vault)
		if h != nil || err == nil {
			t.Errorf("%s: got hasher %v, err %v; want an error and no hasher", name, h, err)
			continue
		}
		// An error is logged: it must not repeat the secret.
		for _, secret := range []string{tc.dedicated, tc.vault} {
			if secret != "" && strings.Contains(err.Error(), secret) {
				t.Errorf("%s: the error repeats the key", name)
			}
		}
	}
}

// The stored digest is not one a reader of the table can compute: neither
// the plain SHA-256 of the masked match that earlier versions stored, nor
// that of the value.
func TestHashFindingMatches_IsKeyed(t *testing.T) {
	h := useTestFindingHasher(t)
	out := hashFindingMatches([]scanner.Finding{{Type: "pii", Category: "tc_kimlik", Match: testTCKNMasked}})
	got := out[0].Match
	if !strings.HasPrefix(got, KeyedMatchPrefix) || len(got) != len(KeyedMatchPrefix)+64 {
		t.Fatalf("stored match %q", got)
	}
	for _, guess := range []string{testTCKNMasked, testTCKN} {
		if strings.Contains(got, plainSHA256(guess)) {
			t.Errorf("the stored digest is the plain SHA-256 of %q", guess)
		}
	}
	if got != h.Hash(testTCKNMasked) {
		t.Error("the same match does not give the same digest")
	}
	other, _ := NewFindingHasher(testKeyB64(32, 0x43), "")
	if got == other.Hash(testTCKNMasked) {
		t.Error("another key gives the same digest")
	}
}

// Without a key nothing derived from the match is stored, and it is said.
func TestHashFindings_WithoutKey(t *testing.T) {
	noFindingHasher(t)
	var logged bytes.Buffer
	spy := &spyStore{}
	hashing := func() *policy.Policy { return &policy.Policy{Data: &policy.DataControl{HashFindings: true}} }
	handler := DBHandler(zerolog.New(&logged), spy, "org-1", hashing)
	before := UnkeyedMatches()
	for i := 0; i < 3; i++ {
		handler(events.Event{
			EventType: "request_scanned",
			OrgID:     "org-1",
			Findings:  []scanner.Finding{{Type: "pii", Category: "tc_kimlik", Severity: "critical", Match: testTCKNMasked}},
		})
	}
	var stored []scanner.Finding
	if err := json.Unmarshal(spy.lastFindings, &stored); err != nil || len(stored) != 1 {
		t.Fatalf("stored findings %s: %v", spy.lastFindings, err)
	}
	if stored[0].Match != UnkeyedMatch || stored[0].Category != "tc_kimlik" || stored[0].Severity != "critical" {
		t.Errorf("stored finding %+v", stored[0])
	}
	for _, leak := range []string{testTCKNMasked, plainSHA256(testTCKNMasked), LegacyMatchPrefix + plainSHA256(testTCKNMasked)[:8]} {
		if strings.Contains(string(spy.lastFindings), leak) {
			t.Errorf("without a key the row holds %q: %s", leak, spy.lastFindings)
		}
	}
	if got := UnkeyedMatches() - before; got != 3 {
		t.Errorf("counted %d matches left out, want 3", got)
	}
	if n := strings.Count(logged.String(), `"level":"error"`); n != 1 {
		t.Errorf("logged %d errors for three rows, want one: %s", n, logged.String())
	}
	if !strings.Contains(logged.String(), FindingHashKeyEnv) {
		t.Errorf("the log line does not name the variable: %s", logged.String())
	}

	// With hashing off the key is not needed and nothing is said.
	logged.Reset()
	DBHandler(zerolog.New(&logged), spy, "org-1", nil)(events.Event{
		EventType: "request_scanned",
		OrgID:     "org-1",
		Findings:  []scanner.Finding{{Type: "pii", Match: testTCKNMasked}},
	})
	if logged.Len() != 0 || !strings.Contains(string(spy.lastFindings), testTCKNMasked) {
		t.Errorf("hashing off: stored %s, logged %s", spy.lastFindings, logged.String())
	}
}

// A national id is looked for in every form a row may hold it in, and is in
// none of them.
func TestSubjectMatchForms(t *testing.T) {
	legacy := LegacyMatchPrefix + plainSHA256(testTCKNMasked)

	noFindingHasher(t)
	forms := SubjectMatchForms(testTCKN)
	if len(forms) != 2 || forms[0] != testTCKNMasked || forms[1] != legacy {
		t.Fatalf("without a key: %v", forms)
	}

	h := useTestFindingHasher(t)
	forms = SubjectMatchForms(testTCKN)
	if len(forms) != 3 || forms[0] != testTCKNMasked || forms[1] != legacy || forms[2] != h.Hash(testTCKNMasked) {
		t.Fatalf("with a key: %v", forms)
	}
	// What a row written now holds is one of them.
	stored := hashFindingMatches([]scanner.Finding{{Match: scanner.MaskMatch(testTCKN)}})[0].Match
	if stored != forms[2] {
		t.Errorf("a row written now holds %q, which the lookup does not ask for", stored)
	}
	for _, f := range forms {
		if strings.Contains(f, testTCKN) || strings.Contains(f, plainSHA256(testTCKN)) {
			t.Errorf("form %q holds the id or its plain hash", f)
		}
	}
}

func TestMatchContainmentDocs(t *testing.T) {
	docs := matchContainmentDocs([]string{"hmac-sha256:ab", "", `a"b\c%_`})
	if len(docs) != 2 {
		t.Fatalf("docs: %v", docs)
	}
	for i, want := range []string{"hmac-sha256:ab", `a"b\c%_`} {
		var parsed []map[string]string
		if err := json.Unmarshal([]byte(docs[i]), &parsed); err != nil || len(parsed) != 1 || len(parsed[0]) != 1 || parsed[0]["match"] != want {
			t.Errorf("doc %d = %s, want one finding with match %q", i, docs[i], want)
		}
	}
	// The statement is a constant: the forms are an argument, never text.
	if strings.Contains(subjectMatchWhere, "%") || !strings.Contains(subjectMatchWhere, "$2::text[]") {
		t.Errorf("subjectMatchWhere: %s", subjectMatchWhere)
	}
}

// TestPG_SubjectByMatch looks a national id up in rows of every age: masked,
// hashed by an earlier version, hashed under the key, in the request's
// findings and in the response's. Needs TAMGA_TEST_DB_URL.
func TestPG_SubjectByMatch(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()
	h := useTestFindingHasher(t)

	finding := func(match string) []byte {
		b, _ := json.Marshal([]scanner.Finding{{Type: "pii", Category: "tc_kimlik", Severity: "critical", Match: match}})
		return b
	}
	const otherMasked = "20*******46"
	rows := map[string]RequestLog{
		"masked":          {Findings: finding(testTCKNMasked)},
		"legacy":          {Findings: finding(legacyMatchHash(testTCKNMasked))},
		"keyed":           {Findings: finding(h.Hash(testTCKNMasked))},
		"keyed-in-answer": {Findings: []byte(`[]`), OutputFindings: finding(h.Hash(testTCKNMasked))},
		"other-masked":    {Findings: finding(otherMasked)},
		"other-keyed":     {Findings: finding(h.Hash(otherMasked))},
		// A longer match that contains the masked form is another value.
		"superstring": {Findings: finding("x" + testTCKNMasked)},
		"unkeyed":     {Findings: finding(UnkeyedMatch)},
		"none":        {Findings: []byte(`[]`)},
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

	got, err := s.SubjectAccessByMatch(ctx, testOrgID, SubjectMatchForms(testTCKN), 0)
	if err != nil {
		t.Fatalf("SubjectAccessByMatch: %v", err)
	}
	found := map[string]bool{}
	for _, r := range got {
		found[r.UserID] = true
	}
	want := []string{"masked", "legacy", "keyed", "keyed-in-answer"}
	if len(found) != len(want) {
		t.Errorf("found %v, want %v", found, want)
	}
	for _, u := range want {
		if !found[u] {
			t.Errorf("row %q was not found", u)
		}
	}

	if _, err := s.SubjectAccessByMatch(ctx, testOrgID, nil, 0); err == nil {
		t.Error("no forms: want an error, not every row")
	}
	if _, err := s.EraseSubjectByMatch(ctx, testOrgID, []string{""}); err == nil {
		t.Error("an empty form: want an error, not a delete")
	}

	deleted, err := s.EraseSubjectByMatch(ctx, testOrgID, SubjectMatchForms(testTCKN))
	if err != nil || deleted != len(want) {
		t.Fatalf("EraseSubjectByMatch deleted %d, err %v; want %d", deleted, err, len(want))
	}
	var left int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM request_logs WHERE org_id = $1", testOrgID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != len(rows)-len(want) {
		t.Errorf("%d rows left, want %d", left, len(rows)-len(want))
	}
}
