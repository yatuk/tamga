FROM python:3.11-slim

# Build stage — compile deps, install heavy packages.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user early so pip/wheels are owned correctly.
RUN groupadd -r tamga && useradd -r -g tamga -d /app -s /sbin/nologin tamga

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt && \
    python -m spacy download en_core_web_sm

COPY . .

RUN chown -R tamga:tamga /app

USER tamga

EXPOSE 8444 50051

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8444/health')" || exit 1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8444"]
