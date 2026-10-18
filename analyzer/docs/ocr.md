# OCR engines: measurement and choice

The analyzer's OCR module (`app/ocr`) reads the text in an image so that the
DLP scanners can look at it. It has two engines and is off by default
(`TAMGA_OCR_ENGINE=rapidocr|tesseract|off`). This page records how the engines
were measured and what was found. Operators: the short version is in
`docs/operations.md`, "OCR engine choice".

## The sample set

`tests/ocr_samples/` holds eight images drawn with Pillow in DejaVu Sans,
DejaVu Sans Bold and DejaVu Sans Mono, and `samples.json` with the text each
shows and the identifiers in it. Every identifier is fake: TC identity numbers
with a valid checksum built for tests (10000000146 and four more of the same
kind), the IBAN example used across the repository and three IBANs with
made-up account numbers and valid check digits, the card networks' published
test numbers, and addresses at example.com, example.org and example.net.

| Image | What it is |
|---|---|
| `customer_table.png` | A CRM screenshot: title bar, header row, four customers with name, TCKN, IBAN, card number and e-mail |
| `chat.png` | A messaging app: a Turkish conversation with an IBAN, a TCKN and a card number, an English message with an e-mail |
| `customer_table_photo.jpg` | The table as a phone photo of a screen: 80 % size, contrast 0.45, rotated 3.5 degrees, blurred, noise, uneven light, JPEG quality 62 |
| `turkish_text.png` | Seven Turkish sentences, every Turkish letter in both cases (73 of them) |
| `english_form.png` | An English form with one identifier of each kind |
| `small_font_10px.png`, `small_font_8px.png` | An account summary at 10 px and 8 px |
| `dark_mono.png` | The same summary in light monospace on a dark background |

`python scripts/ocr_measure.py --generate --font-dir <dir with the DejaVu
fonts>` draws them again. The noise in the photo variant is not seeded, so a
regenerated photo differs from the committed one in its noise; measure with
the committed files to compare engines.

## What is measured

`scripts/ocr_measure.py --engine <name> --threads 4` reads each image and
reports:

- **CER**: Levenshtein distance between the expected and the read text over
  the expected length, after NFC normalisation and with every run of
  whitespace made one space. Reading order counts, so a table read column by
  column scores worse than one read row by row.
