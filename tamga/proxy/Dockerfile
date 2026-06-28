FROM golang:1.25.4-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tamga ./cmd/tamga

FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1001 -s /sbin/nologin tamga

WORKDIR /app

COPY --from=builder /build/tamga .
COPY --from=builder /build/tamga-policy.yaml .

RUN chown -R tamga:tamga /app

USER tamga

EXPOSE 8443

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8443/api/v1/health/detailed || exit 1

ENTRYPOINT ["./tamga"]
