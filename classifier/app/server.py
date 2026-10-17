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
    TopicScores,
    TopicsRequest,
    TopicsResponse,
)
from classifier.v1.classifier_pb2_grpc import (  # type: ignore[import-untyped]
    ClassifierServiceServicer,
    add_ClassifierServiceServicer_to_server,
)

from app.classifier import Classifier, ClassifierUnavailable
from app.topics import TopicModel, TopicsUnavailable

logger = structlog.get_logger()

# A request is one model run per window. Bounding what one call may ask for
# keeps a single caller from holding the CPU.
MAX_TEXTS = 64
MAX_CHARS = 200_000

# A topic call is one model run per window of each text, with every topic
# in it, and a run grows with the number of topics. These bound it well
# above what the policy lets the proxy send (topics.banned, max_chars).
MAX_TOPIC_TEXTS = 32
MAX_TOPICS = 32
MAX_TOPIC_CHARS = 500
MAX_TOPIC_TEXT_CHARS = 50_000


class ClassifierServicer(ClassifierServiceServicer):
    def __init__(self, classifier: Classifier, topics: TopicModel | None = None) -> None:
        self._classifier = classifier
        self._topics = topics if topics is not None else TopicModel(model_dir="")

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

    async def ClassifyTopics(  # type: ignore[override]
        self,
        request: TopicsRequest,
        context: grpc.aio.ServicerContext,
    ) -> TopicsResponse:
        start = time.monotonic()
        model = self._topics
        if not model.available:
            await context.abort(grpc.StatusCode.UNAVAILABLE, model.reason)
        texts = list(request.texts)
        topics = list(request.topics)
        if not topics or any(not t.strip() for t in topics):
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "every topic needs a description")
        if (
            len(texts) > MAX_TOPIC_TEXTS
            or len(topics) > MAX_TOPICS
            or any(len(t) > MAX_TOPIC_CHARS for t in topics)
            or sum(len(t) for t in texts) > MAX_TOPIC_TEXT_CHARS
        ):
            await context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "too much text or too many topics in one call")
        try:
            results = await asyncio.to_thread(model.score, texts, topics)
        except TopicsUnavailable as exc:
            await context.abort(grpc.StatusCode.UNAVAILABLE, str(exc))
        except Exception as exc:
            # The text is not logged, and neither are the topics.
            logger.warning("topics_failed", request_id=request.request_id, error_type=type(exc).__name__)
            await context.abort(grpc.StatusCode.INTERNAL, "topic classification failed")
        return TopicsResponse(
            results=[TopicScores(scores=r.scores, truncated=r.truncated) for r in results],
            model=model.model_name,
            duration_ms=round((time.monotonic() - start) * 1000, 2),
        )

    async def Health(  # type: ignore[override]
        self,
        request: HealthRequest,
        context: grpc.aio.ServicerContext,
    ) -> HealthResponse:
        clf, topics = self._classifier, self._topics
        return HealthResponse(
            available=clf.available,
            model=clf.model_name,
            reason=clf.reason,
            topics_available=topics.available,
            topics_model=topics.model_name,
            topics_reason=topics.reason,
        )


async def serve(classifier: Classifier, port: int, topics: TopicModel | None = None) -> grpc.aio.Server:
    server = grpc.aio.server(
        options=[
            ("grpc.keepalive_time_ms", 30000),
            ("grpc.keepalive_timeout_ms", 5000),
            ("grpc.keepalive_permit_without_calls", True),
            ("grpc.http2.max_pings_without_data", 0),
        ],
    )
    servicer = ClassifierServicer(classifier, topics)
    add_ClassifierServiceServicer_to_server(servicer, server)
    server.add_insecure_port(f"[::]:{port}")
    await server.start()
    logger.info(
        "classifier_listening",
        port=port,
        available=classifier.available,
        model=classifier.model_name,
        topics_available=servicer._topics.available,
        topics_model=servicer._topics.model_name,
    )
    return server


async def main() -> None:
    port = int(os.environ.get("TAMGA_CLASSIFIER_PORT", "50052"))
    server = await serve(Classifier(), port, TopicModel())
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
