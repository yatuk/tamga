"""Example custom scanner — detects negative sentiment in LLM output.

This is a minimal working example of the Tamga Custom Scanner SDK.
Copy this file to ``custom_scanners/`` (or set TAMGA_CUSTOM_SCANNERS_DIR)
and the scanner will be auto-discovered at startup.

Requirements: none (uses simple keyword matching for demo purposes).
For production: replace keyword matching with a real ML model (e.g. HuggingFace
transformers, or call an external API).
"""

from __future__ import annotations

from typing import Any

from app.scanners.base import BaseScanner, Finding, ScanResult

# Keywords that indicate negative/harmful sentiment in a business context.
NEGATIVE_KEYWORDS = [
    "lawsuit", "sue you", "legal action",
    "data breach", "security incident",
    "discriminate", "harassment",
    "illegal", "fraud", "scam",
]


class SentimentScanner(BaseScanner):
    """Detects negative or legally risky sentiment in LLM responses.

    This is a demo scanner — replace the keyword list with your own ML model
    or API call for production use.
    """

    name: str = "sentiment"

    async def scan(
        self, content: str, config: dict[str, Any] | None = None
    ) -> ScanResult:
        import time

        t0 = time.monotonic()

        if not content or not content.strip():
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.0)

        content_lower = content.lower()
        findings: list[Finding] = []

        for keyword in NEGATIVE_KEYWORDS:
            if keyword in content_lower:
                findings.append(Finding(
                    type="custom",
                    category="negative_sentiment",
                    severity="medium",
                    match=keyword,
                    confidence=0.75,
                ))

        duration = (time.monotonic() - t0) * 1000
        return ScanResult(
            scanner=self.name,
            findings=findings,
            duration_ms=round(duration, 2),
        )
