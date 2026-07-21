package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/yatuk/tamga/internal/scanner/operator_state"
	"github.com/yatuk/tamga/internal/telemetry"
	pb "github.com/yatuk/tamga/proto/analyzer/v1"
)

// LogHandler writes each event as structured zerolog output.
func LogHandler(log zerolog.Logger) func(Event) {
	return func(e Event) {
		ev := log.With().
			Str("component", "event_handler").
			Str("event_type", e.EventType).
			Str("request_id", e.RequestID).
			Str("org_id", e.OrgID).
			Str("provider", e.Provider).
			Str("model", e.Model).
			Str("action", e.Action).
			Str("content_type", e.ContentType).
			Int("findings", len(e.Findings)).
			Time("timestamp", e.Timestamp).
			Logger()
		if len(e.Body) > 0 {
			ev = ev.With().Int("body_bytes", len(e.Body)).Logger()
		}
		if e.EventType == "request_scanned" || e.EventType == "request_blocked" {
			lvl := e.InputRisk.Level
			if lvl == "" {
				lvl = "none"
			}
			ev = ev.With().
				Int("input_risk", e.InputRisk.Percentage).
				Int("output_risk", e.OutputRisk.Percentage).
				Str("risk_level", lvl).
				Logger()
		}
		ev.Info().Msg("tamga event")
	}
}

// Metrics holds in-memory counters for the proxy (best-effort; process-local).
type Metrics struct {
	TotalRequests atomic.Int64
	Blocked       atomic.Int64
	Redacted      atomic.Int64
	Warned        atomic.Int64
}

// MetricsHandler updates counters from event stream.
func MetricsHandler(m *Metrics) func(Event) {
	if m == nil {
		return func(Event) {}
	}
	return func(e Event) {
		switch e.EventType {
		case "request_blocked":
			m.Blocked.Add(1)
			m.TotalRequests.Add(1)
		case "request_scanned":
			m.TotalRequests.Add(1)
			switch e.Action {
			case "REDACT":
				m.Redacted.Add(1)
			case "WARN":
				m.Warned.Add(1)
			}
		case "output_scan_hint":
			// not counted toward request totals
		}
	}
}

// hybridSkipLow is the lower risk threshold below which deep scanning is skipped
// (benign traffic — no value from LLM judge).
const hybridSkipLow = 0.15

// hybridSkipHigh is the upper risk threshold above which deep scanning is skipped
// (fast scanner already certain — action taken, deep scan would be redundant).
const hybridSkipHigh = 0.90

// scanTypesForRisk returns the analyzer scan_types list based on risk score.
// Medium risk: PII + injection (full deep scan).
// Low-medium risk: PII only (cheaper, injection unlikely).
func scanTypesForRisk(riskScore float64) []string {
	if riskScore >= 0.45 {
		return []string{"pii", "injection"}
	}
	return []string{"pii"}
}

// AnalyzerClient is the subset of analyzer.GRPCClient needed by event handlers.
// Extracted as an interface to allow mocking in tests without importing gRPC internals.
type AnalyzerClient interface {
	Enabled() bool
	Analyze(ctx context.Context, req *pb.AnalyzeRequest) (*pb.AnalyzeResponse, error)
}

// AnalyzerHandler fans scan_complete events to the Python analyzer over gRPC (fail-open).
// Hybrid routing: skips dispatch when input risk is outside [hybridSkipLow, hybridSkipHigh].
func AnalyzerHandler(client AnalyzerClient) func(Event) {
	return func(e Event) {
		if client == nil || !client.Enabled() {
			return
		}
		if e.EventType != "request_scanned" && e.EventType != "request_blocked" {
			return
		}

		// Hybrid confidence gate — skip trivially clean or already-certain events.
		risk := e.InputRisk.Score
		if risk < hybridSkipLow {
			log.Debug().
				Str("component", "analyzer_handler").
				Str("request_id", e.RequestID).
				Float64("input_risk", risk).
				Msg("analyzer: skip (risk below threshold)")
			return
		}
		if risk > hybridSkipHigh {
			log.Debug().
				Str("component", "analyzer_handler").
				Str("request_id", e.RequestID).
				Float64("input_risk", risk).
				Msg("analyzer: skip (risk above ceiling — fast scanner certain)")
			return
		}

		ctx := context.Background()
		if len(e.TraceContext) > 0 {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(e.TraceContext))
		}
		_, sp := telemetry.Tracer().Start(ctx, "analyzer.deep_scan")
		defer sp.End()

		content := string(e.Body)
		if content == "" {
			return
		}
		req := &pb.AnalyzeRequest{
			RequestId: e.RequestID,
			Content:   content,
			ScanTypes: scanTypesForRisk(risk),
			Provider:  e.Provider,
			Model:     e.Model,
			Metadata: map[string]string{
				"org_id":     e.OrgID,
				"action":     e.Action,
				"endpoint":   e.Endpoint,
				"direction":  "input",
				"input_risk": fmt.Sprintf("%.3f", risk),
			},
			PreScanned: true,
		}
		resp, err := client.Analyze(ctx, req)
		if err != nil {
			log.Warn().Err(err).Str("component", "analyzer_handler").Str("request_id", e.RequestID).Msg("analyzer: deep scan failed (fail-open)")
			return
		}
		if resp != nil && len(resp.Findings) > 0 {
			log.Info().
				Str("component", "analyzer_handler").
				Str("request_id", e.RequestID).
				Int("deep_findings", len(resp.Findings)).
				Float64("duration_ms", resp.DurationMs).
				Float64("input_risk", risk).
				Strs("scan_types", req.ScanTypes).
				Msg("analyzer: deep scan complete")
		}
	}
}

