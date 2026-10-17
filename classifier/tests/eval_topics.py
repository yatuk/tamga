"""Precision, recall and latency of banned topics on tests/data/topics_eval.json.

Not a test (pytest does not collect it). Run from classifier/ with the
model's directory:

    python -m tests.eval_topics --model-dir /path/to/nli-model --threads 4

Every text is scored against every topic, once with the topics' English
descriptions and once with their Turkish ones. For a topic, its positives
are the positive texts written for it; every other text, hard negatives of
it and texts written for other topics included, is a negative.
"""

from __future__ import annotations

import argparse
import json
import statistics
import time
from pathlib import Path

import numpy as np

from app.topics import TopicModel, entailment_probability

DATA = Path(__file__).parent / "data" / "topics_eval.json"

TEMPLATES = {
    "bare": "{}",
    "this_is": "This is {}.",
    "this_text_is": "This text is {}.",
    "about": "This text is about {}.",
    "contains": "This text contains {}.",
    "example": "This example is {}.",
    "topic_is": "The topic of this text is {}.",
    "tr_hakkinda": "Bu metin {} hakkında.",
}
THRESHOLDS = (0.5, 0.6, 0.7, 0.8, 0.9)


def three_way(row, entail: int) -> float:
    row = np.asarray(row, dtype=np.float64)
    row = row - row.max()
    p = np.exp(row) / np.exp(row).sum()
    return float(p[entail])


def scores_for(model: TopicModel, texts: list[str], topics: list[str], scoring: str) -> np.ndarray:
    raw, _ = model.logits(texts, topics)
    out = np.zeros((len(texts), len(topics)))
    for i, per_window in enumerate(raw):
        for window in per_window or []:
            for j, row in enumerate(window):
                if scoring == "two_way":
                    s = entailment_probability(row, model._entail, model._contradict)
                else:
                    s = three_way(row, model._entail)
                out[i, j] = max(out[i, j], s)
    return out


def pr(scores: np.ndarray, labels: list[str], names: list[str], threshold: float):
    rows = []
    for j, name in enumerate(names):
        pos = np.array([lab == name for lab in labels])
        hit = scores[:, j] >= threshold
        tp = int((hit & pos).sum())
        fp = int((hit & ~pos).sum())
        fn = int((~hit & pos).sum())
        prec = tp / (tp + fp) if tp + fp else 1.0
        rec = tp / (tp + fn) if tp + fn else 1.0
        rows.append((name, prec, rec, tp, fp, fn))
    return rows


