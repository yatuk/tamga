package webhooks

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/scanner"
)

// The text a real request would carry: a prompt, a secret in it, and what
// the scanners keep of each match.
const (
	liveSecret = "AKIAIOSFODNN7EXAMPLE"
	livePrompt = "Kindly forward the zebrafish quartz memo using " + liveSecret
)

var allKinds = []Kind{
	KindSlack, KindTeams, KindSplunk, KindSplunkHEC, KindSentinel, KindQRadar,
	KindDatadog, KindJira, KindPagerDuty, KindOpsgenie, KindServiceNow, KindGeneric,
}

// liveEvent is a blocked request as the proxy publishes it: the body is the
// request, and each finding carries its match.
func liveEvent() events.Event {
	return events.Event{
		RequestID: "0198c0de-aaaa-7bbb-8ccc-1234567890ab",
		OrgID:     "acme",
		Provider:  "openai",
		Model:     "gpt-4o",
		EventType: "request_blocked",
		Action:    "BLOCK",
		Endpoint:  "/v1/chat/completions/zebrafish",
		UserID:    "alice@example.com",
		Timestamp: time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC),
		Body:      []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"` + livePrompt + `"}]}`),
		InputRisk: scanner.RiskScore{Percentage: 91, Level: "critical"},
		Findings: []scanner.Finding{
			{Type: "secret", Category: "aws_access_key", Severity: "critical", Match: "AKIAIOSF************"},
			{Type: "injection", Category: "instruction_override", Severity: "high", Match: "forward the zebrafish quartz memo"},
			{Type: "secret", Category: "aws_access_key", Severity: "critical", Match: "AKIAIOSF************"},
		},
	}
}

func liveAlert(t *testing.T) Alert {
	t.Helper()
	a, ok := AlertFromEvent(liveEvent(), "default")
	if !ok {
		t.Fatal("a blocked request is an alert")
	}
	return a
}

// leak reports the first piece of request text found in a payload: any five
// consecutive characters of the secret, a word of the prompt, or the path.
func leak(payload string) string {
	low := strings.ToLower(payload)
	for i := 0; i+5 <= len(liveSecret); i++ {
		if strings.Contains(payload, liveSecret[i:i+5]) {
			return liveSecret[i : i+5]
		}
	}
	for _, word := range []string{"kindly", "zebrafish", "quartz", "memo", "********"} {
		if strings.Contains(low, word) {
			return word
		}
	}
	return ""
}

func TestAlertFromEvent(t *testing.T) {
	a := liveAlert(t)
	if a.RequestID != "0198c0de-aaaa-7bbb-8ccc-1234567890ab" || a.OrgID != "acme" || a.Provider != "openai" ||
		a.Model != "gpt-4o" || a.Action != "BLOCK" || a.Direction != "input" || a.UserID != "alice@example.com" {
		t.Fatalf("alert lost a field: %+v", a)
	}
	if a.Endpoint != "" {
		t.Fatalf("a live alert must not carry the request path, got %q", a.Endpoint)
	}
	if len(a.Findings) != 3 || a.Findings[0] != (AlertFinding{Type: "secret", Category: "aws_access_key", Severity: "critical"}) {
		t.Fatalf("findings = %+v", a.Findings)
	}
	if a.InputRisk != 91 {
		t.Fatalf("input risk = %d, want 91", a.InputRisk)
	}

	// An event with no organisation takes the default.
	e := liveEvent()
	e.OrgID = ""
	if a, _ := AlertFromEvent(e, "default"); a.OrgID != "default" {
		t.Fatalf("org = %q, want default", a.OrgID)
	}

	// A decision on the response reads the output fields.
	out, ok := AlertFromEvent(events.Event{
		RequestID:      "r1",
		EventType:      "output_scanned",
		OutputAction:   "BLOCK",
		OutputFindings: []scanner.Finding{{Type: "pii", Category: "email", Severity: "medium", Match: "j***@example.com"}},
	}, "default")
	if !ok || out.Direction != "output" || out.Action != "BLOCK" || len(out.Findings) != 1 {
		t.Fatalf("output alert = %+v ok=%v", out, ok)
	}

	// A request's own event that judged an answer alone (the LiteLLM route)
	// is a decision on the output, with the event's action and findings.
	answer, ok := AlertFromEvent(events.Event{
		RequestID: "r2",
		EventType: "request_blocked",
		Provider:  "litellm",
		Direction: "output",
		Action:    "BLOCK",
		Findings:  []scanner.Finding{{Type: "pii", Category: "tc_kimlik", Severity: "critical", Match: "10*******46"}},
	}, "default")
	if !ok || answer.Direction != "output" || answer.Action != "BLOCK" || answer.Provider != "litellm" ||
		len(answer.Findings) != 1 || answer.Findings[0].Category != "tc_kimlik" {
		t.Fatalf("bridge answer alert = %+v ok=%v", answer, ok)
	}

	for _, e := range []events.Event{
		{EventType: "request_scanned", Action: "PASS"},
		{EventType: "request_scanned", Action: "LOG"},
		{EventType: "output_scan_hint"},
		{EventType: "output_scanned", OutputAction: "PASS"},
		{EventType: "operator_state_deep_scan", Action: "BLOCK"},
	} {
		if _, ok := AlertFromEvent(e, "default"); ok {
			t.Errorf("%s/%s%s must not be an alert", e.EventType, e.Action, e.OutputAction)
		}
	}
}

// Every kind, fed a real event: the request id is there, and nothing of the
// request's text is.
func TestRender_LiveEvent_EveryKind(t *testing.T) {
	a := liveAlert(t)
	// The check has teeth: the event itself would fail it.
	ev := liveEvent()
	if leak(string(ev.Body)) == "" || leak(ev.Findings[0].Match) == "" || leak(ev.Findings[1].Match) == "" || leak(ev.Endpoint) == "" {
		t.Fatal("leak() does not recognise the request's own text")
	}
	for _, k := range append(allKinds, Kind("something_new")) {
		body, err := Render(Webhook{Kind: k, AuthToken: "tok"}, a)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		s := string(body)
		if !strings.Contains(s, a.RequestID) {
			t.Errorf("%s: no request id in %s", k, s)
		}
		if !strings.Contains(s, "BLOCK") {
			t.Errorf("%s: no action in %s", k, s)
		}
		for _, want := range []string{"acme", "openai", "gpt-4o", "aws_access_key"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: %q missing from %s", k, want, s)
			}
		}
		if got := leak(s); got != "" {
			t.Errorf("%s: request text %q in %s", k, got, s)
		}
		if WebhookContentType(k) == "application/json" && !json.Valid(body) {
			t.Errorf("%s: body is not JSON: %s", k, s)
		}
	}
	body, err := RenderRuleNotify(a)
	if err != nil {
		t.Fatal(err)
	}
	if got := leak(string(body)); got != "" || !strings.Contains(string(body), a.RequestID) {
		t.Errorf("rule notify: leak %q in %s", got, body)
	}
}

func TestRender_Generic_Live(t *testing.T) {
	a := liveAlert(t)
	a.CorrelatedCount = 4
	body, _ := Render(Webhook{Kind: KindGeneric}, a)
	var doc struct {
		RequestID     string         `json:"request_id"`
		TS            string         `json:"ts"`
		OrgID         string         `json:"org_id"`
		Provider      string         `json:"provider"`
		Model         string         `json:"model"`
		Action        string         `json:"action"`
		Direction     string         `json:"direction"`
		UserID        string         `json:"user_id"`
		Severity      string         `json:"severity"`
		FindingsCount int            `json:"findings_count"`
		Findings      []AlertFinding `json:"findings"`
		Categories    map[string]int `json:"categories"`
		Correlated    int            `json:"correlated_count"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.RequestID != a.RequestID || doc.TS != "2026-10-09T08:30:00Z" || doc.OrgID != "acme" ||
		doc.Provider != "openai" || doc.Model != "gpt-4o" || doc.Action != "BLOCK" || doc.Direction != "input" ||
		doc.UserID != "alice@example.com" || doc.Severity != "critical" {
		t.Fatalf("generic document: %s", body)
	}
	if doc.FindingsCount != 3 || len(doc.Findings) != 3 || doc.Categories["aws_access_key"] != 2 || doc.Categories["instruction_override"] != 1 {
		t.Fatalf("findings: %s", body)
	}
	if doc.Correlated != 4 {
		t.Fatalf("correlated_count = %d, want 4", doc.Correlated)
	}
	if strings.Contains(string(body), `"match"`) {
		t.Fatalf("a finding's match has no place in an alert: %s", body)
	}
}

