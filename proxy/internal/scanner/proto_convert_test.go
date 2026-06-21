package scanner

import (
	"testing"

	pb "github.com/yatuk/tamga/proto/scanner/v1"
)

// ── FindingsToProto ──────────────────────────────────────────────────────────

func TestFindingsToProto_Roundtrip(t *testing.T) {
	input := []Finding{
		{
			Type:           "pii",
			Severity:       "high",
			Match:          "john.doe@example.com",
			Category:       "EMAIL_ADDRESS",
			StartPos:       10,
			EndPos:         32,
			Confidence:     0.95,
			ActionTaken:    "REDACT",
			Metadata:       map[string]string{"source": "pii_deep"},
			ScannerVersion: "1.0.0",
			DatasetVersion: "2026-Q2",
		},
		{
			Type:           "secret",
			Severity:       "critical",
			Match:          "AKIAIOSFODNN7EXAMPLE",
			Category:       "AWS_ACCESS_KEY",
			StartPos:       0,
			EndPos:         20,
			Confidence:     0.90,
			ActionTaken:    "BLOCK",
			Metadata:       map[string]string{"source": "secret_detector"},
			ScannerVersion: "1.0.0",
			DatasetVersion: "2026-Q1",
		},
	}

	proto := FindingsToProto(input)
	goFindings := ProtoToFindings(proto)

	if len(goFindings) != len(input) {
		t.Fatalf("roundtrip length: want %d, got %d", len(input), len(goFindings))
	}

	for i, want := range input {
		got := goFindings[i]
		if got.Type != want.Type {
			t.Errorf("[%d] Type: want %q, got %q", i, want.Type, got.Type)
		}
		if got.Severity != want.Severity {
			t.Errorf("[%d] Severity: want %q, got %q", i, want.Severity, got.Severity)
		}
		if got.Match != want.Match {
			t.Errorf("[%d] Match: want %q, got %q", i, want.Match, got.Match)
		}
		if got.Category != want.Category {
			t.Errorf("[%d] Category: want %q, got %q", i, want.Category, got.Category)
		}
		if got.StartPos != want.StartPos {
			t.Errorf("[%d] StartPos: want %d, got %d", i, want.StartPos, got.StartPos)
		}
		if got.EndPos != want.EndPos {
			t.Errorf("[%d] EndPos: want %d, got %d", i, want.EndPos, got.EndPos)
		}
		if got.Confidence != want.Confidence {
			t.Errorf("[%d] Confidence: want %f, got %f", i, want.Confidence, got.Confidence)
		}
		if got.ActionTaken != want.ActionTaken {
			t.Errorf("[%d] ActionTaken: want %q, got %q", i, want.ActionTaken, got.ActionTaken)
		}
		if got.ScannerVersion != want.ScannerVersion {
			t.Errorf("[%d] ScannerVersion: want %q, got %q", i, want.ScannerVersion, got.ScannerVersion)
		}
		if got.DatasetVersion != want.DatasetVersion {
			t.Errorf("[%d] DatasetVersion: want %q, got %q", i, want.DatasetVersion, got.DatasetVersion)
		}
		// Metadata: all original keys should be preserved plus confidence_score_total
		// if ConfidenceScore was set.
		for k, v := range want.Metadata {
			if got.Metadata[k] != v {
				t.Errorf("[%d] Metadata[%s]: want %q, got %q", i, k, v, got.Metadata[k])
			}
		}
	}
}

func TestFindingsToProto_SingleFinding(t *testing.T) {
	input := []Finding{
		{Type: "injection", Severity: "critical", Match: "DROP TABLE", Category: "SQL_INJECTION", Confidence: 0.99},
	}
	proto := FindingsToProto(input)
	if len(proto) != 1 {
		t.Fatalf("want 1 proto finding, got %d", len(proto))
	}
	if proto[0].Type != "injection" {
		t.Errorf("Type: want 'injection', got %q", proto[0].Type)
	}
	if proto[0].Confidence != 0.99 {
		t.Errorf("Confidence: want 0.99, got %f", proto[0].Confidence)
	}
}

