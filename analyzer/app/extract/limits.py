"""The limits an extraction runs within.

The proxy sends its own (attachments.extract in the policy); a value of
zero takes the default, and every value is held to the hard bounds here
whatever the proxy asks for, so that one request cannot make the analyzer
read without end.
"""

from __future__ import annotations

from dataclasses import dataclass

DEFAULT_MAX_BYTES = 10 << 20
DEFAULT_MAX_PAGES = 200
DEFAULT_MAX_CHARS = 1_000_000
DEFAULT_TIMEOUT_S = 10.0

# The proxy's policy bounds are the same: attachments.extract.max_bytes up
# to 32 MiB, max_pages up to 2000, timeout_ms up to 60000.
HARD_MAX_BYTES = 32 << 20
HARD_MAX_PAGES = 2000
HARD_MAX_CHARS = 4_000_000
HARD_TIMEOUT_S = 60.0

# A ZIP container (DOCX, XLSX, PPTX) is refused as too large when its
# entries would unpack to more than MAX_UNPACKED_BYTES, or to more than
# MAX_UNPACK_RATIO times the file's size once past RATIO_FLOOR_BYTES: an
# ordinary office file unpacks to 2 to 20 times its size, a ZIP bomb to
# thousands. MAX_ENTRIES bounds the entries looked at.
MAX_UNPACKED_BYTES = 256 << 20
MAX_UNPACK_RATIO = 100
RATIO_FLOOR_BYTES = 20 << 20
MAX_ENTRIES = 10_000

# An image larger than this many pixels is not decoded: Pillow's own bound
# for a decompression bomb is near 179 million, and no page scan needs more
# than a small part of that.
MAX_IMAGE_PIXELS = 60_000_000


def _bounded(value: float, default: float, hard: float) -> float:
    if value is None or value <= 0:
        return default
    return min(value, hard)


@dataclass(frozen=True)
class Limits:
    max_bytes: int = DEFAULT_MAX_BYTES
    max_pages: int = DEFAULT_MAX_PAGES
    max_chars: int = DEFAULT_MAX_CHARS
    timeout_s: float = DEFAULT_TIMEOUT_S

    @classmethod
    def of(cls, max_bytes: int = 0, max_pages: int = 0, max_chars: int = 0, timeout_ms: int = 0) -> "Limits":
        """Limits from what a request asks for: zero is the default, and
        nothing goes past the hard bounds."""
        return cls(
            max_bytes=int(_bounded(max_bytes, DEFAULT_MAX_BYTES, HARD_MAX_BYTES)),
            max_pages=int(_bounded(max_pages, DEFAULT_MAX_PAGES, HARD_MAX_PAGES)),
            max_chars=int(_bounded(max_chars, DEFAULT_MAX_CHARS, HARD_MAX_CHARS)),
            timeout_s=_bounded(timeout_ms / 1000 if timeout_ms else 0, DEFAULT_TIMEOUT_S, HARD_TIMEOUT_S),
        )
