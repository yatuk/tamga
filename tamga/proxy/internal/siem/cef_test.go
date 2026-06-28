package siem

import (
	"strings"
	"testing"
	"time"
)

func sampleEvent() EventInput {
	return EventInput{
		RequestID:    "req_12345",
		Timestamp:    time.Date(2026, 4, 17, 12, 34, 56, 0, time.UTC),
		Provider:     "openai",
		Model:        "gpt-4o",
		Action:       "BLOCK",
		EventType:    "request_blocked",
		Endpoint:     "/v1/chat/completions",
		UserID:       "alice@example.com",
		InputTokens:  1024,
		OutputTokens: 0,
		CostUSD:      0.0032,
		InputRisk:    92,
		OutputRisk:   0,
		Findings: []FindingLike{
			{Type: "pii", Category: "tckn", Severity: "critical", Match: "12345678902", Confidence: 0.97},
			{Type: "secret", Category: "aws", Severity: "high", Match: "AKIA...", Confidence: 0.91},
		},
	}
}

func TestFormatCEFHeader(t *testing.T) {
	line := FormatCEF(sampleEvent())
	if !strings.HasPrefix(line, "CEF:0|Tamga|Proxy|0.5|") {
		t.Fatalf("bad header: %q", line)
	}
	for _, substr := range []string{
		"tamga.block.tckn", // block action + highest-severity category
		"deviceExternalId=req_12345",
		"cs1=openai",
		"cs2=gpt-4o",
		"cn1=92",
		"cn3=2",
		"|10|", // severity 10 (critical)
	} {
		if !strings.Contains(line, substr) {
			t.Errorf("CEF missing %q in %q", substr, line)
		}
	}
}

func TestFormatLEEFHeader(t *testing.T) {
	line := FormatLEEF(sampleEvent())
	if !strings.HasPrefix(line, "LEEF:2.0|Tamga|Proxy|0.5|tamga.block.tckn|^|") {
		t.Fatalf("bad LEEF header: %q", line)
	}
	for _, substr := range []string{
		"requestID=req_12345",
		"provider=openai",
		"action=BLOCK",
		"sev=10",
		"findingsCount=2",
	} {
		if !strings.Contains(line, substr) {
			t.Errorf("LEEF missing %q in %q", substr, line)
		}
	}
}

func TestCEFEscape(t *testing.T) {
	e := EventInput{
		RequestID: "req|with|pipes",
		Action:    "BLOCK",
		Endpoint:  "/v1/chat?foo=bar",
		Findings:  []FindingLike{{Type: "pii", Category: "tckn", Severity: "high"}},
	}
	line := FormatCEF(e)
	// Extension value with literal `=` must be escaped (\=).
	if !strings.Contains(line, `request=/v1/chat?foo\=bar`) {
		t.Errorf("expected escaped `=` in extension, got: %q", line)
	}
}

func TestEmptyFindings(t *testing.T) {
	e := sampleEvent()
	e.Findings = nil
	line := FormatCEF(e)
	if !strings.Contains(line, "tamga.block") {
		t.Errorf("expected fallback signature, got: %q", line)
	}
}