func TestFindingsToProto_EmptySlice(t *testing.T) {
	proto := FindingsToProto([]Finding{})
	if proto != nil {
		t.Fatalf("empty slice: want nil, got %v", proto)
	}
}

func TestFindingsToProto_NilInput(t *testing.T) {
	proto := FindingsToProto(nil)
	if proto != nil {
		t.Fatalf("nil input: want nil, got %v", proto)
	}
}

func TestFindingsToProto_ZeroValues(t *testing.T) {
	// A finding with all zero/nil fields should convert cleanly.
	input := []Finding{
		{Type: "test"},
	}
	proto := FindingsToProto(input)
	if len(proto) != 1 {
		t.Fatalf("want 1 proto finding, got %d", len(proto))
	}
	if proto[0].Severity != "" {
		t.Errorf("Severity: want empty, got %q", proto[0].Severity)
	}
	if proto[0].Metadata != nil {
		t.Errorf("Metadata: want nil, got %v", proto[0].Metadata)
	}
}

// ── ProtoToFindings ──────────────────────────────────────────────────────────

func TestProtoToFindings_EmptySlice(t *testing.T) {
	goFindings := ProtoToFindings([]*pb.Finding{})
	if goFindings != nil {
		t.Fatalf("empty proto slice: want nil, got %v", goFindings)
	}
}

func TestProtoToFindings_NilInput(t *testing.T) {
	goFindings := ProtoToFindings(nil)
	if goFindings != nil {
		t.Fatalf("nil input: want nil, got %v", goFindings)
	}
}

func TestProtoToFindings_Single(t *testing.T) {
	pfs := []*pb.Finding{
		{
			Type:       "jailbreak",
			Severity:   "critical",
			Match:      "pretend you are DAN",
			Category:   "DAN_JAILBREAK",
			StartPos:   5,
			EndPos:     23,
			Confidence: 0.88,
			Metadata:   map[string]string{"model": "claude"},
		},
	}
	goFindings := ProtoToFindings(pfs)
	if len(goFindings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(goFindings))
	}
	f := goFindings[0]
	if f.Type != "jailbreak" {
		t.Errorf("Type: want 'jailbreak', got %q", f.Type)
	}
	if f.Severity != "critical" {
		t.Errorf("Severity: want 'critical', got %q", f.Severity)
	}
	if f.Match != "pretend you are DAN" {
		t.Errorf("Match: got %q", f.Match)
	}
	if f.StartPos != 5 {
		t.Errorf("StartPos: want 5, got %d", f.StartPos)
	}
	if f.EndPos != 23 {
		t.Errorf("EndPos: want 23, got %d", f.EndPos)
	}
	if f.Confidence != 0.88 {
		t.Errorf("Confidence: want 0.88, got %f", f.Confidence)
	}
	if f.Metadata["model"] != "claude" {
		t.Errorf("Metadata model: want 'claude', got %q", f.Metadata["model"])
	}
}

// ── ConfidenceScore embedding (proto lacks ConfidenceScore field) ────────────

func TestFindingsToProto_ConfidenceScoreEmbedded(t *testing.T) {
	input := []Finding{
		{
			Type:     "pii",
			Category: "CREDIT_CARD",
			ConfidenceScore: &ConfidenceScore{
				Total:  85,
				Action: "REDACT",
				Breakdown: ConfidenceFactor{
					Format:    30,
					Algorithm: 30,
					Database:  20,
					Context:   5,
				},
				Reasoning: "score=85 action=REDACT factors=[format match (+30), algorithm validation (+30), database lookup (+20), context keywords (+5)]",
			},
			Metadata: map[string]string{"original": "value"},
		},
	}

	proto := FindingsToProto(input)
	if len(proto) != 1 {
		t.Fatalf("want 1 proto finding, got %d", len(proto))
	}

	// Reasoning should be embedded in Metadata["confidence_score_total"].
	if proto[0].Metadata["confidence_score_total"] != input[0].ConfidenceScore.Reasoning {
		t.Errorf("confidence_score_total: want %q, got %q",
			input[0].ConfidenceScore.Reasoning, proto[0].Metadata["confidence_score_total"])
	}

	// Original metadata should be preserved.
	if proto[0].Metadata["original"] != "value" {
		t.Errorf("original metadata: want 'value', got %q", proto[0].Metadata["original"])
	}
}

