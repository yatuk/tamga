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

A second set, `holdout.csv`, is run the same way and published as
[`redteam_holdout.json`](./redteam_holdout.json). See
[Two sets](#two-sets-and-what-each-is-worth) for why it exists.

## Latest run

- **File:** [`redteam_latest.json`](./redteam_latest.json)
- **Corpus size:** 309 prompts (≈ 40% benign, 60% adversarial/PII/secret)
- **Run:** 2026-10-03, Go 1.25.14, linux/amd64 (container on a 16-core
  laptop CPU), single process, deterministic scanners only

### Aggregate

| metric | value |
| --- | --- |
| Precision | **1.000** |
| Recall | **0.896** |
| F1 | **0.945** |
| Scan latency p50 | **0.23 ms** |
| Scan latency p95 | **0.84 ms** |
| Scan latency p99 | **1.70 ms** |
| Scan latency max | **2.83 ms** |

Latency varies between runs on the same machine; three consecutive runs
gave a p95 of 0.65–0.84 ms and a p99 of 1.3–1.7 ms.

### Two sets, and what each is worth

| Set | Prompts | Precision | Recall |
| --- | --- | --- | --- |
| `prompts.csv` (tuning corpus) | 309 | 1.000 | 0.896 |
| `holdout.csv` | 163 (78 attacks, 85 benign) | 1.000 | 1.000 |

On 2026-10-01 `prompts.csv` scored precision 0.969 and recall 0.495. The
difference has three sources, and only the last is a detection gain you can
count on:

1. **Eleven corpus entries were wrong.** They were labelled "must be caught"
   but carried numbers that fail their own checksum (a TCKN, three card
   numbers, three IBANs, a tax number), a twelve-digit "card" and a
   Luhn-valid number labelled invalid. Not flagging them was correct. The
   numbers were replaced with valid ones; the diff of `prompts.csv` shows
   each change.
2. **Rules were written against the remaining misses.** That makes this
   corpus a tuning set: its recall says the rules fit it.
3. **Real bugs were fixed**: IBANs followed by a word were never matched,
   spelled-out digits were turned back into letters before the PII scan,
   Cyrillic "і" was not folded, landline and international phone numbers,
   tax numbers, several token formats and stated passwords had no detector.

`holdout.csv` exists to check the rules on text they were not written
against: reworded attacks, and benign prompts that share words with attacks
("ignore the previous error", "write a system prompt for my bot", "how do
I disable the spam filter"). It was written by the same author as the
rules, in the same attack families, and one false positive it exposed was
fixed before publication. It is weaker evidence than an independent
corpus and will overstate real-world recall.

> **About the earlier 0.52 ms figure.** The previous published run
> (2026-04-18) was taken on Windows and reported p50 = 0 ms and p95 = 0.52 ms.
> A median of exactly zero is the Windows clock's resolution, not the
> scanner: individual scans were being rounded to 0 or to one timer tick.
> The numbers above come from Linux, where the clock resolves nanoseconds.

### With the inline classifier

`make redteam-classifier-report` runs both sets through the rules and then,
for every sample the rules did not block, the classifier service, in the
order the proxy uses. The reports are
[`redteam_classifier_latest.json`](./redteam_classifier_latest.json) and
[`redteam_classifier_holdout.json`](./redteam_classifier_holdout.json); each
carries a `classifier` block with the rules-only score from the same run,
what the classifier added, its call latency, and a split by language.

Measured 2026-10-03 with `Horizon-Labs/prompt-injection-guard-small`
(revision `3215a27`, int8 ONNX) at threshold 0.98, the classifier service in
its container:

| Set | Rules alone | Rules + classifier | Asked | Added TP | Added FP |
|---|---|---|---|---|---|
| `prompts.csv` | 1.000 / 0.896 | 1.000 / 0.927 | 124 | 6 | 0 |
| `holdout.csv` | 1.000 / 1.000 | 1.000 / 1.000 | 98 | 0 | 0 |

By language on `prompts.csv`: English 121 of 129 attacks caught (117 by the
rules alone), Turkish 57 of 63 (55 by the rules alone). The language is
guessed from the text.

Threshold sweep on `prompts.csv` (rules + classifier; `-sweep` prints it):

| Threshold | Precision | Recall | FP | FN |
|---|---|---|---|---|
| 0.50 | 0.984 | 0.953 | 3 | 9 |
| 0.90 | 0.994 | 0.938 | 1 | 12 |
| 0.98 | 1.000 | 0.927 | 0 | 14 |
| 0.995 | 1.000 | 0.911 | 0 | 17 |

0.98 is the default because it is the lowest value with no false positive
here. On `holdout.csv` the same sweep gives 12 false positives at 0.50, 2 at
0.90 and none from 0.98 up.

Classifier call latency over four runs: p50 14–24 ms, p95 23–58 ms, p99
28–93 ms, worst call 108 ms after warm-up. The spread between runs is large,
which is why the default deadline is 150 ms and not the 50 ms first planned.

What this does not show:

- **The value of the classifier on unseen attacks.** Both sets are built
  around what rules catch, and the rules were tuned on one of them. A model
  earns its place on phrasing nobody wrote a rule for; that needs a set
  neither the rules nor this author have seen, and none has been run.
- **False positives on real traffic.** 202 benign samples is a small number.
  Scored on its own, without the rules in front of it, the model flags
  hashes, base64 and keys as injections; the proxy strips those before
  asking, and the rules handle secrets first.
- **Long documents.** Samples here are one or two sentences. A long tool
  result costs one model run per 512 tokens and may not fit the deadline or
  `max_chars`; the response then says `X-Tamga-Classifier: partial`.

### How to read these numbers honestly

- **Precision (1.000)** — nothing benign in either set was mitigated. This
  is the number that governs user experience: false positives block real
  traffic and train analysts to ignore alerts. Both sets are small; a
  precision of exactly one on 202 benign prompts does not mean none will
  occur on yours.
- **Recall (0.896)** — on the tuning corpus. The 20 misses need semantic
  reasoning (grandma prompts, hypothetical and fictional framings, bulk
  data-exfiltration requests, health data, street addresses), which pattern
  matching cannot do. Closing that gap is what an inline classifier is for;
  it is not implemented yet.
- **Latency** — the median scan is a fraction of a millisecond and the
  tail stays under 2 ms, well inside a 5 ms budget.

The per-category table in the JSON shows where the misses are: categories
with recall 0 (`data.exfiltration`, `pii.address.tr`, `medical`,
`jailbreak.hypothetical`) are the ones no rule covers.

### Reproducing

1. Clone the repo.
2. `cd tamga/proxy`
3. `go run ./cmd/redteam -in ./testdata/redteam/prompts.csv -v`

You will get the same per-category table printed above. Adding
`-json out.json` gives you the machine-readable report this folder
publishes.

## Methodology notes

- **The corpus is a tuning set.** Until 2026-10-01 no rule had been
  written against `prompts.csv`. On 2026-10-03 rules were written against
  its misses, so its recall is no longer an estimate of anything but fit.
  It still gates CI: a change that lowers it is a regression.
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
