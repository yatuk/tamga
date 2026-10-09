package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/apikeys"
	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/store"
	"github.com/yatuk/tamga/internal/upstream"
)

// healthTestKey is the admin key of the health tests that read the full body.
const healthTestKey = "health-test-admin-key"

// healthGet reads a health URL as the holder of healthTestKey.
func healthGet(url string) (*http.Response, error) {
	return healthGetAs(url, healthTestKey)
}

func healthGetAs(url, key string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("X-Tamga-Admin-Key", key)
	}
	return http.DefaultClient.Do(req)
}

// What an anonymous caller may see, and nothing else.
var (
	publicDetailedKeys = []string{
		"authenticated", "proxy", "proxy_status",
		"scan_latency_ms_p50", "scan_latency_ms_p95", "scan_latency_ms_p99", "status",
	}
	publicDetailKeys = []string{"authenticated", "proxy"}
)

// Values placed in the configuration that no anonymous answer may contain.
const (
	sentinelPool     = "sentinel-pool-4c1d"
	sentinelEndpoint = "sentinel-endpoint-9e2b"
	sentinelPath     = "/srv/sentinel-dir-73aa/policy.yaml"
	sentinelPolicy   = "sentinel-policy-name-51f0"
	sentinelAllow    = "203.0.113.77/32"
	sentinelVersion  = "v9.9.9-sentinel-build"
	sentinelTrace    = "https://sentinel-trace.example"
)

var loopbackRules = &policy.UpstreamRules{AllowNets: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}

// closedAddr is a loopback address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// failedRegistry is a registry with one pool of one endpoint, used once so
// that the endpoint has a recorded failure.
func failedRegistry(t *testing.T, route, name, baseURL, timeout string, eg *upstream.Egress) *upstream.Registry {
	t.Helper()
	pol := &policy.Policy{Name: "t", Providers: &policy.Providers{Pools: map[string]policy.ProviderUpstreamPool{
		route: {Endpoints: []policy.ProviderUpstreamEndpoint{{Name: name, BaseURL: baseURL, Priority: 1, Timeout: timeout}}},
	}}}
	reg := upstream.NewRegistry(upstream.Options{GetPolicy: func() *policy.Policy { return pol }, Egress: eg})
	pool := reg.Pool(route)
	if pool == nil {
		t.Fatalf("no pool was built for %s", baseURL)
	}
	req := httptest.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", strings.NewReader(`{}`))
	resp, err := pool.RoundTrip(context.Background(), req, []byte(`{}`), 0, upstream.Hooks{})
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("the request to %s succeeded; the test needs it to fail", baseURL)
	}
	return reg
}

type healthFixture struct {
	url      string
	deadAddr string
	keys     map[string]string // scope, or "revoked", to raw key
}

// newHealthFixture serves a proxy whose configuration is made of sentinels,
// with one pool endpoint that refused a connection.
func newHealthFixture(t *testing.T) healthFixture {
	t.Helper()
	dead := closedAddr(t)
	reg := failedRegistry(t, sentinelPool, sentinelEndpoint, "http://"+dead, "2s", upstream.NewEgress(loopbackRules))

	keys := apikeys.NewMemoryStore()
	raw := map[string]string{}
	for _, scope := range []string{apikeys.ScopeRead, apikeys.ScopeWrite, apikeys.ScopeAdmin, apikeys.ScopeProxy} {
		ck, err := keys.Create("health "+scope, scope)
		if err != nil {
			t.Fatal(err)
		}
		raw[scope] = ck.RawKey
	}
	gone, err := keys.Create("revoked", apikeys.ScopeAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := keys.Delete(gone.Key.ID); err != nil {
		t.Fatal(err)
	}
	raw["revoked"] = gone.RawKey

	recent := events.NewRecentBuffer(10)
	recent.Add(events.Event{RequestID: "r1", EventType: "request_scanned", ScanLatencyMs: 3.5})

	cfg := Config{
		AdminKey:     healthTestKey,
		APIKeys:      keys,
		Started:      time.Now().Add(-time.Hour),
		PolicyPath:   sentinelPath,
		PolicyStore:  policy.NewPolicyStore(&policy.Policy{Name: sentinelPolicy}),
		IPAllowlist:  sentinelAllow,
		Version:      sentinelVersion,
		TraceUIURL:   sentinelTrace,
		TLSEnabled:   true,
		MTLSEnabled:  true,
		RedisEnabled: true,
		ScannerCount: 7,
		Store:        store.NewNoopStoreSilent(),
		Recent:       recent,
		Upstream:     reg,
	}
	ts := httptest.NewServer(testMux(cfg))
	t.Cleanup(ts.Close)
	return healthFixture{url: ts.URL, deadAddr: dead, keys: raw}
}

// readHealth returns the status, the raw body and its keys, sorted.
func readHealth(t *testing.T, resp *http.Response, err error) (int, string, map[string]interface{}, []string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, b)
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return resp.StatusCode, string(b), body, keys
}

func sameKeys(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
}

