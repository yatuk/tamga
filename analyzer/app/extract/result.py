"""What an extraction returns."""

from __future__ import annotations

from dataclasses import dataclass, field

# The statuses are the categories of the proxy's attachment_unreadable
# finding, and "ok".
STATUS_OK = "ok"
STATUS_ENCRYPTED = "encrypted"
STATUS_UNSUPPORTED_TYPE = "unsupported_type"
STATUS_TOO_LARGE = "too_large"
STATUS_EXTRACT_FAILED = "extract_failed"
STATUS_OCR_UNAVAILABLE = "ocr_unavailable"


@dataclass
class Part:
    """The text of one page, sheet, slide or image."""

    kind: str
    index: int
    text: str
    name: str = ""
    ocr: bool = False


@dataclass
class Result:
    status: str
    parts: list[Part] = field(default_factory=list)
    # A limit (pages, characters) cut the text short.
    truncated: bool = False
    detected_type: str = ""
