package analyzer

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	pb "github.com/yatuk/tamga/proto/analyzer/v1"
)

// defaultGRPCTimeout is the per-RPC deadline applied to Analyze calls.
const defaultGRPCTimeout = 8 * time.Second

// GRPCClient is a persistent, multiplexed gRPC client for the Python analyzer.
// It replaces the old HTTP/JSON client with a protobuf wire-format connection.
//
// The underlying gRPC connection is created once at startup and reused across
// all requests. gRPC handles connection pooling, health checking, automatic
// reconnection, and load balancing natively.
type GRPCClient struct {
	conn *grpc.ClientConn
	stub pb.AnalyzerServiceClient
}

// NewGRPCClient creates a gRPC client connected to addr (e.g. "analyzer:50051").
// The connection is established immediately; callers should treat dial errors
// as non-fatal (fail-open — the analyzer is optional).
//
// Keepalive parameters are tuned for container-to-container communication on
// an internal Docker bridge network (low latency, reliable).
func NewGRPCClient(_ context.Context, addr string) (*GRPCClient, error) {
	if addr == "" {
		return nil, nil
	}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second, // send pings every 30s
			Timeout:             5 * time.Second,  // wait 5s for ping ack
			PermitWithoutStream: true,             // send pings even without active RPCs
		}),
		grpc.WithDefaultServiceConfig(`{
			"loadBalancingPolicy": "round_robin",
			"healthCheckConfig": {
				"serviceName": "proto.analyzer.v1.AnalyzerService"
			}
		}`),
	)
	if err != nil {
		return nil, fmt.Errorf("gRPC dial %s: %w", addr, err)
	}

	return &GRPCClient{
		conn: conn,
		stub: pb.NewAnalyzerServiceClient(conn),
	}, nil
}

// Enabled reports whether the client is connected and ready for RPCs.
func (c *GRPCClient) Enabled() bool {
	if c == nil || c.conn == nil {
		return false
	}
	state := c.conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// Analyze calls the Python analyzer's Analyze RPC over gRPC.
// Callers MUST treat errors as non-fatal (fail-open): analyzer down must
// not block the proxy request path.
func (c *GRPCClient) Analyze(ctx context.Context, req *pb.AnalyzeRequest) (*pb.AnalyzeResponse, error) {
	if !c.Enabled() {
		return nil, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, defaultGRPCTimeout)
	defer cancel()

	resp, err := c.stub.Analyze(rpcCtx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC Analyze: %w", err)
	}
	return resp, nil
}

// HealthCheck calls the analyzer's HealthCheck RPC (used for observability).
func (c *GRPCClient) HealthCheck(ctx context.Context) (*pb.HealthCheckResponse, error) {
	if !c.Enabled() {
		return nil, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return c.stub.HealthCheck(rpcCtx, &pb.HealthCheckRequest{})
}

// Close shuts down the gRPC connection gracefully.
func (c *GRPCClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
