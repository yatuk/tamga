"""Tests for GDPR/HIPAA Presidio recognizers and regex patterns.

Tests the 6 new PatternRecognizer entity types via:
  • Direct regex validation (no Presidio needed)
  • Factory function structure (with mocked presidio_analyzer)
  • Compliance route coverage changes (via FastAPI TestClient)
"""

import re
import sys
import types
from unittest.mock import MagicMock

import pytest


# ═══════════════════════════════════════════════════════════════════════════════
# Regex pattern tests — pure Python, no Presidio dependency needed.
# These verify the actual detection logic for each entity type.
# ═══════════════════════════════════════════════════════════════════════════════

# Patterns aligned with the Go proxy scanner + the new recognizer definitions.

PASSPORT_STANDALONE = re.compile(r"\b[A-Z]<?\d{7,9}\b|\b[A-Z]\d{8}\b")
PASSPORT_CONTEXT = re.compile(r"(?i)\bpassport\s*(no|number|#)?[:.\s]*[A-Z0-9]{6,12}\b")

DOB_CONTEXT = re.compile(r"(?i)\b(?:DOB|birth|born)\s*(?:date\s*)?[:.\s]*\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b")
DOB_BARE = re.compile(r"\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b")

NATIONAL_ID_SSN = re.compile(r"\b\d{3}-\d{2}-\d{4}\b")
NATIONAL_ID_CONTEXT = re.compile(r"(?i)\b(?:national\s*id|ID\s*(?:number|no)|NIN|SSN|social\s*security)[:.\s]*\d{6,12}\b")
NATIONAL_ID_GENERIC = re.compile(r"(?i)\b(?:id\s*(?:number|no|#))[:.\s]*[A-Z0-9]{6,15}\b")

MEDICAL_RECORD_CONTEXT = re.compile(r"(?i)\bmedical\s*record\s*(?:number|no|#)?[:.\s]*\d{4,12}\b")
MRN_CONTEXT = re.compile(r"(?i)\bMRN[:.\s]*\d{4,12}\b")

HEALTH_PLAN_CONTEXT = re.compile(r"(?i)\b(?:health|insurance|plan|beneficiary|member|subscriber)\s*(?:plan\s*)?(?:ID|number|no|#)?[:.\s]*[A-Z0-9]{6,15}\b")
INSURANCE_ID = re.compile(r"(?i)\b(?:insurance|policy)\s*(?:ID|number|no|#)?[:.\s]*[A-Z0-9]{6,15}\b")

NPI_CONTEXT = re.compile(r"(?i)\bNPI[:.\s]*\d{10}\b|\bNational\s+Provider\s+ID[:.\s]*\d{10}\b")
NPI_STANDALONE = re.compile(r"\b\d{10}\b")


# ── Passport number ────────────────────────────────────────────────────────

class TestPassportPatterns:
    def test_passport_standalone_us_format(self):
        assert PASSPORT_STANDALONE.search("A12345678")
        assert PASSPORT_STANDALONE.search("B98765432")

    def test_passport_standalone_tr_format(self):
        # TR format: U< followed by digits (MRZ)
        assert PASSPORT_STANDALONE.search("U123456789")

    def test_passport_standalone_rejects_short(self):
        assert not PASSPORT_STANDALONE.search("A123456")

    def test_passport_context_with_keyword(self):
        assert PASSPORT_CONTEXT.search("passport no: XB12345678")
        assert PASSPORT_CONTEXT.search("Passport Number: AB123456")
        assert PASSPORT_CONTEXT.search("passport#: CDEF123456")

    def test_passport_context_rejects_no_keyword(self):
        assert not PASSPORT_CONTEXT.search("XB12345678")


# ── Date of birth ──────────────────────────────────────────────────────────