func TestRender_Slack_Live(t *testing.T) {
	body, _ := Render(Webhook{Kind: KindSlack}, liveAlert(t))
	var doc map[string]string
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	text := doc["text"]
	if !strings.HasPrefix(text, "[Tamga] BLOCK on input: secret/aws_access_key (critical) and 2 more\n") {
		t.Fatalf("slack text = %q", text)
	}
	for _, line := range []string{
		"Request: 0198c0de-aaaa-7bbb-8ccc-1234567890ab",
		"Time: 2026-10-09T08:30:00Z",
		"Organisation: acme",
		"Model: gpt-4o",
		"User: alice@example.com",
		"Findings: 3",
		"Kinds: secret/aws_access_key (critical) x2, injection/instruction_override (high) x1",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("slack text lacks %q:\n%s", line, text)
		}
	}
}

func TestRender_CEFAndLEEF_Live(t *testing.T) {
	a := liveAlert(t)
	for _, k := range []Kind{KindSplunkHEC, KindSentinel} {
		body, _ := Render(Webhook{Kind: k}, a)
		s := string(body)
		if !strings.HasPrefix(s, "CEF:0|Tamga|Proxy|0.5|tamga.block.aws_access_key|secret aws_access_key|10|") {
			t.Fatalf("%s header: %s", k, s)
		}
		for _, want := range []string{
			"deviceExternalId=" + a.RequestID,
			"act=BLOCK",
			"suser=alice@example.com",
			"cs1=openai", "cs2=gpt-4o",
			"cn1=91", "cn3=3",
			"cs5Label=org cs5=acme",
			"cs6Label=findings cs6=secret/aws_access_key:critical,injection/instruction_override:high,secret/aws_access_key:critical",
			"flexString2=input",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("%s lacks %q in %s", k, want, s)
			}
		}
		// The path is the caller's text; a live line has no request= at all.
		if strings.Contains(s, "request=") {
			t.Errorf("%s carries the request path: %s", k, s)
		}
	}
	body, _ := Render(Webhook{Kind: KindQRadar}, a)
	s := string(body)
	if !strings.HasPrefix(s, "LEEF:2.0|Tamga|Proxy|0.5|tamga.block.aws_access_key|^|") {
		t.Fatalf("LEEF header: %s", s)
	}
	for _, want := range []string{"requestID=" + a.RequestID, "action=BLOCK", "orgID=acme", "sev=10", "findingsCount=3", "direction=input"} {
		if !strings.Contains(s, want) {
			t.Errorf("LEEF lacks %q in %s", want, s)
		}
	}
	if strings.Contains(s, "url=") {
		t.Errorf("LEEF carries the request path: %s", s)
	}
}

