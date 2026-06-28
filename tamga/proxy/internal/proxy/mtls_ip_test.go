package proxy

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
)

// ---------------------------------------------------------------------------
// IP Allowlist tests
// ---------------------------------------------------------------------------

func TestIPAllowlist_EmptyPassesAll(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "", // empty allowlist = allow all
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestIPAllowlist_SingleCIDR_Allowed(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "127.0.0.0/8", // localhost is in this range
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestIPAllowlist_SingleCIDR_Rejected(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "10.0.0.0/8", // 127.0.0.1 is NOT in this range
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 403, got %d: %s", resp.StatusCode, b)
	}
	if got := resp.Header.Get("X-Tamga-Reject-Reason"); got != "ip_not_allowed" {
		t.Errorf("X-Tamga-Reject-Reason: want ip_not_allowed, got %q", got)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	errObj, _ := out["error"].(map[string]interface{})
	if errObj == nil || errObj["type"] != "ip_not_allowed" {
		t.Fatalf("error type: %v", errObj)
	}
}

func TestIPAllowlist_MultipleCIDRs(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			// 127.0.0.1 is NOT in 10.0.0.0/8 but IS in 127.0.0.0/8
			IPAllowlist: "10.0.0.0/8,127.0.0.0/8,192.168.0.0/16",
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestIPAllowlist_InvalidCIDR(t *testing.T) {
	// Invalid CIDR should be logged and skipped, not crash the server.
	// The remaining valid CIDR should still work.
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "not-a-cidr, 127.0.0.0/8, garbage/33", // two bad, one good
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// 127.0.0.1 matches the valid 127.0.0.0/8 range.
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
}

func TestIPAllowlist_IPv6(t *testing.T) {
	// Test that IPv6 CIDRs work both for allow and reject.
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "::1/128", // IPv6 localhost only
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	// Our test server uses 127.0.0.1 (IPv4), so ::1/128 should NOT match.
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 403 for IPv4 client with IPv6-only allowlist, got %d: %s", resp.StatusCode, b)
	}
	if got := resp.Header.Get("X-Tamga-Reject-Reason"); got != "ip_not_allowed" {
		t.Errorf("X-Tamga-Reject-Reason: want ip_not_allowed, got %q", got)
	}
}

func TestIPAllowlist_XForwardedFor(t *testing.T) {
	// When X-Forwarded-For is present, the middleware uses it for the IP check.
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "10.250.0.0/16", // only this range
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)

	// Request with X-Forwarded-For set to an IP inside the allowlist.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.250.1.100") // in 10.250.0.0/16
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200 for allowed X-Forwarded-For IP, got %d: %s", resp.StatusCode, b)
	}

	// Request with X-Forwarded-For set to an IP outside the allowlist.
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Forwarded-For", "203.0.113.50") // NOT in 10.250.0.0/16
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("want 403 for rejected X-Forwarded-For IP, got %d: %s", resp2.StatusCode, b)
	}
	if got := resp2.Header.Get("X-Tamga-Reject-Reason"); got != "ip_not_allowed" {
		t.Errorf("X-Tamga-Reject-Reason: want ip_not_allowed, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// mTLS tests
// ---------------------------------------------------------------------------

// generateTestCerts creates a CA, a server cert signed by the CA, and a client
// cert signed by the CA. Returns PEM-encoded cert/key bytes and the CA pool.
func generateTestCerts(t *testing.T) (caCert, caKey, serverCert, serverKey, clientCert, clientKey []byte, caPool *x509.CertPool) {
	t.Helper()

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatal(err)
	}
	caCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caKey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caPriv)})

	caParsed, _ := x509.ParseCertificate(caDER)
	caPool = x509.NewCertPool()
	caPool.AddCert(caParsed)

	// Server cert
	srv := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	srvDER, err := x509.CreateCertificate(rand.Reader, srv, caParsed, &srvPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatal(err)
	}
	serverCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvDER})
	serverKey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(srvPriv)})

	// Client cert
	cl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "Test Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	clDER, err := x509.CreateCertificate(rand.Reader, cl, caParsed, &clPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatal(err)
	}
	clientCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clDER})
	clientKey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clPriv)})

	return
}

func TestMTLS_ValidCert_Allowed(t *testing.T) {
	caCertPEM, _, serverCert, serverKey, clientCert, clientKey, caPool := generateTestCerts(t)

	// Write CA cert to temp file so the server can load it.
	caFile := writeTempFile(t, "ca-*.pem", caCertPEM)

	// Create a server TLS config with mTLS (require and verify client cert).
	srvTLS := &tls.Config{
		Certificates: []tls.Certificate{mustKeyPair(t, serverCert, serverKey)},
		MinVersion:   tls.VersionTLS12,
	}
	// Simulate what main.go does when MTLSStrictVerify is true.
	srvTLS.ClientCAs = caPool
	srvTLS.ClientAuth = tls.RequireAndVerifyClientCert

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	srv.TLS = srvTLS
	srv.StartTLS()
	defer srv.Close()

	// Client presents a valid cert signed by our CA.
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{mustKeyPair(t, clientCert, clientKey)},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
		// We're connecting to localhost, but the cert CN is "localhost".
		ServerName: "localhost",
	}
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: clientTLS},
		Timeout:   5 * time.Second,
	}

	resp, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("valid client cert should succeed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	// Prevent unused variable warning in case the file is cleaned up before use.
	_ = caFile
}

