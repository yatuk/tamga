package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/patterns"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// The patterns of the review, and the limit each one breaks.
var costlyPatterns = []struct {
	name, pattern, limit string
}{
	{"twenty thousand alternations", strings.Repeat("(?:a|b)", 20000), "the limit is 1024"},
	{"just over the length limit", strings.Repeat("(?:a|b)", 147), "the limit is 1024"},
	{"bounded dot before a word", `(?s).{0,1000}secret`, "the limit is 500"},
	{"a thousand letters", `\p{L}{1000}`, "the limit is 500"},
	{"star", `x*`, "matches the empty string"},
}

// send posts or puts a JSON body with the admin key and returns the status
// and the response body.
func send(t *testing.T, method, url, key string, body interface{}) (int, string) {
	t.Helper()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	case []byte:
		raw = b
	default:
		raw, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, url, bytes.NewReader(raw))
	adminHeaders(key)(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

// refusedForLimit checks a refusal: the status, that the message names the
// limit, and that it does not carry the pattern.
func refusedForLimit(t *testing.T, where string, status int, body, limit string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Errorf("%s: status %d, want 400: %s", where, status, body)
	}
	if !strings.Contains(body, limit) {
		t.Errorf("%s: the answer does not name the limit %q: %s", where, limit, body)
	}
	if len(body) > 500 {
		t.Errorf("%s: the answer is %d bytes; it must not carry the pattern", where, len(body))
	}
}

