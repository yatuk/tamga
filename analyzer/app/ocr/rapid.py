"""RapidOCR: PaddleOCR's PP-OCRv6 models run by onnxruntime, in the process.

The recognition model is multilingual (Latin script, Turkish included), so the
``lang`` argument is checked for form and otherwise not used.

The engine loads model files from TAMGA_OCR_MODEL_DIR, or from the rapidocr
package's own ``models`` directory when that is unset, and checks each file
against the SHA-256 pinned in ``model_pins.py`` before loading it. It never
downloads: every model path is passed to RapidOCR explicitly, which turns its
download-on-first-use off, and a missing or altered file makes the engine
unavailable. ``scripts/fetch_ocr_models.py`` puts the files in place; the
analyzer image does so at build time with ``--build-arg OCR_RAPIDOCR=true``.

Images are handed to RapidOCR as decoded PIL images, never as a path or a
string: a string that looks like a URL is fetched by RapidOCR's loader.
"""

from __future__ import annotations

import hashlib
import importlib.metadata
import importlib.util
import logging
import os
import threading
import time
from pathlib import Path
from typing import Any

from .base import Fragment, OcrEngine, OcrLimits
from .errors import OcrUnavailable
from .model_pins import RAPIDOCR_MODELS, RAPIDOCR_VERSION

logger = logging.getLogger("tamga.ocr")


def default_model_dir() -> Path | None:
    """The rapidocr package's models directory, found without importing it."""
    spec = importlib.util.find_spec("rapidocr")
    if spec is None or not spec.submodule_search_locations:
        return None
    return Path(next(iter(spec.submodule_search_locations))) / "models"


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


class RapidOcrEngine(OcrEngine):
    name = "rapidocr"

    def __init__(
        self,
        limits: OcrLimits | None = None,
        model_dir: str | os.PathLike | None = None,
        det_limit_side_len: int | None = None,
        det_limit_type: str | None = None,
    ) -> None:
        super().__init__(limits)
        # RapidOCR's detector resizes the image so that its shorter side is
        # det_limit_side_len (type "min") or its longer side at most that
        # (type "max"). None keeps RapidOCR's defaults.
        self.det_limit_side_len = det_limit_side_len
        self.det_limit_type = det_limit_type
        configured = model_dir or os.environ.get("TAMGA_OCR_MODEL_DIR", "").strip() or None
        self.model_dir = Path(configured) if configured else default_model_dir()
        self._status: tuple[bool, str] | None = None
        self._status_lock = threading.Lock()
        self._ocr: Any = None

    def available(self) -> tuple[bool, str]:
        with self._status_lock:
            if self._status is None:
                self._status = self._check()
                if not self._status[0]:
                    logger.warning("OCR engine rapidocr is unavailable: %s", self._status[1])
            return self._status

    def _check(self) -> tuple[bool, str]:
        for module in ("rapidocr", "onnxruntime"):
            if importlib.util.find_spec(module) is None:
                return False, f"{module} is not installed (pip install -r requirements-ocr.txt)"
        try:
            version = importlib.metadata.version("rapidocr")
            import rapidocr  # noqa: F401
        except Exception as exc:  # a broken install: cv2 without its libraries, for example
            return False, f"rapidocr cannot be imported: {type(exc).__name__}"
        if version != RAPIDOCR_VERSION:
            return False, f"rapidocr {version} is installed; the pinned models are for {RAPIDOCR_VERSION}"
        if self.model_dir is None:
            return False, "no model directory: set TAMGA_OCR_MODEL_DIR"
        for role, (filename, _url, sha256, size) in RAPIDOCR_MODELS.items():
            path = self.model_dir / filename
            if not path.is_file():
                return False, (
                    f"the {role} model {filename} is not in {self.model_dir}; "
                    "run scripts/fetch_ocr_models.py or build the image with OCR_RAPIDOCR=true"
                )
            if path.stat().st_size != size or _sha256(path) != sha256:
                return False, f"the {role} model {path} does not match its pinned SHA-256"
        return True, f"rapidocr {version}, models in {self.model_dir}"

    def _load(self) -> Any:
        # Called on the engine's single worker thread only.
        if self._ocr is None:
            import cv2
            from rapidocr import RapidOCR

            cv2.setNumThreads(self.limits.threads)
            assert self.model_dir is not None
            params: dict[str, Any] = {
                "Global.log_level": "error",
                "Global.model_root_dir": str(self.model_dir),
                "EngineConfig.onnxruntime.intra_op_num_threads": self.limits.threads,
                "EngineConfig.onnxruntime.inter_op_num_threads": 1,
            }
            if self.det_limit_side_len is not None:
                params["Det.limit_side_len"] = int(self.det_limit_side_len)
            if self.det_limit_type is not None:
                params["Det.limit_type"] = str(self.det_limit_type)
            for role, section in (("det", "Det"), ("cls", "Cls"), ("rec", "Rec")):
                params[f"{section}.model_path"] = str(self.model_dir / RAPIDOCR_MODELS[role][0])
            try:
                self._ocr = RapidOCR(params=params)
            except Exception as exc:
                raise OcrUnavailable(f"rapidocr failed to start: {type(exc).__name__}") from None
        return self._ocr

    def warm_up(self) -> None:
        """Load the models now instead of on the first read."""
        self._require_available()
        self._run(self._load, deadline=time.monotonic() + max(self.limits.timeout_s, 300.0))

    def _recognize(self, image: Any, lang: str, timeout_s: float) -> list[Fragment]:
        ocr = self._load()
        output = ocr(image)
        if output is None or not getattr(output, "txts", None):
            return []
        fragments: list[Fragment] = []
        for box, text, score in zip(output.boxes, output.txts, output.scores):
            points = tuple((int(round(float(x))), int(round(float(y)))) for x, y in box)
            fragments.append((str(text), float(score), points))
        return fragments
