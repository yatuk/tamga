package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	pb "github.com/yatuk/tamga/proto/scanner/v1"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// operatorGate is a request-bound scanner: it only has something to say when
// the proxy hands it the per-request context.
type operatorGate struct{}

func (operatorGate) Name() string { return "operator_gate" }
func (operatorGate) Scan(context.Context, []byte) ([]scanner.Finding, error) {
	return nil, nil
}
func (operatorGate) ScanWithContext(_ context.Context, _ []byte, rc *scanner.RequestContext) ([]scanner.Finding, error) {
	if rc == nil || rc.OperatorId != "intruder" {
		return nil, nil
	}
	return []scanner.Finding{{Type: "secret", Category: "aws_access_key", Severity: "critical", Confidence: 1}}, nil
}

const delegatedPolicy = `
version: "1.0"
rules:
  secret_detection:
    action: BLOCK
    sensitivity: low
custom_entities:
  - name: "musteri_no"
    pattern: "MN-\\d{8}"
    severity: "high"
    action: BLOCK
    confidence: 0.9
providers:
  allowed: [openai]
`

// delegatedProxy returns a proxy whose stateless scanning is delegated to a
// scanner-service that never finds anything, as the real one would not for a
// policy-defined entity or a request-header condition.
func delegatedProxy(t *testing.T, bound *scanner.Registry) *httptest.Server {
	t.Helper()
	remote, cleanup := newMockGRPCClient(t, func(context.Context, *pb.ScanRequest) (*pb.ScanResponse, error) {
		return &pb.ScanResponse{}, nil
	})
	t.Cleanup(cleanup)

	pol := mustPolicy(t, delegatedPolicy)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:           testRegistry(),
		ProxyBoundRegistry: bound,
		GetPolicy:          func() *policy.Policy { return pol },
		UpstreamURLs:       map[string]*url.URL{"openai": newUpstreamEcho(t)},
		Config:             &config.Config{},
		ScannerClient:      remote,
	}))
	t.Cleanup(srv.Close)
	return srv
}

func postDelegated(t *testing.T, srv *httptest.Server, content, operator string) int {
	t.Helper()
	body := []byte(`{"messages":[{"role":"user","content":"` + content + `"}]}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if operator != "" {
		req.Header.Set("X-Tamga-Operator-Id", operator)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func boundRegistry(pol *policy.Policy) *scanner.Registry {
	reg := scanner.NewRegistry()
	reg.Register(scanner.NewCustomScanner(func() []scanner.CustomEntitySpec {
		out := make([]scanner.CustomEntitySpec, 0, len(pol.CustomEntities))
		for _, ce := range pol.CustomEntities {
			out = append(out, scanner.CustomEntitySpec{
				Name: ce.Name, Pattern: ce.Pattern, Severity: ce.Severity, Confidence: ce.Confidence,
			})
		}
		return out
	}))
	reg.Register(operatorGate{})
	return reg
}

// Delegating the stateless scanners must not switch off the ones that depend
// on the proxy's policy: a custom entity is still enforced.
func TestDelegatedScan_CustomEntityStillEnforced(t *testing.T) {
	srv := delegatedProxy(t, boundRegistry(mustPolicy(t, delegatedPolicy)))

	if got := postDelegated(t, srv, "musteri MN-12345678 icin ozet", ""); got != http.StatusForbidden {
		t.Errorf("custom entity under delegation: got %d, want 403", got)
	}
	if got := postDelegated(t, srv, "sadece bir soru", ""); got != http.StatusOK {
		t.Errorf("benign request under delegation: got %d, want 200", got)
	}
}

// Request-bound scanners must still receive the per-request context.
func TestDelegatedScan_RequestContextReachesBoundScanners(t *testing.T) {
	srv := delegatedProxy(t, boundRegistry(mustPolicy(t, delegatedPolicy)))

	if got := postDelegated(t, srv, "devam et", "intruder"); got != http.StatusForbidden {
		t.Errorf("request-bound scanner under delegation: got %d, want 403", got)
	}
	if got := postDelegated(t, srv, "devam et", "mike"); got != http.StatusOK {
		t.Errorf("authorized operator under delegation: got %d, want 200", got)
	}
}

// Documents the failure this guards against: with no proxy-bound registry the
// delegated path sees nothing and the custom entity passes.
func TestDelegatedScan_WithoutBoundRegistryMissesCustomEntity(t *testing.T) {
	srv := delegatedProxy(t, nil)
	if got := postDelegated(t, srv, "musteri MN-12345678 icin ozet", ""); got != http.StatusOK {
		t.Fatalf("expected the unprotected delegated path to pass (200), got %d", got)
	}
}
