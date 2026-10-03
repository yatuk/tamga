# The classifier is its own small image on purpose. It sits on the proxy's
# decision path, so it must not share a process with the analyzer's heavy,
# asynchronous work, and the analyzer's pinned dependencies (llm-guard holds
# tokenizers below 0.20) cannot read current model files.
#
# No model is baked in: mount one at TAMGA_CLASSIFIER_MODEL_DIR.
FROM python:3.11-slim AS builder

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

FROM python:3.11-slim

RUN groupadd -r tamga && useradd -r -g tamga -d /app -s /sbin/nologin tamga && \
    mkdir -p /app && chown tamga:tamga /app

COPY --from=builder /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    TAMGA_CLASSIFIER_PORT=50052 \
    TAMGA_CLASSIFIER_MODEL_DIR=/models/classifier

WORKDIR /app
COPY --chown=tamga:tamga . .

USER tamga

EXPOSE 50052

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD python -m app.healthcheck || exit 1

CMD ["python", "-m", "app.server"]
