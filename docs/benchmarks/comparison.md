# Tamga beside other detectors

This page sets Tamga's prompt-injection detection beside LLM Guard's on
public sets, with the same texts given to every system, and says where each
does better. It was measured once, on 2026-10-23, on one laptop. The
numbers are in [`comparison.json`](./comparison.json); the command that
produces them is at the end.

Meta's Llama Prompt Guard 2 and Llama Guard 3 were planned and **were not
run**: their weights are gated (see [Not run](#not-run)).

## Setup

**Systems.** Each runs as its documentation says it runs by default.

| System | What runs | Default verdict |
|---|---|---|
| Tamga rules | The proxy's scanners (PII, secrets, injection, jailbreak, content moderation) and the shipped `proxy/tamga-policy.yaml`, through `proxy/cmd/benchscore`. This is what a deployment does out of the box: the classifier is off by default. | An action on the text's injection findings. A PII or secret finding does not count as an injection caught (but see "any action" below). |
| Tamga rules + classifier | The same, with the optional inline classifier on at its defaults: asked only about a text the rules did not block and found no injection in, after `Prepare`, if it fits `max_chars` (6000). | The rules' action, or a classifier score at or above 0.98 (0.90 for a text with a signal). |
| Tamga classifier alone | `Horizon-Labs/prompt-injection-guard-small`, int8 ONNX (`model_quantized.onnx`, SHA-256 `541471ab…052dae02`), the model the repository's numbers use, through the service's `app.classifier`, on what `Prepare` leaves of each text. Not how the proxy uses it; given for the comparison of models. | Score ≥ 0.98. |
| LLM Guard | `llm-guard` 0.3.16, input scanner `PromptInjection()` with no arguments: model `protectai/deberta-v3-base-prompt-injection-v2` at revision `89b085cd`, threshold 0.92, match type `FULL`, PyTorch on the CPU. | `scan()` returns not valid. |

LLM Guard's scanner also has an `Anonymize` (PII) counterpart; it was not
compared. Tamga's PII rules are about Turkish identifiers, IBANs and cards
checked by checksum, Presidio's about names and English-language entities,
and no public PII set with a licence that allows this use covers both;
counting "found some PII in the text" would say little.

**Scores.** For "recall at a fixed false-positive rate" a system needs a
score. The Tamga rules give none. The Tamga classifier's score is the
service's. LLM Guard's `scan()` rounds its score to two decimals, so the
model's INJECTION probability for the same input is read from its logits
in double precision instead (in float32 many benign rows tie just under
1.0). The threshold is the lowest at which at most 1% of the 1,364 benign
rows of all six sets, pooled, score at or above it (the rule of
`classifier/finetune`); recall on each set is read at that one threshold.

**Same inputs.** Every system gets each text whole, as published; nothing
is cut before it. What each does with a long text is its own default: the
Tamga rules read all of it; the Tamga classifier reads it in windows of
512 tokens (overlap 64, at most 16) and takes the highest score; LLM Guard
with `FULL` reads the first 512 tokens. 78 of the 399 `jackhhao` texts are
longer than 2,000 characters; no text of the other sets is. Exact duplicate
texts within a set were counted once (one in `jackhhao`); no text had two
labels.

**Machine.** Intel Core Ultra 7 255H (16 logical CPUs), 23 GB of memory, Windows 11,
Python 3.14.7, Go 1.27.0, onnxruntime 1.31.0, PyTorch 2.14.1 (CPU),
transformers 4.51.3. Each system ran alone, one text at a time, with its
library's default thread count. Other work was running on the laptop, so
times are indicative only.

## Sets

| Set | Source and revision | Split | Licence | Language | Attacks | Benign |
|---|---|---|---|---|---|---|
| `deepset-test` | `deepset/prompt-injections` @ `4f61ecb` | test | Apache-2.0 | English, German | 60 | 56 |
| `jackhhao-test` | `jackhhao/jailbreak-classification` @ `2f2ceeb` | test (`default/`) | Apache-2.0 | English | 140 | 259 |
| `notinject` | `leolee99/NotInject` @ `847ae76` | `NotInject_one`, `_two`, `_three` | MIT | English | — | 339 |
| `ifeval` | `google/IFEval` @ `966cd89` | all 541 prompts | Apache-2.0 | English | — | 541 |
| `trd_locked` | `proxy/testdata/redteam/trd_locked.csv` (deepset's test split in Turkish) | locked | Apache-2.0 | Turkish | 59 | 44 |
| `tr_locked` | `proxy/testdata/redteam/tr_locked.csv` | locked | this repository | Turkish | 130 | 125 |

`notinject` is benign prompts that contain the words attacks use
("ignore", "bypass", "system prompt"); `ifeval` is ordinary instructions
with formatting constraints. Both measure false positives only. Of the
repository's sets only the locked parts are used: the rules were written
against the tuning parts. In the two Turkish sets, rows that are neither an
attack nor benign by `classifier/finetune`'s labels (bulk-data requests)
are left out.

**Left out, and why.**

- `xTRam1/safe-guard-prompt-injection`: its card states no licence.
- `Lakera/gandalf_ignore_instructions`: the Tamga classifier's model was
  trained on it, and ProtectAI's model card lists eight unnamed MIT sets.
- `databricks/databricks-dolly-15k` as the benign set: ProtectAI's model
  was trained on `VMware/open-instruct`, which includes Dolly. IFEval is
  on neither model's training list.

**Training overlap that remains.** These are the known overlaps; neither
model publishes its exact training rows.

| Set | Tamga classifier's model | LLM Guard's model |
|---|---|---|
| `deepset-test` | trained on deepset's train split; its card says test sets were excluded. No test text is in the train split. | deepset is not among its named sources. |
| `jackhhao-test` | trained on jackhhao's train split. | trained on `jackhhao/jailbreak-classification`, split not stated: the test rows may be training rows. |
| both | 6 texts of jackhhao's test split are also in its train split. | |
| `tr_locked` | not trained on it. The Tamga rules were written by the author of this set, in the same categories as its tuning part: weak evidence for the rules. | not trained on it. |

## Results

Recall and false-positive rate at each system's default, and at the
threshold that flags 1% of the pooled benign rows:

| Set | Attacks | Benign | System | Recall (default) | FPR (default) | Recall at 1% pooled FPR | FPR at that threshold |
|---|---|---|---|---|---|---|---|
| `deepset-test` | 60 | 56 | Tamga rules | 0.100 | 0.000 | — | — |
| | | | Tamga rules + classifier | 0.300 | 0.000 | — | — |
| | | | Tamga classifier alone | 0.217 | 0.000 | 0.250 | 0.000 |
| | | | LLM Guard | 0.317 | 0.000 | 0.083 | 0.000 |
| `jackhhao-test` | 140 | 259 | Tamga rules | 0.457 | 0.012 | — | — |
| | | | Tamga rules + classifier | 0.671 | 0.012 | — | — |
| | | | Tamga classifier alone | 0.307 | 0.000 | 0.493 | 0.004 |
| | | | LLM Guard | 0.786 | 0.019 | 0.136 | 0.000 |
| `notinject` | — | 339 | Tamga rules | — | 0.035 | — | — |
| | | | Tamga rules + classifier | — | 0.044 | — | — |
| | | | Tamga classifier alone | — | 0.012 | — | 0.021 |
| | | | LLM Guard | — | 0.375 | — | 0.038 |
| `ifeval` | — | 541 | Tamga rules | — | 0.000 | — | — |
| | | | Tamga rules + classifier | — | 0.000 | — | — |
| | | | Tamga classifier alone | — | 0.000 | — | 0.000 |
| | | | LLM Guard | — | 0.046 | — | 0.000 |
| `trd_locked` | 59 | 44 | Tamga rules | 0.034 | 0.000 | — | — |
| | | | Tamga rules + classifier | 0.169 | 0.000 | — | — |
| | | | Tamga classifier alone | 0.136 | 0.000 | 0.153 | 0.000 |
| | | | LLM Guard | 0.254 | 0.091 | 0.000 | 0.000 |
| `tr_locked` | 130 | 125 | Tamga rules | 0.800 | 0.024 | — | — |
| | | | Tamga rules + classifier | 0.877 | 0.064 | — | — |
| | | | Tamga classifier alone | 0.515 | 0.032 | 0.615 | 0.040 |
| | | | LLM Guard | 0.446 | 0.448 | 0.015 | 0.000 |

The thresholds at 1% pooled FPR: Tamga classifier 0.9503, LLM Guard
0.999999.

With every action counted, not only those on injection findings, the Tamga
rules flag 1.9% of `jackhhao`'s benign rows (1.2% above), 3.5% of
`notinject`, 2.4% of `tr_locked` and none of the others. Of the 1,753 texts
the classifier would be asked about 1,535, 128 of them with a signal; 3
are longer than `max_chars` and would not be sent.

Time per text, on this machine:

| System | Median | p95 |
|---|---|---|
| Tamga rules | 0.51 ms | 3.2 ms |
| Tamga rules + classifier | 16.6 ms | 82 ms |
| Tamga classifier alone | 17.5 ms | 133 ms |
| LLM Guard | 78 ms | 278 ms |

The rules' time is the mean of repeated scans of each text (Windows'
monotonic clock is too coarse for one). With the classifier, a text the
rules decide costs only the rules, which is why that median is below the
classifier's. The classifier is int8 ONNX in onnxruntime; LLM Guard's
default is the PyTorch model (its `use_onnx=True` option was not measured).
Neither includes a network hop.

## What this says

- **At the same false-positive rate the Tamga classifier catches more on
  every attack set.** At 1% of the pooled benign rows: `deepset` 0.250
  against 0.083, `jackhhao` 0.493 against 0.136, `trd_locked` 0.153
  against 0.000, `tr_locked` 0.615 against 0.015. LLM Guard's model gives
  many benign rows of `notinject` and of the Turkish sets a score close to
  1, so its threshold for 1% has to sit at 0.999999, where little else is
  flagged.
- **At the defaults LLM Guard catches more, and flags much more.** It
  catches more of `deepset` (0.317 against 0.300 for Tamga with the
  classifier), `jackhhao` (0.786 against 0.671) and `trd_locked` (0.254
  against 0.169). It also flags 127 of the 339 `notinject` prompts (37.5%),
  25 of the 541 IFEval prompts (4.6%), 4 of 44 and 56 of 125 benign Turkish
  rows. Tamga with the classifier flags 15, 0, 0 and 8.
- **Tamga out of the box catches little that has no override phrase.**
  The rules alone get 10% of `deepset` and 3.4% of `trd_locked`: most of
  those attacks are a new task slipped after a question, or a persona. The
  classifier is what catches more of them, and it is off by default. Even
  with it on, recall on deepset and its Turkish translation stays at 0.30
  and 0.17. No system here does well on that set.
- **Tamga's rules are not free of false positives.** 12 of the 339
  `notinject` prompts (3.5%) carry phrases the rules act on; the classifier
  adds 3 more. On `tr_locked` the classifier moves the rate from 2.4% to
  6.4%.
- **`tr_locked` favours Tamga.** It was written by the author of Tamga's
  Turkish rules, and the rules catch 80% of it; on `trd_locked`, Turkish
  written by someone else, they catch 3.4%. Read the `trd_locked` row as
  the Turkish result.
- **The two tools are not built for the same input.** ProtectAI's card says
  its model is for English and does not detect jailbreaks; `jackhhao` and
  the Turkish sets are outside that, and it is measured on them because a
  user will meet them. Tamga's classifier is multilingual and was trained
  on jailbreaks. On `jackhhao` both models have seen training data from
  the same source, so that row flatters both.
- **Small sets.** 56 and 44 benign rows give a false-positive rate in steps
  of about 2%; the pooled threshold exists to avoid choosing one on them.
  One run, one machine, no spread.

## Not run

**Llama Prompt Guard 2 (86M, 22M) and Llama Guard 3 1B.** Their Hugging
Face repositories are gated, no token was set, and the downloads were
refused (`GatedRepoError`, 401). The script has the code for them and skips
them with a message when access is refused; that code has not been run. To
measure them, the owner of a Hugging Face account must:

1. accept Meta's licence on
   <https://huggingface.co/meta-llama/Llama-Prompt-Guard-2-86M>,
   <https://huggingface.co/meta-llama/Llama-Prompt-Guard-2-22M> and
   <https://huggingface.co/meta-llama/Llama-Guard-3-1B>, and wait for the
   access to be granted;
2. create a read token in the account settings and run `setx HF_TOKEN <token>`
   in a terminal, then open a new one;
3. run the command below again. Results already computed are read from the
   cache.

How they would be read: Prompt Guard 2 gives the probability of its
"malicious" label from the first 512 tokens; flagged at 0.5 (its card's
argmax), with a score for the fixed-FPR column. Llama Guard 3 is a
content-safety model, not an injection detector: it is asked about the
text as a user turn, and an answer that starts with "unsafe" (any of its
categories S1 to S14) counts as flagged. It gives no score. An injection
that asks for nothing harmful is "safe" to it by design, so its recall
here would measure overlap between the attack sets and harmful content,
not its own quality.

## Reproduce

Install the environment in `scripts/bench/requirements.txt` (its header
says how on Python 3.13 and later), have Go on the PATH and the classifier
model in a directory (docs/operations.md, "Inline classifier"), then from
the repository root:

```bash
python scripts/bench/compare.py --cache /path/to/bench-cache \
    --model-dir models/prompt-injection-guard-small \
    --json docs/benchmarks/comparison.json --md table.md
```

Sets and models are downloaded into `--cache` (`HF_HOME` is set there),
pinned to the revisions above; no row of a public set is written to the
repository. `--systems` runs a subset. The first run took about 15 minutes
on the machine above, most of it LLM Guard. The scoring code is tested
without network or model by `python scripts/bench/test_compare.py`, and the
Go side by `go test ./cmd/benchscore` in `proxy/`.
