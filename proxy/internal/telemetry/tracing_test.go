package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// ── ConfigFromEnv tests ───────────────────────────────────────────────

func TestConfigFromEnv_OTLPFallback(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENABLED", "")
	t.Setenv("TAMGA_OTLP_ENDPOINT", "localhost:4318")
	t.Setenv("TAMGA_OTEL_SAMPLE_RATE", "0.25")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if !cfg.Enabled {
		t.Fatal("expected enabled from legacy OTLP endpoint")
	}
	if cfg.Endpoint != "localhost:4318" {
		t.Fatalf("endpoint: %q", cfg.Endpoint)
	}
	if cfg.SampleRate != 0.25 {
		t.Fatalf("sample: %v", cfg.SampleRate)
	}
}

func TestConfigFromEnv_ExplicitOff(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENABLED", "false")
	t.Setenv("TAMGA_OTLP_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.Enabled {
		t.Fatal("expected disabled")
	}
}

func TestConfigFromEnv_DisabledByDefault(t *testing.T) {
	// No env vars set: default disabled, sample 0.05.
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.Enabled {
		t.Fatal("expected disabled by default")
	}
	if cfg.SampleRate != 0.05 {
		t.Fatalf("default sample rate: %v", cfg.SampleRate)
	}
	if cfg.Environment != "development" {
		t.Fatalf("default env: %q", cfg.Environment)
	}
	if cfg.ServiceName != "svc" {
		t.Fatalf("ServiceName: %q", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "1.0.0" {
		t.Fatalf("ServiceVersion: %q", cfg.ServiceVersion)
	}
}

func TestConfigFromEnv_ExplicitlyEnabled(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
}

func TestConfigFromEnv_EnabledViaOne(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENABLED", "1")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if !cfg.Enabled {
		t.Fatal("expected enabled when set to '1'")
	}
}

func TestConfigFromEnv_ExplicitlyDisabledViaZero(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENABLED", "0")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.Enabled {
		t.Fatal("expected disabled when set to '0'")
	}
}

func TestConfigFromEnv_LegacyEndpointEnables(t *testing.T) {
	// When OTEL_ENABLED is empty but OTLP_ENDPOINT is set,
	// the legacy path should enable tracing.
	t.Setenv("TAMGA_OTEL_ENABLED", "")
	t.Setenv("TAMGA_OTLP_ENDPOINT", "otel-collector:4317")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if !cfg.Enabled {
		t.Fatal("legacy endpoint should enable tracing")
	}
	if cfg.Endpoint != "otel-collector:4317" {
		t.Fatalf("endpoint: %q", cfg.Endpoint)
	}
}

func TestConfigFromEnv_OTEL_ENDPOINTPreferred(t *testing.T) {
	// TAMGA_OTEL_ENDPOINT takes priority over TAMGA_OTLP_ENDPOINT.
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "primary:4318")
	t.Setenv("TAMGA_OTLP_ENDPOINT", "fallback:4317")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.Endpoint != "primary:4318" {
		t.Fatalf("expected primary endpoint, got %q", cfg.Endpoint)
	}
}

func TestConfigFromEnv_SampleRateBounds(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  float64
	}{
		{"below zero", "-0.5", 0.05},
		{"above one", "1.5", 0.05},
		{"exactly zero", "0.0", 0.0},
		{"exactly one", "1.0", 1.0},
		{"invalid", "not-a-number", 0.05},
		{"empty", "", 0.05},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TAMGA_OTEL_SAMPLE_RATE", tt.value)
			t.Setenv("TAMGA_OTEL_ENABLED", "true")
			t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
			cfg := ConfigFromEnv("svc", "1.0.0")
			if cfg.SampleRate != tt.want {
				t.Fatalf("SampleRate: want %v, got %v", tt.want, cfg.SampleRate)
			}
		})
	}
}

func TestConfigFromEnv_Environment(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENVIRONMENT", "production")
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.Environment != "production" {
		t.Fatalf("Environment: %q", cfg.Environment)
	}
}

func TestConfigFromEnv_InsecureFlag(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"default insecure", "", true},
		{"explicit true", "true", true},
		{"explicit 1", "1", true},
		{"explicit false", "false", false},
		{"explicit 0", "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TAMGA_OTEL_INSECURE", tt.value)
			t.Setenv("TAMGA_OTEL_ENABLED", "true")
			t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
			cfg := ConfigFromEnv("svc", "1.0.0")
			if cfg.Insecure != tt.want {
				t.Fatalf("Insecure: want %v, got %v", tt.want, cfg.Insecure)
			}
		})
	}
}

