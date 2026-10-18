"""The OCR engine module (app/ocr).

Most tests need no engine: they use a stand-in engine whose recognition step
is scripted, and Pillow for the decoding tests (skipped without it). The
tests at the end read the sample images with a real engine and run only where
that engine is available (TAMGA_OCR_ENGINE does not have to be set).
"""

from __future__ import annotations

import io
import json
import logging
import os
import struct
import subprocess
import sys
import threading
import time
import zlib
from pathlib import Path

import pytest

from app.ocr import (
    OcrError,
    OcrInputError,
    OcrLimits,
    OcrResult,
    OcrTimeout,
    OcrUnavailable,
    engine_from_env,
    make_engine,
)
from app.ocr.base import OcrEngine, assemble, check_lang
from app.ocr.model_pins import RAPIDOCR_MODELS, RAPIDOCR_VERSION
from app.ocr.tess import lines_from_tsv

ANALYZER_ROOT = Path(__file__).resolve().parent.parent
SAMPLES = Path(__file__).resolve().parent / "ocr_samples"
SECRET = "TR33 0006 1005 1978 6457 8413 26"


def _png(size=(40, 20), mode="RGB", color=(255, 255, 255)) -> bytes:
    from PIL import Image

    buf = io.BytesIO()
    Image.new(mode, size, color).save(buf, "PNG")
    return buf.getvalue()


def _png_header_only(width: int, height: int) -> bytes:
    """A PNG whose header claims width x height, with one tiny data chunk."""

    def chunk(kind: bytes, data: bytes) -> bytes:
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

    ihdr = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(b"\x00" * 16)) + chunk(b"IEND", b"")


class ScriptedEngine(OcrEngine):
    """Returns fixed fragments; can be made slow."""

    name = "scripted"

    def __init__(self, fragments=None, delay=0.0, limits=None):
        super().__init__(limits or OcrLimits(timeout_s=5.0))
        self.fragments = fragments if fragments is not None else [(SECRET, 0.9, ((0, 0), (100, 0), (100, 10), (0, 10)))]
        self.delay = delay
        self.calls = 0
        self.sizes = []

    def available(self):
        return True, "scripted"

    def _recognize(self, image, lang, timeout_s):
        self.calls += 1
        self.sizes.append(image.size)
        assert image.mode == "RGB"
        if self.delay:
            time.sleep(self.delay)
        return list(self.fragments)


# ── Selection and import ────────────────────────────────────────────────────


def test_importing_the_package_loads_no_engine_dependency():
    code = (
        "import sys; import app.ocr; from app.ocr import get_engine; get_engine().available(); "
        "heavy = [m for m in ('rapidocr', 'onnxruntime', 'cv2', 'pytesseract', 'PIL') if m in sys.modules]; "
        "print(','.join(heavy))"
    )
    out = subprocess.run(
        [sys.executable, "-c", code], cwd=ANALYZER_ROOT, capture_output=True, text=True, check=True,
        env={k: v for k, v in os.environ.items() if not k.startswith("TAMGA_OCR_")},
    )
    assert out.stdout.strip() == ""


def test_off_is_the_default_and_refuses_to_read():
    engine = engine_from_env({})
    ok, reason = engine.available()
    assert not ok and "off" in reason
    with pytest.raises(OcrUnavailable):
        engine.read(b"\x89PNG\r\n\x1a\n")


@pytest.mark.parametrize("value", ["paddle", "RAPID", "on"])
def test_an_unknown_engine_name_is_unavailable_and_says_so(value):
    engine = engine_from_env({"TAMGA_OCR_ENGINE": value})
    ok, reason = engine.available()
    assert not ok and value.lower() in reason and "rapidocr" in reason


def test_engine_names_select_their_class():
    assert make_engine("rapidocr").name == "rapidocr"
    assert make_engine(" Tesseract ").name == "tesseract"
    assert make_engine("off").name == "off"


