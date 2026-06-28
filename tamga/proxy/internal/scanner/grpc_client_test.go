package scanner

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/yatuk/tamga/proto/scanner/v1"
)

// ── Mock gRPC scanner server ─────────────────────────────────────────────────

type mockScannerServer struct {
	pb.UnimplementedScannerServiceServer
	scanFn        func(context.Context, *pb.ScanRequest) (*pb.ScanResponse, error)
	healthCheckFn func(context.Context, *pb.HealthRequest) (*pb.HealthResponse, error)
}

func (m *mockScannerServer) Scan(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
	if m.scanFn != nil {
		return m.scanFn(ctx, req)
	}
	return &pb.ScanResponse{}, nil
}

func (m *mockScannerServer) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	if m.healthCheckFn != nil {
		return m.healthCheckFn(ctx, req)
	}
	return &pb.HealthResponse{}, nil
}

// newTestScannerClient creates a GRPCScannerClient connected to an in-memory
// gRPC server with the given mock implementation. It does NOT use
// NewGRPCScannerClient (which adds the telemetry interceptor) so tests can
// run without an OpenTelemetry SDK.
func newTestScannerClient(t *testing.T, server pb.ScannerServiceServer) (*GRPCScannerClient, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterScannerServiceServer(srv, server)
	go func() {
		_ = srv.Serve(lis)
	}()

	addr := lis.Addr().String()
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		_ = lis.Close()
		t.Fatalf("failed to dial %s: %v", addr, err)
	}

	c := &GRPCScannerClient{
		conn: conn,
		stub: pb.NewScannerServiceClient(conn),
	}

	if !c.Enabled() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
		t.Fatal("client should be enabled immediately after construction")
	}

	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return c, cleanup
}

// ── NewGRPCScannerClient ─────────────────────────────────────────────────────

func TestNewGRPCScannerClient_EmptyAddr(t *testing.T) {
	c, err := NewGRPCScannerClient(context.Background(), GRPCScannerConfig{Addr: ""})
	if err != nil {
		t.Fatalf("empty addr should return nil, nil: %v", err)
	}
	if c != nil {
		t.Fatal("empty addr should return nil client")
	}
}

