package policy

import "testing"

func TestOnMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		pol  *Policy
		want string
	}{
		{"nil policy", nil, MalformedBlock},
		{"no scan section", &Policy{}, MalformedBlock},
		{"empty value", &Policy{Scan: &ScanConfig{}}, MalformedBlock},
		{"block", &Policy{Scan: &ScanConfig{OnMalformed: "block"}}, MalformedBlock},
		{"raw_scan", &Policy{Scan: &ScanConfig{OnMalformed: "raw_scan"}}, MalformedRawScan},
		// A typo must not open the gate.
		{"unknown value", &Policy{Scan: &ScanConfig{OnMalformed: "rawscan"}}, MalformedBlock},
	}
	for _, tt := range tests {
		if got := tt.pol.OnMalformedJSON(); got != tt.want {
			t.Errorf("%s: OnMalformedJSON() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestScanSection_ParsesAndValidates(t *testing.T) {
	pol, err := LoadFromBytes([]byte("version: \"1.0\"\nname: t\nscan:\n  on_malformed: raw_scan\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if pol.OnMalformedJSON() != MalformedRawScan {
		t.Fatalf("on_malformed not read from YAML: %+v", pol.Scan)
	}
	if HasValidationErrors(ValidateSemantics(pol)) {
		t.Fatal("raw_scan is a valid value")
	}

	pol.Scan.OnMalformed = "ignore"
	if !HasValidationErrors(ValidateSemantics(pol)) {
		t.Fatal("an unknown on_malformed value must be a validation error")
	}

	// Strict YAML: a misspelled key under scan is a load error, not a default.
	if _, err := LoadFromBytes([]byte("version: \"1.0\"\nname: t\nscan:\n  on_malformd: raw_scan\n")); err == nil {
		t.Fatal("a misspelled key under scan must fail to load")
	}
}