- **Turkish letters wrong**: of the ç ğ ı İ ö ş ü Ç Ğ Ö Ş Ü in the expected
  text, the share not matched by the same letter in the read text (aligned
  with difflib's SequenceMatcher). 155 letters over the set.
- **Recall per identifier kind**: an identifier counts as found when its exact
  string is in the read text once all whitespace is removed from both
  (`TR33 0006 1005 ...` read as `TR33 00061005 ...` is found; one wrong digit
  is not).
- **Latency**: the median of five reads per image after a warm-up read; the
  warm-up, which loads the models, is reported as the cold start.

## Results

Measured on 2026-10-18 on an Intel Core Ultra 7 255H (16 logical CPUs),
Windows 11, Python 3.14.7, 4 threads. The full report is
`analyzer/docs/ocr-measure-rapidocr.json`.

**RapidOCR 3.10.0** (onnxruntime 1.31.0; PP-OCRv6 small detection and
recognition, PP-OCRv4 mobile direction classifier; RapidOCR's default
settings). Cold start 2,480 ms.

| Image | CER | Turkish letters wrong | TCKN | IBAN | Card | E-mail | Latency |
|---|---|---|---|---|---|---|---|
| customer_table.png | 0.4% | 2/18 | 4/4 | 4/4 | 4/4 | 4/4 | 925 ms |
| chat.png | 0.3% | 0/19 | 1/1 | 1/1 | 1/1 | 1/1 | 513 ms |
| customer_table_photo.jpg | 6.3% | 15/18 | 4/4 | 4/4 | 4/4 | 2/4 | 948 ms |
| turkish_text.png | 2.4% | 10/73 | - | - | - | - | 769 ms |
| english_form.png | 0.0% | - | 1/1 | 1/1 | 1/1 | 1/1 | 764 ms |
| small_font_10px.png | 0.9% | 2/9 | 1/1 | 1/1 | 1/1 | 1/1 | 733 ms |
| small_font_8px.png | 1.9% | 4/9 | 1/1 | 1/1 | 1/1 | 1/1 | 673 ms |
| dark_mono.png | 0.9% | 1/9 | 1/1 | 1/1 | 1/1 | 1/1 | 861 ms |
| **All** | **1.9%** | **21.9%** | **13/13** | **13/13** | **13/13** | **11/13** | 766 ms median |

Turkish letters read wrong, by letter: İ 8/8, ı 7/31, ş 6/27, Ğ 2/2, ğ 2/13,
Ö 2/8, Ü 2/9, ü 2/23, Ç 1/6, Ş 1/7, ö 1/9, ç 0/12. The capital dotted İ is
read as `I` or `i` every time, and the dotless ı often as `i`; most of the
other misses are in the photo, where the cedilla and breve are lost to the
blur (`Şimşek` read as `Simgek`). The two e-mails missed are in the photo too
(`ismal.ozturk@`, `guhen.ylldirim@`). Every TCKN, IBAN and card number was
recovered, in the photo as well.

**Tesseract** 5.5.0 (Debian trixie, `tur+eng`), measured on 2026-10-18 in the
analyzer image built with `--build-arg OCR_TESSERACT=true`, 4 CPUs
(`docker run --cpus 4`), median of three reads, Linux x86_64 under Docker
Desktop on the same laptop; cold start 1,432 ms:

| Image | CER | Turkish letters wrong | TCKN | IBAN | Card | E-mail | Latency |
|---|---|---|---|---|---|---|---|
| customer_table.png | 3.7% | 3/18 | 4/4 | 4/4 | 4/4 | 4/4 | 662 ms |
| chat.png | 8.6% | 2/19 | 1/1 | 1/1 | 1/1 | 0/1 | 545 ms |
| customer_table_photo.jpg | 53.6% | 10/18 | 2/4 | 0/4 | 1/4 | 0/4 | 651 ms |
| turkish_text.png | 0.0% | 0/73 | - | - | - | - | 458 ms |
| english_form.png | 0.0% | - | 1/1 | 1/1 | 1/1 | 1/1 | 597 ms |
| small_font_10px.png | 0.9% | 0/9 | 1/1 | 1/1 | 1/1 | 1/1 | 232 ms |
| small_font_8px.png | 53.5% | 8/9 | 0/1 | 0/1 | 0/1 | 0/1 | 439 ms |
| dark_mono.png | 1.9% | 1/9 | 1/1 | 1/1 | 1/1 | 0/1 | 563 ms |
| **All** | **15.8%** | **15.5%** of 155 | **10/13** | **8/13** | **9/13** | **6/13** | 554 ms median |

Tesseract reads clean Turkish prose better than RapidOCR (no letter wrong in
`turkish_text.png`, the capital İ wrong 2 times of 8 against 8 of 8) and is
faster, but it loses most of the photo and all of the 8 px text, and with them
the identifiers: 8 of 13 IBANs and 6 of 13 e-mail addresses. The two
latencies were measured on different systems (Windows natively, Linux in
Docker) and compare only roughly.

To repeat it, from the repository root:

```
docker build -t tamga-analyzer:ocr --build-arg OCR_TESSERACT=true analyzer
docker run --rm --cpus 4 -v "$PWD/analyzer:/src:ro" -w /src \
    tamga-analyzer:ocr python scripts/ocr_measure.py --engine tesseract --threads 4
```

The same command with `OCR_RAPIDOCR=true` and `--engine rapidocr` measures
RapidOCR in the image, on Linux.

### RapidOCR detector size

RapidOCR scales each image so that its shorter side is 736 pixels before
detecting text (`limit_type min`), which enlarges a wide, short screenshot a
lot and is most of its latency. Three other settings were measured on the
same set (median of three reads, so the latencies are less steady):

| Detector setting | CER | Turkish letters wrong | IBAN | Card | E-mail | Latency (median) |
|---|---|---|---|---|---|---|
| shorter side 736 (default) | 1.9% | 21.9% | 13/13 | 13/13 | 11/13 | 766 ms |
| shorter side 480 | 1.7% | 20.6% | 13/13 | 12/13 | 11/13 | 526 ms |
| longer side at most 960 | 3.7% | 19.4% | 10/13 | 13/13 | 11/13 | 349 ms |
| longer side at most 1280 | 3.7% | 19.4% | 10/13 | 13/13 | 11/13 | 285 ms |

TCKN recall was 13/13 in every row. Capping the longer side halves the
latency but loses three of the four IBANs in the photo; the default is kept.
`scripts/ocr_measure.py --option det_limit_type=max --option
det_limit_side_len=1280` repeats a row.

## Recommendation

RapidOCR, when OCR is turned on. On this set it recovers every TCKN, IBAN and
card number, including in the photo, at under a second per image on four
threads, and needs nothing from the operating system. Its weak point for
Turkish is the dotted capital İ and the dotless ı, which matters for names and
little for the identifiers DLP acts on, since those are digits and ASCII.
Tesseract, measured with the command above, does better on the Turkish
letters of clean text but finds far fewer identifiers in a photo or in small
text (10, 8, 9 and 6 of 13 against 13, 13, 13 and 11), which is what DLP acts
on; it also needs about 95 MB of system packages and a subprocess per image.
RapidOCR stays the recommendation.

## Licences, sources and sizes

| Component | Version | Licence | Size |
|---|---|---|---|
| rapidocr | 3.10.0 (PyPI) | Apache-2.0 | 1 MB of code |
| onnxruntime | 1.31.0 (PyPI) | MIT | about 48 MB installed |
| opencv-python-headless (rapidocr's dependency) | 5.0.0.93 (PyPI) | Apache-2.0 | about 113 MB installed |
| shapely, pyclipper, omegaconf (rapidocr's) | 2.2.0, 1.4.0, 2.4.0 | BSD-3, MIT, BSD-3 | about 10 MB |
| PP-OCRv6_det_small.onnx | RapidOCR v3.10.0 release | Apache-2.0 (PaddleOCR) | 9,929,594 bytes, SHA-256 `090f04ab...e1ff94f` |
| PP-OCRv6_rec_small.onnx | RapidOCR v3.10.0 release | Apache-2.0 (PaddleOCR) | 21,234,383 bytes, SHA-256 `6f327246...b3c14884` |
| ch_ppocr_mobile_v2.0_cls_mobile.onnx | RapidOCR v3.10.0 release | Apache-2.0 (PaddleOCR) | 585,532 bytes, SHA-256 `e47acedf...2d6215c` |
| pytesseract | 0.3.13 (PyPI) | Apache-2.0 | small |
| tesseract-ocr, libtesseract5 | 5.5.0 (Debian trixie) | Apache-2.0 | 6.2 MB installed |
| tesseract-ocr-eng, -osd, -tur | 4.1.0 (Debian trixie) | Apache-2.0 | 4.0, 10.3 and 4.5 MB installed |
| Pillow | 13.0.0 (PyPI) | MIT-CMU | already in the image through reportlab |

The model files come from
`https://www.modelscope.cn/models/RapidAI/RapidOCR/resolve/v3.10.0/onnx/...`;
the full URLs and hashes are in `app/ocr/model_pins.py`, copied from the
rapidocr 3.10.0 wheel's `default_models.yaml` and checked against the files it
downloaded. The installed sizes of the Python packages are from a Windows
install; the Linux wheels are of the same order. The Debian sizes are from
packages.debian.org; the Tesseract packages pull in libicu76 (37 MB), cairo,
pango, glib, curl and image libraries, about 95 MB in all.
