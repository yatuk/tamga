"""Download RapidOCR's model files into a directory and check their SHA-256.

    python scripts/fetch_ocr_models.py /opt/ocr-models

The URLs and hashes are in app/ocr/model_pins.py. A file already present with
the right hash is kept; a download with the wrong hash is deleted and the
script exits 1. The analyzer image runs this at build time when built with
``--build-arg OCR_RAPIDOCR=true``; the engine itself never downloads.

The script loads model_pins.py by path, so it works in the Docker build stage
where only these two files are copied, with the standard library alone.
"""

from __future__ import annotations

import hashlib
import importlib.util
import sys
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
CANDIDATES = [HERE.parent / "app" / "ocr" / "model_pins.py", HERE / "model_pins.py"]


def _pins():
    for path in CANDIDATES:
        if path.is_file():
            spec = importlib.util.spec_from_file_location("model_pins", path)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            return module.RAPIDOCR_MODELS
    raise SystemExit(f"model_pins.py not found in {', '.join(str(p) for p in CANDIDATES)}")


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main(argv: list[str]) -> int:
    if len(argv) != 1:
        print(__doc__, file=sys.stderr)
        return 2
    target = Path(argv[0])
    target.mkdir(parents=True, exist_ok=True)
    for role, (filename, url, sha256, size) in _pins().items():
        path = target / filename
        if path.is_file() and _sha256(path) == sha256:
            print(f"{role}: {filename} present")
            continue
        partial = path.with_suffix(path.suffix + ".part")
        with urllib.request.urlopen(url, timeout=120) as response, partial.open("wb") as out:
            while chunk := response.read(1 << 20):
                out.write(chunk)
        got = _sha256(partial)
        if got != sha256 or partial.stat().st_size != size:
            partial.unlink()
            print(f"{role}: {url} has SHA-256 {got}, expected {sha256}", file=sys.stderr)
            return 1
        partial.replace(path)
        print(f"{role}: {filename} fetched, {size} bytes")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
