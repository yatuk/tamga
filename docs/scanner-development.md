# Writing a Custom Scanner

This guide covers the scanner contract, the optional interfaces a scanner can
implement, and — for stateful scanners — how per-request context
(`RequestContext`) reaches your code. The `operator_state` scanner
(`proxy/internal/scanner/operator_state/`) is the reference implementation for
the stateful pattern; `CustomScanner` (`proxy/internal/scanner/custom.go`) is
the reference for policy-driven stateless scanners.

## The Scanner interface

Every scanner implements two methods (`proxy/internal/scanner/scanner.go`):

```go
type Scanner interface {
    Name() string
    Scan(ctx context.Context, content []byte) ([]Finding, error)
}
```

- `Name()` is the registry key — used by `Registry.SetSpeed`, policy rule keys
  (`rules.<name>` / `rules.<name>_detection`), and Prometheus labels. Add new
  names to `knownScannerTypes` in `proxy/internal/policy/validate.go` or
  policy validation will warn that your rule key never fires.
- `Scan` must be safe for concurrent use: the pipeline calls it from multiple
  goroutines. Guard any mutable state with a mutex (see the `sync.RWMutex`
  pattern in `OperatorStateScanner`).
- Fail open: return `(nil, err)` on internal errors; the pipeline logs and
  continues with other scanners' findings.

## Optional interfaces

Declared in `proxy/internal/scanner/context.go`; the harness type-asserts them:

```go
type ContextualScanner interface {        // per-request state (see below)
    Scanner
    ScanWithContext(ctx context.Context, content []byte, reqCtx *RequestContext) ([]Finding, error)
}
type Refreshable interface { Refresh() }  // hot-reload hook on policy change
type HealthReporter interface { IsHealthy(ctx context.Context) bool }
```

Add compile-time assertions so drift fails the build:

```go
var (
    _ scanner.Scanner           = (*MyScanner)(nil)
    _ scanner.ContextualScanner = (*MyScanner)(nil)
)
```

## RequestContext — stateful scanners

Stateless scanners see only the prompt bytes. Stateful checks (does this
prompt contradict a locked decision, is this operator authorized) also need
request identity. That arrives as:

```go
type RequestContext struct {
    RequestId             string        // proxy request id (provenance)
    OperatorId            string        // X-Tamga-Operator-Id
    ActiveDecisionIds     []string      // X-Tamga-Active-Decisions (CSV)
    FreshnessTTL          time.Duration // policy default, request-overridable
    LastVerifiableByFired time.Time     // X-Tamga-Last-Verifiable-By (RFC3339)
}
```

Flow: `handler.go` builds a `RequestContext` from request headers and policy
defaults (`buildRequestContext`), sets it on `PipelineConfig.RequestCtx`, and
every pipeline mode (sync, async, adaptive, worker pool) dispatches through
one helper:

```go
if cs, ok := s.(ContextualScanner); ok && reqCtx != nil {
    return cs.ScanWithContext(ctx, content, reqCtx)
}
return s.Scan(ctx, content)
```

So: implement `ScanWithContext` for the real logic and delegate `Scan` to it
with a nil context —

```go
func (s *MyScanner) Scan(ctx context.Context, content []byte) ([]scanner.Finding, error) {
    return s.ScanWithContext(ctx, content, nil)
}
```

`reqCtx` may be nil (context-free callers, older pipelines) and any field may
be zero — degrade gracefully, never require it. Existing scanners are
untouched by this mechanism: only `ContextualScanner` implementers see it.

## Findings and per-finding actions

Return `scanner.Finding` values with `Type` = your scanner name and a stable
`Category` per check. Two ways findings drive policy action:

1. **Rule-level** (default): the policy rule for your `Type` decides
   (`action: BLOCK` + `sensitivity` thresholds).
2. **Per-finding** (`mode: confidence_based`): set `Finding.ConfidenceScore`
   with an explicit `Action` (`scanner.ActionBlock` / `ActionWarn` /
   `ActionPassLog` / `ActionRedact`). This is how `operator_state` maps each
   assertion's `action_on_fail` to block/warn/log per finding — see
   `confidenceForAction` in `operator_state/scan_assertion.go`.

Note: `stampVersions` (`pipeline.go`) overwrites `Finding.ScannerVersion` on
every finding with the global pipeline version — don't rely on a per-scanner
version string surviving to the API consumer.

## Speed classification

Register the scanner and classify it:

```go
registry.Register(myScanner)
registry.SetSpeed("my_scanner", scanner.SpeedFast) // <1ms: sequential
// scanner.SpeedSlow (≥1ms, network calls): parallel goroutines
```

Fast scanners run sequentially in the calling goroutine (goroutine spawn would
dominate sub-millisecond work); slow scanners run in parallel. If your scanner
blocks on I/O, it must be `SpeedSlow` — or better, publish an event and do the
slow work asynchronously (see the `operator_state` deep-analysis route:
`SetDeepAnalysisPublisher` + `OperatorStateAnalyzerHandler` in
`proxy/internal/events/handlers.go`).

## Wiring checklist (`proxy/cmd/tamga/main.go`)

1. Declare the scanner variable before the policy watcher closure if it needs
   hot-reload (`var opScanner *operator_state.OperatorStateScanner` pattern).
2. In the policy watcher callback, reload rules on change — fail open, keep
   the previous rules on error.
3. Subscribe any event handlers **before** `eventBus.Start()` (Subscribe is
   not safe afterwards); publishing is safe at any time.
4. Build state (projection, watcher, Redis via the shared `rdx` client —
   `redisx.Client` structurally satisfies narrow store interfaces), then
   `registry.Register` + `registry.SetSpeed`.
5. Policy: add the scanner name to `knownScannerTypes` (`validate.go`), a
   `rules.<name>` entry in `tamga-policy.yaml`, and semantic validation for
   any new policy block.
6. Tests: unit tests beside the code, `<subject>_integration_test.go` for
   end-to-end fixture-driven paths, `Benchmark*` proving your latency class,
   and an adversarial vector script under `tests/stress/adversarial/` wired
   into `run_stress_suite.sh` + `baseline.json`.
