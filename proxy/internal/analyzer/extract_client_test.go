package analyzer

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/yatuk/tamga/proto/analyzer/v1"
)

type extractServer struct {
	pb.UnimplementedAnalyzerServiceServer
	got int
}

func (s *extractServer) Extract(_ context.Context, req *pb.ExtractRequest) (*pb.ExtractResponse, error) {
	s.got = len(req.GetData())
	// An answer larger than gRPC's default 4 MiB as well.
	return &pb.ExtractResponse{Status: "ok", Parts: []*pb.ExtractedPart{{Kind: "page", Index: 1, Text: strings.Repeat("a", 6<<20)}}}, nil
}

// A file over gRPC's default 4 MiB goes out, and an answer over it comes
// back, to a server that takes them as the analyzer's does.
func TestGRPCClient_Extract_LargeFile(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(extractMaxSendBytes), grpc.MaxSendMsgSize(extractMaxRecvBytes))
	impl := &extractServer{}
	pb.RegisterAnalyzerServiceServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := &GRPCClient{conn: conn, stub: pb.NewAnalyzerServiceClient(conn)}

	data := bytes.Repeat([]byte{1}, 20<<20)
	resp, err := c.Extract(context.Background(), &pb.ExtractRequest{Data: data})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if impl.got != len(data) || len(resp.GetParts()[0].GetText()) != 6<<20 {
		t.Fatalf("sent %d of %d bytes, got %d characters", impl.got, len(data), len(resp.GetParts()[0].GetText()))
	}
}

func TestGRPCClient_Extract_NotConnected(t *testing.T) {
	var c *GRPCClient
	if _, err := c.Extract(context.Background(), &pb.ExtractRequest{}); err == nil {
		t.Fatal("a nil client answered")
	}
}
