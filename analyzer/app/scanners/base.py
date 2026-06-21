"""Base scanner abstract class — shared interface for all Tamga analyzers."""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any

from pydantic import BaseModel


class Finding(BaseModel):
    """A single security finding from a scanner."""
    type: str
    category: str
    severity: str  # critical, high, medium, low
    match: str
    confidence: float  # 0.0 – 1.0


class ScanResult(BaseModel):
    """Complete result from one scanner invocation."""
    scanner: str
    findings: list[Finding]
    duration_ms: float


class BaseScanner(ABC):
    """Abstract base class for all Tamga scanners.

    Every scanner must implement scan() and provide a unique scanner name
    via the `name` class attribute.
    """

    name: str = ""

    @abstractmethod
    async def scan(self, content: str, config: dict[str, Any] | None = None) -> ScanResult:
        """Scan content and return findings.

        Args:
            content: The text to scan (prompt or response).
            config: Optional per-invocation overrides (sensitivity, types, etc.).

        Returns:
            ScanResult with findings and timing.
        """
        ...
