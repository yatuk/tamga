package webhooks

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/yatuk/tamga/internal/siem"
)

// Alert is what a destination is told about one decision. It has no field
// for the request or response text and none for what a scanner matched, so
// no renderer can send either: a masked match still keeps the ends of a
// value, and some scanners do not mask at all.
type Alert struct {
	RequestID string
	Timestamp time.Time
	OrgID     string
	Provider  string
	Model     string
	// Action is BLOCK, STRIP, REDACT or WARN; the probe uses TEST.
	Action string
	// Direction is "input" for a decision on the request and "output" for
	// one on the response.
	Direction string
	EventType string
	// Endpoint is set by the probe only. A request path is the caller's
	// text, so a live alert leaves it empty.
	Endpoint string
	UserID   string
	// InputRisk and OutputRisk are 0-100.
	InputRisk  int
	OutputRisk int
	Findings   []AlertFinding
	// CorrelatedCount is how many events the correlation window held when
	// this alert fired; 0 or 1 means it stands for itself.
	CorrelatedCount int
	// DedupKey groups repeats of the same alert in tools that keep one open
	// incident per key (PagerDuty dedup_key, Opsgenie alias).
	DedupKey string
}

// AlertFinding is a finding without its match.
type AlertFinding struct {
	Type     string `json:"type"`
	Category string `json:"category"`
	Severity string `json:"severity"`
}

const actionTest = "TEST"

// maxFieldLen bounds a value that came from the request (model, user and
// organisation headers).
const maxFieldLen = 96

// maxListedFindings bounds the per-finding list in one message; the count
// beside it is always the full number.
const maxListedFindings = 50

// cleanField keeps letters, digits and a few separators and cuts the value
// short. The model name and the user and organisation headers are chosen by
// the caller; cleaned, they cannot break a CEF or LEEF line, mention a Slack
// channel, or carry more than a label's worth of text.
func cleanField(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxFieldLen {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._:/@+- ", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		n++
	}
	return b.String()
}

// clean returns the alert with every text field passed through cleanField
// and the defaults filled in. Render calls it, so a caller cannot skip it.
func (a Alert) clean() Alert {
	a.RequestID = cleanField(a.RequestID)
	a.OrgID = cleanField(a.OrgID)
	a.Provider = cleanField(a.Provider)
	a.Model = cleanField(a.Model)
	a.Action = strings.ToUpper(cleanField(a.Action))
	a.Direction = cleanField(a.Direction)
	if a.Direction == "" {
		a.Direction = "input"
	}
	a.EventType = cleanField(a.EventType)
	a.Endpoint = cleanField(a.Endpoint)
	a.UserID = cleanField(a.UserID)
	a.DedupKey = cleanField(a.DedupKey)
	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now().UTC()
	}
	a.Timestamp = a.Timestamp.UTC()
	fs := make([]AlertFinding, len(a.Findings))
	for i, f := range a.Findings {
		fs[i] = AlertFinding{
			Type:     cleanField(f.Type),
			Category: cleanField(f.Category),
			Severity: strings.ToLower(cleanField(f.Severity)),
		}
	}
	a.Findings = fs
	return a
}

func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// primary is the finding the alert is named after: the most severe one, the
// first of them on a tie.
func (a Alert) primary() (AlertFinding, bool) {
	if len(a.Findings) == 0 {
		return AlertFinding{}, false
	}
	best := a.Findings[0]
	for _, f := range a.Findings[1:] {
		if severityRank(f.Severity) > severityRank(best.Severity) {
			best = f
		}
	}
	return best, true
}

func (a Alert) isTest() bool { return a.Action == actionTest }

// level is 0-4 from the most severe finding; 0 when there is none.
func (a Alert) level() int {
	p, _ := a.primary()
	return severityRank(p.Severity)
}

// severityName is the most severe finding's severity, "none" without one.
func (a Alert) severityName() string {
	if p, ok := a.primary(); ok && p.Severity != "" {
		return p.Severity
	}
	return "none"
}