func TestPatternsAPIRefusesCostlyPatterns(t *testing.T) {
	store := patterns.NewMemoryStore()
	existing, err := store.Create(patterns.Pattern{Name: "ok", Kind: "regex", Pattern: `MN-\d{8}`, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{AdminKey: "test-key", Patterns: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	for _, tc := range costlyPatterns {
		t.Run(tc.name, func(t *testing.T) {
			body := patterns.Pattern{Name: "costly", Kind: "regex", Pattern: tc.pattern, Enabled: true}
			status, out := send(t, "POST", ts.URL+"/api/v1/patterns", cfg.AdminKey, body)
			if len(tc.pattern) > maxPatternBodyBytes {
				if status != http.StatusRequestEntityTooLarge {
					t.Errorf("create: status %d, want 413: %s", status, out)
				}
			} else {
				refusedForLimit(t, "create", status, out, tc.limit)
			}
			status, out = send(t, "PUT", ts.URL+"/api/v1/patterns/"+existing.ID, cfg.AdminKey, body)
			if len(tc.pattern) > maxPatternBodyBytes {
				if status != http.StatusRequestEntityTooLarge {
					t.Errorf("update: status %d, want 413: %s", status, out)
				}
			} else {
				refusedForLimit(t, "update", status, out, tc.limit)
			}
		})
	}
	if got := store.List(); len(got) != 1 || got[0].Pattern != `MN-\d{8}` {
		t.Errorf("the store changed: %+v", got)
	}
}

func TestPatternsAPICountLimit(t *testing.T) {
	store := patterns.NewMemoryStore()
	cfg := Config{AdminKey: "test-key", Patterns: store, DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	for i := 0; i < scanner.MaxStoredPatterns; i++ {
		body := patterns.Pattern{Name: fmt.Sprintf("p%d", i), Kind: "regex", Pattern: fmt.Sprintf(`ID%d-\d{4}`, i), Enabled: true}
		if status, out := send(t, "POST", ts.URL+"/api/v1/patterns", cfg.AdminKey, body); status != http.StatusCreated {
			t.Fatalf("pattern %d of %d: status %d: %s", i+1, scanner.MaxStoredPatterns, status, out)
		}
	}
	body := patterns.Pattern{Name: "one more", Kind: "regex", Pattern: `ONE-\d{4}`, Enabled: true}
	status, out := send(t, "POST", ts.URL+"/api/v1/patterns", cfg.AdminKey, body)
	if status != http.StatusBadRequest || !strings.Contains(out, "100 patterns") {
		t.Errorf("one over the limit: status %d: %s", status, out)
	}
	// A full store can still be edited.
	first := store.List()[0]
	first.Pattern = `EDIT-\d{4}`
	if status, out := send(t, "PUT", ts.URL+"/api/v1/patterns/"+first.ID, cfg.AdminKey, first); status != http.StatusOK {
		t.Errorf("update at the limit: status %d: %s", status, out)
	}
}

func TestPatternsAPIBodyLimit(t *testing.T) {
	cfg := Config{AdminKey: "test-key", Patterns: patterns.NewMemoryStore(), DefaultOrgID: "org-1"}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	big := `{"name":"` + strings.Repeat("n", maxPatternBodyBytes) + `","kind":"regex","pattern":"abc"}`
	if status, out := send(t, "POST", ts.URL+"/api/v1/patterns", cfg.AdminKey, big); status != http.StatusRequestEntityTooLarge {
		t.Errorf("status %d, want 413: %s", status, out)
	}
}

func simulateConfig() Config {
	return Config{
		AdminKey:     "test-key",
		DefaultOrgID: "org-1",
		PolicyStore:  policy.NewPolicyStore(&policy.Policy{Name: "active", Version: "1.0"}),
	}
}

func TestSimulateRefusesCostlyPatterns(t *testing.T) {
	cfg := simulateConfig()
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	for _, tc := range costlyPatterns {
		t.Run(tc.name, func(t *testing.T) {
			quoted, _ := json.Marshal(tc.pattern)
			for where, doc := range map[string]string{
				"custom entity": "name: draft\ncustom_entities:\n  - name: e\n    pattern: " + string(quoted) + "\n",
				"competitor":    "name: draft\ncompetitors:\n  - name: Rival\n    enabled: true\n    patterns: [" + string(quoted) + "]\n",
			} {
				status, out := send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey,
					map[string]string{"yaml": doc, "sample_text": strings.Repeat("ab secret ", 1000)})
				refusedForLimit(t, where, status, out, tc.limit)
			}
		})
	}
}

func TestSimulateBodyLimit(t *testing.T) {
	cfg := simulateConfig()
	// This is a test of the size limit, not of how fast the machine is: the
	// scan of a text this size gets all the time it needs.
	cfg.SimulateTimeout = 2 * time.Minute
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	fits := map[string]string{"sample_text": strings.Repeat("a", maxSimulateTextBytes)}
	status, out := send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, fits)
	if status != http.StatusOK {
		t.Errorf("a text of exactly the limit: status %d: %.200s", status, out)
	}
	over := map[string]string{"sample_text": strings.Repeat("a", maxSimulateTextBytes+1)}
	status, out = send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, over)
	if status != http.StatusRequestEntityTooLarge || !strings.Contains(out, "262144") {
		t.Errorf("a text one byte over the limit: status %d, want 413: %.200s", status, out)
	}

	// The body as a whole: {"yaml":"...","sample_text":""} is 28 bytes
	// around the policy.
	pad := func(n int) map[string]string {
		return map[string]string{"yaml": "name: t #" + strings.Repeat("x", n-28-len("name: t #")), "sample_text": ""}
	}
	if status, out := send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, pad(maxSimulateBodyBytes)); status != http.StatusOK {
		t.Errorf("a body of exactly the limit: status %d: %.200s", status, out)
	}
	status, out = send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, pad(maxSimulateBodyBytes+1))
	if status != http.StatusRequestEntityTooLarge || !strings.Contains(out, "524288") {
		t.Errorf("a body one byte over the limit: status %d, want 413: %.200s", status, out)
	}
}

func TestSimulateReturnsMaskedMatches(t *testing.T) {
	cfg := simulateConfig()
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	doc := `name: draft
custom_entities:
  - name: musteri_no
    pattern: "MN-\\d{8}"
    action: REDACT
competitors:
  - name: Rival
    enabled: true
    action: warn
    patterns: ["rival\\s+guard\\s+\\w+"]
`
	text := "Customer MN-12345678 asked how we compare with Rival Guard Enterprise."
	status, out := send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, map[string]string{"yaml": doc, "sample_text": text})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, out)
	}
	var res struct {
		Findings []struct {
			Type, Category, Match string
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range res.Findings {
		got[f.Type] = f.Match
	}
	if got["custom"] != "MN-******78" {
		t.Errorf("custom match = %q", got["custom"])
	}
	if got["competitor"] != "Ri*** ***** ********se" {
		t.Errorf("competitor match = %q", got["competitor"])
	}
	for _, raw := range []string{"MN-12345678", "Rival Guard Enterprise", "Guard", "12345678"} {
		if strings.Contains(out, raw) {
			t.Errorf("the response carries %q: %s", raw, out)
		}
	}
}

// A simulation the scan could not finish has no verdict; it must not come
// back as a 200 with whatever was found before the cut.
func TestSimulateDoesNotAnswerForATruncatedScan(t *testing.T) {
	cfg := simulateConfig()
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	doc := "name: draft\ncustom_entities:\n  - name: every_a\n    pattern: \"a\"\n    action: BLOCK\n"
	status, out := send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey,
		map[string]string{"yaml": doc, "sample_text": strings.Repeat("a", scanner.MaxFindingsPerScan+1)})
	if status != http.StatusServiceUnavailable || !strings.Contains(out, "more than 1000 matches") {
		t.Errorf("status %d, want 503 naming the cap: %.300s", status, out)
	}
	status, out = send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey,
		map[string]string{"yaml": doc, "sample_text": strings.Repeat("a", scanner.MaxFindingsPerScan)})
	if status != http.StatusOK || !strings.Contains(out, `"action":"BLOCK"`) {
		t.Errorf("at the cap: status %d: %.300s", status, out)
	}
}

