"""Measure an OCR engine on the DLP sample images, or regenerate those images.

The samples are in ``analyzer/tests/ocr_samples``: PNG and JPEG files drawn
with Pillow, and ``samples.json`` with the text each one shows and the
identifiers in it. Every identifier is fake: checksum-valid TC identity
numbers built for tests (10000000146 and others of the same kind), the IBAN
example used across the repository and IBANs with a made-up account number,
the card networks' published test card numbers, and addresses at example.com.

Measure (the engine is picked as the analyzer picks it, see app/ocr)::

    python scripts/ocr_measure.py --engine rapidocr --threads 4
    python scripts/ocr_measure.py --engine tesseract --threads 4 --json out.json

In the analyzer image built with ``--build-arg OCR_TESSERACT=true``, from the
repository root (the samples are not in the image)::

    docker run --rm --cpus 4 -v "$PWD/analyzer:/src:ro" -w /src \\
        tamga-analyzer:ocr python scripts/ocr_measure.py --engine tesseract --threads 4

Regenerate the samples (needs the DejaVu fonts: fonts-dejavu-core on Debian;
on Windows they are in C:\\Windows\\Fonts when installed)::

    python scripts/ocr_measure.py --generate --font-dir /usr/share/fonts/truetype/dejavu

What is reported, per image and in total:

- ``cer``: character error rate, the Levenshtein distance between the
  expected and the read text divided by the expected length, after both are
  NFC-normalised and every run of whitespace is made one space;
- ``tr_cer``: the same for the Turkish letters only (ç ğ ı İ ö ş ü and their
  capitals): the share of those letters in the expected text that are not
  matched by the same letter in the read text, the two texts aligned with
  difflib's SequenceMatcher;
- ``recall`` per identifier kind: the share of identifiers found exactly in
  the read text once all whitespace is removed from both;
- ``latency``: the median of ``--repeat`` reads per image, after one warm-up
  read of the first image; the warm-up (model loading included) is reported
  as ``cold_ms``.

The read text is not printed unless ``--show-text`` is given.
"""

from __future__ import annotations

import argparse
import difflib
import json
import os
import platform
import statistics
import sys
import time
import unicodedata
from pathlib import Path

ANALYZER_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_SAMPLES = ANALYZER_ROOT / "tests" / "ocr_samples"
TURKISH_LETTERS = set("çğıİöşüÇĞÖŞÜ")

if str(ANALYZER_ROOT) not in sys.path:
    sys.path.insert(0, str(ANALYZER_ROOT))


# ── Fake identifiers ────────────────────────────────────────────────────────


def tckn(first9: str) -> str:
    """A checksum-valid TC identity number from nine digits (the first not 0)."""
    d = [int(c) for c in first9]
    d10 = ((d[0] + d[2] + d[4] + d[6] + d[8]) * 7 - (d[1] + d[3] + d[5] + d[7])) % 10
    d11 = (sum(d) + d10) % 10
    return first9 + str(d10) + str(d11)


def tr_iban(bank: str, account: str) -> str:
    """A Turkish IBAN with valid check digits, grouped in fours."""
    bban = bank + "0" + account
    assert len(bban) == 22
    rearranged = bban + "2927" + "00"  # T=29, R=27
    check = 98 - int(rearranged) % 97
    raw = f"TR{check:02d}{bban}"
    return " ".join(raw[i : i + 4] for i in range(0, len(raw), 4))


TCKNS = [tckn("100000001"), tckn("200000002"), tckn("300000003"), tckn("123456789"), tckn("400000004")]
IBANS = [
    "TR33 0006 1005 1978 6457 8413 26",
    tr_iban("00099", "0000000012345678"),
    tr_iban("00099", "0000000087654321"),
    tr_iban("00099", "0000000011223344"),
]
CARDS = ["4111 1111 1111 1111", "5555 5555 5555 4444", "4012 8888 8888 1881", "3782 822463 10005"]
EMAILS = [
    "cagla.simsek@example.com",
    "ismail.ozturk@example.com",
    "gulsen.yildirim@example.org",
    "umit.dogan@example.net",
    "ayse.yilmaz@example.com",
]
NAMES = ["Çağla Şimşek", "İsmail Öztürk", "Gülşen Yıldırım", "Ümit Doğan"]


