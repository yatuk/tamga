# Stage 1 — build the binary.
FROM golang:1.25.4-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o scanner-service ./cmd/scanner-service

# Stage 2 — minimal runtime.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget
RUN adduser -D -u 1001 tamga
COPY --from=builder /build/scanner-service /usr/local/bin/scanner-service
RUN GRPC_HEALTH_PROBE_VERSION=v0.4.24 && \
    wget -q -O /bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe
WORKDIR /app
USER tamga
EXPOSE 50052
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
    CMD ["grpc_health_probe", "-addr=localhost:50052"]
ENTRYPOINT ["scanner-service"]
