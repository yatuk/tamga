// Package grpcauth authenticates the proxy to its internal gRPC services:
// the analyzer, the classifier and the scanner service.
//
// Each service has a token of its own, a shared secret the operator
// generates (openssl rand -hex 32) and gives to the service and to the
// proxy. The proxy sends it with every call as "authorization: Bearer
// <token>"; a service with a token refuses a call without the same one with
// UNAUTHENTICATED. A service without a token accepts every call, as before.
// TLS, with or without client certificates, is optional on top.
//
// The standard health service (grpc.health.v1.Health) answers without a
// token, so a container probe needs none. It says only whether the service
// is up.
package grpcauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RequireEnv names the setting that makes a token mandatory. With it on, a
// service refuses to start without its token, and the proxy refuses to start
// when it has an address for a service but no token for it.
const RequireEnv = "TAMGA_REQUIRE_SERVICE_TOKENS"

const (
	metadataKey  = "authorization"
	bearerPrefix = "Bearer "
	healthPrefix = "/grpc.health.v1.Health/"
)

// Required reports whether RequireEnv is on in the environment.
func Required() bool {
	return truthy(os.Getenv(RequireEnv))
}

func truthy(v string) bool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	}
	return false
}

// CheckToken returns an error when tokens are required and the one named
// env is empty. The message says how to make one.
func CheckToken(env, token string, required bool) error {
	if required && strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s is empty and %s is on: generate a token with `openssl rand -hex 32` and give the same value to the service and the proxy", env, RequireEnv)
	}
	return nil
}

// ── client ────────────────────────────────────────────────────────────────

// ClientTLS is the proxy's TLS setting for its connections to the services.
// All empty means plain connections. CAFile alone verifies the services'
// certificates against that CA; CertFile and KeyFile add a client
// certificate, for a service that requires one.
type ClientTLS struct {
	CAFile   string
	CertFile string
	KeyFile  string
}

// Enabled reports whether any TLS setting is given.
func (c ClientTLS) Enabled() bool {
	return c.CAFile != "" || c.CertFile != "" || c.KeyFile != ""
}

// DialOptions returns the transport credentials and the per-call token for a
// connection to one service. They are meant to be given after a client's own
// options, which use a plain connection: a later transport option replaces
// an earlier one.
func DialOptions(token string, tlsCfg ClientTLS) ([]grpc.DialOption, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if tlsCfg.Enabled() {
		creds, err := tlsCfg.credentials()
		if err != nil {
			return nil, err
		}
		opts[0] = grpc.WithTransportCredentials(creds)
	}
	if token = strings.TrimSpace(token); token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(tokenCredentials{header: bearerPrefix + token, secure: tlsCfg.Enabled()}))
	}
	return opts, nil
}

func (c ClientTLS) credentials() (credentials.TransportCredentials, error) {
	if (c.CertFile == "") != (c.KeyFile == "") {
		return nil, errors.New("service TLS: TAMGA_SERVICE_TLS_CERT and TAMGA_SERVICE_TLS_KEY must be set together")
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		pool, err := readPool(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("service TLS: CA %s: %w", c.CAFile, err)
		}
		cfg.RootCAs = pool
	}
	if c.CertFile != "" {
		pair, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("service TLS: client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	}
	return credentials.NewTLS(cfg), nil
}

// tokenCredentials puts the token on every call. It does not insist on TLS:
// on the Compose network the connection is plain unless TLS is configured.
type tokenCredentials struct {
	header string
	secure bool
}

func (t tokenCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{metadataKey: t.header}, nil
}

func (t tokenCredentials) RequireTransportSecurity() bool { return t.secure }

// ── server ────────────────────────────────────────────────────────────────

// UnaryServerInterceptor refuses unary calls without the token. An empty
// token lets every call through.
func UnaryServerInterceptor(token string) grpc.UnaryServerInterceptor {
	want := digest(token)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, want, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamServerInterceptor is UnaryServerInterceptor for streaming calls.
func StreamServerInterceptor(token string) grpc.StreamServerInterceptor {
	want := digest(token)
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := authorize(ss.Context(), want, info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

var errUnauthenticated = status.Error(codes.Unauthenticated, "missing or wrong service token")

// digest is nil for an empty token, which turns the check off.
func digest(token string) []byte {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// authorize compares hashes of the two tokens, so the comparison takes the
// same time whatever the length of what was sent.
func authorize(ctx context.Context, want []byte, method string) error {
	if want == nil || strings.HasPrefix(method, healthPrefix) {
		return nil
	}
	md, _ := metadata.FromIncomingContext(ctx)
	for _, v := range md.Get(metadataKey) {
		presented, ok := strings.CutPrefix(v, bearerPrefix)
		if !ok {
			continue
		}
		got := sha256.Sum256([]byte(strings.TrimSpace(presented)))
		if subtle.ConstantTimeCompare(got[:], want) == 1 {
			return nil
		}
	}
	return errUnauthenticated
}

// ServerCredentials returns TLS credentials for a service's listener, or nil
// when certFile and keyFile are both empty. With clientCAFile set, a client
// must present a certificate signed by that CA.
func ServerCredentials(certFile, keyFile, clientCAFile string) (credentials.TransportCredentials, error) {
	if certFile == "" && keyFile == "" {
		if clientCAFile != "" {
			return nil, errors.New("gRPC TLS: TAMGA_GRPC_TLS_CLIENT_CA needs TAMGA_GRPC_TLS_CERT and TAMGA_GRPC_TLS_KEY")
		}
		return nil, nil
	}
	if certFile == "" || keyFile == "" {
		return nil, errors.New("gRPC TLS: TAMGA_GRPC_TLS_CERT and TAMGA_GRPC_TLS_KEY must be set together")
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("gRPC TLS: %w", err)
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	if clientCAFile != "" {
		pool, err := readPool(clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("gRPC TLS: client CA %s: %w", clientCAFile, err)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return credentials.NewTLS(cfg), nil
}

func readPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("no certificate in the file")
	}
	return pool, nil
}
