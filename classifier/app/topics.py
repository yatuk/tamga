"""Banned topics: zero-shot topic matching with a local NLI model.

The operator writes topics down in free text, in Turkish or English ("a
request to change a customer's credit score"). The model is a natural
language inference model: given a premise (the request) and a hypothesis
(the topic put into a sentence by the template), it says how likely the
premise entails the hypothesis. No training on the operator's topics is
needed, and none is done.

The model is optional and separate from the injection model. Its directory
is ``TAMGA_TOPICS_MODEL_DIR``; with none, or one that cannot be loaded, the
topic RPC answers UNAVAILABLE and the injection path is untouched.

A text longer than one window is cut into overlapping windows, as the
injection path does, and each topic takes the score of its best window: a
request buried in a long message must not be averaged away.
"""

from __future__ import annotations

import json
import os
import threading
import time
from dataclasses import dataclass
from pathlib import Path

import structlog

from app.classifier import _MODEL_FILES, _env_int, _windows

logger = structlog.get_logger()

# The hypothesis the topic is put into; "{}" is replaced by the topic. Of
# the sentences measured on the evaluation set in tests/data (the bare
# description, "This text is about {}.", "This text contains {}.", a
# Turkish frame and others; docs/operations.md, "Banned topics"), this one
# found the most requests at a threshold of 0.8 with English descriptions
# (11 of 30). With Turkish descriptions the bare description found more (8
# of 30 at 0.7, against 5); TAMGA_TOPICS_TEMPLATE="{}" chooses it.
DEFAULT_TEMPLATE = "This is {}."

# Rows of one window are run together while the batch holds at most this
# many tokens. A batch is cheaper than one run per row for short rows and
# dearer for long ones: on four cores five rows of 64 tokens took 95 ms
# together and 113 ms one by one, five of 128 took 242 ms and 198 ms.
_BATCH_TOKENS = 384


@dataclass(frozen=True)
class TopicScore:
    """Result for one text: one probability per topic, in topic order."""

    scores: list[float]
    # True when the text had more windows than max_windows and only the
    # first ones were read. A low score is then not a verdict.
    truncated: bool


class TopicsUnavailable(RuntimeError):
    """No topic model is loaded."""


