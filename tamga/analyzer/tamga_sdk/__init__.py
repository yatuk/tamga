"""Tamga Custom Scanner SDK — build your own AI security scanners.

Quick start::

    from tamga_sdk import BaseScanner, Finding, ScanResult, register

    class MyScanner(BaseScanner):
        name = "my_scanner"

        async def scan(self, content, config=None):
            # Your detection logic here
            if "sensitive" in content.lower():
                return ScanResult(
                    scanner=self.name,
                    findings=[Finding(
                        type="custom", category="my_rule",
                        severity="medium", match="sensitive", confidence=0.9
                    )],
                    duration_ms=1.5,
                )
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.5)

    register(MyScanner)
"""

from app.scanners.base import BaseScanner, Finding, ScanResult

from .discovery import discover_scanners, get_custom_scanners

__all__ = [
    "BaseScanner",
    "Finding",
    "ScanResult",
    "discover_scanners",
    "get_custom_scanners",
]
