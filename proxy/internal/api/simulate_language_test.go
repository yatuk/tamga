package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The simulation judges the sample as a user message, as the proxy path
// judges one: a German text under a policy that allows Turkish and English
// is a language finding with the section's action, and names the languages.
func TestSimulate_LanguageFinding(t *testing.T) {
	cfg := simulateConfig()
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()
	doc := "name: draft\nlanguage:\n  enabled: true\n  allow: [tr, en]\n  action: BLOCK\n"
	german := "Hallo, das Produkt, das ich letzte Woche bestellt habe, kam beschädigt an. Eine Ecke des Kartons war eingedrückt und das Gerät funktioniert nicht."
	status, out := send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, map[string]string{"yaml": doc, "sample_text": german})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, out)
	}
	var res struct {
		Action   string `json:"action"`
		Findings []struct {
			Type, Category, Match, Action string
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Action != "BLOCK" || len(res.Findings) != 1 || res.Findings[0].Type != "language" ||
		res.Findings[0].Category != "language_not_allowed" || res.Findings[0].Match != "de 100%" {
		t.Fatalf("simulation: %s", out)
	}
	// With the section off nothing is found.
	off := "name: draft\nlanguage:\n  enabled: false\n  allow: [tr, en]\n"
	status, out = send(t, "POST", ts.URL+"/api/v1/policies/simulate", cfg.AdminKey, map[string]string{"yaml": off, "sample_text": german})
	if status != http.StatusOK || json.Unmarshal([]byte(out), &res) != nil || len(res.Findings) != 0 {
		t.Fatalf("section off: %d %s", status, out)
	}
}
