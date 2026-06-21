"""Tamga Analyzer — deep semantic NLP analysis for LLM security scanning.

All static regex-based scanning (PII, secrets, injection patterns, content
moderation, jailbreak keywords) is handled by the Go proxy inline at <1ms.
This service is strictly reserved for semantic/deep analysis that requires
ML models or NLP context:

  • Presidio spaCy NER — contextual entity recognition (PERSON, ORG, GPE, LOC)
  • Claude Haiku judge — semantic prompt injection classification
  • LLM Guard ML — toxicity, refusal, banned code/topics classifiers
"""

import asyncio
import json
import os
import time
from contextlib import asynccontextmanager
from typing import Any

import structlog
from fastapi import FastAPI, BackgroundTasks, Query
from fastapi.responses import JSONResponse, Response
from pydantic import BaseModel
from pydantic_settings import BaseSettings, SettingsConfigDict

from app.scanners.base import BaseScanner, Finding, ScanResult
from app.scanners.pii_deep import DeepPIIScanner
from app.scanners.injection_llm import LLMInjectionScanner
from app.scanners.toxicity import ToxicityScanner
from app.compliance import compute_compliance_report
from app.reports import generate_owasp_pdf_report
from tamga_sdk.discovery import discover_scanners, get_custom_scanners
from app.grpc_server import serve_grpc
from app.executors import shutdown_pools

logger = structlog.get_logger()


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8")

    port: int = 8444
    anthropic_api_key: str = ""
    openai_api_key: str = ""
    database_url: str = ""


settings = Settings()

deep_pii = DeepPIIScanner()
injection = LLMInjectionScanner(api_key=settings.anthropic_api_key)
toxicity = ToxicityScanner()

# Custom scanner SDK — auto-discovered from TAMGA_CUSTOM_SCANNERS_DIR.
# Each scanner class is instantiated once at startup. An empty directory
# (or unset env var) is a no-op — zero overhead.
_custom_scanner_instances: list[BaseScanner] = [
    cls() for cls in discover_scanners()
]

# Lazy asyncpg pool — None when DB is not configured
_db_pool: Any = None

# gRPC server (started in lifespan alongside FastAPI).
_grpc_server: Any = None


async def _init_db() -> None:
    """Initialise asyncpg connection pool. Noop when DATABASE_URL is empty."""
    global _db_pool
    if not settings.database_url:
        logger.info("DATABASE_URL not set — analyzer results will not be persisted")
        return

    try:
        import asyncpg
        # Convert sqlalchemy URL to asyncpg DSN if needed
        dsn = settings.database_url
        if dsn.startswith("postgresql://"):
            dsn = dsn.replace("postgresql://", "postgres://", 1)
        elif dsn.startswith("postgresql+asyncpg://"):
            dsn = dsn.replace("postgresql+asyncpg://", "postgres://", 1)

        _db_pool = await asyncpg.create_pool(dsn=dsn, min_size=1, max_size=4)
        logger.info("Analyzer DB pool created")
    except Exception as exc:
        logger.warning("Failed to create analyzer DB pool (findings will not be persisted): %s", exc)
        _db_pool = None


async def _close_db() -> None:
    """Close the asyncpg pool gracefully."""
    global _db_pool
    if _db_pool is not None:
        await _db_pool.close()
        _db_pool = None
        logger.info("Analyzer DB pool closed")


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _grpc_server
    logger.info("analyzer_starting")
    await _init_db()

    # Start gRPC server alongside FastAPI on port 50051.
    # Both share the same asyncio event loop.
    grpc_port = int(os.environ.get("TAMGA_GRPC_PORT", "50051"))
    _grpc_server = await serve_grpc(deep_pii, injection, toxicity, _custom_scanner_instances, grpc_port)

    yield

    if _grpc_server is not None:
        await _grpc_server.stop(5.0)
        logger.info("gRPC server stopped")

    # Shut down ML process pools.  Each worker process is given 5 seconds
    # to finish any in-flight inference before being terminated.
    shutdown_pools()

    await _close_db()
    logger.info("analyzer_shutting_down")