class TopicModel:
    """Scores texts against topics with the NLI model in ``model_dir``."""

    def __init__(
        self,
        model_dir: str | None = None,
        window: int | None = None,
        overlap: int | None = None,
        max_windows: int | None = None,
        threads: int | None = None,
        template: str | None = None,
    ) -> None:
        self.model_dir = model_dir if model_dir is not None else os.environ.get("TAMGA_TOPICS_MODEL_DIR", "")
        # One run of one row on four cores cost about 0.3 ms per token at
        # 64 and 128 tokens and 0.43 ms at 256.
        self.window = window or _env_int("TAMGA_TOPICS_WINDOW", 128)
        self.overlap = overlap if overlap is not None else _env_int("TAMGA_TOPICS_OVERLAP", 16)
        self.max_windows = max_windows or _env_int("TAMGA_TOPICS_MAX_WINDOWS", 8)
        self.threads = threads or _env_int("TAMGA_TOPICS_THREADS", _env_int("TAMGA_CLASSIFIER_THREADS", 0))
        self.template = template or os.environ.get("TAMGA_TOPICS_TEMPLATE", "") or DEFAULT_TEMPLATE
        if "{}" not in self.template:
            self.template = DEFAULT_TEMPLATE
        self.model_name = ""
        self.reason = "TAMGA_TOPICS_MODEL_DIR is not set"
        self._session = None
        self._tokenizer = None
        self._inputs: list[str] = []
        self._entail = 0
        self._contradict = 2
        self._cls = 1
        self._sep = 2
        self._pad_id = 0
        self._lock = threading.Lock()
        if self.model_dir:
            self._load()

    @classmethod
    def from_parts(cls, session, tokenizer, *, name: str = "injected", inputs=("input_ids", "attention_mask"),
                   entail: int = 0, contradict: int = 2, cls_id: int = 1, sep_id: int = 2,
                   window: int = 128, overlap: int = 16, max_windows: int = 8,
                   template: str = DEFAULT_TEMPLATE) -> "TopicModel":
        """Build a topic model around an already loaded session and tokenizer."""
        self = cls(model_dir="", window=window, overlap=overlap, max_windows=max_windows, template=template)
        self._session = session
        self._tokenizer = tokenizer
        self._inputs = list(inputs)
        self._entail, self._contradict = entail, contradict
        self._cls, self._sep = cls_id, sep_id
        self.model_name = name
        self.reason = ""
        return self

    @property
    def available(self) -> bool:
        return self._session is not None

    def _load(self) -> None:
        root = Path(self.model_dir)
        model_file = next((root / f for f in _MODEL_FILES if (root / f).is_file()), None)
        if model_file is None or not (root / "tokenizer.json").is_file():
            self.reason = f"no ONNX model and tokenizer.json found in {self.model_dir}"
            logger.warning("topics_unavailable", reason=self.reason)
            return
        try:
            config = {}
            if (root / "config.json").is_file():
                config = json.loads((root / "config.json").read_text(encoding="utf-8"))
            entail, contradict = _nli_labels(config.get("label2id") or {}, config.get("id2label") or {})

            import onnxruntime as ort
            from tokenizers import Tokenizer

            opts = ort.SessionOptions()
            if self.threads > 0:
                opts.intra_op_num_threads = self.threads
            opts.inter_op_num_threads = 1
            session = ort.InferenceSession(str(model_file), sess_options=opts, providers=["CPUExecutionProvider"])

            tokenizer = Tokenizer.from_file(str(root / "tokenizer.json"))
            tokenizer.no_padding()
            tokenizer.no_truncation()
            # The pair is put together here, window by window, so the
            # special tokens are looked up rather than left to the
            # tokenizer's template.
            cls_id = tokenizer.token_to_id("[CLS]")
            sep_id = tokenizer.token_to_id("[SEP]")
            if cls_id is None or sep_id is None:
                cls_id = tokenizer.token_to_id("<s>")
                sep_id = tokenizer.token_to_id("</s>")
            if cls_id is None or sep_id is None:
                raise ValueError("the tokenizer has neither [CLS]/[SEP] nor <s>/</s>")

            self._entail, self._contradict = entail, contradict
            self._cls, self._sep = cls_id, sep_id
            self._pad_id = int(config.get("pad_token_id") or 0)
            self._inputs = [i.name for i in session.get_inputs()]
            self._tokenizer = tokenizer
            self._session = session
            self.model_name = os.environ.get("TAMGA_TOPICS_MODEL_NAME", "") or root.name
            self.reason = ""
        except Exception as exc:  # a broken model must not take the service down
            self.reason = f"topic model could not be loaded: {exc}"
            logger.warning("topics_unavailable", reason=self.reason)
            return

        started = time.perf_counter()
        for _ in range(2):
            self.score(["warm up", "warm up the full window " * self.window], ["a test", "another test"])
        logger.info(
            "topics_loaded",
            model=self.model_name,
            file=model_file.name,
            window=self.window,
            template=self.template,
            warmup_ms=round((time.perf_counter() - started) * 1000, 1),
        )

    def _ids(self, text: str) -> list[int]:
        return list(self._tokenizer.encode(text, add_special_tokens=False).ids)

    def logits(self, texts: list[str], topics: list[str]) -> tuple[list[list[list[float]] | None], list[bool]]:
        """Raw logits per text, per window, per topic; None for an empty text."""
        if not self.available:
            raise TopicsUnavailable(self.reason)

        hyps = [self._ids(self.template.replace("{}", t.strip())) for t in topics]
        longest = max((len(h) for h in hyps), default=0)
        # [CLS] premise [SEP] hypothesis [SEP]: the premise gets what the
        # longest hypothesis leaves of the window.
        room = max(self.window - longest - 3, 16)

        out: list[list[list[float]] | None] = []
        truncated = [False] * len(texts)
        for i, text in enumerate(texts):
            if not text or not text.strip() or not hyps:
                out.append(None)
                continue
            ids = self._ids(text)
            pieces = _windows(ids, [0] * len(ids), room, self.overlap)
            if len(pieces) > self.max_windows:
                pieces = pieces[: self.max_windows]
                truncated[i] = True
            per_window = []
            for premise in pieces:
                rows = [[self._cls] + premise + [self._sep] + h + [self._sep] for h in hyps]
                # The rows differ only by their hypothesis, so padding them
                # to one width costs little.
                per_batch = max(1, _BATCH_TOKENS // max(len(r) for r in rows))
                logits = []
                for start in range(0, len(rows), per_batch):
                    logits.extend(self._run(rows[start : start + per_batch]))
                per_window.append(logits)
            out.append(per_window)
        return out, truncated

    def _run(self, rows: list[list[int]]) -> list[list[float]]:
        import numpy as np

        width = max(len(r) for r in rows)
        arr = np.full((len(rows), width), self._pad_id, dtype=np.int64)
        mask = np.zeros((len(rows), width), dtype=np.int64)
        for r, row in enumerate(rows):
            arr[r, : len(row)] = row
            mask[r, : len(row)] = 1
        feed = {}
        for name in self._inputs:
            if name == "input_ids":
                feed[name] = arr
            elif name == "attention_mask":
                feed[name] = mask
            else:
                feed[name] = np.zeros_like(arr)
        with self._lock:
            logits = self._session.run(None, feed)[0]
        return np.asarray(logits, dtype=np.float64).reshape(len(rows), -1).tolist()

    def score(self, texts: list[str], topics: list[str]) -> list[TopicScore]:
        """Return, for each text, the probability that it is about each topic."""
        raw, truncated = self.logits(texts, topics)
        results = []
        for per_window, cut in zip(raw, truncated):
            best = [0.0] * len(topics)
            for window in per_window or []:
                for j, row in enumerate(window):
                    best[j] = max(best[j], entailment_probability(row, self._entail, self._contradict))
            results.append(TopicScore(scores=best, truncated=cut))
        return results


def entailment_probability(logits, entail: int, contradict: int) -> float:
    """P(entailment) against P(contradiction), with neutral left out.

    This is how zero-shot classification with an NLI model is usually read
    when topics are judged one by one. With this model a topic hypothesis
    is mostly judged neutral, so the three-way entailment probability of a
    request that is about the topic is still low (0.1 to 0.3); the two-way
    reading spreads scores over the range, and separated the two kinds as
    well as any other reading on the evaluation set (docs/operations.md,
    "Banned topics").
    """
    import numpy as np

    row = np.asarray(logits, dtype=np.float64).reshape(-1)
    if row.size < 2:
        return float(1.0 / (1.0 + np.exp(-row[0])))
    pair = np.array([row[entail], row[contradict]])
    pair = pair - pair.max()
    probs = np.exp(pair) / np.exp(pair).sum()
    return float(probs[0])


def _nli_labels(label2id: dict, id2label: dict) -> tuple[int, int]:
    """Indexes of the entailment and contradiction classes, by name."""
    names = {str(k).lower(): int(v) for k, v in label2id.items()}
    if not names:
        names = {str(v).lower(): int(k) for k, v in id2label.items()}
    entail = next((i for n, i in names.items() if n.startswith("entail")), None)
    contradict = next((i for n, i in names.items() if n.startswith("contradict")), None)
    if entail is None or contradict is None:
        raise ValueError("config.json does not name an entailment and a contradiction label: not an NLI model")
    return entail, contradict
