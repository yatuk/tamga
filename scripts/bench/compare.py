"""Tamga beside other prompt-injection detectors, on public sets.

Every system reads the same texts. For each set and system the command
reports recall and false-positive rate at the system's documented default,
recall at one fixed false-positive rate for the systems that give a score,
and the median time per text on this machine. The result is written as
JSON (numbers only, no text of any set) and as the Markdown table in
docs/benchmarks/comparison.md.

Systems:

- ``tamga_rules``: the proxy's scanners and the shipped policy, as the
  proxy runs them by default (no classifier). Run by ``proxy/cmd/benchscore``.
- ``tamga_rules_classifier``: the same with the optional inline classifier
  on at its defaults (threshold 0.98, 0.90 for a text with a signal,
  max_chars 6000), asked only about what the rules let through.
- ``tamga_classifier``: the classifier model alone, on what the proxy's
  ``Prepare`` leaves of each text, through the service's ``app.classifier``.
- ``llm_guard``: LLM Guard's ``PromptInjection`` input scanner with its
  defaults (its default model, threshold 0.92, match type FULL).
- ``prompt_guard_2_86m``, ``prompt_guard_2_22m``: Meta's Llama Prompt Guard 2
  (gated: needs a Hugging Face token whose account accepted Meta's licence).
- ``llama_guard_3_1b``: Meta's Llama Guard 3 1B, a content-safety model;
  "unsafe" counts as flagged (gated, as above).

Usage (an environment with llm-guard, onnxruntime, tokenizers, pyarrow and
huggingface_hub; Go on the PATH)::

    python scripts/bench/compare.py --cache C:/tmp/bench-cache \\
        --model-dir models/prompt-injection-guard-small \\
        --json docs/benchmarks/comparison.json --md comparison-table.md

Downloads go to ``--cache`` (``HF_HOME`` is set there unless it is set
already). Per-text results are cached there too, so a second run only
scores what is new. Only the standard library is imported at module level;
the tests (scripts/bench/test_compare.py) use no network and no model.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import math
import os
import platform
import statistics
import subprocess
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
REDTEAM = REPO / "proxy" / "testdata" / "redteam"

DEFAULT_FPR = 0.01
# The proxy's classifier defaults (proxy/internal/policy/classifier.go).
TAMGA_THRESHOLD = 0.98
TAMGA_SIGNAL_THRESHOLD = 0.90
TAMGA_MAX_CHARS = 6000


def _load(name: str, path: Path):
    if name in sys.modules:
        return sys.modules[name]
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


# classifier/finetune/finetune.py imports only the standard library at
# module level; its corpus reader, labels and threshold rule are reused so
# that these numbers are read the way the repository's other ones are.
finetune = _load("tamga_finetune", REPO / "classifier" / "finetune" / "finetune.py")


# ── Sets ────────────────────────────────────────────────────────────────────


@dataclass(frozen=True)
class Item:
    id: str
    text: str
    label: int  # 1 = attack, 0 = benign


@dataclass
class DataSet:
    name: str
    source: str
    revision: str
    split: str
    licence: str
    items: list[Item]
    language: str
    duplicates: int = 0
    conflicts: int = 0
    notes: list[str] = field(default_factory=list)

    @property
    def attacks(self) -> int:
        return sum(1 for i in self.items if i.label == 1)

    @property
    def benign(self) -> int:
        return sum(1 for i in self.items if i.label == 0)


def dedupe(pairs: list[tuple[str, int]]) -> tuple[list[tuple[str, int]], int, int]:
    """Keep each exact text once. A text that appears with both labels is
    dropped altogether. Returns the rows, the number of repeated rows
    removed, and the number of texts dropped for conflicting labels."""
    labels: dict[str, set[int]] = {}
    order: list[str] = []
    for text, label in pairs:
        if text not in labels:
            labels[text] = set()
            order.append(text)
        labels[text].add(label)
    kept = [(t, next(iter(labels[t]))) for t in order if len(labels[t]) == 1]
    conflicts = sum(1 for t in order if len(labels[t]) > 1)
    return kept, len(pairs) - len(order), conflicts


def _norm(text: str) -> str:
    return " ".join(text.lower().split())


def overlap(test: list[str], train: list[str]) -> int:
    """How many test texts appear in the train texts, ignoring case and
    whitespace."""
    seen = {_norm(t) for t in train}
    return sum(1 for t in test if _norm(t) in seen)


def _make(name: str, pairs: list[tuple[str, int]], **meta) -> DataSet:
    pairs = [(t, label) for t, label in pairs if t and t.strip()]
    kept, dup, conflicts = dedupe(pairs)
    items = [Item(id=f"{name}-{i:05d}", text=t, label=label) for i, (t, label) in enumerate(kept)]
    return DataSet(name=name, items=items, duplicates=dup, conflicts=conflicts, **meta)


# Public sets, pinned by revision. Licences as stated on each dataset card
# at that revision.
DEEPSET = ("deepset/prompt-injections", "4f61ecb038e9c3fb77e21034b22511b523772cdd")
JACKHHAO = ("jackhhao/jailbreak-classification", "2f2ceeb39658696fd3f462403562b6eea5306287")
NOTINJECT = ("leolee99/NotInject", "847ae76cf8fea5ed325429e569ae8cfef022d2e0")
IFEVAL = ("google/IFEval", "966cd89545d6b6acfd7638bc708b98261ca58e84")


def _hf_file(repo: tuple[str, str], filename: str) -> str:
    from huggingface_hub import hf_hub_download

    return hf_hub_download(repo[0], filename, repo_type="dataset", revision=repo[1])


def _parquet(path: str, columns: list[str]) -> list[dict]:
    import pyarrow.parquet as pq

    return pq.read_table(path, columns=columns).to_pylist()


def load_public_sets() -> list[DataSet]:
    import csv

    sets = []

    test = _parquet(_hf_file(DEEPSET, "data/test-00000-of-00001-701d16158af87368.parquet"), ["text", "label"])
    train = _parquet(_hf_file(DEEPSET, "data/train-00000-of-00001-9564e8b05b4757ab.parquet"), ["text"])
    ds = _make(
        "deepset-test",
        [(r["text"], int(r["label"])) for r in test],
        source=DEEPSET[0], revision=DEEPSET[1], split="test", licence="Apache-2.0", language="en, de",
    )
    ds.notes.append(f"{overlap([i.text for i in ds.items], [r['text'] for r in train])} texts also in the train split")
    sets.append(ds)

    def jack(file: str) -> list[dict]:
        with open(_hf_file(JACKHHAO, file), encoding="utf-8", newline="") as f:
            return list(csv.DictReader(f))

    test, train = jack("default/jailbreak_dataset_test.csv"), jack("default/jailbreak_dataset_train.csv")
    ds = _make(
        "jackhhao-test",
        [(r["prompt"], 1 if r["type"] == "jailbreak" else 0) for r in test],
        source=JACKHHAO[0], revision=JACKHHAO[1], split="test (default)", licence="Apache-2.0", language="en",
    )
    ds.notes.append(f"{overlap([i.text for i in ds.items], [r['prompt'] for r in train])} texts also in the train split")
    sets.append(ds)

    rows = []
    for part in ("one", "two", "three"):
        rows += _parquet(_hf_file(NOTINJECT, f"data/NotInject_{part}-00000-of-00001.parquet"), ["prompt"])
    sets.append(_make(
        "notinject",
        [(r["prompt"], 0) for r in rows],
        source=NOTINJECT[0], revision=NOTINJECT[1], split="NotInject_one, _two, _three", licence="MIT", language="en",
    ))

    with open(_hf_file(IFEVAL, "ifeval_input_data.jsonl"), encoding="utf-8") as f:
        rows = [json.loads(line) for line in f if line.strip()]
    sets.append(_make(
        "ifeval",
        [(r["prompt"], 0) for r in rows],
        source=IFEVAL[0], revision=IFEVAL[1], split="train (all 541 prompts)", licence="Apache-2.0", language="en",
    ))
    return sets


def load_repo_sets() -> list[DataSet]:
    """The locked Turkish parts in proxy/testdata/redteam. Only the locked
    parts: the rules were tuned on the others. Rows labelled as
    classifier/finetune labels them (exfiltration requests are neither)."""
    sets = []
    for name, source, licence in (
        ("trd_locked", "proxy/testdata/redteam/trd_locked.csv (beratcmn/turkish-prompt-injections, corrected)", "Apache-2.0"),
        ("tr_locked", "proxy/testdata/redteam/tr_locked.csv", "AGPL-3.0 (this repository)"),
    ):
        pairs = []
        for rec in finetune.read_corpus(REDTEAM / f"{name}.csv"):
            label = finetune.label_for(rec["category"], rec["expected_action"])
            if label is not None:
                pairs.append((rec["text"], label))
        sets.append(_make(name, pairs, source=source, revision="this commit", split="locked", licence=licence, language="tr"))
    return sets


# ── Metrics ─────────────────────────────────────────────────────────────────


@dataclass
class Result:
    """One system's answer for one text."""

    flagged: bool
    score: float | None
    ms: float


