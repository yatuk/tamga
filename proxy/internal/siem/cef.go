// Package siem formats Tamga events into vendor-specific SIEM payloads
// (ArcSight CEF 0.1 and QRadar LEEF 2.0). The same semantic fields used by
// the dashboard are emitted so Security Operations Centers can correlate
// Tamga detections with firewall / EDR / WAF data in the same panel.
package siem

import (
	"fmt"
	"strings"
	"time"
)

// FindingLike is a minimal view of a scanner finding so we can avoid an
// import cycle with the scanner package.
type FindingLike struct {
	Type       string
	Category   string
	Severity   string
	Match      string
	Confidence float64
}

// EventInput is the normalised fields we expect from an events.Event. The
// caller (proxy/integrations layer) is responsible for populating these
// from its internal representation — this keeps `siem` free of deps.
type EventInput struct {
	RequestID string
	Timestamp time.Time
	// OrgID and Direction ("input" or "output") are left out of the line
	// when empty.
	OrgID        string
	Direction    string
	Provider     string
	Model        string
	Action       string
	EventType    string
	Endpoint     string
	UserID       string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	InputRisk    int // 0-100
	OutputRisk   int // 0-100
	Findings     []FindingLike
}

// cefEscape escapes the five reserved CEF metadata characters.
func cefEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "=", `\=`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

// cefExt escapes reserved characters inside extension values. In CEF, `=`
// and `\` must be escaped within extension values.
func cefExt(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "=", `\=`)
	return s
}

// severityToCEF maps Tamga severities to CEF 0-10 scale.
func severityToCEF(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 10
	case "high":
		return 8
	case "medium":
		return 5
	case "low":
		return 3
	}
	return 1
}

// FormatCEF returns a CEF 0 line for the given event. The class/signature
// IDs are chosen so Splunk CIM / ArcSight field mapping works out of the
// box: signature=tamga.<action>.<primary_category>.
//
// Example output:
//
//	CEF:0|Tamga|Proxy|0.5|tamga.block.pii|PII leak blocked|8|...
func FormatCEF(e EventInput) string {
	primary := primaryFinding(e)
	sig := fmt.Sprintf("tamga.%s.%s", strings.ToLower(e.Action), primary.Category)
	if primary.Category == "" {
		sig = fmt.Sprintf("tamga.%s", strings.ToLower(e.Action))
	}
	name := "Tamga policy action"
	if primary.Type != "" {
		name = fmt.Sprintf("%s %s", primary.Type, primary.Category)
	}

	sev := severityToCEF(primary.Severity)

	header := fmt.Sprintf("CEF:0|Tamga|Proxy|0.5|%s|%s|%d|",
		cefEscape(sig),
		cefEscape(name),
		sev,
	)

	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	// Key=value extension block (RFC-ish; SIEMs like key=value kv pairs).
	kv := []string{
		"rt=" + cefExt(ts.UTC().Format("Jan 02 2006 15:04:05")),
		"deviceExternalId=" + cefExt(e.RequestID),
		"requestMethod=" + cefExt(e.EventType),
	}
	// A live alert carries no request path, and not every request names a
	// user: an empty value is left out instead of sent as "key=".
	if e.Endpoint != "" {
		kv = append(kv, "request="+cefExt(e.Endpoint))
	}
	if e.UserID != "" {
		kv = append(kv, "suser="+cefExt(e.UserID))
	}
	kv = append(kv,
		"act="+cefExt(e.Action),
		"cs1Label=provider", "cs1="+cefExt(e.Provider),
		"cs2Label=model", "cs2="+cefExt(e.Model),
		"cs3Label=finding_type", "cs3="+cefExt(primary.Type),
		"cs4Label=policy_channel", "cs4=input_risk",
		"cn1Label=input_risk", "cn1="+fmt.Sprintf("%d", e.InputRisk),
		"cn2Label=output_risk", "cn2="+fmt.Sprintf("%d", e.OutputRisk),
		"cn3Label=findings_count", "cn3="+fmt.Sprintf("%d", len(e.Findings)),
		"flexNumber1Label=input_tokens", "flexNumber1="+fmt.Sprintf("%d", e.InputTokens),
		"flexNumber2Label=output_tokens", "flexNumber2="+fmt.Sprintf("%d", e.OutputTokens),
		"flexString1Label=cost_usd", "flexString1="+fmt.Sprintf("%.6f", e.CostUSD),
	)
	if e.OrgID != "" {
		kv = append(kv, "cs5Label=org", "cs5="+cefExt(e.OrgID))
	}
	if fs := findingsSummary(e.Findings); fs != "" {
		kv = append(kv, "cs6Label=findings", "cs6="+cefExt(fs))
	}
	if e.Direction != "" {
		kv = append(kv, "flexString2Label=direction", "flexString2="+cefExt(e.Direction))
	}

	return header + strings.Join(kv, " ")
}

