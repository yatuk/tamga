"""Tesseract through pytesseract: the ``tesseract`` binary, one subprocess per image.

Available only when pytesseract is installed, the binary answers (on PATH, or
at TAMGA_OCR_TESSERACT_CMD) and the ``tur`` traineddata is installed; on
Debian that is ``apt-get install tesseract-ocr tesseract-ocr-tur``, which the
analyzer image does with ``--build-arg OCR_TESSERACT=true``. Every language a
call asks for has to be installed.

pytesseract writes each image to a temporary file (in TMPDIR) for the
subprocess and removes it afterwards; the subprocess is killed at the call's
deadline. Its thread count is set with OMP_THREAD_LIMIT, which is put in the
environment of the analyzer process when it is not set already.
"""

from __future__ import annotations

import importlib.util
import logging
import os
import shutil
import threading
from typing import Any

from .base import Fragment, OcrEngine, OcrLimits
from .errors import OcrError, OcrInputError, OcrTimeout

logger = logging.getLogger("tamga.ocr")

REQUIRED_LANGS = ("tur", "eng")


class TesseractEngine(OcrEngine):
    name = "tesseract"

    def __init__(
        self,
        limits: OcrLimits | None = None,
        cmd: str | None = None,
        psm: int = 3,
        oem: int = 1,
    ) -> None:
        super().__init__(limits)
        self.cmd = cmd or os.environ.get("TAMGA_OCR_TESSERACT_CMD", "").strip() or "tesseract"
        # 3: automatic page segmentation; 1: the LSTM recogniser only.
        self.config = f"--oem {int(oem)} --psm {int(psm)}"
        self._status: tuple[bool, str] | None = None
        self._status_lock = threading.Lock()
        self._languages: frozenset[str] = frozenset()
        self.version = ""

    def available(self) -> tuple[bool, str]:
        with self._status_lock:
            if self._status is None:
                self._status = self._check()
                if not self._status[0]:
                    logger.warning("OCR engine tesseract is unavailable: %s", self._status[1])
            return self._status

    def _check(self) -> tuple[bool, str]:
        if importlib.util.find_spec("pytesseract") is None:
            return False, "pytesseract is not installed (pip install -r requirements-ocr.txt)"
        if shutil.which(self.cmd) is None:
            return False, f"the tesseract binary ({self.cmd}) is not installed"
        try:
            import pytesseract

            pytesseract.pytesseract.tesseract_cmd = self.cmd
            self.version = str(pytesseract.get_tesseract_version())
            languages = frozenset(pytesseract.get_languages(config=""))
        except Exception as exc:
            return False, f"tesseract does not answer: {type(exc).__name__}"
        missing = [lang for lang in REQUIRED_LANGS if lang not in languages]
        if missing:
            return False, (
                f"tesseract {self.version} has no traineddata for {'+'.join(missing)} "
                "(apt-get install tesseract-ocr-tur)"
            )
        self._languages = languages
        os.environ.setdefault("OMP_THREAD_LIMIT", str(self.limits.threads))
        return True, f"tesseract {self.version}"

    def _recognize(self, image: Any, lang: str, timeout_s: float) -> list[Fragment]:
        import pytesseract

        missing = [part for part in lang.split("+") if part not in self._languages]
        if missing:
            raise OcrInputError("bad_lang", f"tesseract has no traineddata for {'+'.join(missing)}")
        try:
            data = pytesseract.image_to_data(
                image,
                lang=lang,
                config=self.config,
                timeout=max(1, int(timeout_s + 0.999)),
                output_type=pytesseract.Output.DICT,
            )
        except RuntimeError as exc:
            # pytesseract raises RuntimeError("Tesseract process timeout");
            # TesseractError (a RuntimeError too) carries the subprocess's
            # stderr, which is not repeated here.
            if "timeout" in str(exc).lower() and not isinstance(exc, pytesseract.TesseractError):
                raise OcrTimeout("tesseract did not finish in time") from None
            raise OcrError(f"tesseract failed: {type(exc).__name__}") from None
        return lines_from_tsv(data)


def lines_from_tsv(data: dict[str, list[Any]]) -> list[Fragment]:
    """Group Tesseract's words into lines, one fragment per line."""
    # A TSV written with CRLF line ends leaves "\r" on the last column name.
    data = {str(key).strip(): value for key, value in data.items()}
    lines: dict[tuple[int, int, int, int], list[tuple[str, float, int, int, int, int]]] = {}
    for i, word in enumerate(data.get("text", [])):
        word = str(word).strip()
        try:
            conf = float(data["conf"][i])
        except (KeyError, ValueError, IndexError):
            continue
        if not word or conf < 0:
            continue
        key = (
            int(data["page_num"][i]),
            int(data["block_num"][i]),
            int(data["par_num"][i]),
            int(data["line_num"][i]),
        )
        left, top = int(data["left"][i]), int(data["top"][i])
        width, height = int(data["width"][i]), int(data["height"][i])
        lines.setdefault(key, []).append((word, conf, left, top, left + width, top + height))

    fragments: list[Fragment] = []
    for words in lines.values():
        words.sort(key=lambda w: w[2])
        text = " ".join(w[0] for w in words)
        chars = sum(len(w[0]) for w in words)
        conf = sum(w[1] * len(w[0]) for w in words) / chars / 100.0
        x0 = min(w[2] for w in words)
        y0 = min(w[3] for w in words)
        x1 = max(w[4] for w in words)
        y1 = max(w[5] for w in words)
        fragments.append((text, conf, ((x0, y0), (x1, y0), (x1, y1), (x0, y1))))
    return fragments
