"""Running an extraction: dispatch, limits, the worker pool and the deadline."""

from __future__ import annotations

import asyncio
import logging
import os
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Any, Callable

from app.extract import readers, sniff
from app.extract.limits import Limits
from app.extract.result import (
    STATUS_EXTRACT_FAILED,
    STATUS_OK,
    STATUS_TOO_LARGE,
    Result,
)

logger = logging.getLogger("tamga.extract")


def extract_bytes(
    data: bytes,
    limits: Limits | None = None,
    ocr: readers.OcrRead | None = None,
    deadline: float | None = None,
) -> Result:
    """Reads the text of one file, synchronously. ocr reads an image's text,
    or is None when OCR is off or not installed. It does not raise."""
    limits = limits or Limits()
    if deadline is None:
        deadline = time.monotonic() + limits.timeout_s
    if len(data) > limits.max_bytes:
        return Result(status=STATUS_TOO_LARGE)
    found = sniff.sniff(data)
    if found.status != STATUS_OK:
        return Result(status=found.status, detected_type=found.type)
    budget = readers.Budget(limits, deadline)
    try:
        if found.type == sniff.TYPE_PDF:
            return readers.read_pdf(data, budget, ocr)
        if found.type == sniff.TYPE_DOCX:
            return readers.read_docx(data, budget)
        if found.type == sniff.TYPE_XLSX:
            return readers.read_xlsx(data, budget)
        if found.type == sniff.TYPE_PPTX:
            return readers.read_pptx(data, budget)
        if found.type == sniff.TYPE_IMAGE:
            return readers.read_image(data, budget, ocr)
    except readers.DeadlineExceeded:
        logger.warning("extraction ran past its deadline: type=%s", found.type)
        return Result(status=STATUS_EXTRACT_FAILED, detected_type=found.type)
    except MemoryError:
        logger.warning("extraction ran out of memory: type=%s", found.type)
        return Result(status=STATUS_TOO_LARGE, detected_type=found.type)
    except Exception as exc:  # noqa: BLE001 - a damaged file is a status, not a crash
        # By type only: a library's message can quote what it was reading.
        logger.warning("extraction failed: type=%s error=%s", found.type, type(exc).__name__)
        return Result(status=STATUS_EXTRACT_FAILED, detected_type=found.type)
    return Result(status=STATUS_EXTRACT_FAILED, detected_type=found.type)  # pragma: no cover


# ── OCR engine ────────────────────────────────────────────────────────────


def load_ocr_engine() -> Any | None:
    """The OCR engine of app.ocr, or None when that module is not part of
    this install or its engine is not available (no model, no binary)."""
    try:
        from app.ocr import OcrEngine  # type: ignore[import-not-found]
    except Exception:  # noqa: BLE001 - ImportError, or a module that fails to load
        return None
    try:
        engine = OcrEngine()
        if not engine.available():
            return None
    except Exception as exc:  # noqa: BLE001
        logger.warning("OCR engine unavailable: %s", type(exc).__name__)
        return None
    return engine


def _default_workers() -> int:
    try:
        n = int(os.environ.get("TAMGA_EXTRACT_WORKERS", "2"))
    except ValueError:
        n = 2
    return max(1, min(n, 8))


class Extractor:
    """Runs extractions on a bounded pool of threads, each with a deadline.

    A thread cannot be stopped from outside. One that runs past its
    deadline is answered for (extract_failed) and goes on until its reader
    returns, still holding its worker; pypdf's own bounds on decompressed
    streams, the page and character limits and the checks between parts
    keep that short. While every worker is busy a new request is answered
    extract_failed at once rather than queued behind them.
    """

    def __init__(
        self,
        workers: int | None = None,
        ocr_factory: Callable[[], Any | None] = load_ocr_engine,
    ) -> None:
        self._workers = workers or _default_workers()
        self._pool = ThreadPoolExecutor(max_workers=self._workers, thread_name_prefix="extract")
        self._busy = 0
        self._lock = threading.Lock()
        self._ocr_factory = ocr_factory
        self._ocr: Any | None = None
        self._ocr_loaded = False

    def ocr_engine(self) -> Any | None:
        if not self._ocr_loaded:
            with self._lock:
                if not self._ocr_loaded:
                    self._ocr = self._ocr_factory()
                    self._ocr_loaded = True
        return self._ocr

    def _release(self, _future: Any) -> None:
        with self._lock:
            self._busy -= 1

    async def extract(self, data: bytes, limits: Limits, ocr: bool = False, ocr_lang: str = "") -> Result:
        if len(data) > limits.max_bytes:
            return Result(status=STATUS_TOO_LARGE)
        read_image: readers.OcrRead | None = None
        if ocr:
            engine = self.ocr_engine()
            if engine is not None:

                def read_image(image: bytes, _engine: Any = engine) -> str:
                    return readers.ocr_text(_engine.read(image, ocr_lang))

        with self._lock:
            if self._busy >= self._workers:
                logger.warning("extraction refused: all %d workers are busy", self._workers)
                return Result(status=STATUS_EXTRACT_FAILED)
            self._busy += 1
        deadline = time.monotonic() + limits.timeout_s
        try:
            future = self._pool.submit(extract_bytes, data, limits, read_image, deadline)
        except RuntimeError:  # the pool is shut down
            with self._lock:
                self._busy -= 1
            return Result(status=STATUS_EXTRACT_FAILED)
        future.add_done_callback(self._release)
        try:
            # A little past the reader's own deadline, which it checks
            # between parts; this one is for a reader stuck inside one.
            return await asyncio.wait_for(asyncio.wrap_future(future), timeout=limits.timeout_s + 0.5)
        except asyncio.TimeoutError:
            logger.warning("extraction timed out after %.1fs", limits.timeout_s)
            return Result(status=STATUS_EXTRACT_FAILED)
        except Exception as exc:  # noqa: BLE001
            logger.warning("extraction failed: %s", type(exc).__name__)
            return Result(status=STATUS_EXTRACT_FAILED)

    def shutdown(self) -> None:
        self._pool.shutdown(wait=False, cancel_futures=True)
