package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yatuk/tamga/internal/config"
	"github.com/yatuk/tamga/internal/policy"
	"github.com/yatuk/tamga/internal/scanner"
)

// hangingScanner never finishes before its context ends: a scan that runs
// into the pipeline deadline.
type hangingScanner struct{}

func (hangingScanner) Name() string { return "hanging" }
func (hangingScanner) Scan(ctx context.Context, _ []byte) ([]scanner.Finding, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// holdingScanner blocks until release is closed. It fills a worker pool.
type holdingScanner struct{ release chan struct{} }

func (holdingScanner) Name() string { return "holding" }
func (h holdingScanner) Scan(context.Context, []byte) ([]scanner.Finding, error) {
	<-h.release
	return nil, nil
}

// A request that carries a tool result, and one that only defines a tool.
const (
	toolResultChat = `{"model":"gpt-4o","messages":[` +
		`{"role":"user","content":"look it up"},` +
		`{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"fetch","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"c1","content":"page text"}]}`
	toolDefinitionChat = `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],` +
		`"tools":[{"type":"function","function":{"name":"fetch","description":"Fetch a page","parameters":{"type":"object","properties":{}}}}]}`
)

// failureMode names one way a scan does not finish, and how to cause it.
type failureMode struct {
	name  string
	build func(t *testing.T) (scanner.Scanner, *config.Config, *scanner.WorkerPool)
}

var failureModes = []failureMode{
	{"scanner error", func(*testing.T) (scanner.Scanner, *config.Config, *scanner.WorkerPool) {
		return failingScanner{}, &config.Config{}, nil
	}},
	{"deadline", func(*testing.T) (scanner.Scanner, *config.Config, *scanner.WorkerPool) {
		return hangingScanner{}, &config.Config{ScannerPipelineMode: "adaptive", ScannerPipelineTimeoutMs: 40}, nil
	}},
	{"load shedding", func(t *testing.T) (scanner.Scanner, *config.Config, *scanner.WorkerPool) {
		// One worker, a queue of two. Three held jobs leave the queue full,
		// which is above the 95% line where every scanner is shed.
		release := make(chan struct{})
		pool := scanner.NewWorkerPool(1, 2)
		t.Cleanup(func() {
			close(release)
			_ = pool.Shutdown(2 * time.Second)
		})
		resultCh := make(chan scanner.ScanResult, 3)
		for i := 0; i < 3; i++ {
			job := scanner.Job{Scanner: holdingScanner{release}, Content: []byte("x"), Ctx: context.Background(), ResultCh: resultCh}
			if err := pool.Submit(job); err != nil {
				t.Fatalf("filling the pool: %v", err)
			}
			if i == 0 {
				// Let the worker take the first job before the queue is filled.
				for end := time.Now().Add(2 * time.Second); pool.Stats().ActiveJobs == 0 && time.Now().Before(end); {
					time.Sleep(time.Millisecond)
				}
			}
		}
		return scanner.NewPIIScanner(), &config.Config{ScannerPipelineMode: "workerpool", ScannerLoadShed: true}, pool
	}},
}

func failureServer(t *testing.T, mode failureMode, policyYAML string) (string, *int32) {
	t.Helper()
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)

	sc, cfg, pool := mode.build(t)
	registry := scanner.NewRegistry()
	registry.Register(sc)
	pol := mustPolicy(t, policyYAML)
	srv := httptest.NewServer(NewHandler(HandlerConfig{
		Registry:     registry,
		GetPolicy:    func() *policy.Policy { return pol },
		UpstreamURLs: map[string]*url.URL{"openai": u},
		Config:       cfg,
		ScannerPool:  pool,
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits
}

// Every way a scan can fail gives the answer scan.on_error names, for a plain
// chat request and for agent traffic alike. With the key left out the two
// differ: a request with tool results or tool definitions is refused.
func TestScanError_EveryFailureFollowsOnError(t *testing.T) {
	bodies := []struct {
		name string
		body string
		tool bool
	}{
		{"chat", benignChat, false},
		{"tool result", toolResultChat, true},
		{"tool definition", toolDefinitionChat, true},
	}
	policies := []struct {
		name  string
		scan  string
		block func(tool bool) bool
	}{
		{"block", "scan:\n  on_error: block\n", func(bool) bool { return true }},
		{"pass", "scan:\n  on_error: pass\n", func(bool) bool { return false }},
		{"not set", "", func(tool bool) bool { return tool }},
	}
	for _, mode := range failureModes {
		for _, pc := range policies {
			for _, b := range bodies {
				t.Run(mode.name+"/"+pc.name+"/"+b.name, func(t *testing.T) {
					proxyURL, hits := failureServer(t, mode, scanErrorPolicy+pc.scan)
					resp := post(t, proxyURL, "application/json", b.body)
					body, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if resp.Header.Get("X-Tamga-Scan-Degraded") != "true" {
						t.Fatalf("the failure must show in X-Tamga-Scan-Degraded; headers = %v", resp.Header)
					}
					if pc.block(b.tool) {
						if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), `"code":"tamga_scan_unavailable"`) {
							t.Fatalf("status = %d, want 503 tamga_scan_unavailable: %s", resp.StatusCode, body)
						}
						if got := atomic.LoadInt32(hits); got != 0 {
							t.Fatalf("the request reached the provider %d times", got)
						}
						return
					}
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
					}
					if got := atomic.LoadInt32(hits); got != 1 {
						t.Fatalf("provider hits = %d, want 1", got)
					}
				})
			}
		}
	}
}
