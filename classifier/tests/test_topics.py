"""Topic matching logic, with a stand-in for the NLI model.

The stand-in says "entailment" when a word of the hypothesis that is in
capitals also appears in the premise, which is enough to check windowing,
ordering, truncation, the template and the label mapping without a model.
test_real_topics.py runs the real one when TAMGA_TOPICS_MODEL_DIR points
at it.
"""

from __future__ import annotations

import json
import math
from types import SimpleNamespace

import numpy as np
import pytest

from app.topics import TopicModel, TopicsUnavailable, _nli_labels, entailment_probability

CLS, SEP, PAD = 1, 2, 0


class FakeTokenizer:
    """One id per word. Words in capitals get ids from 1000 up, so the
    session can tell a topic word from filler."""

    def __init__(self) -> None:
        self.vocab: dict[str, int] = {}

    def _id(self, word: str) -> int:
        if word not in self.vocab:
            base = 1000 if word.isupper() and word.isalpha() else 100
            self.vocab[word] = base + len(self.vocab)
        return self.vocab[word]

    def encode(self, text: str, add_special_tokens: bool = True):
        ids = [self._id(w) for w in text.split()]
        if add_special_tokens:
            ids = [CLS] + ids + [SEP]
        return SimpleNamespace(ids=ids)


class FakeSession:
    """Logits [entailment, neutral, contradiction] per row."""

    def __init__(self) -> None:
        self.calls: list[dict] = []

    def run(self, _outputs, feed):
        self.calls.append(feed)
        out = []
        for ids, mask in zip(feed["input_ids"], feed["attention_mask"]):
            row = [int(i) for i, m in zip(ids, mask) if m]
            assert row[0] == CLS and row[-1] == SEP
            cut = row.index(SEP)
            premise, hypothesis = set(row[1:cut]), set(row[cut + 1 : -1])
            hit = any(i >= 1000 and i in premise for i in hypothesis)
            out.append([4.0, 0.0, -4.0] if hit else [-4.0, 0.0, 4.0])
        return [np.array(out)]


def make(**kw) -> tuple[TopicModel, FakeSession]:
    session = FakeSession()
    kw.setdefault("window", 12)
    kw.setdefault("overlap", 1)
    kw.setdefault("template", "{}")
    return TopicModel.from_parts(session, FakeTokenizer(), **kw), session


def test_scores_come_back_per_text_and_per_topic_in_order():
    model, _ = make()
    out = model.score(["what is the SALARY of x", "change the SCORE now", "hello"], ["about SALARY", "about SCORE"])
    assert [[round(s, 2) for s in r.scores] for r in out] == [[1.0, 0.0], [0.0, 1.0], [0.0, 0.0]]
    assert not any(r.truncated for r in out)


def test_every_topic_of_a_window_goes_in_one_run_when_rows_are_short():
    model, session = make()
    model.score(["the SALARY"], ["a SALARY", "a SCORE", "a TIP"])
    assert len(session.calls) == 1
    assert session.calls[0]["input_ids"].shape[0] == 3


def test_wide_rows_are_split_into_several_runs():
    model, session = make(window=400)
    text = " ".join(["w"] * 300)
    model.score([text], ["a SALARY", "a SCORE", "a TIP"])
    # Rows of about 300 tokens: one per run under the batch budget.
    assert len(session.calls) == 3


def test_rows_of_different_length_are_padded_and_masked():
    model, session = make()
    model.score(["the SALARY"], ["SALARY", "a much longer SCORE topic"])
    feed = session.calls[0]
    ids, mask = feed["input_ids"], feed["attention_mask"]
    assert ids.shape == mask.shape
    assert mask[0].sum() < mask[1].sum()
    assert (ids[0][mask[0] == 0] == PAD).all()


