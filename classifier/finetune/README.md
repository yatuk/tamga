# Fine-tuning the inline classifier on Turkish

**It was run once, on 2026-10-17**, after the owner approved the downloads:
on a laptop CPU (16 threads), Python 3.14, torch 2.14.1 (CPU build),
transformers 5.20.1, with the command below and the default seed. The whole
run, model download included, took five minutes. Nothing in it needed a
change. The report it wrote:

| Set | Model | Attacks | Benign | Threshold at FPR ≤ 2% | FPR | Recall | Recall at 0.98 | FPR at 0.98 |
|---|---|---|---|---|---|---|---|---|
| `tr_locked.csv` | before | 130 | 125 | 0.9965 | 0.016 | 0.400 | 0.508 | 0.032 |
| `tr_locked.csv` | after | 130 | 125 | 0.9988 | 0.016 | 0.708 | 0.838 | 0.032 |
| `holdout.csv` | before | 38 | 91 | 0.9931 | 0.011 | 0.842 | 0.921 | 0.011 |
| `holdout.csv` | after | 38 | 91 | 0.9826 | 0.011 | 0.974 | 0.974 | 0.011 |

"before" is the base model, "after" the fine-tuned one, both in 32-bit
PyTorch, not the int8 ONNX the service runs. Both sets were written by the
author of the rules (see "What a good result would and would not show"
below). The fine-tuned model is not in the service and no default has
changed. `export.py` turns it into a directory the service can load; see
"Exporting for the service" below.

Nothing in this directory is part of the classifier service. It is left out
of the image (`classifier/.dockerignore`) and adds nothing to
`classifier/requirements.txt`.

## What the run produces

The sprint plan's acceptance for S6-2: Turkish locked-set recall before and
after fine-tuning, at the same false-positive rate. `finetune.py run` writes
that table to `out/report.md`, and the same two rows for `holdout.csv`,
which is mostly English and which the fine-tune must not make worse.

| Step | Reads | Writes |
|---|---|---|
| Build the training rows | `proxy/testdata/redteam/tr_tuning.csv`, `prompts.csv` | `out/train.jsonl`, `out/dev.jsonl` |
| Score with the base model ("before") | `tr_locked.csv`, `holdout.csv` | kept in memory |
| Fine-tune | `out/train.jsonl`; `out/dev.jsonl` picks the checkpoint | `out/model/` |
| Score with the result ("after") | `tr_locked.csv`, `holdout.csv` | `out/report.md`, `out/report.json` |