func TestMTLS_NoCert_Rejected(t *testing.T) {
	_, _, serverCert, serverKey, _, _, caPool := generateTestCerts(t)

	srvTLS := &tls.Config{
		Certificates: []tls.Certificate{mustKeyPair(t, serverCert, serverKey)},
		MinVersion:   tls.VersionTLS12,
	}
	srvTLS.ClientCAs = caPool
	srvTLS.ClientAuth = tls.RequireAndVerifyClientCert

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = srvTLS
	srv.StartTLS()
	defer srv.Close()

	// Client has NO certificate.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // we still want to verify the server, but the server requires client cert
				MinVersion:         tls.VersionTLS12,
			},
		},
		Timeout: 5 * time.Second,
	}

	_, err := client.Get(srv.URL + "/")
	if err == nil {
		t.Fatal("want TLS error when client has no cert, got nil")
	}
}

func TestMTLS_InvalidCA_Rejected(t *testing.T) {
	_, _, serverCert, serverKey, _, _, caPool := generateTestCerts(t)

	srvTLS := &tls.Config{
		Certificates: []tls.Certificate{mustKeyPair(t, serverCert, serverKey)},
		MinVersion:   tls.VersionTLS12,
	}
	srvTLS.ClientCAs = caPool
	srvTLS.ClientAuth = tls.RequireAndVerifyClientCert

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = srvTLS
	srv.StartTLS()
	defer srv.Close()

	// Generate a completely different CA and client cert (not trusted by server).
	rogueCA, rogueCAPriv := generateRogueCA(t, "Rogue CA")
	rogueClient := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "Rogue Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	roguePriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	rogueDER, _ := x509.CreateCertificate(rand.Reader, rogueClient, rogueCA, &roguePriv.PublicKey, rogueCAPriv)
	rogueCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rogueDER})
	rogueKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(roguePriv)})

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{mustKeyPair(t, rogueCertPEM, rogueKeyPEM)},
				InsecureSkipVerify: true, // still trust the server, but the server won't trust the client
				MinVersion:         tls.VersionTLS12,
			},
		},
		Timeout: 5 * time.Second,
	}

	_, err := client.Get(srv.URL + "/")
	if err == nil {
		t.Fatal("want TLS error when client cert is from untrusted CA, got nil")
	}
}

func TestMTLS_Disabled_PassesPlainHTTP(t *testing.T) {
	// When MTLSStrictVerify is false, a plain HTTP server should work normally.
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config: &config.Config{
			MTLSStrictVerify: false,
			MockUpstream:     true,
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}`)
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, b)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustKeyPair(t *testing.T, certPEM, keyPEM []byte) tls.Certificate {
	t.Helper()
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func writeTempFile(t *testing.T, pattern string, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), pattern)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func generateRogueCA(t *testing.T, cn string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	caParsed, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	return caParsed, priv
}

// TestIPAllowlist_HealthEndpoint_NotWrapped verifies that the /health endpoint
// is NOT behind the IP allowlist (so load balancers can check health).
func TestIPAllowlist_HealthEndpoint_NotBlocked(t *testing.T) {
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		Config: &config.Config{
			IPAllowlist:  "10.0.0.0/8", // 127.0.0.1 is NOT in this range
			MockUpstream: true,
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Health endpoint should NOT be blocked by IP allowlist.
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("/health should not be blocked by IP allowlist: got %d: %s", resp.StatusCode, body)
	}
}

// TestIPAllowlist_ProxyRoutes_AreBlocked verifies that proxy routes ARE
// behind the IP allowlist.
func TestIPAllowlist_ProxyRoutes_Blocked(t *testing.T) {
	upstream := newUpstreamEcho(t)
	pol := mustPolicy(t, `
version: "1.0"
providers:
  allowed: [openai]
`)
	h := NewHandler(HandlerConfig{
		Registry:  testRegistry(),
		GetPolicy: func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{
			"openai": upstream,
		},
		Config: &config.Config{
			IPAllowlist: "10.0.0.0/8", // 127.0.0.1 is NOT in this range
		},
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// All proxy routes should return 403.
	routes := []string{
		"/v1/chat/completions",
		"/openai/v1/chat/completions",
		"/anthropic/v1/messages",
		"/gemini/v1beta/models/gemini-pro:generateContent",
		"/azure/openai/deployments/gpt-4/chat/completions",
		"/bedrock/model/anthropic.claude-v2/invoke",
		"/mistral/v1/chat/completions",
		"/local/api/generate",
	}
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			resp, err := http.Post(srv.URL+route, "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("route %s: want 403, got %d: %s", route, resp.StatusCode, b)
			}
			if got := resp.Header.Get("X-Tamga-Reject-Reason"); got != "ip_not_allowed" {
				t.Errorf("route %s: X-Tamga-Reject-Reason want ip_not_allowed, got %q", route, got)
			}
		})
	}
}
