"""Decoding an untrusted image for OCR.

The bytes come from a request body, so decoding is the part an attacker
controls. What is done about it:

- the format is decided from the first bytes, and only PNG, JPEG, WebP, GIF
  and BMP are accepted; Pillow is then allowed that one format and no other,
  so a file cannot reach a decoder by claiming to be something else (TIFF,
  PSD, EPS, ICO and the rest are refused; EPS would run Ghostscript);
- the pixel count is checked from the header, before any pixel is decoded,
  against TAMGA_OCR_MAX_PIXELS; Pillow's own decompression-bomb warning is
  turned into a refusal, and its hard limit stays in place behind it;
- only the first frame of an animated GIF or WebP is read;
- the EXIF orientation is applied, so a phone photo is read upright;
- the result is always an RGB image, with transparency flattened onto white.

Any failure inside Pillow becomes ``OcrInputError("undecodable")``, without
Pillow's message, which can quote file content.
"""

from __future__ import annotations

import io
import warnings
from typing import Any

from .errors import OcrInputError, OcrUnavailable

SUPPORTED_FORMATS = ("PNG", "JPEG", "WEBP", "GIF", "BMP")

# Pillow warns above this many pixels (its MAX_IMAGE_PIXELS default) and
# refuses above twice as many. TAMGA_OCR_MAX_PIXELS cannot be set higher.
PILLOW_WARN_PIXELS = 89_478_485


def sniff_format(data: bytes) -> str | None:
    """The format named by the file's signature, if it is one we accept."""
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "PNG"
    if data.startswith(b"\xff\xd8\xff"):
        return "JPEG"
    if data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return "WEBP"
    if data[:6] in (b"GIF87a", b"GIF89a"):
        return "GIF"
    if data[:2] == b"BM":
        return "BMP"
    return None


def decode_image(data: bytes, max_pixels: int) -> Any:
    """Decode ``data`` into an RGB ``PIL.Image.Image``, or raise OcrInputError."""
    if not data:
        raise OcrInputError("undecodable", "the image is empty")
    fmt = sniff_format(data)
    if fmt is None:
        raise OcrInputError(
            "unsupported_format", f"the image is not one of {', '.join(SUPPORTED_FORMATS)}"
        )
    try:
        from PIL import Image, ImageOps
    except ImportError:
        raise OcrUnavailable("Pillow is not installed") from None
    max_pixels = min(max_pixels, PILLOW_WARN_PIXELS)

    with warnings.catch_warnings():
        warnings.simplefilter("error", Image.DecompressionBombWarning)
        try:
            image = Image.open(io.BytesIO(data), formats=[fmt])
        except (Image.DecompressionBombError, Image.DecompressionBombWarning):
            raise OcrInputError("too_many_pixels", "the image has too many pixels") from None
        except Exception:
            raise OcrInputError("undecodable", f"the {fmt} image cannot be decoded") from None

        width, height = image.size
        if width <= 0 or height <= 0:
            raise OcrInputError("undecodable", f"the {fmt} image has no pixels")
        if width * height > max_pixels:
            raise OcrInputError(
                "too_many_pixels",
                f"the image is {width}x{height} pixels; the limit is {max_pixels} pixels",
            )

        try:
            if getattr(image, "n_frames", 1) > 1:
                image.seek(0)
            image.load()
            image = _upright(image, ImageOps)
            return _to_rgb(image, Image)
        except (Image.DecompressionBombError, Image.DecompressionBombWarning):
            raise OcrInputError("too_many_pixels", "the image has too many pixels") from None
        except OcrInputError:
            raise
        except Exception:
            raise OcrInputError("undecodable", f"the {fmt} image cannot be decoded") from None


def _upright(image: Any, image_ops: Any) -> Any:
    """Apply the EXIF orientation; a broken EXIF block leaves the image as it is."""
    try:
        return image_ops.exif_transpose(image) or image
    except Exception:
        return image


def _to_rgb(image: Any, image_module: Any) -> Any:
    if image.mode == "RGB":
        return image
    if image.mode in ("I", "I;16", "I;16B", "I;16L", "F"):
        image = image.convert("L")
    if image.mode == "P":
        image = image.convert("RGBA")
    if image.mode in ("RGBA", "LA", "PA"):
        rgba = image.convert("RGBA")
        background = image_module.new("RGB", rgba.size, (255, 255, 255))
        background.paste(rgba, mask=rgba.getchannel("A"))
        return background
    return image.convert("RGB")
