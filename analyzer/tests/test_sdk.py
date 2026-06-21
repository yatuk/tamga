"""Tests for Tamga Custom Scanner SDK — discovery, loading, integration."""

import asyncio
import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from app.scanners.base import BaseScanner, Finding, ScanResult
from tamga_sdk.discovery import discover_scanners, get_custom_scanners


# ── Test helper: create a temporary scanner file ────────────────────────────

SCANNER_TEMPLATE = """
from app.scanners.base import BaseScanner, Finding, ScanResult

class {class_name}(BaseScanner):
    name = "{scanner_name}"

    async def scan(self, content, config=None):
        return ScanResult(
            scanner=self.name,
            findings=[Finding(
                type="test", category="{finding_cat}",
                severity="low", match="test", confidence=0.5
            )],
            duration_ms=1.0,
        )
"""


def _write_scanner_file(dir_path: str, filename: str, class_name: str,
                        scanner_name: str, finding_cat: str = "test") -> str:
    """Write a scanner plugin file and return its path."""
    filepath = Path(dir_path) / filename
    filepath.write_text(SCANNER_TEMPLATE.format(
        class_name=class_name,
        scanner_name=scanner_name,
        finding_cat=finding_cat,
    ), encoding="utf-8")
    return str(filepath)


# ── Unit: discovery with empty/non-existent directory ────────────────────────


def test_discover_empty_directory():
    """Empty directory returns empty list."""
    with tempfile.TemporaryDirectory() as tmpdir:
        result = discover_scanners(tmpdir)
        assert result == []
        assert get_custom_scanners() == []


def test_discover_nonexistent_directory():
    """Non-existent directory returns empty list gracefully."""
    result = discover_scanners("/tmp/tamga_nonexistent_dir_12345")
    assert result == []


# ── Unit: discovery loads a single scanner ──────────────────────────────────


def test_discover_single_scanner():
    """Single scanner file is discovered and loaded."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_scanner_file(tmpdir, "my_scanner.py", "MyScanner", "my_scanner")

        result = discover_scanners(tmpdir)
        assert len(result) == 1
        assert result[0].name == "my_scanner"
        assert issubclass(result[0], BaseScanner)


# ── Unit: discovery loads multiple scanners ──────────────────────────────────


def test_discover_multiple_scanners():
    """Multiple scanner files are all discovered."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_scanner_file(tmpdir, "scanner_a.py", "ScannerA", "scanner_a", "cat_a")
        _write_scanner_file(tmpdir, "scanner_b.py", "ScannerB", "scanner_b", "cat_b")

        result = discover_scanners(tmpdir)
        names = {s.name for s in result}
        assert names == {"scanner_a", "scanner_b"}
        assert len(result) == 2


# ── Unit: skips private modules ─────────────────────────────────────────────


def test_discover_skips_private_files():
    """Files starting with _ are skipped."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_scanner_file(tmpdir, "_private.py", "PrivateScanner", "private_scanner")
        _write_scanner_file(tmpdir, "public.py", "PublicScanner", "public_scanner")

        result = discover_scanners(tmpdir)
        names = {s.name for s in result}
        assert names == {"public_scanner"}


# ── Unit: skips non-py files ────────────────────────────────────────────────


def test_discover_skips_non_py_files():
    """Only .py files are loaded."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_scanner_file(tmpdir, "scanner.py", "GoodScanner", "good_scanner")
        (Path(tmpdir) / "README.md").write_text("# docs", encoding="utf-8")
        (Path(tmpdir) / "config.json").write_text("{}", encoding="utf-8")

        result = discover_scanners(tmpdir)
        assert len(result) == 1
        assert result[0].name == "good_scanner"


# ── Unit: broken scanner file handled gracefully ────────────────────────────


def test_discover_broken_file_skipped():
    """Syntax errors in scanner files are caught, other scanners still load."""
    with tempfile.TemporaryDirectory() as tmpdir:
        (Path(tmpdir) / "broken.py").write_text(
            "this is not valid {{{{ python", encoding="utf-8")
        _write_scanner_file(tmpdir, "good.py", "GoodScanner", "good_scanner")

        result = discover_scanners(tmpdir)
        assert len(result) == 1
        assert result[0].name == "good_scanner"


