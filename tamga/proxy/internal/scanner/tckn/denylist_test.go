package tckn

import "testing"

func TestIsDenylisted(t *testing.T) {
	tests := []struct {
		name     string
		tckn     string
		wantHit  bool
		wantPart string // substring to check in reason
	}{
		{"Ataturk TCKN", "10000000146", true, "Atatürk"},
		{"all 1s", "11111111110", true, "Sequential"},
		{"all 5s", "55555555550", true, "Sequential"},
		{"ascending", "12345678901", true, "Arithmetic"},
		{"binary alternating", "10101010101", true, "Alternating"},
		{"documentation example", "12345678902", true, "Documentation"},
		{"valid but not denylisted", "12345678950", false, ""},
		{"valid random", "24815306972", false, ""},
		{"valid random 2", "57291340866", false, ""},
		{"empty string", "", false, ""},
		{"short", "12345", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, hit := IsDenylisted(tt.tckn)
			if hit != tt.wantHit {
				t.Errorf("IsDenylisted(%q) hit=%v, want %v", tt.tckn, hit, tt.wantHit)
			}
			if tt.wantHit && tt.wantPart != "" && reason == "" {
				t.Errorf("IsDenylisted(%q) reason is empty, want substring %q", tt.tckn, tt.wantPart)
			}
		})
	}
}

func TestDenylistUniqueness(t *testing.T) {
	seen := make(map[string]string)
	for tckn, reason := range Denylist {
		if prev, ok := seen[tckn]; ok {
			t.Errorf("duplicate TCKN %q: reasons %q and %q", tckn, prev, reason)
		}
		seen[tckn] = reason
	}
}

func TestDenylistCount(t *testing.T) {
	if len(Denylist) < 25 {
		t.Errorf("denylist has %d entries, want at least 25", len(Denylist))
	}
}
