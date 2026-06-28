"""Tests for process-pool executors — Presidio and LLM Guard pool lifecycle.

Module-level globals (_presidio_pool, _llm_guard_pool, _worker_count) are
reset between tests via an autouse fixture to prevent state leakage.
"""

import sys
import types
from concurrent.futures import ProcessPoolExecutor
from unittest.mock import MagicMock, patch

import pytest

# Import the module under test.  app.executors does NOT import scanner modules
# at import time (imports happen lazily inside get_*_pool functions).
from app import executors as ex  # noqa: E402


# ── Fixtures ────────────────────────────────────────────────────────────────────


@pytest.fixture(autouse=True)
def _reset_module_state():
    """Reset the module-level pool globals before and after each test."""
    saved_presidio = ex._presidio_pool
    saved_llm = ex._llm_guard_pool
    saved_worker_count = ex._worker_count

    ex._presidio_pool = None
    ex._llm_guard_pool = None
    # Recompute _worker_count so environment changes are visible.
    ex._worker_count = ex._default_workers()

    yield

    ex._presidio_pool = saved_presidio
    ex._llm_guard_pool = saved_llm
    ex._worker_count = saved_worker_count


# ── Tests: get_presidio_pool ────────────────────────────────────────────────────


def test_get_presidio_pool_returns_singleton():
    """Calling get_presidio_pool() twice returns the same ProcessPoolExecutor."""
    mock_pool = MagicMock(spec=ProcessPoolExecutor)

    with patch.object(ex, "_create_pool", return_value=mock_pool) as mock_create:
        pool1 = ex.get_presidio_pool()
        pool2 = ex.get_presidio_pool()

        # Both calls return the same object.
        assert pool1 is pool2
        assert pool1 is mock_pool

        # _create_pool should only be called once (lazy init).
        mock_create.assert_called_once()


def test_get_presidio_pool_passes_correct_initializer():
    """Verify that _create_pool is called with the presidio worker initializer."""
    mock_pool = MagicMock(spec=ProcessPoolExecutor)

    with patch.object(ex, "_create_pool", return_value=mock_pool) as mock_create:
        ex.get_presidio_pool()

        mock_create.assert_called_once()
        args, kwargs = mock_create.call_args
        # First positional arg should be the initializer from pii_deep module.
        assert callable(args[0])
        assert args[1] == "presidio"


# ── Tests: get_llm_guard_pool ───────────────────────────────────────────────────


def test_get_llm_guard_pool_returns_none_when_not_available(monkeypatch):
    """When _is_llm_guard_available returns False, get_llm_guard_pool returns None."""
    # Mock the entire app.scanners.toxicity module so the lazy import
    # inside get_llm_guard_pool() finds our stubs.
    mock_tox = types.ModuleType("app.scanners.toxicity")
    mock_tox._is_llm_guard_available = lambda: False
    mock_tox._init_llm_guard_worker = lambda: None

    # If real module is loaded, pop it first so our mock is used.
    real_tox = sys.modules.pop("app.scanners.toxicity", None)
    sys.modules["app.scanners.toxicity"] = mock_tox

    try:
        result = ex.get_llm_guard_pool()
        assert result is None
        # The module-level singleton should still be None (not a pool).
        assert ex._llm_guard_pool is None
    finally:
        sys.modules.pop("app.scanners.toxicity", None)
        if real_tox is not None:
            sys.modules["app.scanners.toxicity"] = real_tox


def test_get_llm_guard_pool_returns_pool_when_available(monkeypatch):
    """When LLM Guard is importable, get_llm_guard_pool creates and returns a pool."""
    mock_pool = MagicMock(spec=ProcessPoolExecutor)

    mock_tox = types.ModuleType("app.scanners.toxicity")
    mock_tox._is_llm_guard_available = lambda: True
    mock_tox._init_llm_guard_worker = lambda: None

    real_tox = sys.modules.pop("app.scanners.toxicity", None)
    sys.modules["app.scanners.toxicity"] = mock_tox

    try:
        with patch.object(ex, "_create_pool", return_value=mock_pool) as mock_create:
            pool = ex.get_llm_guard_pool()
            assert pool is mock_pool
            mock_create.assert_called_once()
            # Verify the pool name is "llm_guard".
            args, kwargs = mock_create.call_args
            assert args[1] == "llm_guard"
    finally:
        sys.modules.pop("app.scanners.toxicity", None)
        if real_tox is not None:
            sys.modules["app.scanners.toxicity"] = real_tox


def test_get_llm_guard_pool_cache_when_available(monkeypatch):
    """Once LLM Guard pool is created, subsequent calls return the cached pool."""
    mock_pool = MagicMock(spec=ProcessPoolExecutor)

    mock_tox = types.ModuleType("app.scanners.toxicity")
    mock_tox._is_llm_guard_available = lambda: True
    mock_tox._init_llm_guard_worker = lambda: None

    real_tox = sys.modules.pop("app.scanners.toxicity", None)
    sys.modules["app.scanners.toxicity"] = mock_tox

    try:
        with patch.object(ex, "_create_pool", return_value=mock_pool) as mock_create:
            pool1 = ex.get_llm_guard_pool()
            pool2 = ex.get_llm_guard_pool()
            assert pool1 is pool2
            mock_create.assert_called_once()
    finally:
        sys.modules.pop("app.scanners.toxicity", None)
        if real_tox is not None:
            sys.modules["app.scanners.toxicity"] = real_tox


