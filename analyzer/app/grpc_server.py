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

from app import service_auth
from app.scanners.base import BaseScanner, Finding as AppFinding
from app.scanners.base import ScanResult
from app.scanners.egress import content_is_blocked
from app.scanners.pii_deep import DeepPIIScanner
from app.scanners.injection_llm import LLMInjectionScanner
from app.removed import report_removed_scan_types

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
        custom_scanners: list[BaseScanner] | None = None,
    ) -> None:
        self._pii = pii_scanner
        self._injection = injection_scanner
        self._custom = custom_scanners or []

    async def Analyze(  # type: ignore[override]
        self,
        request: AnalyzeRequest,
        context: ServicerContext,
    ) -> AnalyzeResponse:
        """Run deep semantic analysis (gRPC entry point)."""
        start = time.monotonic()

        scan_types = set(request.scan_types)
        # "toxicity" was removed with llm-guard: logged once, answers nothing.
        report_removed_scan_types(scan_types)
        tasks: list[asyncio.Task] = []
        task_labels: list[str] = []

        if "pii" in scan_types:
            tasks.append(asyncio.ensure_future(self._pii.scan(request.content)))
            task_labels.append("pii")
        # The injection scanner is the LLM judge: when it is on, it sends the
        # text to a model provider. It is never asked about a request the
        # proxy blocked (see app.scanners.egress).
        if "injection" in scan_types and not content_is_blocked(request.metadata):
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
    custom_scanners: list[BaseScanner] | None = None,
    port: int = 50051,
    token: str = "",
    credentials: grpc.ServerCredentials | None = None,
) -> grpc.aio.Server:
    """Start the gRPC server and return it (caller manages lifecycle).

    The server listens on all interfaces on *port*.  It runs in the same
    asyncio event loop as the FastAPI application. With *token*, a call
    without it is refused; with *credentials*, the port speaks TLS.
    """
    server = grpc.aio.server(
        interceptors=[service_auth.TokenInterceptor(token)],
        options=[
            ("grpc.keepalive_time_ms", 30000),
            ("grpc.keepalive_timeout_ms", 5000),
            ("grpc.keepalive_permit_without_calls", True),
            ("grpc.http2.max_pings_without_data", 0),
        ],
    )

    servicer = AnalyzerServicer(pii_scanner, injection_scanner, custom_scanners)
    add_AnalyzerServiceServicer_to_server(servicer, server)

    listen_addr = f"[::]:{port}"
    if credentials is not None:
        server.add_secure_port(listen_addr, credentials)
    else:
        server.add_insecure_port(listen_addr)

    await server.start()
    logger.info("gRPC server listening on %s (token %s, tls %s)", listen_addr, bool(token), credentials is not None)
    return server
