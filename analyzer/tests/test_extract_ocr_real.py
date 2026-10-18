"""Extraction through the real OCR module (app.ocr), not a fake engine.

Runs only where RapidOCR and its models are installed (requirements-ocr.txt);
it is skipped elsewhere. It checks the seam between app.extract and app.ocr:
the engine is found with get_engine(), its available() answer is read as
(ok, reason), and an image and a PDF without a text layer come back as text.
"""

import asyncio
import io
from pathlib import Path

import pytest

SAMPLES = Path(__file__).parent / "ocr_samples"


def _engine_or_skip(monkeypatch):
    monkeypatch.setenv("TAMGA_OCR_ENGINE", "rapidocr")
    pytest.importorskip("rapidocr")
    pytest.importorskip("PIL")
    from app.extract import service

    engine = service.load_ocr_engine()
    if engine is None:
        pytest.skip("RapidOCR is not available here")
    return service


def _run(service, data: bytes):
    from app.extract.limits import Limits

    extractor = service.Extractor(ocr_factory=service.load_ocr_engine)
    return asyncio.run(extractor.extract(data, Limits(), ocr=True))


def test_an_image_is_read_through_the_real_engine(monkeypatch):
    service = _engine_or_skip(monkeypatch)
    result = _run(service, (SAMPLES / "english_form.png").read_bytes())
    text = " ".join(part.text for part in result.parts)
    assert "National ID" in text and "Customer onboarding form" in text


def test_a_pdf_without_text_is_read_through_the_real_engine(monkeypatch):
    service = _engine_or_skip(monkeypatch)
    from PIL import Image

    buf = io.BytesIO()
    Image.open(SAMPLES / "english_form.png").convert("RGB").save(buf, "PDF", resolution=150)
    result = _run(service, buf.getvalue())
    text = " ".join(part.text for part in result.parts)
    assert "National ID" in text and "Customer onboarding form" in text
