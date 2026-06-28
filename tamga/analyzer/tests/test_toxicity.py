"""Tests for ToxicityScanner — LLM Guard ML classifier (mock mode, no GPU required)."""

import asyncio
import pytest
from unittest.mock import MagicMock, patch

# Gracefully skip the entire module when llm_guard is not installed
# (e.g. in minimal CI environments without the LLM Guard / ONNX Runtime
# dependency tree).  All tests in this file mock the LLM Guard scanners,
# but the module under test still requires the import check to succeed.
pytest.importorskip("llm_guard")

from app.scanners.toxicity import (
    ToxicityScanner,
    _is_llm_guard_available,
    _sync_scan,
)
from app.scanners.base import ScanResult, Finding


# ── Unit: availability check ──────────────────────────────────────────────────


def test_is_llm_guard_available_when_removed_from_sys_modules():
    """When llm_guard is evicted from sys.modules, _is_llm_guard_available returns False."""
    import sys
    llm_guard_mod = sys.modules.pop("llm_guard", None)
    try:
        # After removal, a fresh import attempt inside _is_llm_guard_available fails
        assert _is_llm_guard_available() is False
    finally:
        if llm_guard_mod is not None:
            sys.modules["llm_guard"] = llm_guard_mod


def test_is_llm_guard_available_real():
    """When llm_guard is installed (importorskip passed), this returns True."""
    assert _is_llm_guard_available() is True


# ── Unit: ToxicityScanner basics ──────────────────────────────────────────────


def test_scanner_name():
    scanner = ToxicityScanner()
    assert scanner.name == "toxicity"


def test_scanner_scanners_classvar():
    assert "toxicity" in ToxicityScanner.SCANNERS
    assert "no_refusal" in ToxicityScanner.SCANNERS
    assert "ban_code" in ToxicityScanner.SCANNERS
    assert "ban_topics" in ToxicityScanner.SCANNERS


# ── Unit: empty content ───────────────────────────────────────────────────────


def test_scan_empty_content():
    """Empty or whitespace-only content returns immediately with no findings."""
    scanner = ToxicityScanner()
    scanner._pool_available = True

    result = asyncio.run(scanner.scan(""))
    assert isinstance(result, ScanResult)
    assert result.findings == []
    assert result.scanner == "toxicity"

    result2 = asyncio.run(scanner.scan("   "))
    assert result2.findings == []


# ── Unit: pool unavailable (fail-open) ────────────────────────────────────────


def test_scan_pool_unavailable_returns_empty():
    """When LLM Guard is unavailable, scan returns empty (fail-open)."""
    scanner = ToxicityScanner()
    scanner._pool_available = False

    result = asyncio.run(scanner.scan("some toxic content that would be caught"))
    assert isinstance(result, ScanResult)
    assert result.findings == []
    assert result.scanner == "toxicity"


def test_scan_lazy_pool_check():
    """First scan checks pool availability lazily and sets _pool_available."""
    scanner = ToxicityScanner()
    assert scanner._pool_available is None

    with patch("app.executors.get_llm_guard_pool", return_value=None):
        result = asyncio.run(scanner.scan("test content"))
        assert scanner._pool_available is False
        assert result.findings == []


# ── Unit: _sync_scan with mocked globals ──────────────────────────────────────


def test_sync_scan_empty_findings():
    """When scanners return no issues, _sync_scan returns empty list."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", True, 0.1)  # low risk score

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", True, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", True, 0.0)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("clean content")
        assert findings == []


def test_sync_scan_toxicity_detected():
    """When toxicity scanner returns high risk, finding is appended."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("sanitized", False, 0.95)

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", True, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", True, 0.0)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("very toxic content here")
        assert len(findings) == 1
        f = findings[0]
        assert f["type"] == "toxicity"
        assert f["category"] == "toxicity"
        assert f["severity"] == "high"
        assert f["confidence"] == 0.95