func TestConfigFromEnv_URLPath(t *testing.T) {
	t.Setenv("TAMGA_OTEL_URL_PATH", "/v1/traces")
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.URLPath != "/v1/traces" {
		t.Fatalf("URLPath: %q", cfg.URLPath)
	}
}

func TestConfigFromEnv_DefaultURLPath(t *testing.T) {
	t.Setenv("TAMGA_OTEL_URL_PATH", "")
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "localhost:4318")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.URLPath != "/v1/traces" {
		t.Fatalf("URLPath default: %q", cfg.URLPath)
	}
}

func TestConfigFromEnv_HoneycombHeaders(t *testing.T) {
	t.Setenv("TAMGA_OTEL_HEADERS", "x-honeycomb-team=abc123,x-honeycomb-dataset=tamga")
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "api.honeycomb.io:443")
	cfg := ConfigFromEnv("svc", "1.0.0")
	if cfg.Headers["x-honeycomb-team"] != "abc123" {
		t.Fatalf("x-honeycomb-team: %q", cfg.Headers["x-honeycomb-team"])
	}
	if cfg.Headers["x-honeycomb-dataset"] != "tamga" {
		t.Fatalf("x-honeycomb-dataset: %q", cfg.Headers["x-honeycomb-dataset"])
	}
}

// ── parseHeaderList tests ─────────────────────────────────────────────

func TestParseHeaderList_Empty(t *testing.T) {
	if parseHeaderList("") != nil {
		t.Fatal("empty string should return nil")
	}
	if parseHeaderList("   ") != nil {
		t.Fatal("whitespace-only should return nil")
	}
}

func TestParseHeaderList_SingleHeader(t *testing.T) {
	out := parseHeaderList("x-api-key: secret123")
	if len(out) != 1 {
		t.Fatalf("want 1 header, got %d: %v", len(out), out)
	}
	if out["x-api-key"] != "secret123" {
		t.Fatalf("x-api-key: %q", out["x-api-key"])
	}
}

func TestParseHeaderList_MultipleHeaders(t *testing.T) {
	out := parseHeaderList("x-key: val1, x-team: val2, x-env: val3")
	if len(out) != 3 {
		t.Fatalf("want 3 headers, got %d: %v", len(out), out)
	}
	if out["x-key"] != "val1" {
		t.Fatalf("x-key: %q", out["x-key"])
	}
	if out["x-team"] != "val2" {
		t.Fatalf("x-team: %q", out["x-team"])
	}
	if out["x-env"] != "val3" {
		t.Fatalf("x-env: %q", out["x-env"])
	}
}

func TestParseHeaderList_TrimWhitespace(t *testing.T) {
	out := parseHeaderList("  x-key  :  val1  ,  x-team  :  val2  ")
	if len(out) != 2 {
		t.Fatalf("want 2 headers, got %d: %v", len(out), out)
	}
	if out["x-key"] != "val1" {
		t.Fatalf("x-key: %q", out["x-key"])
	}
	if out["x-team"] != "val2" {
		t.Fatalf("x-team: %q", out["x-team"])
	}
}

func TestParseHeaderList_EqualsSignSeparator(t *testing.T) {
	out := parseHeaderList("x-honeycomb-team=abc123")
	if len(out) != 1 {
		t.Fatalf("want 1 header, got %d: %v", len(out), out)
	}
	if out["x-honeycomb-team"] != "abc123" {
		t.Fatalf("x-honeycomb-team: %q", out["x-honeycomb-team"])
	}
}

func TestParseHeaderList_MalformedSkipped(t *testing.T) {
	out := parseHeaderList("x-key: val1, invalid-no-colon, x-team: val2")
	if len(out) != 2 {
		t.Fatalf("malformed entry should be skipped, got %d: %v", len(out), out)
	}
	if out["x-key"] != "val1" {
		t.Fatalf("x-key: %q", out["x-key"])
	}
	if out["x-team"] != "val2" {
		t.Fatalf("x-team: %q", out["x-team"])
	}
}

func TestParseHeaderList_EmptyKeySkipped(t *testing.T) {
	out := parseHeaderList(": val, x-key: val1")
	if len(out) != 1 {
		t.Fatalf("empty key should be skipped, got %d: %v", len(out), out)
	}
	if out["x-key"] != "val1" {
		t.Fatalf("x-key: %q", out["x-key"])
	}
}

func TestParseHeaderList_AllEmptyKeys(t *testing.T) {
	// Every entry has an empty key.
	out := parseHeaderList(": v1, : v2")
	if out != nil {
		t.Fatalf("all empty keys should return nil, got %v", out)
	}
}