func entityConfig(t *testing.T, policyYAML string) (Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(policyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	pol, err := policy.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return Config{AdminKey: "test-key", DefaultOrgID: "org-1", PolicyPath: path, PolicyStore: policy.NewPolicyStore(pol)}, path
}

func TestCustomEntityCreateRefusesCostlyPatterns(t *testing.T) {
	const before = "version: \"1.0\"\nname: test\n"
	cfg, path := entityConfig(t, before)
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	for _, tc := range costlyPatterns {
		t.Run(tc.name, func(t *testing.T) {
			status, out := send(t, "POST", ts.URL+"/api/v1/policies/custom-entities", cfg.AdminKey,
				policy.CustomEntity{Name: "costly", Pattern: tc.pattern, Action: "BLOCK"})
			if len(tc.pattern) > maxEntityBodyBytes {
				if status != http.StatusRequestEntityTooLarge {
					t.Errorf("status %d, want 413: %s", status, out)
				}
				return
			}
			refusedForLimit(t, "create", status, out, tc.limit)
		})
	}
	if raw, _ := os.ReadFile(path); string(raw) != before {
		t.Errorf("the policy file changed: %s", raw)
	}
}

func TestCustomEntityCreateRefusesNamesThatAreNotIdentifiers(t *testing.T) {
	const before = "version: \"1.0\"\nname: test\n"
	cfg, path := entityConfig(t, before)
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	for _, name := range []string{"employee id", "a]b", "line\nbreak", "müşteri", "dash-name", strings.Repeat("n", policy.MaxEntityNameBytes+1)} {
		status, out := send(t, "POST", ts.URL+"/api/v1/policies/custom-entities", cfg.AdminKey,
			policy.CustomEntity{Name: name, Pattern: `MN-\d{8}`, Action: "REDACT"})
		if status != http.StatusBadRequest || !strings.Contains(out, "custom_entities[0].name") {
			t.Errorf("%q: status %d: %s", name, status, out)
		}
	}
	if raw, _ := os.ReadFile(path); string(raw) != before {
		t.Errorf("the policy file changed: %s", raw)
	}
}

func TestCustomEntityCreateCountLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("version: \"1.0\"\nname: test\ncustom_entities:\n")
	for i := 0; i < scanner.MaxCustomEntities-1; i++ {
		fmt.Fprintf(&b, "  - name: e%d\n    pattern: \"ID%d-[0-9]{4}\"\n", i, i)
	}
	cfg, _ := entityConfig(t, b.String())
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	status, out := send(t, "POST", ts.URL+"/api/v1/policies/custom-entities", cfg.AdminKey,
		policy.CustomEntity{Name: "the_last", Pattern: `LAST-\d{4}`, Action: "REDACT"})
	if status != http.StatusCreated {
		t.Fatalf("entity %d: status %d: %s", scanner.MaxCustomEntities, status, out)
	}
	status, out = send(t, "POST", ts.URL+"/api/v1/policies/custom-entities", cfg.AdminKey,
		policy.CustomEntity{Name: "one_more", Pattern: `MORE-\d{4}`, Action: "REDACT"})
	if status != http.StatusBadRequest || !strings.Contains(out, "101 custom entities, the limit is 100") {
		t.Errorf("one over the limit: status %d: %s", status, out)
	}
	if n := len(cfg.PolicyStore.GetPolicy().CustomEntities); n != scanner.MaxCustomEntities {
		t.Errorf("the policy holds %d entities, want %d", n, scanner.MaxCustomEntities)
	}
}
