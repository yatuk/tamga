"""gRPC server implementing the AnalyzerService defined in tamga.proto.

Runs alongside FastAPI in the same process.  The gRPC server handles deep
semantic analysis (the former /api/v1/analyze REST endpoint) over a
persistent, multiplexed connection on port 50051.

FastAPI continues to serve HTTP endpoints (health, compliance, reports)
on port 8444.
"""

from __future__ import annotations

import asyncio
import time

import grpc
import structlog
from grpc.aio import ServicerContext

from analyzer.v1.tamga_pb2 import (  # type: ignore[import-untyped]
    AnalyzeRequest,
    AnalyzeResponse,
    Finding,
    HealthCheckRequest,
    HealthCheckResponse,
    ScannerResult,
)
from analyzer.v1.tamga_pb2_grpc import (  # type: ignore[import-untyped]
    AnalyzerServiceServicer,
    add_AnalyzerServiceServicer_to_server,
)

from app.scanners.base import BaseScanner, Finding as AppFinding
from app.scanners.base import ScanResult
from app.scanners.pii_deep import DeepPIIScanner
from app.scanners.injection_llm import LLMInjectionScanner
from app.scanners.toxicity import ToxicityScanner

logger = structlog.get_logger()


def _app_finding_to_pb(f: AppFinding) -> Finding:
    """Convert internal Finding model to protobuf Finding."""
    return Finding(
        type=f.type,
        category=f.category,
        severity=f.severity,
        match=f.match,
        confidence=f.confidence,
    )


def _scan_result_to_pb(sr: ScanResult) -> ScannerResult:
    """Convert internal ScanResult to protobuf ScannerResult."""
    return ScannerResult(
        scanner=sr.scanner,
        findings=[_app_finding_to_pb(f) for f in sr.findings],
        duration_ms=sr.duration_ms,
    )


class AnalyzerServicer(AnalyzerServiceServicer):
    """gRPC servicer that runs deep semantic NLP analysis.

    This replaces the HTTP POST /api/v1/analyze endpoint with a protobuf
    wire-format RPC.  The Go proxy calls this over a persistent gRPC
    connection — no per-request TCP handshake or JSON serialization overhead.
    """

    def __init__(
        self,
        pii_scanner: DeepPIIScanner,
        injection_scanner: LLMInjectionScanner,
        toxicity_scanner: ToxicityScanner,
        custom_scanners: list[BaseScanner] | None = None,
    ) -> None:
        self._pii = pii_scanner
        self._injection = injection_scanner
        self._toxicity = toxicity_scanner
        self._custom = custom_scanners or []

    async def Analyze(  # type: ignore[override]
        self,
        request: AnalyzeRequest,
        context: ServicerContext,
    ) -> AnalyzeResponse:
        """Run deep semantic analysis (gRPC entry point)."""
        start = time.monotonic()

        scan_types = set(request.scan_types)
        tasks: list[asyncio.Task] = []
        task_labels: list[str] = []

        if "pii" in scan_types:
            tasks.append(asyncio.ensure_future(self._pii.scan(request.content)))
            task_labels.append("pii")
        if "injection" in scan_types:
            tasks.append(asyncio.ensure_future(self._injection.scan(request.content)))
            task_labels.append("injection")

        results = await asyncio.gather(*tasks, return_exceptions=True)

        raw_findings: list[Finding] = []
        scanner_results: list[ScannerResult] = []

        for label, result in zip(task_labels, results):
            if isinstance(result, Exception):
                logger.warning("scanner %s failed (fail-open): %s", label, result)
                continue
            if isinstance(result, ScanResult):
                scanner_results.append(_scan_result_to_pb(result))
                for f in result.findings:
                    raw_findings.append(_app_finding_to_pb(f))

        if "toxicity" in scan_types:
            try:
                tox_result = await self._toxicity.scan(request.content)
                scanner_results.append(_scan_result_to_pb(tox_result))
                for f in tox_result.findings:
                    raw_findings.append(_app_finding_to_pb(f))
            except Exception as exc:
                logger.warning("toxicity scanner failed (fail-open): %s", exc)

        # Custom scanner SDK.
        if "custom" in scan_types or any(
            s.name in scan_types for s in self._custom
        ):
            for scanner in self._custom:
                try:
                    result = await scanner.scan(request.content)
                    scanner_results.append(_scan_result_to_pb(result))
                    for f in result.findings:
                        raw_findings.append(_app_finding_to_pb(f))
                except Exception as exc:
                    logger.warning(
                        "custom scanner %s failed (fail-open): %s",
                        scanner.name, exc,
                    )

        duration = (time.monotonic() - start) * 1000

        logger.info(
            "gRPC Analyze: request=%s findings=%d duration=%.1fms",
            request.request_id,
            len(raw_findings),
            duration,
        )

        return AnalyzeResponse(
            request_id=request.request_id,
            findings=raw_findings,
            duration_ms=round(duration, 2),
            scanner_results=scanner_results,
        )

    async def HealthCheck(  # type: ignore[override]
        self,
        request: HealthCheckRequest,
        context: ServicerContext,
    ) -> HealthCheckResponse:
        """Standard gRPC health probe."""
        return HealthCheckResponse(status="ok", service="tamga-analyzer")


async def serve_grpc(
    pii_scanner: DeepPIIScanner,
    injection_scanner: LLMInjectionScanner,
    toxicity_scanner: ToxicityScanner,
    custom_scanners: list[BaseScanner] | None = None,
    port: int = 50051,
) -> grpc.aio.Server:
    """Start the gRPC server and return it (caller manages lifecycle).

    The server listens on all interfaces on *port*.  It runs in the same
    asyncio event loop as the FastAPI application.
    """
    server = grpc.aio.server(
        options=[
            ("grpc.keepalive_time_ms", 30000),
            ("grpc.keepalive_timeout_ms", 5000),
            ("grpc.keepalive_permit_without_calls", True),
            ("grpc.http2.max_pings_without_data", 0),
        ],
    )

    servicer = AnalyzerServicer(pii_scanner, injection_scanner, toxicity_scanner, custom_scanners)
    add_AnalyzerServiceServicer_to_server(servicer, server)

    listen_addr = f"[::]:{port}"
    server.add_insecure_port(listen_addr)

    await server.start()
    logger.info("gRPC server listening on %s", listen_addr)
    return server
