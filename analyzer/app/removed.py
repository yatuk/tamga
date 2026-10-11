"""Settings and scan types the analyzer used to read and no longer does.

The toxicity scanner (LLM Guard's Toxicity, NoRefusal, BanCode and BanTopics
classifiers) was removed together with the ``llm-guard`` dependency. A
deployment that still sets its environment variable, or a caller that still
asks for its scan type, is told so in the log once per process, so that a
setting that does nothing is not mistaken for one that works.
"""

from __future__ import annotations

import os
from collections.abc import Iterable, Mapping

import structlog

logger = structlog.get_logger()

# Environment variable -> what it used to do.
REMOVED_ENV: dict[str, str] = {
    "TAMGA_BAN_TOPICS": "the topic list of the removed banned-topics classifier",
}

# Scan type -> what it used to run.
REMOVED_SCAN_TYPES: dict[str, str] = {
    "toxicity": "the removed LLM Guard classifiers (toxicity, refusal, banned code, banned topics)",
}

_env_reported = False
_scan_types_reported: set[str] = set()


def report_removed_env(environ: Mapping[str, str] | None = None) -> list[str]:
    """Log, once per process, each removed variable that is set.

    Returns the names found on this call (empty on every call after the
    first). The value is not logged.
    """
    global _env_reported
    if _env_reported:
        return []
    _env_reported = True

    env = os.environ if environ is None else environ
    found = [name for name in REMOVED_ENV if env.get(name, "").strip()]
    for name in found:
        logger.warning(
            "%s is set and is no longer read: it was %s. Remove it from the "
            "analyzer's environment.",
            name, REMOVED_ENV[name],
        )
    return found


def report_removed_scan_types(scan_types: Iterable[str]) -> list[str]:
    """Log, once per process and scan type, a request for a removed scan type.

    Returns the removed types reported on this call. The request is still
    answered: a removed type contributes no scanner result and no findings.
    """
    reported: list[str] = []
    for scan_type in scan_types:
        if scan_type in REMOVED_SCAN_TYPES and scan_type not in _scan_types_reported:
            _scan_types_reported.add(scan_type)
            reported.append(scan_type)
            logger.warning(
                "scan type %r was requested and no longer exists: it ran %s. "
                "Nothing is scanned for it and it returns no findings.",
                scan_type, REMOVED_SCAN_TYPES[scan_type],
            )
    return reported
