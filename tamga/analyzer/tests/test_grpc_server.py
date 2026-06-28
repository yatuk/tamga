"""Tests for the gRPC AnalyzerServicer — Analyze and HealthCheck RPCs.

Uses sys.modules mocking so scanner dependencies (presidio, anthropic)
are not required at test time.  Protobuf stubs must be generated.
"""

import asyncio
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

# Guard: skip the entire module if grpcio is not installed.
pytest.importorskip("grpc")

# ═══════════════════════════════════════════════════════════════════════════════
# Mock scanner modules so app.grpc_server can be imported without the full
# dependency tree of presidio, anthropic, llm_guard, etc.
# ═══════════════════════════════════════════════════════════════════════════════

_ORIGINAL_SCANNER_MODULES: dict[str, types.ModuleType | None] = {}
_SCANNER_NAMES = [
    ("app.scanners.pii_deep", "DeepPIIScanner"),
    ("app.scanners.injection_llm", "LLMInjectionScanner"),
    ("app.scanners.toxicity", "ToxicityScanner"),
]

for _name, _cls_name in _SCANNER_NAMES:
    _ORIGINAL_SCANNER_MODULES[_name] = sys.modules.pop(_name, None)

for _name, _cls_name in _SCANNER_NAMES:
    _mod = types.ModuleType(_name)

    # Provide a minimal scanner class.
    _ScannerCls = type(
        _cls_name,
        (),
        {
            "name": _cls_name.lower(),
            "__init__": lambda self, *a, **kw: None,
            "scan": AsyncMock(),
        },
    )
    setattr(_mod, _cls_name, _ScannerCls)

    # Provide other module-level names that might be imported.
    _mod._init_presidio_worker = MagicMock()
    _mod._sync_scan = MagicMock()
    _mod._init_llm_guard_worker = MagicMock()
    _mod._is_llm_guard_available = MagicMock(return_value=False)
    _mod._sync_scan = MagicMock()
    _mod.logger = MagicMock()

    sys.modules[_name] = _mod

# Now it is safe to import the modules under test.
from app.grpc_server import AnalyzerServicer, _app_finding_to_pb, _scan_result_to_pb  # noqa: E402
from app.scanners.base import Finding, ScanResult  # noqa: E402
from analyzer.v1.tamga_pb2 import (  # type: ignore[import-untyped]  # noqa: E402
    AnalyzeRequest,
    AnalyzeResponse,
    Finding as PBFinding,
    ScannerResult as PBScannerResult,
    HealthCheckRequest,
    HealthCheckResponse,
)

# Restore the original scanner modules so other test files (test_pii_deep,
# test_injection_llm, test_toxicity) can import the real versions.
for _name, _orig in _ORIGINAL_SCANNER_MODULES.items():
    if _orig is not None:
        sys.modules[_name] = _orig
    else:
        sys.modules.pop(_name, None)


# ── Helpers ────────────────────────────────────────────────────────────────────


def _mock_context():
    """Return a MagicMock that satisfies grpc.aio.ServicerContext."""
    ctx = MagicMock()
    ctx.set_code = MagicMock()
    ctx.set_details = MagicMock()
    ctx.abort = MagicMock()
    ctx.abort_with_status = MagicMock()
    ctx.invocation_metadata = MagicMock(return_value=[])
    return ctx


def _make_pii_scan_result() -> ScanResult:
    """Return a ScanResult with sample PII findings."""
    return ScanResult(
        scanner="pii_deep",
        findings=[
            Finding(type="pii", category="EMAIL_ADDRESS", severity="medium", match="u…@e…", confidence=0.85),
            Finding(type="pii", category="PERSON", severity="high", match="John", confidence=0.92),
        ],
        duration_ms=1.5,
    )


def _make_injection_scan_result() -> ScanResult:
    """Return a ScanResult with a sample injection finding."""
    return ScanResult(
        scanner="injection_llm",
        findings=[
            Finding(
                type="injection",
                category="ignore_prev",
                severity="critical",
                match="Ignore all previous instructions",
                confidence=0.93,
            ),
        ],
        duration_ms=12.3,
    )


