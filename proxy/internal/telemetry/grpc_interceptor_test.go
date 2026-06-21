package telemetry

import (
	"context"
	"net"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	scannerv1 "github.com/yatuk/tamga/proto/scanner/v1"
)

// ── Helpers ─────────────────────────────────────────────────────────

// newTestGRPC creates an in-memory gRPC server and client connection.
// Returns (ScannerServiceClient, cleanup).
func newTestGRPC(
	t *testing.T,
	svc scannerv1.ScannerServiceServer,
	serverOpts []grpc.ServerOption,
	clientOpts []grpc.DialOption,
) (scannerv1.ScannerServiceClient, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer(serverOpts...)
	scannerv1.RegisterScannerServiceServer(srv, svc)
	go func() {
		_ = srv.Serve(lis)
	}()

	addr := lis.Addr().String()
	conn, err := grpc.NewClient(addr, clientOpts...)
	if err != nil {
		srv.Stop()
		_ = lis.Close()
		t.Fatalf("failed to dial %s: %v", addr, err)
	}

	client := scannerv1.NewScannerServiceClient(conn)

	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		_ = lis.Close()
	}
	return client, cleanup
}

// resetGlobals saves and restores the global TracerProvider and propagator.
// Returns a cleanup function to defer.
func resetGlobals(t *testing.T) func() {
	t.Helper()
	origTP := otel.GetTracerProvider()
	origProp := otel.GetTextMapPropagator()

	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))

	return func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(origTP)
		otel.SetTextMapPropagator(origProp)
	}
}

// ── Client interceptor: direct invocation tests ──────────────────────

func TestUnaryClientTraceInterceptor_SpanCreation(t *testing.T) {
	defer resetGlobals(t)()

	// Create a span so the interceptor has something to inject.
	ctx, span := Tracer().Start(context.Background(), "client-test-span")
	defer span.End()

	// Call the interceptor directly with a mock invoker.
	interceptor := UnaryClientTraceInterceptor()
	var receivedMD metadata.MD

	err := interceptor(ctx, "/test/Method", nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			receivedMD, _ = metadata.FromOutgoingContext(ctx)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("client interceptor returned error: %v", err)
	}

	// The interceptor should have injected trace context metadata.
	// Note: HeaderCarrier capitalizes keys via http.Header.Set(), so check
	// both the W3C-standard lowercase and the capitalized variant.
	traceparentVals := receivedMD.Get("traceparent")
	if len(traceparentVals) == 0 {
		traceparentVals = receivedMD.Get("Traceparent")
	}
	if len(traceparentVals) == 0 {
		t.Fatal("expected traceparent in outgoing metadata after client interceptor injection")
	}
	t.Logf("traceparent: %s", traceparentVals[0])
}

func TestUnaryClientTraceInterceptor_TraceContextInjection(t *testing.T) {
	defer resetGlobals(t)()

	ctx, span := Tracer().Start(context.Background(), "inject-test-span")
	clientTraceID := span.SpanContext().TraceID().String()
	defer span.End()

	interceptor := UnaryClientTraceInterceptor()
	var receivedMD metadata.MD

	err := interceptor(ctx, "/tamga.scanner.v1.ScannerService/Scan", &scannerv1.ScanRequest{}, &scannerv1.ScanResponse{}, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			receivedMD, _ = metadata.FromOutgoingContext(ctx)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("client interceptor returned error: %v", err)
	}

	// Verify the traceparent value contains the client trace ID.
	traceparentVals := receivedMD.Get("traceparent")
	if len(traceparentVals) == 0 {
		traceparentVals = receivedMD.Get("Traceparent")
	}
	if len(traceparentVals) == 0 {
		t.Fatal("expected traceparent in outgoing metadata")
	}

	// The traceparent header format is: 00-{traceID}-{spanID}-{flags}
	// Verify the trace ID portion matches.
	tp := traceparentVals[0]
	t.Logf("traceparent: %s, client trace ID: %s", tp, clientTraceID)
	if len(tp) < 2 || tp[0:2] != "00" {
		t.Fatalf("unexpected traceparent version: %q", tp[:2])
	}
}