// assertPublic fails unless the answer is the reduced body and carries
// nothing of the configuration.
func assertPublic(t *testing.T, f healthFixture, path string, resp *http.Response, err error) {
	t.Helper()
	want := publicDetailedKeys
	if path == "/api/v1/health/detail" {
		want = publicDetailKeys
	}
	status, raw, body, keys := readHealth(t, resp, err)
	if status != http.StatusOK {
		t.Fatalf("%s: status %d, want 200: an uptime check must not fail on a bad credential", path, status)
	}
	if !sameKeys(keys, want) {
		t.Fatalf("%s: keys %v, want exactly %v", path, keys, want)
	}
	if body["authenticated"] != false || body["proxy"] != "up" {
		t.Fatalf("%s: authenticated=%v proxy=%v", path, body["authenticated"], body["proxy"])
	}
	host, port, _ := net.SplitHostPort(f.deadAddr)
	for _, s := range []string{
		sentinelPool, sentinelEndpoint, sentinelPath, "sentinel-dir", sentinelPolicy, sentinelAllow, "203.0.113",
		sentinelVersion, sentinelTrace, f.deadAddr, host, port, "dial", "refused", "tcp",
	} {
		if strings.Contains(raw, s) {
			t.Fatalf("%s: the anonymous body contains %q: %s", path, s, raw)
		}
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("%s: Cache-Control %q, want no-store", path, cc)
	}
}

var healthPaths = []string{"/api/v1/health/detailed", "/api/v1/health/detail"}

func TestHealth_AnonymousGetsOnlyThePublicFields(t *testing.T) {
	f := newHealthFixture(t)
	for _, path := range healthPaths {
		resp, err := http.Get(f.url + path)
		assertPublic(t, f, path, resp, err)
	}
}

func TestHealth_ManagementCredentialGetsTheFullBody(t *testing.T) {
	f := newHealthFixture(t)
	for name, key := range map[string]string{
		"admin key":       healthTestKey,
		"read-scope key":  f.keys[apikeys.ScopeRead],
		"write-scope key": f.keys[apikeys.ScopeWrite],
		"admin-scope key": f.keys[apikeys.ScopeAdmin],
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := healthGetAs(f.url+"/api/v1/health/detailed", key)
			status, raw, body, _ := readHealth(t, resp, err)
			if status != http.StatusOK || body["authenticated"] != true {
				t.Fatalf("detailed: status %d, body %s", status, raw)
			}
			for _, k := range []string{
				"proxy", "proxy_status", "status", "database", "redis", "analyzer", "scanner_count",
				"uptime_seconds", "policy_path", "events_dropped", "providers", "scan_latency_ms_p50",
			} {
				if _, ok := body[k]; !ok {
					t.Fatalf("detailed: %q is missing: %s", k, raw)
				}
			}
			if body["policy_path"] != sentinelPath || !strings.Contains(raw, sentinelPool) || !strings.Contains(raw, sentinelEndpoint) {
				t.Fatalf("detailed: not the full body: %s", raw)
			}
			// The pool and the endpoint are named; the address is not.
			if !strings.Contains(raw, `"failure_reason":"`+upstream.FailRefused+`"`) || strings.Contains(raw, f.deadAddr) {
				t.Fatalf("detailed: failure_reason: %s", raw)
			}

			resp, err = healthGetAs(f.url+"/api/v1/health/detail", key)
			status, raw, body, _ = readHealth(t, resp, err)
			if status != http.StatusOK || body["authenticated"] != true {
				t.Fatalf("detail: status %d, body %s", status, raw)
			}
			for k, want := range map[string]interface{}{
				"version": sentinelVersion, "policy_name": sentinelPolicy, "policy_path": sentinelPath,
				"ip_allowlist": sentinelAllow, "trace_ui_url": sentinelTrace,
				"tls_enabled": true, "mtls_enabled": true, "redis_enabled": true,
			} {
				if body[k] != want {
					t.Fatalf("detail: %s = %v, want %v", k, body[k], want)
				}
			}
			for _, k := range []string{"policy_writable", "scanner_count", "database", "uptime_seconds", "timestamp"} {
				if _, ok := body[k]; !ok {
					t.Fatalf("detail: %q is missing: %s", k, raw)
				}
			}
		})
	}
}

func TestHealth_OtherCredentialsGetThePublicBody(t *testing.T) {
	f := newHealthFixture(t)
	for _, path := range healthPaths {
		for name, key := range map[string]string{
			"proxy-scope key": f.keys[apikeys.ScopeProxy],
			"revoked key":     f.keys["revoked"],
			"wrong key":       "not-" + healthTestKey,
		} {
			t.Run(name+" "+path, func(t *testing.T) {
				resp, err := healthGetAs(f.url+path, key)
				assertPublic(t, f, path, resp, err)
			})
		}
		// The management API takes a key in the query string for
		// EventSource. Health does not: only a header opens the full body.
		for name, key := range map[string]string{
			"admin key in ?key=":      healthTestKey,
			"read-scope key in ?key=": f.keys[apikeys.ScopeRead],
		} {
			t.Run(name+" "+path, func(t *testing.T) {
				resp, err := http.Get(f.url + path + "?key=" + key)
				assertPublic(t, f, path, resp, err)
			})
		}
	}
}