def _make_toxicity_scan_result() -> ScanResult:
    """Return a ScanResult with a sample toxicity finding."""
    return ScanResult(
        scanner="toxicity",
        findings=[
            Finding(type="toxicity", category="toxicity", severity="high", match="toxic output", confidence=0.88),
        ],
        duration_ms=5.0,
    )


def _make_empty_scan_result(scanner_name: str = "test") -> ScanResult:
    """Return an empty ScanResult."""
    return ScanResult(scanner=scanner_name, findings=[], duration_ms=0.0)


def _build_servicer(
    pii_scanner=None,
    injection_scanner=None,
    toxicity_scanner=None,
    custom_scanners=None,
) -> AnalyzerServicer:
    """Build an AnalyzerServicer with mock scanners."""
    if pii_scanner is None:
        pii_scanner = MagicMock()
        pii_scanner.scan = AsyncMock(return_value=_make_empty_scan_result("pii_deep"))
    if injection_scanner is None:
        injection_scanner = MagicMock()
        injection_scanner.scan = AsyncMock(return_value=_make_empty_scan_result("injection_llm"))
    if toxicity_scanner is None:
        toxicity_scanner = MagicMock()
        toxicity_scanner.scan = AsyncMock(return_value=_make_empty_scan_result("toxicity"))
    return AnalyzerServicer(
        pii_scanner=pii_scanner,
        injection_scanner=injection_scanner,
        toxicity_scanner=toxicity_scanner,
        custom_scanners=custom_scanners,
    )


# ── Tests ──────────────────────────────────────────────────────────────────────


