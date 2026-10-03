"""Prompt-injection classifier served from a local ONNX model directory.

The model is not part of the image and not named in the code. The directory
in ``TAMGA_CLASSIFIER_MODEL_DIR`` holds a sequence-classification model
exported to ONNX together with its ``tokenizer.json`` and ``config.json``;
any model of that shape can be dropped in. With no directory, or one that
cannot be loaded, the classifier reports itself unavailable and the rest of
the analyzer runs as before.

A text longer than one window is cut into overlapping windows and scored by
its worst one: an injection hidden in the middle of a long document must not
be averaged away.
"""

from __future__ import annotations

import json
import os
import threading
import time
from dataclasses import dataclass
from pathlib import Path

import structlog

logger = structlog.get_logger()

# Candidate model files, in order of preference. The quantized export is
# about half the size and the one meant for CPU.
_MODEL_FILES = (
    "onnx/model_quantized.onnx",
    "model_quantized.onnx",
    "onnx/model.onnx",
    "model.onnx",
)

# Labels that mean "nothing wrong". Whatever is left is the attack class.
_BENIGN_LABELS = {"safe", "benign", "legit", "legitimate", "label_0", "negative", "no_injection"}


@dataclass(frozen=True)
class Score:
    """Result for one text."""

    score: float
    # True when the text had more windows than max_windows and only the
    # first ones were scored. The caller must not read a low score on a
    # truncated text as "clean".
    truncated: bool


class ClassifierUnavailable(RuntimeError):
    """No model is loaded."""


def _env_int(name: str, default: int) -> int:
    try:
        return int(os.environ.get(name, "") or default)
    except ValueError:
        return default


