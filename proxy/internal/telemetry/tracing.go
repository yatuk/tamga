package telemetry

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/yatuk/tamga"

// Shutdown flushes pending spans on process exit.
type Shutdown func(context.Context) error

// TracingConfig controls OTLP export and sampling.
type TracingConfig struct {
	Enabled        bool
	Endpoint       string // host:port or full URL — passed to OTLP HTTP client
	URLPath        string // e.g. /v1/traces
	Insecure       bool
	Headers        map[string]string
	SampleRate     float64
	Environment    string
	ServiceName    string
	ServiceVersion string
}

// ConfigFromEnv loads tracing settings. Legacy: TAMGA_OTLP_ENDPOINT still enables export
// when TAMGA_OTEL_ENABLED is unset.
func ConfigFromEnv(serviceName, version string) TracingConfig {
	explicitOff := strings.EqualFold(strings.TrimSpace(os.Getenv("TAMGA_OTEL_ENABLED")), "false") ||
		strings.TrimSpace(os.Getenv("TAMGA_OTEL_ENABLED")) == "0"

	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("TAMGA_OTEL_ENABLED")), "true") ||
		strings.TrimSpace(os.Getenv("TAMGA_OTEL_ENABLED")) == "1"
	endpoint := strings.TrimSpace(os.Getenv("TAMGA_OTEL_ENDPOINT"))
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("TAMGA_OTLP_ENDPOINT"))
	}
	if !explicitOff && endpoint != "" && !enabled {
		// Backward compatibility: OTLP endpoint alone turns tracing on.
		enabled = true
	}
	if explicitOff {
		enabled = false
	}

	sample := 0.05 // 5% — production-safe default, prevents trace volume explosion
	if s := strings.TrimSpace(os.Getenv("TAMGA_OTEL_SAMPLE_RATE")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 && f <= 1 {
			sample = f
		}
	}
	env := strings.TrimSpace(os.Getenv("TAMGA_OTEL_ENVIRONMENT"))
	if env == "" {
		env = "development"
	}
	insecure := true
	if v := strings.TrimSpace(os.Getenv("TAMGA_OTEL_INSECURE")); v == "0" || strings.EqualFold(v, "false") {
		insecure = false
	}

	headers := parseHeaderList(os.Getenv("TAMGA_OTEL_HEADERS"))

	path := strings.TrimSpace(os.Getenv("TAMGA_OTEL_URL_PATH"))
	if path == "" {
		path = "/v1/traces"
	}

	ep, pathFromURL, useInsecure := normalizeOTLPEndpoint(endpoint, insecure)
	if pathFromURL != "" {
		path = pathFromURL
	}
	if useInsecure != nil {
		insecure = *useInsecure
	}

	return TracingConfig{
		Enabled:        enabled,
		Endpoint:       ep,
		URLPath:        path,
		Insecure:       insecure,
		Headers:        headers,
		SampleRate:     sample,
		Environment:    env,
		ServiceName:    serviceName,
		ServiceVersion: version,
	}
}

func parseHeaderList(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			k, v, ok = strings.Cut(part, "=")
		}
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeOTLPEndpoint returns host:port, optional path override, optional insecure override.
func normalizeOTLPEndpoint(endpoint string, defaultInsecure bool) (hostPort string, path string, insecure *bool) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", "", nil
	}
	if !strings.Contains(endpoint, "://") {
		return endpoint, "", nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return endpoint, "", nil
	}
	hostPort = u.Host
	path = u.Path
	inv := u.Scheme == "http"
	return hostPort, path, &inv
}

// InitTracing wires OTLP/HTTP export and global propagator. No-op shutdown when disabled.
func InitTracing(ctx context.Context, cfg TracingConfig) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Enabled || cfg.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithTimeout(8 * time.Second),
	}
	if cfg.URLPath != "" {
		opts = append(opts, otlptracehttp.WithURLPath(cfg.URLPath))
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
	}

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return func(context.Context) error { return nil }, err
	}

	res, _ := sdkresource.Merge(
		sdkresource.Default(),
		sdkresource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRate))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Init is a convenience wrapper: ConfigFromEnv + InitTracing.
func Init(ctx context.Context, serviceName, version string) (Shutdown, error) {
	return InitTracing(ctx, ConfigFromEnv(serviceName, version))
}

// Tracer returns the global Tamga tracer (no-op if telemetry was not enabled).
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// TraceIDFromContext extracts the hex trace ID from a context, or "" if none.
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanName returns stable span names for scanner sub-spans.
func SpanNameForScanner(scannerName string) string {
	s := strings.TrimSpace(strings.ToLower(scannerName))
	s = strings.ReplaceAll(s, " ", "_")
	if s == "" {
		return "scanner.unknown"
	}
	return "scanner." + s
}
