"""Choose the classifier's threshold from evidence, for one model directory.

The model is scored, through the service's own code (``app.classifier``),
on every row of the tuning sets and on a benign pool as large as the
repository and the machine allow. The command prints recall per attack set
and the false-positive rate per benign set across thresholds, and proposes
the lowest threshold at which at most ``--fpr`` (default 1%) of the pooled
benign rows score at or above it.

The benign pool is every benign row the repository has: those of
``prompts.csv``, ``holdout.csv``, ``tr_tuning.csv``, ``trd_tuning.csv``
and ``discourse_tuning.csv``, and the benign developer texts of
``proxy/cmd/redteam/benign_dev_test.go`` (written out by ``go test``; see
``benign_dev_rows``). With ``--external DIR``, the benign rows of the CSV
files there join it: a local copy of a set whose licence keeps it out of
the repository. Only counts and rates of it are printed or written.

The locked parts (``tr_locked.csv``, ``trd_locked.csv``,
``discourse_locked.csv``) are scored after the threshold is chosen, and
only to report recall and false-positive rate at it and at the shipped
0.98. ``propose`` is never given a locked score.

Texts are sent as the proxy sends them: ``prepare`` is a port of
``proxy/internal/classifier/Prepare``, which takes long key- and hash-like
runs out and skips a text with fewer than 12 letters (scored 0 here, since
the proxy does not ask about it). The rules are not run: these are the
model's numbers, not the proxy's (``go run ./cmd/redteam -classifier
... -sweep`` measures the two together).

Usage (the fine-tune environment, or any with onnxruntime, tokenizers and
numpy)::

    python classifier/finetune/sweep.py MODEL_DIR [--external DIR] [--fpr 0.01]
        [--threads 8] [--json out.json] [--cache scores.json]
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import importlib.util
import json
import math
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
import unicodedata
from dataclasses import dataclass
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
REDTEAM = REPO / "proxy" / "testdata" / "redteam"

DEFAULT_FPR = 0.01
SHIPPED_THRESHOLD = 0.98
THRESHOLDS = (0.5, 0.8, 0.9, 0.95, 0.98, 0.99, 0.995, 0.998, 0.999)

# The tuning sets: their attack rows give recall, their benign rows join the
# pool. holdout.csv was never a locked set and is read here like the others.
TUNING_SETS = ("prompts.csv", "holdout.csv", "tr_tuning.csv", "trd_tuning.csv", "discourse_tuning.csv")
LOCKED_SETS = ("tr_locked.csv", "trd_locked.csv", "discourse_locked.csv")
BENIGN_DEV = "benign_dev"


def _sibling(name: str):
    spec = importlib.util.spec_from_file_location(f"tamga_{name}", HERE / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


finetune = _sibling("finetune")


@dataclass(frozen=True)
class Row:
    source: str
    id: str
    text: str
    label: int  # 1 = attack, 0 = benign


# ── What the proxy sends ────────────────────────────────────────────────────

_OPAQUE_RUN = re.compile(r"[A-Za-z0-9+/=_\-]{24,}")
_MIN_LETTERS = 12


def _is_letter(ch: str) -> bool:
    return unicodedata.category(ch).startswith("L")


def _word_like(run: str) -> bool:
    if any(not _is_letter(ch) and ch not in "-_" for ch in run):
        return False
    upper = sum(1 for ch in run if unicodedata.category(ch) == "Lu")
    lower = sum(1 for ch in run if unicodedata.category(ch) == "Ll")
    return upper == 0 or lower == 0 or upper <= 2


def prepare(text: str) -> str:
    """What the proxy sends the classifier for this text; "" when nothing.

    A port of ``Prepare`` in proxy/internal/classifier/prepare.go: a long
    unbroken run of key-, hash- or base64-like characters is replaced by a
    space unless it is a word, and a text with fewer than 12 letters left
    is not sent at all.
    """
    cleaned = text
    if _OPAQUE_RUN.search(text):
        cleaned = _OPAQUE_RUN.sub(lambda m: m.group(0) if _word_like(m.group(0)) else " ", text)
    letters = 0
    for ch in cleaned:
        if _is_letter(ch):
            letters += 1
            if letters >= _MIN_LETTERS:
                return cleaned.strip()
    return ""


# ── The rows ────────────────────────────────────────────────────────────────


def label_of(category: str, expected_action: str) -> int | None:
    """1 for an attack row, 0 for a benign one, None for neither. The
    fine-tune's labels, and those of the discourse sets (``ds.``)."""
    if category.startswith("ds.benign"):
        return 0 if expected_action == "PASS" else None
    if category.startswith("ds.attack"):
        return 1
    return finetune.label_for(category, expected_action)


