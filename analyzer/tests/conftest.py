"""Shared fixtures and optional-dependency detection for analyzer tests."""

import asyncio
import sys
import types
from unittest.mock import MagicMock

import pytest


# ── Optional dependency detection helpers ──────────────────────────────────────

def is_module_available(module_name: str) -> bool:
    """Check whether an optional Python package can be imported."""
    try:
        __import__(module_name)
        return True
    except ImportError:
        return False


_HAS_ANTHROPIC = is_module_available("anthropic")
_HAS_PRESIDIO = is_module_available("presidio_analyzer")
_HAS_LLM_GUARD = is_module_available("llm_guard")


def pytest_configure(config: pytest.Config) -> None:
    """Register custom markers and log optional-dependency availability."""
    config.addinivalue_line(
        "markers",
        "requires_anthropic: tests that need the Anthropic SDK installed",
    )
    config.addinivalue_line(
        "markers",
        "requires_presidio: tests that need Presidio Analyzer installed",
    )
    config.addinivalue_line(
        "markers",
        "requires_llm_guard: tests that need LLM Guard installed",
    )


def pytest_collection_modifyitems(config: pytest.Config, items: list[pytest.Item]) -> None:
    """Auto-skip tests that require missing optional dependencies."""
    for item in items:
        if item.get_closest_marker("requires_anthropic") and not _HAS_ANTHROPIC:
            item.add_marker(pytest.mark.skip(reason="anthropic not installed"))
        if item.get_closest_marker("requires_presidio") and not _HAS_PRESIDIO:
            item.add_marker(pytest.mark.skip(reason="presidio_analyzer not installed"))
        if item.get_closest_marker("requires_llm_guard") and not _HAS_LLM_GUARD:
            item.add_marker(pytest.mark.skip(reason="llm_guard not installed"))


# ── Fixtures ──────────────────────────────────────────────────────────────────


@pytest.fixture(scope="session")
def event_loop():
    """Create a session-scoped event loop for async tests.

    Uses a fresh event loop per session to avoid interference between tests
    that call asyncio.run() and fixtures that yield async generators.
    """
    loop = asyncio.new_event_loop()
    yield loop
    loop.close()


@pytest.fixture
def mock_anthropic_client():
    """Return a MagicMock configured like anthropic.AsyncAnthropic.

    Tests that need a realistic Anthropic client mock (with .messages.create
    returning a properly shaped response) can build on this fixture.
    """
    mock_content = MagicMock()
    mock_content.text = ""

    mock_response = MagicMock()
    mock_response.content = [mock_content]

    mock_messages = MagicMock()
    mock_messages.create = MagicMock(return_value=mock_response)

    client = MagicMock()
    client.messages = mock_messages
    return client


@pytest.fixture
def mock_presidio_module():
    """Register a lightweight mock for presidio_analyzer in sys.modules.

    Returns the mock module so tests can customize it per-test.
    Cleans up after the test finishes.
    """
    pres_mod = types.ModuleType("presidio_analyzer")
    pres_mod.AnalyzerEngine = MagicMock
    pres_mod.RecognizerRegistry = MagicMock
    sys.modules["presidio_analyzer"] = pres_mod

    predef = types.ModuleType("presidio_analyzer.predefined_recognizers")
    predef.SpacyRecognizer = MagicMock
    sys.modules["presidio_analyzer.predefined_recognizers"] = predef

    yield pres_mod

    sys.modules.pop("presidio_analyzer", None)
    sys.modules.pop("presidio_analyzer.predefined_recognizers", None)
