"""GDPR and HIPAA entity recognizers for Microsoft Presidio.

Defines PatternRecognizer instances for 6 entity types that were previously
marked "Not yet implemented" in the privacy coverage matrix. The regex
patterns are aligned with the Go proxy scanner for consistency.

All recognizers are registered in the Presidio AnalyzerEngine worker
initialisation alongside the existing spaCy NER SpacyRecognizer.
"""

from __future__ import annotations

from presidio_analyzer import PatternRecognizer, Pattern


def _build_passport_recognizer() -> PatternRecognizer:
    """Passport number — standalone format + context-keyword patterns."""
    return PatternRecognizer(
        supported_entity="PASSPORT_NUMBER",
        name="tamga_passport_recognizer",
        patterns=[
            Pattern(
                name="passport_standalone",
                regex=r"\b[A-Z]<?\d{7,9}\b|\b[A-Z]\d{8}\b",
                score=0.70,
            ),
            Pattern(
                name="passport_context",
                regex=r"(?i)\bpassport\s*(no|number|#)?[:.\s]*[A-Z0-9]{6,12}\b",
                score=0.85,
            ),
        ],
        context=["passport", "travel", "nationality", "border"],
        supported_language="en",
    )


def _build_dob_recognizer() -> PatternRecognizer:
    """Date of birth — context-keyword + bare date patterns."""
    return PatternRecognizer(
        supported_entity="DATE_OF_BIRTH",
        name="tamga_dob_recognizer",
        patterns=[
            Pattern(
                name="dob_context",
                regex=r"(?i)\b(?:DOB|birth|born)\s*(?:date\s*)?[:.\s]*\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b",
                score=0.75,
            ),
            Pattern(
                name="bare_date",
                regex=r"\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b",
                score=0.55,
            ),
        ],
        context=["birth", "born", "DOB", "age", "date"],
        supported_language="en",
    )


def _build_national_id_recognizer() -> PatternRecognizer:
    """National ID number — non-TR formats (SSN, NIN, INSEE, etc.)."""
    return PatternRecognizer(
        supported_entity="NATIONAL_ID_NUMBER",
        name="tamga_national_id_recognizer",
        patterns=[
            Pattern(
                name="ssn_pattern",
                regex=r"\b\d{3}-\d{2}-\d{4}\b",
                score=0.70,
            ),
            Pattern(
                name="context_id",
                regex=r"(?i)\b(?:national\s*id|ID\s*(?:number|no)|NIN|SSN|social\s*security)[:.\s]*\d{6,12}\b",
                score=0.85,
            ),
            Pattern(
                name="generic_id_number",
                regex=r"(?i)\b(?:id\s*(?:number|no|#))[:.\s]*[A-Z0-9]{6,15}\b",
                score=0.60,
            ),
        ],
        context=["national", "identity", "citizen", "SSN", "NIN"],
        supported_language="en",
    )


def _build_medical_record_recognizer() -> PatternRecognizer:
    """Medical record number (HIPAA) — context-keyword + digits."""
    return PatternRecognizer(
        supported_entity="MEDICAL_RECORD_NUMBER",
        name="tamga_medical_record_recognizer",
        patterns=[
            Pattern(
                name="medical_record_context",
                regex=r"(?i)\bmedical\s*record\s*(?:number|no|#)?[:.\s]*\d{4,12}\b",
                score=0.80,
            ),
            Pattern(
                name="mrn_context",
                regex=r"(?i)\bMRN[:.\s]*\d{4,12}\b",
                score=0.80,
            ),
        ],
        context=["medical", "record", "patient", "hospital", "MRN", "chart"],
        supported_language="en",
    )


def _build_health_plan_recognizer() -> PatternRecognizer:
    """Health plan beneficiary number (HIPAA) — insurance/plan ID patterns."""
    return PatternRecognizer(
        supported_entity="HEALTH_PLAN_BENEFICIARY",
        name="tamga_health_plan_recognizer",
        patterns=[
            Pattern(
                name="health_plan_context",
                regex=r"(?i)\b(?:health|insurance|plan|beneficiary|member|subscriber)\s*(?:plan\s*)?(?:ID|number|no|#)?[:.\s]*[A-Z0-9]{6,15}\b",
                score=0.80,
            ),
            Pattern(
                name="insurance_id",
                regex=r"(?i)\b(?:insurance|policy)\s*(?:ID|number|no|#)?[:.\s]*[A-Z0-9]{6,15}\b",
                score=0.75,
            ),
        ],
        context=["health", "insurance", "plan", "beneficiary", "member", "subscriber", "policy"],
        supported_language="en",
    )


def _build_npi_recognizer() -> PatternRecognizer:
    """NPI (National Provider ID, HIPAA) — 10-digit number with context."""
    return PatternRecognizer(
        supported_entity="NPI_NUMBER",
        name="tamga_npi_recognizer",
        patterns=[
            Pattern(
                name="npi_context",
                regex=r"(?i)\bNPI[:.\s]*\d{10}\b|\bNational\s+Provider\s+ID[:.\s]*\d{10}\b",
                score=0.85,
            ),
            Pattern(
                name="npi_standalone",
                regex=r"\b\d{10}\b",
                score=0.30,
            ),
        ],
        context=["NPI", "provider", "National Provider ID", "healthcare", "clinician"],
        supported_language="en",
    )


def get_gdpr_hipaa_recognizers() -> list[PatternRecognizer]:
    """Return all GDPR/HIPAA PatternRecognizer instances for registration.

    Called once per worker process during Presidio AnalyzerEngine
    initialisation. Each recognizer targets a specific entity type
    with regex patterns aligned to the Go proxy scanner.
    """
    return [
        _build_passport_recognizer(),
        _build_dob_recognizer(),
        _build_national_id_recognizer(),
        _build_medical_record_recognizer(),
        _build_health_plan_recognizer(),
        _build_npi_recognizer(),
    ]
