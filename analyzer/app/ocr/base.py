"""The OCR engine interface, its result types, limits and errors.

An engine turns the bytes of one image into text. The text read from an image
is as sensitive as the image: nothing in this package logs it, and the result
types leave it out of their repr so that a stray log line or a traceback does
not print it either.

Every engine runs its recognition on one worker thread of its own, so that a
call can give up at its deadline (``OcrTimeout``) instead of holding the
caller. A recognition that overruns keeps its worker until it ends; calls made
meanwhile wait for the worker until their own deadline. Tesseract runs as a
subprocess and is killed at the deadline; RapidOCR runs in the process and
cannot be interrupted, which is why the worker is never shared by two
recognitions at once.
"""

from __future__ import annotations

import logging
import math
import os
import statistics
import threading
import time
from abc import ABC, abstractmethod
from collections.abc import Callable, Iterable, Mapping
from concurrent.futures import ThreadPoolExecutor
from concurrent.futures import TimeoutError as FutureTimeout
from dataclasses import dataclass, field
from typing import Any, TypeVar

from .decode import PILLOW_WARN_PIXELS, decode_image
from .errors import OcrError, OcrInputError, OcrTimeout, OcrUnavailable

logger = logging.getLogger("tamga.ocr")

T = TypeVar("T")

DEFAULT_LANG = "tur+eng"


# ── Limits ──────────────────────────────────────────────────────────────────


def _env_int(env: Mapping[str, str], name: str, default: int, lo: int, hi: int) -> int:
    raw = env.get(name, "").strip()
    if not raw:
        return default
    try:
        value = int(raw)
    except ValueError:
        logger.warning("%s=%r is not an integer; using %d", name, raw, default)
        return default
    return max(lo, min(value, hi))


def _env_float(env: Mapping[str, str], name: str, default: float, lo: float, hi: float) -> float:
    raw = env.get(name, "").strip()
    if not raw:
        return default
    try:
        value = float(raw)
    except ValueError:
        logger.warning("%s=%r is not a number; using %s", name, raw, default)
        return default
    return max(lo, min(value, hi))


@dataclass(frozen=True)
class OcrLimits:
    """What one call may cost. Each has an environment variable."""

    # TAMGA_OCR_MAX_BYTES: the encoded image, before decoding.
    max_bytes: int = 20 * 1024 * 1024
    # TAMGA_OCR_MAX_PIXELS: width x height, checked from the header before
    # the pixels are decoded. 25 million is a 5000 x 5000 image; a 4K
    # screenshot is 8.3 million and a 12-megapixel phone photo 12 million.
    max_pixels: int = 25_000_000
    # TAMGA_OCR_MAX_IMAGES: images in one read_many call; the rest are skipped.
    max_images: int = 16
    # TAMGA_OCR_TIMEOUT_SECONDS: one read, or one whole read_many.
    timeout_s: float = 30.0
    # TAMGA_OCR_THREADS: CPU threads for recognition. The Compose file gives
    # the analyzer two CPUs, hence the default.
    threads: int = 2
    # TAMGA_OCR_MAX_CHARS: text kept per image; the result says truncated.
    max_chars: int = 200_000

    @classmethod
    def from_env(cls, env: Mapping[str, str] | None = None) -> "OcrLimits":
        env = os.environ if env is None else env
        d = cls()
        return cls(
            max_bytes=_env_int(env, "TAMGA_OCR_MAX_BYTES", d.max_bytes, 1024, 256 * 1024 * 1024),
            max_pixels=_env_int(env, "TAMGA_OCR_MAX_PIXELS", d.max_pixels, 10_000, PILLOW_WARN_PIXELS),
            max_images=_env_int(env, "TAMGA_OCR_MAX_IMAGES", d.max_images, 1, 1000),
            timeout_s=_env_float(env, "TAMGA_OCR_TIMEOUT_SECONDS", d.timeout_s, 0.1, 3600.0),
            threads=_env_int(env, "TAMGA_OCR_THREADS", d.threads, 1, 64),
            max_chars=_env_int(env, "TAMGA_OCR_MAX_CHARS", d.max_chars, 100, 10_000_000),
        )


# ── Results ─────────────────────────────────────────────────────────────────

Point = tuple[int, int]


@dataclass(frozen=True)
class OcrBox:
    """One piece of text the engine found, with its corners in image pixels."""

    text: str = field(repr=False)
    confidence: float
    points: tuple[Point, ...]


