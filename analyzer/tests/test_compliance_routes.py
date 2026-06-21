"""Tests for FastAPI compliance and report routes.

Uses sys.modules mocking so scanner dependencies (presidio, anthropic,
llm_guard) are not required at test time.  The compliance/report routes
themselves do not use scanners — only the module-level imports in
app.main need to be satisfied.
"""

import re
import sys
import types
import zlib
from unittest.mock import MagicMock, patch

import pytest

# ═══════════════════════════════════════════════════════════════════════════════
# Mock scanner modules so app.main can be imported without the full
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
    _ScannerCls = type(
        _cls_name,
        (),
        {
            "name": _cls_name.lower(),
            "__init__": lambda self, *a, **kw: None,
            "scan": MagicMock(),
        },
    )
    setattr(_mod, _cls_name, _ScannerCls)
    # Provide other attributes that may be imported.
    _mod.Finding = MagicMock()
    _mod.ScanResult = MagicMock()
    _mod._init_presidio_worker = MagicMock()
    _mod._sync_scan = MagicMock()
    _mod._init_llm_guard_worker = MagicMock()
    _mod._is_llm_guard_available = MagicMock(return_value=False)
    _mod.logger = MagicMock()
    sys.modules[_name] = _mod

# Mock tamga_sdk.discovery so the custom scanner import in app.main succeeds.
_tamga_sdk_orig = sys.modules.pop("tamga_sdk.discovery", None)
mock_sdk = types.ModuleType("tamga_sdk.discovery")
mock_sdk.discover_scanners = lambda: []
mock_sdk.get_custom_scanners = lambda: []
sys.modules["tamga_sdk.discovery"] = mock_sdk

# Also ensure tamga_sdk itself exists (imported by app.grpc_server).
_tamga_sdk_pkg_orig = sys.modules.pop("tamga_sdk", None)
mock_sdk_pkg = types.ModuleType("tamga_sdk")
mock_sdk_pkg.discovery = mock_sdk
sys.modules["tamga_sdk"] = mock_sdk_pkg

# Now it is safe to import app.main.
from app.main import app  # noqa: E402
from fastapi.testclient import TestClient  # noqa: E402

# Restore the original scanner modules so other test files can import the
# real versions.
for _name, _orig in _ORIGINAL_SCANNER_MODULES.items():
    if _orig is not None:
        sys.modules[_name] = _orig
    else:
        sys.modules.pop(_name, None)
if _tamga_sdk_orig is not None:
    sys.modules["tamga_sdk.discovery"] = _tamga_sdk_orig
else:
    sys.modules.pop("tamga_sdk.discovery", None)
if _tamga_sdk_pkg_orig is not None:
    sys.modules["tamga_sdk"] = _tamga_sdk_pkg_orig
else:
    sys.modules.pop("tamga_sdk", None)

client = TestClient(app)


# ── Tests: Compliance routes ────────────────────────────────────────────────────


