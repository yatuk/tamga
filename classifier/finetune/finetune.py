"""Fine-tune the inline classifier on Turkish and measure it before and after.

Written on 2026-10-11 and run once on 2026-10-17; the result is in
README.md in this directory, which says how to run it.

What it does, in order (``run``):

1. builds the training rows from ``tr_tuning.csv`` (Turkish, written for
   this repository) and the injection and benign rows of ``prompts.csv``
   (the English and Turkish tuning corpus of the rules);
2. scores ``tr_locked.csv``, ``trd_locked.csv`` and ``holdout.csv`` with
   the base model: the "before";
3. fine-tunes the base model on the training rows, holding a tenth of them
   back to pick the checkpoint;
4. scores the same three sets with the result: the "after";
5. writes ``report.md``: Turkish locked-set recall before and after at the
   same false-positive rate, and the same for the English holdout, which
   the fine-tune must not make worse.

The two locked sets are read in steps 2 and 4 only. No row of them, and
nothing derived from a score on them, reaches step 3. ``trd_tuning.csv`` (the
tuning part of the translated deepset set) is not read at all yet: the base
model was trained on the English originals of its rows.

Only the standard library is imported at module level. torch and
transformers are imported inside the functions that need them, so the
service's tests can load this file without either.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import math
import random
import sys
from dataclasses import asdict, dataclass
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
REDTEAM = REPO / "proxy" / "testdata" / "redteam"

# The base is the model the service's published numbers were measured with.
# Its PyTorch weights are not in the repository or in models/: the operator
# download holds the int8 ONNX export only, which cannot be trained.
BASE_MODEL = "Horizon-Labs/prompt-injection-guard-small"
BASE_REVISION = "3215a27"

# The false-positive rate the two models are compared at. With the 125
# benign rows of the locked part, 0.02 is two rows.
DEFAULT_FPR = 0.02
# The threshold the proxy ships with (policy: scan.classifier.threshold).
SHIPPED_THRESHOLD = 0.98

# Categories of prompts.csv that are an instruction to the model. The rest
# of that corpus is PII and secrets, which are not this model's task and
# would teach it that an IBAN is an attack.
_ATTACK_PREFIXES = ("jailbreak.", "injection.", "indirect.", "tool.")
_BENIGN_PREFIX = "benign"


@dataclass(frozen=True)
class Row:
    id: str
    text: str
    label: int  # 1 = injection, 0 = safe
    lang: str
    source: str
    category: str


def read_corpus(path: Path) -> list[dict]:
    """Read a red-team CSV the way proxy/cmd/redteam does.

    Lines that start with ``#`` are the file's header notes, the first record
    is the column names, and ``\\n`` inside a prompt stands for a line break.
    """
    with open(path, encoding="utf-8", newline="") as f:
        lines = [line for line in f if not line.startswith("#")]
    records = list(csv.reader(lines))
    out = []
    for rec in records[1:]:
        if len(rec) < 4:
            continue
        out.append(
            {
                "id": rec[0],
                "category": rec[1],
                "expected_action": rec[2].strip().upper(),
                "text": rec[3].replace("\\n", "\n"),
            }
        )
    return out


def label_for(category: str, expected_action: str) -> int | None:
    """1 for an instruction aimed at the model, 0 for benign text, None for
    a row that is neither (PII, secrets, bulk-data requests)."""
    if category.startswith(("tr.benign.", "trd.benign")):
        return 0
    if category.startswith("trd."):
        return 1 if expected_action != "PASS" else None
    if category.startswith("tr."):
        # Requests for data with no instruction to the model in them are a
        # policy question, not an injection; the classifier is not taught
        # that "list the customers" is an attack.
        return None if category == "tr.exfiltration" else 1
    if category == _BENIGN_PREFIX or category.startswith(_BENIGN_PREFIX + "."):
        return 0 if expected_action == "PASS" else None
    if category.startswith(_ATTACK_PREFIXES):
        return 1 if expected_action != "PASS" else None
    return None


def language_of(category: str, text: str) -> str:
    """"tr" or "en". The Turkish set is Turkish by construction; in the older
    corpora a category ending in .tr or a Turkish letter in the text says so."""
    if category.startswith(("tr.", "trd.")) or category.endswith(".tr"):
        return "tr"
    return "tr" if any(ch in text for ch in "çğıöşüÇĞİÖŞÜ") else "en"


def build_rows(records: list[dict], source: str) -> list[Row]:
    """Turn corpus records into labelled rows, dropping what has no label."""
    rows = []
    for rec in records:
        label = label_for(rec["category"], rec["expected_action"])
        if label is None or not rec["text"].strip():
            continue
        rows.append(
            Row(
                id=rec["id"],
                text=rec["text"],
                label=label,
                lang=language_of(rec["category"], rec["text"]),
                source=source,
                category=rec["category"],
            )
        )
    return rows


def split_train_dev(rows: list[Row], dev_percent: int = 10, seed: str = "tamga-finetune-dev") -> tuple[list[Row], list[Row]]:
    """Hold back a share of the training rows to choose the checkpoint, by a
    hash of the id so that the split is the same on every machine."""
    train, dev = [], []
    for row in rows:
        h = hashlib.sha256(f"{seed}|{row.source}|{row.id}".encode("utf-8")).digest()
        (dev if int.from_bytes(h[:4], "big") % 100 < dev_percent else train).append(row)
    return train, dev


def write_jsonl(path: Path, rows: list[Row]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        for row in rows:
            f.write(json.dumps(asdict(row), ensure_ascii=False) + "\n")


def read_jsonl(path: Path) -> list[Row]:
    with open(path, encoding="utf-8") as f:
        return [Row(**json.loads(line)) for line in f if line.strip()]


def threshold_at_fpr(benign_scores: list[float], fpr: float) -> float:
    """The lowest threshold at which at most ``fpr`` of the benign scores are
    at or above it. When even the highest score is too many (a small set, or
    several rows share it), that is the next number above the highest score,
    so no benign row is flagged. ``math.inf`` only when there are no scores."""
    allowed = math.floor(fpr * len(benign_scores) + 1e-9)
    best = math.inf
    for value in sorted(set(benign_scores), reverse=True):
        if sum(1 for s in benign_scores if s >= value) > allowed:
            if math.isinf(best):
                best = math.nextafter(value, math.inf)
            break
        best = value
    return best


def rate_at(scores: list[float], threshold: float) -> float:
    """Share of the scores at or above the threshold."""
    if not scores:
        return 0.0
    return sum(1 for s in scores if s >= threshold) / len(scores)


def measure(rows: list[Row], scores: list[float], fpr: float) -> dict:
    """Recall at a fixed false-positive rate, and at the shipped threshold."""
    attack = [s for r, s in zip(rows, scores) if r.label == 1]
    benign = [s for r, s in zip(rows, scores) if r.label == 0]
    threshold = threshold_at_fpr(benign, fpr)
    by_category: dict[str, list[float]] = {}
    for r, s in zip(rows, scores):
        by_category.setdefault(r.category, []).append(s)
    return {
        "attacks": len(attack),
        "benign": len(benign),
        "fpr_target": fpr,
        "threshold": threshold,
        "fpr": rate_at(benign, threshold),
        "recall": rate_at(attack, threshold),
        "shipped_threshold": SHIPPED_THRESHOLD,
        "fpr_at_shipped": rate_at(benign, SHIPPED_THRESHOLD),
        "recall_at_shipped": rate_at(attack, SHIPPED_THRESHOLD),
        "by_category": {cat: rate_at(vals, threshold) for cat, vals in sorted(by_category.items())},
    }


def report_markdown(before: dict[str, dict], after: dict[str, dict], meta: dict) -> str:
    """The table the sprint plan's acceptance asks for."""
    lines = [
        "# Fine-tune report",
        "",
        f"Base model: `{meta.get('base_model', '')}` (revision `{meta.get('base_revision', '')}`). "
        f"Training rows: {meta.get('train_rows', '?')} ({meta.get('train_tr', '?')} Turkish). "
        f"Epochs: {meta.get('epochs', '?')}. Seed: {meta.get('seed', '?')}.",
        "",
        "Recall is the share of attack rows scored at or above the threshold. The threshold of each model is the",
        "lowest one at which the false-positive rate on the benign rows of the same set is at most the target, so",
        "the two models are compared at the same false-positive rate and not at the same threshold.",
        "",
        "| Set | Model | Attacks | Benign | FPR target | Threshold | FPR | Recall | "
        f"Recall at {SHIPPED_THRESHOLD} | FPR at {SHIPPED_THRESHOLD} |",
        "|---|---|---|---|---|---|---|---|---|---|",
    ]
    for name in before:
        for label, m in (("before", before[name]), ("after", after[name])):
            threshold = "none" if math.isinf(m["threshold"]) else f"{m['threshold']:.4f}"
            lines.append(
                f"| `{name}` | {label} | {m['attacks']} | {m['benign']} | {m['fpr_target']:.3f} | {threshold} | "
                f"{m['fpr']:.3f} | {m['recall']:.3f} | {m['recall_at_shipped']:.3f} | {m['fpr_at_shipped']:.3f} |"
            )
    lines += [
        "",
        "`tr_locked.csv` and `holdout.csv` were written by the author of Tamga's rules. They are weaker evidence",
        "than a set from someone else; see docs/benchmarks/README.md. `trd_locked.csv` is deepset's test split",
        "translated into Turkish by someone else and corrected for Tamga.",
        "",
    ]
    return "\n".join(lines)


