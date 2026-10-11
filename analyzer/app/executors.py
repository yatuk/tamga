"""Process-pool executor for CPU-bound NLP.

Presidio with spaCy is the one CPU-bound scanner the analyzer runs. It gets a
ProcessPoolExecutor: each worker process loads its own copy of the spaCy model
through an initializer callback, which avoids GIL contention under concurrent
gRPC requests.

Worker count is configurable via TAMGA_ANALYZER_WORKERS. Memory scales
linearly with worker count: each process loads its own spaCy model.
"""

from __future__ import annotations

import os
from concurrent.futures import ProcessPoolExecutor
from typing import Callable

# ── Worker count ──────────────────────────────────────────────────────────────

def _default_workers() -> int:
    """Return the number of worker processes in the pool.

    Reads TAMGA_ANALYZER_WORKERS from the environment.  Defaults to
    min(cpu_count, 4), clamped to [1, 8].
    """
    cpu = os.cpu_count() or 2
    default = min(cpu, 4)
    n = int(os.environ.get("TAMGA_ANALYZER_WORKERS", str(default)))
    return max(1, min(n, 8))


_worker_count = _default_workers()

# ── Pool (created lazily on first use) ───────────────────────────────────────

_presidio_pool: ProcessPoolExecutor | None = None


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


def shutdown_pools() -> None:
    """Shut down the process pool gracefully (called from lifespan)."""
    import structlog
    logger = structlog.get_logger()

    global _presidio_pool

    if _presidio_pool is not None:
        logger.info("shutting down presidio process pool")
        _presidio_pool.shutdown(wait=True, cancel_futures=True)
        logger.info("presidio process pool shut down")

    _presidio_pool = None
