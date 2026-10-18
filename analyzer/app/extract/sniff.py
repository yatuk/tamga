"""What a file is, from its own bytes.

The declared MIME type and the file name are what the request says and
are not trusted: a file named report.pdf is read as whatever its signature
says it is. A ZIP container is looked at through its central directory
only, before any reader opens it, so that an encrypted entry or a ZIP bomb
is refused without a byte being unpacked.
"""

from __future__ import annotations

import io
import zipfile
from dataclasses import dataclass

from app.extract import limits as L
from app.extract.result import (
    STATUS_ENCRYPTED,
    STATUS_OK,
    STATUS_TOO_LARGE,
    STATUS_UNSUPPORTED_TYPE,
)

TYPE_PDF = "pdf"
TYPE_DOCX = "docx"
TYPE_XLSX = "xlsx"
TYPE_PPTX = "pptx"
TYPE_IMAGE = "image"
TYPE_ZIP = "zip"
TYPE_OFFICE_LEGACY = "office_legacy"
TYPE_UNKNOWN = "unknown"

_CFB = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"

_OOXML_MAIN = {
    "word/document.xml": TYPE_DOCX,
    "xl/workbook.xml": TYPE_XLSX,
    "ppt/presentation.xml": TYPE_PPTX,
}


@dataclass(frozen=True)
class Sniffed:
    type: str
    # STATUS_OK when a reader may open the file, or why it may not.
    status: str


def image_format(data: bytes) -> str:
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "png"
    if data.startswith(b"\xff\xd8\xff"):
        return "jpeg"
    if data.startswith((b"GIF87a", b"GIF89a")):
        return "gif"
    if len(data) >= 12 and data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return "webp"
    if data.startswith((b"II*\x00", b"MM\x00*")):
        return "tiff"
    if data.startswith(b"BM") and len(data) >= 26:
        return "bmp"
    return ""


def sniff(data: bytes) -> Sniffed:
    if b"%PDF-" in data[:1024]:
        return Sniffed(TYPE_PDF, STATUS_OK)
    if data.startswith((b"PK\x03\x04", b"PK\x05\x06")):
        return _sniff_zip(data)
    if data.startswith(_CFB):
        # A password-protected .docx, .xlsx or .pptx is a CFB container with
        # these two streams; the names are in its directory as UTF-16LE.
        if "EncryptedPackage".encode("utf-16-le") in data and "EncryptionInfo".encode("utf-16-le") in data:
            return Sniffed(TYPE_OFFICE_LEGACY, STATUS_ENCRYPTED)
        return Sniffed(TYPE_OFFICE_LEGACY, STATUS_UNSUPPORTED_TYPE)
    if image_format(data):
        return Sniffed(TYPE_IMAGE, STATUS_OK)
    return Sniffed(TYPE_UNKNOWN, STATUS_UNSUPPORTED_TYPE)


def _sniff_zip(data: bytes) -> Sniffed:
    try:
        zf = zipfile.ZipFile(io.BytesIO(data))
    except (zipfile.BadZipFile, ValueError, OSError):
        return Sniffed(TYPE_ZIP, STATUS_UNSUPPORTED_TYPE)
    with zf:
        infos = zf.infolist()
        if len(infos) > L.MAX_ENTRIES:
            return Sniffed(TYPE_ZIP, STATUS_TOO_LARGE)
        names = {i.filename for i in infos}
        kind = next((t for name, t in _OOXML_MAIN.items() if name in names), TYPE_ZIP)
        if any(i.flag_bits & 0x1 for i in infos):
            return Sniffed(kind, STATUS_ENCRYPTED)
        if unpacks_too_large(infos, len(data)):
            return Sniffed(kind, STATUS_TOO_LARGE)
    if kind == TYPE_ZIP:
        return Sniffed(kind, STATUS_UNSUPPORTED_TYPE)
    return Sniffed(kind, STATUS_OK)


def unpacks_too_large(infos: list[zipfile.ZipInfo], packed: int) -> bool:
    """Whether the entries, by the sizes their headers declare, would unpack
    to too much. zipfile does not read past the declared size of an entry,
    so the declared sizes are what a reader can be made to unpack."""
    total = 0
    for info in infos:
        total += max(info.file_size, 0)
        if total > L.MAX_UNPACKED_BYTES:
            return True
    return total > L.RATIO_FLOOR_BYTES and total > L.MAX_UNPACK_RATIO * max(packed, 1)