The training rows, as `finetune.py build-data` counts them today (this step
needs no model and has been run): 544 for training and 72 held back, 616 in
all, 464 of them Turkish, 267 labelled injection. They are the 351 rows of
`tr_tuning.csv` that are an instruction to the model or benign text, and the
265 rows of `prompts.csv` that are (its PII and secret rows are not this
model's task). Requests for bulk data with no instruction in them
(`tr.exfiltration`, `data.exfiltration`) are left out on both sides: whether
"list all customers" may be asked is a policy question, and a classifier
taught that it is an injection would flag every report request.

The evaluation rows: 255 of `tr_locked.csv` (130 attacks, 125 benign) and
129 of `holdout.csv` (38 attacks, 91 benign).

**The locked part stays locked.** `tr_locked.csv` is read to score, never to
train, never to choose a checkpoint, a learning rate or a number of epochs.
If a run disappoints and something is changed because of it, the next
number on the locked part is no longer a test: say so next to it, or write
a new locked set first. The rule is in
[`docs/benchmarks/README.md`](../../docs/benchmarks/README.md).

**How the two models are compared.** Each model gets the lowest threshold at
which at most 2% of the benign rows of the set score at or above it (two of
125 on the locked part; `--fpr` changes it), and recall is read at that
threshold. The threshold is therefore chosen on the benign rows of the set
it is reported for, which is what "recall at a fixed false-positive rate"
means and is not how a deployment would pick one. The report also gives
both models at 0.98, the threshold the proxy ships with.

## What has to be downloaded

| What | From | Size | Licence | Checked |
|---|---|---|---|---|
| The PyTorch weights, tokenizer and config of `Horizon-Labs/prompt-injection-guard-small`, revision `3215a27` | <https://huggingface.co/Horizon-Labs/prompt-injection-guard-small> | **Not verified.** The model has 141M parameters, which is about 0.56 GB as 32-bit floats; the file's size on the hub was not looked up | Apache-2.0 | The licence is as the model card states it; the copy of the card in `models/prompt-injection-guard-small/README.md` was read on 2026-10-11. Its backbone, `jhu-clsp/mmBERT-small`, is MIT by its own card |
| `torch` and `transformers` (with ModernBERT support; the model's architecture is `ModernBertForSequenceClassification`) | PyPI, see `requirements.txt` here | torch alone is several hundred MB, more with CUDA; not measured | BSD-3-Clause and Apache-2.0 respectively, **stated from general knowledge and not checked on this date** | The repository's licence gate does not read `classifier/finetune/requirements.txt`; check both licences, and those of what they pull in, before installing |

The copy an operator placed in `models/prompt-injection-guard-small/` holds
`onnx/model_quantized.onnx` only. That is the int8 export the service runs;
it cannot be trained, so the weights have to be fetched again in their
PyTorch form.

**The training rows are the two files above**, both written for this
repository. One external set is now in the repository, for evaluation only:
`trd_tuning.csv` and `trd_locked.csv` in `proxy/testdata/redteam/`, the
Turkish translation of `deepset/prompt-injections` published as
`beratcmn/turkish-prompt-injections` (Apache-2.0), corrected row by row on
2026-10-17 (see the header of each file and `NOTICE`). The three things that
had to be settled before adding it:

1. its licence on the day of download: Apache-2.0, per the dataset card at
   commit `c40c38f`;
2. where its rows come from: deepset's train split, whose English originals
   the base model was trained on, is `trd_tuning.csv`; deepset's test split,
   which the model's card lists as an evaluation set and not as training
   data, is `trd_locked.csv`. `run` scores `trd_locked.csv` before and after,
   and reads `trd_tuning.csv` only with `--with-trd-tuning`, which is off by
   default because it made the model worse (next section);
3. that no row of either is in `tr_locked.csv`: none is, and no text is in
   both of its parts (`TestDeepsetTurkishSplit`).

## Commands

From the repository root, in an environment of its own (not the service's):

```bash
python -m venv .venv-finetune && . .venv-finetune/bin/activate
pip install -r classifier/finetune/requirements.txt

# No model needed: write the training rows and look at the counts.
python classifier/finetune/finetune.py build-data

# The run. --base may be a local directory holding the PyTorch weights.
python classifier/finetune/finetune.py run \
    --base Horizon-Labs/prompt-injection-guard-small --revision 3215a27 \
    --epochs 3 --lr 2e-5 --batch-size 16 --fpr 0.02
```

`out/` is not in version control. The report is what gets published, in
`docs/benchmarks/README.md`, with the run's date, machine and seed.

## Expected runtime

Not measured. What can be said honestly:

- The service measures 15 to 30 ms per short message for this model as
  int8 ONNX on a laptop CPU (`docs/benchmarks/README.md`). A training step
  in 32-bit PyTorch costs several times an inference step per row.
- A run is 544 rows for three epochs plus four scoring passes over 384
  rows: about 1,600 training rows and 1,500 scored rows, each a sentence or
  two.
- On that basis a CPU run should take minutes to a few tens of minutes, and
  a run on any current GPU a few minutes, most of it loading the model.
  These are estimates from the row counts, not timings. If the first epoch
  on CPU has not finished after half an hour, something is wrong (most
  likely padding to `--max-length`; the rows are short and batches are
  padded to their longest row).

## Three seeds, with and without the translated rows (2026-10-17)

Six runs, the command above with seeds 20261011, 1 and 2, each once as it
is and once with `--with-trd-tuning` (1,161 training rows instead of 616).
Recall of the fine-tuned model, lowest to highest over the three seeds:

| Training rows | `tr_locked` at FPR ≤ 2% | `tr_locked` at 0.98 | `trd_locked` at 0.98 | `holdout` at FPR ≤ 2% | `holdout` at 0.98 |
|---|---|---|---|---|---|
| none (the base model) | 0.400 | 0.508 | 0.136 | 0.842 | 0.921 |
| `tr_tuning` + `prompts` | 0.654 to 0.708 | 0.654 to 0.885 | 0.169 to 0.186 | 0.974 | 0.921 to 0.974 |
| the same + `trd_tuning` | 0.277 to 0.323 | 0.477 to 0.685 | 0.407 to 0.559 | 0.921 to 0.974 | 0.816 to 0.921 |

The false-positive rate at 0.98 stayed between 0 and 0.040 in every run.
Adding the translated rows lifts the set they come from and costs half
the recall on the Turkish locked set and some on the English holdout, so
they are not used. The seed moves `tr_locked` at 0.98 by up to 0.23: one
run says little about a threshold. The candidate for the service is the
run with the default seed, chosen before these runs, not the best of the
three on a locked set.

## After the run

The number in the report is for the fine-tuned model in PyTorch. The service
runs an int8 ONNX export:

1. Export `out/model/` with `export.py` and check its parity (next
   section).
2. Point the service at the export and measure it through the proxy's
   own path: `go run ./cmd/redteam -classifier localhost:50052 -sweep` on
   `prompts.csv` and `holdout.csv`, as `make redteam-classifier-report`
   does. The proxy strips long hashes and keys before asking the
   classifier; this script scores the text as it is.
3. Only then change a default. The shipped threshold (0.98) was chosen as
   the lowest with no false positive on the two older sets for the base
   model; a fine-tuned model needs its own sweep.

## Exporting for the service

`export.py` (same environment as `finetune.py`, plus `onnx` and
`onnxruntime`; `onnxruntime.quantization` imports `onnx`) has three
commands:

```bash
# PyTorch directory -> the layout the service loads. --out must be new or
# empty; --threads limits torch and onnxruntime.
python classifier/finetune/export.py export \
    --model classifier/finetune/out/model --out /path/to/tamga-tr-export

# Score tr_locked.csv, trd_locked.csv and holdout.csv with the PyTorch
# model, the fp32 ONNX and the int8 ONNX; writes parity.md and parity.json
# into the export directory (--report elsewhere).
python classifier/finetune/export.py parity \
    --model classifier/finetune/out/model --export /path/to/tamga-tr-export

# Whether the service would load a directory, and which file; runs no model.
python classifier/finetune/export.py check /path/to/tamga-tr-export
```

What `export` writes, the same layout as the operator's copy in
`models/prompt-injection-guard-small/`:

| File | What it is |
|---|---|
| `onnx/model_quantized.onnx` | int8, the file the service loads first |
| `onnx/model.onnx` | fp32, for `parity`; the service does not load it while the int8 file is there. `--drop-fp32` deletes it (about 560 MB) |
| `config.json`, `tokenizer.json`, `tokenizer_config.json` | copied; `tokenizer.json` with the padding and truncation the training run saved in it set to none, as in the shipped file (the service turns both off anyway) |
| `export.json` | the settings, the library versions, the size and SHA-256 of each file, and the graph check below |

How it is made, matched to what the shipped `onnx/model_quantized.onnx`
shows when read with `onnx`: the legacy (TorchScript) exporter, opset 17,
inputs `input_ids` and `attention_mask` (int64, `[batch, seq]`), output
`logits` (`[batch, 2]`), then `quantize_dynamic` with
`op_types_to_quantize=["Gather"]`, per-tensor `QUInt8`: only the token
embedding table is int8, every MatMul stays fp32. That is the variant the
operator copy's `quantization_check.json` calls `gather_only`. The export
is traced at 200 tokens and refused if the fp32 graph differs from PyTorch
by more than 1e-3 in the logits at 9 and 700 tokens, so the sequence axis
is known to be dynamic.

`parity` scores all three through the service's own code: the int8 export
is loaded by `app.classifier.Classifier` from the directory, and the fp32
file and the PyTorch model are wrapped with `Classifier.from_parts`, so the
tokenizer, the windows and the softmax are the same for all three. (Without
structlog installed, its log calls are dropped.)

**Pointing the service at the export.** The service reads the directory
named by `TAMGA_CLASSIFIER_MODEL_DIR`; in Compose that is fixed at
`/models/classifier`, and `TAMGA_CLASSIFIER_MODEL_PATH` in `.env` names the
host directory mounted there (default `../models/prompt-injection-guard-small`).
Set `TAMGA_CLASSIFIER_MODEL_NAME` too, so the verdicts name the model they
came from. Nothing in this repository points at an export: the default
model, the threshold and every other default are unchanged.

### One test export, 2026-10-17

`export` and `parity` were run once each on the model of the run at the top
of this file, on the laptop CPU limited to 4 threads (Python 3.14, torch
2.14.1, transformers 5.20.1, onnx 1.23, onnxruntime 1.31). `export` took 42
seconds and `parity` 90.

The int8 file is 268,405,226 bytes (the shipped one 268,409,234), with the
same opset, inputs, outputs, IR version, number of initializers and single
uint8 weight. Its graph has a few nodes fewer (115 Gather and 134 MatMul
against 116 and 135, and some shape arithmetic), which probably comes from the
transformers version tracing the attention mask differently; the
weights that are quantized are the same. The fp32 export differs from
PyTorch by at most 7.6e-6 in the logits at the untraced lengths.

| Set | Rows | PyTorch / int8 max | mean | fp32 / int8 max | PyTorch / fp32 max | Rows across 0.98 |
|---|---|---|---|---|---|---|
| `tr_locked.csv` | 255 | 0.1315 | 0.0020 | 0.1315 | 0.000004 | 0 |
| `trd_locked.csv` | 103 | 0.0388 | 0.0021 | 0.0388 | 0.000003 | 0 |
| `holdout.csv` | 129 | 0.0319 | 0.0008 | 0.0319 | 0.000004 | 0 |

All the difference is quantization: fp32 ONNX matches PyTorch. The largest
differences are on rows scored in the middle (0.72 against 0.59 for
`tr-a-obf-002`), far from 0.98, and no row of any set changes side of 0.98.
The base model's own check (`quantization_check.json`) reports a largest
difference of 0.038 for the same quantization, on its own set.

The int8 model as `finetune.py`'s `measure()` reads it:

| Set | Attacks | Benign | Threshold at FPR ≤ 2% | FPR | Recall | Recall at 0.98 | FPR at 0.98 |
|---|---|---|---|---|---|---|---|
| `tr_locked.csv` | 130 | 125 | 0.9988 | 0.016 | 0.715 | 0.838 | 0.032 |
| `trd_locked.csv` | 59 | 44 | 0.0002 | 0.000 | 0.746 | 0.186 | 0.000 |
| `holdout.csv` | 38 | 91 | 0.9822 | 0.011 | 0.974 | 0.974 | 0.011 |

PyTorch scored through the service's windows gives the same `tr_locked.csv`
and `holdout.csv` numbers as the fine-tune report at the top (0.708 and
0.974 recall), so the service's path and `finetune.py`'s agree on these
short rows. On `trd_locked.csv` 2% of 44 benign rows is less than one row,
so its threshold is just above the highest benign score, which is very low;
at 0.98 the fine-tuned model catches 11 of its 59 attacks. That is a
property of the model, not of the export: PyTorch and int8 agree on it.

Not measured: latency of the export in the service, and the proxy's own
sweep (`cmd/redteam -sweep`) against it.

## What a good result would and would not show

- `tr_locked.csv` was written by the author of the rules and of
  `tr_tuning.csv`, on the same day, in the same ten attack families. A gain
  on it shows the model learned those families in Turkish, not that it
  catches Turkish attacks in general.
- 616 rows is a small set for a 141M-parameter model. The risks are
  forgetting (watch the `holdout.csv` rows of the report) and learning the
  author's phrasing. Three epochs at 2e-5 is a cautious start, not a tuned
  choice.
- The benign half matters as much as the attacks: in Turkish, "talimat",
  "kural" and "yok say" are ordinary words in banking, law and teaching. A
  model that gains recall and flags "düzenli ödeme talimatımı iptal et"
  has become worse for a Turkish bank.