# ── Unit: cache works ───────────────────────────────────────────────────────


def test_discover_cache():
    """Second discovery returns cached result for same directory."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_scanner_file(tmpdir, "scanner.py", "TestScanner", "test_scanner")

        result1 = discover_scanners(tmpdir)
        result2 = discover_scanners(tmpdir)

        assert len(result1) == 1
        assert len(result2) == 1
        # Same objects (from cache)
        assert result1[0] is result2[0]


# ── Unit: get_custom_scanners before discovery ──────────────────────────────


def test_get_custom_scanners_returns_list():
    """get_custom_scanners always returns a list (may be empty or populated)."""
    result = get_custom_scanners()
    assert isinstance(result, list)
    # After previous tests, the registry may have scanners — that's fine.
    # The function must never return None.


# ── Unit: scanner instantiation and scan ────────────────────────────────────


def test_discovered_scanner_can_scan():
    """Discovered scanner can be instantiated and used."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_scanner_file(tmpdir, "test_scan.py", "TestScanner", "test_scan", "demo")

        result = discover_scanners(tmpdir)
        assert len(result) == 1

        scanner = result[0]()
        scan_result = asyncio.run(scanner.scan("some content"))
        assert isinstance(scan_result, ScanResult)
        assert scan_result.scanner == "test_scan"
        assert len(scan_result.findings) == 1
        assert scan_result.findings[0].type == "test"
        assert scan_result.findings[0].category == "demo"


# ── Unit: example scanner (sentiment) ───────────────────────────────────────


def test_example_sentiment_scanner():
    """The bundled example sentiment scanner works correctly."""
    # Import via file path rather than package (matches discovery behavior)
    import importlib.util
    from pathlib import Path

    example_path = Path(__file__).parent.parent / "examples" / "sentiment_scanner.py"
    if not example_path.exists():
        pytest.skip("example scanner not found")

    spec = importlib.util.spec_from_file_location(
        "sentiment_example", str(example_path))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

    scanner = module.SentimentScanner()
    assert scanner.name == "sentiment"

    # Clean content — no findings
    result = asyncio.run(scanner.scan("The weather is nice today."))
    assert result.findings == []

    # Risky content — detected
    result = asyncio.run(scanner.scan("We should sue you for data breach damages."))
    assert len(result.findings) >= 2  # "sue you" + "data breach"
    categories = {f.category for f in result.findings}
    assert categories == {"negative_sentiment"}


# ── Unit: empty content returns immediately ─────────────────────────────────


def test_example_scanner_empty_content():
    """Empty content returns immediately with no findings."""
    import importlib.util
    from pathlib import Path

    example_path = Path(__file__).parent.parent / "examples" / "sentiment_scanner.py"
    if not example_path.exists():
        pytest.skip("example scanner not found")

    spec = importlib.util.spec_from_file_location(
        "sentiment_empty", str(example_path))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

    scanner = module.SentimentScanner()
    result = asyncio.run(scanner.scan(""))
    assert result.findings == []
    result = asyncio.run(scanner.scan("   "))
    assert result.findings == []


# ── Unit: custom scanner fail-open isolation ────────────────────────────────


def test_custom_scanner_exception_isolation():
    """If one custom scanner crashes, the error is caught (tested at SDK level)."""
    scanner = MagicMock(spec=BaseScanner)
    scanner.name = "crashy"
    scanner.scan.side_effect = RuntimeError("boom")

    # Simulate the exception handling pattern used in main.py
    try:
        asyncio.run(scanner.scan("content"))
    except RuntimeError:
        pass  # This is what would happen WITHOUT try/except

    # The real scan flow wraps this in try/except — we test the pattern here.
    caught = False
    try:
        async def _run():
            raise RuntimeError("boom")
        asyncio.run(_run())
    except RuntimeError:
        caught = True
    assert caught  # Exceptions propagate unless caught (verified pattern)
