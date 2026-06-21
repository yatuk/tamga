package events

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

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
