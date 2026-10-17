"""Shadow mode, with stand-ins for both models.

The shadow model scores what the primary scores and its scores come back
next to the primary's; it never changes them, a slow or broken shadow does
not hold the call up beyond the shadow wait, and a missing one is reported
in health. test_real_model.py covers a real shadow when
TAMGA_CLASSIFIER_SHADOW_MODEL_DIR is set.
"""

from __future__ import annotations

import asyncio
import threading
import time

import numpy as np

from classifier.v1.classifier_pb2 import ClassifyRequest, HealthRequest

from app.classifier import Classifier
from app.shadow import SKIP_BUSY, SKIP_FAILED, SKIP_LATE, Shadow
from tests.test_classifier import MARK, FakeSession, FakeTokenizer
from tests.test_server import call, loaded


class InvertedSession(FakeSession):
    """A candidate that disagrees with the primary on everything."""

    def run(self, _outputs, feed):
        self.calls.append(feed)
        hit = bool((feed["input_ids"] == MARK).any())
        return [np.array([[4.0, 0.0]] if hit else [[0.0, 4.0]])]


class GatedSession(FakeSession):
    """Waits for a gate before answering: a shadow slower than the primary."""

    def __init__(self) -> None:
        super().__init__()
        self.gate = threading.Event()
        self.started = threading.Event()

    def run(self, outputs, feed):
        self.started.set()
        self.gate.wait(5)
        return super().run(outputs, feed)


class BrokenSession(FakeSession):
    def run(self, _outputs, _feed):
        raise RuntimeError("model file is corrupt")


def shadow_of(session, wait_ms: float = 500, name: str = "candidate") -> Shadow:
    return Shadow(Classifier.from_parts(session, FakeTokenizer(), name=name), wait_ms=wait_ms)


def test_shadow_scores_come_back_next_to_the_primary_and_change_nothing():
    shadow = shadow_of(InvertedSession())
    req = ClassifyRequest(request_id="r1", texts=["hello there", "please ATTACK now"])
    with_shadow = call(loaded(), lambda stub: stub.Classify(req), shadow=shadow)
    without = call(loaded(), lambda stub: stub.Classify(req))

    assert list(with_shadow.scores) == list(without.scores)
    assert with_shadow.model == without.model == "stand-in"
    assert with_shadow.shadow_model == "candidate"
    assert len(with_shadow.shadow_scores) == 2 and len(with_shadow.shadow_truncated) == 2
    # The candidate disagrees on both, which is what the comparison is for.
    assert with_shadow.shadow_scores[0] > 0.5 > with_shadow.shadow_scores[1]
    assert with_shadow.shadow_skipped == ""
    assert not without.shadow_model and not without.shadow_scores and not without.shadow_skipped


def test_late_shadow_is_dropped_within_the_wait():
    session = GatedSession()
    shadow = shadow_of(session, wait_ms=30)
    req = ClassifyRequest(texts=["please ATTACK now"])

    started = time.perf_counter()
    resp = call(loaded(), lambda stub: stub.Classify(req), shadow=shadow)
    elapsed = time.perf_counter() - started
    session.gate.set()

    assert resp.scores[0] > 0.5
    assert resp.shadow_model == "candidate" and not resp.shadow_scores
    assert resp.shadow_skipped == SKIP_LATE
    # The call waited about the shadow wait, not for the shadow (gated 5 s).
    assert elapsed < 2.0


def test_busy_shadow_is_skipped_not_queued():
    session = GatedSession()
    shadow = shadow_of(session, wait_ms=0)

    async def go():
        first = shadow.start(["one"])
        assert session.started.wait(2)
        second = shadow.start(["two"])
        r1 = await shadow.collect(first)
        r2 = await shadow.collect(second)
        session.gate.set()
        first.result(timeout=5)
        return r1, r2

    r1, r2 = asyncio.run(go())
    assert r1.skipped == SKIP_LATE
    assert r2.skipped == SKIP_BUSY and not r2.scores
    # Once the first run has ended the shadow takes calls again.
    assert not isinstance(shadow.start(["three"]), str)


def test_broken_shadow_does_not_touch_the_primary():
    shadow = shadow_of(BrokenSession())
    resp = call(loaded(), lambda stub: stub.Classify(ClassifyRequest(texts=["please ATTACK now"])), shadow=shadow)
    assert resp.scores[0] > 0.5
    assert resp.shadow_skipped == SKIP_FAILED and not resp.shadow_scores


def test_no_shadow_dir_is_reported_in_health_and_scoring_goes_on():
    off = Shadow(Classifier(model_dir="", dir_env="TAMGA_CLASSIFIER_SHADOW_MODEL_DIR"))
    health = call(loaded(), lambda stub: stub.Health(HealthRequest()), shadow=off)
    assert health.available and not health.shadow_available
    assert "TAMGA_CLASSIFIER_SHADOW_MODEL_DIR" in health.shadow_reason
    resp = call(loaded(), lambda stub: stub.Classify(ClassifyRequest(texts=["please ATTACK now"])), shadow=off)
    assert resp.scores[0] > 0.5 and not resp.shadow_model and not resp.shadow_skipped


def test_unloadable_shadow_dir_is_reported_in_health(tmp_path):
    off = Shadow(Classifier(model_dir=str(tmp_path), dir_env="TAMGA_CLASSIFIER_SHADOW_MODEL_DIR", label="shadow"))
    health = call(loaded(), lambda stub: stub.Health(HealthRequest()), shadow=off)
    assert health.available and not health.shadow_available
    assert str(tmp_path) in health.shadow_reason


def test_health_names_a_loaded_shadow():
    health = call(loaded(), lambda stub: stub.Health(HealthRequest()), shadow=shadow_of(FakeSession()))
    assert health.shadow_available and health.shadow_model == "candidate" and not health.shadow_reason


def test_shadow_reads_its_own_variables(monkeypatch, tmp_path):
    monkeypatch.setenv("TAMGA_CLASSIFIER_MODEL_DIR", "/primary/only")
    monkeypatch.setenv("TAMGA_CLASSIFIER_SHADOW_MODEL_DIR", str(tmp_path))
    monkeypatch.setenv("TAMGA_CLASSIFIER_SHADOW_WAIT_MS", "35")
    shadow = Shadow()
    assert shadow.classifier.model_dir == str(tmp_path)
    assert abs(shadow.wait_s - 0.035) < 1e-9
    assert not shadow.available