def corpus_rows(name: str, path: Path | None = None) -> list[Row]:
    rows = []
    for rec in finetune.read_corpus(path or REDTEAM / name):
        label = label_of(rec["category"], rec["expected_action"])
        if label is None or not rec["text"].strip():
            continue
        rows.append(Row(source=name, id=rec["id"], text=rec["text"], label=label))
    return rows


def benign_dev_rows(path: Path | None = None) -> tuple[list[Row], str]:
    """The benign developer texts, from a JSON-lines file or, without one,
    written by ``go test`` into a temporary file. Returns the rows and a
    note when they could not be had."""
    if path is None:
        go = shutil.which("go")
        if go is None:
            return [], "go is not on PATH: the benign developer texts are left out"
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "benign_dev.jsonl"
            env = dict(os.environ, TAMGA_BENIGN_DEV_OUT=str(out))
            proc = subprocess.run(
                [go, "test", "./cmd/redteam", "-run", "^TestWriteBenignDevTexts$", "-count=1"],
                cwd=REPO / "proxy", env=env, capture_output=True, text=True,
            )
            if proc.returncode != 0 or not out.is_file():
                return [], "go test could not write the benign developer texts: they are left out"
            return _read_dev(out), ""
    return _read_dev(path), ""


def _read_dev(path: Path) -> list[Row]:
    rows = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.strip():
                rec = json.loads(line)
                rows.append(Row(source=BENIGN_DEV, id=rec["id"], text=rec["text"], label=0))
    return rows


def external_rows(directory: Path) -> list[Row]:
    """Rows of the CSV files in ``directory`` (columns id, category,
    expected_action, prompt) whose category is ``attack`` or ``benign``.
    Other categories (harmful requests, toxicity) are not injections and are
    left out. An exact duplicate text is counted once per file."""
    rows = []
    # Some jailbreak prompts are longer than the csv module's default limit.
    csv.field_size_limit(min(sys.maxsize, 2**31 - 1))
    for path in sorted(directory.glob("*.csv")):
        seen = set()
        with open(path, encoding="utf-8", newline="") as f:
            for rec in csv.DictReader(f):
                category = (rec.get("category") or "").strip().lower()
                if category not in ("attack", "benign"):
                    continue
                text = (rec.get("prompt") or "").replace("\\n", "\n")
                if not text.strip() or text in seen:
                    continue
                seen.add(text)
                rows.append(Row(source=f"local:{path.stem}", id=rec.get("id", ""), text=text, label=int(category == "attack")))
    return rows


# ── Numbers ─────────────────────────────────────────────────────────────────


def rate_at(scores: list[float], threshold: float) -> float:
    return finetune.rate_at(scores, threshold)


def propose(pool: list[float], fpr: float) -> dict:
    """The lowest threshold at which at most ``fpr`` of the pooled benign
    scores are at or above it, exact and rounded up to three decimals (a
    value for the policy file; rounding up can only lower the rate)."""
    exact = finetune.threshold_at_fpr(pool, fpr)
    if math.isinf(exact) or exact > 1.0:
        return {"exact": None, "threshold": None, "fpr": 0.0, "benign": len(pool)}
    rounded = round(exact, 3)
    if rounded < exact:
        rounded = round(rounded + 0.001, 3)
    rounded = min(1.0, rounded)
    return {"exact": exact, "threshold": rounded, "fpr": rate_at(pool, rounded), "benign": len(pool)}


