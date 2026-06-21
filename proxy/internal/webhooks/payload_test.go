package webhooks

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRenderTeamsWorkflow verifies that the Teams payload produces a
// Workflows-compatible Adaptive Card envelope (the classic MessageCard
// format was retired by Microsoft in Q4 2024).
func TestRenderTeamsWorkflow(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindTeams})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if doc["type"] != "message" {
		t.Fatalf("want top-level type=message, got %v", doc["type"])
	}
	atts, ok := doc["attachments"].([]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("want 1 attachment, got %v", doc["attachments"])
	}
	att := atts[0].(map[string]any)
	if att["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Fatalf("want adaptive card contentType, got %v", att["contentType"])
	}
	content := att["content"].(map[string]any)
	if content["type"] != "AdaptiveCard" {
		t.Fatalf("want AdaptiveCard, got %v", content["type"])
	}
}

// TestRenderJiraCloudV3 verifies the Jira payload hits the v3 shape
// (project.key required + ADF description object). Without this, Jira
// Cloud returns 400 Bad Request.
func TestRenderJiraCloudV3(t *testing.T) {
	body, err := RenderTestPayload(Webhook{
		Kind:       KindJira,
		ProjectKey: "SEC",
		IssueType:  "Bug",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	fields := doc["fields"].(map[string]any)
	project := fields["project"].(map[string]any)
	if project["key"] != "SEC" {
		t.Fatalf("want project.key=SEC, got %v", project["key"])
	}
	issueType := fields["issuetype"].(map[string]any)
	if issueType["name"] != "Bug" {
		t.Fatalf("want issuetype.name=Bug, got %v", issueType["name"])
	}
	desc := fields["description"].(map[string]any)
	if desc["type"] != "doc" {
		t.Fatalf("description must be ADF doc, got %v", desc["type"])
	}
	if _, ok := desc["content"].([]any); !ok {
		t.Fatalf("description.content must be array, got %T", desc["content"])
	}
}

// TestRenderJiraDefaults falls back to OPS/Task when unspecified.
func TestRenderJiraDefaults(t *testing.T) {
	body, _ := RenderTestPayload(Webhook{Kind: KindJira})
	if !strings.Contains(string(body), `"key":"OPS"`) {
		t.Fatalf("want default OPS key, got %s", body)
	}
	if !strings.Contains(string(body), `"name":"Task"`) {
		t.Fatalf("want default Task issuetype, got %s", body)
	}
}

// TestRenderSplunkHECRaw checks the HEC raw endpoint receives bare CEF.
func TestRenderSplunkHECRaw(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindSplunkHEC})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(string(body), "CEF:0|") {
		t.Fatalf("want CEF prefix, got %s", body)
	}
}

// TestRenderPagerDuty verifies the Events API v2 shape — routing_key in
// body (not URL), event_action=trigger, dedup_key for idempotency.
func TestRenderPagerDuty(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindPagerDuty, AuthToken: "RKEY_1234"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if doc["routing_key"] != "RKEY_1234" {
		t.Fatalf("want routing_key in body, got %v", doc["routing_key"])
	}
	if doc["event_action"] != "trigger" {
		t.Fatalf("want event_action=trigger, got %v", doc["event_action"])
	}
	if _, ok := doc["dedup_key"].(string); !ok {
		t.Fatalf("dedup_key must be a string, got %T", doc["dedup_key"])
	}
	payload, ok := doc["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload must be object, got %T", doc["payload"])
	}
	if payload["severity"] != "info" {
		t.Fatalf("want severity=info, got %v", payload["severity"])
	}
}

// TestRenderOpsgenie verifies message/alias/priority shape + tags array.
func TestRenderOpsgenie(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindOpsgenie})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := doc["message"].(string); !ok {
		t.Fatalf("opsgenie requires message field")
	}
	if doc["alias"] == "" || doc["alias"] == nil {
		t.Fatalf("opsgenie alias (idempotency key) must be set")
	}
	if doc["priority"] != "P5" {
		t.Fatalf("want P5 probe priority, got %v", doc["priority"])
	}
	if _, ok := doc["tags"].([]any); !ok {
		t.Fatalf("tags must be array, got %T", doc["tags"])
	}
}

