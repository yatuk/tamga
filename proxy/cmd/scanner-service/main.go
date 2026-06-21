// Command scanner-service runs the Tamga scanner pipeline as a standalone
// gRPC service. It registers the same seven scanners as the main proxy
// binary and exposes them over gRPC on port 50052 (configurable via
// SCANNER_SERVICE_PORT).
package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/yatuk/tamga/internal/telemetry"
	"google.golang.org/grpc/keepalive"

	"github.com/yatuk/tamga/internal/scanner"
	pb "github.com/yatuk/tamga/proto/scanner/v1"
)

type scannerServer struct {
	pb.UnimplementedScannerServiceServer
	registry *scanner.Registry
	mode     scanner.PipelineMode
	pool     *scanner.WorkerPool
}

// Scan runs the full scanner pipeline and returns all findings.
func (s *scannerServer) Scan(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
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

	findings, err := s.registry.ScanAllWithConfig(ctx, req.Content, cfg)
	if err != nil {
		log.Warn().Err(err).Str("request_id", req.RequestId).Msg("scan failed")
		// Return empty response on error — fail-open semantics.
		return &pb.ScanResponse{
			RequestId:  req.RequestId,
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}, nil
	}

	return &pb.ScanResponse{
		RequestId:  req.RequestId,
		Findings:   scanner.FindingsToProto(findings),
		DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
	}, nil
}

// HealthCheck reports the scanner service status.
func (s *scannerServer) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Status:  "SERVING",
		Service: "tamga-scanner",
	}, nil
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Str("component", "scanner-service").Logger()

	port := os.Getenv("SCANNER_SERVICE_PORT")
	if port == "" {
		port = "50052"
	}

	// Pipeline mode for this instance.
	mode := scanner.ModeAdaptive
	if m := os.Getenv("SCANNER_PIPELINE_MODE"); m != "" {
		mode = scanner.PipelineMode(m)
	}

	// Build scanner registry — same seven scanners as main proxy.
	registry := scanner.NewRegistry()

	// Fast scanners (< 1ms) — sequential.
	registry.Register(scanner.NewPIIScanner())
	registry.SetSpeed("pii", scanner.SpeedFast)
	registry.Register(scanner.NewSecretScanner())
	registry.SetSpeed("secret", scanner.SpeedFast)

	// Custom scanner — spec getter returns nil for standalone mode
	// (custom entities are policy-driven; scanner-service doesn't load policy YAML).
	registry.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec { return nil }))
	registry.SetSpeed("custom", scanner.SpeedFast)
	registry.Register(scanner.NewCompetitorScanner(func() []scanner.CompetitorSpec { return nil }))
	registry.SetSpeed("competitor", scanner.SpeedFast)

	// Slow scanners (≥ 1ms) — parallel.
	registry.Register(scanner.NewInjectionScanner())
	registry.SetSpeed("injection", scanner.SpeedSlow)
	// DFA needs initialisation once before use.
	if err := scanner.InitDFA(); err != nil {
		log.Warn().Err(err).Msg("DFA init failed — injection scanner will use regex fallback")
	}
	registry.Register(scanner.NewJailbreakScanner())
	registry.SetSpeed("jailbreak", scanner.SpeedSlow)
	registry.Register(scanner.NewContentModerationScanner())
	registry.SetSpeed("content_moderation", scanner.SpeedSlow)

	// BIN database — try embedded, warn on failure.
	if err := scanner.InitBINLookupEmbed(); err != nil {
		log.Warn().Err(err).Msg("embedded BIN DB load failed — card detection continues without issuer validation")
	}

	// Optional worker pool.
	var pool *scanner.WorkerPool
	if ws := os.Getenv("SCANNER_WORKER_POOL_SIZE"); ws != "" {
		n := parseInt(ws)
		if n > 0 {
			qs := parseInt(os.Getenv("SCANNER_WORKER_QUEUE_SIZE"))
			if qs <= 0 {
				qs = n * 2
			}
			pool = scanner.NewWorkerPool(n, qs)
			log.Info().Int("workers", n).Int("queue_size", qs).Msg("worker pool started")
			defer func() { _ = pool.Shutdown(30 * time.Second) }()
		}
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal().Err(err).Str("port", port).Msg("failed to listen")
	}

	server := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              30 * time.Second,
			Timeout:           5 * time.Second,
		}),
		grpc.MaxConcurrentStreams(1000),
		grpc.UnaryInterceptor(telemetry.UnaryServerTraceInterceptor()),
	)

	pb.RegisterScannerServiceServer(server, &scannerServer{
		registry: registry,
		mode:     mode,
		pool:     pool,
	})

	log.Info().
		Str("port", port).
		Str("mode", string(mode)).
		Bool("worker_pool", pool != nil).
		Int("scanners", registry.Count()).
		Msg("tamga scanner service starting")

	// Start gRPC server in a goroutine.
	go func() {
		log.Info().Msg("gRPC server listening")
		if err := server.Serve(listener); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
		}
	}()

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down scanner service...")

	// Graceful stop with timeout.
	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		log.Info().Msg("gRPC server stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Warn().Msg("graceful stop timed out, forcing stop")
		server.Stop()
	}
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