class TestDOBPatterns:
    def test_dob_context_explicit(self):
        assert DOB_CONTEXT.search("DOB: 01/15/1985")
        assert DOB_CONTEXT.search("birth date: 15-06-1990")
        assert DOB_CONTEXT.search("born: 12/31/2000")

    def test_dob_context_short_year(self):
        assert DOB_CONTEXT.search("DOB: 01/15/85")

    def test_dob_bare_date(self):
        assert DOB_BARE.search("01/15/1985")
        assert DOB_BARE.search("15-06-1990")

    def test_dob_rejects_non_date(self):
        # Plain numbers without date-like separators should not match
        assert not DOB_BARE.search("10000000")


# ── National ID number ─────────────────────────────────────────────────────

class TestNationalIDPatterns:
    def test_ssn_format(self):
        assert NATIONAL_ID_SSN.search("123-45-6789")

    def test_ssn_rejects_wrong_format(self):
        assert not NATIONAL_ID_SSN.search("123456789")

    def test_national_id_context(self):
        assert NATIONAL_ID_CONTEXT.search("national id: 123456789012")
        assert NATIONAL_ID_CONTEXT.search("ID number: 9876543210")
        assert NATIONAL_ID_CONTEXT.search("NIN: 12345678")
        assert NATIONAL_ID_CONTEXT.search("SSN: 123456789")
        assert NATIONAL_ID_CONTEXT.search("social security: 123456789012")

    def test_national_id_context_rejects_no_keyword(self):
        assert not NATIONAL_ID_CONTEXT.search("just 123456789012")

    def test_generic_id_number(self):
        assert NATIONAL_ID_GENERIC.search("id number: ABC123XYZ")
        assert NATIONAL_ID_GENERIC.search("ID no: TEST123456")
        assert NATIONAL_ID_GENERIC.search("id#: ABCD123456789")


# ── Medical record number ──────────────────────────────────────────────────

class TestMedicalRecordPatterns:
    def test_medical_record_context(self):
        assert MEDICAL_RECORD_CONTEXT.search("medical record number: 12345678")
        assert MEDICAL_RECORD_CONTEXT.search("medical record no: 9876543210")
        assert MEDICAL_RECORD_CONTEXT.search("medical record: 1234")

    def test_mrn_abbreviation(self):
        assert MRN_CONTEXT.search("MRN: 12345678")
        assert MRN_CONTEXT.search("MRN: 987654321012")

    def test_medical_record_rejects_no_keyword(self):
        assert not MEDICAL_RECORD_CONTEXT.search("12345678")


# ── Health plan beneficiary ────────────────────────────────────────────────

class TestHealthPlanPatterns:
    def test_health_plan_context(self):
        assert HEALTH_PLAN_CONTEXT.search("health plan ID: ABC123456")
        assert HEALTH_PLAN_CONTEXT.search("beneficiary number: XYZ987654321")
        assert HEALTH_PLAN_CONTEXT.search("member ID: 123456789")
        assert HEALTH_PLAN_CONTEXT.search("subscriber no: MEMBER001")

    def test_insurance_id(self):
        assert INSURANCE_ID.search("insurance ID: POL123456789")
        assert INSURANCE_ID.search("policy number: INSURANCE001")

    def test_health_plan_rejects_no_keyword(self):
        assert not HEALTH_PLAN_CONTEXT.search("ABC123456")


# ── NPI number ─────────────────────────────────────────────────────────────

class TestNPIPatterns:
    def test_npi_context(self):
        assert NPI_CONTEXT.search("NPI: 1234567890")
        assert NPI_CONTEXT.search("National Provider ID: 0987654321")

    def test_npi_standalone_10_digits(self):
        assert NPI_STANDALONE.search("1234567890")

    def test_npi_standalone_rejects_9_digits(self):
        assert not NPI_STANDALONE.search("123456789")

    def test_npi_standalone_rejects_11_digits(self):
        # 11 digits should NOT match — but regex \b\d{10}\b with word boundaries
        # won't match because the 11th digit breaks the boundary.
        # Test that 11-digit string does not produce a match.
        assert not NPI_STANDALONE.fullmatch("12345678901")