def rate(values: list[bool]) -> float | None:
    return sum(values) / len(values) if values else None


def set_metrics(items: list[Item], results: dict[str, Result], threshold: float | None) -> dict:
    """Recall and FPR at the system's default, and at ``threshold`` (the
    one fixed by the pooled benign rows) when the system gives a score."""
    attacks = [results[i.id] for i in items if i.label == 1]
    benign = [results[i.id] for i in items if i.label == 0]
    out = {
        "attacks": len(attacks),
        "benign": len(benign),
        "recall": rate([r.flagged for r in attacks]),
        "fpr": rate([r.flagged for r in benign]),
    }
    if threshold is not None and all(r.score is not None for r in attacks + benign):
        out["recall_at_fixed_fpr"] = rate([r.score >= threshold for r in attacks])
        out["fpr_at_fixed_fpr"] = rate([r.score >= threshold for r in benign])
    return out


def pooled_threshold(sets: list[DataSet], results: dict[str, Result], fpr: float) -> float | None:
    """The lowest threshold at which at most ``fpr`` of every benign row of
    every set score at or above it (classifier/finetune's rule), or None
    for a system without scores."""
    scores = [results[i.id].score for s in sets for i in s.items if i.label == 0]
    if not scores or any(v is None for v in scores):
        return None
    return finetune.threshold_at_fpr(scores, fpr)


