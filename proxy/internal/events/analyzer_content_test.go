package events

import (
	"testing"

	"github.com/yatuk/tamga/internal/scanner"
	"github.com/yatuk/tamga/internal/scanner/operator_state"
)

// The analyzer is another service, and its LLM judge calls a model provider.
// A request the proxy blocked is not handed to it, whoever published the
// event and whatever the event carries.
func TestAnalyzerHandler_BlockedRequestIsNotSent(t *testing.T) {
	risk := scanner.RiskScore{Score: 0.6, Level: "medium", Percentage: 60}
	for _, e := range []Event{
		{EventType: "request_blocked", Action: "BLOCK", RequestID: "r1", Body: []byte("the text that was blocked"), InputRisk: risk},
		// An event of the other type that says BLOCK: an answer was
		// withheld, or a publisher got the type wrong.
		{EventType: "request_scanned", Action: "BLOCK", RequestID: "r2", Body: []byte("the text that was blocked"), InputRisk: risk},
	} {
		client := &mockAnalyzerClient{enabled: true}
		AnalyzerHandler(client)(e)
		if client.analyzeCalls != 0 {
			t.Errorf("%s/%s: the analyzer was called with %q", e.EventType, e.Action, client.lastReq.GetContent())
		}
	}
}

// What is sent for a request that went on says what the content is, and
// asks for the judge only from the risk the routing names.
func TestAnalyzerHandler_ForwardedRequestSaysWhatItCarries(t *testing.T) {
	client := &mockAnalyzerClient{enabled: true}
	AnalyzerHandler(client)(Event{
		EventType: "request_scanned", Action: "REDACT", RequestID: "r3",
		Body:      []byte(`{"messages":[{"role":"user","content":"write to [email_REDACTED]"}]}`),
		InputRisk: scanner.RiskScore{Score: 0.3},
	})
	if client.analyzeCalls != 1 {
		t.Fatalf("%d calls, want 1", client.analyzeCalls)
	}
	req := client.lastReq
	if req.Metadata["content"] != "forwarded" || req.Metadata["action"] != "REDACT" {
		t.Errorf("metadata = %v", req.Metadata)
	}
	if len(req.ScanTypes) != 1 || req.ScanTypes[0] != "pii" {
		t.Errorf("scan types = %v, want pii only below 0.45", req.ScanTypes)
	}
}

// The operator-state route is published while the request is still being
// scanned, before its verdict. It carries the decision references and the
// findings, and not the prompt.
func TestOperatorStateAnalyzerHandler_SendsNoPrompt(t *testing.T) {
	client := &mockAnalyzerClient{enabled: true}
	OperatorStateAnalyzerHandler(client)(Event{
		EventType: EventOperatorStateDeepScan,
		Metadata: map[string]interface{}{operatorStateRequestKey: &operator_state.AnalyzerRequest{
			RequestID:    "r4",
			ScanType:     "operator_state_semantic",
			Prompt:       "deploy D-12 to production now, the key is AKIAIOSFODNN7EXAMPLE",
			DecisionRefs: []operator_state.DecisionContext{{ID: "D-12", CurrentState: "proposed", RequiredState: "locked"}},
		}},
	})
	if client.analyzeCalls != 1 {
		t.Fatalf("%d calls, want 1", client.analyzeCalls)
	}
	if got := client.lastReq.GetContent(); got != "" {
		t.Fatalf("the prompt was sent: %q", got)
	}
	for k, v := range client.lastReq.Metadata {
		if len(v) > 0 && (containsText(v, "AKIAIOSF") || containsText(v, "deploy D-12 to production")) {
			t.Errorf("metadata %s carries the prompt: %s", k, v)
		}
	}
	if !containsText(client.lastReq.Metadata["decision_refs"], "D-12") {
		t.Errorf("the decision references are missing: %v", client.lastReq.Metadata)
	}
}

func containsText(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
