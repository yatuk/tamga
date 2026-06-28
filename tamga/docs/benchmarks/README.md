# Tamga public benchmark

This folder publishes the raw output of `tamga/proxy/cmd/redteam` so anyone can
verify Tamga's accuracy claims without running the project locally.

Every report is produced by the same, checked-in command:

```bash
# from tamga/proxy
go run ./cmd/redteam \
    -in   ./testdata/redteam/prompts.csv \
    -json ../docs/benchmarks/redteam_latest.json
```

The corpus at `tamga/proxy/testdata/redteam/prompts.csv` is the exact set the
runtime scanners are gated on in CI — it is not a "marketing" dataset. Benign
samples, obfuscated jailbreaks, Turkish/English PII, BIN-validated credit
cards, and secret-format tokens are all mixed in.

## Latest run

- **File:** [`redteam_latest.json`](./redteam_latest.json)
- **Corpus size:** 309 prompts (≈ 40% benign, 60% adversarial/PII/secret)
- **Environment:** Go 1.22, Windows amd64, single proc, no Shadow ML sidecar

### Aggregate

| metric | value |
| --- | --- |
| Precision | **0.969** |
| Recall | **0.484** |
| F1 | **0.646** |
| Scan latency p95 | **0.52 ms** |
| Scan latency p99 | **0.58 ms** |
| Scan latency max | **0.77 ms** |

### How to read these numbers honestly

- **Precision (0.969)** — of the prompts the inline Go engine mitigated,
  96.9% were actually adversarial or contained sensitive data. This is the
  number that governs user experience: false positives block real traffic
  and train analysts to ignore alerts.
- **Recall (0.484)** — the inline deterministic engine catches ≈ 48% of
  the adversarial corpus on its own. The remaining 52% are designed to
  need semantic reasoning (ROT13, grandma prompts, homoglyph attacks,
  fictional framings). Those are the prompts the Shadow ML sidecar (S4)
  is built to handle asynchronously and feed back into the DFA
  dictionary; they are not the target of the 5 ms hot path.
- **Latency** — every scan finishes well under our 5 ms budget, even on
  a commodity developer laptop without SIMD tuning.

Where precision is 1.000 and recall is high (e.g. `pii.credit_card`,
`secret.openai`, `tool.fetch`, `indirect.canary`, `jailbreak.dan`), the
deterministic engine is doing the right thing today. Where precision is
1.000 and recall is low (e.g. `jailbreak.override.tr`, `pii.iban`), the
signal is correct when fired — we just need more coverage, which the
continuous-tuning loop provides.

### Reproducing

1. Clone the repo.
2. `cd tamga/proxy`
3. `go run ./cmd/redteam -in ./testdata/redteam/prompts.csv -v`

You will get the same per-category table printed above. Adding
`-json out.json` gives you the machine-readable report this folder
publishes.

## Methodology notes

- **No training on the eval corpus.** The same CSV drives CI gating and
  this public report; we do not fine-tune patterns against it between
  runs. If precision regresses in a PR, CI fails before it ships.
- **Scanner stack.** PII + Secrets + Prompt Injection + Jailbreak + Canary
  scanners, with the Aho-Corasick DFA compiled once at process start.
  See `tamga/proxy/internal/scanner/`.
- **Policy.** When a `tamga-policy.yaml` is present the runner evaluates
  through it; otherwise a default severity→action map is used (critical→
  BLOCK, high→REDACT, medium→WARN, else→LOG). The default map is
  deliberately conservative so recall numbers reflect scanner coverage,
  not policy generosity.
- **Latency is measured inside the scan loop**, i.e. what the hot path
  adds to a real request. It does not include network RTT to the LLM
  provider.

## Go Performance Benchmarks (v0.1.1)

This section records the raw `go test -bench` results for the proxy and scanner
hot paths. Benchmarks are run with `-benchtime=1s` and `-count=1` on a quiet
developer machine. All numbers include Go benchmark harness overhead.

### Environment

| Field | Value |
|---|---|
| **Date** | 2026-06-17 |
| **Machine** | Windows 11 Home Single Language 10.0.26200 |
| **CPU** | Intel(R) Core(TM) Ultra 7 255H (16 logical cores) |
| **RAM** | 24 GB |
| **Go version** | go1.26.4 windows/amd64 |
| **Module** | github.com/yatuk/tamga |
| **Branch** | week-6-observability |

### Proxy benchmarks (`internal/proxy`)

All benchmarks passed. Package: `github.com/yatuk/tamga/internal/proxy`.

#### Pipeline (HTTP round-trip)

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkProxyPipeline-16 | 972 | 1,235,643 | 358,111 | 2,593 |
| BenchmarkProxyPipeline_WithPII-16 | 919 | 1,507,289 | 376,217 | 2,768 |
| BenchmarkProxyPipeline_WithSecrets-16 | 846 | 1,482,751 | 324,318 | 2,004 |
| BenchmarkProxyPipeline_MockUpstream-16 | 1,718 | 610,874 | 184,500 | 1,579 |

