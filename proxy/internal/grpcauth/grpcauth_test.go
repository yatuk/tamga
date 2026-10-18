package grpcauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/yatuk/tamga/proto/scanner/v1"
)

const testToken = "test-service-token-not-a-secret"

// scanServer answers every Scan with no findings.
type scanServer struct {
	pb.UnimplementedScannerServiceServer
}

func (scanServer) Scan(context.Context, *pb.ScanRequest) (*pb.ScanResponse, error) {
	return &pb.ScanResponse{}, nil
}

// serve starts a scanner service and a health service on a loopback port,
// with the token interceptors and the given server options.
func serve(t *testing.T, token string, opts ...grpc.ServerOption) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	opts = append(opts,
		grpc.ChainUnaryInterceptor(UnaryServerInterceptor(token)),
		grpc.ChainStreamInterceptor(StreamServerInterceptor(token)),
	)
	srv := grpc.NewServer(opts...)
	pb.RegisterScannerServiceServer(srv, scanServer{})
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func dial(t *testing.T, addr, token string, tlsCfg ClientTLS) *grpc.ClientConn {
	t.Helper()
	opts, err := DialOptions(token, tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func scan(conn *grpc.ClientConn) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pb.NewScannerServiceClient(conn).Scan(ctx, &pb.ScanRequest{Content: []byte("hello")})
	return err
}

func TestTokenIsRequiredWhenConfigured(t *testing.T) {
	addr := serve(t, testToken)
	cases := []struct {
		name  string
		token string
		want  codes.Code
	}{
		{"right token", testToken, codes.OK},
		{"no token", "", codes.Unauthenticated},
		{"wrong token", "test-other-token", codes.Unauthenticated},
		{"prefix of the token", testToken[:10], codes.Unauthenticated},
		{"token with more after it", testToken + "x", codes.Unauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := scan(dial(t, addr, tc.token, ClientTLS{}))
			if got := status.Code(err); got != tc.want {
				t.Fatalf("code %v, want %v (err %v)", got, tc.want, err)
			}
		})
	}
}

func TestNoTokenConfiguredAcceptsEveryCall(t *testing.T) {
	addr := serve(t, "")
	if err := scan(dial(t, addr, "", ClientTLS{})); err != nil {
		t.Fatalf("without a token on the server: %v", err)
	}
	// A client that has a token for a server without one still gets in.
	if err := scan(dial(t, addr, testToken, ClientTLS{})); err != nil {
		t.Fatalf("token sent to a server without one: %v", err)
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	addr := serve(t, testToken)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := healthpb.NewHealthClient(dial(t, addr, "", ClientTLS{})).Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health without a token: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("status %v", resp.Status)
	}
}