@dataclass(frozen=True)
class OcrResult:
    """The text of one image.

    ``confidence`` is the engine's own score in [0, 1], averaged over the
    characters read; 0.0 when nothing was read. ``truncated`` is true when the
    text was cut at ``max_chars``. ``boxes`` is filled only when asked for.
    """

    text: str = field(repr=False)
    confidence: float
    duration_ms: float
    truncated: bool = False
    boxes: tuple[OcrBox, ...] | None = field(default=None, repr=False)
    engine: str = ""
    width: int = 0
    height: int = 0


@dataclass(frozen=True)
class OcrBatch:
    """The results of read_many, in input order, for the images that were read.

    ``failed`` holds the input index and error code of each image that was
    refused or failed; ``skipped`` counts the images not attempted because of
    ``max_images`` or the deadline, which also sets ``truncated``.
    """

    results: tuple[tuple[int, OcrResult], ...]
    failed: tuple[tuple[int, str], ...] = ()
    skipped: int = 0
    truncated: bool = False
    duration_ms: float = 0.0


# What an engine's recognition step returns: text, confidence, corner points.
Fragment = tuple[str, float, tuple[Point, ...]]


# ── The engine interface ────────────────────────────────────────────────────


class OcrEngine(ABC):
    """Base class: limits, decoding, deadline, text assembly."""

    name: str = "base"

    def __init__(self, limits: OcrLimits | None = None) -> None:
        self.limits = limits or OcrLimits.from_env()
        self._slot = threading.Semaphore(1)
        self._pool: ThreadPoolExecutor | None = None
        self._pool_lock = threading.Lock()

    # Subclasses implement these two.

    @abstractmethod
    def available(self) -> tuple[bool, str]:
        """Whether read() can work here, and if not, why (never raises)."""

    @abstractmethod
    def _recognize(self, image: Any, lang: str, timeout_s: float) -> list[Fragment]:
        """Recognise a decoded RGB PIL image; called on the worker thread."""

    # Public API.

    def read(self, image_bytes: bytes, lang: str = DEFAULT_LANG, *, boxes: bool = False) -> OcrResult:
        """Read the text of one encoded image.

        Raises OcrUnavailable, OcrInputError or OcrTimeout.
        """
        self._require_available()
        deadline = time.monotonic() + self.limits.timeout_s
        return self._read_one(image_bytes, lang, boxes, deadline)

    def read_many(
        self, images: Iterable[bytes], lang: str = DEFAULT_LANG, *, boxes: bool = False
    ) -> OcrBatch:
        """Read up to max_images images within one timeout.

        An image that is refused or fails is recorded in ``failed`` and the
        rest are still read; the deadline or the image limit stops the call
        and the remaining images are counted in ``skipped``.
        """
        self._require_available()
        start = time.monotonic()
        deadline = start + self.limits.timeout_s
        images = list(images)
        results: list[tuple[int, OcrResult]] = []
        failed: list[tuple[int, str]] = []
        skipped = max(0, len(images) - self.limits.max_images)
        todo = images[: self.limits.max_images]
        for index, data in enumerate(todo):
            if time.monotonic() >= deadline:
                skipped += len(todo) - index
                break
            try:
                results.append((index, self._read_one(data, lang, boxes, deadline)))
            except OcrInputError as exc:
                failed.append((index, exc.code))
            except OcrTimeout:
                skipped += len(todo) - index
                break
            except OcrError:
                failed.append((index, "engine_error"))
        return OcrBatch(
            results=tuple(results),
            failed=tuple(failed),
            skipped=skipped,
            truncated=skipped > 0,
            duration_ms=(time.monotonic() - start) * 1000.0,
        )

    # Internals.

    def _require_available(self) -> None:
        ok, reason = self.available()
        if not ok:
            raise OcrUnavailable(reason)

    def _read_one(self, image_bytes: bytes, lang: str, boxes: bool, deadline: float) -> OcrResult:
        lang = check_lang(lang)
        if not isinstance(image_bytes, (bytes, bytearray, memoryview)):
            raise OcrInputError("undecodable", "the image must be bytes")
        if len(image_bytes) > self.limits.max_bytes:
            raise OcrInputError(
                "too_large", f"the image is {len(image_bytes)} bytes; the limit is {self.limits.max_bytes}"
            )

        start = time.monotonic()

        def job() -> tuple[list[Fragment], int, int]:
            image = decode_image(bytes(image_bytes), self.limits.max_pixels)
            width, height = image.size
            remaining = max(0.1, deadline - time.monotonic())
            return self._recognize(image, lang, remaining), width, height

        fragments, width, height = self._run(job, deadline)
        text, confidence = assemble(fragments)
        truncated = len(text) > self.limits.max_chars
        if truncated:
            text = text[: self.limits.max_chars]
        return OcrResult(
            text=text,
            confidence=confidence,
            duration_ms=(time.monotonic() - start) * 1000.0,
            truncated=truncated,
            boxes=tuple(OcrBox(t, c, p) for t, c, p in fragments) if boxes else None,
            engine=self.name,
            width=width,
            height=height,
        )

    def _executor(self) -> ThreadPoolExecutor:
        with self._pool_lock:
            if self._pool is None:
                self._pool = ThreadPoolExecutor(max_workers=1, thread_name_prefix=f"ocr-{self.name}")
            return self._pool

    def _run(self, fn: Callable[[], T], deadline: float) -> T:
        """Run fn on the engine's worker, giving up at the deadline."""
        remaining = deadline - time.monotonic()
        if remaining <= 0 or not self._slot.acquire(timeout=remaining):
            raise OcrTimeout("the OCR worker is still busy with an earlier image")

        def guarded() -> T:
            try:
                return fn()
            finally:
                self._slot.release()

        try:
            future = self._executor().submit(guarded)
        except BaseException:
            self._slot.release()
            raise
        try:
            return future.result(timeout=max(0.0, deadline - time.monotonic()))
        except FutureTimeout:
            raise OcrTimeout(f"OCR did not finish within {self.limits.timeout_s:g} s") from None