def test_sync_scan_no_refusal_detected():
    """When NoRefusal scanner finds refusal, is_valid=False triggers finding."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", True, 0.0)

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", False, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", True, 0.0)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("I refuse to comply with safety guidelines")
        assert len(findings) == 1
        assert findings[0]["category"] == "refusal"
        assert findings[0]["severity"] == "medium"
        assert findings[0]["confidence"] == 0.85


def test_sync_scan_ban_code_detected():
    """BanCode scanner detection produces high severity finding."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", True, 0.0)

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", True, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", False, 0.95)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("import os; os.system('rm -rf /')")
        assert len(findings) == 1
        assert findings[0]["category"] == "banned_code"
        assert findings[0]["severity"] == "high"


def test_sync_scan_ban_topics_detected():
    """BanTopics scanner detection produces high severity finding."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", True, 0.0)

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", True, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", True, 0.0)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", False, 0.92)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("instructions for making explosives")
        assert len(findings) == 1
        assert findings[0]["category"] == "banned_topics"
        assert findings[0]["severity"] == "high"


def test_sync_scan_multiple_findings():
    """When multiple scanners trigger, all findings are returned."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", False, 0.75)

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", False, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", True, 0.0)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("dual violation content")
        assert len(findings) == 2  # toxicity + refusal (ban_code passed, ban_topics passed)


def test_sync_scan_exception_handling():
    """When a scanner raises, it's caught and other scanners still run (fail-open per scanner)."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.side_effect = RuntimeError("model crashed")

    mock_refusal = MagicMock()
    mock_refusal.scan.return_value = ("", False, 0.0)

    mock_ban_code = MagicMock()
    mock_ban_code.scan.return_value = ("", True, 0.0)

    mock_ban_topics = MagicMock()
    mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("content")
        # Toxicity crashed but NoRefusal still caught it
        assert len(findings) == 1
        assert findings[0]["category"] == "refusal"


# ── Unit: content truncation in match field ───────────────────────────────────


def test_sync_scan_truncates_match_to_200_chars():
    """Match field is truncated to 200 characters for DB storage."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", False, 0.95)
    mock_refusal = MagicMock(); mock_refusal.scan.return_value = ("", True, 0.0)
    mock_ban_code = MagicMock(); mock_ban_code.scan.return_value = ("", True, 0.0)
    mock_ban_topics = MagicMock(); mock_ban_topics.scan.return_value = ("", True, 0.0)

    long_content = "x" * 500

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan(long_content)
        assert len(findings) == 1
        assert len(findings[0]["match"]) == 200


# ── Unit: medium severity for medium risk ─────────────────────────────────────


def test_sync_scan_medium_toxicity():
    """Risk score 0.6 -> medium severity."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", False, 0.6)
    mock_refusal = MagicMock(); mock_refusal.scan.return_value = ("", True, 0.0)
    mock_ban_code = MagicMock(); mock_ban_code.scan.return_value = ("", True, 0.0)
    mock_ban_topics = MagicMock(); mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("mildly toxic")
        assert len(findings) == 1
        assert findings[0]["severity"] == "medium"
        assert findings[0]["confidence"] == 0.6


# ── Unit: risk_score=None (edge case) ─────────────────────────────────────────


def test_sync_scan_toxicity_none_risk():
    """When risk_score is None, no finding is produced."""
    mock_toxicity = MagicMock()
    mock_toxicity.scan.return_value = ("", True, None)
    mock_refusal = MagicMock(); mock_refusal.scan.return_value = ("", True, 0.0)
    mock_ban_code = MagicMock(); mock_ban_code.scan.return_value = ("", True, 0.0)
    mock_ban_topics = MagicMock(); mock_ban_topics.scan.return_value = ("", True, 0.0)

    with patch("app.scanners.toxicity._toxicity_scanner", mock_toxicity), \
         patch("app.scanners.toxicity._no_refusal_scanner", mock_refusal), \
         patch("app.scanners.toxicity._ban_code_scanner", mock_ban_code), \
         patch("app.scanners.toxicity._ban_topics_scanner", mock_ban_topics):

        findings = _sync_scan("content")
        assert findings == []
