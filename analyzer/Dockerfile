# ── Build stage: compilers and wheels stay here ─────────────────────────────
# TODO: pin by digest (image:tag@sha256:...): docs/operations.md, "Image pins".
FROM python:3.11-slim AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY requirements.txt .
# Nothing in requirements.txt asks for torch or transformers, so PyPI is the
# only index. The spaCy model is fetched here, at build time.
RUN pip install --no-cache-dir -r requirements.txt && \
    python -m spacy download en_core_web_sm

# Optional OCR engines (app/ocr, TAMGA_OCR_ENGINE), both off by default.
# OCR_RAPIDOCR=true installs rapidocr and onnxruntime and fetches the model
# files pinned by URL and SHA-256 in app/ocr/model_pins.py (about 205 MB more
# in the image, 32 MB of it the models). OCR_TESSERACT=true installs
# pytesseract here and the Debian packages in the runtime stage below.
ARG OCR_RAPIDOCR=false
ARG OCR_TESSERACT=false
COPY requirements-ocr.txt .
COPY app/ocr/model_pins.py scripts/fetch_ocr_models.py /build/ocr/
RUN mkdir -p /opt/ocr-models && \
    if [ "$OCR_RAPIDOCR" = "true" ]; then \
        pip install --no-cache-dir $(grep -E '^(pillow|rapidocr|onnxruntime|opencv-python-headless)==' requirements-ocr.txt) && \
        python /build/ocr/fetch_ocr_models.py /opt/ocr-models; \
    fi && \
    if [ "$OCR_TESSERACT" = "true" ]; then \
        pip install --no-cache-dir $(grep -E '^(pillow|pytesseract)==' requirements-ocr.txt); \
    fi

# ── Runtime stage ───────────────────────────────────────────────────────────
# TODO: pin by digest (image:tag@sha256:...): docs/operations.md, "Image pins".
FROM python:3.11-slim

LABEL org.opencontainers.image.title="Tamga analyzer" \
      org.opencontainers.image.description="Asynchronous deep analysis and PDF reports for the Tamga LLM security proxy" \
      org.opencontainers.image.source="https://github.com/yatuk/tamga" \
      org.opencontainers.image.licenses="AGPL-3.0-only"

# /app is the service user's home and is writable, for whatever a library
# caches under ~/.cache.
RUN groupadd -r tamga && useradd -r -g tamga -d /app -s /sbin/nologin tamga && \
    mkdir -p /app && chown tamga:tamga /app

# Tesseract and its Turkish traineddata, for TAMGA_OCR_ENGINE=tesseract
# (OCR_TESSERACT=true). About 95 MB installed: tesseract-ocr, libtesseract5,
# the eng, osd and tur traineddata and leptonica are 28 MB, libicu76 37 MB,
# and cairo, pango, glib, curl and the image libraries the rest.
ARG OCR_TESSERACT=false
RUN if [ "$OCR_TESSERACT" = "true" ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends tesseract-ocr tesseract-ocr-tur && \
        rm -rf /var/lib/apt/lists/*; \
    fi

COPY --from=builder /opt/venv /opt/venv
COPY --from=builder /opt/ocr-models /opt/ocr-models
ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    TAMGA_OCR_MODEL_DIR=/opt/ocr-models

WORKDIR /app
COPY --chown=tamga:tamga . .

USER tamga

EXPOSE 8444 50051

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8444/health')" || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8444"]
