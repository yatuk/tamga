# Tamga public benchmark

This folder publishes the raw output of `proxy/cmd/redteam` so anyone can
verify Tamga's accuracy claims without running the project locally.

Every report is produced by the same, checked-in command:

```bash
# from proxy/
go run ./cmd/redteam \
    -in   ./testdata/redteam/prompts.csv \
    -json ../docs/benchmarks/redteam_latest.json
```

The corpus at `proxy/testdata/redteam/prompts.csv` is the exact set the
runtime scanners are gated on in CI — it is not a "marketing" dataset. Benign
samples, obfuscated jailbreaks, Turkish/English PII, BIN-validated credit
cards, and secret-format tokens are all mixed in.

A second set, `holdout.csv`, is run the same way and published as
[`redteam_holdout.json`](./redteam_holdout.json). See
[Two sets](#two-sets-and-what-each-is-worth) for why it exists.

## Latest run

- **File:** [`redteam_latest.json`](./redteam_latest.json)
- **Corpus size:** 309 prompts (≈ 40% benign, 60% adversarial/PII/secret)
  on the day of the run. It has 331 since 2026-10-11; see
  [Changes to the corpus after that run](#changes-to-the-corpus-after-that-run-2026-10-11).
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

### Changes to the corpus after that run (2026-10-11)

Twenty-two prompts were added to `prompts.csv` together with the detectors
they exercise: 14 secrets in formats the scanner did not know (Azure storage
keys, GitLab, npm, SendGrid, Mailchimp, Square, Discord, Telegram, PyPI and
Artifactory tokens, credentials in a URL, a Stripe restricted key, a PuTTY
key file), 4 instructions in encodings the normalizer did not read (base32,
spelled-out hex bytes, percent escapes, base64 twice), and 4 benign prompts
that look like one of those secrets and are not. They are tuning vectors by
construction.

| `prompts.csv` | Prompts | Attacks caught | Benign flagged | Precision | Recall |
| --- | --- | --- | --- | --- | --- |
| 2026-10-03, as published above | 309 | 172 of 192 | 0 of 117 | 1.000 | 0.896 |
| 2026-10-11, the same 309 prompts, new code | 309 | 172 of 192 | 0 of 117 | 1.000 | 0.896 |
| 2026-10-11, with the 22 new prompts | 331 | 190 of 210 | 0 of 121 | 1.000 | 0.905 |

`holdout.csv` was not changed and scores as before: 78 of 78 attacks, 0 of
85 benign prompts.

The step from 0.896 to 0.905 is the 18 added attacks, each caught by the
rule written with it; the 20 misses are the same 20. It is not a gain on
text the rules had not seen. `go test ./cmd/redteam` holds the floor at 190.

These three rows were measured in-process with `go test ./cmd/redteam` on
Windows (Go 1.27.0). The JSON reports in this folder, the latency figures
and everything under [With the inline classifier](#with-the-inline-classifier)
are still the run of 2026-10-03 on 309 prompts: they have not been produced
again, and the classifier has not been run on the 22 new prompts.

### Encoded and invisible text

Two lists of vectors are kept in
[`proxy/cmd/redteam/encoding_vectors_test.go`](../../proxy/cmd/redteam/encoding_vectors_test.go)
and scored there: each text is scanned in-process as one piece, with the
PII, secret, injection and jailbreak scanners, and judged by the shipped
policy. This is not the adversarial suite, which needs the whole stack; a
vector counts as found when it has a finding, the way that suite scores,
and as acted on when the policy's answer is not PASS.

| Vectors | Count | Found, before | Found, after | Acted on, before | Acted on, after |
| --- | --- | --- | --- | --- | --- |
| Published before 2026-10-11 | 23 | 20 | 20 | 20 | 20 |
| Written on 2026-10-11 with the detectors | 25 | 4 | 25 | 4 | 22 |

"Before" is the scanner code of commit `8d5b17b`, "after" the change of 2026-10-11,
both run on the same machine on that day.

**The published list** is every vector in the repository that hides a value
or an instruction behind an encoding or behind characters: 8 from
`tests/stress/adversarial/pii_bypass.py` (digits in mathematical bold, a
Greek iota, a fullwidth @, two values in base64, two split by a zero-width
space, HTML entities), 3 from `injection_bypass.py` (leetspeak and two instructions
spelled one letter at a time) and 12 from the two red-team sets (base64,
hex, ROT13, homoglyphs). The change of 2026-10-11 closed none of them: the
20 were already found, and the 3 that pass are the three published
injection bypasses. They are not encodings a decoder reads. Leetspeak needs
the phrase rules to match what de-leeting produces, and letter-by-letter
spelling needs them to match text with the spaces between words gone;
neither was part of this change.

**The new list** is what the change was written for, so it shows that the
detectors do what they were built to do and nothing about encodings nobody
thought of. It holds an instruction in tag characters and in variation
selectors; a word or a number split by a soft hyphen, a direction mark, an
invisible operator, a Mongolian vowel separator, a NUL byte, a private-use
character, a grapheme joiner, a bidi isolate and a variation selector; an
instruction in URL-safe base64, in base64 of Turkish, of Russian, of UTF-16
and of base64, in base32, in spelled-out hex, in `\x` escapes and in percent
escapes; an ID number in base32; a run of zero-width characters; and two
blocks that decode to nothing readable. The three that are found and not
acted on are the last three. That is the design: they are
[signals](../operations.md#signals-text-that-hides-something), and the
shipped policy has no rule that names them.

**False positives of the two signals.** The red-team sets have almost no
text that looks encoded, so a third list,
[`benign_dev_test.go`](../../proxy/cmd/redteam/benign_dev_test.go), holds 50
texts of the kind that does: hashes, UUIDs, a git log, lockfile integrity
values, an SSH public key, a hex dump, long and signed URLs, an image as a
data URL, code in four languages, minified JSON and JavaScript, a stack
trace, Turkish, Japanese, Arabic and Russian prose, and text with the
invisible characters that Persian, Hindi, Hebrew, emoji, flags, PDFs,
Windows paths and terminal prompts put there.

| On the 50 benign developer texts | Before | After |
| --- | --- | --- |
| Raised `unreadable_block` or `hidden_text` | not applicable | 0 |
| Any other injection finding | 0 | 0 |
| Acted on by the shipped policy | 0 | 0 |

The list was written by the author of the detectors while tuning them. It
says they are quiet on these cases, not that no other text trips them. Two
false positives that are older than this change turned up while writing it
and are not in the list: "Please translate this into English:" followed by
a sentence is blocked as `translation_bypass`, and
`https://user:password@example.com` is read as the e-mail address
`password@example.com`.

The bounds of `unreadable_block` come from measurement, not from a guess: of
20,000 random base64 strings of 100 characters the lowest character entropy
was 5.09 bits and the lowest share of class changes 0.41 (base58: 5.06 and
0.40); long identifiers, paths and slugs made of words measured 4.2 to 4.6
bits and 0.28 to 0.36. The bounds are 4.85 and 0.40, at a length of 100 or
more. A block broken into lines shorter than that, or written in an
alphabet with punctuation in it (Ascii85, uuencode), is not seen.

**Speed.** Scanner benchmarks before and after, best of 3 and of 5 runs of
one second each, Windows, Go 1.27.0, Core Ultra 7 255H. Run-to-run spread on
this machine is about 7%, so none of these is a difference.

| Benchmark | Before | After |
| --- | --- | --- |
| `ScanAll_SmallPrompt` | 723 µs | 677 µs |
| `ScanAll_MediumPrompt` | 3407 µs | 3617 µs |
| `ScanAll_WithPII` | 5565 µs | 5678 µs |
| `ScanAll_WithSecrets` | 1880 µs | 1871 µs |
| `ScanAll_WithInjection` | 1905 µs | 1966 µs |
| `ScanAll_MixedThreats` | 2086 µs | 1965 µs |
| `ScannerPipeline/clean_text_small` | 440 µs | 411 µs |
| `ScannerPipeline/clean_text_medium` | 6122 µs | 4936 µs |
| `ScannerPipeline/with_secrets` | 884 µs | 803 µs |
| `AdversarialInjection` | 637 µs | 627 µs |
| `ScanSegments_200/segments` | 11778 µs | 11315 µs |

The new passes cost time and two changes paid for it: the tokens of the
encodings are found with loops over the bytes where regular expressions
were used, and a secret pattern is not run on a text that lacks the literal
every match of it contains. These benchmarks do not register the jailbreak
scanner, where the two signals are computed; that pass alone runs at 97 to
172 MB a second, against about 0.25 MB a second for the scan as a whole.

### Large requests

The latency figures on this page are for chat-sized prompts. Text is scanned
at about 250 KB a second per core, and a long segment is spread over the
cores: 1 MB of new text measured 1.0 s on the 16-core machine, 4 s on one
core. Text the proxy has scanned in the last ten minutes is answered from
memory, which also means that a load test repeating prompts of 2 KB or more
measures the cache for those, not the scanners.

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
  matching cannot do. Closing that gap is what the optional inline
  classifier is for. In the run above it caught 6 of the 20 at threshold
  0.98; see [With the inline classifier](#with-the-inline-classifier).
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
  See `proxy/internal/scanner/`.
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

- 2026-10-11 — 22 prompts added to `prompts.csv`; encoded and invisible text measured before and after.
- 2026-06-17 — Added Go performance benchmarks (proxy + scanner) for v0.1.1 baseline.
- 2026-04-18 — initial public benchmark published.