// TestRenderServiceNow verifies Incident Table API field names.
func TestRenderServiceNow(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindServiceNow})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := doc["short_description"].(string); !ok {
		t.Fatalf("servicenow requires short_description")
	}
	if doc["urgency"] != "3" || doc["impact"] != "3" {
		t.Fatalf("probe must be low urgency/impact=3, got %v/%v", doc["urgency"], doc["impact"])
	}
}

// TestContentTypeMatrix makes sure JSON providers stay application/json
// and CEF/LEEF providers drop to text/plain.
func TestContentTypeMatrix(t *testing.T) {
	jsonKinds := []Kind{KindSlack, KindTeams, KindSplunk, KindDatadog, KindJira, KindPagerDuty, KindOpsgenie, KindServiceNow, KindGeneric}
	for _, k := range jsonKinds {
		if got := WebhookContentType(k); got != "application/json" {
			t.Fatalf("%s: want application/json, got %s", k, got)
		}
	}
	plainKinds := []Kind{KindSplunkHEC, KindSentinel, KindQRadar}
	for _, k := range plainKinds {
		if got := WebhookContentType(k); !strings.HasPrefix(got, "text/plain") {
			t.Fatalf("%s: want text/plain, got %s", k, got)
		}
	}
}

// TestRenderTestPayload_Slack verifies Slack produces valid JSON with
// a Markdown-formatted `text` field containing the test webhook message.
func TestRenderTestPayload_Slack(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindSlack})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	text, ok := doc["text"].(string)
	if !ok || text == "" {
		t.Fatalf("slack payload must have non-empty text field, got %v", doc["text"])
	}
	if !strings.Contains(text, "Test webhook") {
		t.Errorf("slack text should contain 'Test webhook', got %q", text)
	}
	if !strings.Contains(text, "[Tamga]") {
		t.Errorf("slack text should contain '[Tamga]' prefix, got %q", text)
	}
}

// TestRenderTestPayload_Sentinel verifies Sentinel produces CEF-formatted
// text with the required CEF header prefix and event data from
// sampleSIEMEvent.
func TestRenderTestPayload_Sentinel(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindSentinel})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(body)
	if !strings.HasPrefix(s, "CEF:0|") {
		t.Fatalf("want CEF prefix, got %q", s)
	}
	// Sentinel uses same CEF formatter as SplunkHEC; verify event data present.
	required := []string{
		"deviceExternalId=req_test_",
		"suser=tamga-admin",
		"act=TEST",
		"cn1=50",
	}
	for _, sub := range required {
		if !strings.Contains(s, sub) {
			t.Errorf("sentinel CEF missing %q in %q", sub, s)
		}
	}
}

// TestRenderTestPayload_QRadar verifies QRadar produces LEEF-formatted text
// with the LEEF 2.0 header and expected key-value fields from the sample
// event.
func TestRenderTestPayload_QRadar(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindQRadar})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	s := string(body)
	if !strings.HasPrefix(s, "LEEF:2.0|") {
		t.Fatalf("want LEEF prefix, got %q", s)
	}
	required := []string{
		"requestID=req_test_",
		"usrName=tamga-admin",
		"action=TEST",
		"inputRisk=50",
		"provider=tamga",
		"model=probe",
	}
	for _, sub := range required {
		if !strings.Contains(s, sub) {
			t.Errorf("qradar LEEF missing %q in %q", sub, s)
		}
	}
}

// TestRenderTestPayload_Datadog verifies the Datadog Events API payload
// shape: title, text, alert_type, source_type, and tags array.
func TestRenderTestPayload_Datadog(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindDatadog})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := doc["title"].(string); !ok {
		t.Errorf("datadog payload must have title")
	}
	if _, ok := doc["text"].(string); !ok {
		t.Errorf("datadog payload must have text")
	}
	if doc["alert_type"] != "info" {
		t.Errorf("datadog alert_type want info, got %v", doc["alert_type"])
	}
	if doc["source_type"] != "tamga" {
		t.Errorf("datadog source_type want tamga, got %v", doc["source_type"])
	}
	tags, ok := doc["tags"].([]any)
	if !ok || len(tags) == 0 {
		t.Errorf("datadog tags must be non-empty array, got %v", doc["tags"])
	}
}