app = FastAPI(
    title="Tamga Analyzer",
    version="0.2.0",
    lifespan=lifespan,
)


class AnalyzeRequest(BaseModel):
    request_id: str
    content: str
    scan_types: list[str] = ["pii", "injection"]
    provider: str | None = None
    model: str | None = None
    org_id: str | None = None
    metadata: dict[str, str] | None = None
    pre_scanned: bool = True  # set by Go proxy; text already passed static regex scan


class AnalyzeResponse(BaseModel):
    request_id: str
    findings: list[Finding]
    duration_ms: float
    scanner_results: list[ScanResult] = []


@app.get("/health")
async def health():
    from app.scanners.toxicity import _is_llm_guard_available
    llm_guard_ok = _is_llm_guard_available()
    return {
        "status": "ok",
        "service": "tamga-analyzer",
        "llm_guard_available": llm_guard_ok,
        "scanners": {
            "pii_deep": True,
            "injection_llm": True,
            "toxicity": llm_guard_ok,
            "custom_count": len(_custom_scanner_instances),
            "custom_names": [s.name for s in _custom_scanner_instances],
        },
    }


@app.post("/api/v1/analyze", response_model=AnalyzeResponse)
async def analyze(req: AnalyzeRequest, background_tasks: BackgroundTasks):
    """Run deep semantic analysis.

    Called by the Go proxy AFTER static regex scanning completes.  The Go
    proxy has already detected and redacted/blocked based on regex patterns
    (email, phone, credit card, SSN, TCKN, IP, toxic keywords, jailbreak
    phrases, etc.).  This endpoint provides the ML/NLP layer:

      • PII deep — spaCy NER for contextual entities (names, orgs, locations)
      • Injection LLM — Claude Haiku judge for semantic injection detection
      • Toxicity — LLM Guard ML classifiers for nuanced toxicity

    PII and injection run concurrently via asyncio.gather.  Toxicity runs
    sequentially after (it operates on output, not input).
    """
    start = time.monotonic()

    scan_types = set(req.scan_types)
    tasks: list[asyncio.Task] = []
    task_labels: list[str] = []

    if "pii" in scan_types:
        tasks.append(asyncio.ensure_future(deep_pii.scan(req.content)))
        task_labels.append("pii")
    if "injection" in scan_types:
        tasks.append(asyncio.ensure_future(injection.scan(req.content)))
        task_labels.append("injection")

    results = await asyncio.gather(*tasks, return_exceptions=True)

    raw_findings: list[Finding] = []
    scanner_results: list[ScanResult] = []

    for label, result in zip(task_labels, results):
        if isinstance(result, Exception):
            logger.warning("scanner %s failed (fail-open): %s", label, result)
            continue
        if isinstance(result, ScanResult):
            scanner_results.append(result)
            for f in result.findings:
                raw_findings.append(Finding(
                    type=f.type, category=f.category,
                    severity=f.severity, match=f.match, confidence=f.confidence,
                ))

    if "toxicity" in scan_types:
        try:
            tox_result = await toxicity.scan(req.content)
            scanner_results.append(tox_result)
            for f in tox_result.findings:
                raw_findings.append(Finding(
                    type=f.type, category=f.category,
                    severity=f.severity, match=f.match, confidence=f.confidence,
                ))
        except Exception as exc:
            logger.warning("toxicity scanner failed (fail-open): %s", exc)

    # Custom scanner SDK — run all discovered custom scanners.
    # Custom scanners accept a "custom" scan_type or their specific name.
    if "custom" in scan_types or any(
        s.name in scan_types for s in _custom_scanner_instances
    ):
        for scanner in _custom_scanner_instances:
            try:
                result = await scanner.scan(req.content)
                scanner_results.append(result)
                for f in result.findings:
                    raw_findings.append(Finding(
                        type=f.type, category=f.category,
                        severity=f.severity, match=f.match, confidence=f.confidence,
                    ))
            except Exception as exc:
                logger.warning(
                    "custom scanner %s failed (fail-open): %s",
                    scanner.name, exc,
                )

    duration = (time.monotonic() - start) * 1000
    background_tasks.add_task(store_results, req.request_id, raw_findings, req.org_id)

    return AnalyzeResponse(
        request_id=req.request_id,
        findings=raw_findings,
        duration_ms=round(duration, 2),
        scanner_results=scanner_results,
    )