# ── Everything below needs torch and transformers ──────────────────────────


def _load(model_dir: str, revision: str | None):
    from transformers import AutoModelForSequenceClassification, AutoTokenizer

    tokenizer = AutoTokenizer.from_pretrained(model_dir, revision=revision)
    model = AutoModelForSequenceClassification.from_pretrained(model_dir, revision=revision)
    return tokenizer, model


def _attack_index(model) -> int:
    """Index of the attack class, the way the service finds it."""
    benign = {"safe", "benign", "legit", "legitimate", "label_0", "negative", "no_injection"}
    labels = {int(k): str(v).lower() for k, v in (model.config.id2label or {}).items()}
    attack = [i for i, name in labels.items() if name not in benign]
    return attack[0] if len(attack) == 1 else 1


def score_rows(tokenizer, model, rows: list[Row], max_length: int, batch_size: int, device: str) -> list[float]:
    """Attack probability of each row. Texts longer than max_length tokens
    are cut; the rows of the sets this is used on are a few sentences."""
    import torch

    index = _attack_index(model)
    model.eval().to(device)
    scores: list[float] = []
    with torch.no_grad():
        for start in range(0, len(rows), batch_size):
            batch = [r.text for r in rows[start : start + batch_size]]
            enc = tokenizer(batch, truncation=True, max_length=max_length, padding=True, return_tensors="pt").to(device)
            probs = torch.softmax(model(**enc).logits.float(), dim=-1)[:, index]
            scores.extend(float(p) for p in probs.cpu())
    return scores


