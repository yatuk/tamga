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
	RequestID    string
	Timestamp    time.Time
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
		"request=" + cefExt(e.Endpoint),
		"suser=" + cefExt(e.UserID),
		"act=" + cefExt(e.Action),
		"cs1Label=provider", "cs1=" + cefExt(e.Provider),
		"cs2Label=model", "cs2=" + cefExt(e.Model),
		"cs3Label=finding_type", "cs3=" + cefExt(primary.Type),
		"cs4Label=policy_channel", "cs4=input_risk",
		"cn1Label=input_risk", "cn1=" + fmt.Sprintf("%d", e.InputRisk),
		"cn2Label=output_risk", "cn2=" + fmt.Sprintf("%d", e.OutputRisk),
		"cn3Label=findings_count", "cn3=" + fmt.Sprintf("%d", len(e.Findings)),
		"flexNumber1Label=input_tokens", "flexNumber1=" + fmt.Sprintf("%d", e.InputTokens),
		"flexNumber2Label=output_tokens", "flexNumber2=" + fmt.Sprintf("%d", e.OutputTokens),
		"flexString1Label=cost_usd", "flexString1=" + fmt.Sprintf("%.6f", e.CostUSD),
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
		"usrName=" + e.UserID,
		"url=" + e.Endpoint,
		"findingType=" + primary.Type,
		"findingsCount=" + fmt.Sprintf("%d", len(e.Findings)),
		"inputRisk=" + fmt.Sprintf("%d", e.InputRisk),
		"outputRisk=" + fmt.Sprintf("%d", e.OutputRisk),
		"inputTokens=" + fmt.Sprintf("%d", e.InputTokens),
		"outputTokens=" + fmt.Sprintf("%d", e.OutputTokens),
		"costUSD=" + fmt.Sprintf("%.6f", e.CostUSD),
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
