"""Classifier logic, with a stand-in for the model.

The stand-in scores a window by whether it contains a marker token, which is
enough to check windowing, ordering, truncation and the label mapping without
shipping a model in the repository. test_real_model.py runs the real one when
TAMGA_CLASSIFIER_MODEL_DIR points at it.
"""

from __future__ import annotations

import math
from types import SimpleNamespace

import numpy as np
import pytest

from app.classifier import Classifier, ClassifierUnavailable, _attack_index, _attack_probability, _windows

MARK = 999


BOS, EOS = 2, 3


class FakeTokenizer:
    """One id per word, wrapped in a start and an end token like a real
    tokenizer's output; the word "ATTACK" is the marker."""

    def encode(self, text: str):
        body = [MARK if w == "ATTACK" else 1 for w in text.split()]
        ids = [BOS] + body + [EOS]
        return SimpleNamespace(ids=ids, special_tokens_mask=[1] + [0] * len(body) + [1])


class FakeSession:
    def __init__(self) -> None:
        self.calls: list[dict] = []

    def run(self, _outputs, feed):
        self.calls.append(feed)
        hit = bool((feed["input_ids"] == MARK).any())
        # logits [safe, attack]
        return [np.array([[0.0, 4.0]] if hit else [[4.0, 0.0]])]


def make(**kw) -> tuple[Classifier, FakeSession]:
    session = FakeSession()
    kw.setdefault("window", 6)
    kw.setdefault("overlap", 1)
    return Classifier.from_parts(session, FakeTokenizer(), **kw), session


def test_scores_come_back_in_order():
    clf, _ = make()
    out = clf.score(["hello there", "please ATTACK now", "fine"])
    assert [round(s.score, 2) for s in out] == [0.02, 0.98, 0.02]
    assert not any(s.truncated for s in out)


def test_long_text_is_scored_by_its_worst_window():
    clf, session = make()
    text = " ".join(["w"] * 20 + ["ATTACK"] + ["w"] * 20)
    out = clf.score([text])
    assert out[0].score > 0.9, "an injection in the middle of a long text was averaged away"
    assert len(session.calls) > 1


def test_too_long_text_is_marked_truncated():
    clf, session = make(max_windows=2)
    text = " ".join(["w"] * 40 + ["ATTACK"])
    out = clf.score([text])
    assert out[0].truncated
    assert len(session.calls) == 2
    # The marker is past what was read: the low score is not a verdict.
    assert out[0].score < 0.5


def test_windows_cover_the_whole_text_and_keep_the_special_tokens():
    ids = [BOS] + list(range(10, 30)) + [EOS]
    special = [1] + [0] * 20 + [1]
    pieces = _windows(ids, special, window=8, overlap=2)
    assert all(p[0] == BOS and p[-1] == EOS and len(p) <= 8 for p in pieces)
    seen = [t for p in pieces for t in p[1:-1]]
    assert set(seen) == set(range(10, 30)), "part of the text was never scored"
    # Neighbouring windows share the overlap, so nothing sits only on a cut.
    assert pieces[0][-3:-1] == pieces[1][1:3]
    assert _windows(ids[:6], special[:6], window=8, overlap=2) == [ids[:6]]


def test_empty_text_is_not_sent_to_the_model():
    clf, session = make()
    out = clf.score(["", "   ", "ok"])
    assert [s.score for s in out[:2]] == [0.0, 0.0]
    assert len(session.calls) == 1


def test_inputs_the_model_declares_are_all_fed():
    clf, session = make(inputs=("input_ids", "attention_mask", "token_type_ids"))
    clf.score(["a b"])
    feed = session.calls[0]
    assert set(feed) == {"input_ids", "attention_mask", "token_type_ids"}
    assert feed["input_ids"].tolist() == [[BOS, 1, 1, EOS]]
    assert feed["attention_mask"].tolist() == [[1, 1, 1, 1]]
    assert feed["token_type_ids"].tolist() == [[0, 0, 0, 0]]


def test_unconfigured_classifier_is_unavailable():
    clf = Classifier(model_dir="")
    assert not clf.available
    with pytest.raises(ClassifierUnavailable):
        clf.score(["x"])


def test_missing_directory_is_unavailable_not_a_crash(tmp_path):
    clf = Classifier(model_dir=str(tmp_path / "nope"))
    assert not clf.available
    assert "no ONNX model" in clf.reason


def test_broken_model_file_is_unavailable_not_a_crash(tmp_path):
    (tmp_path / "onnx").mkdir()
    (tmp_path / "onnx" / "model.onnx").write_bytes(b"not a model")
    (tmp_path / "tokenizer.json").write_text("{}", encoding="utf-8")
    clf = Classifier(model_dir=str(tmp_path))
    assert not clf.available
    assert "could not be loaded" in clf.reason


@pytest.mark.parametrize(
    "id2label,want",
    [
        ({"0": "SAFE", "1": "INJECTION"}, 1),
        ({"0": "BENIGN", "1": "MALICIOUS"}, 1),
        ({"0": "INJECTION", "1": "SAFE"}, 0),
        ({"0": "LABEL_0", "1": "LABEL_1"}, 1),
        ({}, 1),
    ],
)
def test_attack_label_is_found_by_name(id2label, want):
    assert _attack_index(id2label) == want


def test_probability_from_logits():
    assert math.isclose(_attack_probability([0.0, 0.0], 1), 0.5)
    assert _attack_probability([10.0, -10.0], 1) < 0.001
    assert _attack_probability([10.0, -10.0], 0) > 0.999
    # A single logit is a sigmoid.
    assert math.isclose(_attack_probability([0.0], 1), 0.5)