func TestParseHeaderList_TrailingComma(t *testing.T) {
	out := parseHeaderList("x-key: val1,")
	if len(out) != 1 {
		t.Fatalf("trailing comma should be fine, got %d: %v", len(out), out)
	}
	if out["x-key"] != "val1" {
		t.Fatalf("x-key: %q", out["x-key"])
	}
}

func TestParseHeaderList_LeadingComma(t *testing.T) {
	out := parseHeaderList(", x-key: val1")
	if len(out) != 1 {
		t.Fatalf("leading comma should be fine, got %d: %v", len(out), out)
	}
	if out["x-key"] != "val1" {
		t.Fatalf("x-key: %q", out["x-key"])
	}
}

// ── normalizeOTLPEndpoint tests ───────────────────────────────────────

func TestNormalizeOTLPEndpoint_Empty(t *testing.T) {
	host, path, insecure := normalizeOTLPEndpoint("", true)
	if host != "" || path != "" || insecure != nil {
		t.Fatalf("empty: (%q, %q, %v)", host, path, insecure)
	}
}

func TestNormalizeOTLPEndpoint_PlainHostPort(t *testing.T) {
	host, path, insecure := normalizeOTLPEndpoint("localhost:4318", true)
	if host != "localhost:4318" {
		t.Fatalf("host: %q", host)
	}
	if path != "" {
		t.Fatalf("path: %q", path)
	}
	if insecure != nil {
		t.Fatalf("plain should not override insecure: %v", insecure)
	}
}

func TestNormalizeOTLPEndpoint_HTTPS_URL(t *testing.T) {
	host, path, insecure := normalizeOTLPEndpoint("https://api.honeycomb.io:443/v1/traces", false)
	if host != "api.honeycomb.io:443" {
		t.Fatalf("host: %q", host)
	}
	if path != "/v1/traces" {
		t.Fatalf("path: %q", path)
	}
	if insecure == nil || *insecure {
		t.Fatalf("https should set insecure to false, got %v", insecure)
	}
}

func TestNormalizeOTLPEndpoint_HTTP_URL(t *testing.T) {
	host, path, insecure := normalizeOTLPEndpoint("http://localhost:4318/v1/traces", false)
	if host != "localhost:4318" {
		t.Fatalf("host: %q", host)
	}
	if path != "/v1/traces" {
		t.Fatalf("path: %q", path)
	}
	if insecure == nil || !*insecure {
		t.Fatalf("http should set insecure to true, got %v", insecure)
	}
}

func TestNormalizeOTLPEndpoint_NoPath(t *testing.T) {
	host, path, insecure := normalizeOTLPEndpoint("https://collector.example.com", true)
	if host != "collector.example.com" {
		t.Fatalf("host: %q", host)
	}
	if path != "" {
		t.Fatalf("path: %q", path)
	}
	if insecure == nil || *insecure {
		t.Fatalf("https should override to not insecure, got %v", insecure)
	}
}

func TestNormalizeOTLPEndpoint_InvalidURL(t *testing.T) {
	// URL with no host after stripping scheme — falls through to raw endpoint.
	host, _, insecure := normalizeOTLPEndpoint("://nohost", true)
	// The parsed URL has empty Host, so the function returns the raw endpoint.
	if host != "://nohost" {
		t.Fatalf("host: %q", host)
	}
	if insecure != nil {
		t.Fatalf("invalid could not determine insecure flag: %v", insecure)
	}
}

// ── SpanNameForScanner tests ──────────────────────────────────────────

func TestSpanNameForScanner_Normal(t *testing.T) {
	name := SpanNameForScanner("PII Scanner")
	if name != "scanner.pii_scanner" {
		t.Fatalf("want scanner.pii_scanner, got %q", name)
	}
}

func TestSpanNameForScanner_LowercaseInput(t *testing.T) {
	name := SpanNameForScanner("injection scanner")
	if name != "scanner.injection_scanner" {
		t.Fatalf("want scanner.injection_scanner, got %q", name)
	}
}

func TestSpanNameForScanner_MultipleSpaces(t *testing.T) {
	name := SpanNameForScanner("custom  entity  scanner")
	if name != "scanner.custom__entity__scanner" {
		t.Fatalf("want scanner.custom__entity__scanner, got %q", name)
	}
}

func TestSpanNameForScanner_Trimmed(t *testing.T) {
	name := SpanNameForScanner("  DFA Core  ")
	if name != "scanner.dfa_core" {
		t.Fatalf("want scanner.dfa_core, got %q", name)
	}
}

