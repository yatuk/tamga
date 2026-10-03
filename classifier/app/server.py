"""gRPC server of the classifier service."""

from __future__ import annotations

import asyncio
import os
import signal
import time

import grpc
import structlog

from classifier.v1.classifier_pb2 import (  # type: ignore[import-untyped]
    ClassifyRequest,
    ClassifyResponse,
    HealthRequest,
    HealthResponse,
)
from classifier.v1.classifier_pb2_grpc import (  # type: ignore[import-untyped]
    ClassifierServiceServicer,
    add_ClassifierServiceServicer_to_server,
)

from app.classifier import Classifier, ClassifierUnavailable

logger = structlog.get_logger()

# A request is one model run per window. Bounding what one call may ask for
# keeps a single caller from holding the CPU.
MAX_TEXTS = 64
MAX_CHARS = 200_000


class ClassifierServicer(ClassifierServiceServicer):
    def __init__(self, classifier: Classifier) -> None:
        self._classifier = classifier

    async def Classify(  # type: ignore[override]
        self,
        request: ClassifyRequest,
        context: grpc.aio.ServicerContext,
    ) -> ClassifyResponse:
        start = time.monotonic()
        clf = self._classifier
        if not clf.available:
            await context.abort(grpc.StatusCode.UNAVAILABLE, clf.reason)
        texts = list(request.texts)
        if len(texts) > MAX_TEXTS or sum(len(t) for t in texts) > MAX_CHARS:
            await context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "too much text in one call")
        try:
            # Inference is CPU-bound; off the event loop, so health checks
            # are still answered while it runs.
            scores = await asyncio.to_thread(clf.score, texts)
        except ClassifierUnavailable as exc:
            await context.abort(grpc.StatusCode.UNAVAILABLE, str(exc))
        except Exception as exc:
            logger.warning("classifier_failed", request_id=request.request_id, error=str(exc))
            await context.abort(grpc.StatusCode.INTERNAL, "classifier failed")
        return ClassifyResponse(
            scores=[s.score for s in scores],
            truncated=[s.truncated for s in scores],
            model=clf.model_name,
            duration_ms=round((time.monotonic() - start) * 1000, 2),
        )

    async def Health(  # type: ignore[override]
        self,
        request: HealthRequest,
        context: grpc.aio.ServicerContext,
    ) -> HealthResponse:
        clf = self._classifier
        return HealthResponse(available=clf.available, model=clf.model_name, reason=clf.reason)


async def serve(classifier: Classifier, port: int) -> grpc.aio.Server:
    server = grpc.aio.server(
        options=[
            ("grpc.keepalive_time_ms", 30000),
            ("grpc.keepalive_timeout_ms", 5000),
            ("grpc.keepalive_permit_without_calls", True),
            ("grpc.http2.max_pings_without_data", 0),
        ],
    )
    add_ClassifierServiceServicer_to_server(ClassifierServicer(classifier), server)
    server.add_insecure_port(f"[::]:{port}")
    await server.start()
    logger.info("classifier_listening", port=port, available=classifier.available, model=classifier.model_name)
    return server


async def main() -> None:
    port = int(os.environ.get("TAMGA_CLASSIFIER_PORT", "50052"))
    server = await serve(Classifier(), port)
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, stop.set)
        except NotImplementedError:  # Windows
            pass
    await stop.wait()
    await server.stop(5.0)


if __name__ == "__main__":
    asyncio.run(main())