func (a Alert) title() string {
	if a.isTest() {
		return "[Tamga] Test webhook"
	}
	t := fmt.Sprintf("[Tamga] %s on %s", a.Action, a.Direction)
	p, ok := a.primary()
	if !ok {
		return t
	}
	t += fmt.Sprintf(": %s/%s (%s)", p.Type, p.Category, p.Severity)
	if n := len(a.Findings) - 1; n > 0 {
		t += fmt.Sprintf(" and %d more", n)
	}
	return t
}

// findingGroup is one type/category/severity and how often it was found.
type findingGroup struct {
	AlertFinding
	Count int
}

func (a Alert) groups() []findingGroup {
	idx := map[AlertFinding]int{}
	var out []findingGroup
	for _, f := range a.Findings {
		if i, ok := idx[f]; ok {
			out[i].Count++
			continue
		}
		idx[f] = len(out)
		out = append(out, findingGroup{AlertFinding: f, Count: 1})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return severityRank(out[i].Severity) > severityRank(out[j].Severity)
	})
	return out
}

func (a Alert) groupsText() string {
	gs := a.groups()
	parts := make([]string, 0, len(gs))
	for i, g := range gs {
		if i == 10 {
			parts = append(parts, fmt.Sprintf("+%d more kinds", len(gs)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("%s/%s (%s) x%d", g.Type, g.Category, g.Severity, g.Count))
	}
	return strings.Join(parts, ", ")
}

type fact struct{ Label, Value string }

// facts is the body of the human-readable formats, one label per line.
func (a Alert) facts() []fact {
	out := []fact{
		{"Request", a.RequestID},
		{"Time", a.Timestamp.Format(time.RFC3339)},
		{"Action", a.Action},
		{"Direction", a.Direction},
	}
	add := func(label, v string) {
		if v != "" {
			out = append(out, fact{label, v})
		}
	}
	add("Organisation", a.OrgID)
	add("Provider", a.Provider)
	add("Model", a.Model)
	add("User", a.UserID)
	out = append(out, fact{"Findings", strconv.Itoa(len(a.Findings))})
	add("Kinds", a.groupsText())
	if a.CorrelatedCount > 1 {
		out = append(out, fact{"Events in window", strconv.Itoa(a.CorrelatedCount)})
	}
	return out
}

func (a Alert) text() string {
	fs := a.facts()
	lines := make([]string, len(fs))
	for i, f := range fs {
		lines[i] = f.Label + ": " + f.Value
	}
	return strings.Join(lines, "\n")
}

func (a Alert) details() map[string]string {
	m := map[string]string{}
	for _, f := range a.facts() {
		m[strings.ToLower(strings.ReplaceAll(f.Label, " ", "_"))] = f.Value
	}
	return m
}

func (a Alert) listedFindings() []AlertFinding {
	if len(a.Findings) > maxListedFindings {
		return a.Findings[:maxListedFindings]
	}
	return a.Findings
}

// document is the alert as JSON, for receivers that take any shape.
func (a Alert) document() map[string]interface{} {
	cats := map[string]int{}
	for _, f := range a.Findings {
		cats[f.Category]++
	}
	doc := map[string]interface{}{
		"source":         "tamga",
		"message":        a.title(),
		"ts":             a.Timestamp.Format(time.RFC3339),
		"event_type":     a.EventType,
		"request_id":     a.RequestID,
		"org_id":         a.OrgID,
		"provider":       a.Provider,
		"model":          a.Model,
		"action":         a.Action,
		"direction":      a.Direction,
		"severity":       a.severityName(),
		"findings_count": len(a.Findings),
		"findings":       a.listedFindings(),
		"categories":     cats,
		"input_risk":     a.InputRisk,
		"output_risk":    a.OutputRisk,
	}
	if a.UserID != "" {
		doc["user_id"] = a.UserID
	}
	if a.CorrelatedCount > 1 {
		doc["correlated_count"] = a.CorrelatedCount
	}
	return doc
}

func (a Alert) siemEvent() siem.EventInput {
	fs := make([]siem.FindingLike, len(a.Findings))
	for i, f := range a.Findings {
		fs[i] = siem.FindingLike{Type: f.Type, Category: f.Category, Severity: f.Severity}
	}
	return siem.EventInput{
		RequestID:  a.RequestID,
		Timestamp:  a.Timestamp,
		OrgID:      a.OrgID,
		Direction:  a.Direction,
		Provider:   a.Provider,
		Model:      a.Model,
		Action:     a.Action,
		EventType:  a.EventType,
		Endpoint:   a.Endpoint,
		UserID:     a.UserID,
		InputRisk:  a.InputRisk,
		OutputRisk: a.OutputRisk,
		Findings:   fs,
	}
}

// dedupKey never returns empty: PagerDuty and Opsgenie both need a key.
func (a Alert) dedupKey() string {
	if a.DedupKey != "" {
		return a.DedupKey
	}
	return "tamga-" + a.RequestID
}

// sampleAlert is the canonical probe used for webhook tests. It goes through
// the same renderer as a live alert, so what an operator sees parse in
// QRadar or Sentinel is the shape live alerts arrive in.
func sampleAlert() Alert {
	now := time.Now().UTC()
	return Alert{
		RequestID:  "req_test_" + now.Format("150405"),
		Timestamp:  now,
		OrgID:      "tamga",
		Provider:   "tamga",
		Model:      "probe",
		Action:     actionTest,
		Direction:  "input",
		EventType:  "webhook_probe",
		Endpoint:   "/api/v1/webhooks/test",
		UserID:     "tamga-admin",
		InputRisk:  50,
		OutputRisk: 0,
		Findings:   []AlertFinding{{Type: "test", Category: "probe", Severity: "medium"}},
		// A fixed key, so pressing Test repeatedly reopens one alert
		// instead of creating one per press.
		DedupKey: "tamga-test-probe",
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

// RenderTestPayload produces the body for the "Test webhook" action: the
// sample alert in the destination's own format.
func RenderTestPayload(w Webhook) ([]byte, error) {
	return Render(w, sampleAlert())
}

// Render produces the body one destination is sent for one alert. SIEM
// presets (Splunk HEC raw, Azure Sentinel, IBM QRadar) receive CEF/LEEF
// plain-text lines; the rest receive the JSON their API expects.
//
// A destination with a payload template is sent the template as it is,
// whatever the alert: the template has no placeholders.
func Render(w Webhook, a Alert) ([]byte, error) {
	if w.PayloadTemplate != "" {
		return []byte(w.PayloadTemplate), nil
	}
	a = a.clean()
	now := a.Timestamp.Format(time.RFC3339)
	switch w.Kind {
	case KindSlack:
		return json.Marshal(map[string]interface{}{
			"text": a.title() + "\n" + a.text(),
		})
	case KindTeams:
		// Microsoft retired the classic "Office 365 Connector / Incoming
		// Webhook" channel (Q4 2024). The replacement is Power Automate
		// "Workflows" inside Teams, which expects the Bot Framework
		// message shape wrapping an Adaptive Card. The legacy MessageCard
		// schema is silently ignored by Workflow endpoints.
		facts := make([]map[string]string, 0, 12)
		for _, f := range a.facts() {
			facts = append(facts, map[string]string{"title": f.Label, "value": f.Value})
		}
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
							{"type": "TextBlock", "size": "Medium", "weight": "Bolder", "text": a.title(), "wrap": true},
							{"type": "FactSet", "facts": facts},
						},
					},
				},
			},
		})
	case KindSplunk:
		sourcetype := "tamga:alert"
		if a.isTest() {
			sourcetype = "tamga:test"
		}
		return json.Marshal(map[string]interface{}{
			"event":      a.document(),
			"sourcetype": sourcetype,
		})
	case KindSplunkHEC:
		// Splunk HEC `raw` endpoint accepts bare CEF; customers set
		// sourcetype=cef on the HEC token for the field extractor.
		return []byte(siem.FormatCEF(a.siemEvent())), nil
	case KindSentinel:
		// Azure Sentinel data connector ingests CEF via a Syslog HTTP
		// bridge; same wire format.
		return []byte(siem.FormatCEF(a.siemEvent())), nil
	case KindQRadar:
		// QRadar DSM Editor targets LEEF 2.0.
		return []byte(siem.FormatLEEF(a.siemEvent())), nil
	case KindDatadog:
		alertType := "warning"
		tags := []string{"service:tamga", "action:" + strings.ToLower(a.Action), "direction:" + a.Direction}
		switch {
		case a.isTest():
			alertType = "info"
			tags = append(tags, "env:test")
		case a.Action == "BLOCK":
			alertType = "error"
		}
		if a.Provider != "" {
			tags = append(tags, "provider:"+a.Provider)
		}
		if p, ok := a.primary(); ok {
			tags = append(tags, "category:"+p.Category, "severity:"+p.Severity)
		}
		return json.Marshal(map[string]interface{}{
			"title":       a.title(),
			"text":        a.text(),
			"alert_type":  alertType,
			"source_type": "tamga",
			"tags":        tags,
		})
	case KindPagerDuty:
		// PagerDuty Events API v2: the routing_key (integration key) is
		// required in the body, not the URL. Every event is a "trigger";
		// the dedup_key folds repeats of the same alert, and a retried
		// delivery, into one incident.
		routing := w.AuthToken
		if routing == "" {
			routing = "TAMGA_ROUTING_KEY_NOT_SET"
		}
		severity := [...]string{"warning", "info", "warning", "error", "critical"}[a.level()]
		class := "policy"
		if p, ok := a.primary(); ok {
			class = p.Category
		}
		if a.isTest() {
			severity = "info"
		}
		return json.Marshal(map[string]interface{}{
			"routing_key":  routing,
			"event_action": "trigger",
			"dedup_key":    a.dedupKey(),
			"payload": map[string]interface{}{
				"summary":        a.title(),
				"severity":       severity,
				"source":         "tamga-proxy",
				"timestamp":      now,
				"component":      "proxy",
				"group":          a.OrgID,
				"class":          class,
				"custom_details": a.details(),
			},
		})
	case KindOpsgenie:
		// Opsgenie Alert API v2. Authorization header is injected at the
		// transport layer from AuthToken (see deliveryHeaders). The `alias`
		// field is Opsgenie's idempotency key: repeats of the same alert
		// count up on one open alert instead of creating one each.
		priority := [...]string{"P3", "P4", "P3", "P2", "P1"}[a.level()]
		tags := []string{"tamga", strings.ToLower(a.Action), a.Direction}
		if a.isTest() {
			priority = "P5"
			tags = []string{"tamga", "probe", "test"}
		}
		return json.Marshal(map[string]interface{}{
			"message":     truncate(a.title(), 130),
			"description": a.text(),
			"alias":       a.dedupKey(),
			"priority":    priority,
			"source":      "tamga-proxy",
			"tags":        tags,
			"details":     a.details(),
		})
	case KindServiceNow:
		// ServiceNow Incident table (/api/now/table/incident). Auth is
		// per-instance (Basic or OAuth bearer); operators set it via the
		// Headers map because ServiceNow instances vary. urgency/impact
		// "3" = low so a test probe doesn't wake an on-call.
		urgency := [...]string{"3", "3", "2", "1", "1"}[a.level()]
		if a.isTest() {
			urgency = "3"
		}
		return json.Marshal(map[string]interface{}{
			"short_description": truncate(a.title(), 160),
			"description":       a.text(),
			"urgency":           urgency,
			"impact":            urgency,
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
		paragraphs := make([]map[string]interface{}, 0, 12)
		for _, f := range a.facts() {
			paragraphs = append(paragraphs, map[string]interface{}{
				"type": "paragraph",
				"content": []map[string]interface{}{
					{"type": "text", "text": f.Label + ": " + f.Value},
				},
			})
		}
		return json.Marshal(map[string]interface{}{
			"fields": map[string]interface{}{
				"project":   map[string]string{"key": projectKey},
				"summary":   truncate(a.title(), 250),
				"issuetype": map[string]string{"name": issueType},
				"description": map[string]interface{}{
					"type":    "doc",
					"version": 1,
					"content": paragraphs,
				},
			},
		})
	}
	return json.Marshal(a.document())
}

// RenderRuleNotify is the body posted to a policy rule's notify URL. Those
// URLs have no kind, so the body is the JSON document with a "text" line on
// top, which is what a Slack incoming webhook reads.
func RenderRuleNotify(a Alert) ([]byte, error) {
	a = a.clean()
	return json.Marshal(map[string]interface{}{
		"text":  a.title() + "\n" + a.text(),
		"tamga": a.document(),
	})
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