class Classifier:
    """Scores texts with the model in ``model_dir``."""

    def __init__(
        self,
        model_dir: str | None = None,
        window: int | None = None,
        overlap: int | None = None,
        max_windows: int | None = None,
        threads: int | None = None,
    ) -> None:
        self.model_dir = model_dir if model_dir is not None else os.environ.get("TAMGA_CLASSIFIER_MODEL_DIR", "")
        self.window = window or _env_int("TAMGA_CLASSIFIER_WINDOW", 512)
        self.overlap = overlap if overlap is not None else _env_int("TAMGA_CLASSIFIER_OVERLAP", 64)
        self.max_windows = max_windows or _env_int("TAMGA_CLASSIFIER_MAX_WINDOWS", 16)
        self.threads = threads or _env_int("TAMGA_CLASSIFIER_THREADS", 0)
        self.model_name = ""
        self.reason = "TAMGA_CLASSIFIER_MODEL_DIR is not set"
        self._session = None
        self._tokenizer = None
        self._inputs: list[str] = []
        self._attack_index = 1
        self._pad_id = 0
        self._lock = threading.Lock()
        if self.model_dir:
            self._load()

    @classmethod
    def from_parts(cls, session, tokenizer, *, name: str = "injected", inputs=("input_ids", "attention_mask"),
                   attack_index: int = 1, max_windows: int = 16, window: int = 512, overlap: int = 64) -> "Classifier":
        """Build a classifier around an already loaded session and tokenizer."""
        self = cls(model_dir="", max_windows=max_windows, window=window, overlap=overlap)
        self._session = session
        self._tokenizer = tokenizer
        self._inputs = list(inputs)
        self._attack_index = attack_index
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
            logger.warning("classifier_unavailable", reason=self.reason)
            return
        try:
            import onnxruntime as ort
            from tokenizers import Tokenizer

            opts = ort.SessionOptions()
            if self.threads > 0:
                opts.intra_op_num_threads = self.threads
            opts.inter_op_num_threads = 1
            session = ort.InferenceSession(str(model_file), sess_options=opts, providers=["CPUExecutionProvider"])

            tokenizer = Tokenizer.from_file(str(root / "tokenizer.json"))
            # The text is cut into windows here, not by the tokenizer: its
            # own overflow handling returns one short remainder, not the
            # whole text, and a tokenizer.json may carry a truncation
            # setting of its own.
            tokenizer.no_padding()
            tokenizer.no_truncation()

            config = {}
            if (root / "config.json").is_file():
                config = json.loads((root / "config.json").read_text(encoding="utf-8"))
            self._attack_index = _attack_index(config.get("id2label") or {})
            self._pad_id = int(config.get("pad_token_id") or 0)
            self._inputs = [i.name for i in session.get_inputs()]
            self._tokenizer = tokenizer
            self._session = session
            self.model_name = os.environ.get("TAMGA_CLASSIFIER_MODEL_NAME", "") or root.name
            self.reason = ""
        except Exception as exc:  # a broken model must not take the analyzer down
            self.reason = f"model could not be loaded: {exc}"
            logger.warning("classifier_unavailable", reason=self.reason)
            return

        # The first runs of a session are several times slower than the
        # rest. Pay for them here, at a short and at a full window, not on
        # the first request.
        started = time.perf_counter()
        for _ in range(3):
            self.score(["warm up", "warm up the full window " * self.window])
        logger.info(
            "classifier_loaded",
            model=self.model_name,
            file=model_file.name,
            window=self.window,
            warmup_ms=round((time.perf_counter() - started) * 1000, 1),
        )

    def score(self, texts: list[str]) -> list[Score]:
        """Return the attack probability of each text, in order."""
        if not self.available:
            raise ClassifierUnavailable(self.reason)
        import numpy as np

        windows: list[list[int]] = []
        owner: list[int] = []
        truncated = [False] * len(texts)
        for i, text in enumerate(texts):
            if not text or not text.strip():
                continue
            enc = self._tokenizer.encode(text)
            pieces = _windows(list(enc.ids), list(enc.special_tokens_mask), self.window, self.overlap)
            if len(pieces) > self.max_windows:
                pieces = pieces[: self.max_windows]
                truncated[i] = True
            for ids in pieces:
                windows.append(ids)
                owner.append(i)

        best = [0.0] * len(texts)
        if windows:
            # One window per run keeps latency flat: a batch pads every
            # window to the longest, and the short ones pay for it.
            for ids, i in zip(windows, owner):
                arr = np.asarray([ids], dtype=np.int64)
                feed = {}
                for name in self._inputs:
                    if name == "input_ids":
                        feed[name] = arr
                    elif name == "attention_mask":
                        feed[name] = np.ones_like(arr)
                    else:
                        feed[name] = np.zeros_like(arr)
                with self._lock:
                    logits = self._session.run(None, feed)[0][0]
                best[i] = max(best[i], _attack_probability(logits, self._attack_index))
        return [Score(score=float(s), truncated=t) for s, t in zip(best, truncated)]


def _windows(ids: list[int], special: list[int], window: int, overlap: int) -> list[list[int]]:
    """Cut a token sequence into overlapping windows of at most `window`.

    The special tokens the tokenizer put at the start and end are repeated
    on every window, so each one looks to the model like a whole input.
    """
    if len(ids) <= window:
        return [ids]
    lead = 0
    while lead < len(ids) and special[lead]:
        lead += 1
    trail = 0
    while trail < len(ids) - lead and special[len(ids) - 1 - trail]:
        trail += 1
    head, body, tail = ids[:lead], ids[lead : len(ids) - trail], ids[len(ids) - trail :]
    size = max(window - lead - trail, 1)
    step = max(size - overlap, 1)
    out = []
    for start in range(0, len(body), step):
        out.append(head + body[start : start + size] + tail)
        if start + size >= len(body):
            break
    return out


def _attack_index(id2label: dict) -> int:
    """Index of the attack class. Two-class models name the benign one."""
    labels = {int(k): str(v).lower() for k, v in id2label.items()}
    attack = [i for i, name in labels.items() if name not in _BENIGN_LABELS]
    if len(attack) == 1:
        return attack[0]
    return 1


def _attack_probability(logits, index: int) -> float:
    import numpy as np

    logits = np.asarray(logits, dtype=np.float64).reshape(-1)
    if logits.size == 1:
        return float(1.0 / (1.0 + np.exp(-logits[0])))
    shifted = logits - logits.max()
    probs = np.exp(shifted) / np.exp(shifted).sum()
    return float(probs[min(index, probs.size - 1)])