def test_tesseract_without_its_binary_is_unavailable(tmp_path):
    engine = make_engine("tesseract", cmd=str(tmp_path / "no-such-tesseract"))
    ok, reason = engine.available()
    assert not ok
    assert "not installed" in reason


def test_rapidocr_without_model_files_is_unavailable(tmp_path):
    engine = make_engine("rapidocr", model_dir=tmp_path)
    ok, reason = engine.available()
    assert not ok
    assert any(word in reason for word in ("not installed", "cannot be imported", "model"))


def test_rapidocr_refuses_a_model_file_that_does_not_match_its_pin(tmp_path):
    pytest.importorskip("rapidocr")
    pytest.importorskip("onnxruntime")
    for filename, _url, _sha, size in RAPIDOCR_MODELS.values():
        (tmp_path / filename).write_bytes(b"\x00" * size)
    ok, reason = make_engine("rapidocr", model_dir=tmp_path).available()
    assert not ok and "SHA-256" in reason


# ── Limits and arguments ────────────────────────────────────────────────────


def test_limits_from_env_parse_and_clamp(caplog):
    limits = OcrLimits.from_env(
        {
            "TAMGA_OCR_MAX_PIXELS": "1000000000",
            "TAMGA_OCR_MAX_IMAGES": "0",
            "TAMGA_OCR_TIMEOUT_SECONDS": "2.5",
            "TAMGA_OCR_THREADS": "four",
            "TAMGA_OCR_MAX_CHARS": "500",
        }
    )
    assert limits.max_pixels == 89_478_485  # Pillow's own warning threshold
    assert limits.max_images == 1
    assert limits.timeout_s == 2.5
    assert limits.threads == OcrLimits().threads
    assert limits.max_chars == 500
    assert OcrLimits.from_env({}) == OcrLimits()


@pytest.mark.parametrize("lang", ["tur+eng", "eng", "tur", "chi_sim+eng"])
def test_language_lists_are_accepted(lang):
    assert check_lang(lang) == lang


@pytest.mark.parametrize("lang", ["", "tur+", "tu", "tur;rm -rf", "--psm 0", "TUR", "tur eng", "a" * 70])
def test_anything_else_as_language_is_refused(lang):
    with pytest.raises(OcrInputError) as exc:
        check_lang(lang)
    assert exc.value.code == "bad_lang"


# ── Decoding ────────────────────────────────────────────────────────────────


def test_supported_images_decode_to_rgb():
    pytest.importorskip("PIL")
    from PIL import Image

    engine = ScriptedEngine()
    for mode, color in (("RGB", (1, 2, 3)), ("L", 9), ("RGBA", (0, 0, 0, 0)), ("P", 3), ("1", 1)):
        for fmt in ("PNG", "BMP", "GIF", "JPEG", "WEBP"):
            if fmt == "JPEG" and mode in ("RGBA", "P"):
                continue
            buf = io.BytesIO()
            try:
                Image.new(mode, (30, 12), color).save(buf, fmt)
            except (OSError, KeyError, ValueError):
                continue  # this Pillow cannot write the combination
            try:
                Image.open(io.BytesIO(buf.getvalue())).load()
            except Exception:
                continue  # this Pillow cannot read back what it wrote (Pillow 12.2, 8-bit BMP)
            result = engine.read(buf.getvalue())
            assert (result.width, result.height) == (30, 12)
    assert engine.calls > 10


def test_transparent_pixels_become_white():
    pytest.importorskip("PIL")
    from app.ocr.decode import decode_image

    image = decode_image(_png(mode="RGBA", color=(0, 0, 0, 0)), 10_000)
    assert image.mode == "RGB" and image.getpixel((0, 0)) == (255, 255, 255)


def test_the_exif_orientation_is_applied():
    pytest.importorskip("PIL")
    from PIL import Image

    from app.ocr.decode import decode_image

    exif = Image.Exif()
    exif[0x0112] = 6  # rotate 90 degrees clockwise to display
    buf = io.BytesIO()
    Image.new("RGB", (40, 10), (255, 255, 255)).save(buf, "JPEG", exif=exif)
    assert decode_image(buf.getvalue(), 10_000).size == (10, 40)


