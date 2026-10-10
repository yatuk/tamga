"""The LLM Guard worker must start: BanTopics has no default topic list.

LLM Guard itself is not needed here. A stand-in for
``llm_guard.output_scanners`` gives BanTopics the signature it has upstream,
where ``topics`` is required, so that constructing it without any raises as
the real one does.
"""

import importlib
import sys
import types

import pytest


class _Scanner:
    def __init__(self, *args, **kwargs):
        self.args, self.kwargs = args, kwargs

    def scan(self, prompt, output):
        return output, True, 0.0


class _BanTopics(_Scanner):
    def __init__(self, topics, *, threshold=0.6):  # topics is required upstream
        if not topics:
            raise ValueError("No topics provided")
        super().__init__(topics=topics, threshold=threshold)


@pytest.fixture
def toxicity(monkeypatch):
    """The real app.scanners.toxicity, next to a stand-in LLM Guard."""
    guard = types.ModuleType("llm_guard")
    outputs = types.ModuleType("llm_guard.output_scanners")
    outputs.Toxicity = outputs.NoRefusal = outputs.BanCode = _Scanner
    outputs.BanTopics = _BanTopics
    guard.output_scanners = outputs
    monkeypatch.setitem(sys.modules, "llm_guard", guard)
    monkeypatch.setitem(sys.modules, "llm_guard.output_scanners", outputs)
    # Other test modules leave a stub under this name while they are
    # collected; this test needs the module itself.
    monkeypatch.delitem(sys.modules, "app.scanners.toxicity", raising=False)
    mod = importlib.import_module("app.scanners.toxicity")
    monkeypatch.setitem(sys.modules, "app.scanners.toxicity", mod)
    return mod


def test_worker_starts_without_topics(toxicity, monkeypatch):
    """No topics configured: the worker loads, and BanTopics is left out."""
    monkeypatch.delenv("TAMGA_BAN_TOPICS", raising=False)
    toxicity._init_llm_guard_worker()  # raised before: BanTopics() needs topics
    assert toxicity._toxicity_scanner is not None
    assert toxicity._ban_topics_scanner is None
    # A scan runs the scanners that are loaded and skips the one that is not.
    assert toxicity._sync_scan("some model output") == []


def test_worker_loads_ban_topics_when_configured(toxicity, monkeypatch):
    monkeypatch.setenv("TAMGA_BAN_TOPICS", "weapons, self-harm ,,")
    toxicity._init_llm_guard_worker()
    assert toxicity._ban_topics_scanner is not None
    assert toxicity._ban_topics_scanner.kwargs["topics"] == ["weapons", "self-harm"]
