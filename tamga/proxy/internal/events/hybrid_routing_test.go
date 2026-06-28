package events

import "testing"

func TestScanTypesForRisk(t *testing.T) {
	cases := []struct {
		risk    float64
		wantPII bool
		wantInj bool
	}{
		{0.0, true, false}, // always pii; injection skipped below 0.45
		{0.20, true, false},
		{0.44, true, false},
		{0.45, true, true}, // injection added at 0.45
		{0.80, true, true},
		{0.95, true, true},
	}
	for _, tc := range cases {
		types := scanTypesForRisk(tc.risk)
		hasPII := contains(types, "pii")
		hasInj := contains(types, "injection")
		if hasPII != tc.wantPII {
			t.Errorf("risk=%.2f: pii=%v want %v", tc.risk, hasPII, tc.wantPII)
		}
		if hasInj != tc.wantInj {
			t.Errorf("risk=%.2f: injection=%v want %v", tc.risk, hasInj, tc.wantInj)
		}
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