func TestRender_PagerDuty_Live(t *testing.T) {
	a := liveAlert(t)
	a.DedupKey = "tamga-acme|input|BLOCK|aws_access_key"
	body, _ := Render(Webhook{Kind: KindPagerDuty, AuthToken: "RKEY_1234"}, a)
	var doc struct {
		RoutingKey  string `json:"routing_key"`
		EventAction string `json:"event_action"`
		DedupKey    string `json:"dedup_key"`
		Payload     struct {
			Summary  string            `json:"summary"`
			Severity string            `json:"severity"`
			Source   string            `json:"source"`
			Group    string            `json:"group"`
			Class    string            `json:"class"`
			Details  map[string]string `json:"custom_details"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.RoutingKey != "RKEY_1234" || doc.EventAction != "trigger" || doc.DedupKey != "tamga-acme_input_BLOCK_aws_access_key" {
		t.Fatalf("envelope: %s", body)
	}
	p := doc.Payload
	if p.Severity != "critical" || p.Source != "tamga-proxy" || p.Group != "acme" || p.Class != "aws_access_key" ||
		p.Details["request"] != a.RequestID || p.Details["model"] != "gpt-4o" {
		t.Fatalf("payload: %s", body)
	}
}

func TestRender_SeverityMapping_Live(t *testing.T) {
	for _, tc := range []struct {
		severity                       string
		pagerduty, opsgenie, serviceNo string
	}{
		{"critical", "critical", "P1", "1"},
		{"high", "error", "P2", "1"},
		{"medium", "warning", "P3", "2"},
		{"low", "info", "P4", "3"},
	} {
		a := Alert{RequestID: "r", Action: "WARN", Findings: []AlertFinding{{Type: "pii", Category: "email", Severity: tc.severity}}}
		body, _ := Render(Webhook{Kind: KindPagerDuty}, a)
		if !strings.Contains(string(body), `"severity":"`+tc.pagerduty+`"`) {
			t.Errorf("%s: pagerduty %s", tc.severity, body)
		}
		body, _ = Render(Webhook{Kind: KindOpsgenie}, a)
		if !strings.Contains(string(body), `"priority":"`+tc.opsgenie+`"`) || !strings.Contains(string(body), `"alias":"tamga-r"`) {
			t.Errorf("%s: opsgenie %s", tc.severity, body)
		}
		body, _ = Render(Webhook{Kind: KindServiceNow}, a)
		if !strings.Contains(string(body), `"urgency":"`+tc.serviceNo+`"`) {
			t.Errorf("%s: servicenow %s", tc.severity, body)
		}
	}
	block, _ := Render(Webhook{Kind: KindDatadog}, Alert{RequestID: "r", Action: "BLOCK"})
	warn, _ := Render(Webhook{Kind: KindDatadog}, Alert{RequestID: "r", Action: "WARN"})
	if !strings.Contains(string(block), `"alert_type":"error"`) || !strings.Contains(string(warn), `"alert_type":"warning"`) {
		t.Errorf("datadog alert_type: %s / %s", block, warn)
	}
}

// The model name and the user and organisation headers are the caller's.
// They reach the message as a short label and nothing else.
func TestRender_CallerChosenFieldsAreCleaned(t *testing.T) {
	a := Alert{
		RequestID: "r1",
		Action:    "BLOCK",
		OrgID:     "acme|x=1\nCEF:0|evil",
		Model:     "gpt-4o <!channel> " + strings.Repeat("A", 400),
		UserID:    "bob^sev=0\tx",
		Findings:  []AlertFinding{{Type: "secret", Category: "aws_access_key", Severity: "high"}},
	}
	for _, k := range allKinds {
		body, _ := Render(Webhook{Kind: k}, a)
		s := string(body)
		for _, bad := range []string{"<!channel>", "\nCEF", "\\nCEF", "acme|", "bob^", strings.Repeat("A", maxFieldLen+1)} {
			if strings.Contains(s, bad) {
				t.Errorf("%s: %q survived in %s", k, bad, s)
			}
		}
	}
}

func TestRender_TemplateIsSentAsIs(t *testing.T) {
	body, err := Render(Webhook{Kind: KindSlack, PayloadTemplate: `{"text":"fixed"}`}, Alert{RequestID: "r1", Action: "BLOCK"})
	if err != nil || string(body) != `{"text":"fixed"}` {
		t.Fatalf("template: %s %v", body, err)
	}
}

func TestRender_ManyFindingsAreCounted(t *testing.T) {
	a := Alert{RequestID: "r1", Action: "BLOCK"}
	for i := 0; i < 500; i++ {
		a.Findings = append(a.Findings, AlertFinding{Type: "pii", Category: "email", Severity: "medium"})
	}
	body, _ := Render(Webhook{Kind: KindGeneric}, a)
	var doc struct {
		FindingsCount int            `json:"findings_count"`
		Findings      []AlertFinding `json:"findings"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.FindingsCount != 500 || len(doc.Findings) != maxListedFindings {
		t.Fatalf("count %d, listed %d", doc.FindingsCount, len(doc.Findings))
	}
	cef, _ := Render(Webhook{Kind: KindSplunkHEC}, a)
	if len(cef) > 4096 || !strings.Contains(string(cef), "cn3=500") {
		t.Fatalf("CEF line is %d bytes: %s", len(cef), cef)
	}
}