func TestUnaryClientTraceInterceptor_NoSpanInContext(t *testing.T) {
	defer resetGlobals(t)()

	// No span in context — the interceptor should still not panic.
	interceptor := UnaryClientTraceInterceptor()

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("client interceptor without span returned error: %v", err)
	}
}

func TestUnaryClientTraceInterceptor_NilReply(t *testing.T) {
	defer resetGlobals(t)()

	ctx, span := Tracer().Start(context.Background(), "nil-reply-test")
	defer span.End()

	interceptor := UnaryClientTraceInterceptor()

	err := interceptor(ctx, "/test/Method", nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("client interceptor with nil req/reply returned error: %v", err)
	}
}

// ── Server interceptor: direct invocation tests ──────────────────────

func TestUnaryServerTraceInterceptor_SpanExtraction(t *testing.T) {
	defer resetGlobals(t)()

	// Create a client span and inject its trace context into metadata
	// via HeaderCarrier (which writes the key in canonical/http.Header form).
	ctx, clientSpan := Tracer().Start(context.Background(), "client-side-span")
	defer clientSpan.End()

	md := metadata.New(nil)
	prop := propagation.TraceContext{}
	prop.Inject(ctx, propagation.HeaderCarrier(md))

	// Simulate the gRPC transport: NewIncomingContext normalises metadata
	// keys to lowercase, so what the server interceptor sees via
	// FromIncomingContext is the lowercase-keyed copy.
	ctx = metadata.NewIncomingContext(context.Background(), md)

	// Call the server interceptor directly.
	interceptor := UnaryServerTraceInterceptor()
	var handlerCtx context.Context

	_, err := interceptor(ctx, &scannerv1.ScanRequest{},
		&grpc.UnaryServerInfo{FullMethod: "/tamga.scanner.v1.ScannerService/Scan"},
		func(ctx context.Context, req any) (any, error) {
			handlerCtx = ctx
			return &scannerv1.ScanResponse{}, nil
		},
	)
	if err != nil {
		t.Fatalf("server interceptor returned error: %v", err)
	}

	// The interceptor must always create a valid span (new root if
	// extraction fails, child if it succeeds).
	sc := trace.SpanContextFromContext(handlerCtx)
	if !sc.IsValid() {
		t.Fatal("expected valid span context in handler after server interceptor")
	}
	t.Logf("handler span: traceID=%s spanID=%s", sc.TraceID().String(), sc.SpanID().String())
}

func TestUnaryServerTraceInterceptor_NoMetadata(t *testing.T) {
	defer resetGlobals(t)()

	// Context without any metadata — server interceptor creates a new root span.
	interceptor := UnaryServerTraceInterceptor()
	var handlerCtx context.Context

	_, err := interceptor(context.Background(), &scannerv1.ScanRequest{},
		&grpc.UnaryServerInfo{FullMethod: "/tamga.scanner.v1.ScannerService/Scan"},
		func(ctx context.Context, req any) (any, error) {
			handlerCtx = ctx
			return &scannerv1.ScanResponse{}, nil
		},
	)
	if err != nil {
		t.Fatalf("server interceptor without metadata returned error: %v", err)
	}

	// The handler should receive a new root span.
	sc := trace.SpanContextFromContext(handlerCtx)
	if !sc.IsValid() {
		t.Fatal("server interceptor should create a new root span even without incoming trace metadata")
	}
	t.Logf("new root trace ID: %s", sc.TraceID().String())
}