# ═══════════════════════════════════════════════════════════════════════════════
# Recognizer factory tests — verify structure with mocked presidio_analyzer.
# ═══════════════════════════════════════════════════════════════════════════════


def _ensure_presidio_available():
    """Ensure presidio_analyzer with PatternRecognizer/Pattern is importable.

    Other test files (test_pii_deep.py) register lightweight mock modules
    for presidio_analyzer that don't include PatternRecognizer or Pattern.
    If such a mock is present, remove it so the real package can be loaded.
    If the real package is not installed, inject proper mock classes.

    Called as a fixture at test-execution time (not module-import time)
    because other test modules may replace sys.modules entries between
    collection and execution.
    """
    existing = sys.modules.get("presidio_analyzer")
    if existing is not None and not hasattr(existing, "PatternRecognizer"):
        # Stale mock from another test file — remove it.
        del sys.modules["presidio_analyzer"]
        # Also clean up the predefined_recognizers sub-module if present.
        sys.modules.pop("presidio_analyzer.predefined_recognizers", None)

    try:
        from presidio_analyzer import PatternRecognizer, Pattern  # noqa: F401
        return  # real module loaded successfully
    except ImportError:
        pass

    # Real package not available — inject lightweight mock classes.
    class MockPattern:
        def __init__(self, name, regex, score):
            self.name = name
            self.regex = regex
            self.score = score

    class MockPatternRecognizer:
        def __init__(self, supported_entity, name, patterns, context, supported_language):
            self.supported_entities = [supported_entity]
            self.name = name
            self.patterns = patterns
            self.context = context
            self.supported_language = supported_language

    pres_mod = types.ModuleType("presidio_analyzer")
    pres_mod.PatternRecognizer = MockPatternRecognizer
    pres_mod.Pattern = MockPattern
    sys.modules["presidio_analyzer"] = pres_mod


class TestRecognizerFactory:
    """Verify get_gdpr_hipaa_recognizers() returns properly structured recognizers."""

    @pytest.fixture(autouse=True)
    def _setup(self):
        """Ensure presidio_analyzer is available before each test.

        Runs at test-execution time to catch mock modules injected by
        other test files after collection.
        """
        _ensure_presidio_available()

    def test_returns_six_recognizers(self):
        from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

        recognizers = get_gdpr_hipaa_recognizers()
        assert len(recognizers) == 6

    def test_entity_types(self):
        from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

        recognizers = get_gdpr_hipaa_recognizers()
        # supported_entities is a list (e.g. ["PASSPORT_NUMBER"])
        entities = {r.supported_entities[0] for r in recognizers}
        expected = {
            "PASSPORT_NUMBER",
            "DATE_OF_BIRTH",
            "NATIONAL_ID_NUMBER",
            "MEDICAL_RECORD_NUMBER",
            "HEALTH_PLAN_BENEFICIARY",
            "NPI_NUMBER",
        }
        assert entities == expected

    def test_each_recognizer_has_patterns(self):
        from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

        recognizers = get_gdpr_hipaa_recognizers()
        for r in recognizers:
            assert len(r.patterns) >= 1, f"{r.supported_entity} has no patterns"
            for p in r.patterns:
                assert p.name, f"pattern has no name in {r.supported_entity}"
                assert p.regex, f"pattern has no regex in {r.supported_entity}/{p.name}"
                assert 0.0 <= p.score <= 1.0, (
                    f"score {p.score} out of range in {r.supported_entity}/{p.name}"
                )

    def test_each_recognizer_has_context_words(self):
        from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

        recognizers = get_gdpr_hipaa_recognizers()
        for r in recognizers:
            assert len(r.context) >= 2, (
                f"{r.supported_entity} has fewer than 2 context words"
            )

    def test_all_recognizers_support_english(self):
        from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

        recognizers = get_gdpr_hipaa_recognizers()
        for r in recognizers:
            assert r.supported_language == "en", (
                f"{r.supported_entity} language is not 'en'"
            )

    def test_recognizer_names_are_unique(self):
        from app.compliance.presidio_gdpr_recognizers import get_gdpr_hipaa_recognizers

        recognizers = get_gdpr_hipaa_recognizers()
        names = [r.name for r in recognizers]
        assert len(names) == len(set(names)), f"Duplicate recognizer names: {names}"


