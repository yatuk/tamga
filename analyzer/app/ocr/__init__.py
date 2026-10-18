"""OCR: reading the text in an image, for the DLP scanners to look at.

Off unless TAMGA_OCR_ENGINE names an engine:

- ``rapidocr``: PP-OCRv6 models run by onnxruntime in the analyzer process
  (``pip install -r requirements-ocr.txt`` and the pinned model files);
- ``tesseract``: the ``tesseract`` binary with the ``tur`` traineddata;
- ``off`` (the default) or empty: no OCR.

Importing this package needs none of the engines' dependencies: each is
imported when its engine is first checked or used. Use::

    from app.ocr import OcrError, get_engine

    engine = get_engine()
    ok, reason = engine.available()
    if ok:
        result = engine.read(image_bytes, lang="tur+eng")

The text read from an image is never logged, and the result types leave it
out of their repr.
"""

from __future__ import annotations

import os
import threading
from collections.abc import Mapping
from typing import Any

from .base import DEFAULT_LANG, Fragment, OcrBatch, OcrBox, OcrEngine, OcrLimits, OcrResult
from .errors import OcrError, OcrInputError, OcrTimeout, OcrUnavailable

__all__ = [
    "DEFAULT_LANG",
    "ENGINES",
    "OcrBatch",
    "OcrBox",
    "OcrEngine",
    "OcrError",
    "OcrInputError",
    "OcrLimits",
    "OcrResult",
    "OcrTimeout",
    "OcrUnavailable",
    "engine_from_env",
    "get_engine",
    "make_engine",
]

ENGINES = ("rapidocr", "tesseract", "off")


class DisabledEngine(OcrEngine):
    """What TAMGA_OCR_ENGINE=off, or a value that names no engine, gives."""

    name = "off"

    def __init__(self, reason: str, limits: OcrLimits | None = None) -> None:
        super().__init__(limits)
        self.reason = reason

    def available(self) -> tuple[bool, str]:
        return False, self.reason

    def _recognize(self, image: Any, lang: str, timeout_s: float) -> list[Fragment]:
        raise OcrUnavailable(self.reason)


def make_engine(name: str, limits: OcrLimits | None = None, **kwargs: Any) -> OcrEngine:
    """Build the named engine; an unknown name gives a disabled engine saying so."""
    name = (name or "off").strip().lower()
    if name == "rapidocr":
        from .rapid import RapidOcrEngine

        return RapidOcrEngine(limits, **kwargs)
    if name == "tesseract":
        from .tess import TesseractEngine

        return TesseractEngine(limits, **kwargs)
    if name in ("off", ""):
        return DisabledEngine("OCR is off (TAMGA_OCR_ENGINE=off)", limits)
    return DisabledEngine(
        f"TAMGA_OCR_ENGINE={name!r} names no engine; use one of {', '.join(ENGINES)}", limits
    )


def engine_from_env(env: Mapping[str, str] | None = None) -> OcrEngine:
    env = os.environ if env is None else env
    return make_engine(env.get("TAMGA_OCR_ENGINE", "off"), OcrLimits.from_env(env))


_engine: OcrEngine | None = None
_engine_lock = threading.Lock()


def get_engine() -> OcrEngine:
    """The process's engine, built from the environment on first use."""
    global _engine
    with _engine_lock:
        if _engine is None:
            _engine = engine_from_env()
        return _engine