class TestComplianceOWASP:
    """Tests for GET /api/v1/compliance/owasp."""

    def test_compliance_owasp_returns_200(self):
        """GET /api/v1/compliance/owasp returns HTTP 200."""
        response = client.get("/api/v1/compliance/owasp")
        assert response.status_code == 200

    def test_compliance_owasp_response_has_expected_keys(self):
        """Response includes owasp_coverage_pct, owasp_details, overall_score, etc."""
        response = client.get("/api/v1/compliance/owasp")
        body = response.json()

        assert "owasp_coverage_pct" in body
        assert "owasp_details" in body
        assert "eu_ai_act_checks" in body
        assert "privacy_entity_coverage" in body
        assert "overall_score" in body
        assert "critical_gaps" in body

    def test_compliance_owasp_coverage_pct_is_reasonable(self):
        """owasp_coverage_pct is a float between 0 and 100."""
        response = client.get("/api/v1/compliance/owasp")
        body = response.json()
        pct = body["owasp_coverage_pct"]
        assert isinstance(pct, (int, float))
        assert 0.0 <= pct <= 100.0

    def test_compliance_owasp_details_is_list(self):
        """owasp_details is a non-empty list of risk entries."""
        response = client.get("/api/v1/compliance/owasp")
        body = response.json()
        details = body["owasp_details"]
        assert isinstance(details, list)
        assert len(details) >= 1
        # Each entry should have id, title, coverage, scanners, recommendations.
        for entry in details:
            assert "id" in entry
            assert "title" in entry
            assert "coverage" in entry
            assert "scanners" in entry
            assert "recommendations" in entry

    def test_compliance_owasp_overall_score_is_reasonable(self):
        """overall_score is a float between 0 and 100."""
        response = client.get("/api/v1/compliance/owasp")
        body = response.json()
        score = body["overall_score"]
        assert isinstance(score, (int, float))
        assert 0.0 <= score <= 100.0

    def test_compliance_owasp_includes_eu_ai_act_checks(self):
        """eu_ai_act_checks list has expected fields."""
        response = client.get("/api/v1/compliance/owasp")
        body = response.json()
        eu_checks = body["eu_ai_act_checks"]
        assert isinstance(eu_checks, list)
        assert len(eu_checks) >= 1
        for check in eu_checks:
            assert "article" in check
            assert "requirement" in check
            assert "status" in check

    def test_compliance_owasp_includes_privacy_coverage(self):
        """privacy_entity_coverage list has expected fields."""
        response = client.get("/api/v1/compliance/owasp")
        body = response.json()
        priv = body["privacy_entity_coverage"]
        assert isinstance(priv, list)
        assert len(priv) >= 1
        for entry in priv:
            assert "entity" in entry
            assert "regulation" in entry
            assert "supported" in entry


class TestCompliancePrivacy:
    """Tests for GET /api/v1/compliance/privacy."""

    def test_compliance_privacy_returns_200(self):
        """GET /api/v1/compliance/privacy returns HTTP 200."""
        response = client.get("/api/v1/compliance/privacy")
        assert response.status_code == 200

    def test_compliance_privacy_has_expected_keys(self):
        """Response includes meta, total_entities, supported_entities, coverage_pct, entities."""
        response = client.get("/api/v1/compliance/privacy")
        body = response.json()

        assert "meta" in body
        assert "total_entities" in body
        assert "supported_entities" in body
        assert "coverage_pct" in body
        assert "entities" in body

    def test_compliance_privacy_coverage_pct_is_reasonable(self):
        """coverage_pct is a float between 0 and 100."""
        response = client.get("/api/v1/compliance/privacy")
        body = response.json()
        pct = body["coverage_pct"]
        assert isinstance(pct, (int, float))
        assert 0.0 <= pct <= 100.0

    def test_compliance_privacy_entities_list_has_fields(self):
        """Each entity in the entities list has entity, regulation, tamga_supported, scanner."""
        response = client.get("/api/v1/compliance/privacy")
        body = response.json()
        entities = body["entities"]
        assert isinstance(entities, list)
        assert len(entities) >= 1
        for entry in entities:
            assert "entity" in entry
            assert "regulation" in entry
            assert "tamga_supported" in entry
            assert "scanner" in entry

    def test_compliance_privacy_response_is_json(self):
        """Response content-type is application/json."""
        response = client.get("/api/v1/compliance/privacy")
        assert "application/json" in response.headers.get("content-type", "")


class TestReportsOWASPPDF:
    """Tests for GET /api/v1/reports/owasp/pdf."""

    def test_reports_owasp_pdf_returns_200_or_501(self):
        """GET /api/v1/reports/owasp/pdf returns 200 (PDF available) or 501 (ReportLab missing)."""
        response = client.get("/api/v1/reports/owasp/pdf")
        assert response.status_code in (200, 501)

    def test_reports_owasp_pdf_error_body_when_501(self):
        """When 501 is returned, the body includes an error message."""
        response = client.get("/api/v1/reports/owasp/pdf")
        if response.status_code == 501:
            body = response.json()
            assert "error" in body
            assert "PDF" in body["error"]

    def test_reports_owasp_pdf_media_type_when_200(self):
        """When 200 is returned, the content-type is application/pdf."""
        response = client.get("/api/v1/reports/owasp/pdf")
        if response.status_code == 200:
            assert response.headers.get("content-type", "").startswith("application/pdf")
            assert len(response.content) > 0