async def store_results(request_id: str, findings: list[Finding], org_id: str | None):
    """Persist analyzer findings to PostgreSQL (async background task).

    Attempts to insert into analyzer_findings table. Falls back to
    structured logging when DB is unavailable.
    """
    global _db_pool

    if _db_pool is None:
        logger.debug(
            "store request=%s findings=%d org=%s (no DB — log only)",
            request_id, len(findings), org_id,
        )
        return

    try:
        async with _db_pool.acquire() as conn:
            # Schema is managed by deploy/migrations/007_analyzer_findings.up.sql.
            # No DDL belongs in the application write path — it acquires
            # AccessExclusiveLock on every call and serialises concurrent writes.
            if findings:
                rows = [
                    (request_id, org_id, f.type, f.category, f.severity, f.match[:500] if f.match else None, f.confidence)
                    for f in findings
                ]
                await conn.executemany(
                    """INSERT INTO analyzer_findings
                       (request_id, org_id, finding_type, category, severity, match_text, confidence)
                       VALUES ($1, $2, $3, $4, $5, $6, $7)""",
                    rows,
                )
                logger.debug("persisted %d findings for request=%s", len(findings), request_id)

    except Exception as exc:
        logger.warning("Failed to persist findings for request=%s (fail-open): %s", request_id, exc)


# ---------------------------------------------------------------------------
# Report endpoints
# ---------------------------------------------------------------------------

@app.get("/api/v1/compliance/owasp")
async def compliance_owasp():
    """Return OWASP LLM Top 10 compliance report as JSON."""
    report = compute_compliance_report()
    return report.model_dump()


@app.get("/api/v1/compliance/privacy")
async def compliance_privacy():
    """Return privacy entity coverage report as JSON."""
    from app.reports import generate_privacy_json_report
    return JSONResponse(content=json.loads(generate_privacy_json_report()))


@app.get("/api/v1/reports/owasp/pdf")
async def reports_owasp_pdf(
    range: str = Query("7d"),
    org_id: str = Query(""),
):
    """Download OWASP compliance report as PDF.

    Query params ``range`` and ``org_id`` are forwarded for future filtering
    (reserved for per-tenant compliance scoping).
    """
    try:
        pdf_bytes = generate_owasp_pdf_report()
    except ImportError:
        return JSONResponse(
            status_code=501,
            content={
                "error": "PDF generation unavailable: ReportLab not installed",
                "available": False,
            },
        )
    return Response(
        content=pdf_bytes,
        media_type="application/pdf",
        headers={"Content-Disposition": "attachment; filename=tamga-owasp-report.pdf"},
    )


@app.get("/api/v1/reports/incident/pdf")
async def reports_incident_pdf(
    total_requests: int = Query(0),
    blocked: int = Query(0),
    redacted: int = Query(0),
    warned: int = Query(0),
    period_hours: int = Query(24),
    range: str = Query("24h"),
    org_id: str = Query(""),
):
    """Download incident summary report as PDF.

    Query params ``range`` and ``org_id`` are forwarded from the proxy
    for per-tenant filtering and time-range scoping.
    """
    from app.reports import generate_incident_pdf_report
    stats = {
        "total_requests": total_requests,
        "blocked": blocked,
        "redacted": redacted,
        "warned": warned,
        "period_hours": period_hours,
    }
    try:
        pdf_bytes = generate_incident_pdf_report(stats)
    except ImportError:
        return JSONResponse(
            status_code=501,
            content={
                "error": "PDF generation unavailable: ReportLab not installed",
                "available": False,
            },
        )
    return Response(
        content=pdf_bytes,
        media_type="application/pdf",
        headers={"Content-Disposition": "attachment; filename=tamga-incident-report.pdf"},
    )


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=settings.port)