# ── Tests: shutdown_pools ───────────────────────────────────────────────────────


def test_shutdown_pools_with_initialized_pools():
    """shutdown_pools shuts down both pools and clears the globals."""
    mock_presidio = MagicMock()
    mock_llm = MagicMock()

    ex._presidio_pool = mock_presidio
    ex._llm_guard_pool = mock_llm

    ex.shutdown_pools()

    mock_presidio.shutdown.assert_called_once_with(wait=True, cancel_futures=True)
    mock_llm.shutdown.assert_called_once_with(wait=True, cancel_futures=True)
    assert ex._presidio_pool is None
    assert ex._llm_guard_pool is None


def test_shutdown_pools_when_one_is_none():
    """shutdown_pools handles the case where one pool is None."""
    mock_presidio = MagicMock()

    ex._presidio_pool = mock_presidio
    ex._llm_guard_pool = None

    ex.shutdown_pools()

    mock_presidio.shutdown.assert_called_once_with(wait=True, cancel_futures=True)
    assert ex._presidio_pool is None
    assert ex._llm_guard_pool is None


def test_shutdown_pools_when_both_are_none():
    """shutdown_pools is a no-op when both pools are None."""
    ex._presidio_pool = None
    ex._llm_guard_pool = None

    # Should not raise.
    ex.shutdown_pools()

    assert ex._presidio_pool is None
    assert ex._llm_guard_pool is None


# ── Tests: _default_workers ─────────────────────────────────────────────────────


def test_default_workers_default():
    """_default_workers returns a value between 1 and 8 when no env var is set."""
    w = ex._default_workers()
    assert 1 <= w <= 8


def test_worker_count_from_env(monkeypatch):
    """_default_workers reads TAMGA_ANALYZER_WORKERS from the environment."""
    monkeypatch.setenv("TAMGA_ANALYZER_WORKERS", "2")
    assert ex._default_workers() == 2


def test_worker_count_env_clamped_low(monkeypatch):
    """TAMGA_ANALYZER_WORKERS=0 is clamped to 1."""
    monkeypatch.setenv("TAMGA_ANALYZER_WORKERS", "0")
    assert ex._default_workers() == 1


def test_worker_count_env_clamped_high(monkeypatch):
    """TAMGA_ANALYZER_WORKERS=100 is clamped to 8."""
    monkeypatch.setenv("TAMGA_ANALYZER_WORKERS", "100")
    assert ex._default_workers() == 8


def test_worker_count_affects_pool_creation(monkeypatch):
    """When TAMGA_ANALYZER_WORKERS is set, pools are created with that count."""
    monkeypatch.setenv("TAMGA_ANALYZER_WORKERS", "3")
    # Recompute _worker_count since the fixture already reset it.
    ex._worker_count = ex._default_workers()
    assert ex._worker_count == 3

    mock_pool = MagicMock(spec=ProcessPoolExecutor)
    with patch.object(ex, "_create_pool", return_value=mock_pool) as mock_create:
        ex.get_presidio_pool()
        mock_create.assert_called_once()
        # _create_pool uses ex._worker_count internally.
        # Verify it was called (the count is baked into the logger message).
        args, kwargs = mock_create.call_args
        assert args[1] == "presidio"  # name argument

    # Reset to ensure it doesn't affect later tests.
    ex._worker_count = ex._default_workers()


# ── Tests: _create_pool ─────────────────────────────────────────────────────────


def test_create_pool_returns_process_pool_executor():
    """_create_pool returns a ProcessPoolExecutor with the given initializer."""
    mock_initializer = MagicMock()

    with patch("app.executors.ProcessPoolExecutor", wraps=ProcessPoolExecutor) as mock_pe:
        pool = ex._create_pool(mock_initializer, "test_pool")
        assert isinstance(pool, ProcessPoolExecutor)
        mock_pe.assert_called_once()
        _, kwargs = mock_pe.call_args
        assert kwargs.get("initializer") is mock_initializer
        assert kwargs.get("max_workers") > 0
        pool.shutdown(wait=True, cancel_futures=True)


# ── Tests: Presidio pool disabled check (feature gating) ────────────────────────


def test_presidio_pool_not_gated_by_env_var(monkeypatch):
    """TAMGA_PRESIDIO_ENABLED=false does NOT prevent pool creation (not yet implemented).

    This test documents the current behavior.  If a future release adds
    a TAMGA_PRESIDIO_ENABLED gate, this test should be updated to verify
    that the pool returns None when disabled.
    """
    monkeypatch.setenv("TAMGA_PRESIDIO_ENABLED", "false")
    assert ex._presidio_pool is None  # pool not yet created

    mock_pool = MagicMock(spec=ProcessPoolExecutor)
    with patch.object(ex, "_create_pool", return_value=mock_pool):
        pool = ex.get_presidio_pool()
        # Current behavior: pool IS created regardless of TAMGA_PRESIDIO_ENABLED.
        assert pool is mock_pool