def by_set(rows: list[Row], scores: dict[str, float], label: int) -> dict[str, list[float]]:
    out: dict[str, list[float]] = {}
    for r in rows:
        if r.label == label:
            out.setdefault(r.source, []).append(scores[r.text])
    return out


# ── Scoring ─────────────────────────────────────────────────────────────────


def score_texts(model_dir: str, texts: list[str], threads: int, cache_path: Path | None) -> tuple[dict[str, float], str]:
    """Score every distinct text once. Texts ``prepare`` empties score 0.
    Scores are cached by a hash of the prepared text and the model file, so
    a second run reads them back; the cache holds no text."""
    export = _sibling("export")
    clf_mod = export.service_module()
    clf = clf_mod.Classifier(model_dir=model_dir, threads=threads)
    if not clf.available:
        raise SystemExit(f"{model_dir}: {clf.reason}")
    model_file = next(Path(model_dir) / f for f in clf_mod._MODEL_FILES if (Path(model_dir) / f).is_file())
    salt = f"{model_file.stat().st_size}:{model_file.stat().st_mtime_ns}"

    cache: dict[str, float] = {}
    if cache_path and cache_path.is_file():
        stored = json.loads(cache_path.read_text(encoding="utf-8"))
        if stored.get("salt") == salt:
            cache = stored.get("scores", {})

    prepared = {t: prepare(t) for t in texts}
    todo = sorted({p for p in prepared.values() if p and _key(p) not in cache})
    started = time.perf_counter()
    batch = 32
    for i in range(0, len(todo), batch):
        chunk = todo[i : i + batch]
        for p, s in zip(chunk, clf.score(chunk)):
            cache[_key(p)] = s.score
        done = min(i + batch, len(todo))
        if done % 512 < batch or done == len(todo):
            print(f"scored {done}/{len(todo)} in {time.perf_counter() - started:.0f} s", file=sys.stderr)
            if cache_path:
                cache_path.parent.mkdir(parents=True, exist_ok=True)
                cache_path.write_text(json.dumps({"salt": salt, "scores": cache}), encoding="utf-8")
    scores = {t: (cache[_key(p)] if p else 0.0) for t, p in prepared.items()}
    return scores, clf.model_name