class TestReportsIncidentPDF:
    """Tests for GET /api/v1/reports/incident/pdf."""

    def test_reports_incident_pdf_returns_200_or_501(self):
        """GET /api/v1/reports/incident/pdf returns 200 or 501."""
        response = client.get("/api/v1/reports/incident/pdf")
        assert response.status_code in (200, 501)

    def test_reports_incident_pdf_with_query_params(self):
        """The incident PDF endpoint accepts query parameters."""
        response = client.get(
            "/api/v1/reports/incident/pdf?total_requests=1000&blocked=50&redacted=30&warned=10&period_hours=48"
        )
        assert response.status_code in (200, 501)


# ── PDF text extraction helper ────────────────────────────────────────────────
# PDF content streams use ASCII85 + FlateDecode compression.  This helper
# decompresses them so tests can verify actual report content rather than
# just checking status codes.


def _ascii85_decode_adobe(data: bytes) -> bytes:
    """Decode Adobe ASCII85 encoded data (delimited by <~ ... ~>)."""
    if b"<~" in data:
        data = data.split(b"<~", 1)[1]
    if b"~>" in data:
        data = data.split(b"~>", 1)[0]

    result = bytearray()
    group = bytearray()
    for ch in data:
        if 33 <= ch <= 117:  # "!" to "u"
            group.append(ch)
            if len(group) == 5:
                result.extend(_decode_ascii85_group(group))
                group.clear()

    if len(group) > 0:
        padding = 5 - len(group)
        group.extend(b"u" * padding)
        decoded = _decode_ascii85_group(group)
        result.extend(decoded[: len(decoded) - padding])

    return bytes(result)


def _decode_ascii85_group(group: bytearray) -> bytes:
    n = 0
    for c in group:
        n = n * 85 + (c - 33)
    return bytes([(n >> 24) & 0xFF, (n >> 16) & 0xFF, (n >> 8) & 0xFF, n & 0xFF])


def extract_pdf_text(pdf_bytes: bytes) -> str:
    """Extract decompressed text from PDF content streams.

    Handles the ASCII85Decode + FlateDecode filter chain used by ReportLab.
    """
    texts: list[str] = []
    # Match: <<dict>> newline stream newline — standard PDF object structure.
    obj_pattern = re.compile(rb"<<(.*?)>>\s*\nstream\n", re.DOTALL)

    for m in obj_pattern.finditer(pdf_bytes):
        obj_dict = m.group(1)

        filter_match = re.search(rb"/Filter\s*\[([^\]]*)\]", obj_dict)
        if not filter_match:
            continue
        filters = [
            f.strip().lstrip(b"/")
            for f in filter_match.group(1).split()
            if f.strip()
        ]

        length_match = re.search(rb"/Length\s+(\d+)", obj_dict)
        if not length_match:
            continue
        length = int(length_match.group(1))

        stream_start = m.end()
        stream_data = pdf_bytes[stream_start : stream_start + length]

        try:
            decoded = stream_data
            for f in filters:
                fname = f.decode()
                if "FlateDecode" in fname:
                    decoded = zlib.decompress(decoded)
                elif "ASCII85Decode" in fname:
                    decoded = _ascii85_decode_adobe(decoded)
            texts.append(decoded.decode("latin-1", errors="replace"))
        except Exception:
            pass

    return "\n".join(texts)


# ── Content verification tests (require ReportLab) ────────────────────────────

# ReportLab is required for PDF content-verification tests.
# When ReportLab is not installed, these tests are skipped gracefully.
reportlab = pytest.importorskip("reportlab", reason="ReportLab required for PDF content verification")