// Dev mode opens the whole management API without a key, and health with it.
func TestHealth_DevModeFollowsTheManagementAPI(t *testing.T) {
	ts := httptest.NewServer(testMux(Config{DevMode: true, PolicyPath: sentinelPath, Store: store.NewNoopStoreSilent()}))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	_, _, body, _ := readHealth(t, resp, err)
	if body["authenticated"] != true || body["policy_path"] != sentinelPath {
		t.Fatalf("dev mode: %v", body)
	}
}

// A failing dependency turns the anonymous answer into 503 and "degraded",
// without saying which dependency it is.
func TestHealth_AnonymousDegradedNamesNoDependency(t *testing.T) {
	cfg := Config{
		AdminKey:     healthTestKey,
		Store:        store.NewNoopStoreSilent(),
		AnalyzerPing: func(context.Context) error { return context.DeadlineExceeded },
	}
	ts := httptest.NewServer(testMux(cfg))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/health/detailed")
	status, raw, body, keys := readHealth(t, resp, err)
	if status != http.StatusServiceUnavailable || body["status"] != "degraded" {
		t.Fatalf("status %d, body %s", status, raw)
	}
	if want := []string{"authenticated", "proxy", "proxy_status", "status"}; !sameKeys(keys, want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
	if strings.Contains(raw, "analyzer") || strings.Contains(raw, "unreachable") {
		t.Fatalf("the anonymous body names the dependency: %s", raw)
	}

	resp, err = healthGet(ts.URL + "/api/v1/health/detailed")
	status, _, body, _ = readHealth(t, resp, err)
	if status != http.StatusServiceUnavailable || body["analyzer"] != "unreachable" || body["status"] != "degraded" {
		t.Fatalf("with the key: status %d, body %v", status, body)
	}
}

// failureReason reads the one endpoint's failure_reason as the admin sees it.
func failureReason(t *testing.T, reg *upstream.Registry) (string, string) {
	t.Helper()
	ts := httptest.NewServer(testMux(Config{AdminKey: healthTestKey, Store: store.NewNoopStoreSilent(), Upstream: reg}))
	defer ts.Close()
	resp, err := healthGet(ts.URL + "/api/v1/health/detailed")
	_, raw, body, _ := readHealth(t, resp, err)
	pools, _ := body["providers"].([]interface{})
	if len(pools) != 1 {
		t.Fatalf("providers: %s", raw)
	}
	eps, _ := pools[0].(map[string]interface{})["providers"].([]interface{})
	if len(eps) != 1 {
		t.Fatalf("endpoints: %s", raw)
	}
	reason, _ := eps[0].(map[string]interface{})["failure_reason"].(string)
	return reason, raw
}

type failingResolver struct{}

func (failingResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

type inwardResolver struct{}

func (inwardResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("10.0.0.5")}, nil
}

func TestHealth_FailureReasonIsAFixedClass(t *testing.T) {
	loop := upstream.NewEgress(loopbackRules)

	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal detail from the upstream"))
	}))
	t.Cleanup(failing.Close)
	// A certificate nothing trusts; the handshake error it logs is expected.
	selfSigned := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	selfSigned.Config.ErrorLog = log.New(io.Discard, "", 0)
	selfSigned.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	selfSigned.StartTLS()
	t.Cleanup(selfSigned.Close)
	dead := "http://" + closedAddr(t)

	for _, tc := range []struct {
		name, want, route, base, timeout string
		eg                               *upstream.Egress
	}{
		{"refused dial", upstream.FailRefused, "openai", dead, "2s", loop},
		{"timeout", upstream.FailTimeout, "openai", slow.URL, "150ms", loop},
		{"dns failure", upstream.FailDNS, "openai", "https://api.openai.com", "2s", upstream.NewEgressWith(nil, failingResolver{}, nil)},
		{"tls failure", upstream.FailTLS, "openai", selfSigned.URL, "2s", loop},
		{"500", upstream.FailUpstream5xx, "openai", failing.URL, "2s", loop},
		{"refused by the rules", upstream.FailNotAllowed, "openai", failing.URL, "2s", upstream.NewEgress(nil)},
		{"refused at connect", upstream.FailNotAllowed, "openai", "https://api.openai.com", "2s", upstream.NewEgressWith(nil, inwardResolver{}, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := failedRegistry(t, tc.route, "ep", tc.base, tc.timeout, tc.eg)
			got, raw := failureReason(t, reg)
			if got != tc.want {
				t.Fatalf("failure_reason %q, want %q", got, tc.want)
			}
			host := strings.TrimPrefix(strings.TrimPrefix(tc.base, "https://"), "http://")
			for _, s := range []string{host, "127.0.0.1", "10.0.0.5", "dial", "tcp", "x509", "certificate", "internal detail", "no such host"} {
				if strings.Contains(raw, s) {
					t.Fatalf("the body contains %q: %s", s, raw)
				}
			}
		})
	}
}