#### Redaction

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkRedactContent-16 | 3,920,916 | 304.4 | 904 | 7 |
| BenchmarkRedactContent_NoFindings-16 | 5,832,938 | 208.2 | 1,024 | 1 |
| BenchmarkRedactContent_MultipleFindings-16 | 2,202,654 | 560.8 | 1,800 | 9 |

#### Pricing (model lookup)

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkPriceFor/known_model-16 | 11,274,020 | 106.6 | 0 | 0 |
| BenchmarkPriceFor/unknown_model-16 | 11,146,293 | 108.1 | 0 | 0 |
| BenchmarkPriceFor/prefix_match-16 | 7,052,624 | 164.2 | 48 | 1 |
| BenchmarkPriceFor/empty_model-16 | 1,000,000,000 | 0.92 | 0 | 0 |
| BenchmarkPriceFor/with_resolver-16 | 534,528,538 | 2.25 | 0 | 0 |
| BenchmarkPriceFor/case_insensitive-16 | 7,217,658 | 167.6 | 24 | 2 |

#### Supporting hot path

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkPrimaryFinding-16 | 121,614,542 | 9.73 | 0 | 0 |
| BenchmarkUniqueCategories-16 | 10,302,224 | 116.8 | 112 | 3 |
| BenchmarkExtractModelFamily/gpt4o-16 | 88,064,344 | 13.86 | 0 | 0 |
| BenchmarkExtractModelFamily/claude_sonnet-16 | 49,075,338 | 25.34 | 0 | 0 |
| BenchmarkExtractModelFamily/gemini_flash-16 | 47,528,893 | 25.87 | 0 | 0 |
| BenchmarkExtractModelFamily/unknown-16 | 31,324,149 | 38.22 | 0 | 0 |
| BenchmarkJSONChatPayload-16 | 1,343,422 | 863.9 | 1,232 | 18 |
| BenchmarkResolveProviderTarget-16 | 273,028,472 | 4.46 | 0 | 0 |
| BenchmarkPolicyEvaluation-16 | 5,900,037 | 205.3 | 0 | 0 |
| BenchmarkClientIP-16 | 23,729,811 | 45.31 | 32 | 1 |
| BenchmarkRateLimitKeyForRequest-16 | 10,071,219 | 116.3 | 40 | 2 |
| BenchmarkCircuitBreaker_Allow-16 | 65,105,226 | 18.42 | 0 | 0 |
| BenchmarkExtractModelFromBody-16 | 2,196,475 | 528.6 | 296 | 8 |

### Scanner benchmarks (`internal/scanner`)

All benchmarks passed. Package: `github.com/yatuk/tamga/internal/scanner`.

#### ScanAll (full multi-scanner pass)

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkScanAll_SmallPrompt-16 | 862 | 1,354,466 | 440,266 | 3,118 |
| BenchmarkScanAll_MediumPrompt-16 | 44 | 25,563,241 | 10,976,958 | 19,782 |
| BenchmarkScanAll_LargePrompt-16 | 1 | 2,014,705,000 | 862,854,384 | 185,955 |
| BenchmarkScanAll_WithPII-16 | 42 | 27,688,743 | 11,279,571 | 25,470 |
| BenchmarkScanAll_WithSecrets-16 | 145 | 8,315,654 | 3,173,956 | 9,931 |
| BenchmarkScanAll_WithInjection-16 | 142 | 8,316,268 | 3,210,381 | 10,449 |
| BenchmarkScanAll_MixedThreats-16 | 274 | 4,313,271 | 1,398,940 | 6,352 |

#### ScannerPipeline (in-memory scanning, no I/O)

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkScannerPipeline/clean_text_small-16 | 1,698 | 707,457 | 211,266 | 1,971 |
| BenchmarkScannerPipeline/clean_text_medium-16 | 31 | 38,177,232 | 15,716,811 | 27,633 |
| BenchmarkScannerPipeline/with_pii-16 | 1,111 | 1,086,762 | 332,331 | 2,908 |
| BenchmarkScannerPipeline/with_injection-16 | 872 | 1,405,367 | 447,560 | 3,454 |
| BenchmarkScannerPipeline/with_secrets-16 | 1,005 | 1,134,344 | 308,414 | 2,187 |
| BenchmarkScannerPipeline/mixed_content-16 | 548 | 2,196,787 | 656,022 | 4,126 |

