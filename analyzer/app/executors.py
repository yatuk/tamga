"""Process-pool executors for CPU-bound ML inference.

Each ML scanner (Presidio/spaCy, LLM Guard) gets its own ProcessPoolExecutor.
Worker processes load their own copy of the ML models via an initializer
callback — this avoids GIL contention under concurrent gRPC requests.

Worker count is configurable via TAMGA_ANALYZER_WORKERS (default: 2 per pool).
Memory scales linearly with worker count: each process loads spaCy (~15 MB)
or LLM Guard models (~200 MB).  Keep the default low unless you have the RAM.
"""

from __future__ import annotations

import os
from concurrent.futures import ProcessPoolExecutor
from typing import Callable

# ── Worker count ──────────────────────────────────────────────────────────────

def _default_workers() -> int:
    """Return the number of worker processes per executor pool.

    Reads TAMGA_ANALYZER_WORKERS from the environment.  Defaults to
    min(cpu_count, 4), clamped to [1, 8].
    """
    cpu = os.cpu_count() or 2
    default = min(cpu, 4)
    n = int(os.environ.get("TAMGA_ANALYZER_WORKERS", str(default)))
    return max(1, min(n, 8))


_worker_count = _default_workers()

# ── Pools (created lazily on first use) ──────────────────────────────────────

_presidio_pool: ProcessPoolExecutor | None = None
_llm_guard_pool: ProcessPoolExecutor | None = None


def _create_pool(
    initializer: Callable[[], None],
    name: str,
) -> ProcessPoolExecutor:
    """Create a ProcessPoolExecutor with per-worker model initializer."""
    import structlog
    logger = structlog.get_logger()
    logger.info(
        "creating %s process pool: workers=%d", name, _worker_count,
    )
    return ProcessPoolExecutor(
        max_workers=_worker_count,
        initializer=initializer,
    )


def get_presidio_pool() -> ProcessPoolExecutor:
    """Return the Presidio/spaCy process pool (lazy init)."""
    global _presidio_pool
    if _presidio_pool is None:
        from app.scanners.pii_deep import _init_presidio_worker
        _presidio_pool = _create_pool(_init_presidio_worker, "presidio")
    return _presidio_pool


def get_llm_guard_pool() -> ProcessPoolExecutor | None:
    """Return the LLM Guard process pool, or None if LLM Guard is unavailable."""
    global _llm_guard_pool
    if _llm_guard_pool is None:
        from app.scanners.toxicity import _init_llm_guard_worker, _is_llm_guard_available
        # Only create the pool if LLM Guard is actually importable.
        # The check runs in the main process to avoid forking useless workers.
        if not _is_llm_guard_available():
            return None
        _llm_guard_pool = _create_pool(_init_llm_guard_worker, "llm_guard")
    return _llm_guard_pool


def shutdown_pools() -> None:
    """Shut down both process pools gracefully (called from lifespan)."""
    import structlog
    logger = structlog.get_logger()

    global _presidio_pool, _llm_guard_pool

    for name, pool in [("presidio", _presidio_pool), ("llm_guard", _llm_guard_pool)]:
        if pool is not None:
            logger.info("shutting down %s process pool", name)
            pool.shutdown(wait=True, cancel_futures=True)
            logger.info("%s process pool shut down", name)

    _presidio_pool = None
    _llm_guard_pool = None