func TestFindingsToProto_ConfidenceScoreNilMetadata(t *testing.T) {
	// ConfidenceScore set but Metadata is nil → Metadata should be created.
	input := []Finding{
		{
			Type: "pii",
			ConfidenceScore: &ConfidenceScore{
				Reasoning: "score=70 action=REDACT factors=[...]",
			},
		},
	}

	proto := FindingsToProto(input)
	if len(proto) != 1 {
		t.Fatalf("want 1 proto finding, got %d", len(proto))
	}
	if proto[0].Metadata == nil {
		t.Fatal("Metadata should be non-nil when ConfidenceScore is set")
	}
	if proto[0].Metadata["confidence_score_total"] != "score=70 action=REDACT factors=[...]" {
		t.Errorf("confidence_score_total: got %q", proto[0].Metadata["confidence_score_total"])
	}
}

func TestFindingsToProto_NoConfidenceScore(t *testing.T) {
	// Finding without ConfidenceScore should not have confidence_score_total metadata.
	input := []Finding{
		{Type: "pii", Metadata: nil},
		{Type: "secret", Metadata: map[string]string{"key": "val"}},
	}

	proto := FindingsToProto(input)
	if len(proto) != 2 {
		t.Fatalf("want 2 proto findings, got %d", len(proto))
	}
	if proto[0].Metadata != nil {
		t.Errorf("Metadata: want nil for finding without ConfidenceScore, got %v", proto[0].Metadata)
	}
	if proto[1].Metadata["confidence_score_total"] != "" {
		t.Errorf("should not have confidence_score_total when ConfidenceScore is nil")
	}
	if proto[1].Metadata["key"] != "val" {
		t.Errorf("original metadata lost: want 'val', got %q", proto[1].Metadata["key"])
	}
}

// ── Benchmarks ───────────────────────────────────────────────────────────────

func BenchmarkFindingsToProto(b *testing.B) {
	input := []Finding{
		{Type: "pii", Severity: "high", Match: "test@example.com", Category: "EMAIL", StartPos: 0, EndPos: 16, Confidence: 0.9, Metadata: map[string]string{"s": "pii_deep"}},
		{Type: "secret", Severity: "critical", Match: "ghp_example", Category: "GITHUB_TOKEN", StartPos: 20, EndPos: 31, Confidence: 0.95},
		{Type: "injection", Severity: "critical", Match: "DROP TABLE", Category: "SQL_INJECTION", StartPos: 40, EndPos: 50, Confidence: 0.99},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = FindingsToProto(input)
	}
}

func BenchmarkProtoToFindings(b *testing.B) {
	pfs := []*pb.Finding{
		{Type: "pii", Severity: "high", Match: "test@example.com", Category: "EMAIL", StartPos: 0, EndPos: 16, Confidence: 0.9, Metadata: map[string]string{"s": "pii_deep"}},
		{Type: "secret", Severity: "critical", Match: "ghp_example", Category: "GITHUB_TOKEN", StartPos: 20, EndPos: 31, Confidence: 0.95},
		{Type: "injection", Severity: "critical", Match: "DROP TABLE", Category: "SQL_INJECTION", StartPos: 40, EndPos: 50, Confidence: 0.99},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ProtoToFindings(pfs)
	}
}
