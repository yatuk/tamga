package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripScope(t *testing.T) {
	for value, fields := range map[string]bool{"": false, "text": false, "field": true, " Field ": true} {
		p, err := LoadFromBytes([]byte("version: \"1.0\"\nscan:\n  strip_scope: \"" + value + "\"\n"))
		if err != nil {
			t.Fatalf("strip_scope %q: %v", value, err)
		}
		if p.StripFields() != fields {
			t.Errorf("strip_scope %q: StripFields() = %v", value, p.StripFields())
		}
	}
	_, err := LoadFromBytes([]byte("version: \"1.0\"\nscan:\n  strip_scope: fields\n"))
	if err == nil || !strings.Contains(err.Error(), "strip_scope") {
		t.Fatalf("a misspelled strip_scope loaded: %v", err)
	}
	var nilPolicy *Policy
	if nilPolicy.StripFields() {
		t.Fatal("a nil policy narrows STRIP")
	}
}

// A policy without strip_scope keeps the digest it had before the setting
// existed: the JSON form leaves it out.
func TestStripScopeLeavesTheDigestAlone(t *testing.T) {
	p, err := LoadFromBytes([]byte("version: \"1.0\"\nscan:\n  on_error: block\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustJSON(t, p.Scan)), "strip_scope") {
		t.Fatal("strip_scope is in the JSON form of a policy that does not set it")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
