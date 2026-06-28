package scanner

import "testing"

func TestCalculateRisk_Empty(t *testing.T) {
	r := CalculateRisk(nil)
	if r.Score != 0 || r.Percentage != 0 || r.Level != "none" {
		t.Fatalf("got %+v", r)
	}
	if len(r.Breakdown) != 0 {
		t.Fatalf("breakdown: %v", r.Breakdown)
	}
}

func TestCalculateRisk_SingleCritical(t *testing.T) {
	r := CalculateRisk([]Finding{
		{Type: "secret", Category: "aws_access_key", Severity: "critical"},
	})
	if r.Level != "critical" {
		t.Fatalf("level %q", r.Level)
	}
	if r.Score < 0.76 {
		t.Fatalf("score %v want >= 0.76", r.Score)
	}
	if r.Percentage < 76 {
		t.Fatalf("percentage %d", r.Percentage)
	}
}

func TestCalculateRisk_MultipleMediumSameType(t *testing.T) {
	r := CalculateRisk([]Finding{
		{Type: "pii", Category: "email", Severity: "medium"},
		{Type: "pii", Category: "phone_tr", Severity: "medium"},
		{Type: "pii", Category: "tc_kimlik", Severity: "medium"},
	})
	// max medium weight 0.4 + cumulative bump 0.2 => 0.6 → "high" band
	if r.Level != "high" {
		t.Fatalf("level %q score=%v", r.Level, r.Score)
	}
	if r.Score < 0.55 || r.Score > 0.65 {
		t.Fatalf("score %v unexpected", r.Score)
	}
}

func TestCalculateRisk_MixedDominantCritical(t *testing.T) {
	r := CalculateRisk([]Finding{
		{Type: "pii", Category: "email", Severity: "medium"},
		{Type: "secret", Category: "openai_key", Severity: "critical"},
	})
	if r.Level != "critical" {
		t.Fatalf("level %q want critical", r.Level)
	}
	if got := r.Breakdown["pii"]; got == 0 {
		t.Fatal("missing pii breakdown")
	}
	if got := r.Breakdown["secret"]; got == 0 {
		t.Fatal("missing secret breakdown")
	}
}
