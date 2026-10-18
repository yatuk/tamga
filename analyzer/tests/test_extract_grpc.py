"""The Extract RPC: limits in, statuses and parts out, nothing raised."""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

pytest.importorskip("grpc")

# test_grpc_server stands in for the scanners' heavy dependencies before
# app.grpc_server is imported.
from tests.test_grpc_server import _mock_context  # noqa: E402

from analyzer.v1.tamga_pb2 import ExtractLimits, ExtractRequest  # noqa: E402
from app.extract import STATUS_OK, Extractor, Limits, Part, Result  # noqa: E402
from app.grpc_server import _MAX_RECEIVE_BYTES, AnalyzerServicer  # noqa: E402

TCKN = "10000000146"


def _servicer(extractor) -> AnalyzerServicer:
    return AnalyzerServicer(MagicMock(), MagicMock(), extractor=extractor)


def test_extract_maps_the_result_and_the_limits():
    fake = MagicMock()
    fake.extract = AsyncMock(
        return_value=Result(
            status=STATUS_OK,
            parts=[Part(kind="sheet", index=2, name="Customers", text=TCKN)],
            truncated=True,
            detected_type="xlsx",
        )
    )
    req = ExtractRequest(
        request_id="r-1",
        data=b"PK\x03\x04",
        mime_type="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        filename="x.xlsx",
        limits=ExtractLimits(max_bytes=1000, max_pages=3, max_chars=50, timeout_ms=2500),
        ocr=True,
        ocr_lang="tur",
    )
    resp = asyncio.run(_servicer(fake).Extract(req, _mock_context()))
    assert resp.request_id == "r-1" and resp.status == "ok" and resp.truncated
    assert resp.detected_type == "xlsx"
    assert [(p.kind, p.index, p.name, p.text) for p in resp.parts] == [("sheet", 2, "Customers", TCKN)]
    args, kwargs = fake.extract.call_args
    assert args[0] == b"PK\x03\x04"
    assert args[1] == Limits(max_bytes=1000, max_pages=3, max_chars=50, timeout_s=2.5)
    assert kwargs == {"ocr": True, "ocr_lang": "tur"}


def test_extract_never_raises():
    fake = MagicMock()
    fake.extract = AsyncMock(side_effect=RuntimeError("boom with file text " + TCKN))
    resp = asyncio.run(_servicer(fake).Extract(ExtractRequest(data=b"x"), _mock_context()))
    assert resp.status == "extract_failed" and len(resp.parts) == 0


def test_extract_with_the_real_extractor():
    pytest.importorskip("pypdf")
    from tests.test_extract import text_pdf

    ex = Extractor(workers=1, ocr_factory=lambda: None)
    try:
        req = ExtractRequest(data=text_pdf([f"page {TCKN}"]), limits=ExtractLimits())
        resp = asyncio.run(_servicer(ex).Extract(req, _mock_context()))
        assert resp.status == "ok" and TCKN in resp.parts[0].text and resp.parts[0].kind == "page"
        # Unknown bytes: a status, not an error.
        resp = asyncio.run(_servicer(ex).Extract(ExtractRequest(data=b"\x00\x01"), _mock_context()))
        assert resp.status == "unsupported_type"
    finally:
        ex.shutdown()


def test_server_takes_a_file_of_the_largest_size():
    assert _MAX_RECEIVE_BYTES > Limits.of(max_bytes=1 << 40).max_bytes
