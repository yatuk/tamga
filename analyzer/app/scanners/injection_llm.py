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
from app.scanners.egress import content_is_blocked, record_judge_call

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


# The judge sends request text to a model provider outside this deployment.
# That is an egress of prompt content, so it happens only when the operator
# asked for it by name: TAMGA_LLM_JUDGE=anthropic, together with a key.
JUDGE_ANTHROPIC = "anthropic"

# How much of a text the judge is sent.
JUDGE_MAX_CHARS = 2000

# What an ANTHROPIC_API_KEY holds when nobody put a key there: the text of
# an example file. A key is in the analyzer's environment in many
# deployments for other reasons (one .env for every service), so its
# presence says nothing; a value like these says it is not a key at all.
# The marks are long enough that a real key, which is random letters and
# digits, does not hold one by chance.
_PLACEHOLDER_MARKS = (
    "replace_me", "replace-me", "replaceme", "your_", "your-", "yourkey",
    "changeme", "change_me", "change-me", "example", "placeholder",
    "dummy", "xxxxx", "<", ">", "...",
)

def looks_like_placeholder(api_key: str) -> bool:
    """True for a value that is plainly not an API key."""
    key = api_key.strip()
    if len(key) < 20 or any(c.isspace() for c in key):
        return True
    lowered = key.lower()
    return any(mark in lowered for mark in _PLACEHOLDER_MARKS)


class LLMInjectionScanner(BaseScanner):
    """Uses Claude Haiku for semantic prompt injection classification.

    Off unless turned on by name (``judge="anthropic"``, from
    TAMGA_LLM_JUDGE) with a real key: a key alone does not turn it on. When
    off, or when the API call fails, it returns no findings — it never
    blocks proxy traffic.
    """

    name: str = "injection_llm"
    _MODEL = "claude-haiku-4-5-20251001"
    _MAX_TOKENS = 256
    _TIMEOUT = 8.0  # seconds
    _RETRY_ATTEMPTS = 2

    def __init__(self, api_key: str = "", judge: str = ""):
        self._enabled = False
        self._client = None
        self.disabled_reason = ""

        judge = (judge or "").strip().lower()
        if judge in ("", "off", "none", "false", "0"):
            self.disabled_reason = "TAMGA_LLM_JUDGE is not set"
            if api_key:
                logger.info(
                    "injection_llm: an Anthropic key is present and the judge is off; "
                    "set TAMGA_LLM_JUDGE=anthropic to send request text to Anthropic"
                )
            return
        if judge != JUDGE_ANTHROPIC:
            self.disabled_reason = "TAMGA_LLM_JUDGE names an unknown provider"
            logger.warning("injection_llm: unknown TAMGA_LLM_JUDGE value — judge off")
            return
        if not api_key:
            self.disabled_reason = "ANTHROPIC_API_KEY is empty"
            logger.warning("injection_llm: TAMGA_LLM_JUDGE=anthropic without ANTHROPIC_API_KEY — judge off")
            return
        if looks_like_placeholder(api_key):
            self.disabled_reason = "ANTHROPIC_API_KEY is a placeholder"
            logger.warning("injection_llm: ANTHROPIC_API_KEY looks like a placeholder — judge off")
            return

        try:
            import anthropic  # local import so tests work without the package
            self._client = anthropic.AsyncAnthropic(api_key=api_key)
        except ImportError:
            self.disabled_reason = "the anthropic package is not installed"
            logger.warning("injection_llm: anthropic package not found — judge off")
            return
        self._enabled = True
        logger.warning(
            "injection_llm: judge ON — up to %d characters of a request the proxy "
            "forwards are sent to Anthropic (%s) when the proxy asks for an injection scan",
            JUDGE_MAX_CHARS, self._MODEL,
        )

    @property
    def enabled(self) -> bool:
        return bool(self._enabled and self._client is not None)

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
        # The servers do not ask for a blocked request's text to be judged;
        # a caller that passes the request's metadata gets the same answer.
        if content_is_blocked(config):
            return ScanResult(scanner=self.name, findings=[], duration_ms=0.0)

        sent = content[:JUDGE_MAX_CHARS]
        prompt = _JUDGE_TEMPLATE.render(content=sent)
        # Request text leaves for the provider here. Said in the log each
        # time, by size and never by content, and counted for /health.
        record_judge_call(len(sent))
        logger.info(
            "injection_llm: sending %d characters of request text to Anthropic (%s)",
            len(sent), self._MODEL,
        )
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