func TestNewGRPCScannerClient_ValidAddr(t *testing.T) {
	// gRPC v1.80 uses lazy connections — grpc.NewClient does not dial at
	// construction time. Creating a client with any well-formed address
	// succeeds; actual connection errors surface on the first RPC.
	c, err := NewGRPCScannerClient(context.Background(), GRPCScannerConfig{
		Addr:        "127.0.0.1:1",
		DialTimeout: 3 * time.Second,
		CallTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewGRPCScannerClient should succeed (lazy connect): %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client for non-empty address")
	}
	// With lazy connect, the initial state is Idle — Enabled() returns true.
	if !c.Enabled() {
		t.Log("client not enabled — gRPC state may differ on this platform")
	}
	defer c.Close()
}

func TestNewGRPCScannerClient_DefaultTimeouts(t *testing.T) {
	c, err := NewGRPCScannerClient(context.Background(), GRPCScannerConfig{
		Addr: "127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("NewGRPCScannerClient without explicit timeouts: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	defer c.Close()
}

// ── Enabled ──────────────────────────────────────────────────────────────────

func TestGRPCScannerClient_Enabled_AfterDial(t *testing.T) {
	c, cleanup := newTestScannerClient(t, &mockScannerServer{})
	defer cleanup()
	if !c.Enabled() {
		t.Fatal("client should report enabled after successful dial")
	}
}

func TestGRPCScannerClient_Enabled_NilClient(t *testing.T) {
	var c *GRPCScannerClient
	if c.Enabled() {
		t.Fatal("nil client should not be enabled")
	}
}

func TestGRPCScannerClient_Enabled_NilConnection(t *testing.T) {
	c := &GRPCScannerClient{conn: nil, stub: nil}
	if c.Enabled() {
		t.Fatal("client with nil conn should not be enabled")
	}
}

// ── Name ─────────────────────────────────────────────────────────────────────

func TestGRPCScannerClient_Name(t *testing.T) {
	c, cleanup := newTestScannerClient(t, &mockScannerServer{})
	defer cleanup()

	if c.Name() != "grpc-scanner" {
		t.Errorf("Name: want 'grpc-scanner', got %q", c.Name())
	}
}

func TestGRPCScannerClient_Name_NilClient(t *testing.T) {
	// Method on nil *GRPCScannerClient should return "grpc-scanner" (no field access).
	var c *GRPCScannerClient
	if c.Name() != "grpc-scanner" {
		t.Errorf("nil Name: want 'grpc-scanner', got %q", c.Name())
	}
}

// ── Scan ─────────────────────────────────────────────────────────────────────

func TestGRPCScannerClient_Scan_HappyPath(t *testing.T) {
	srv := &mockScannerServer{
		scanFn: func(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
			return &pb.ScanResponse{
				RequestId: req.RequestId,
				Findings: []*pb.Finding{
					{Type: "pii", Category: "EMAIL_ADDRESS", Severity: "medium", Match: "us…@ex…", Confidence: 0.85},
					{Type: "secret", Category: "GITHUB_TOKEN", Severity: "critical", Match: "ghp_…", Confidence: 0.95},
				},
				DurationMs: 15.2,
			}, nil
		},
	}
	c, cleanup := newTestScannerClient(t, srv)
	defer cleanup()

	findings, err := c.Scan(context.Background(), []byte("Email: user@example.com"))
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("want 2 findings, got %d", len(findings))
	}
	if findings[0].Type != "pii" {
		t.Errorf("finding[0] Type: want 'pii', got %q", findings[0].Type)
	}
	if findings[1].Type != "secret" {
		t.Errorf("finding[1] Type: want 'secret', got %q", findings[1].Type)
	}
}

func TestGRPCScannerClient_Scan_EmptyFindings(t *testing.T) {
	srv := &mockScannerServer{
		scanFn: func(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
			return &pb.ScanResponse{
				RequestId:  req.RequestId,
				Findings:   nil,
				DurationMs: 3.1,
			}, nil
		},
	}
	c, cleanup := newTestScannerClient(t, srv)
	defer cleanup()

	findings, err := c.Scan(context.Background(), []byte("clean content"))
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("want 0 findings for clean content, got %d", len(findings))
	}
}

func TestGRPCScannerClient_Scan_ServerError(t *testing.T) {
	srv := &mockScannerServer{
		scanFn: func(ctx context.Context, req *pb.ScanRequest) (*pb.ScanResponse, error) {
			return nil, status.Error(codes.Internal, "scanner engine failed")
		},
	}
	c, cleanup := newTestScannerClient(t, srv)
	defer cleanup()

	_, err := c.Scan(context.Background(), []byte("test"))
	if err == nil {
		t.Fatal("expected error from server, got nil")
	}
}

func TestGRPCScannerClient_Scan_DisabledClient(t *testing.T) {
	// A client that reports !Enabled() returns nil, nil without making RPC.
	c := &GRPCScannerClient{conn: nil, stub: nil}
	findings, err := c.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatalf("disabled Scan should return nil error: %v", err)
	}
	if findings != nil {
		t.Fatalf("disabled Scan should return nil findings, got %v", findings)
	}
}

func TestGRPCScannerClient_Scan_NilClient(t *testing.T) {
	var c *GRPCScannerClient
	findings, err := c.Scan(context.Background(), []byte("test"))
	if err != nil {
		t.Fatalf("nil Scan should return nil error: %v", err)
	}
	if findings != nil {
		t.Fatalf("nil Scan should return nil findings, got %v", findings)
	}
}

// ── Close ────────────────────────────────────────────────────────────────────

func TestGRPCScannerClient_Close(t *testing.T) {
	c, cleanup := newTestScannerClient(t, &mockScannerServer{})
	defer cleanup()

	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if c.Enabled() {
		t.Fatal("client should not be enabled after close")
	}
}

func TestGRPCScannerClient_Close_NilClient(t *testing.T) {
	var c *GRPCScannerClient
	if err := c.Close(); err != nil {
		t.Fatalf("nil Close should return nil: %v", err)
	}
}

func TestGRPCScannerClient_Close_NilConnection(t *testing.T) {
	c := &GRPCScannerClient{conn: nil, stub: nil}
	if err := c.Close(); err != nil {
		t.Fatalf("Close with nil conn: %v", err)
	}
}

// grpcSinkName and grpcSinkEnabled prevent DCE in benchmarks.
var grpcSinkName string
var grpcSinkEnabled bool

//go:noinline
func grpcNameForBench(c *GRPCScannerClient, slot int) string {
	_ = slot
	return c.Name()
}

//go:noinline
func grpcEnabledForBench(c *GRPCScannerClient, slot int) bool {
	_ = slot
	return c.Enabled()
}

// ── Benchmarks ──

func BenchmarkGRPCScannerClient_Name(b *testing.B) {
	// Use a simple client without a real connection for benchmark.
	c := &GRPCScannerClient{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		grpcSinkName = grpcNameForBench(c, i)
	}
}

func BenchmarkGRPCScannerClient_Enabled(b *testing.B) {
	c := &GRPCScannerClient{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		grpcSinkEnabled = grpcEnabledForBench(c, i)
	}
}
