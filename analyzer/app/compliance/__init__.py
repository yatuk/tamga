"""Compliance assessment module — OWASP LLM Top 10, EU AI Act, KVKK/GDPR, HIPAA.

Re-exports from sub-modules:
- owasp_scorer: OWASPRisk, OWASP_LLM_TOP_10_2025
- eu_ai_act:   EUAIRiskLevel, EUAIActCheck, EU_AI_ACT_CHECKS
- privacy:     PrivacyEntity, PRIVACY_ENTITIES
"""

from app.compliance.owasp_scorer import OWASPRisk, OWASP_LLM_TOP_10_2025
from app.compliance.eu_ai_act import EUAIRiskLevel, EUAIActCheck, EU_AI_ACT_CHECKS
from app.compliance.privacy import PrivacyEntity, PRIVACY_ENTITIES

from pydantic import BaseModel

# ---------------------------------------------------------------------------
# Compliance scoring (aggregates across all sub-modules)
# ---------------------------------------------------------------------------


class ComplianceReport(BaseModel):
    owasp_coverage_pct: float
    owasp_details: list[dict] = []
    eu_ai_act_checks: list[dict] = []
    privacy_entity_coverage: list[dict] = []
    overall_score: float  # 0–100
    critical_gaps: list[str] = []


def compute_compliance_report() -> ComplianceReport:
    """Generate a compliance posture report for the current deployment."""

    # OWASP coverage
    owasp_full = sum(1 for r in OWASP_LLM_TOP_10_2025 if r.tamga_coverage == "full")
    owasp_partial = sum(1 for r in OWASP_LLM_TOP_10_2025 if r.tamga_coverage == "partial")
    owasp_n = len(OWASP_LLM_TOP_10_2025)
    owasp_pct = (owasp_full * 100 + owasp_partial * 50) / owasp_n

    owasp_details = [
        {
            "id": r.id, "title": r.title,
            "coverage": r.tamga_coverage,
            "scanners": r.mitigating_scanners,
            "recommendations": r.recommendations,
        }
        for r in OWASP_LLM_TOP_10_2025
    ]

    # EU AI Act
    eu_checks = [
        {"article": c.article, "requirement": c.requirement, "status": c.tamga_status, "notes": c.notes}
        for c in EU_AI_ACT_CHECKS
    ]
    eu_compliant = sum(1 for c in EU_AI_ACT_CHECKS if c.tamga_status == "compliant")
    eu_pct = (eu_compliant * 100 + sum(1 for c in EU_AI_ACT_CHECKS if c.tamga_status == "partial") * 50) / len(EU_AI_ACT_CHECKS)

    # Privacy entities
    priv_supported = sum(1 for e in PRIVACY_ENTITIES if e.tamga_supported)
    priv_pct = priv_supported * 100 / len(PRIVACY_ENTITIES)

    privacy_details = [
        {"entity": e.entity, "regulation": e.regulation, "supported": e.tamga_supported, "scanner": e.scanner, "notes": e.notes}
        for e in PRIVACY_ENTITIES
    ]

    overall = round((owasp_pct * 0.50) + (eu_pct * 0.25) + (priv_pct * 0.25), 1)

    critical_gaps = (
        [f"{r.id}: {r.title}" for r in OWASP_LLM_TOP_10_2025 if r.tamga_coverage == "missing"]
        + [f"{c.article}: {c.requirement}" for c in EU_AI_ACT_CHECKS if c.tamga_status == "not_implemented"]
        + [f"MISSING: {e.entity} ({e.regulation})" for e in PRIVACY_ENTITIES if not e.tamga_supported]
    )

    return ComplianceReport(
        owasp_coverage_pct=round(owasp_pct, 1),
        owasp_details=owasp_details,
        eu_ai_act_checks=eu_checks,
        privacy_entity_coverage=privacy_details,
        overall_score=overall,
        critical_gaps=critical_gaps,
    )


# ---------------------------------------------------------------------------
# Scanner-to-OWASP mapping
# ---------------------------------------------------------------------------

_FINDING_TYPE_TO_OWASP: dict[str, list[str]] = {
    "pii": ["LLM02"],
    "secret": ["LLM02"],
    "injection": ["LLM01"],
    "jailbreak": ["LLM01"],
    "canary": ["LLM04"],
    "competitor": [],
    "toxicity": ["LLM05"],
    "code_leakage": ["LLM05"],
    "custom": ["LLM02"],
}


def map_findings_to_owasp(findings: list[dict]) -> dict[str, list[str]]:
    """Map scan findings to OWASP LLM Top 10 risk IDs."""
    owasp_map: dict[str, list[str]] = {}
    for f in findings:
        ftype = f.get("type", "")
        owasp_ids = _FINDING_TYPE_TO_OWASP.get(ftype, [])
        for oid in owasp_ids:
            owasp_map.setdefault(oid, []).append(f.get("category", ftype))
    return owasp_map
