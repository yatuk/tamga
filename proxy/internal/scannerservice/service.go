// Package scannerservice is the scanner pipeline behind gRPC: what the
// scanner-service command serves, in a package of its own so that a test can
// run the same server the command runs.
package scannerservice

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/yatuk/tamga/internal/errkind"
	"github.com/yatuk/tamga/internal/scanner"
	pb "github.com/yatuk/tamga/proto/scanner/v1"
)

// Server answers Scan calls with the findings of a registry.
type Server struct {
	pb.UnimplementedScannerServiceServer
	registry *scanner.Registry
	mode     scanner.PipelineMode
	pool     *scanner.WorkerPool
}

// New returns a server that scans with registry. mode is the pipeline mode
// of a call that names none; pool may be nil.
func New(registry *scanner.Registry, mode scanner.PipelineMode, pool *scanner.WorkerPool) *Server {
	return &Server{registry: registry, mode: mode, pool: pool}
}

// NewRegistry returns the scanners the service runs: the ones that need
// nothing but the text. Custom entities and competitor names come from the
// proxy's policy and stay with the proxy; they are registered here without
// patterns so that the set is the one the proxy's own registry starts from.
func NewRegistry() *scanner.Registry {
	registry := scanner.NewRegistry()

	// Fast scanners (< 1ms) — sequential.
	registry.Register(scanner.NewPIIScanner())
	registry.SetSpeed("pii", scanner.SpeedFast)
	registry.Register(scanner.NewSecretScanner())
	registry.SetSpeed("secret", scanner.SpeedFast)
	registry.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec { return nil }))
	registry.SetSpeed("custom", scanner.SpeedFast)
	registry.Register(scanner.NewCompetitorScanner(func() []scanner.CompetitorSpec { return nil }))
	registry.SetSpeed("competitor", scanner.SpeedFast)

	// Slow scanners (≥ 1ms) — parallel.
	registry.Register(scanner.NewInjectionScanner())
	registry.SetSpeed("injection", scanner.SpeedSlow)
	registry.Register(scanner.NewJailbreakScanner())
	registry.SetSpeed("jailbreak", scanner.SpeedSlow)
	registry.Register(scanner.NewContentModerationScanner())
	registry.SetSpeed("content_moderation", scanner.SpeedSlow)
	return registry
}

// Scan runs the scanners over one text and returns the findings.
//
// The text is one piece of a request: the proxy calls once per segment
// (scanner.ScanSegmentsRemote), or once with the whole body when it does not
// know the request's shape. What needs more than one segment (an instruction
// split over two, what a text's place in the conversation says) is decided by
// the proxy, which has the segments and their roles.
func (s *Server) Scan(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
	start := time.Now()

	mode := s.mode
	if req.PipelineMode != "" {
		mode = scanner.PipelineMode(req.PipelineMode)
	}
	if mode == "" {
		mode = scanner.ModeAdaptive
	}

	cfg := scanner.PipelineConfig{
		Mode:    mode,
		Timeout: 5 * time.Second,
		Pool:    s.pool,
	}

	// ScanRaw settles the findings before they leave: a place in the text
	// found again in a normalised view of it is reported once. That has to
	// happen here, where the text is. A secret's match holds none of its
	// value, so the proxy cannot tell an echo from a second secret.
	findings, err := s.registry.ScanRaw(ctx, req.Content, cfg)
	if err != nil {
		// By kind only: a scanner's error can quote the text it read.
		log.Warn().Str("reason", scanner.IncompleteReason(err)).Str("error_kind", errkind.Of(err)).Str("request_id", req.RequestId).Msg("scan failed")
		// An empty response would read as a clean scan. The caller has to
		// know the scan did not finish, so that its own scan.on_error applies.
		code := codes.Unavailable
		if ctx.Err() != nil {
			code = codes.DeadlineExceeded
		}
		return nil, status.Error(code, "scan incomplete")
	}

	return &pb.ScanResponse{
		RequestId:  req.RequestId,
		Findings:   scanner.FindingsToProto(findings),
		DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
	}, nil
}

// HealthCheck reports the scanner service status.
func (s *Server) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Status:  "SERVING",
		Service: "tamga-scanner",
	}, nil
}
