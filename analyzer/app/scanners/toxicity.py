"""Output toxicity scanner — LLM Guard ML models only.

Uses LLM Guard output scanners (Toxicity, NoRefusal, BanCode, BanTopics) for
ML-based classification of LLM responses.  All regex-based heuristic patterns
have been offloaded to the Go proxy's content_moderation scanner which runs
inline at <1ms latency.

LLM Guard inference is CPU-bound ML (ONNX Runtime / transformer models).
To avoid GIL contention under concurrent gRPC requests, inference is
offloaded to a ProcessPoolExecutor.  Each worker process loads its own
copy of the LLM Guard models — true parallelism, no GIL sharing.

When LLM Guard is not installed, this scanner returns empty findings
(fail-open) — no regex fallback.
"""

from __future__ import annotations

import asyncio
import time
from typing import Any, ClassVar

import structlog

from app.scanners.base import BaseScanner, Finding, ScanResult

logger = structlog.get_logger()

# ═══════════════════════════════════════════════════════════════════════════════
# Per-process globals — initialised once per worker by _init_llm_guard_worker().
# ═══════════════════════════════════════════════════════════════════════════════

_toxicity_scanner: Any = None
_no_refusal_scanner: Any = None
_ban_code_scanner: Any = None
_ban_topics_scanner: Any = None


def _is_llm_guard_available() -> bool:
    """Lightweight import check — runs in the main process.

    Returns True if LLM Guard can be imported.  Does NOT create scanner
    instances (those are created per-worker by _init_llm_guard_worker).
    """
    try:
        import llm_guard.output_scanners  # noqa: F401
        return True
    except ImportError:
        return False


def _init_llm_guard_worker() -> None:
    """Initialiser called once per worker process.

    Loads the four LLM Guard output scanner instances.  Each worker
    process gets its own copy so inference runs without GIL contention.
    """
    global _toxicity_scanner, _no_refusal_scanner, _ban_code_scanner, _ban_topics_scanner

    from llm_guard.output_scanners import (  # type: ignore[import-untyped]
        Toxicity,
        NoRefusal,
        BanCode,
        BanTopics,
    )

    _toxicity_scanner = Toxicity()
    _no_refusal_scanner = NoRefusal()
    _ban_code_scanner = BanCode()
    _ban_topics_scanner = BanTopics()

    print("[llm_guard worker] LLM Guard models loaded", flush=True)


def _sync_scan(content: str) -> list[dict[str, Any]]:
    """Run LLM Guard inference in the worker process.

    Returns a list of plain dicts (not Pydantic models) because return
    values must cross process boundaries via pickle.
    """
    global _toxicity_scanner, _no_refusal_scanner, _ban_code_scanner, _ban_topics_scanner

    findings: list[dict[str, Any]] = []

    # Toxicity
    try:
        _sanitized, _is_valid, risk_score = _toxicity_scanner.scan(
            prompt="", output=content,
        )
        if risk_score is not None and risk_score > 0.5:
            findings.append({
                "type": "toxicity",
                "category": "toxicity",
                "severity": "high" if risk_score > 0.8 else "medium",
                "match": content[:200],
                "confidence": round(float(risk_score), 2),
            })
    except Exception:
        pass  # fail-open per scanner

    # NoRefusal
    try:
        _sanitized, is_valid, _risk_score = _no_refusal_scanner.scan(
            prompt="", output=content,
        )
        if not is_valid:
            findings.append({
                "type": "toxicity",
                "category": "refusal",
                "severity": "medium",
                "match": content[:200],
                "confidence": 0.85,
            })
    except Exception:
        pass

    # BanCode
    try:
        _sanitized, is_valid, risk_score = _ban_code_scanner.scan(
            prompt="", output=content,
        )
        if not is_valid:
            findings.append({
                "type": "toxicity",
                "category": "banned_code",
                "severity": "high",
                "match": content[:200],
                "confidence": round(float(risk_score) if risk_score else 0.90, 2),
            })
    except Exception:
        pass

    # BanTopics
    try:
        _sanitized, is_valid, risk_score = _ban_topics_scanner.scan(
            prompt="", output=content,
        )
        if not is_valid:
            findings.append({
                "type": "toxicity",
                "category": "banned_topics",
                "severity": "high",
                "match": content[:200],
                "confidence": round(float(risk_score) if risk_score else 0.90, 2),
            })
    except Exception:
        pass

    return findings


# ═══════════════════════════════════════════════════════════════════════════════
# Scanner
# ═══════════════════════════════════════════════════════════════════════════════


class ToxicityScanner(BaseScanner):
    """Scans LLM output for toxic, harmful, or policy-violating content.

    Uses LLM Guard output scanners (Toxicity, NoRefusal, BanCode, BanTopics)
    which run local ML transformer models for classification.  Inference is
    offloaded to a ProcessPoolExecutor so the asyncio event loop stays
    responsive and concurrent requests don't contend for the GIL.

    When LLM Guard is unavailable, returns empty findings (fail-open).

    All regex-based heuristic patterns (hate speech, profanity, CSAM, refusal,
    banned code, banned topics) are handled by the Go proxy's
    ContentModerationScanner — this module is strictly ML-focused.
    """

    name: str = "toxicity"
    SCANNERS: ClassVar[list[str]] = ["toxicity", "no_refusal", "ban_code", "ban_topics"]

    def __init__(self) -> None:
        self._pool_available: bool | None = None  # lazily determined on first scan

    async def scan(self, content: str, config: dict[str, Any] | None = None) -> ScanResult:
        """Run LLM Guard toxicity scanning on output content.

        When LLM Guard is unavailable, returns empty findings immediately
        (fail-open).  Regex patterns are handled by the Go proxy — no
        heuristic fallback needed here.
        """
        t0 = time.monotonic()

        if not content or not content.strip():
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.0)

        # Lazily check pool availability (first scan).
        if self._pool_available is None:
            from app.executors import get_llm_guard_pool
            self._pool_available = get_llm_guard_pool() is not None
            if not self._pool_available:
                logger.warning(
                    "LLM Guard not available — toxicity scanner disabled "
                    "(fail-open, Go proxy handles regex)"
                )

        if not self._pool_available:
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.0)

        # Offload synchronous LLM Guard ML inference to process pool.
        # Each worker has its own Python interpreter + GIL + model copies.
        from app.executors import get_llm_guard_pool

        pool = get_llm_guard_pool()
        loop = asyncio.get_running_loop()
        raw_findings: list[dict[str, Any]] = await loop.run_in_executor(
            pool, _sync_scan, content,
        )

        findings = [
            Finding(
                type=f["type"],
                category=f["category"],
                severity=f["severity"],
                match=f["match"],
                confidence=f["confidence"],
            )
            for f in raw_findings
        ]

        duration = (time.monotonic() - t0) * 1000
        return ScanResult(
            scanner=self.name,
            findings=findings,
            duration_ms=round(duration, 2),
        )
