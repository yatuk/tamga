"""Errors raised by the OCR engines. No message contains text read from an image."""

from __future__ import annotations


class OcrError(Exception):
    """Base class of every OCR error."""


class OcrUnavailable(OcrError):
    """The engine is turned off or cannot run here (see ``available()``)."""


class OcrInputError(OcrError):
    """The image was refused before recognition.

    ``code`` is one of ``too_large`` (bytes), ``too_many_pixels``,
    ``unsupported_format``, ``undecodable`` or ``bad_lang``.
    """

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


class OcrTimeout(OcrError):
    """The call reached its deadline, waiting for the worker or recognising."""