func TestSpanNameForScanner_Empty(t *testing.T) {
	name := SpanNameForScanner("")
	if name != "scanner.unknown" {
		t.Fatalf("empty should be scanner.unknown, got %q", name)
	}
}

func TestSpanNameForScanner_WhitespaceOnly(t *testing.T) {
	name := SpanNameForScanner("   ")
	if name != "scanner.unknown" {
		t.Fatalf("whitespace-only should be scanner.unknown, got %q", name)
	}
}

func TestSpanNameForScanner_SingleWord(t *testing.T) {
	name := SpanNameForScanner("DFA")
	if name != "scanner.dfa" {
		t.Fatalf("want scanner.dfa, got %q", name)
	}
}

// ── TraceIDFromContext tests ──────────────────────────────────────────

func TestTraceIDFromContext_NoSpan(t *testing.T) {
	ctx := context.Background()
	id := TraceIDFromContext(ctx)
	if id != "" {
		t.Fatalf("expected empty trace ID from bare context, got %q", id)
	}
}

func TestTraceIDFromContext_ValidSpan(t *testing.T) {
	// Create a real span context with a non-zero trace ID.
	tid, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	sid, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	id := TraceIDFromContext(ctx)
	if id != "0af7651916cd43dd8448eb211c80319c" {
		t.Fatalf("trace ID: %q", id)
	}
}

func TestTraceIDFromContext_InvalidSpanContext(t *testing.T) {
	// An invalid span context should return empty string.
	sc := trace.SpanContext{}
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	id := TraceIDFromContext(ctx)
	if id != "" {
		t.Fatalf("expected empty trace ID from invalid span context, got %q", id)
	}
}

// ── ConfigFromEnv integration: full Honeycomb config ──────────────────

func TestConfigFromEnv_HoneycombFull(t *testing.T) {
	t.Setenv("TAMGA_OTEL_ENABLED", "true")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "https://api.honeycomb.io:443")
	t.Setenv("TAMGA_OTEL_HEADERS", "x-honeycomb-team=key123,x-honeycomb-dataset=tamga-proxy")
	t.Setenv("TAMGA_OTEL_SAMPLE_RATE", "0.1")
	t.Setenv("TAMGA_OTEL_ENVIRONMENT", "staging")
	t.Setenv("TAMGA_OTEL_URL_PATH", "/v1/traces")

	cfg := ConfigFromEnv("tamga-proxy", "2.0.0")
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	if cfg.Endpoint != "api.honeycomb.io:443" {
		t.Fatalf("Endpoint: %q", cfg.Endpoint)
	}
	if cfg.URLPath != "/v1/traces" {
		t.Fatalf("URLPath: %q", cfg.URLPath)
	}
	if cfg.Insecure {
		t.Fatal("expected secure (HTTPS) connection")
	}
	if cfg.SampleRate != 0.1 {
		t.Fatalf("SampleRate: %v", cfg.SampleRate)
	}
	if cfg.Environment != "staging" {
		t.Fatalf("Environment: %q", cfg.Environment)
	}
	if len(cfg.Headers) != 2 {
		t.Fatalf("Headers: %v", cfg.Headers)
	}
	if cfg.Headers["x-honeycomb-team"] != "key123" {
		t.Fatalf("x-honeycomb-team: %q", cfg.Headers["x-honeycomb-team"])
	}
}

// ── InitTracing tests ──────────────────────────────────────────────────

func TestInitTracing_Disabled(t *testing.T) {
	cfg := TracingConfig{
		Enabled:  false,
		Endpoint: "localhost:4318",
	}
	shutdown, err := InitTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("disabled config should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil no-op shutdown")
	}
	// Calling shutdown should not panic.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown should not error: %v", err)
	}
}

func TestInitTracing_EmptyEndpoint(t *testing.T) {
	// Enabled but no endpoint: returns no-op without error.
	cfg := TracingConfig{
		Enabled:  true,
		Endpoint: "",
	}
	shutdown, err := InitTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("empty endpoint should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil no-op shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown should not error: %v", err)
	}
}

func TestInitTracing_ValidConfig(t *testing.T) {
	// Save and restore global tracer provider so other tests are not affected.
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	// Start a minimal HTTP server that accepts OTLP export requests.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	cfg := TracingConfig{
		Enabled:        true,
		Endpoint:       u.Host,
		URLPath:        "/",
		Insecure:       true,
		SampleRate:     1.0,
		Environment:    "test",
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.0",
	}

	shutdown, err := InitTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitTracing with valid config should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}

	// Verify the tracer provider was set.
	tp := otel.GetTracerProvider()
	if tp == nil {
		t.Fatal("tracer provider should not be nil after InitTracing")
	}

	// Shutdown should not panic.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown should not error: %v", err)
	}
}

