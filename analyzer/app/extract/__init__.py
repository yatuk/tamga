"""Text extraction from attached files, for the proxy to scan.

The proxy sends a file a request carries (a PDF, a Word, Excel or
PowerPoint file, an image) to the Extract RPC, and scans the text that comes
back as it scans any other text. This package is that call's work:

- ``sniff`` tells what the file is from its own bytes, and refuses before
  anything is unpacked what is encrypted or would unpack to far more than it
  weighs;
- ``readers`` gets the text out, per page, sheet or slide, within the limits
  of the request;
- ``Extractor`` runs a reader on a bounded pool of threads with a deadline,
  and never lets an exception out.

Nothing here logs or returns any part of a file's content other than the
text it was asked for: errors are reported by status, and logged by the
exception's type.
"""

from __future__ import annotations

from app.extract.limits import Limits
from app.extract.result import (
    STATUS_ENCRYPTED,
    STATUS_EXTRACT_FAILED,
    STATUS_OCR_UNAVAILABLE,
    STATUS_OK,
    STATUS_TOO_LARGE,
    STATUS_UNSUPPORTED_TYPE,
    Part,
    Result,
)
from app.extract.service import Extractor, extract_bytes

__all__ = [
    "Extractor",
    "Limits",
    "Part",
    "Result",
    "STATUS_ENCRYPTED",
    "STATUS_EXTRACT_FAILED",
    "STATUS_OCR_UNAVAILABLE",
    "STATUS_OK",
    "STATUS_TOO_LARGE",
    "STATUS_UNSUPPORTED_TYPE",
    "extract_bytes",
]
