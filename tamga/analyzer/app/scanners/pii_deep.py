"""Semantic NER scanner using Microsoft Presidio spaCy engine only.

All static regex-based PII detection (email, phone, credit card, SSN, TCKN,
IBAN, IP, passport, DOB, medical, NPI, DEA) is handled by the Go proxy inline
at <1ms latency.  This module is strictly reserved for semantic entity
recognition that regex cannot accomplish — person names, organization names,
geopolitical entities, and locations identified through NLP context.

Presidio's NLP pipeline is CPU-bound (spaCy NER).  To avoid blocking the
asyncio event loop AND to avoid GIL contention, all Presidio calls are
offloaded to a ProcessPoolExecutor.  Each worker process loads its own
copy of the spaCy model so inference runs in true parallelism.
"""

from __future__ import annotations

import asyncio
import time
from typing import Any

import structlog

from app.scanners.base import BaseScanner, Finding, ScanResult

logger = structlog.get_logger()

# ═══════════════════════════════════════════════════════════════════════════════
# Per-process globals — initialised once per worker by _init_presidio_worker().
# These are NOT shared across processes; each worker has its own copy.
# ═══════════════════════════════════════════════════════════════════════════════

_analyzer_engine: Any = None  # Presidio AnalyzerEngine (per-process singleton)


def _init_presidio_worker() -> None:
    """Initialiser called once per worker process.

    Loads the spaCy model and creates a Presidio AnalyzerEngine configured
    with:
      • SpacyRecognizer — semantic NLP NER (PERSON, ORG, GPE, LOC, DATE)
      • GDPR/HIPAA PatternRecognizers — regex-based PII for passport, DOB,
        national ID, medical record, health plan, NPI.

    The Go proxy handles these regex patterns at <1ms inline; the Presidio
    recognizers here provide defense-in-depth for standalone deployments.
    """
    global _analyzer_engine

    from presidio_analyzer import AnalyzerEngine, RecognizerRegistry
    from presidio_analyzer.predefined_recognizers import SpacyRecognizer  # type: ignore[import-untyped]

    registry = RecognizerRegistry()
    # SEMANTIC: SpacyRecognizer for contextual NLP NER.
    # Detects PERSON (names), ORG (companies), GPE (countries/cities),
    # LOC (locations), DATE (contextual dates), and other spaCy entities
    # that require linguistic context — not pattern matching.
    registry.add_recognizer(SpacyRecognizer())

    # GDPR/HIPAA regex recognizers — aligned with Go proxy scanner patterns.
    from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

    for recognizer in get_gdpr_hipaa_recognizers():
        registry.add_recognizer(recognizer)

    _analyzer_engine = AnalyzerEngine(registry=registry)

    # Logging from worker processes is tricky (structlog may not be
    # configured).  We use print as a fallback — it goes to stderr and
    # shows up in container logs.
    print("[presidio worker] spaCy model loaded + GDPR/HIPAA recognizers registered", flush=True)


def _sync_scan(content: str) -> list[dict[str, Any]]:
    """Run Presidio analysis in the worker process.

    Returns a list of plain dicts (not Pydantic models) because return
    values must cross process boundaries via pickle.
    """
    global _analyzer_engine

    results = _analyzer_engine.analyze(text=content, language="en")

    return [
        {
            "entity_type": r.entity_type,
            "start": r.start,
            "end": r.end,
            "score": r.score,
        }
        for r in results
    ]


# ═══════════════════════════════════════════════════════════════════════════════
# Scanner
# ═══════════════════════════════════════════════════════════════════════════════


def _severity(score: float) -> str:
    if score >= 0.85:
        return "high"
    if score >= 0.6:
        return "medium"
    return "low"


class DeepPIIScanner(BaseScanner):
    """Semantic PII scanner using Presidio spaCy NER exclusively.

    Detects entity types that require linguistic context: person names,
    organization names, locations, dates — NOT static patterns like email
    or credit card numbers (those are handled by the Go proxy).

    The CPU-bound Presidio analyze() call is offloaded to a process pool
    via loop.run_in_executor() — each worker runs in its own Python
    interpreter, eliminating GIL contention.
    """

    name: str = "pii_deep"

    async def scan(self, content: str, config: dict[str, Any] | None = None) -> ScanResult:
        if not content.strip():
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.0)

        start = time.monotonic()

        # Offload CPU-bound Presidio NLP to a PROCESS pool.
        # Each worker has its own Python interpreter + GIL + spaCy model.
        from app.executors import get_presidio_pool

        pool = get_presidio_pool()
        loop = asyncio.get_running_loop()
        raw_results: list[dict[str, Any]] = await loop.run_in_executor(
            pool, _sync_scan, content,
        )

        duration_ms = (time.monotonic() - start) * 1000

        findings: list[Finding] = []
        for r in raw_results:
            matched = content[r["start"] : r["end"]]
            findings.append(
                Finding(
                    type="pii",
                    category=r["entity_type"],
                    severity=_severity(r["score"]),
                    match=matched[:4] + "…" if len(matched) > 4 else matched,
                    confidence=round(r["score"], 3),
                )
            )

        logger.info(
            "pii_deep semantic scan: %d findings in %.1fms",
            len(findings), duration_ms,
        )
        return ScanResult(
            scanner=self.name,
            findings=findings,
            duration_ms=round(duration_ms, 2),
        )
