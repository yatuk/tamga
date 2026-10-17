"""Shadow mode: a candidate injection model scored next to the one in service.

The shadow model is optional and off unless ``TAMGA_CLASSIFIER_SHADOW_MODEL_DIR``
names a directory; it is a model of the same shape as the primary one,
loaded by the same code. When it is loaded, every text Classify scores is
also scored by it, and its scores go back in the same response for the
proxy to record. They are never a verdict: the proxy decides on the primary
scores alone.

The shadow must not slow the primary down by more than a known bound:

- it runs on a thread of its own, started before the primary, so the two
  run at the same time;
- once the primary has its scores, the call waits at most
  ``TAMGA_CLASSIFIER_SHADOW_WAIT_MS`` (default 20) for the shadow's; a
  shadow that is later is reported as ``late`` and its scores are dropped;
- one shadow run at a time: a call that finds the shadow still busy with
  an earlier one skips it (``busy``) rather than queue behind it.

What it can still cost is CPU: both models run on the same cores.
``TAMGA_CLASSIFIER_SHADOW_THREADS`` bounds the shadow's onnxruntime threads
(docs/operations.md gives measured numbers).
"""

from __future__ import annotations

import asyncio
import os
import threading
import time
from concurrent.futures import Future, ThreadPoolExecutor
from dataclasses import dataclass, field

import structlog

from app.classifier import Classifier, Score, _env_int

logger = structlog.get_logger()

DEFAULT_WAIT_MS = 20

SKIP_LATE = "late"
SKIP_BUSY = "busy"
SKIP_FAILED = "failed"


@dataclass
class ShadowResult:
    """What the shadow gave for one call. scores is empty when skipped says
    why, and both are empty when no shadow model is loaded."""

    scores: list[Score] = field(default_factory=list)
    skipped: str = ""
    duration_ms: float = 0.0


class Shadow:
    """Runs a second Classifier beside the primary, within a bounded wait."""

    def __init__(self, classifier: Classifier | None = None, wait_ms: float | None = None) -> None:
        if classifier is None:
            classifier = Classifier(
                dir_env="TAMGA_CLASSIFIER_SHADOW_MODEL_DIR",
                name_env="TAMGA_CLASSIFIER_SHADOW_MODEL_NAME",
                label="shadow",
                threads=_env_int("TAMGA_CLASSIFIER_SHADOW_THREADS", 0) or None,
            )
        self.classifier = classifier
        if wait_ms is None:
            wait_ms = _env_int("TAMGA_CLASSIFIER_SHADOW_WAIT_MS", DEFAULT_WAIT_MS)
        self.wait_s = max(0.0, float(wait_ms)) / 1000.0
        self._busy = threading.Lock()
        self._executor: ThreadPoolExecutor | None = None
        if self.classifier.available:
            self._executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="shadow")
            logger.info("shadow_ready", model=self.classifier.model_name, wait_ms=wait_ms)

    @property
    def available(self) -> bool:
        return self.classifier.available

    @property
    def model_name(self) -> str:
        return self.classifier.model_name

    @property
    def reason(self) -> str:
        return self.classifier.reason

    def start(self, texts: list[str]) -> Future | str | None:
        """Start scoring texts. Returns the job, a skip reason, or None when
        no shadow model is loaded."""
        if not self.available or self._executor is None:
            return None
        if not self._busy.acquire(blocking=False):
            return SKIP_BUSY
        try:
            return self._executor.submit(self._run, texts)
        except Exception:
            self._busy.release()
            return SKIP_FAILED

    def _run(self, texts: list[str]) -> tuple[list[Score], float]:
        started = time.perf_counter()
        try:
            return self.classifier.score(texts), (time.perf_counter() - started) * 1000
        finally:
            self._busy.release()

    async def collect(self, job: Future | str | None) -> ShadowResult:
        """The shadow's answer for a job start returned, waiting at most the
        shadow wait for it. Called once the primary has its scores."""
        if job is None:
            return ShadowResult()
        if isinstance(job, str):
            return ShadowResult(skipped=job)
        try:
            if job.done():
                scores, ms = job.result()
            else:
                # shield: a shadow that is late keeps running to its end (a
                # model run cannot be stopped) and releases the busy lock
                # itself. Its outcome is then read by no one, on purpose.
                inner = asyncio.wrap_future(job)
                inner.add_done_callback(lambda f: f.cancelled() or f.exception())
                scores, ms = await asyncio.wait_for(asyncio.shield(inner), timeout=self.wait_s)
        except asyncio.TimeoutError:
            return ShadowResult(skipped=SKIP_LATE)
        except Exception as exc:
            logger.warning("shadow_failed", error_type=type(exc).__name__)
            return ShadowResult(skipped=SKIP_FAILED)
        return ShadowResult(scores=scores, duration_ms=round(ms, 2))