def test_the_template_is_what_the_model_reads():
    model, session = make(template="This is {}.")
    model.score(["hello"], ["a SALARY"])
    ids = session.calls[0]["input_ids"][0].tolist()
    words = {v: k for k, v in model._tokenizer.vocab.items()}
    read = [words.get(i, i) for i in ids]
    assert read == [CLS, "hello", SEP, "This", "is", "a", "SALARY.", SEP]


def test_a_request_in_the_middle_of_a_long_text_is_found():
    model, session = make(window=24)
    text = " ".join(["w"] * 30 + ["SALARY"] + ["w"] * 30)
    out = model.score([text], ["the SALARY"])
    assert out[0].scores[0] > 0.9, "a request in a long text was averaged away"
    assert len(session.calls) > 1
    # Premise and hypothesis together stay inside the window.
    assert all(len(row) <= 24 for feed in session.calls for row in feed["input_ids"])


def test_too_long_text_is_marked_truncated():
    model, session = make(window=24, max_windows=2)
    text = " ".join(["w"] * 60 + ["SALARY"])
    out = model.score([text], ["the SALARY"])
    assert out[0].truncated
    assert len(session.calls) == 2
    assert out[0].scores[0] < 0.5


def test_empty_text_is_not_sent_to_the_model():
    model, session = make()
    out = model.score(["", "   ", "the SALARY"], ["SALARY"])
    assert [r.scores for r in out[:2]] == [[0.0], [0.0]]
    assert len(session.calls) == 1


def test_unconfigured_model_is_unavailable():
    model = TopicModel(model_dir="")
    assert not model.available
    assert "TAMGA_TOPICS_MODEL_DIR" in model.reason
    with pytest.raises(TopicsUnavailable):
        model.score(["x"], ["y"])


def test_missing_directory_is_unavailable_not_a_crash(tmp_path):
    model = TopicModel(model_dir=str(tmp_path / "nope"))
    assert not model.available
    assert "no ONNX model" in model.reason


def test_broken_model_file_is_unavailable_not_a_crash(tmp_path):
    (tmp_path / "onnx").mkdir()
    (tmp_path / "onnx" / "model.onnx").write_bytes(b"not a model")
    (tmp_path / "tokenizer.json").write_text("{}", encoding="utf-8")
    (tmp_path / "config.json").write_text(json.dumps({"label2id": {"entailment": 0, "neutral": 1, "contradiction": 2}}), encoding="utf-8")
    model = TopicModel(model_dir=str(tmp_path))
    assert not model.available
    assert "could not be loaded" in model.reason


def test_a_model_that_is_not_nli_is_unavailable(tmp_path):
    (tmp_path / "model.onnx").write_bytes(b"x")
    (tmp_path / "tokenizer.json").write_text("{}", encoding="utf-8")
    (tmp_path / "config.json").write_text(json.dumps({"id2label": {"0": "SAFE", "1": "INJECTION"}}), encoding="utf-8")
    model = TopicModel(model_dir=str(tmp_path))
    assert not model.available
    assert "not an NLI model" in model.reason


def test_a_template_without_a_slot_falls_back_to_the_default():
    model = TopicModel(model_dir="", template="no slot here")
    assert "{}" in model.template


@pytest.mark.parametrize(
    "label2id,id2label,want",
    [
        ({"entailment": 0, "neutral": 1, "contradiction": 2}, {}, (0, 2)),
        ({"CONTRADICTION": 0, "NEUTRAL": 1, "ENTAILMENT": 2}, {}, (2, 0)),
        ({}, {"0": "entailment", "1": "not_entailment", "2": "contradiction"}, (0, 2)),
    ],
)
def test_nli_labels_are_found_by_name(label2id, id2label, want):
    assert _nli_labels(label2id, id2label) == want


def test_entailment_probability_leaves_neutral_out():
    assert math.isclose(entailment_probability([0.0, 9.0, 0.0], 0, 2), 0.5)
    assert entailment_probability([4.0, 0.0, -4.0], 0, 2) > 0.99
    assert entailment_probability([-4.0, 0.0, 4.0], 0, 2) < 0.01