def latency(results: dict[str, Result]) -> dict:
    ms = sorted(r.ms for r in results.values())
    if not ms:
        return {}
    return {
        "median_ms": statistics.median(ms),
        "p95_ms": ms[min(len(ms) - 1, math.ceil(0.95 * len(ms)) - 1)],
        "texts": len(ms),
    }


def tamga_with_classifier(verdict: dict, score: float) -> bool:
    """Whether the proxy, with the classifier on at its defaults, acts on
    the text as an injection: the rules did, or the classifier was asked
    and scored at or above the threshold that applies to the text."""
    if verdict["injection_action"] != "PASS":
        return True
    prepared = verdict.get("prepared") or ""
    if not verdict.get("ask_classifier") or len(prepared.encode("utf-8")) > TAMGA_MAX_CHARS:
        return False
    threshold = TAMGA_SIGNAL_THRESHOLD if verdict.get("signal") else TAMGA_THRESHOLD
    return score >= threshold and verdict.get("classifier_action", "PASS") != "PASS"


# ── Systems ─────────────────────────────────────────────────────────────────


def _key(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


class Cache:
    """Per-text results of one system, by a hash of the text. Holds no text."""

    def __init__(self, path: Path):
        self.path = path
        self.data: dict[str, dict] = {}
        if path.is_file():
            self.data = json.loads(path.read_text(encoding="utf-8"))

    def get(self, text: str) -> dict | None:
        return self.data.get(_key(text))

    def put(self, text: str, value: dict) -> None:
        self.data[_key(text)] = value

    def save(self) -> None:
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.path.write_text(json.dumps(self.data), encoding="utf-8")


def run_benchscore(items: list[Item], work: Path) -> dict[str, dict]:
    """Tamga's rules, through proxy/cmd/benchscore, one verdict per item."""
    work.mkdir(parents=True, exist_ok=True)
    src, dst = work / "texts.jsonl", work / "verdicts.jsonl"
    with open(src, "w", encoding="utf-8", newline="\n") as f:
        for i in items:
            f.write(json.dumps({"id": i.id, "text": i.text}, ensure_ascii=False) + "\n")
    exe = work / ("benchscore.exe" if os.name == "nt" else "benchscore")
    subprocess.run(["go", "build", "-o", str(exe), "./cmd/benchscore"], cwd=REPO / "proxy", check=True)
    subprocess.run([str(exe), "-in", str(src), "-out", str(dst), "-policy", "tamga-policy.yaml"], cwd=REPO / "proxy", check=True)
    with open(dst, encoding="utf-8") as f:
        return {v["id"]: v for v in map(json.loads, f) if v}


def score_loop(name: str, items: list[Item], cache: Cache, fn) -> dict[str, Result]:
    """Run ``fn(text) -> (flagged, score)`` on each distinct text, one at a
    time, timing each call (or ``(flagged, score, ms)`` when ``fn`` times
    itself); cached results are read back."""
    out = {}
    started = time.perf_counter()
    for n, item in enumerate(items, 1):
        hit = cache.get(item.text)
        if hit is None:
            t0 = time.perf_counter()
            answer = fn(item.text)
            ms = answer[2] if len(answer) > 2 else (time.perf_counter() - t0) * 1000
            hit = {"flagged": bool(answer[0]), "score": answer[1], "ms": ms}
            cache.put(item.text, hit)
        out[item.id] = Result(hit["flagged"], hit["score"], hit["ms"])
        if n % 200 == 0 or n == len(items):
            cache.save()
            print(f"{name}: {n}/{len(items)} in {time.perf_counter() - started:.0f} s", file=sys.stderr, flush=True)
    return out


def tamga_classifier(model_dir: str):
    export = _load("tamga_export", REPO / "classifier" / "finetune" / "export.py")
    mod = export.service_module()
    clf = mod.Classifier(model_dir=model_dir)
    if not clf.available:
        raise SystemExit(f"{model_dir}: {clf.reason}")
    return clf


def injection_probability(logits: list[dict]) -> float:
    """P(INJECTION) of a two-label model from its raw logits, computed in
    double precision: 1 / (1 + exp(safe - injection))."""
    by = {r["label"].upper(): float(r["score"]) for r in logits}
    margin = by["INJECTION"] - by["SAFE"]
    if margin >= 0:
        return 1.0 / (1.0 + math.exp(-margin))
    e = math.exp(margin)
    return e / (1.0 + e)


def llm_guard_scanner():
    from llm_guard.input_scanners import PromptInjection

    scanner = PromptInjection()  # the documented defaults

    def fn(text: str):
        # The verdict is scan()'s own. The score is the model's INJECTION
        # probability for the same input, from its logits in double
        # precision: scan() rounds it to two decimals, and even unrounded
        # the pipeline's float32 probability ties many rows just under 1,
        # which would make a fixed FPR unreachable.
        # Only scan() is timed: the second call is for the score.
        t0 = time.perf_counter()
        _, valid, _ = scanner.scan(text)
        ms = (time.perf_counter() - t0) * 1000
        if not text.strip():
            return (not valid), 0.0, ms
        res = scanner._pipeline(scanner._match_type.get_inputs(text), top_k=None, function_to_apply="none")
        return (not valid), max(injection_probability(r) for r in res), ms

    return fn, scanner._model


def prompt_guard(repo: str):
    import torch
    from transformers import AutoModelForSequenceClassification, AutoTokenizer

    tok = AutoTokenizer.from_pretrained(repo)
    model = AutoModelForSequenceClassification.from_pretrained(repo).eval()

    @torch.no_grad()
    def fn(text: str):
        enc = tok(text, return_tensors="pt", truncation=True, max_length=512)
        logits = model(**enc).logits[0].tolist()
        p = injection_probability([{"label": "SAFE", "score": logits[0]}, {"label": "INJECTION", "score": logits[1]}])
        return p >= 0.5, p  # the model card's argmax

    return fn


def llama_guard(repo: str):
    import torch
    from transformers import AutoModelForCausalLM, AutoTokenizer

    tok = AutoTokenizer.from_pretrained(repo)
    model = AutoModelForCausalLM.from_pretrained(repo, torch_dtype=torch.bfloat16).eval()

    @torch.no_grad()
    def fn(text: str):
        conv = [{"role": "user", "content": [{"type": "text", "text": text}]}]
        ids = tok.apply_chat_template(conv, return_tensors="pt")
        out = model.generate(ids, max_new_tokens=10, do_sample=False, pad_token_id=tok.eos_token_id)
        answer = tok.decode(out[0, ids.shape[-1]:], skip_special_tokens=True).strip().lower()
        return answer.startswith("unsafe"), None  # a verdict, no score

    return fn


SYSTEMS = ("tamga_rules", "tamga_rules_classifier", "tamga_classifier", "llm_guard",
           "prompt_guard_2_86m", "prompt_guard_2_22m", "llama_guard_3_1b")
META = {
    "prompt_guard_2_86m": "meta-llama/Llama-Prompt-Guard-2-86M",
    "prompt_guard_2_22m": "meta-llama/Llama-Prompt-Guard-2-22M",
    "llama_guard_3_1b": "meta-llama/Llama-Guard-3-1B",
}


# ── Report ──────────────────────────────────────────────────────────────────


def _f(v) -> str:
    return "—" if v is None else f"{v:.3f}"


def markdown(report: dict) -> str:
    systems = [s for s in report["systems"] if report["results"].get(s)]
    lines = [
        f"Fixed false-positive rate: {report['fpr_target']:.0%} of the {report['pooled_benign']} pooled benign rows.",
        "",
        "| Set | Attacks | Benign | System | Recall (default) | FPR (default) | Recall at fixed FPR | FPR at that threshold |",
        "|---|---|---|---|---|---|---|---|",
    ]
    for ds in report["datasets"]:
        for s in systems:
            m = report["results"][s][ds["name"]]
            lines.append(
                f"| `{ds['name']}` | {m['attacks']} | {m['benign']} | {report['labels'][s]} | {_f(m['recall'])} | "
                f"{_f(m['fpr'])} | {_f(m.get('recall_at_fixed_fpr'))} | {_f(m.get('fpr_at_fixed_fpr'))} |"
            )
    lines += ["", "| System | Threshold at fixed FPR | Median ms per text | p95 ms |", "|---|---|---|---|"]
    for s in systems:
        t = report["thresholds"].get(s)
        lat = report["latency"].get(s, {})
        lines.append(f"| {report['labels'][s]} | {'—' if t is None else f'{t:.6f}'} | "
                     f"{_f(lat.get('median_ms'))} | {_f(lat.get('p95_ms'))} |")
    return "\n".join(lines) + "\n"


def _versions() -> dict:
    from importlib.metadata import PackageNotFoundError, version

    out = {}
    for pkg in ("llm-guard", "torch", "transformers", "onnxruntime", "tokenizers"):
        try:
            out[pkg] = version(pkg)
        except PackageNotFoundError:
            pass
    go = subprocess.run(["go", "version"], capture_output=True, text=True)
    out["go"] = go.stdout.strip()
    out["python"] = platform.python_version()
    return out


def _cpu() -> str:
    if os.name == "nt":
        r = subprocess.run(["powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_Processor).Name"],
                           capture_output=True, text=True)
        if r.stdout.strip():
            return r.stdout.strip()
    return platform.processor() or platform.machine()


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--cache", required=True, help="directory for downloads and per-text results")
    ap.add_argument("--model-dir", default=str(REPO / "models" / "prompt-injection-guard-small"),
                    help="Tamga classifier model directory (config.json, tokenizer.json, onnx/)")
    ap.add_argument("--systems", default=",".join(SYSTEMS), help="comma-separated subset of: " + ", ".join(SYSTEMS))
    ap.add_argument("--fpr", type=float, default=DEFAULT_FPR)
    ap.add_argument("--json", help="write the report here")
    ap.add_argument("--md", help="write the Markdown table here")
    args = ap.parse_args(argv)
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")

    cache_dir = Path(args.cache).resolve()
    os.environ.setdefault("HF_HOME", str(cache_dir / "hf"))
    os.environ.setdefault("HF_HUB_DISABLE_SYMLINKS_WARNING", "1")
    wanted = [s.strip() for s in args.systems.split(",") if s.strip()]

    sets = load_public_sets() + load_repo_sets()
    items = [i for s in sets for i in s.items]
    results: dict[str, dict[str, Result]] = {}
    skipped: dict[str, str] = {}
    labels = {
        "tamga_rules": "Tamga rules (default)",
        "tamga_rules_classifier": "Tamga rules + classifier",
        "tamga_classifier": "Tamga classifier alone",
        "llm_guard": "LLM Guard PromptInjection",
        "prompt_guard_2_86m": "Prompt Guard 2 86M",
        "prompt_guard_2_22m": "Prompt Guard 2 22M",
        "llama_guard_3_1b": "Llama Guard 3 1B",
    }
    models: dict[str, str] = {}

    verdicts = None
    if {"tamga_rules", "tamga_rules_classifier", "tamga_classifier"} & set(wanted):
        verdicts = run_benchscore(items, cache_dir / "work")
    if "tamga_rules" in wanted:
        results["tamga_rules"] = {
            i: Result(v["injection_action"] != "PASS", None, v["elapsed_us"] / 1000) for i, v in verdicts.items()
        }
        any_action = {i: v["action"] != "PASS" for i, v in verdicts.items()}
    if {"tamga_rules_classifier", "tamga_classifier"} & set(wanted):
        clf = tamga_classifier(args.model_dir)
        model_file = next(Path(args.model_dir) / f for f in ("onnx/model_quantized.onnx", "model_quantized.onnx",
                                                              "onnx/model.onnx", "model.onnx")
                          if (Path(args.model_dir) / f).is_file())
        digest = hashlib.sha256(model_file.read_bytes()).hexdigest()
        models["tamga_classifier"] = f"{clf.model_name}, {model_file.name} sha256 {digest}"
        prepared = [Item(i.id, verdicts[i.id].get("prepared") or "", i.label) for i in items]

        def classify(text: str):
            if not text:
                return False, 0.0
            s = clf.score([text])[0].score
            return s >= TAMGA_THRESHOLD, float(s)

        # Texts that Prepare empties are not sent (score 0, 0 ms): the cache
        # key is the prepared text, so they share the empty-string entry.
        cls = score_loop("tamga_classifier", prepared, Cache(cache_dir / "scores" / "tamga_classifier.json"), classify)
        if "tamga_classifier" in wanted:
            results["tamga_classifier"] = cls
        if "tamga_rules_classifier" in wanted:
            results["tamga_rules_classifier"] = {
                i: Result(
                    tamga_with_classifier(v, cls[i].score),
                    None,
                    v["elapsed_us"] / 1000 + (cls[i].ms if v.get("ask_classifier") else 0.0),
                )
                for i, v in verdicts.items()
            }
    if "llm_guard" in wanted:
        fn, model = llm_guard_scanner()
        models["llm_guard"] = f"{model.path} @ {model.revision}"
        results["llm_guard"] = score_loop("llm_guard", items, Cache(cache_dir / "scores" / "llm_guard.json"), fn)
    for name in ("prompt_guard_2_86m", "prompt_guard_2_22m", "llama_guard_3_1b"):
        if name not in wanted:
            continue
        try:
            fn = llama_guard(META[name]) if name.startswith("llama_guard") else prompt_guard(META[name])
        except Exception as exc:  # gated models without access
            skipped[name] = f"{type(exc).__name__}: {str(exc).splitlines()[0][:160]}"
            print(f"{name}: skipped ({skipped[name]})", file=sys.stderr)
            continue
        models[name] = META[name]
        results[name] = score_loop(name, items, Cache(cache_dir / "scores" / f"{name}.json"), fn)

    pooled = sum(s.benign for s in sets)
    report = {
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "fpr_target": args.fpr,
        "pooled_benign": pooled,
        "machine": {"cpu": _cpu(), "logical_cpus": os.cpu_count(), "os": platform.platform(), **_versions()},
        "datasets": [
            {"name": s.name, "source": s.source, "revision": s.revision, "split": s.split, "licence": s.licence,
             "language": s.language, "attacks": s.attacks, "benign": s.benign, "duplicates_removed": s.duplicates,
             "conflicting_labels_removed": s.conflicts, "notes": s.notes}
            for s in sets
        ],
        "systems": [s for s in SYSTEMS if s in results],
        "labels": labels,
        "models": models,
        "skipped": skipped,
        "thresholds": {},
        "latency": {},
        "results": {},
    }
    for name, res in results.items():
        t = pooled_threshold(sets, res, args.fpr)
        report["thresholds"][name] = t
        report["latency"][name] = latency(res)
        report["results"][name] = {s.name: set_metrics(s.items, res, t) for s in sets}
    if "tamga_rules" in results:
        report["tamga_rules_any_action_fpr"] = {
            s.name: rate([any_action[i.id] for i in s.items if i.label == 0]) for s in sets
        }
        asked = [v for v in verdicts.values() if v.get("ask_classifier")]
        report["tamga_classifier_asked"] = {
            "asked": len(asked),
            "with_signal": sum(1 for v in asked if v.get("signal")),
            "over_max_chars": sum(1 for v in asked if len((v.get("prepared") or "").encode("utf-8")) > TAMGA_MAX_CHARS),
            "texts": len(verdicts),
        }
    long_texts = {s.name: sum(1 for i in s.items if len(i.text) > 2000) for s in sets}
    report["texts_over_2000_chars"] = long_texts

    md = markdown(report)
    print(md)
    if args.json:
        Path(args.json).write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    if args.md:
        Path(args.md).write_text(md, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
