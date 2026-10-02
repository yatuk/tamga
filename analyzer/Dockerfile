# ── Build stage: compilers and wheels stay here ─────────────────────────────
FROM python:3.11-slim AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY requirements.txt .
# The analyzer runs on CPU. llm-guard pulls in torch, and the default PyPI
# wheel bundles the CUDA runtime (several GB of nvidia-* packages). The PyTorch
# CPU index serves "+cpu" builds, which pip prefers for the same version.
RUN pip install --no-cache-dir \
        --extra-index-url https://download.pytorch.org/whl/cpu \
        -r requirements.txt && \
    python -m spacy download en_core_web_sm

# ── Runtime stage ───────────────────────────────────────────────────────────
FROM python:3.11-slim

# /app is the service user's home and must be writable: transformers caches
# downloaded models under ~/.cache.
RUN groupadd -r tamga && useradd -r -g tamga -d /app -s /sbin/nologin tamga && \
    mkdir -p /app && chown tamga:tamga /app

COPY --from=builder /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app
COPY --chown=tamga:tamga . .

USER tamga

EXPOSE 8444 50051

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8444/health')" || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8444"]
