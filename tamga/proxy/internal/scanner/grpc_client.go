package scanner

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "github.com/yatuk/tamga/proto/scanner/v1"

	"github.com/yatuk/tamga/internal/telemetry"
)

// GRPCScannerClient is a gRPC-based Scanner implementation. It connects to
// a remote scanner-service and forwards Scan calls over gRPC.
type GRPCScannerClient struct {
	conn *grpc.ClientConn
	stub pb.ScannerServiceClient
}

// GRPCScannerConfig holds connection parameters for the scanner gRPC client.
type GRPCScannerConfig struct {
	Addr        string
	DialTimeout time.Duration
	CallTimeout time.Duration
}

// NewGRPCScannerClient connects to a scanner-service at addr. Returns nil if
// addr is empty (fail-open — caller falls back to local Registry).
func NewGRPCScannerClient(ctx context.Context, cfg GRPCScannerConfig) (*GRPCScannerClient, error) {
	if cfg.Addr == "" {
		return nil, nil
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 5 * time.Second
	}

	conn, err := grpc.NewClient(cfg.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		grpc.WithUnaryInterceptor(telemetry.UnaryClientTraceInterceptor()),
	)
	if err != nil {
		return nil, err
	}

	return &GRPCScannerClient{
		conn: conn,
		stub: pb.NewScannerServiceClient(conn),
	}, nil
}

// Enabled returns true when the gRPC connection is in a usable state.
func (c *GRPCScannerClient) Enabled() bool {
	if c == nil || c.conn == nil {
		return false
	}
	state := c.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// Name identifies this scanner in logs and metrics.
func (c *GRPCScannerClient) Name() string {
	return "grpc-scanner"
}

// Scan sends content to the remote scanner service and converts the proto
// response back to internal Findings. Implements the Scanner interface.
func (c *GRPCScannerClient) Scan(ctx context.Context, content []byte) ([]Finding, error) {
	if !c.Enabled() {
		return nil, nil
	}

	resp, err := c.stub.Scan(ctx, &pb.ScanRequest{Content: content})
	if err != nil {
		return nil, err
	}
	return ProtoToFindings(resp.Findings), nil
}

// Close shuts down the gRPC connection.
func (c *GRPCScannerClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
