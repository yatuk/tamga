package webhooks

import (
	"encoding/json"
	"time"

	"github.com/yatuk/tamga/internal/siem"
)

// sampleSIEMEvent is the canonical probe used for webhook tests. Real runtime
// firings populate EventInput from the events bus; keeping this shape stable
// helps operators verify QRadar / Sentinel parsing before switching to live.
func sampleSIEMEvent() siem.EventInput {
	return siem.EventInput{
		RequestID:    "req_test_" + time.Now().UTC().Format("150405"),
		Timestamp:    time.Now().UTC(),
		Provider:     "tamga",
		Model:        "probe",
		Action:       "TEST",
		EventType:    "webhook_probe",
		Endpoint:     "/api/v1/webhooks/test",
		UserID:       "tamga-admin",
		InputTokens:  0,
		OutputTokens: 0,
		InputRisk:    50,
		OutputRisk:   0,
		Findings: []siem.FindingLike{
			{Type: "test", Category: "probe", Severity: "medium", Match: "tamga probe"},
		},
	}
}

// WebhookContentType returns the Content-Type to set for a given kind.
// CEF/LEEF go over text/plain; JSON-family stays application/json.
func WebhookContentType(k Kind) string {
	switch k {
	case KindSplunkHEC, KindSentinel, KindQRadar:
		return "text/plain; charset=utf-8"
	}
	return "application/json"
}

// RenderTestPayload produces a per-provider body for the "Test webhook"
// action. SIEM presets (Splunk HEC raw, Azure Sentinel, IBM QRadar) receive
// CEF/LEEF plain-text lines so operators can wire real SIEM pipelines.
func RenderTestPayload(w Webhook) ([]byte, error) {
	if w.PayloadTemplate != "" {
		return []byte(w.PayloadTemplate), nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	switch w.Kind {
	case KindSlack:
		return json.Marshal(map[string]interface{}{
			"text": "[Tamga] Test webhook — " + now,
		})
	case KindTeams:
		// Microsoft retired the classic "Office 365 Connector / Incoming
		// Webhook" channel (Q4 2024). The replacement is Power Automate
		// "Workflows" inside Teams, which expects the Bot Framework
		// message shape wrapping an Adaptive Card. The legacy MessageCard
		// schema is silently ignored by Workflow endpoints.
		return json.Marshal(map[string]interface{}{
			"type": "message",
			"attachments": []map[string]interface{}{
				{
					"contentType": "application/vnd.microsoft.card.adaptive",
					"contentUrl":  nil,
					"content": map[string]interface{}{
						"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
						"type":    "AdaptiveCard",
						"version": "1.4",
						"body": []map[string]interface{}{
							{"type": "TextBlock", "size": "Medium", "weight": "Bolder", "text": "Tamga · Test webhook"},
							{"type": "TextBlock", "text": "Probe fired at " + now, "wrap": true, "isSubtle": true},
						},
					},
				},
			},
		})
	case KindSplunk:
		return json.Marshal(map[string]interface{}{
			"event": map[string]string{
				"source":  "tamga",
				"message": "Test webhook probe",
				"ts":      now,
			},
			"sourcetype": "tamga:test",
		})
	case KindSplunkHEC:
		// Splunk HEC `raw` endpoint accepts bare CEF; customers set
		// sourcetype=cef on the HEC token for the field extractor.
		return []byte(siem.FormatCEF(sampleSIEMEvent())), nil
	case KindSentinel:
		// Azure Sentinel data connector ingests CEF via a Syslog HTTP
		// bridge; same wire format.
		return []byte(siem.FormatCEF(sampleSIEMEvent())), nil
	case KindQRadar:
		// QRadar DSM Editor targets LEEF 2.0.
		return []byte(siem.FormatLEEF(sampleSIEMEvent())), nil
	case KindDatadog:
		return json.Marshal(map[string]interface{}{
			"title":       "Tamga Test Webhook",
			"text":        "Probe fired at " + now,
			"alert_type":  "info",
			"source_type": "tamga",
			"tags":        []string{"service:tamga", "env:test"},
		})
	case KindPagerDuty:
		// PagerDuty Events API v2 — the routing_key (integration key) is
		// required in the body, not the URL. All Tamga probes are
		// "trigger" events; real runtime uses dedup_key to de-duplicate
		// incidents across retry storms.
		routing := w.AuthToken
		if routing == "" {
			routing = "TAMGA_ROUTING_KEY_NOT_SET"
		}
		return json.Marshal(map[string]interface{}{
			"routing_key":  routing,
			"event_action": "trigger",
			"dedup_key":    "tamga-test-" + now,
			"payload": map[string]interface{}{
				"summary":   "Tamga · Test webhook",
				"severity":  "info",
				"source":    "tamga-proxy",
				"timestamp": now,
				"component": "proxy",
				"class":     "probe",
				"custom_details": map[string]string{
					"probe": "webhooks_test",
					"ts":    now,
				},
			},
		})
	case KindOpsgenie:
		// Opsgenie Alert API v2. Authorization header is injected at the
		// transport layer from AuthToken (see store.Test). The `alias`
		// field is Opsgenie's idempotency key — using a fixed value for
		// the probe prevents test storms from creating N duplicate
		// alerts.
		return json.Marshal(map[string]interface{}{
			"message":     "Tamga · Test webhook",
			"description": "Probe fired at " + now,
			"alias":       "tamga-test-probe",
			"priority":    "P5",
			"source":      "tamga-proxy",
			"tags":        []string{"tamga", "probe", "test"},
			"details": map[string]string{
				"ts":     now,
				"action": "TEST",
			},
		})
	case KindServiceNow:
		// ServiceNow Incident table (/api/now/table/incident). Auth is
		// per-instance (Basic or OAuth bearer); operators set it via the
		// Headers map because ServiceNow instances vary. urgency/impact
		// "3" = low so a test probe doesn't wake an on-call.
		return json.Marshal(map[string]interface{}{
			"short_description": "Tamga · Test webhook",
			"description":       "Probe fired at " + now,
			"urgency":           "3",
			"impact":            "3",
			"category":          "security",
			"subcategory":       "ai_proxy",
			"source":            "tamga-proxy",
		})
	case KindJira:
		// Jira Cloud v3 /rest/api/3/issue requires a `project` field and
		// returns 400 without it. `description` must be an Atlassian
		// Document Format (ADF) object in v3, not a plain string.
		projectKey := w.ProjectKey
		if projectKey == "" {
			projectKey = "OPS"
		}
		issueType := w.IssueType
		if issueType == "" {
			issueType = "Task"
		}
		return json.Marshal(map[string]interface{}{
			"fields": map[string]interface{}{
				"project":   map[string]string{"key": projectKey},
				"summary":   "Tamga Test Webhook",
				"issuetype": map[string]string{"name": issueType},
				"description": map[string]interface{}{
					"type":    "doc",
					"version": 1,
					"content": []map[string]interface{}{
						{
							"type": "paragraph",
							"content": []map[string]interface{}{
								{"type": "text", "text": "Probe fired at " + now},
							},
						},
					},
				},
			},
		})
	}
	return json.Marshal(map[string]string{
		"message": "[Tamga] Test webhook",
		"ts":      now,
	})
}