@pytest.mark.skipif(reportlab is None, reason="ReportLab not installed")
class TestPDFContentVerification:
    """Verify that generated PDFs contain expected content (decompressed)."""

    def test_owasp_pdf_content(self):
        """OWASP PDF contains key section headers and risk terms."""
        response = client.get("/api/v1/reports/owasp/pdf")
        assert response.status_code == 200
        pdf_bytes = response.content

        # PDF magic bytes.
        assert pdf_bytes.startswith(b"%PDF")

        # Raw-bytes checks (document metadata is uncompressed).
        assert b"OWASP" in pdf_bytes
        assert b"LLM" in pdf_bytes

        # Decompressed content checks (key section headers).
        text = extract_pdf_text(pdf_bytes)
        assert "Detailed Risk Assessment" in text
        assert "Privacy Entity Coverage" in text
        assert "Overall OWASP Coverage" in text

    def test_incident_pdf_content(self):
        """Incident PDF contains KPI labels and passed-in numbers."""
        response = client.get(
            "/api/v1/reports/incident/pdf"
            "?total_requests=1234&blocked=567&redacted=89&warned=12&period_hours=48"
        )
        assert response.status_code == 200
        pdf_bytes = response.content

        assert pdf_bytes.startswith(b"%PDF")

        # Raw-bytes checks.
        assert b"Incident" in pdf_bytes

        # Decompressed content checks (KPI labels and numbers).
        text = extract_pdf_text(pdf_bytes)
        assert "Total Requests" in text
        assert "Blocked" in text
        assert "Redacted" in text
        assert "Warned" in text
        # KPI numbers are embedded in the content stream.
        assert "1234" in text
        assert "567" in text

    def test_pdf_content_type(self):
        """PDF responses carry Content-Type: application/pdf."""
        response = client.get("/api/v1/reports/owasp/pdf")
        assert response.status_code == 200
        ct = response.headers.get("content-type", "")
        assert "application/pdf" in ct

        response2 = client.get("/api/v1/reports/incident/pdf")
        assert response2.status_code == 200
        ct2 = response2.headers.get("content-type", "")
        assert "application/pdf" in ct2

    def test_pdf_content_disposition(self):
        """PDF responses carry Content-Disposition attachment header."""
        response = client.get("/api/v1/reports/owasp/pdf")
        assert response.status_code == 200
        cd = response.headers.get("content-disposition", "")
        assert "attachment" in cd
        assert "tamga-owasp-report.pdf" in cd


# ── 501 tests (mock ReportLab as unavailable) ────────────────────────────────


class TestReports501WhenReportLabMissing:
    """Verify 501 Not Implemented response when ReportLab is not importable."""

    def test_owasp_pdf_501_when_reportlab_missing(self):
        """Return 501 with JSON body when ReportLab is unavailable."""
        with patch("app.main.generate_owasp_pdf_report", side_effect=ImportError("ReportLab not installed")):
            response = client.get("/api/v1/reports/owasp/pdf")
            assert response.status_code == 501
            body = response.json()
            assert "error" in body
            assert "ReportLab not installed" in body["error"]
            assert body.get("available") is False
            assert response.headers.get("content-type", "").startswith("application/json")

    def test_incident_pdf_501_when_reportlab_missing(self):
        """Return 501 with JSON body when ReportLab is unavailable."""
        # The incident route imports the function inside the handler body,
        # so we patch the source module rather than app.main.
        with patch("app.reports.generate_incident_pdf_report", side_effect=ImportError("ReportLab not installed")):
            response = client.get("/api/v1/reports/incident/pdf")
            assert response.status_code == 501
            body = response.json()
            assert "error" in body
            assert "ReportLab not installed" in body["error"]
            assert body.get("available") is False

    def test_owasp_pdf_501_body_structure(self):
        """501 response body has error and available fields."""
        with patch("app.main.generate_owasp_pdf_report", side_effect=ImportError("ReportLab not installed")):
            response = client.get("/api/v1/reports/owasp/pdf")
            body = response.json()
            assert set(body.keys()) == {"error", "available"}

    def test_incident_pdf_501_body_structure(self):
        """501 response body has error and available fields."""
        with patch("app.reports.generate_incident_pdf_report", side_effect=ImportError("ReportLab not installed")):
            response = client.get("/api/v1/reports/incident/pdf")
            body = response.json()
            assert set(body.keys()) == {"error", "available"}


class TestHealthEndpoint:
    """Tests for GET /health."""

    def test_health_returns_ok(self):
        """GET /health returns status ok and service name."""
        response = client.get("/health")
        assert response.status_code == 200
        body = response.json()
        assert body["status"] == "ok"
        assert body["service"] == "tamga-analyzer"
        assert "scanners" in body
        assert "pii_deep" in body["scanners"]
        assert "injection_llm" in body["scanners"]
        assert "custom_count" in body["scanners"]