# ── Sample generation ───────────────────────────────────────────────────────


def _fonts(font_dir: Path):
    from PIL import ImageFont

    def load(name: str, size: int):
        return ImageFont.truetype(str(font_dir / name), size)

    return load


def _table_image(load, scale: float = 1.0):
    """A CRM-like customer list: title bar, header row, zebra rows, grid."""
    from PIL import Image, ImageDraw

    size = round(15 * scale)
    font, bold = load("DejaVuSans.ttf", size), load("DejaVuSans-Bold.ttf", size)
    title_font = load("DejaVuSans-Bold.ttf", round(18 * scale))
    headers = ["Ad Soyad", "TC Kimlik No", "IBAN", "Kart No", "E-posta"]
    rows = [[NAMES[i], TCKNS[i], IBANS[i], CARDS[i], EMAILS[i]] for i in range(4)]
    pad = round(12 * scale)
    widths = []
    for col in range(len(headers)):
        cells = [headers[col]] + [r[col] for r in rows]
        widths.append(max(round(bold.getlength(c)) for c in cells) + 2 * pad)
    row_h = round(34 * scale)
    title_h = round(48 * scale)
    width = sum(widths) + 2 * pad
    height = title_h + row_h * (len(rows) + 1) + 2 * pad
    img = Image.new("RGB", (width, height), (246, 247, 249))
    d = ImageDraw.Draw(img)
    title = "Müşteri Listesi - Kurumsal Satış"
    d.rectangle([0, 0, width, title_h], fill=(32, 41, 56))
    d.text((pad, title_h // 2), title, font=title_font, fill=(255, 255, 255), anchor="lm")
    y = title_h + pad
    lines = [title]
    for r, cells in enumerate([headers] + rows):
        x = pad
        fill = (225, 229, 236) if r == 0 else ((255, 255, 255) if r % 2 else (240, 243, 247))
        d.rectangle([pad, y, width - pad, y + row_h], fill=fill)
        for col, cell in enumerate(cells):
            d.text((x + pad, y + row_h // 2), cell, font=bold if r == 0 else font, fill=(20, 24, 31), anchor="lm")
            x += widths[col]
            d.line([x, y, x, y + row_h], fill=(200, 205, 214))
        d.line([pad, y + row_h, width - pad, y + row_h], fill=(200, 205, 214))
        lines.append(" ".join(cells))
        y += row_h
    identifiers = (
        [("tckn", v) for v in TCKNS[:4]]
        + [("iban", v) for v in IBANS[:4]]
        + [("card", v) for v in CARDS[:4]]
        + [("email", v) for v in EMAILS[:4]]
    )
    return img, lines, identifiers


def _chat_image(load):
    """A messaging app: two people, bubbles left and right, timestamps."""
    from PIL import Image, ImageDraw

    font = load("DejaVuSans.ttf", 16)
    small = load("DejaVuSans.ttf", 12)
    head = load("DejaVuSans-Bold.ttf", 17)
    messages = [
        (False, "Merhaba, ödemeyi şu hesaba yapabilir misiniz?", "09:41"),
        (False, "IBAN: " + IBANS[0], "09:41"),
        (True, "Tabii. Fatura için TC kimlik numaranızı da yazar mısınız?", "09:43"),
        (False, "TCKN " + TCKNS[0] + ", adres Çankaya, Ankara.", "09:44"),
        (True, "Kartla ödersem: " + CARDS[0] + " son kullanma 12/30", "09:46"),
        (False, "Please send the invoice to " + EMAILS[4], "09:47"),
        (True, "Sure, I will e-mail it today. Görüşürüz!", "09:48"),
    ]
    width = 760
    pad, bubble_pad, gap = 16, 12, 12
    bh = 16 + 2 * bubble_pad + 6
    heights = 64 + pad // 2 + len(messages) * (bh + gap + 18) + pad
    img = Image.new("RGB", (width, heights), (236, 229, 221))
    d = ImageDraw.Draw(img)
    d.rectangle([0, 0, width, 56], fill=(7, 94, 84))
    d.text((pad, 28), "Ayşe Yılmaz", font=head, fill=(255, 255, 255), anchor="lm")
    d.text((width - pad, 28), "çevrimiçi", font=small, fill=(220, 240, 235), anchor="rm")
    lines = ["Ayşe Yılmaz çevrimiçi"]
    y = 64 + pad // 2
    for mine, text, stamp in messages:
        tw = round(font.getlength(text))
        bw = tw + 2 * bubble_pad + 44
        x0 = width - pad - bw if mine else pad
        d.rounded_rectangle([x0, y, x0 + bw, y + bh], radius=8, fill=(220, 248, 198) if mine else (255, 255, 255))
        d.text((x0 + bubble_pad, y + bubble_pad), text, font=font, fill=(17, 27, 33))
        d.text((x0 + bw - 8, y + bh - 6), stamp, font=small, fill=(110, 120, 125), anchor="rd")
        lines.append(f"{text} {stamp}")
        y += bh + gap + 18
    identifiers = [("iban", IBANS[0]), ("tckn", TCKNS[0]), ("card", CARDS[0]), ("email", EMAILS[4])]
    return img, lines, identifiers


def _paragraph_image(load, paragraphs, size: int, width: int, mono: bool = False, bg=(255, 255, 255), fg=(0, 0, 0)):
    from PIL import Image, ImageDraw

    font = load("DejaVuSansMono.ttf" if mono else "DejaVuSans.ttf", size)
    line_h = round(size * 1.5)
    pad = 20
    height = pad * 2 + line_h * len(paragraphs)
    img = Image.new("RGB", (width, height), bg)
    d = ImageDraw.Draw(img)
    for i, line in enumerate(paragraphs):
        d.text((pad, pad + i * line_h), line, font=font, fill=fg)
    return img, list(paragraphs)


TURKISH_LINES = [
    "Pijamalı hasta yağız şoföre çabucak güvendi.",
    "PİJAMALI HASTA YAĞIZ ŞOFÖRE ÇABUCAK GÜVENDİ.",
    "Müşterinin kimlik bilgileri ışığında ödeme günü değiştirildi.",
    "Şirket çalışanlarının özlük dosyaları gizlidir; İnsan Kaynakları onaylar.",
    "Öğrenci işleri, üçüncü çeyrekte düzenlenen sınav sonuçlarını açıkladı.",
    "Iğdır, Muğla, Çorum, Şırnak, Düzce ve İzmir şubeleri güncellendi.",
    "ÇIĞ ÖNLEMİ: GÜNEŞLİ, ÜSKÜDAR VE IŞIKKENT DEPOLARI KAPALI.",
]

ENGLISH_LINES = [
    "Customer onboarding form - internal use only",
    "Full name: Gulsen Yildirim    Customer ID: CUST-000123",
    "National ID (TCKN): " + TCKNS[2],
    "Payment account: " + IBANS[2],
    "Card on file: " + CARDS[2] + "    Expiry: 11/29",
    "Contact e-mail: " + EMAILS[2],
    "Notes: the customer asked us to call back after 5 pm.",
]

SMALL_LINES = [
    "Hesap özeti / Account summary",
    "Müşteri: Ümit Doğan   TCKN: " + TCKNS[3],
    "IBAN: " + IBANS[3],
    "Kart: " + CARDS[3] + "   E-posta: " + EMAILS[3],
    "Ödeme tarihi geçmiş; gecikme faizi işletilecektir.",
]


def _photo_like(img):
    """Low contrast, a slight rotation, blur, noise and JPEG compression."""
    from PIL import Image, ImageEnhance, ImageFilter

    w, h = img.size
    img = img.resize((round(w * 0.8), round(h * 0.8)), Image.Resampling.LANCZOS)
    img = ImageEnhance.Contrast(img).enhance(0.45)
    img = ImageEnhance.Brightness(img).enhance(0.92)
    img = img.rotate(3.5, resample=Image.Resampling.BICUBIC, expand=True, fillcolor=(150, 146, 140))
    img = img.filter(ImageFilter.GaussianBlur(1.1))
    noise = Image.effect_noise(img.size, 18).convert("RGB")
    img = Image.blend(img, noise, 0.12)
    # Uneven light: darker towards the bottom right, as a phone photo of a screen.
    shade = Image.linear_gradient("L").resize(img.size)
    img = Image.composite(img, ImageEnhance.Brightness(img).enhance(0.75), shade.point(lambda v: 255 - v // 2))
    return img


def generate(out_dir: Path, font_dir: Path) -> None:
    load = _fonts(font_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    samples = []

    def add(name: str, kind: str, img, lines, identifiers, fmt: str = "PNG"):
        path = out_dir / name
        if fmt == "JPEG":
            img.save(path, "JPEG", quality=62)
        else:
            img.save(path, "PNG", optimize=True)
        samples.append(
            {
                "file": name,
                "kind": kind,
                "text": "\n".join(lines),
                "identifiers": [{"type": t, "value": v} for t, v in identifiers],
            }
        )

    table, table_lines, table_ids = _table_image(load)
    add("customer_table.png", "screenshot of a customer table", table, table_lines, table_ids)

    chat, chat_lines, chat_ids = _chat_image(load)
    add("chat.png", "chat screenshot", chat, chat_lines, chat_ids)

    add("customer_table_photo.jpg", "photo of the table: low contrast, rotated 3.5 degrees, blurred, JPEG",
        _photo_like(table), table_lines, table_ids, fmt="JPEG")

    img, lines = _paragraph_image(load, TURKISH_LINES, 20, 980)
    add("turkish_text.png", "Turkish sentences, every Turkish letter in both cases", img, lines, [])

    img, lines = _paragraph_image(load, ENGLISH_LINES, 17, 900)
    add("english_form.png", "English form with identifiers", img, lines,
        [("tckn", TCKNS[2]), ("iban", IBANS[2]), ("card", CARDS[2]), ("email", EMAILS[2])])

    img, lines = _paragraph_image(load, SMALL_LINES, 10, 520)
    add("small_font_10px.png", "10 px text", img, lines,
        [("tckn", TCKNS[3]), ("iban", IBANS[3]), ("card", CARDS[3]), ("email", EMAILS[3])])

    img, lines = _paragraph_image(load, SMALL_LINES, 8, 440)
    add("small_font_8px.png", "8 px text", img, lines,
        [("tckn", TCKNS[3]), ("iban", IBANS[3]), ("card", CARDS[3]), ("email", EMAILS[3])])

    img, lines = _paragraph_image(load, SMALL_LINES, 14, 700, mono=True, bg=(30, 30, 30), fg=(212, 212, 212))
    add("dark_mono.png", "light monospace text on a dark background", img, lines,
        [("tckn", TCKNS[3]), ("iban", IBANS[3]), ("card", CARDS[3]), ("email", EMAILS[3])])

    manifest = {
        "about": "Generated by scripts/ocr_measure.py --generate. Every identifier is fake.",
        "fonts": "DejaVu Sans, DejaVu Sans Bold, DejaVu Sans Mono",
        "samples": samples,
    }
    (out_dir / "samples.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    for s in samples:
        print(f"{s['file']}: {(out_dir / s['file']).stat().st_size} bytes")


# ── Metrics ─────────────────────────────────────────────────────────────────


def normalise(text: str) -> str:
    return " ".join(unicodedata.normalize("NFC", text).split())


def compact(text: str) -> str:
    return "".join(unicodedata.normalize("NFC", text).split())


def levenshtein(a: str, b: str) -> int:
    if len(a) < len(b):
        a, b = b, a
    previous = list(range(len(b) + 1))
    for i, ca in enumerate(a, 1):
        current = [i]
        for j, cb in enumerate(b, 1):
            current.append(min(previous[j] + 1, current[j - 1] + 1, previous[j - 1] + (ca != cb)))
        previous = current
    return previous[-1]


def turkish_letter_errors(expected: str, read: str) -> dict[str, list[int]]:
    """letter -> [wrong, total] over the Turkish letters of ``expected``."""
    positions = [i for i, c in enumerate(expected) if c in TURKISH_LETTERS]
    counts: dict[str, list[int]] = {}
    if not positions:
        return counts
    matched = set()
    matcher = difflib.SequenceMatcher(None, expected, read, autojunk=False)
    for block in matcher.get_matching_blocks():
        matched.update(range(block.a, block.a + block.size))
    for i in positions:
        entry = counts.setdefault(expected[i], [0, 0])
        entry[0] += int(i not in matched)
        entry[1] += 1
    return counts


# ── Measurement ─────────────────────────────────────────────────────────────


def measure(args: argparse.Namespace) -> int:
    from app.ocr import OcrError, OcrLimits, make_engine

    samples_dir = Path(args.samples)
    manifest = json.loads((samples_dir / "samples.json").read_text(encoding="utf-8"))
    limits = OcrLimits(threads=args.threads, timeout_s=args.timeout)
    kwargs: dict[str, object] = {}
    for option in args.option:
        key, _, value = option.partition("=")
        kwargs[key] = int(value) if value.lstrip("-").isdigit() else value
    engine = make_engine(args.engine, limits, **kwargs)
    ok, reason = engine.available()
    if not ok:
        print(f"engine {args.engine} is not available: {reason}", file=sys.stderr)
        return 2

    images = [(s, (samples_dir / s["file"]).read_bytes()) for s in manifest["samples"]]
    t0 = time.perf_counter()
    engine.read(images[0][1], lang=args.lang)
    cold_ms = (time.perf_counter() - t0) * 1000.0

    rows = []
    tr_err_total = tr_total = 0
    dist_total = len_total = 0
    recall: dict[str, list[int]] = {}
    letters: dict[str, list[int]] = {}
    for sample, data in images:
        times, result = [], None
        for _ in range(args.repeat):
            try:
                result = engine.read(data, lang=args.lang)
            except OcrError as exc:
                print(f"{sample['file']}: {type(exc).__name__}: {exc}", file=sys.stderr)
                result = None
                break
            times.append(result.duration_ms)
        read_text = result.text if result else ""
        expected = normalise(sample["text"])
        got = normalise(read_text)
        dist = levenshtein(expected, got)
        by_letter = turkish_letter_errors(expected, got)
        tr_err = sum(v[0] for v in by_letter.values())
        tr_n = sum(v[1] for v in by_letter.values())
        for letter, (wrong, total) in by_letter.items():
            entry = letters.setdefault(letter, [0, 0])
            entry[0] += wrong
            entry[1] += total
        found_ids = []
        haystack = compact(read_text)
        for ident in sample["identifiers"]:
            hit = compact(ident["value"]) in haystack
            recall.setdefault(ident["type"], [0, 0])
            recall[ident["type"]][0] += int(hit)
            recall[ident["type"]][1] += 1
            found_ids.append({"type": ident["type"], "found": hit})
        rows.append(
            {
                "file": sample["file"],
                "kind": sample["kind"],
                "cer": dist / max(1, len(expected)),
                "tr_letters": tr_n,
                "tr_errors": tr_err,
                "identifiers": found_ids,
                "latency_ms": statistics.median(times) if times else None,
                "confidence": result.confidence if result else None,
            }
        )
        tr_err_total += tr_err
        tr_total += tr_n
        dist_total += dist
        len_total += len(expected)
        if args.show_text:
            print(f"--- {sample['file']}\n{read_text}\n")

    latencies = [r["latency_ms"] for r in rows if r["latency_ms"] is not None]
    report = {
        "engine": args.engine,
        "engine_status": reason,
        "options": kwargs,
        "lang": args.lang,
        "threads": args.threads,
        "repeat": args.repeat,
        "machine": f"{platform.system()} {platform.machine()}, {os.cpu_count()} logical CPUs, Python {platform.python_version()}",
        "processor": platform.processor(),
        "cold_ms": cold_ms,
        "total": {
            "cer": dist_total / max(1, len_total),
            "tr_cer": tr_err_total / max(1, tr_total),
            "tr_letters": tr_total,
            "tr_by_letter": {k: {"wrong": v[0], "of": v[1]} for k, v in sorted(letters.items())},
            "recall": {k: {"found": v[0], "of": v[1]} for k, v in sorted(recall.items())},
            "latency_ms_median": statistics.median(latencies) if latencies else None,
            "latency_ms_max": max(latencies) if latencies else None,
        },
        "images": rows,
    }
    if args.json:
        Path(args.json).write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(markdown(report))
    return 0


def markdown(report: dict) -> str:
    kinds = ["tckn", "iban", "card", "email"]
    out = [
        f"Engine: {report['engine_status']}"
        + (f" ({', '.join(f'{k}={v}' for k, v in report['options'].items())})" if report["options"] else "")
        + f"; lang {report['lang']}; {report['threads']} threads; "
        f"median of {report['repeat']}; {report['machine']}. Cold start (first read, models loading): "
        f"{report['cold_ms']:.0f} ms.",
        "",
        "| Image | CER | Turkish letters wrong | " + " | ".join(k.upper() if k != "email" else "E-mail" for k in kinds) + " | Latency |",
        "|---|---|---|" + "---|" * len(kinds) + "---|",
    ]
    for r in report["images"]:
        cells = []
        for k in kinds:
            ids = [i for i in r["identifiers"] if i["type"] == k]
            cells.append(f"{sum(i['found'] for i in ids)}/{len(ids)}" if ids else "-")
        tr = f"{r['tr_errors']}/{r['tr_letters']}" if r["tr_letters"] else "-"
        lat = f"{r['latency_ms']:.0f} ms" if r["latency_ms"] is not None else "failed"
        out.append(f"| {r['file']} | {r['cer']:.1%} | {tr} | " + " | ".join(cells) + f" | {lat} |")
    t = report["total"]
    cells = []
    for k in kinds:
        v = t["recall"].get(k)
        cells.append(f"{v['found']}/{v['of']}" if v else "-")
    med = f"{t['latency_ms_median']:.0f} ms median" if t["latency_ms_median"] is not None else "-"
    out.append(
        f"| **All** | **{t['cer']:.1%}** | **{t['tr_cer']:.1%}** ({t['tr_letters']} letters) | "
        + " | ".join(f"**{c}**" for c in cells)
        + f" | {med} |"
    )
    out.append("")
    out.append(
        "Turkish letters read wrong: "
        + ", ".join(f"{k} {v['wrong']}/{v['of']}" for k, v in t["tr_by_letter"].items())
        + "."
    )
    return "\n".join(out)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n", 1)[0])
    parser.add_argument("--engine", choices=["rapidocr", "tesseract"], default=os.environ.get("TAMGA_OCR_ENGINE") or "rapidocr")
    parser.add_argument("--lang", default="tur+eng")
    parser.add_argument("--threads", type=int, default=4)
    parser.add_argument("--repeat", type=int, default=3)
    parser.add_argument("--timeout", type=float, default=120.0, help="seconds per read")
    parser.add_argument(
        "--option", action="append", default=[], metavar="KEY=VALUE",
        help="an engine constructor argument, e.g. psm=6 (tesseract) or det_limit_side_len=960 (rapidocr)",
    )
    parser.add_argument("--samples", default=str(DEFAULT_SAMPLES))
    parser.add_argument("--json", help="also write the full report here")
    parser.add_argument("--show-text", action="store_true", help="print the text read from each image")
    parser.add_argument("--generate", action="store_true", help="regenerate the sample images instead")
    parser.add_argument("--font-dir", help="directory with DejaVuSans.ttf, DejaVuSans-Bold.ttf, DejaVuSansMono.ttf")
    args = parser.parse_args(argv)
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
    if args.generate:
        if not args.font_dir:
            parser.error("--generate needs --font-dir")
        generate(Path(args.samples), Path(args.font_dir))
        return 0
    return measure(args)


if __name__ == "__main__":
    raise SystemExit(main())