func TestAuthorizeReadsTheBearerHeader(t *testing.T) {
	want := digest(testToken)
	cases := []struct {
		name   string
		values []string
		ok     bool
	}{
		{"bearer", []string{"Bearer " + testToken}, true},
		{"surrounding space", []string{"Bearer  " + testToken + " "}, true},
		{"second value", []string{"Bearer wrong", "Bearer " + testToken}, true},
		{"no scheme", []string{testToken}, false},
		{"other scheme", []string{"Basic " + testToken}, false},
		{"absent", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := metadata.MD{}
			for _, v := range tc.values {
				md.Append(metadataKey, v)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)
			err := authorize(ctx, want, "/proto.scanner.v1.ScannerService/Scan")
			if (err == nil) != tc.ok {
				t.Fatalf("authorize: %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestStreamInterceptorRefusesWithoutToken(t *testing.T) {
	icpt := StreamServerInterceptor(testToken)
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }
	err := icpt(nil, fakeStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: "/proto.scanner.v1.ScannerService/Watch"}, handler)
	if status.Code(err) != codes.Unauthenticated || called {
		t.Fatalf("err %v, handler called %v", err, called)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(metadataKey, "Bearer "+testToken))
	if err := icpt(nil, fakeStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/x/Y"}, handler); err != nil || !called {
		t.Fatalf("with the token: err %v, handler called %v", err, called)
	}
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeStream) Context() context.Context { return f.ctx }

func TestCheckToken(t *testing.T) {
	if err := CheckToken("TAMGA_ANALYZER_TOKEN", "", true); err == nil || !strings.Contains(err.Error(), "TAMGA_ANALYZER_TOKEN") {
		t.Fatalf("required and empty: %v", err)
	}
	if err := CheckToken("TAMGA_ANALYZER_TOKEN", "  ", true); err == nil {
		t.Fatal("a token of spaces is empty")
	}
	if err := CheckToken("TAMGA_ANALYZER_TOKEN", "", false); err != nil {
		t.Fatalf("not required: %v", err)
	}
	if err := CheckToken("TAMGA_ANALYZER_TOKEN", testToken, true); err != nil {
		t.Fatalf("set: %v", err)
	}
}

func TestRequired(t *testing.T) {
	for v, want := range map[string]bool{"true": true, "1": true, "TRUE": true, "false": false, "": false, "0": false} {
		t.Setenv(RequireEnv, v)
		if got := Required(); got != want {
			t.Errorf("%s=%q: %v, want %v", RequireEnv, v, got, want)
		}
	}
}

// ── TLS ───────────────────────────────────────────────────────────────────

type pki struct {
	dir                             string
	ca, serverCert, serverKey       string
	clientCert, clientKey, otherCA  string
	otherClientCert, otherClientKey string
}

func TestMutualTLS(t *testing.T) {
	p := makePKI(t)
	creds, err := ServerCredentials(p.serverCert, p.serverKey, p.ca)
	if err != nil {
		t.Fatal(err)
	}
	addr := serve(t, testToken, grpc.Creds(creds))

	if err := scan(dial(t, addr, testToken, ClientTLS{CAFile: p.ca, CertFile: p.clientCert, KeyFile: p.clientKey})); err != nil {
		t.Fatalf("client certificate and token: %v", err)
	}
	if err := scan(dial(t, addr, "", ClientTLS{CAFile: p.ca, CertFile: p.clientCert, KeyFile: p.clientKey})); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("client certificate without the token: %v", err)
	}
	if err := scan(dial(t, addr, testToken, ClientTLS{CAFile: p.ca})); err == nil {
		t.Fatal("a client without a certificate got in")
	}
	if err := scan(dial(t, addr, testToken, ClientTLS{CAFile: p.ca, CertFile: p.otherClientCert, KeyFile: p.otherClientKey})); err == nil {
		t.Fatal("a certificate from another CA got in")
	}
	if err := scan(dial(t, addr, testToken, ClientTLS{})); err == nil {
		t.Fatal("a plain connection to a TLS listener got in")
	}
	if err := scan(dial(t, addr, testToken, ClientTLS{CAFile: p.otherCA, CertFile: p.clientCert, KeyFile: p.clientKey})); err == nil {
		t.Fatal("the client accepted a server certificate from a CA it does not trust")
	}
}

func TestServerTLSWithoutClientCertificates(t *testing.T) {
	p := makePKI(t)
	creds, err := ServerCredentials(p.serverCert, p.serverKey, "")
	if err != nil {
		t.Fatal(err)
	}
	addr := serve(t, testToken, grpc.Creds(creds))
	if err := scan(dial(t, addr, testToken, ClientTLS{CAFile: p.ca})); err != nil {
		t.Fatalf("server TLS, no client certificate: %v", err)
	}
}

func TestTLSSettingsMustBeComplete(t *testing.T) {
	p := makePKI(t)
	if _, err := ServerCredentials(p.serverCert, "", ""); err == nil {
		t.Error("certificate without key accepted")
	}
	if _, err := ServerCredentials("", "", p.ca); err == nil {
		t.Error("client CA without a server certificate accepted")
	}
	if c, err := ServerCredentials("", "", ""); c != nil || err != nil {
		t.Errorf("nothing set: %v, %v", c, err)
	}
	if _, err := DialOptions(testToken, ClientTLS{CertFile: p.clientCert}); err == nil {
		t.Error("client certificate without key accepted")
	}
	if _, err := DialOptions(testToken, ClientTLS{CAFile: filepath.Join(p.dir, "missing.pem")}); err == nil {
		t.Error("missing CA file accepted")
	}
	notPEM := filepath.Join(p.dir, "not.pem")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DialOptions(testToken, ClientTLS{CAFile: notPEM}); err == nil {
		t.Error("CA file without a certificate accepted")
	}
}

// makePKI writes a CA, a server certificate for 127.0.0.1 and a client
// certificate signed by it, and a second CA with a client certificate of its
// own.
func makePKI(t *testing.T) pki {
	t.Helper()
	dir := t.TempDir()
	p := pki{dir: dir}
	caCert, caKey := newCA(t, "test CA")
	p.ca = writeCert(t, dir, "ca.pem", caCert)
	srv, srvKey := newLeaf(t, caCert, caKey, "localhost", true)
	p.serverCert, p.serverKey = writeCert(t, dir, "server.pem", srv), writeKey(t, dir, "server-key.pem", srvKey)
	cli, cliKey := newLeaf(t, caCert, caKey, "tamga-proxy", false)
	p.clientCert, p.clientKey = writeCert(t, dir, "client.pem", cli), writeKey(t, dir, "client-key.pem", cliKey)

	otherCert, otherKey := newCA(t, "other CA")
	p.otherCA = writeCert(t, dir, "other-ca.pem", otherCert)
	oc, ocKey := newLeaf(t, otherCert, otherKey, "intruder", false)
	p.otherClientCert, p.otherClientKey = writeCert(t, dir, "other-client.pem", oc), writeKey(t, dir, "other-client-key.pem", ocKey)
	return p
}

var serial int64

func nextSerial() *big.Int { serial++; return big.NewInt(serial) }

func newCA(t *testing.T, name string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          nextSerial(),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func newLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, name string, server bool) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: nextSerial(),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if server {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		// The tests dial 127.0.0.1: resolving a name is slow on some hosts.
		tmpl.DNSNames = []string{name}
		tmpl.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func writeCert(t *testing.T, dir, name string, cert *x509.Certificate) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeKey(t *testing.T, dir, name string, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