def _key(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


# ── Report ──────────────────────────────────────────────────────────────────


def sweep_report(tuning: list[Row], pool_rows: list[Row], scores: dict[str, float], fpr: float) -> dict:
    """Recall and false-positive rate across THRESHOLDS, and the proposal.
    Reads only tuning rows and the benign pool."""
    attacks = by_set(tuning, scores, 1)
    benign = by_set(pool_rows, scores, 0)
    pooled = [s for vals in benign.values() for s in vals]
    repo_pool = [s for name, vals in benign.items() if not name.startswith("local:") for s in vals]
    proposal = propose(pooled, fpr)
    grid = sorted(set(THRESHOLDS) | ({proposal["threshold"]} if proposal["threshold"] is not None else set()))
    return {
        "fpr_target": fpr,
        "thresholds": grid,
        "recall": {name: {"rows": len(v), "at": [rate_at(v, t) for t in grid]} for name, v in attacks.items()},
        "fpr": {name: {"rows": len(v), "at": [rate_at(v, t) for t in grid]} for name, v in benign.items()},
        "pooled_fpr": {"rows": len(pooled), "at": [rate_at(pooled, t) for t in grid]},
        "proposal": proposal,
        "proposal_repository_only": propose(repo_pool, fpr),
    }


def locked_report(locked: list[Row], scores: dict[str, float], thresholds: list[float]) -> dict:
    """Recall and false-positive rate of the locked parts at the given
    thresholds. Called after the threshold is chosen, never before."""
    out = {}
    for name in LOCKED_SETS:
        att = [scores[r.text] for r in locked if r.source == name and r.label == 1]
        ben = [scores[r.text] for r in locked if r.source == name and r.label == 0]
        out[name] = {
            "attacks": len(att),
            "benign": len(ben),
            "at": {f"{t:g}": {"recall": rate_at(att, t), "fpr": rate_at(ben, t)} for t in thresholds},
        }
    return out


def markdown(rep: dict, model: str) -> str:
    grid = rep["thresholds"]
    head = "| Set | Rows | " + " | ".join(f"{t:g}" for t in grid) + " |"
    sep = "|---|---|" + "---|" * len(grid)
    lines = [f"Model: `{model}`", "", "Recall per attack set:", "", head, sep]
    for name, v in rep["recall"].items():
        lines.append(f"| `{name}` | {v['rows']} | " + " | ".join(f"{x:.3f}" for x in v["at"]) + " |")
    lines += ["", "False-positive rate per benign set:", "", head, sep]
    for name, v in rep["fpr"].items():
        lines.append(f"| `{name}` | {v['rows']} | " + " | ".join(f"{x:.3f}" for x in v["at"]) + " |")
    p = rep["pooled_fpr"]
    lines.append(f"| pooled | {p['rows']} | " + " | ".join(f"{x:.4f}" for x in p["at"]) + " |")
    lines.append("")
    for label, prop in (("pooled benign rows", rep["proposal"]), ("repository rows only", rep["proposal_repository_only"])):
        if prop["threshold"] is None:
            lines.append(f"Proposed threshold ({label}): none below 1 meets FPR <= {rep['fpr_target']:g}.")
        else:
            lines.append(
                f"Proposed threshold ({label}, FPR <= {rep['fpr_target']:g}): {prop['threshold']:.3f} "
                f"(exact {prop['exact']:.6f}; FPR {prop['fpr']:.4f} on {prop['benign']} rows)."
            )
    if "locked" in rep:
        lines += ["", "Locked parts, scored after the choice:", "", "| Set | Attacks | Benign | Threshold | Recall | FPR |", "|---|---|---|---|---|---|"]
        for name, v in rep["locked"].items():
            for t, m in v["at"].items():
                lines.append(f"| `{name}` | {v['attacks']} | {v['benign']} | {t} | {m['recall']:.3f} | {m['fpr']:.3f} |")
    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("model_dir", help="a directory the service loads (onnx/model_quantized.onnx, tokenizer.json, config.json)")
    parser.add_argument("--fpr", type=float, default=DEFAULT_FPR, help="the false-positive rate the proposal may not exceed on the pooled benign rows")
    parser.add_argument("--external", type=Path, default=None, help="a directory of CSV files whose benign rows join the pool and whose attack rows are reported (local measurement only)")
    parser.add_argument("--benign-dev", type=Path, default=None, help="the benign developer texts as JSON lines; without it they are written by go test")
    parser.add_argument("--threads", type=int, default=0, help="onnxruntime threads; 0 lets it choose")
    parser.add_argument("--cache", type=Path, default=None, help="a JSON file of scores by text hash, read and written")
    parser.add_argument("--json", type=Path, default=None, help="write the numbers (no text) here")
    parser.add_argument("--no-locked", action="store_true", help="do not score the locked parts")
    args = parser.parse_args(argv)

    tuning = [r for name in TUNING_SETS for r in corpus_rows(name)]
    dev, note = benign_dev_rows(args.benign_dev)
    if note:
        print(note, file=sys.stderr)
    pool_rows = [r for r in tuning if r.label == 0] + dev
    if args.external is not None:
        ext = external_rows(args.external)
        tuning += [r for r in ext if r.label == 1]
        pool_rows += [r for r in ext if r.label == 0]
    locked = [] if args.no_locked else [r for name in LOCKED_SETS for r in corpus_rows(name)]

    texts = sorted({r.text for r in tuning + pool_rows + locked})
    scores, model = score_texts(args.model_dir, texts, args.threads, args.cache)

    rep = sweep_report(tuning, pool_rows, scores, args.fpr)
    if locked:
        chosen = [t for t in (rep["proposal"]["threshold"], SHIPPED_THRESHOLD) if t is not None]
        rep["locked"] = locked_report(locked, scores, chosen)
    rep["model"] = model
    print(markdown(rep, model))
    if args.json:
        args.json.write_text(json.dumps(rep, indent=2), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