def train(tokenizer, model, train_rows: list[Row], dev_rows: list[Row], args, device: str) -> None:
    """Fine-tune in place and leave the checkpoint with the lowest dev loss
    in the model."""
    import torch

    index = _attack_index(model)
    other = 1 - index
    random.seed(args.seed)
    torch.manual_seed(args.seed)
    model.to(device)
    optimizer = torch.optim.AdamW(model.parameters(), lr=args.lr, weight_decay=0.01)
    steps = max(1, math.ceil(len(train_rows) / args.batch_size) * args.epochs)
    warmup = max(1, steps // 10)
    schedule = torch.optim.lr_scheduler.LambdaLR(
        optimizer, lambda s: (s + 1) / warmup if s < warmup else max(0.0, (steps - s) / max(1, steps - warmup))
    )

    def batch_loss(batch: list[Row]):
        enc = tokenizer(
            [r.text for r in batch], truncation=True, max_length=args.max_length, padding=True, return_tensors="pt"
        ).to(device)
        target = torch.tensor([index if r.label == 1 else other for r in batch], device=device)
        return torch.nn.functional.cross_entropy(model(**enc).logits.float(), target)

    best_loss, best_state = math.inf, None
    for epoch in range(args.epochs):
        model.train()
        order = train_rows[:]
        random.shuffle(order)
        for start in range(0, len(order), args.batch_size):
            loss = batch_loss(order[start : start + args.batch_size])
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            optimizer.step()
            schedule.step()
            optimizer.zero_grad()
        model.eval()
        with torch.no_grad():
            losses = [
                float(batch_loss(dev_rows[s : s + args.batch_size])) for s in range(0, len(dev_rows), args.batch_size)
            ]
        dev_loss = sum(losses) / max(1, len(losses))
        print(f"epoch {epoch + 1}/{args.epochs}: dev loss {dev_loss:.4f}", file=sys.stderr)
        if dev_loss < best_loss:
            best_loss = dev_loss
            best_state = {k: v.detach().cpu().clone() for k, v in model.state_dict().items()}
    if best_state is not None:
        model.load_state_dict(best_state)


def training_rows() -> list[Row]:
    rows = build_rows(read_corpus(REDTEAM / "tr_tuning.csv"), "tr_tuning")
    rows += build_rows(read_corpus(REDTEAM / "prompts.csv"), "prompts")
    return rows


def evaluation_sets() -> dict[str, list[Row]]:
    return {
        "tr_locked.csv": build_rows(read_corpus(REDTEAM / "tr_locked.csv"), "tr_locked"),
        "trd_locked.csv": build_rows(read_corpus(REDTEAM / "trd_locked.csv"), "trd_locked"),
        "holdout.csv": build_rows(read_corpus(REDTEAM / "holdout.csv"), "holdout"),
    }


def cmd_build_data(args) -> int:
    rows = training_rows()
    train_rows, dev_rows = split_train_dev(rows)
    out = Path(args.out)
    write_jsonl(out / "train.jsonl", train_rows)
    write_jsonl(out / "dev.jsonl", dev_rows)
    for name, held in (("train", train_rows), ("dev", dev_rows)):
        pos = sum(r.label for r in held)
        tr = sum(r.lang == "tr" for r in held)
        print(f"{name}: {len(held)} rows, {pos} injection, {len(held) - pos} safe, {tr} Turkish")
    return 0


def cmd_run(args) -> int:
    import torch

    device = args.device or ("cuda" if torch.cuda.is_available() else "cpu")
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    rows = training_rows()
    train_rows, dev_rows = split_train_dev(rows)
    write_jsonl(out / "train.jsonl", train_rows)
    write_jsonl(out / "dev.jsonl", dev_rows)
    sets = evaluation_sets()

    tokenizer, model = _load(args.base, args.revision)
    before, scores_before = {}, {}
    for name, held in sets.items():
        scores_before[name] = score_rows(tokenizer, model, held, args.max_length, args.batch_size, device)
        before[name] = measure(held, scores_before[name], args.fpr)

    train(tokenizer, model, train_rows, dev_rows, args, device)
    model.save_pretrained(out / "model")
    tokenizer.save_pretrained(out / "model")

    after, scores_after = {}, {}
    for name, held in sets.items():
        scores_after[name] = score_rows(tokenizer, model, held, args.max_length, args.batch_size, device)
        after[name] = measure(held, scores_after[name], args.fpr)

    meta = {
        "base_model": args.base,
        "base_revision": args.revision,
        "train_rows": len(train_rows),
        "train_tr": sum(r.lang == "tr" for r in train_rows),
        "epochs": args.epochs,
        "seed": args.seed,
        "device": device,
    }
    (out / "report.md").write_text(report_markdown(before, after, meta), encoding="utf-8")

    def finite(m: dict) -> dict:
        return {k: (None if isinstance(v, float) and math.isinf(v) else v) for k, v in m.items()}

    (out / "report.json").write_text(
        json.dumps(
            {
                "meta": meta,
                "before": {k: finite(v) for k, v in before.items()},
                "after": {k: finite(v) for k, v in after.items()},
            },
            indent=2,
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )
    print(report_markdown(before, after, meta))
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)

    build = sub.add_parser("build-data", help="write train.jsonl and dev.jsonl; needs no model and no torch")
    build.add_argument("--out", default=str(Path(__file__).resolve().parent / "out"))
    build.set_defaults(func=cmd_build_data)

    run = sub.add_parser("run", help="measure, fine-tune, measure again, write report.md")
    run.add_argument("--base", default=BASE_MODEL, help="model name or a local directory with the PyTorch weights")
    run.add_argument("--revision", default=BASE_REVISION)
    run.add_argument("--out", default=str(Path(__file__).resolve().parent / "out"))
    run.add_argument("--fpr", type=float, default=DEFAULT_FPR)
    run.add_argument("--epochs", type=int, default=3)
    run.add_argument("--lr", type=float, default=2e-5)
    run.add_argument("--batch-size", type=int, default=16)
    run.add_argument("--max-length", type=int, default=512)
    run.add_argument("--seed", type=int, default=20261011)
    run.add_argument("--device", default="")
    run.set_defaults(func=cmd_run)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