@pytest.mark.parametrize(
    "data",
    [
        b"II*\x00" + b"\x00" * 64,  # TIFF
        b"%!PS-Adobe-3.0 EPSF-3.0\n",  # EPS, which Pillow would hand to Ghostscript
        b"8BPS" + b"\x00" * 64,  # PSD
        b"\x00\x00\x01\x00" + b"\x00" * 64,  # ICO
        b"<svg xmlns='http://www.w3.org/2000/svg'/>",
        b"just some text",
    ],
)
def test_other_formats_are_refused_before_pillow_sees_them(data):
    with pytest.raises(OcrInputError) as exc:
        ScriptedEngine().read(data)
    assert exc.value.code == "unsupported_format"


def test_a_file_that_lies_about_its_format_is_not_decoded_as_another():
    pytest.importorskip("PIL")
    from PIL import Image

    buf = io.BytesIO()
    Image.new("RGB", (8, 8)).save(buf, "TIFF")
    data = b"\x89PNG\r\n\x1a\n" + buf.getvalue()  # a PNG signature in front of a TIFF
    with pytest.raises(OcrInputError) as exc:
        ScriptedEngine().read(data)
    assert exc.value.code == "undecodable"


def test_a_truncated_image_is_undecodable():
    pytest.importorskip("PIL")
    with pytest.raises(OcrInputError) as exc:
        ScriptedEngine().read(_png((200, 200), color=(10, 20, 30))[:60])
    assert exc.value.code == "undecodable"


def test_the_pixel_count_is_checked_from_the_header_before_decoding():
    pytest.importorskip("PIL")
    engine = ScriptedEngine(limits=OcrLimits(max_pixels=1_000_000))
    with pytest.raises(OcrInputError) as exc:
        engine.read(_png_header_only(2000, 1000))
    assert exc.value.code == "too_many_pixels"
    # A header far past Pillow's own limit is refused the same way.
    with pytest.raises(OcrInputError) as exc:
        engine.read(_png_header_only(60000, 60000))
    assert exc.value.code == "too_many_pixels"
    assert engine.calls == 0


def test_too_many_bytes_are_refused_before_decoding():
    engine = ScriptedEngine(limits=OcrLimits(max_bytes=1024))
    with pytest.raises(OcrInputError) as exc:
        engine.read(b"\x89PNG\r\n\x1a\n" + b"\x00" * 2000)
    assert exc.value.code == "too_large"


def test_an_animated_gif_is_read_from_its_first_frame():
    pytest.importorskip("PIL")
    from PIL import Image

    frames = [Image.new("L", (20, 10), v) for v in (0, 128, 255)]
    buf = io.BytesIO()
    frames[0].save(buf, "GIF", save_all=True, append_images=frames[1:])
    engine = ScriptedEngine()
    engine.read(buf.getvalue())
    assert engine.calls == 1 and engine.sizes == [(20, 10)]


# ── Results, deadline and batches ───────────────────────────────────────────


def test_a_result_and_its_boxes_do_not_show_the_text_in_their_repr():
    pytest.importorskip("PIL")
    result = ScriptedEngine().read(_png(), boxes=True)
    assert result.text == SECRET
    assert SECRET not in repr(result) and SECRET not in str(result)
    assert result.boxes and SECRET not in repr(result.boxes[0])
    assert result.boxes[0].text == SECRET
    assert 0.89 < result.confidence < 0.91
    assert result.engine == "scripted" and result.duration_ms >= 0


def test_boxes_are_left_out_unless_asked_for():
    pytest.importorskip("PIL")
    assert ScriptedEngine().read(_png()).boxes is None