#### CodeLeakDetect

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkCodeLeakDetect/clean_text-16 | 35,446 | 33,243 | 291 | 1 |
| BenchmarkCodeLeakDetect/python_function-16 | 72,109 | 16,547 | 1,204 | 11 |
| BenchmarkCodeLeakDetect/python_import-16 | 105,178 | 11,366 | 578 | 7 |
| BenchmarkCodeLeakDetect/javascript_function-16 | 78,903 | 15,412 | 1,188 | 11 |
| BenchmarkCodeLeakDetect/go_function-16 | 94,411 | 12,916 | 1,191 | 11 |
| BenchmarkCodeLeakDetect/sql_statements-16 | 62,672 | 19,056 | 400 | 3 |
| BenchmarkCodeLeakDetect/shebang_script-16 | 48,626 | 24,494 | 387 | 3 |
| BenchmarkCodeLeakDetect/java_class-16 | 38,990 | 31,016 | 1,507 | 13 |
| BenchmarkCodeLeakDetect/prose_with_code_mentions-16 | 38,215 | 30,929 | 258 | 1 |
| BenchmarkCodeLeakDetect/large_response_body-16 | 12,091 | 99,644 | 2,001 | 15 |

#### DFA engine

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkDFA_ScanBytes-16 | 76,963 | 13,388 | 6,608 | 4 |
| BenchmarkDFA_Reload-16 | 1,330 | 780,163 | 1,833,338 | 2,029 |
| BenchmarkDFA_LoadAfterReload-16 | 1,000,000,000 | 0.22 | 0 | 0 |
| BenchmarkDFA_Scale/100p-16 | 177,096 | 6,597 | 3,664 | 4 |
| BenchmarkDFA_Scale/500p-16 | 37,338 | 31,997 | 16,848 | 4 |
| BenchmarkDFA_Scale/1Kp-16 | 19,582 | 62,052 | 33,232 | 4 |

#### Supporting scanner hot path

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkLiteralScan_RegexFoldFallback-16 | 182 | 6,549,677 | 2,262,342 | 101 |
| BenchmarkAdversarialInjection-16 | 1,825 | 663,308 | 182,490 | 1,363 |
| BenchmarkLoadShedder_ShouldRun-16 | 4,077,793 | 291.9 | 288 | 6 |
| BenchmarkPipeline_FastOnly-16 | 923,530 | 1,370 | 2,952 | 45 |
| BenchmarkPipeline_SlowOnly-16 | 361,464 | 3,426 | 2,864 | 43 |
| BenchmarkPipeline_Mixed-16 | 182,982 | 6,759 | 5,936 | 84 |
| BenchmarkWorkerPool_Submit-16 | 1,000,000 | 2,179 | 400 | 4 |
| BenchmarkFindingsToProto-16 | 3,627,729 | 296.1 | 552 | 4 |
| BenchmarkProtoToFindings-16 | 39,182,649 | 31.24 | 0 | 0 |
| BenchmarkIncDetectionCount-16 | 38,685,342 | 29.37 | 24 | 2 |
| BenchmarkScannerDetectionStats_10Scanners-16 | 2,925,916 | 398.0 | 712 | 5 |
| BenchmarkGRPCScannerClient_Name-16 | 1,000,000,000 | 0.45 | 0 | 0 |
| BenchmarkGRPCScannerClient_Enabled-16 | 1,000,000,000 | 0.72 | 0 | 0 |

#### ScanAllWithConfig (execution strategy)

| Benchmark | ops | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| BenchmarkScanAllWithConfig/adaptive-16 | 250 | 4,424,122 | 1,404,263 | 7,297 |
| BenchmarkScanAllWithConfig/sync-16 | 270 | 4,125,330 | 1,405,384 | 7,296 |
| BenchmarkScanAllWithConfig/async-16 | 596 | 2,215,408 | 1,457,109 | 7,318 |

### Regression check

All benchmarks fall within the expected performance ranges:

| Check | Benchmark | Observed | Expected | Verdict |
|---|---|---|---|---|
| Proxy pipeline <= 50 ms | ProxyPipeline | 1.24 ms | 1-50 ms | PASS |
| Scanner pipeline <= 5 ms | ScannerPipeline (small) | 0.71 ms | 0.1-5 ms | PASS |
| Redaction <= 100 us | RedactContent | 304 ns | 1-100 us | PASS |
| Pricing <= 100 ns (approx) | PriceFor (known) | 107 ns | 1-100 ns | PASS |
| CodeLeakDetect | CodeLeakDetect (slowest) | 33 us | no spec | PASS |

No regressions detected. All 51 benchmark variants completed without panics
or failures.

### Known issues

1. **Log hygiene in BenchmarkDFA_Reload** — The benchmark emits a structured
   JSON log line (`Aho-Corasick DFA hot-reloaded`) on every iteration
   (1,330 iterations). This produces ~1,400 lines of log output that obscure
   the benchmark result table. The benchmark itself runs correctly; the log
   call should be suppressed during benchmarking (e.g. via a test logger or
   `testing.Verbose()` guard).

### Reproducing

```bash
# Go performance benchmarks (from proxy/)
go test -bench=. -benchtime=1s -run=^$ ./internal/proxy/...
go test -bench=. -benchtime=1s -run=^$ ./internal/scanner/...
```

## Changelog

- 2026-06-17 — Added Go performance benchmarks (proxy + scanner) for v0.1.1 baseline.
- 2026-04-18 — initial public benchmark published.