def detail(s: np.ndarray, data: dict, names: list[str], labels: list[str], hard: list[str]) -> None:
    """Per topic: precision, recall, recall by the text's language, hard negatives flagged."""
    lang = [t["lang"] for t in data["texts"]]
    print("  | topic | t | precision | recall | recall en | recall tr | hard negatives flagged |")
    for j, name in enumerate(names):
        for th in (0.6, 0.7, 0.8, 0.9):
            hit = s[:, j] >= th
            pos = np.array([lab == name for lab in labels])
            tp, fp = int((hit & pos).sum()), int((hit & ~pos).sum())
            prec = tp / (tp + fp) if tp + fp else float("nan")
            rec = tp / int(pos.sum())
            by = {}
            for lg in ("en", "tr"):
                sel = pos & np.array([x == lg for x in lang])
                by[lg] = f"{int((hit & sel).sum())}/{int(sel.sum())}"
            hn = sum(1 for i, h in enumerate(hard) if h == name and hit[i])
            print(f"  | {name} | {th:.1f} | {prec:.2f} | {rec:.2f} | {by['en']} | {by['tr']} | {hn}/6 |")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model-dir", required=True)
    ap.add_argument("--threads", type=int, default=4)
    ap.add_argument("--templates", default=",".join(TEMPLATES))
    ap.add_argument("--scorings", default="two_way,three_way")
    ap.add_argument("--detail", default="bare", help="template whose per-topic table is printed")
    ap.add_argument("--latency-only", action="store_true")
    ap.add_argument("--json", help="write every score to this file")
    args = ap.parse_args()

    data = json.loads(DATA.read_text(encoding="utf-8"))
    names = [t["name"] for t in data["topics"]]
    texts = [t["text"] for t in data["texts"]]
    labels = [t["topic"] if t["kind"] == "positive" else "" for t in data["texts"]]
    hard = [t["topic"] if t["kind"] == "hard_negative" else "" for t in data["texts"]]
    print(f"{len(texts)} texts, {len(names)} topics, {sum(1 for x in labels if x)} positives, "
          f"{sum(1 for x in hard if x)} hard negatives")

    started = time.perf_counter()
    model = TopicModel(model_dir=args.model_dir, threads=args.threads)
    assert model.available, model.reason
    print(f"load and warm-up: {time.perf_counter() - started:.1f} s")

    # Latency: one call as the proxy makes it, one text against every topic.
    for desc_lang in ("en", "tr"):
        topics = [t[desc_lang] for t in data["topics"]]
        for n_topics in (1, 5):
            took = []
            for text in texts:
                t0 = time.perf_counter()
                model.score([text], topics[:n_topics])
                took.append((time.perf_counter() - t0) * 1000)
            took.sort()
            print(f"latency, {desc_lang} topics, {n_topics} topic(s), one short text: "
                  f"p50 {statistics.median(took):.0f} ms, p95 {took[int(len(took) * 0.95) - 1]:.0f} ms, max {took[-1]:.0f} ms")
    topics = [t["en"] for t in data["topics"]]
    for n_texts in (5, 12, 30):
        long_text = " ".join(t["text"] for t in data["texts"][:n_texts])
        took = []
        for _ in range(3):
            t0 = time.perf_counter()
            res = model.score([long_text], topics)
            took.append((time.perf_counter() - t0) * 1000)
        n_tokens = len(model._ids(long_text))
        print(f"latency, one text of {n_tokens} tokens ({len(long_text)} chars), 5 topics: "
              f"median {statistics.median(took):.0f} ms, truncated={res[0].truncated}")
    if args.latency_only:
        return

    dump = {}
    for tname in args.templates.split(","):
        model.template = TEMPLATES[tname]
        for desc_lang in ("en", "tr"):
            topics = [t[desc_lang] for t in data["topics"]]
            for scoring in args.scorings.split(","):
                s = scores_for(model, texts, topics, scoring)
                dump[f"{tname}/{desc_lang}/{scoring}"] = s.tolist()
                print(f"\n== template {tname!r} ({TEMPLATES[tname]}), {desc_lang} descriptions, {scoring}")
                for th in THRESHOLDS:
                    rows = pr(s, labels, names, th)
                    mp = statistics.mean(r[1] for r in rows)
                    mr = statistics.mean(r[2] for r in rows)
                    hard_fp = sum(1 for i, h in enumerate(hard) if h and s[i, names.index(h)] >= th)
                    other_fp = sum(int(r[4]) for r in rows) - hard_fp
                    tp_all = sum(int(r[3]) for r in rows)
                    fp_all = sum(int(r[4]) for r in rows)
                    micro_p = tp_all / (tp_all + fp_all) if tp_all + fp_all else float("nan")
                    micro_r = tp_all / sum(1 for x in labels if x)
                    cells = "  ".join(f"{r[0][:12]} {r[1]:.2f}/{r[2]:.2f}" for r in rows)
                    print(f"  t={th:.2f}  pooled P {micro_p:.2f} R {micro_r:.2f}  macro P {mp:.2f} R {mr:.2f}  hard-neg FP {hard_fp:2d}/30  other FP {other_fp:2d}  | {cells}")
                if tname == args.detail and scoring == "two_way":
                    detail(s, data, names, labels, hard)
    if args.json:
        Path(args.json).write_text(json.dumps(dump), encoding="utf-8")


if __name__ == "__main__":
    main()
