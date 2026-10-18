"""The RapidOCR model files the analyzer loads, pinned by URL and SHA-256.

This module is plain data with no imports from the rest of the analyzer, so
that ``scripts/fetch_ocr_models.py`` can load it in the Docker build stage
before the application is copied in.

The three files are RapidOCR 3.10.0's defaults for the onnxruntime engine:
PP-OCRv6 small detection and recognition (one multilingual recognition model
whose language list includes Turkish and English) and the PP-OCRv4 mobile
direction classifier. They are PaddleOCR models converted to ONNX by the
RapidOCR project, Apache-2.0. The URLs and hashes are copied from
``rapidocr/default_models.yaml`` in the rapidocr 3.10.0 wheel; the hashes were
checked against the files that wheel downloaded.
"""

from __future__ import annotations

_BASE = "https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/v3.10.0/onnx"

# role -> (file name, URL, SHA-256, size in bytes)
RAPIDOCR_MODELS: dict[str, tuple[str, str, str, int]] = {
    "det": (
        "PP-OCRv6_det_small.onnx",
        f"{_BASE}/PP-OCRv6/det/PP-OCRv6_det_small.onnx",
        "090f04abcd9d9a7498bc4ebf677e4cb9bdce1fe4197ddb7e529f1ef44e1ff94f",
        9_929_594,
    ),
    "rec": (
        "PP-OCRv6_rec_small.onnx",
        f"{_BASE}/PP-OCRv6/rec/PP-OCRv6_rec_small.onnx",
        "6f327246b50388f3c176ae304bd95767ea6dc0c9ae92153ef8cbe210b3c14884",
        21_234_383,
    ),
    "cls": (
        "ch_ppocr_mobile_v2.0_cls_mobile.onnx",
        f"{_BASE}/PP-OCRv4/cls/ch_ppocr_mobile_v2.0_cls_mobile.onnx",
        "e47acedf663230f8863ff1ab0e64dd2d82b838fceb5957146dab185a89d6215c",
        585_532,
    ),
}

# The rapidocr release these files belong to. A different rapidocr version may
# expect other files; requirements-ocr.txt pins the same version.
RAPIDOCR_VERSION = "3.10.0"