def test_the_text_is_cut_at_max_chars():
    pytest.importorskip("PIL")
    engine = ScriptedEngine(fragments=[("x" * 500, 1.0, ())], limits=OcrLimits(max_chars=100))
    result = engine.read(_png())
    assert result.truncated and len(result.text) == 100


def test_a_slow_recognition_times_out_and_holds_the_worker_until_it_ends():
    pytest.importorskip("PIL")
    engine = ScriptedEngine(delay=0.6, limits=OcrLimits(timeout_s=0.15))
    start = time.monotonic()
    with pytest.raises(OcrTimeout):
        engine.read(_png())
    assert time.monotonic() - start < 0.5
    # The first recognition still has the worker: this call waits its own
    # timeout for it and gives up.
    with pytest.raises(OcrTimeout):
        engine.read(_png())
    time.sleep(0.6)
    engine.delay = 0.0
    assert engine.read(_png()).text == SECRET


def test_only_one_recognition_runs_at_a_time():
    pytest.importorskip("PIL")
    running = []
    peak = []
    lock = threading.Lock()

    class Counting(ScriptedEngine):
        def _recognize(self, image, lang, timeout_s):
            with lock:
                running.append(1)
                peak.append(len(running))
            time.sleep(0.05)
            with lock:
                running.pop()
            return []

    engine = Counting(limits=OcrLimits(timeout_s=10))
    threads = [threading.Thread(target=engine.read, args=(_png(),)) for _ in range(5)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert max(peak) == 1 and len(peak) == 5


def test_read_many_keeps_going_past_a_bad_image_and_stops_at_the_limit():
    pytest.importorskip("PIL")
    engine = ScriptedEngine(limits=OcrLimits(max_images=3))
    batch = engine.read_many([_png(), b"not an image", _png(), _png(), _png()])
    assert [i for i, _ in batch.results] == [0, 2]
    assert batch.failed == ((1, "unsupported_format"),)
    assert batch.skipped == 2 and batch.truncated


def test_read_many_stops_at_its_deadline():
    pytest.importorskip("PIL")
    engine = ScriptedEngine(delay=0.2, limits=OcrLimits(timeout_s=0.5, max_images=10))
    batch = engine.read_many([_png()] * 6)
    assert 1 <= len(batch.results) <= 3
    assert batch.skipped == 6 - len(batch.results) and batch.truncated
    time.sleep(0.3)  # let the worker finish before the next test


def test_the_text_never_reaches_the_log(caplog):
    pytest.importorskip("PIL")
    caplog.set_level(logging.DEBUG)
    engine = ScriptedEngine()
    engine.read(_png(), boxes=True)
    engine.read_many([_png(), b"junk"])
    with pytest.raises(OcrError):
        ScriptedEngine(delay=0.3, limits=OcrLimits(timeout_s=0.05)).read(_png())
    assert SECRET not in caplog.text
    assert compact(SECRET) not in compact(caplog.text)


def compact(text: str) -> str:
    return "".join(text.split())


# ── Assembling lines ────────────────────────────────────────────────────────


def _box(x, y, w=80, h=20):
    return ((x, y), (x + w, y), (x + w, y + h), (x, y + h))


def test_fragments_on_one_row_become_one_line_in_reading_order():
    text, conf = assemble(
        [
            ("TR33", 1.0, _box(200, 52)),
            ("10000000146", 0.5, _box(100, 50)),
            ("Ad Soyad", 1.0, _box(0, 0)),
            ("TC", 1.0, _box(100, 2)),
            ("Ayşe", 1.0, _box(0, 51)),
        ]
    )
    assert text == "Ad Soyad TC\nAyşe 10000000146 TR33"
    assert 0.0 < conf < 1.0


def test_a_tilted_table_still_reads_row_by_row():
    import math

    angle = math.radians(-4)

    def tilted(x, y, w=150, h=20):
        pts = [(x, y), (x + w, y), (x + w, y + h), (x, y + h)]
        return tuple(
            (round(px * math.cos(angle) - py * math.sin(angle)), round(px * math.sin(angle) + py * math.cos(angle)))
            for px, py in pts
        )

    rows = [["r0c0", "r0c1", "r0c2", "r0c3", "r0c4"], ["r1c0", "r1c1", "r1c2", "r1c3", "r1c4"]]
    fragments = [(cell, 1.0, tilted(c * 170, r * 28)) for r, row in enumerate(rows) for c, cell in enumerate(row)]
    text, _ = assemble(fragments)
    assert text.splitlines() == [" ".join(rows[0]), " ".join(rows[1])]


def test_nothing_read_is_empty_with_zero_confidence():
    assert assemble([]) == ("", 0.0)
    assert assemble([("  ", 0.9, _box(0, 0))]) == ("", 0.0)


def test_tesseract_words_are_grouped_into_lines():
    data = {
        "page_num": [1, 1, 1, 1, 1],
        "block_num": [1, 1, 1, 1, 2],
        "par_num": [1, 1, 1, 1, 1],
        "line_num": [1, 1, 1, 2, 1],
        "left": [50, 0, 0, 0, 0],
        "top": [0, 0, 0, 30, 60],
        "width": [40, 40, 0, 40, 40],
        "height": [20, 20, 0, 20, 20],
        "conf": ["90", "70", "-1", "50", "95"],
        "text": ["dünya", "Merhaba", "", "IBAN", "x"],
    }
    fragments = lines_from_tsv(data)
    texts = [f[0] for f in fragments]
    assert texts == ["Merhaba dünya", "IBAN", "x"]
    assert fragments[0][1] == pytest.approx((70 * 7 + 90 * 5) / 12 / 100)


# ── Tesseract, with a stand-in binary ───────────────────────────────────────

_FAKE_TESSERACT = r'''
import os, sys, time
args = sys.argv[1:]
if args == ["--version"]:
    print("tesseract 5.5.0\n leptonica-1.84.1")
elif args[:1] == ["--list-langs"]:
    print('List of available languages in "/usr/share/tesseract-ocr/5/tessdata/" (3):')
    for lang in os.environ.get("FAKE_TESS_LANGS", "eng osd tur").split():
        print(lang)
else:
    time.sleep(float(os.environ.get("FAKE_TESS_SLEEP", "0")))
    rows = [
        "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext",
        "5\t1\t1\t1\t1\t1\t10\t10\t60\t20\t96.5\tTCKN",
        "5\t1\t1\t1\t1\t2\t80\t10\t120\t20\t91\t10000000146",
        "5\t1\t1\t1\t2\t1\t10\t40\t60\t20\t88\tÇağla",
    ]
    with open(args[1] + ".tsv", "w", encoding="utf-8") as fh:
        fh.write("\n".join(rows) + "\n")
'''


@pytest.fixture
def fake_tesseract(tmp_path, monkeypatch):
    pytest.importorskip("pytesseract")
    pytest.importorskip("PIL")
    import pytesseract

    script = tmp_path / "fake_tesseract.py"
    script.write_text(_FAKE_TESSERACT, encoding="utf-8")
    if sys.platform == "win32":
        cmd = tmp_path / "tesseract.bat"
        cmd.write_text(f'@"{sys.executable}" "{script}" %*\n', encoding="utf-8")
    else:
        cmd = tmp_path / "tesseract"
        cmd.write_text(f"#!/bin/sh\nexec '{sys.executable}' '{script}' \"$@\"\n", encoding="utf-8")
        cmd.chmod(0o755)
    # get_tesseract_version caches its first answer for the process.
    monkeypatch.setattr(pytesseract.pytesseract, "tesseract_cmd", str(cmd))
    monkeypatch.setenv("TMPDIR", str(tmp_path))
    monkeypatch.setenv("TEMP", str(tmp_path))
    monkeypatch.setenv("TMP", str(tmp_path))
    return cmd


def test_tesseract_with_its_binary_and_traineddata_reads_lines(fake_tesseract, tmp_path):
    engine = make_engine("tesseract", OcrLimits(threads=3), cmd=str(fake_tesseract))
    ok, reason = engine.available()
    assert ok, reason
    result = engine.read(_png((300, 80)), lang="tur+eng", boxes=True)
    assert result.text == "TCKN 10000000146\nÇağla"
    assert result.engine == "tesseract"
    assert 0.88 < result.confidence < 0.97
    assert len(result.boxes) == 2
    assert os.environ.get("OMP_THREAD_LIMIT")


def test_tesseract_without_the_turkish_traineddata_is_unavailable(fake_tesseract, monkeypatch):
    monkeypatch.setenv("FAKE_TESS_LANGS", "eng osd")
    ok, reason = make_engine("tesseract", cmd=str(fake_tesseract)).available()
    assert not ok and "tur" in reason and "tesseract-ocr-tur" in reason


def test_tesseract_is_killed_at_the_deadline(fake_tesseract, monkeypatch):
    monkeypatch.setenv("FAKE_TESS_SLEEP", "5")
    engine = make_engine("tesseract", OcrLimits(timeout_s=1.0), cmd=str(fake_tesseract))
    assert engine.available()[0]
    start = time.monotonic()
    with pytest.raises(OcrTimeout):
        engine.read(_png())
    assert time.monotonic() - start < 4


def test_tesseract_refuses_a_language_it_has_no_traineddata_for_without_running(fake_tesseract):
    engine = make_engine("tesseract", cmd=str(fake_tesseract))
    assert engine.available()[0]
    with pytest.raises(OcrInputError) as exc:
        engine.read(_png(), lang="tur+deu")
    assert exc.value.code == "bad_lang"


# ── Pins, build and samples ─────────────────────────────────────────────────


def test_the_pinned_models_match_the_pinned_rapidocr_and_the_fetch_script():
    reqs = (ANALYZER_ROOT / "requirements-ocr.txt").read_text(encoding="utf-8")
    assert f"rapidocr=={RAPIDOCR_VERSION}" in reqs
    for _filename, url, sha256, size in RAPIDOCR_MODELS.values():
        assert url.startswith("https://") and f"/v{RAPIDOCR_VERSION}/" in url
        assert len(sha256) == 64 and int(sha256, 16) >= 0 and size > 100_000
    dockerfile = (ANALYZER_ROOT / "Dockerfile").read_text(encoding="utf-8")
    assert "fetch_ocr_models.py" in dockerfile and "model_pins.py" in dockerfile
    assert 'ARG OCR_RAPIDOCR=false' in dockerfile and 'ARG OCR_TESSERACT=false' in dockerfile
    assert "tesseract-ocr-tur" in dockerfile


def test_the_fetch_script_keeps_files_that_match_and_does_not_download(tmp_path, monkeypatch):
    sys.path.insert(0, str(ANALYZER_ROOT / "scripts"))
    try:
        import fetch_ocr_models
    finally:
        sys.path.pop(0)

    calls = []
    monkeypatch.setattr(fetch_ocr_models.urllib.request, "urlopen", lambda *a, **k: calls.append(a))
    monkeypatch.setattr(fetch_ocr_models, "_sha256", lambda path: {v[0]: v[2] for v in RAPIDOCR_MODELS.values()}[path.name])
    for filename, *_ in RAPIDOCR_MODELS.values():
        (tmp_path / filename).write_bytes(b"x")
    assert fetch_ocr_models.main([str(tmp_path)]) == 0
    assert calls == []


def _measure_module():
    sys.path.insert(0, str(ANALYZER_ROOT / "scripts"))
    try:
        import ocr_measure
    finally:
        sys.path.pop(0)
    return ocr_measure


def test_the_measurement_metrics():
    m = _measure_module()
    assert m.levenshtein("kitten", "sitting") == 3
    assert m.levenshtein("", "abc") == 3
    assert m.normalise(" a \n b\t") == "a b"
    assert m.compact("TR33 0006\n1005") == "TR3300061005"
    # "ı" read as "i" and "İ" read as "I": three of the four Turkish letters wrong.
    counts = m.turkish_letter_errors("Işık İzmir çay", "Isik Izmir çay")
    assert counts == {"ş": [1, 1], "ı": [1, 1], "İ": [1, 1], "ç": [0, 1]}
    assert m.tckn("100000001") == "10000000146"
    assert m.tr_iban("00061", "0519786457841326") == "TR33 0006 1005 1978 6457 8413 26"


def _luhn(number: str) -> bool:
    digits = [int(c) for c in number if c.isdigit()][::-1]
    total = sum(d if i % 2 == 0 else (d * 2 - 9 if d > 4 else d * 2) for i, d in enumerate(digits))
    return total % 10 == 0


def _tckn_valid(n: str) -> bool:
    d = [int(c) for c in n]
    return (
        len(d) == 11 and d[0] != 0
        and ((d[0] + d[2] + d[4] + d[6] + d[8]) * 7 - (d[1] + d[3] + d[5] + d[7])) % 10 == d[9]
        and sum(d[:10]) % 10 == d[10]
    )


def _iban_valid(iban: str) -> bool:
    raw = iban.replace(" ", "")
    moved = raw[4:] + raw[:4]
    return int("".join(str(int(c, 36)) for c in moved)) % 97 == 1


def test_the_samples_hold_only_fake_checksum_valid_identifiers():
    manifest = json.loads((SAMPLES / "samples.json").read_text(encoding="utf-8"))
    assert len(manifest["samples"]) >= 6
    checks = {"tckn": _tckn_valid, "iban": _iban_valid, "card": _luhn}
    test_pans = {"4111111111111111", "5555555555554444", "4012888888881881", "378282246310005"}
    seen = set()
    for sample in manifest["samples"]:
        assert (SAMPLES / sample["file"]).is_file()
        for ident in sample["identifiers"]:
            value = ident["value"]
            seen.add(ident["type"])
            assert compact(value) in compact(sample["text"])
            if ident["type"] in checks:
                assert checks[ident["type"]](value), value
            if ident["type"] == "card":
                assert compact(value) in test_pans
            if ident["type"] == "email":
                assert value.rsplit("@", 1)[1] in {"example.com", "example.org", "example.net"}
    assert seen == {"tckn", "iban", "card", "email"}
    assert any(s["identifiers"][0]["value"] == "10000000146" for s in manifest["samples"] if s["identifiers"])


# ── Real engines, where installed ───────────────────────────────────────────


def _engine_or_skip(name):
    engine = make_engine(name, OcrLimits(threads=2, timeout_s=120))
    ok, reason = engine.available()
    if not ok:
        pytest.skip(f"{name}: {reason}")
    return engine


@pytest.mark.parametrize("name", ["rapidocr", "tesseract"])
def test_a_real_engine_reads_the_identifiers_in_the_customer_table(name):
    engine = _engine_or_skip(name)
    result = engine.read((SAMPLES / "customer_table.png").read_bytes(), lang="tur+eng")
    assert isinstance(result, OcrResult) and result.engine == name
    text = compact(result.text)
    assert "10000000146" in text
    assert compact(SECRET) in text
    assert "4111111111111111" in text
    assert 0.0 < result.confidence <= 1.0


@pytest.mark.parametrize("name", ["rapidocr", "tesseract"])
def test_a_real_engine_reads_nothing_from_a_blank_image(name):
    pytest.importorskip("PIL")
    engine = _engine_or_skip(name)
    result = engine.read(_png((300, 100)))
    assert result.text == "" and result.confidence == 0.0


def test_tesseract_refuses_a_language_it_has_no_traineddata_for():
    pytest.importorskip("PIL")
    engine = _engine_or_skip("tesseract")
    with pytest.raises(OcrInputError) as exc:
        engine.read(_png(), lang="tur+zzz")
    assert exc.value.code == "bad_lang"
