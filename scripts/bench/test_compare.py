"""Tests of the scoring in compare.py, on tiny fake data. No network, no
model: only the standard library is imported."""

import importlib.util
import math
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("bench_compare", HERE / "compare.py")
compare = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = compare
spec.loader.exec_module(compare)

Item, Result, DataSet = compare.Item, compare.Result, compare.DataSet


def fake_set(name, rows):
    """rows: (label, flagged, score) per item."""
    items = [Item(f"{name}-{n}", f"text {name} {n}", label) for n, (label, _, _) in enumerate(rows)]
    results = {i.id: Result(flagged, score, 1.0) for i, (_, flagged, score) in zip(items, rows)}
    ds = DataSet(name=name, source="example.invalid", revision="0", split="test", licence="none",
                 items=items, language="en")
    return ds, results


class DedupeTest(unittest.TestCase):
    def test_repeated_texts_are_kept_once_and_conflicts_dropped(self):
        rows, dup, conflicts = compare.dedupe([("a", 1), ("a", 1), ("b", 0), ("c", 1), ("c", 0)])
        self.assertEqual(rows, [("a", 1), ("b", 0)])
        self.assertEqual(dup, 2)
        self.assertEqual(conflicts, 1)

    def test_overlap_ignores_case_and_whitespace(self):
        self.assertEqual(compare.overlap(["Hello  World", "other"], ["hello world"]), 1)


class MetricsTest(unittest.TestCase):
    def test_recall_and_fpr_at_the_default(self):
        ds, res = fake_set("s", [(1, True, 0.9), (1, False, 0.2), (0, True, 0.7), (0, False, 0.1),
                                 (0, False, 0.0), (0, False, 0.0)])
        m = compare.set_metrics(ds.items, res, None)
        self.assertEqual((m["attacks"], m["benign"]), (2, 4))
        self.assertAlmostEqual(m["recall"], 0.5)
        self.assertAlmostEqual(m["fpr"], 0.25)
        self.assertNotIn("recall_at_fixed_fpr", m)

    def test_the_fixed_fpr_threshold_comes_from_every_benign_row(self):
        a, ra = fake_set("a", [(1, False, 0.95)] + [(0, False, 0.01 * n) for n in range(50)])
        b, rb = fake_set("b", [(1, False, 0.60)] + [(0, False, 0.9)] + [(0, False, 0.0)] * 49)
        res = {**ra, **rb}
        # 100 benign rows, 1% allowed: one row may score at or above it.
        t = compare.pooled_threshold([a, b], res, 0.01)
        self.assertEqual(t, 0.9)
        self.assertAlmostEqual(compare.set_metrics(a.items, res, t)["recall_at_fixed_fpr"], 1.0)
        mb = compare.set_metrics(b.items, res, t)
        self.assertAlmostEqual(mb["recall_at_fixed_fpr"], 0.0)
        self.assertAlmostEqual(mb["fpr_at_fixed_fpr"], 1 / 50)

    def test_no_threshold_without_scores(self):
        ds, res = fake_set("s", [(1, True, None), (0, False, None)])
        self.assertIsNone(compare.pooled_threshold([ds], res, 0.01))

    def test_too_few_benign_rows_put_the_threshold_above_the_highest(self):
        ds, res = fake_set("s", [(0, False, 0.3), (0, False, 0.5), (1, False, 0.5)])
        t = compare.pooled_threshold([ds], res, 0.01)
        self.assertGreater(t, 0.5)
        self.assertEqual(compare.set_metrics(ds.items, res, t)["fpr_at_fixed_fpr"], 0.0)

    def test_latency(self):
        res = {str(n): Result(False, None, float(n)) for n in range(1, 101)}
        lat = compare.latency(res)
        self.assertEqual(lat["median_ms"], 50.5)
        self.assertEqual(lat["p95_ms"], 95.0)
        self.assertEqual(compare.latency({}), {})


class InjectionProbabilityTest(unittest.TestCase):
    def test_from_logits_keeps_rows_apart_near_one(self):
        p = compare.injection_probability
        self.assertAlmostEqual(p([{"label": "SAFE", "score": 0.0}, {"label": "INJECTION", "score": 0.0}]), 0.5)
        a = p([{"label": "SAFE", "score": -8.0}, {"label": "INJECTION", "score": 8.0}])
        b = p([{"label": "SAFE", "score": -8.5}, {"label": "INJECTION", "score": 8.5}])
        self.assertLess(a, b)
        self.assertLess(b, 1.0)
        self.assertLess(p([{"label": "safe", "score": 900.0}, {"label": "injection", "score": -900.0}]), 1e-300)


class TamgaWithClassifierTest(unittest.TestCase):
    base = {"injection_action": "PASS", "ask_classifier": True, "prepared": "some text here",
            "signal": "", "classifier_action": "BLOCK"}

    def test_the_rules_decide_first(self):
        self.assertTrue(compare.tamga_with_classifier({**self.base, "injection_action": "BLOCK"}, 0.0))

    def test_threshold_and_signal_threshold(self):
        self.assertFalse(compare.tamga_with_classifier(self.base, 0.95))
        self.assertTrue(compare.tamga_with_classifier(self.base, 0.98))
        self.assertTrue(compare.tamga_with_classifier({**self.base, "signal": "persona_frame"}, 0.95))
        self.assertFalse(compare.tamga_with_classifier({**self.base, "signal": "persona_frame"}, 0.89))

    def test_not_asked_or_over_max_chars(self):
        self.assertFalse(compare.tamga_with_classifier({**self.base, "ask_classifier": False}, 1.0))
        long = {**self.base, "prepared": "x" * (compare.TAMGA_MAX_CHARS + 1)}
        self.assertFalse(compare.tamga_with_classifier(long, 1.0))


class MarkdownTest(unittest.TestCase):
    def test_table_has_a_row_per_set_and_system(self):
        ds, res = fake_set("s", [(1, True, 0.9), (0, False, 0.1)])
        report = {
            "fpr_target": 0.01, "pooled_benign": 1, "datasets": [{"name": "s"}],
            "systems": ["a", "b", "c"], "labels": {"a": "A", "b": "B", "c": "C"},
            "results": {"a": {"s": compare.set_metrics(ds.items, res, 0.5)},
                        "b": {"s": compare.set_metrics(ds.items, res, None)}},
            "thresholds": {"a": 0.5, "b": None}, "latency": {"a": compare.latency(res)},
        }
        md = compare.markdown(report)
        self.assertIn("| `s` | 1 | 1 | A | 1.000 | 0.000 | 1.000 | 0.000 |", md)
        self.assertIn("| `s` | 1 | 1 | B | 1.000 | 0.000 | — | — |", md)
        self.assertNotIn("| C |", md)
        self.assertIn("| A | 0.500000 | 1.000 | 1.000 |", md)
        self.assertTrue(math.isfinite(report["thresholds"]["a"]))


if __name__ == "__main__":
    unittest.main()