# ── Helpers shared by the engines ───────────────────────────────────────────

_LANG_PART_CHARS = set("abcdefghijklmnopqrstuvwxyz_")


def check_lang(lang: str) -> str:
    """Accept Tesseract-style language lists such as ``tur+eng``."""
    if not isinstance(lang, str) or not lang or len(lang) > 64:
        raise OcrInputError("bad_lang", "lang must be a list such as 'tur+eng'")
    parts = lang.split("+")
    for part in parts:
        if not (3 <= len(part) <= 16) or not set(part) <= _LANG_PART_CHARS:
            raise OcrInputError("bad_lang", "lang must be a list such as 'tur+eng'")
    return lang


def _skew(fragments: list[Fragment]) -> float:
    """The median slope, in radians, of the top edges of the wide fragments.

    A photo of a screen is rarely level; RapidOCR's boxes follow the text, so
    their top edges give the angle. Tesseract's boxes are level and give 0.
    """
    angles = []
    for _text, _conf, points in fragments:
        if len(points) != 4:
            continue
        (x0, y0), (x1, y1) = points[0], points[1]
        height = math.dist(points[0], points[3])
        if x1 - x0 > 2 * max(1.0, height):
            angles.append(math.atan2(y1 - y0, x1 - x0))
    if not angles:
        return 0.0
    angle = statistics.median(angles)
    return angle if 0.005 < abs(angle) < 0.5 else 0.0


def assemble(fragments: list[Fragment]) -> tuple[str, float]:
    """Join fragments into lines, top to bottom and left to right.

    The boxes are first turned level by the median skew of the text. A
    fragment then joins the current line when its vertical extent overlaps
    the line's mean extent by at least half the smaller height. Fragments of
    a line are joined with a space and lines with a newline, so that a table
    row reads as one line. Returns the text and the character-weighted mean
    confidence.
    """
    angle = _skew(fragments)
    cos_a, sin_a = math.cos(angle), math.sin(angle)
    items = []
    for text, conf, points in fragments:
        text = text.strip()
        if not text:
            continue
        if points:
            xs = [x * cos_a + y * sin_a for x, y in points]
            ys = [y * cos_a - x * sin_a for x, y in points]
            box = (min(xs), min(ys), max(xs), max(ys))
        else:
            box = (0.0, float(len(items)), 0.0, float(len(items) + 1))
        items.append((box, text, conf))
    if not items:
        return "", 0.0

    items.sort(key=lambda it: ((it[0][1] + it[0][3]) / 2.0, it[0][0]))
    lines: list[list[tuple[tuple[float, float, float, float], str, float]]] = []
    for item in items:
        top, bottom = item[0][1], item[0][3]
        if lines:
            line = lines[-1]
            line_top = sum(it[0][1] for it in line) / len(line)
            line_bottom = sum(it[0][3] for it in line) / len(line)
            overlap = min(bottom, line_bottom) - max(top, line_top)
            smaller = max(1.0, min(bottom - top, line_bottom - line_top))
            if overlap >= 0.5 * smaller:
                line.append(item)
                continue
        lines.append([item])

    out_lines = []
    weighted = 0.0
    chars = 0
    for line in lines:
        line.sort(key=lambda it: it[0][0])
        out_lines.append(" ".join(it[1] for it in line))
        for _, text, conf in line:
            weighted += max(0.0, min(1.0, conf)) * len(text)
            chars += len(text)
    return "\n".join(out_lines), (weighted / chars if chars else 0.0)