// EventOperatorStateDeepScan is published by the operator_state scanner when
// fast-tier findings warrant slow-tier semantic comparison.
const EventOperatorStateDeepScan = "operator_state_deep_scan"

// operatorStateRequestKey is the Event.Metadata key holding the
// *operator_state.AnalyzerRequest payload.
const operatorStateRequestKey = "operator_state_request"

// OperatorStateAnalyzerHandler routes operator-state deep-scan events to the
// Python analyzer (fail-open) and logs the resulting advisory with provenance.
//
// Until the analyzer ships an operator-state judge endpoint, it returns no
// findings for this scan type and the advisory verdict is "inconclusive" —
// the deterministic tier's decision stands either way.
func OperatorStateAnalyzerHandler(client AnalyzerClient) func(Event) {
	return func(e Event) {
		if e.EventType != EventOperatorStateDeepScan {
			return
		}
		req, _ := e.Metadata[operatorStateRequestKey].(*operator_state.AnalyzerRequest)
		if req == nil {
			return
		}
		if client == nil || !client.Enabled() {
			logAdvisory(operatorStateAdvisory(req, nil))
			return
		}

		ctx := context.Background()
		if len(e.TraceContext) > 0 {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(e.TraceContext))
		}
		_, sp := telemetry.Tracer().Start(ctx, "analyzer.operator_state_semantic")
		defer sp.End()

		decisionRefs, _ := json.Marshal(req.DecisionRefs)
		noteRefs, _ := json.Marshal(req.NoteRefs)
		fastFindings, _ := json.Marshal(req.FastFindings)
		resp, err := client.Analyze(ctx, &pb.AnalyzeRequest{
			RequestId: req.RequestID,
			Content:   req.Prompt,
			ScanTypes: []string{"operator_state"},
			Metadata: map[string]string{
				"scan_type":     req.ScanType,
				"decision_refs": string(decisionRefs),
				"note_refs":     string(noteRefs),
				"fast_findings": string(fastFindings),
			},
			PreScanned: true,
		})
		if err != nil {
			log.Warn().Err(err).Str("component", "operator_state_handler").Str("request_id", req.RequestID).Msg("analyzer: operator-state semantic scan failed (fail-open)")
			return
		}
		logAdvisory(operatorStateAdvisory(req, resp))
	}
}

// operatorStateAdvisory synthesizes an Advisory from the analyzer response.
func operatorStateAdvisory(req *operator_state.AnalyzerRequest, resp *pb.AnalyzeResponse) operator_state.Advisory {
	adv := operator_state.Advisory{
		RequestID: req.RequestID,
		Verdict:   operator_state.VerdictInconclusive,
		Provenance: operator_state.Provenance{
			EvaluatedAt: time.Now().UTC(),
		},
	}
	for _, dc := range req.DecisionRefs {
		adv.Provenance.DecisionIDs = append(adv.Provenance.DecisionIDs, dc.ID)
		adv.Provenance.LogTimestamps = append(adv.Provenance.LogTimestamps, dc.LastEventTS)
	}
	if resp != nil {
		for _, f := range resp.Findings {
			if f.Type == "operator_state" && f.Confidence >= 0.7 {
				adv.Verdict = operator_state.VerdictContradictionConfirmed
				adv.Confidence = f.Confidence
				break
			}
		}
	}
	return adv
}

func logAdvisory(adv operator_state.Advisory) {
	payload, _ := json.Marshal(adv)
	log.Info().
		Str("component", "operator_state_handler").
		Str("event", "operator_state_advisory").
		Str("request_id", adv.RequestID).
		Str("verdict", adv.Verdict).
		RawJSON("advisory", payload).
		Msg("operator-state semantic advisory")
}
