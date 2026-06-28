"""LLM-as-judge prompt injection detector using Claude Haiku.

Prompt template is loaded from prompts/injection_judge.j2 (Jinja2).
"""

import asyncio
import json
import re
import time
from pathlib import Path
from typing import Any

import structlog
from jinja2 import Environment, FileSystemLoader

from app.scanners.base import BaseScanner, Finding, ScanResult

logger = structlog.get_logger()

# Jinja2 template loader — resolves prompts/ relative to the analyzer package root.
_PROMPTS_DIR = Path(__file__).resolve().parents[2] / "prompts"
_JINJA_ENV = Environment(loader=FileSystemLoader(str(_PROMPTS_DIR)), autoescape=False)
_JUDGE_TEMPLATE = _JINJA_ENV.get_template("injection_judge.j2")

_JSON_RE = re.compile(r"\{.*\}", re.DOTALL)


def _severity_from_confidence(conf: float) -> str:
    if conf >= 0.85:
        return "critical"
    if conf >= 0.65:
        return "high"
    if conf >= 0.40:
        return "medium"
    return "low"


def _parse_judge_response(text: str) -> dict[str, Any] | None:
    m = _JSON_RE.search(text)
    if not m:
        return None
    try:
        return json.loads(m.group())
    except json.JSONDecodeError:
        return None


class LLMInjectionScanner(BaseScanner):
    """Uses Claude Haiku for semantic prompt injection classification.

    Falls back to mock mode (empty findings) when ANTHROPIC_API_KEY is absent
    or the API call fails — never blocks proxy traffic.
    """

    name: str = "injection_llm"
    _MODEL = "claude-haiku-4-5-20251001"
    _MAX_TOKENS = 256
    _TIMEOUT = 8.0  # seconds
    _RETRY_ATTEMPTS = 2

    def __init__(self, api_key: str = ""):
        self._enabled = bool(api_key)
        self._client = None
        if self._enabled:
            try:
                import anthropic  # local import so tests work without the package
                self._client = anthropic.AsyncAnthropic(api_key=api_key)
            except ImportError:
                logger.warning("injection_llm: anthropic package not found — mock mode")
                self._enabled = False

    async def _call_judge_once(self, prompt: str) -> dict[str, Any] | None:
        """Single attempt to call Claude Haiku. Returns parsed JSON or None."""
        try:
            response = await asyncio.wait_for(
                self._client.messages.create(
                    model=self._MODEL,
                    max_tokens=self._MAX_TOKENS,
                    messages=[{"role": "user", "content": prompt}],
                ),
                timeout=self._TIMEOUT,
            )
        except asyncio.TimeoutError:
            raise
        raw = response.content[0].text if response.content else ""
        return _parse_judge_response(raw)

    async def _call_judge_with_retry(self, prompt: str) -> dict[str, Any] | None:
        """Call Claude Haiku with exponential backoff retry."""
        last_exc = None
        for attempt in range(1 + self._RETRY_ATTEMPTS):
            try:
                return await self._call_judge_once(prompt)
            except asyncio.TimeoutError:
                last_exc = asyncio.TimeoutError("injection_llm judge timed out")
                if attempt < self._RETRY_ATTEMPTS:
                    delay = 2.0 * (2 ** attempt)
                    logger.debug("injection_llm: retry %d/%d after %.1fs",
                                 attempt + 1, self._RETRY_ATTEMPTS, delay)
                    await asyncio.sleep(delay)
            except Exception as exc:
                last_exc = exc
                if attempt < self._RETRY_ATTEMPTS:
                    delay = 1.0 * (2 ** attempt)
                    logger.debug("injection_llm: retry %d/%d after %.1fs: %s",
                                 attempt + 1, self._RETRY_ATTEMPTS, delay, exc)
                    await asyncio.sleep(delay)

        logger.warning("injection_llm: all retries exhausted (fail-open): %s", last_exc)
        return None

    async def scan(self, content: str, config: dict[str, Any] | None = None) -> ScanResult:
        t0 = time.monotonic()

        if not self._enabled or self._client is None:
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.0)

        prompt = _JUDGE_TEMPLATE.render(content=content[:2000])
        parsed = await self._call_judge_with_retry(prompt)

        duration = (time.monotonic() - t0) * 1000

        if parsed is None:
            return ScanResult(scanner=self.name, findings=[], duration_ms=round(duration, 2))

        if not parsed.get("is_injection"):
            return ScanResult(scanner=self.name, findings=[], duration_ms=round(duration, 2))

        conf = float(parsed.get("confidence", 0.5))
        technique = str(parsed.get("technique", "unknown"))
        reason = str(parsed.get("reason", ""))

        findings = [Finding(
            type="injection",
            category=technique,
            severity=_severity_from_confidence(conf),
            match=reason[:200],
            confidence=conf,
        )]
        return ScanResult(scanner=self.name, findings=findings, duration_ms=round(duration, 2))
