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
below). The fine-tuned model is not in the service: it has not been
exported to ONNX, and no default has changed.

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

**No external dataset is proposed.** The training rows are the two files
above, both written for this repository. The Turkish sets found in the
research (`docs/research/turkish-prompt-injection.md`) are small and three
of the four come from one group; if one is added later, three things have to
be settled first and written into this file:

1. its licence on the day of download, from the dataset card;
2. where its rows come from: `beratcmn/turkish-prompt-injections` is a
   translation of `deepset/prompt-injections`, which the current model's
   card lists both as training data (train split) and as an evaluation set
   (test split);
3. that none of its rows, or a near copy, is in `tr_locked.csv`.

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

## After the run

The number in the report is for the fine-tuned model in PyTorch. The service
runs an int8 ONNX export, and exporting has not been scripted here:

1. Export `out/model/` to ONNX and quantize it the way the base model's
   export was made. The file `onnx/quantization_check.json` in the operator's
   copy says that export quantizes the embeddings only (`gather_only`); how
   to reproduce that exactly has not been worked out.
2. Put the export in a model directory and measure it through the proxy's
   own path: `go run ./cmd/redteam -classifier localhost:50052 -sweep` on
   `prompts.csv` and `holdout.csv`, as `make redteam-classifier-report`
   does. The proxy strips long hashes and keys before asking the
   classifier; this script scores the text as it is.
3. Only then change a default. The shipped threshold (0.98) was chosen as
   the lowest with no false positive on the two older sets for the base
   model; a fine-tuned model needs its own sweep.

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
