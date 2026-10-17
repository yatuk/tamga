"""The export script, as far as it can be tested without a model.

finetune/export.py needs torch, transformers, onnx and onnxruntime to
export and to compare. What is checked here: that loading it imports none
of those, that its arguments are checked, that the layout check says what
the service would load, and that the report shows the numbers it is given.
describe_onnx runs on a tiny graph built here when onnx is installed.
"""

from __future__ import annotations

import ast
import importlib.util
import json
import sys
from pathlib import Path

import pytest

SCRIPT = Path(__file__).resolve().parents[1] / "finetune" / "export.py"
HEAVY = {"torch", "transformers", "onnx", "onnxruntime", "numpy", "tokenizers"}


def _load():
    spec = importlib.util.spec_from_file_location("tamga_export", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _model_dir(path: Path, *, model_file: str | None = "onnx/model_quantized.onnx", id2label=None) -> Path:
    path.mkdir(parents=True, exist_ok=True)
    (path / "tokenizer.json").write_text("{}", encoding="utf-8")
    labels = {"0": "SAFE", "1": "INJECTION"} if id2label is None else id2label
    (path / "config.json").write_text(json.dumps({"id2label": labels}), encoding="utf-8")
    if model_file:
        (path / model_file).parent.mkdir(parents=True, exist_ok=True)
        (path / model_file).write_bytes(b"not a model")
    return path


def test_script_imports_nothing_heavy_at_module_level():
    tree = ast.parse(SCRIPT.read_text(encoding="utf-8"))
    top = set()
    for node in tree.body:
        if isinstance(node, ast.Import):
            top.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            top.add(node.module.split(".")[0])
    assert not top & HEAVY, f"imported at module level: {sorted(top & HEAVY)}"

    before = set(sys.modules)
    _load()
    assert not (set(sys.modules) - before) & HEAVY


def test_arguments(tmp_path):
    ex = _load()
    model = tmp_path / "model"
    model.mkdir()
    (model / "config.json").write_text("{}", encoding="utf-8")
    (model / "tokenizer.json").write_text("{}", encoding="utf-8")

    args = ex.parse_args(["export", "--model", str(model), "--out", str(tmp_path / "out"), "--threads", "4"])
    assert args.func is ex.cmd_export and args.threads == 4 and not args.drop_fp32

    # A directory without the files is not a model directory.
    (tmp_path / "empty").mkdir()
    with pytest.raises(SystemExit):
        ex.parse_args(["export", "--model", str(tmp_path / "empty"), "--out", str(tmp_path / "out")])
    # Writing over the source, or into a full directory without --force, is refused.
    with pytest.raises(SystemExit):
        ex.parse_args(["export", "--model", str(model), "--out", str(model)])
    full = tmp_path / "full"
    full.mkdir()
    (full / "x").write_text("x", encoding="utf-8")
    with pytest.raises(SystemExit):
        ex.parse_args(["export", "--model", str(model), "--out", str(full)])
    assert ex.parse_args(["export", "--model", str(model), "--out", str(full), "--force"]).force
    with pytest.raises(SystemExit):
        ex.parse_args(["export", "--model", str(model), "--out", str(tmp_path / "o"), "--threads", "-1"])

    args = ex.parse_args(["parity", "--model", str(model), "--export", str(tmp_path / "e")])
    assert args.func is ex.cmd_parity and args.fpr is None and args.report is None
    with pytest.raises(SystemExit):
        ex.parse_args(["parity", "--model", str(model), "--export", str(tmp_path / "e"), "--fpr", "1.5"])


def test_layout_check(tmp_path):
    ex = _load()
    assert ex.check_layout(_model_dir(tmp_path / "good")) == []
    # The shipped operator copy is laid out like this too.
    assert ex.cmd_check(ex.parse_args(["check", str(tmp_path / "good")])) == 0

    problems = ex.check_layout(_model_dir(tmp_path / "fp32", model_file="onnx/model.onnx"))
    assert problems == ["the service would load onnx/model.onnx, not the int8 export onnx/model_quantized.onnx"]

    problems = ex.check_layout(_model_dir(tmp_path / "none", model_file=None))
    assert len(problems) == 1 and problems[0].startswith("no ONNX model")

    problems = ex.check_layout(_model_dir(tmp_path / "labels", id2label={"0": "A", "1": "B"}))
    assert len(problems) == 1 and "exactly one attack class" in problems[0]

    bare = _model_dir(tmp_path / "bare")
    (bare / "tokenizer.json").unlink()
    (bare / "config.json").unlink()
    assert len(ex.check_layout(bare)) == 2
    assert ex.check_layout(tmp_path / "missing") == [f"{tmp_path / 'missing'} is not a directory"]


def _shipped_description():
    return {
        "opset": 17,
        "inputs": [
            {"name": "input_ids", "type": "int64", "shape": ["batch", "seq"]},
            {"name": "attention_mask", "type": "int64", "shape": ["batch", "seq"]},
        ],
        "outputs": [{"name": "logits", "type": "float32", "shape": ["batch", 2]}],
        "quantized": True,
        "quantized_weights": [{"name": "emb", "type": "uint8", "shape": [256000, 384]}],
    }


def test_graph_problems():
    ex = _load()
    assert ex.graph_problems(_shipped_description()) == []

    desc = _shipped_description()
    desc["opset"] = 18
    desc["inputs"][1]["shape"] = [1, "seq"]
    desc["outputs"][0]["shape"] = [1, 2]
    desc["quantized_weights"].append({"name": "w", "type": "int8", "shape": [384, 384]})
    problems = ex.graph_problems(desc)
    assert len(problems) == 4, problems


def test_compare():
    ex = _load()
    c = ex.compare([0.10, 0.979, 0.99, 0.50], [0.12, 0.981, 0.99, 0.50], threshold=0.98)
    assert c["rows"] == 4 and c["flips"] == 1
    assert c["max_abs"] == pytest.approx(0.02) and c["mean_abs"] == pytest.approx(0.0055)
    assert ex.compare([], [], 0.98) == {"rows": 0, "max_abs": 0.0, "mean_abs": 0.0, "flips": 0}
    with pytest.raises(ValueError):
        ex.compare([0.1], [], 0.98)


def test_report():
    ex = _load()
    m = {"attacks": 4, "benign": 4, "threshold": 0.5, "fpr": 0.25, "recall": 0.5,
         "recall_at_shipped": 0.25, "fpr_at_shipped": 0.0}
    result = {
        "meta": {"model": "m", "export": "e", "threshold": 0.98, "fpr": 0.02, "window": 512, "overlap": 64,
                 "max_windows": 16},
        "sets": {
            "tr_locked.csv": {
                "compare": {"pytorch / onnx-int8": {"rows": 8, "max_abs": 0.0123456, "mean_abs": 0.0001, "flips": 1}},
                "measure": {"onnx-int8": m, "pytorch": dict(m, threshold=None)},
                "truncated": 0,
            }
        },
    }
    text = ex.report_markdown(result)
    assert "| `tr_locked.csv` | 8 | pytorch / onnx-int8 | 0.012346 | 0.000100 | 1 |" in text
    assert "| `tr_locked.csv` | onnx-int8 | 4 | 4 | 0.5000 | 0.250 | 0.500 | 0.250 | 0.000 |" in text
    assert "| `tr_locked.csv` | pytorch | 4 | 4 | none |" in text
    assert "more windows" not in text


def test_describe_a_tiny_graph(tmp_path):
    onnx = pytest.importorskip("onnx")
    from onnx import TensorProto, helper

    ex = _load()
    table = helper.make_tensor("table", TensorProto.UINT8, [4, 2], [0] * 8)
    graph = helper.make_graph(
        [helper.make_node("Gather", ["table", "input_ids"], ["logits"])],
        "tiny",
        [
            helper.make_tensor_value_info("input_ids", TensorProto.INT64, ["batch", "seq"]),
            helper.make_tensor_value_info("attention_mask", TensorProto.INT64, ["batch", "seq"]),
        ],
        [helper.make_tensor_value_info("logits", TensorProto.UINT8, ["batch", 2])],
        [table],
    )
    path = tmp_path / "tiny.onnx"
    onnx.save(helper.make_model(graph, opset_imports=[helper.make_opsetid("", 17)]), str(path))
    desc = ex.describe_onnx(path)
    assert desc["opset"] == 17 and desc["quantized_weights"][0]["type"] == "uint8"
    assert ex.graph_problems(desc) == []
