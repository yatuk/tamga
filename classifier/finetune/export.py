"""Turn a fine-tuned model into the directory the classifier service loads.

``finetune.py run`` leaves a PyTorch model in ``out/model/``. The service
runs an int8 ONNX file (``app/classifier.py``). This script closes the gap
and checks that nothing was lost on the way:

``export``
    exports the PyTorch model to ONNX in 32-bit floats, with the batch and
    the sequence length as dynamic axes, then quantizes it to int8 the way
    the shipped ``onnx/model_quantized.onnx`` was made, and copies the
    tokenizer and config files beside it.
``parity``
    scores the sets ``finetune.py`` evaluates with the PyTorch model, the
    32-bit ONNX and the int8 ONNX, all three through the service's own
    windowing and scoring code, and writes how far apart they are.
``check``
    says whether a directory has what the service needs to load it.

How the shipped file was made, as far as it can be read from the file
itself (``onnx`` 1.23, 2026-10-17): opset 17; inputs ``input_ids`` and
``attention_mask``, int64, shape ``[batch, seq]``; one output ``logits``,
float, ``[batch, 2]``; graph names of the legacy (TorchScript) exporter on a
wrapper that returns the logits; producer ``onnx.quantize``, which is
``onnxruntime.quantization.quantize_dynamic``; and one quantized weight,
the token embedding, as per-tensor uint8 behind a ``DequantizeLinear``.
Every MatMul is still in 32-bit floats. The operator copy's
``onnx/quantization_check.json`` names that variant ``gather_only``. The
settings below reproduce it.

Only the standard library is imported at module level, as in
``finetune.py``: torch, transformers, onnx and onnxruntime are imported
inside the functions that need them.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import math
import shutil
import sys
import types
from pathlib import Path

HERE = Path(__file__).resolve().parent
SERVICE_ROOT = HERE.parent  # classifier/, where the service's ``app`` package is

# What the shipped export has, and so what this one is made with.
OPSET = 17
INPUTS = ("input_ids", "attention_mask")
OUTPUT = "logits"
QUANTIZE_OPS = ("Gather",)  # the embedding lookup; MatMuls stay in fp32
QUANT_WEIGHT_TYPE = "QUInt8"

INT8_FILE = "onnx/model_quantized.onnx"
FP32_FILE = "onnx/model.onnx"
# config.json and tokenizer.json are what the service reads; the other two
# are copied when present so the directory matches the operator's copy.
COPY_FILES = ("config.json", "tokenizer.json", "tokenizer_config.json", "special_tokens_map.json")
NEEDED_SOURCE_FILES = ("config.json", "tokenizer.json")

# The export is traced once at this length. It is above the model's local
# attention window (128 tokens) so that the sliding-window path is in the
# trace, and the export is then checked at two other lengths.
TRACE_TOKENS = 200
# Largest logit difference between PyTorch and the fp32 export that the
# export step accepts. fp32 against fp32 differs by rounding only.
FP32_TOLERANCE = 1e-3


# ── Loading the two neighbours ──────────────────────────────────────────────


def finetune_module():
    """finetune.py, loaded from beside this file."""
    name = "tamga_finetune"
    if name in sys.modules:
        return sys.modules[name]
    spec = importlib.util.spec_from_file_location(name, HERE / "finetune.py")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def service_module():
    """The service's ``app.classifier``, so that parity scores through its
    code and the layout check uses its list of model files.

    The service logs with structlog, which the fine-tune environment does
    not install. Without it a stand-in that drops the log lines is used:
    logging is the only thing the module uses it for.
    """
    if str(SERVICE_ROOT) not in sys.path:
        sys.path.insert(0, str(SERVICE_ROOT))
    try:
        import structlog  # noqa: F401
    except ImportError:
        stub = types.ModuleType("structlog")

        class _Quiet:
            def __getattr__(self, _name):
                return lambda *a, **k: None

        stub.get_logger = lambda *a, **k: _Quiet()
        sys.modules["structlog"] = stub
    from app import classifier

    return classifier


# ── Parts that need no model ────────────────────────────────────────────────


def check_layout(directory: Path) -> list[str]:
    """What stops the service from loading ``directory`` as the int8
    export; an empty list when nothing does. Reads files, runs nothing."""
    svc = service_module()
    root = Path(directory)
    if not root.is_dir():
        return [f"{root} is not a directory"]
    problems = []
    first = next((f for f in svc._MODEL_FILES if (root / f).is_file()), None)
    if first is None:
        problems.append(f"no ONNX model: none of {', '.join(svc._MODEL_FILES)}")
    elif first != INT8_FILE:
        problems.append(f"the service would load {first}, not the int8 export {INT8_FILE}")
    if not (root / "tokenizer.json").is_file():
        problems.append("no tokenizer.json")
    config_path = root / "config.json"
    if not config_path.is_file():
        problems.append("no config.json: the service would guess the attack class is index 1")
    else:
        try:
            config = json.loads(config_path.read_text(encoding="utf-8"))
        except ValueError as exc:
            problems.append(f"config.json is not JSON: {exc}")
        else:
            labels = {int(k): str(v).lower() for k, v in (config.get("id2label") or {}).items()}
            attack = [i for i, name in labels.items() if name not in svc._BENIGN_LABELS]
            if len(attack) != 1:
                problems.append(
                    f"config.json id2label {config.get('id2label')} does not name exactly one attack class"
                )
    return problems


def graph_problems(desc: dict) -> list[str]:
    """Where an ONNX file described by ``describe_onnx`` differs from the
    shipped export in what the service relies on."""
    problems = []
    if desc.get("opset") != OPSET:
        problems.append(f"opset {desc.get('opset')}, the shipped export has {OPSET}")
    inputs = {i["name"]: i for i in desc.get("inputs", [])}
    if tuple(inputs) != INPUTS:
        problems.append(f"inputs {list(inputs)}, expected {list(INPUTS)}")
    for name in INPUTS:
        spec = inputs.get(name)
        if spec and (spec["type"] != "int64" or len(spec["shape"]) != 2 or any(isinstance(d, int) for d in spec["shape"])):
            problems.append(f"input {name} is {spec['type']} {spec['shape']}, expected int64 with two dynamic axes")
    outputs = desc.get("outputs", [])
    if [o["name"] for o in outputs] != [OUTPUT]:
        problems.append(f"outputs {[o['name'] for o in outputs]}, expected ['{OUTPUT}']")
    elif isinstance(outputs[0]["shape"][0], int):
        problems.append(f"output {OUTPUT} has a fixed batch axis: {outputs[0]['shape']}")
    if desc.get("quantized"):
        weights = desc.get("quantized_weights", [])
        if len(weights) != 1 or weights[0]["type"] != "uint8":
            problems.append(f"quantized weights {weights}, the shipped export has one uint8 embedding table")
    return problems


def compare(a: list[float], b: list[float], threshold: float) -> dict:
    """Largest and mean absolute difference between two score lists, and
    how many rows are on different sides of the threshold."""
    if len(a) != len(b):
        raise ValueError(f"{len(a)} scores against {len(b)}")
    diffs = [abs(x - y) for x, y in zip(a, b)]
    return {
        "rows": len(diffs),
        "max_abs": max(diffs) if diffs else 0.0,
        "mean_abs": sum(diffs) / len(diffs) if diffs else 0.0,
        "flips": sum(1 for x, y in zip(a, b) if (x >= threshold) != (y >= threshold)),
    }


def _fmt_threshold(value) -> str:
    return "none" if value is None or (isinstance(value, float) and math.isinf(value)) else f"{value:.4f}"


def report_markdown(result: dict) -> str:
    """The parity report. ``result`` is what ``parity`` writes to JSON."""
    meta = result["meta"]
    threshold = meta["threshold"]
    lines = [
        "# Export parity report",
        "",
        f"PyTorch model: `{meta['model']}`. Export: `{meta['export']}`. "
        f"Scored with the service's windowing (window {meta['window']}, overlap {meta['overlap']}, "
        f"at most {meta['max_windows']} windows) and its scoring code, one window per run.",
        "",
        "## How far apart the scores are",
        "",
        f"Absolute difference of the attack probability, row by row. \"Flips\" are rows on different sides "
        f"of {threshold}, the threshold the proxy ships with.",
        "",
        "| Set | Rows | Pair | Max abs diff | Mean abs diff | Flips |",
        "|---|---|---|---|---|---|",
    ]
    for name, data in result["sets"].items():
        for pair, c in data["compare"].items():
            lines.append(
                f"| `{name}` | {c['rows']} | {pair} | {c['max_abs']:.6f} | {c['mean_abs']:.6f} | {c['flips']} |"
            )
    lines += [
        "",
        "## Recall and false-positive rate",
        "",
        f"As `finetune.py` measures them: at the lowest threshold with a false-positive rate of at most "
        f"{meta['fpr']:.3f} on the set's benign rows, and at {threshold}.",
        "",
        "| Set | Model | Attacks | Benign | Threshold | FPR | Recall | "
        f"Recall at {threshold} | FPR at {threshold} |",
        "|---|---|---|---|---|---|---|---|---|",
    ]
    for name, data in result["sets"].items():
        for model, m in data["measure"].items():
            lines.append(
                f"| `{name}` | {model} | {m['attacks']} | {m['benign']} | {_fmt_threshold(m['threshold'])} | "
                f"{m['fpr']:.3f} | {m['recall']:.3f} | {m['recall_at_shipped']:.3f} | {m['fpr_at_shipped']:.3f} |"
            )
    truncated = {n: d["truncated"] for n, d in result["sets"].items() if d.get("truncated")}
    lines += [""]
    if truncated:
        lines += [f"Rows with more windows than were scored: {truncated}.", ""]
    lines += [
        "These are the red-team sets, scored as they are. The proxy strips long hashes and keys before it asks",
        "the classifier, so its own sweep (`go run ./cmd/redteam -classifier ... -sweep`) can differ slightly.",
        "",
    ]
    return "\n".join(lines)


# ── Everything below needs torch, transformers, onnx or onnxruntime ────────


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def describe_onnx(path: Path) -> dict:
    """Opset, inputs, outputs and quantized weights of an ONNX file."""
    import onnx

    model = onnx.load(str(path), load_external_data=False)

    def dims(value):
        return [d.dim_param or d.dim_value for d in value.type.tensor_type.shape.dim]

    def kind(value):
        return onnx.helper.tensor_dtype_to_np_dtype(value.type.tensor_type.elem_type).name

    quantized = [
        {"name": t.name, "type": onnx.helper.tensor_dtype_to_np_dtype(t.data_type).name, "shape": list(t.dims)}
        for t in model.graph.initializer
        if t.data_type in (onnx.TensorProto.UINT8, onnx.TensorProto.INT8) and len(t.dims) >= 2
    ]
    ops: dict[str, int] = {}
    for node in model.graph.node:
        ops[node.op_type] = ops.get(node.op_type, 0) + 1
    return {
        "opset": next((o.version for o in model.opset_import if o.domain in ("", "ai.onnx")), None),
        "producer": f"{model.producer_name} {model.producer_version}".strip(),
        "inputs": [{"name": i.name, "type": kind(i), "shape": dims(i)} for i in model.graph.input],
        "outputs": [{"name": o.name, "type": kind(o), "shape": dims(o)} for o in model.graph.output],
        "quantized": bool(quantized),
        "quantized_weights": quantized,
        "quantize_ops": {k: ops[k] for k in ("DequantizeLinear", "DynamicQuantizeLinear", "MatMulInteger") if k in ops},
    }


def _session(path: Path, threads: int):
    import onnxruntime as ort

    opts = ort.SessionOptions()
    if threads > 0:
        opts.intra_op_num_threads = threads
    opts.inter_op_num_threads = 1
    return ort.InferenceSession(str(path), sess_options=opts, providers=["CPUExecutionProvider"])


def _wrap(model):
    import torch

    class Logits(torch.nn.Module):
        # Named ``m`` like the wrapper the shipped export was traced on.
        def __init__(self, m):
            super().__init__()
            self.m = m

        def forward(self, input_ids, attention_mask):
            return self.m(input_ids=input_ids, attention_mask=attention_mask).logits

    return Logits(model).eval()


def _sample_ids(tokenizer_path: Path, tokens: int) -> list[int]:
    from tokenizers import Tokenizer

    tok = Tokenizer.from_file(str(tokenizer_path))
    tok.no_padding()
    tok.no_truncation()
    enc = tok.encode("Please summarise the attached quarterly report for the board. " * 40)
    ids = list(enc.ids)
    # Keep the leading and trailing special tokens, cut the middle.
    return ids[: tokens - 1] + ids[-1:]


def export(model_dir: Path, out: Path, *, opset: int = OPSET, threads: int = 0, keep_fp32: bool = True) -> dict:
    """Write ``out`` in the layout the service loads; return what was made."""
    import numpy as np
    import torch
    from onnxruntime.quantization import QuantType, quantize_dynamic
    from transformers import AutoModelForSequenceClassification

    if threads > 0:
        torch.set_num_threads(threads)
    (out / "onnx").mkdir(parents=True, exist_ok=True)
    fp32_path, int8_path = out / FP32_FILE, out / INT8_FILE

    # Eager attention: the plainest graph to trace, with the same numbers.
    model = AutoModelForSequenceClassification.from_pretrained(
        str(model_dir), attn_implementation="eager", dtype=torch.float32
    )
    model.eval()
    wrapped = _wrap(model)
    ids = _sample_ids(model_dir / "tokenizer.json", TRACE_TOKENS)
    # Two rows, so the trace does not treat the batch axis as 1.
    input_ids = torch.tensor([ids, ids], dtype=torch.int64)
    attention_mask = torch.ones_like(input_ids)
    with torch.no_grad():
        torch.onnx.export(
            wrapped,
            (input_ids, attention_mask),
            str(fp32_path),
            input_names=list(INPUTS),
            output_names=[OUTPUT],
            dynamic_axes={
                "input_ids": {0: "batch", 1: "seq"},
                "attention_mask": {0: "batch", 1: "seq"},
                OUTPUT: {0: "batch"},
            },
            opset_version=opset,
            do_constant_folding=True,
            dynamo=False,
        )

    # The axes are only dynamic if the graph gives the right answer at a
    # length it was not traced at, short and long.
    session = _session(fp32_path, threads)
    worst = 0.0
    for length in (9, 700):
        probe = torch.tensor([_sample_ids(model_dir / "tokenizer.json", length)], dtype=torch.int64)
        with torch.no_grad():
            expected = wrapped(probe, torch.ones_like(probe)).numpy()
        got = session.run(None, {"input_ids": probe.numpy(), "attention_mask": np.ones_like(probe.numpy())})[0]
        worst = max(worst, float(np.abs(expected - got).max()))
    if worst > FP32_TOLERANCE:
        raise RuntimeError(f"the fp32 export differs from PyTorch by {worst:.2e} in the logits at untraced lengths")
    del session

    quantize_dynamic(
        str(fp32_path),
        str(int8_path),
        op_types_to_quantize=list(QUANTIZE_OPS),
        per_channel=False,
        reduce_range=False,
        weight_type=getattr(QuantType, QUANT_WEIGHT_TYPE),
    )

    for name in COPY_FILES:
        src = model_dir / name
        if not src.is_file():
            continue
        if name == "tokenizer.json":
            # A tokenizer saved after training carries the padding and
            # truncation it was trained with. The service turns both off;
            # the shipped file has neither, and neither has this copy.
            data = json.loads(src.read_text(encoding="utf-8"))
            data["truncation"] = None
            data["padding"] = None
            (out / name).write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
        else:
            shutil.copyfile(src, out / name)

    desc = describe_onnx(int8_path)
    files = [INT8_FILE] + ([FP32_FILE] if keep_fp32 else [])
    if not keep_fp32:
        fp32_path.unlink()
    import onnx
    import onnxruntime
    import transformers

    record = {
        "source": str(model_dir),
        "opset": opset,
        "quantization": {
            "function": "onnxruntime.quantization.quantize_dynamic",
            "op_types_to_quantize": list(QUANTIZE_OPS),
            "weight_type": QUANT_WEIGHT_TYPE,
            "per_channel": False,
            "reduce_range": False,
        },
        "fp32_max_logit_diff_untraced_lengths": worst,
        "int8": desc,
        "graph_problems": graph_problems(desc),
        "files": {
            f: {"bytes": (out / f).stat().st_size, "sha256": _sha256(out / f)}
            for f in files + [n for n in COPY_FILES if (out / n).is_file()]
        },
        "versions": {
            "torch": torch.__version__,
            "transformers": transformers.__version__,
            "onnx": onnx.__version__,
            "onnxruntime": onnxruntime.__version__,
        },
    }
    (out / "export.json").write_text(json.dumps(record, indent=2, ensure_ascii=False), encoding="utf-8")
    return record


class _TorchSession:
    """Answers ``run`` like an onnxruntime session, so the PyTorch model is
    scored by the same ``Classifier.score`` as the two exports."""

    def __init__(self, model) -> None:
        self._model = model.eval()

    def run(self, _outputs, feed):
        import torch

        with torch.no_grad():
            out = self._model(
                input_ids=torch.from_numpy(feed["input_ids"]),
                attention_mask=torch.from_numpy(feed["attention_mask"]),
            )
        return [out.logits.float().numpy()]


def parity(model_dir: Path, export_dir: Path, *, threads: int = 0, fpr: float | None = None) -> dict:
    """Score the evaluation sets three ways; return what the report shows."""
    import torch
    from transformers import AutoModelForSequenceClassification

    svc = service_module()
    ft = finetune_module()
    fpr = ft.DEFAULT_FPR if fpr is None else fpr
    if threads > 0:
        torch.set_num_threads(threads)

    int8 = svc.Classifier(model_dir=str(export_dir), threads=threads)
    if not int8.available:
        raise RuntimeError(f"the service could not load {export_dir}: {int8.reason}")

    def like(session, name):
        return svc.Classifier.from_parts(
            session,
            int8._tokenizer,
            name=name,
            inputs=INPUTS,
            attack_index=int8._attack_index,
            max_windows=int8.max_windows,
            window=int8.window,
            overlap=int8.overlap,
        )

    model = AutoModelForSequenceClassification.from_pretrained(str(model_dir), dtype=torch.float32)
    models = {"pytorch": like(_TorchSession(model), "pytorch")}
    if (export_dir / FP32_FILE).is_file():
        models["onnx-fp32"] = like(_session(export_dir / FP32_FILE, threads), "onnx-fp32")
    models["onnx-int8"] = int8

    pairs = [("pytorch", "onnx-int8"), ("pytorch", "onnx-fp32"), ("onnx-fp32", "onnx-int8")]
    sets = {}
    for name, rows in ft.evaluation_sets().items():
        texts = [r.text for r in rows]
        scored = {label: clf.score(texts) for label, clf in models.items()}
        scores = {label: [s.score for s in out] for label, out in scored.items()}
        sets[name] = {
            "compare": {
                f"{a} / {b}": compare(scores[a], scores[b], ft.SHIPPED_THRESHOLD)
                for a, b in pairs
                if a in scores and b in scores
            },
            "measure": {label: _finite(ft.measure(rows, s, fpr)) for label, s in scores.items()},
            "truncated": sum(s.truncated for s in scored["onnx-int8"]),
            "scores": {label: s for label, s in scores.items()},
            "ids": [r.id for r in rows],
        }
    return {
        "meta": {
            "model": str(model_dir),
            "export": str(export_dir),
            "threshold": ft.SHIPPED_THRESHOLD,
            "fpr": fpr,
            "window": int8.window,
            "overlap": int8.overlap,
            "max_windows": int8.max_windows,
        },
        "sets": sets,
    }


def _finite(m: dict) -> dict:
    out = {k: (None if isinstance(v, float) and math.isinf(v) else v) for k, v in m.items()}
    out.pop("by_category", None)
    return out


# ── Command line ────────────────────────────────────────────────────────────


def _model_dir(value: str) -> Path:
    path = Path(value)
    missing = [n for n in NEEDED_SOURCE_FILES if not (path / n).is_file()]
    if not path.is_dir() or missing:
        raise argparse.ArgumentTypeError(f"{value} is not a model directory (missing {', '.join(missing) or 'it'})")
    return path


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)

    exp = sub.add_parser("export", help="PyTorch model directory -> the directory the service loads")
    exp.add_argument("--model", required=True, type=_model_dir, help="what finetune.py run wrote to out/model")
    exp.add_argument("--out", required=True, type=Path, help="a new or empty directory")
    exp.add_argument("--opset", type=int, default=OPSET)
    exp.add_argument("--threads", type=int, default=0, help="torch and onnxruntime threads; 0 lets them choose")
    exp.add_argument("--drop-fp32", action="store_true", help=f"delete {FP32_FILE} after quantizing")
    exp.add_argument("--force", action="store_true", help="write into a directory that is not empty")
    exp.set_defaults(func=cmd_export)

    par = sub.add_parser("parity", help="score the evaluation sets with PyTorch, fp32 ONNX and int8 ONNX")
    par.add_argument("--model", required=True, type=_model_dir)
    par.add_argument("--export", required=True, type=Path, help="what export wrote")
    par.add_argument("--report", type=Path, default=None, help="default: parity.md in the export directory")
    par.add_argument("--fpr", type=float, default=None, help="default: finetune.py's DEFAULT_FPR")
    par.add_argument("--threads", type=int, default=0)
    par.set_defaults(func=cmd_parity)

    chk = sub.add_parser("check", help="whether the service can load a directory; runs no model")
    chk.add_argument("directory", type=Path)
    chk.set_defaults(func=cmd_check)
    return parser


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.command == "export":
        if args.out.resolve() == args.model.resolve():
            parser.error("--out must not be the model directory")
        if args.out.exists() and not args.out.is_dir():
            parser.error(f"--out {args.out} is a file")
        if args.out.is_dir() and any(args.out.iterdir()) and not args.force:
            parser.error(f"--out {args.out} is not empty (--force writes into it)")
    if getattr(args, "threads", 0) < 0:
        parser.error("--threads must be 0 or more")
    if getattr(args, "fpr", None) is not None and not 0.0 <= args.fpr < 1.0:
        parser.error("--fpr must be at least 0 and below 1")
    return args


def cmd_export(args) -> int:
    record = export(args.model, args.out, opset=args.opset, threads=args.threads, keep_fp32=not args.drop_fp32)
    problems = record["graph_problems"] + check_layout(args.out)
    for f, info in record["files"].items():
        print(f"{f}: {info['bytes']} bytes, sha256 {info['sha256']}")
    print(f"fp32 against PyTorch at untraced lengths: max logit difference {record['fp32_max_logit_diff_untraced_lengths']:.2e}")
    for p in problems:
        print(f"problem: {p}", file=sys.stderr)
    return 1 if problems else 0


def cmd_parity(args) -> int:
    problems = check_layout(args.export)
    if problems:
        for p in problems:
            print(f"problem: {p}", file=sys.stderr)
        return 1
    result = parity(args.model, args.export, threads=args.threads, fpr=args.fpr)
    report = args.report or (args.export / "parity.md")
    text = report_markdown(result)
    report.write_text(text, encoding="utf-8")
    report.with_suffix(".json").write_text(json.dumps(result, indent=2, ensure_ascii=False), encoding="utf-8")
    print(text)
    return 0


def cmd_check(args) -> int:
    problems = check_layout(args.directory)
    for p in problems:
        print(f"problem: {p}")
    if not problems:
        print(f"{args.directory}: the service loads {INT8_FILE} from it")
    return 1 if problems else 0


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
