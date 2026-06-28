package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientTraceInterceptor injects W3C trace context into outgoing gRPC
// metadata so the server can continue the distributed trace. Use this when
// dialing the scanner-service or analyzer gRPC endpoints.
func UnaryClientTraceInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		// Inject W3C Trace Context (traceparent, tracestate) into gRPC metadata.
		prop := propagation.TraceContext{}
		prop.Inject(ctx, propagation.HeaderCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryServerTraceInterceptor extracts W3C trace context from incoming gRPC
// metadata and creates a server span for the RPC. Use this on the scanner-service
// gRPC server to continue the trace started by the proxy.
func UnaryServerTraceInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		prop := propagation.TraceContext{}
		ctx = prop.Extract(ctx, propagation.HeaderCarrier(md))

		ctx, span := Tracer().Start(ctx, "grpc."+info.FullMethod)
		defer span.End()

		return handler(ctx, req)
	}
}