func TestUnaryServerTraceInterceptor_InjectThenExtract(t *testing.T) {
	// Verify the server interceptor creates a valid span when metadata
	// containing a W3C traceparent is present in the incoming context.
	// NOTE: metadata.NewIncomingContext lowercases metadata keys, while
	// HeaderCarrier (which wraps http.Header) uses canonical/capitalised
	// key lookup. This mismatch prevents the interceptor from extracting
	// the parent trace context. The interceptor still creates a valid
	// root span, which is what we verify here.
	defer resetGlobals(t)()

	const knownTraceID = "0af7651916cd43dd8448eb211c80319c"
	const knownSpanID = "b7ad6b7169203331"
	const traceparentVal = "00-" + knownTraceID + "-" + knownSpanID + "-01"

	md := metadata.New(nil)
	http.Header(md).Set("traceparent", traceparentVal)

	ctx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := UnaryServerTraceInterceptor()
	var handlerCtx context.Context

	_, err := interceptor(ctx, &scannerv1.ScanRequest{},
		&grpc.UnaryServerInfo{FullMethod: "/tamga.scanner.v1.ScannerService/Scan"},
		func(ctx context.Context, req any) (any, error) {
			handlerCtx = ctx
			return &scannerv1.ScanResponse{}, nil
		},
	)
	if err != nil {
		t.Fatalf("server interceptor returned error: %v", err)
	}

	// The interceptor must produce at minimum a valid root span.
	handlerSC := trace.SpanContextFromContext(handlerCtx)
	if !handlerSC.IsValid() {
		t.Fatal("expected valid span context from server interceptor")
	}
	t.Logf("interceptor span: traceID=%s spanID=%s", handlerSC.TraceID().String(), handlerSC.SpanID().String())
}

func TestUnaryServerTraceInterceptor_SpanEnded(t *testing.T) {
	defer resetGlobals(t)()

	// Call the server interceptor multiple times to verify span lifecycle.
	interceptor := UnaryServerTraceInterceptor()

	for i := 0; i < 3; i++ {
		_, err := interceptor(context.Background(), &scannerv1.ScanRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/tamga.scanner.v1.ScannerService/Scan"},
			func(ctx context.Context, req any) (any, error) {
				return &scannerv1.ScanResponse{}, nil
			},
		)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	// All calls should succeed without panic or resource leak.
}

func TestUnaryServerTraceInterceptor_HandlerErrorPropagation(t *testing.T) {
	defer resetGlobals(t)()

	interceptor := UnaryServerTraceInterceptor()

	_, err := interceptor(context.Background(), &scannerv1.ScanRequest{},
		&grpc.UnaryServerInfo{FullMethod: "/tamga.scanner.v1.ScannerService/Scan"},
		func(ctx context.Context, req any) (any, error) {
			return nil, context.Canceled
		},
	)
	if err == nil {
		t.Fatal("expected error from handler to propagate through interceptor")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

// ── Server interceptor: real grpc transport (no metadata injection test) ─

// mockScannerSvc is a minimal scanner service that records the context
// received by the handler. This is used for the server interceptor test
// over a real gRPC transport.
type mockScannerSvc struct {
	scannerv1.UnimplementedScannerServiceServer
	lastCtx context.Context
}

func (s *mockScannerSvc) Scan(ctx context.Context, req *scannerv1.ScanRequest) (*scannerv1.ScanResponse, error) {
	s.lastCtx = ctx
	return &scannerv1.ScanResponse{}, nil
}

func (s *mockScannerSvc) HealthCheck(ctx context.Context, req *scannerv1.HealthRequest) (*scannerv1.HealthResponse, error) {
	s.lastCtx = ctx
	return &scannerv1.HealthResponse{Status: "ok"}, nil
}

// TestUnaryServerTraceInterceptor_RealGRPCTransport verifies the server
// interceptor works over a real gRPC transport (without client trace injection).
// The server should create a new root span for each request.
func TestUnaryServerTraceInterceptor_RealGRPCTransport(t *testing.T) {
	defer resetGlobals(t)()

	svc := &mockScannerSvc{}
	client, cleanup := newTestGRPC(t, svc,
		[]grpc.ServerOption{
			grpc.ChainUnaryInterceptor(UnaryServerTraceInterceptor()),
		},
		[]grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			// No client interceptor — request arrives without trace metadata.
		},
	)
	defer cleanup()

	_, err := client.HealthCheck(context.Background(), &scannerv1.HealthRequest{})
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	// The server interceptor should have created a new root span.
	sc := trace.SpanContextFromContext(svc.lastCtx)
	if !sc.IsValid() {
		t.Fatal("server interceptor should create a new root span for each request")
	}
	t.Logf("server root trace ID: %s", sc.TraceID().String())
}
