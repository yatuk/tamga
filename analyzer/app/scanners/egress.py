"""What leaves the analyzer for an outside service, and when it may not.

One scanner sends request text out of the deployment: the LLM judge
(injection_llm), to a model provider. This module holds what both servers
(gRPC and HTTP) need to keep that in hand, without importing the scanner.
"""

from __future__ import annotations

from typing import Any

# Counted for /health: how often request text left for a provider, and how
# many characters of it.
_judge_calls = 0
_judge_chars = 0


def record_judge_call(characters: int) -> None:
    """Count one outbound call of the judge."""
    global _judge_calls, _judge_chars
    _judge_calls += 1
    _judge_chars += characters


def judge_stats() -> dict[str, int]:
    """How many times the judge sent text to its provider, and how much."""
    return {"calls": _judge_calls, "characters": _judge_chars}


def content_is_blocked(metadata: Any) -> bool:
    """True when the proxy says this is the text of a request it blocked.

    The proxy does not send such text. An older proxy did, with
    metadata["action"] == "BLOCK". Either way the judge is not called for
    it: text that was refused to one provider is not sent to another.
    """
    if not metadata:
        return False
    try:
        action = str(metadata.get("action", "")).strip().upper()
        content = str(metadata.get("content", "")).strip().lower()
    except AttributeError:
        return False
    return action == "BLOCK" or content == "blocked"
