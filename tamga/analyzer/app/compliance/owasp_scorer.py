"""OWASP LLM Top 10 (2025) risk definitions and coverage scoring."""

from pydantic import BaseModel


class OWASPRisk(BaseModel):
    id: str
    title: str
    description: str
    tamga_coverage: str  # "full", "partial", "missing"
    mitigating_scanners: list[str] = []
    recommendations: list[str] = []


OWASP_LLM_TOP_10_2025: list[OWASPRisk] = [
    OWASPRisk(
        id="LLM01",
        title="Prompt Injection",
        description="Direct/indirect injection, jailbreaking, adversarial suffix, multimodal injection",
        tamga_coverage="full",
        mitigating_scanners=["injection_scanner", "jailbreak_scanner", "injection_llm"],
        recommendations=[
            "Enable both inline DFA + LLM-as-judge (hybrid routing)",
            "Add multimodal injection detection for image prompts",
            "Enable indirect injection scanning for RAG documents",
        ],
    ),
    OWASPRisk(
        id="LLM02",
        title="Sensitive Information Disclosure",
        description="PII, secrets, PHI, training data leakage through outputs",
        tamga_coverage="full",
        mitigating_scanners=["pii_scanner", "secret_scanner", "pii_deep", "custom_scanner"],
        recommendations=[
            "Enable PII REDACT action for all PII types",
            "Add PHI/HIPAA entity types (medical record numbers, NPI)",
            "Enable output scanning with PII re-check on responses",
        ],
    ),
    OWASPRisk(
        id="LLM03",
        title="Supply Chain Vulnerabilities",
        description="Compromised models, LoRA adapters, dependency CVEs, poisoned datasets",
        tamga_coverage="missing",
        mitigating_scanners=[],
        recommendations=[
            "Pin model weights to content hash (SHA-256)",
            "Generate AI-BOM (CycloneDX ML-BOM format)",
            "Verify cryptographic signatures on model downloads",
            "Run dependency CVE scanning in CI (govulncheck, npm audit, pip-audit)",
            "Scan LoRA adapters before merge",
        ],
    ),
    OWASPRisk(
        id="LLM04",
        title="Data and Model Poisoning",
        description="Training/fine-tuning/RAG data poisoning, backdoor triggers, canary detection",
        tamga_coverage="partial",
        mitigating_scanners=["canary_scanner"],
        recommendations=[
            "Wire canary token injection into proxy handler",
            "Track provenance for all training examples",
            "Implement RAG document validation before ingestion",
            "Add embedding outlier detection",
            "Enable drift monitoring on model behavior",
        ],
    ),
    OWASPRisk(
        id="LLM05",
        title="Improper Output Handling",
        description="LLM output treated as trusted: RCE via code, XSS, SQLi, path traversal",
        tamga_coverage="partial",
        mitigating_scanners=["output_scan", "toxicity"],
        recommendations=[
            "Add HTML/JS/SQL injection patterns to output scanner",
            "Enforce CSP headers on all responses",
            "Use structured output mode (JSON Schema) where possible",
            "Sandbox any code execution from LLM output",
        ],
    ),
    OWASPRisk(
        id="LLM06",
        title="Excessive Agency",
        description="Overprivileged agent tools, unchecked autonomy, missing human-in-the-loop",
        tamga_coverage="missing",
        mitigating_scanners=[],
        recommendations=[
            "Implement agent action audit logging",
            "Add tool permission scoping (least privilege)",
            "Require human-in-the-loop for destructive actions",
            "Limit agent action chain depth",
            "Add per-tool rate limits and budgets",
        ],
    ),
    OWASPRisk(
        id="LLM07",
        title="System Prompt Leakage",
        description="Internal instructions, credentials, guardrails exposed through jailbreaking",
        tamga_coverage="missing",
        mitigating_scanners=[],
        recommendations=[
            "Never store secrets in system prompts",
            "Add system prompt leak detection scanner",
            "Compose prompts per-request from base policy",
            "Deploy output filter blocking responses that mirror system instructions",
            "Rotate system prompts after confirmed exposure",
        ],
    ),
    OWASPRisk(
        id="LLM08",
        title="Vector and Embedding Weaknesses",
        description="Cross-tenant leakage, adversarial retrieval, embedding inversion",
        tamga_coverage="missing",
        mitigating_scanners=[],
        recommendations=[
            "Implement per-tenant vector store namespaces",
            "Add retrieval-source validation",
            "Encrypt embeddings at rest (AES-256)",
            "Apply RBAC to vector database",
            "Monitor for anomalous retrieval patterns",
        ],
    ),
    OWASPRisk(
        id="LLM09",
        title="Misinformation",
        description="Hallucinations, fabricated citations, confident-but-wrong outputs",
        tamga_coverage="missing",
        mitigating_scanners=[],
        recommendations=[
            "Ground responses in retrieved, verifiable sources (RAG)",
            "Require structured citations for factual claims",
            "Add faithfulness/groundedness evaluation at inference",
            "Flag low-confidence outputs for human review",
        ],
    ),
    OWASPRisk(
        id="LLM10",
        title="Unbounded Consumption",
        description="Denial of Wallet, context flooding, infinite loops, model extraction",
        tamga_coverage="partial",
        mitigating_scanners=["rate_limiter", "body_limits"],
        recommendations=[
            "Enforce max_tokens_per_day (currently parsed but not enforced)",
            "Add per-session token quotas",
            "Implement loop detection (recursive agent calls)",
            "Add cost budget alerts (30%+ spike detection)",
        ],
    ),
]