class TestAnalyzeRPC:
    """Tests for the Analyze RPC method."""

    def test_analyze_happy_path(self):
        """PII + injection scan_types — returns findings with correct request_id."""
        pii = MagicMock()
        pii.scan = AsyncMock(return_value=_make_pii_scan_result())
        inj = MagicMock()
        inj.scan = AsyncMock(return_value=_make_injection_scan_result())
        tox = MagicMock()
        tox.scan = AsyncMock(return_value=_make_empty_scan_result("toxicity"))

        servicer = _build_servicer(pii_scanner=pii, injection_scanner=inj, toxicity_scanner=tox)
        req = AnalyzeRequest(request_id="req-001", content="test content", scan_types=["pii", "injection"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-001"
        assert len(response.findings) == 3  # 2 PII + 1 injection
        assert response.duration_ms >= 0
        # Verify scanner_results are present.
        assert len(response.scanner_results) == 2  # pii + injection (toxicity not in scan_types)
        scanner_names = {sr.scanner for sr in response.scanner_results}
        assert scanner_names == {"pii_deep", "injection_llm"}

        pii.scan.assert_awaited_once_with("test content")
        inj.scan.assert_awaited_once_with("test content")

    def test_analyze_empty_scan_types(self):
        """Empty scan_types list — returns empty AnalyzeResponse, no error."""
        servicer = _build_servicer()
        req = AnalyzeRequest(request_id="req-empty", content="whatever", scan_types=[])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-empty"
        assert response.findings == []
        assert response.scanner_results == []
        assert response.duration_ms >= 0

    def test_analyze_scanner_exception(self):
        """One scanner raises — the other scanner's findings are still returned (fail-open)."""
        pii = MagicMock()
        pii.scan = AsyncMock(side_effect=RuntimeError("Presidio unavailable"))
        inj = MagicMock()
        inj.scan = AsyncMock(return_value=_make_injection_scan_result())
        tox = MagicMock()
        tox.scan = AsyncMock(return_value=_make_empty_scan_result("toxicity"))

        servicer = _build_servicer(pii_scanner=pii, injection_scanner=inj, toxicity_scanner=tox)
        req = AnalyzeRequest(request_id="req-failopen", content="test", scan_types=["pii", "injection"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-failopen"
        # PII scanner failed but injection scanner succeeded.
        assert len(response.findings) >= 1
        finding_types = {f.type for f in response.findings}
        assert "injection" in finding_types
        assert "pii" not in finding_types  # PII scanner failed

        scanner_names = {sr.scanner for sr in response.scanner_results}
        assert scanner_names == {"injection_llm"}

    def test_analyze_toxicity(self):
        """scan_type includes 'toxicity' — calls toxicity scanner and returns its findings."""
        pii = MagicMock()
        pii.scan = AsyncMock(return_value=_make_empty_scan_result("pii_deep"))
        inj = MagicMock()
        inj.scan = AsyncMock(return_value=_make_empty_scan_result("injection_llm"))
        tox = MagicMock()
        tox.scan = AsyncMock(return_value=_make_toxicity_scan_result())

        servicer = _build_servicer(pii_scanner=pii, injection_scanner=inj, toxicity_scanner=tox)
        req = AnalyzeRequest(request_id="req-tox", content="bad stuff", scan_types=["toxicity"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-tox"
        assert len(response.findings) == 1
        assert response.findings[0].type == "toxicity"
        assert response.findings[0].severity == "high"

        scanner_names = {sr.scanner for sr in response.scanner_results}
        assert scanner_names == {"toxicity"}
        tox.scan.assert_awaited_once_with("bad stuff")

        # PII and injection should NOT have been called.
        pii.scan.assert_not_awaited()
        inj.scan.assert_not_awaited()

    def test_analyze_invalid_scan_type(self):
        """Unknown scan_types are ignored — no crash."""
        servicer = _build_servicer()
        req = AnalyzeRequest(request_id="req-unknown", content="test", scan_types=["bogus_scanner"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-unknown"
        assert response.findings == []
        assert response.scanner_results == []
        assert response.duration_ms >= 0

    def test_analyze_toxicity_scanner_exception(self):
        """Toxicity scanner raises — fail-open, no findings from toxicity."""
        pii = MagicMock()
        pii.scan = AsyncMock(return_value=_make_pii_scan_result())
        inj = MagicMock()
        inj.scan = AsyncMock(return_value=_make_empty_scan_result("injection_llm"))
        tox = MagicMock()
        tox.scan = AsyncMock(side_effect=RuntimeError("LLM Guard crashed"))

        servicer = _build_servicer(pii_scanner=pii, injection_scanner=inj, toxicity_scanner=tox)
        req = AnalyzeRequest(request_id="req-tox-fail", content="test", scan_types=["pii", "toxicity"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-tox-fail"
        # PII findings should be present; toxicity failed.
        assert len(response.findings) >= 1
        finding_types = {f.type for f in response.findings}
        assert "pii" in finding_types
        assert "toxicity" not in finding_types

    def test_analyze_custom_scanner(self):
        """Custom scanner is invoked when 'custom' is in scan_types."""
        pii = MagicMock()
        pii.scan = AsyncMock(return_value=_make_empty_scan_result("pii_deep"))
        inj = MagicMock()
        inj.scan = AsyncMock(return_value=_make_empty_scan_result("injection_llm"))
        tox = MagicMock()
        tox.scan = AsyncMock(return_value=_make_empty_scan_result("toxicity"))

        custom = MagicMock()
        custom.name = "my_custom_scanner"
        custom.scan = AsyncMock(
            return_value=ScanResult(
                scanner="my_custom_scanner",
                findings=[Finding(type="custom", category="test", severity="low", match="matched", confidence=0.5)],
                duration_ms=1.0,
            )
        )

        servicer = _build_servicer(
            pii_scanner=pii, injection_scanner=inj, toxicity_scanner=tox,
            custom_scanners=[custom],
        )
        req = AnalyzeRequest(request_id="req-custom", content="test", scan_types=["custom"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-custom"
        assert len(response.findings) == 1
        assert response.findings[0].type == "custom"
        assert response.findings[0].category == "test"
        custom.scan.assert_awaited_once_with("test")

    def test_analyze_custom_scanner_by_name(self):
        """Custom scanner is invoked when its specific name is in scan_types (not 'custom')."""
        custom = MagicMock()
        custom.name = "fraud_detector"
        custom.scan = AsyncMock(
            return_value=ScanResult(
                scanner="fraud_detector",
                findings=[Finding(type="fraud", category="suspicious", severity="medium", match="match", confidence=0.7)],
                duration_ms=2.0,
            )
        )

        servicer = _build_servicer(custom_scanners=[custom])
        req = AnalyzeRequest(request_id="req-fraud", content="test", scan_types=["fraud_detector"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-fraud"
        assert len(response.findings) == 1
        assert response.findings[0].type == "fraud"
        custom.scan.assert_awaited_once_with("test")

    def test_analyze_custom_scanner_exception(self):
        """Custom scanner raises — fail-open, other findings still returned."""
        pii = MagicMock()
        pii.scan = AsyncMock(return_value=_make_pii_scan_result())
        inj = MagicMock()
        inj.scan = AsyncMock(return_value=_make_empty_scan_result("injection_llm"))
        tox = MagicMock()
        tox.scan = AsyncMock(return_value=_make_empty_scan_result("toxicity"))

        custom = MagicMock()
        custom.name = "broken_scanner"
        custom.scan = AsyncMock(side_effect=Exception("Boom"))

        servicer = _build_servicer(
            pii_scanner=pii, injection_scanner=inj, toxicity_scanner=tox,
            custom_scanners=[custom],
        )
        req = AnalyzeRequest(request_id="req-customfail", content="test", scan_types=["pii", "custom"])
        ctx = _mock_context()

        response: AnalyzeResponse = asyncio.run(servicer.Analyze(req, ctx))

        assert response.request_id == "req-customfail"
        # PII findings should be present; custom scanner failed.
        assert len(response.findings) >= 1
        finding_types = {f.type for f in response.findings}
        assert "pii" in finding_types


class TestHealthCheckRPC:
    """Tests for the HealthCheck RPC method."""

    def test_health_check(self):
        """HealthCheck RPC returns status 'ok' and service 'tamga-analyzer'."""
        servicer = _build_servicer()
        req = HealthCheckRequest()
        ctx = _mock_context()

        response: HealthCheckResponse = asyncio.run(servicer.HealthCheck(req, ctx))

        assert response.status == "ok"
        assert response.service == "tamga-analyzer"


class TestProtobufConverters:
    """Tests for the internal _app_finding_to_pb and _scan_result_to_pb helpers."""

    def test_app_finding_to_pb(self):
        """_app_finding_to_pb converts an app Finding to a protobuf Finding."""
        app_finding = Finding(
            type="pii",
            category="EMAIL",
            severity="high",
            match="user@ex…",
            confidence=0.95,
        )
        pb = _app_finding_to_pb(app_finding)

        assert isinstance(pb, PBFinding)
        assert pb.type == "pii"
        assert pb.category == "EMAIL"
        assert pb.severity == "high"
        assert pb.match == "user@ex…"
        assert pb.confidence == 0.95

    def test_scan_result_to_pb(self):
        """_scan_result_to_pb converts an app ScanResult to a protobuf ScannerResult."""
        sr = ScanResult(
            scanner="pii_deep",
            findings=[
                Finding(type="pii", category="PERSON", severity="medium", match="John", confidence=0.92),
            ],
            duration_ms=3.5,
        )
        pb = _scan_result_to_pb(sr)

        assert isinstance(pb, PBScannerResult)
        assert pb.scanner == "pii_deep"
        assert pb.duration_ms == 3.5
        assert len(pb.findings) == 1
        assert pb.findings[0].type == "pii"
        assert pb.findings[0].confidence == 0.92
