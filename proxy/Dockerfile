# TODO: pin by digest (image:tag@sha256:...): docs/operations.md, "Image pins".
FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# What the binary reports as its version (banner, /api/v1/health/detail,
# traces). A release passes the tag and the commit:
#   docker build --build-arg VERSION=v0.9.0 --build-arg REVISION=$(git rev-parse HEAD) proxy
# Without VERSION the binary says "dev", or "dev+<commit>" when REVISION is
# given: the build context has no .git for the Go tool to read it from.
ARG VERSION=""
ARG REVISION=""
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X github.com/yatuk/tamga/internal/buildinfo.Version=${VERSION} -X github.com/yatuk/tamga/internal/buildinfo.Revision=${REVISION}" \
    -o tamga ./cmd/tamga

# TODO: pin by digest (image:tag@sha256:...): docs/operations.md, "Image pins".
FROM alpine:3.24

ARG VERSION=""
ARG REVISION=""
LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
LABEL org.opencontainers.image.title="Tamga proxy" \
      org.opencontainers.image.description="Self-hosted LLM security proxy: inline scanning, policy and audit for LLM traffic" \
      org.opencontainers.image.source="https://github.com/yatuk/tamga" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1001 -s /sbin/nologin tamga

WORKDIR /app

COPY --from=builder /build/tamga .
COPY --from=builder /build/tamga-policy.yaml .

RUN chown -R tamga:tamga /app

USER tamga

EXPOSE 8443

# Liveness only: the detailed health answers 503 when the database, Redis or the
# analyzer is down, and a proxy that is up without one of them is not a dead container.
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8443/health || exit 1

ENTRYPOINT ["./tamga"]