// FormatLEEF returns a LEEF 2.0 line. LEEF uses tab as the default
// separator inside the extension block; QRadar parses this natively.
//
//	LEEF:2.0|Tamga|Proxy|0.5|tamga.block.pii|^|key=value^key=value^...
func FormatLEEF(e EventInput) string {
	primary := primaryFinding(e)
	sig := fmt.Sprintf("tamga.%s.%s", strings.ToLower(e.Action), primary.Category)
	if primary.Category == "" {
		sig = fmt.Sprintf("tamga.%s", strings.ToLower(e.Action))
	}

	sep := "^"
	header := fmt.Sprintf("LEEF:2.0|Tamga|Proxy|0.5|%s|%s|", sig, sep)

	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	pairs := []string{
		"devTime=" + ts.UTC().Format(time.RFC3339),
		"devTimeFormat=yyyy-MM-dd'T'HH:mm:ssXXX",
		"src=tamga",
		"cat=" + primary.Category,
		"sev=" + fmt.Sprintf("%d", severityToCEF(primary.Severity)),
		"requestID=" + e.RequestID,
		"provider=" + e.Provider,
		"model=" + e.Model,
		"action=" + e.Action,
	}
	if e.UserID != "" {
		pairs = append(pairs, "usrName="+e.UserID)
	}
	if e.Endpoint != "" {
		pairs = append(pairs, "url="+e.Endpoint)
	}
	pairs = append(pairs,
		"findingType="+primary.Type,
		"findingsCount="+fmt.Sprintf("%d", len(e.Findings)),
		"inputRisk="+fmt.Sprintf("%d", e.InputRisk),
		"outputRisk="+fmt.Sprintf("%d", e.OutputRisk),
		"inputTokens="+fmt.Sprintf("%d", e.InputTokens),
		"outputTokens="+fmt.Sprintf("%d", e.OutputTokens),
		"costUSD="+fmt.Sprintf("%.6f", e.CostUSD),
	)
	if e.OrgID != "" {
		pairs = append(pairs, "orgID="+e.OrgID)
	}
	if fs := findingsSummary(e.Findings); fs != "" {
		pairs = append(pairs, "findings="+fs)
	}
	if e.Direction != "" {
		pairs = append(pairs, "direction="+e.Direction)
	}

	return header + strings.Join(pairs, sep)
}

func primaryFinding(e EventInput) FindingLike {
	if len(e.Findings) == 0 {
		return FindingLike{Category: "policy"}
	}
	// Highest severity wins; ties broken by first appearance.
	best := e.Findings[0]
	for _, f := range e.Findings[1:] {
		if severityToCEF(f.Severity) > severityToCEF(best.Severity) {
			best = f
		}
	}
	return best
}

// maxSummaryFindings bounds the per-finding list in one line.
const maxSummaryFindings = 20

// findingsSummary lists each finding as type/category:severity. It never
// reads Match: a line that leaves the proxy says what kind of thing was
// found, not what the text was.
func findingsSummary(fs []FindingLike) string {
	if len(fs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fs))
	for i, f := range fs {
		if i == maxSummaryFindings {
			parts = append(parts, fmt.Sprintf("+%d more", len(fs)-i))
			break
		}
		parts = append(parts, f.Type+"/"+f.Category+":"+f.Severity)
	}
	return strings.Join(parts, ",")
}