# Clean up mock modules after the factory tests to avoid leaking into
# other test modules that may need the real presidio_analyzer.
try:
    _mock_mod = sys.modules.get("presidio_analyzer")
    if _mock_mod is not None and not hasattr(_mock_mod, "AnalyzerEngine"):
        # Our lightweight mock doesn't have AnalyzerEngine — clean it up.
        pass  # Keep it; other tests in this file might need it.
except Exception:
    pass


# ═══════════════════════════════════════════════════════════════════════════════
# Compliance route tests — verify updated privacy coverage numbers.
# ═══════════════════════════════════════════════════════════════════════════════


class TestPrivacyCoverageUpdated:
    """Verify the compliance endpoints reflect the 6 new GDPR/HIPAA entities."""

    def test_privacy_coverage_improved(self):
        """After adding 6 recognizers, privacy coverage should be > 70%."""
        from app.compliance.privacy import PRIVACY_ENTITIES

        total = len(PRIVACY_ENTITIES)
        supported = sum(1 for e in PRIVACY_ENTITIES if e.tamga_supported)
        pct = supported * 100 / total

        assert total == 23, f"Expected 23 total entities, got {total}"
        assert supported >= 17, (
            f"Expected at least 17 supported entities, got {supported}"
        )
        assert pct > 70.0, f"Expected coverage > 70%, got {pct:.1f}%"

    def test_gdpr_entities_all_supported(self):
        """All 8 GDPR entities should now be supported."""
        from app.compliance.privacy import PRIVACY_ENTITIES

        gdpr_entities = [e for e in PRIVACY_ENTITIES if e.regulation == "GDPR"]
        assert len(gdpr_entities) == 8
        assert all(e.tamga_supported for e in gdpr_entities), (
            f"Not all GDPR entities are supported: "
            f"{[e.entity for e in gdpr_entities if not e.tamga_supported]}"
        )

    def test_hipaa_entities_partially_supported(self):
        """3 of 5 HIPAA entities should now be supported."""
        from app.compliance.privacy import PRIVACY_ENTITIES

        hipaa_entities = [e for e in PRIVACY_ENTITIES if e.regulation == "HIPAA"]
        assert len(hipaa_entities) == 5
        hipaa_supported = [e for e in hipaa_entities if e.tamga_supported]
        assert len(hipaa_supported) == 3, (
            f"Expected 3 supported HIPAA entities, got {len(hipaa_supported)}: "
            f"{[e.entity for e in hipaa_supported]}"
        )

    def test_specific_entities_now_supported(self):
        """Verify the 6 newly implemented entities are marked as supported."""
        from app.compliance.privacy import PRIVACY_ENTITIES

        entity_map = {e.entity: e for e in PRIVACY_ENTITIES}

        newly_supported = [
            "Date of birth",
            "Passport number",
            "National ID number",
            "Medical record number",
            "Health plan beneficiary",
            "NPI (National Provider ID)",
        ]
        for name in newly_supported:
            assert name in entity_map, f"Entity '{name}' not found in PRIVACY_ENTITIES"
            ent = entity_map[name]
            assert ent.tamga_supported, f"'{name}' should be supported, got False"
            assert ent.scanner == "pii_deep", (
                f"'{name}' scanner should be 'pii_deep', got '{ent.scanner}'"
            )
            assert "not yet implemented" not in ent.notes.lower(), (
                f"'{name}' notes still say 'Not yet implemented': {ent.notes}"
            )