// TestRenderTestPayload_GenericJSON verifies the default fallback produces
// valid JSON with message and ts fields.
func TestRenderTestPayload_GenericJSON(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: KindGeneric})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	msg, ok := doc["message"].(string)
	if !ok || msg == "" {
		t.Fatalf("generic payload must have non-empty message field, got %v", doc["message"])
	}
	if _, ok := doc["ts"].(string); !ok {
		t.Fatalf("generic payload must have ts field, got %v", doc["ts"])
	}
}

// TestRenderTestPayload_UnknownKind verifies an unrecognized kind falls back
// to the generic JSON format rather than erroring.
func TestRenderTestPayload_UnknownKind(t *testing.T) {
	body, err := RenderTestPayload(Webhook{Kind: "nonexistent_kind"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("json: %v", err)
	}
	msg, ok := doc["message"].(string)
	if !ok || msg == "" {
		t.Fatalf("unknown kind should fallback to generic JSON with message, got %v", doc)
	}
	if _, ok := doc["ts"].(string); !ok {
		t.Fatalf("unknown kind fallback must have ts, got %v", doc)
	}
}

// TestBuildPayload_WithRealEvent verifies that when SIEM payloads are
// rendered, the sampleSIEMEvent data (request ID prefix, user, action,
// risk) appears correctly in the CEF and LEEF output.
func TestBuildPayload_WithRealEvent(t *testing.T) {
	// SplunkHEC CEF: verify event detail fields from sampleSIEMEvent.
	cefBody, err := RenderTestPayload(Webhook{Kind: KindSplunkHEC})
	if err != nil {
		t.Fatalf("render hec: %v", err)
	}
	cef := string(cefBody)
	cefAssertions := []string{
		"CEF:0|Tamga|Proxy|0.5|",
		"deviceExternalId=req_test_",
		"suser=tamga-admin",
		"act=TEST",
		"cs1=tamga",
		"cs2=probe",
		"requestMethod=webhook_probe",
		"request=/api/v1/webhooks/test",
		"cs4Label=policy_channel",
		"cn1Label=input_risk", "cn1=50",
		"cn2Label=output_risk", "cn2=0",
		"cn3Label=findings_count", "cn3=1",
	}
	for _, sub := range cefAssertions {
		if !strings.Contains(cef, sub) {
			t.Errorf("CEF missing %q in %q", sub, cef)
		}
	}

	// QRadar LEEF: verify same event data in LEEF format.
	leefBody, err := RenderTestPayload(Webhook{Kind: KindQRadar})
	if err != nil {
		t.Fatalf("render leef: %v", err)
	}
	leef := string(leefBody)
	leefAssertions := []string{
		"LEEF:2.0|Tamga|Proxy|0.5|",
		"requestID=req_test_",
		"usrName=tamga-admin",
		"action=TEST",
		"provider=tamga",
		"model=probe",
		"url=/api/v1/webhooks/test",
		"inputRisk=50",
		"outputRisk=0",
		"findingsCount=1",
		"inputTokens=0",
		"outputTokens=0",
	}
	for _, sub := range leefAssertions {
		if !strings.Contains(leef, sub) {
			t.Errorf("LEEF missing %q in %q", sub, leef)
		}
	}
}

// TestPayloadTemplateOverride ensures the raw passthrough short-circuit
// still works (generic "bring your own body" provider).
func TestPayloadTemplateOverride(t *testing.T) {
	body, err := RenderTestPayload(Webhook{
		Kind:            KindGeneric,
		PayloadTemplate: `{"hello":"world"}`,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if string(body) != `{"hello":"world"}` {
		t.Fatalf("template passthrough failed: %s", body)
	}
}
