"""EU AI Act compliance checklist and risk classification."""

from dataclasses import dataclass
from enum import Enum


class EUAIRiskLevel(str, Enum):
    UNACCEPTABLE = "unacceptable"
    HIGH = "high"
    LIMITED = "limited"
    MINIMAL = "minimal"


@dataclass
class EUAIActCheck:
    article: str
    requirement: str
    applies: bool
    tamga_status: str  # "compliant", "partial", "not_implemented"
    notes: str = ""


EU_AI_ACT_CHECKS: list[EUAIActCheck] = [
    EUAIActCheck("Art. 5", "Prohibited AI practices (subliminal manipulation, social scoring)", True,
                  "partial", "Toxicity scanner blocks manipulation; social scoring not in scope"),
    EUAIActCheck("Art. 9", "Risk management system for high-risk AI", True,
                  "partial", "OWASP mapping exists; continuous risk monitoring needed"),
    EUAIActCheck("Art. 10", "Data governance and provenance", True,
                  "not_implemented", "Supply chain / data poisoning coverage missing"),
    EUAIActCheck("Art. 11", "Technical documentation", True,
                  "partial", "Docs exist; AI-BOM generation not implemented"),
    EUAIActCheck("Art. 12", "Record-keeping (logging)", True,
                  "compliant", "Audit logging with hash-chain verification in place"),
    EUAIActCheck("Art. 13", "Transparency and provision of information", True,
                  "partial", "Risk scores exposed; model cards not yet generated"),
    EUAIActCheck("Art. 14", "Human oversight", True,
                  "not_implemented", "Human-in-the-loop for critical actions not implemented"),
    EUAIActCheck("Art. 15", "Accuracy, robustness, cybersecurity", True,
                  "partial", "Benchmark scores available; continuous red-teaming needed"),
    EUAIActCheck("Art. 26", "Obligations of deployers", True,
                  "partial", "Deploy runbook exist; incident response plan partial"),
    EUAIActCheck("Art. 52", "Transparency for certain AI systems", True,
                  "compliant", "X-Tamga-Risk header on responses"),
]