func TestInitTracing_InvalidEndpoint(t *testing.T) {
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	// Use a valid host:port but point to an unreachable port to verify
	// the exporter still initialises (the SDK defers connection attempts).
	cfg := TracingConfig{
		Enabled:        true,
		Endpoint:       "127.0.0.1:19999",
		Insecure:       true,
		SampleRate:     1.0,
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.0",
	}

	shutdown, err := InitTracing(context.Background(), cfg)
	// The OTLP/HTTP exporter creates the client lazily, so InitTracing
	// should succeed even with an unreachable endpoint.
	if err != nil {
		t.Fatalf("InitTracing with unreachable endpoint should not fail at init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	// Shutdown must not panic.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestInitTracing_MalformedURL(t *testing.T) {
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	// A URL-like string that will be treated as raw host:port by normalizeOTLPEndpoint.
	cfg := TracingConfig{
		Enabled:        true,
		Endpoint:       "://nohost:1234",
		Insecure:       true,
		SampleRate:     1.0,
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.0",
	}

	shutdown, err := InitTracing(context.Background(), cfg)
	// The malformed endpoint may cause the OTLP exporter to fail.
	// Either way, the function must not panic.
	if shutdown == nil && err == nil {
		t.Fatal("expected either shutdown or error for malformed endpoint")
	}
	// If a shutdown was returned, calling it must not panic.
	if shutdown != nil {
		_ = shutdown(context.Background())
	}
}

func TestInitTracing_Shutdown(t *testing.T) {
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)

	cfg := TracingConfig{
		Enabled:        true,
		Endpoint:       u.Host,
		URLPath:        "/",
		Insecure:       true,
		SampleRate:     1.0,
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.0",
	}

	shutdown, err := InitTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitTracing error: %v", err)
	}

	// Calling shutdown multiple times should not panic.
	if err := shutdown(context.Background()); err != nil {
		t.Logf("first shutdown (non-fatal): %v", err)
	}
	// Second shutdown is safe.
	if err := shutdown(context.Background()); err != nil {
		t.Logf("second shutdown (non-fatal, may duplicate close): %v", err)
	}
}

// ── Init tests ─────────────────────────────────────────────────────────

func TestInit_Disabled(t *testing.T) {
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	t.Setenv("TAMGA_OTEL_ENABLED", "")
	t.Setenv("TAMGA_OTLP_ENDPOINT", "")
	t.Setenv("TAMGA_OTEL_ENDPOINT", "")

	shutdown, err := Init(context.Background(), "test-svc", "1.0.0")
	if err != nil {
		t.Fatalf("Init disabled should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown even when disabled")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown should not error: %v", err)
	}
}

func TestInit_EnabledViaLegacyEndpoint(t *testing.T) {
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)

	t.Setenv("TAMGA_OTEL_ENABLED", "")
	t.Setenv("TAMGA_OTLP_ENDPOINT", u.Host)
	t.Setenv("TAMGA_OTEL_URL_PATH", "/")
	t.Setenv("TAMGA_OTEL_SAMPLE_RATE", "1.0")

	shutdown, err := Init(context.Background(), "test-svc", "1.0.0")
	if err != nil {
		t.Fatalf("Init with legacy endpoint should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	_ = shutdown(context.Background())
}

// ── Tracer tests ───────────────────────────────────────────────────────

func TestTracer_ReturnsNonNil(t *testing.T) {
	tr := Tracer()
	if tr == nil {
		t.Fatal("Tracer() should never return nil")
	}
	// The no-op tracer should still be able to start a span.
	_, span := tr.Start(context.Background(), "test-span")
	if span == nil {
		t.Fatal("no-op tracer should produce a non-nil span")
	}
	span.End()
}

func TestTracer_AfterInitTracing(t *testing.T) {
	origTP := otel.GetTracerProvider()
	defer otel.SetTracerProvider(origTP)
	origProp := otel.GetTextMapPropagator()
	defer otel.SetTextMapPropagator(origProp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	cfg := TracingConfig{
		Enabled:        true,
		Endpoint:       u.Host,
		URLPath:        "/",
		Insecure:       true,
		SampleRate:     1.0,
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.0",
	}
	shutdown, err := InitTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitTracing error: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	tr := Tracer()
	if tr == nil {
		t.Fatal("Tracer() should return non-nil after InitTracing")
	}
	_, span := tr.Start(context.Background(), "test-span")
	if span == nil {
		t.Fatal("tracer should produce a non-nil span")
	}
	span.End()
}
