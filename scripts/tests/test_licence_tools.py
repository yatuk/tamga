import hashlib
import importlib.util
import json
import shutil
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def load(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


inventory = load("third_party_licenses", ROOT / "scripts" / "third-party-licenses.py")
gate = load("licence_gate", ROOT / "scripts" / "licence-gate.py")


def real_policy():
    import yaml

    return gate.Policy(yaml.safe_load((ROOT / ".github" / "licence-policy.yml").read_text(encoding="utf-8")))


class RecordedEvidenceTests(unittest.TestCase):
    def test_classifier_records_resolve_the_direct_and_transitive_set(self):
        evidence = inventory.checked_python_evidence()["classifier"]
        direct, transitive, missing = inventory.python_component(
            ["onnxruntime>=1.20,<2", "tokenizers>=0.21,<1"],
            {"distributions": {}},
            evidence,
        )
        found = {row["name"]: row for row in direct}
        self.assertEqual(found["onnxruntime"]["license"], "MIT")
        self.assertEqual(found["tokenizers"]["license"], "Apache-2.0")
        self.assertIn("recorded wheel metadata", found["onnxruntime"]["source"])
        names = {row["name"] for row in transitive}
        self.assertIn("huggingface_hub", names)
        self.assertIn("flatbuffers", names)
        self.assertEqual(missing, [])

    def test_an_installed_distribution_wins_over_a_record(self):
        evidence = {"onnxruntime": {"name": "onnxruntime", "version": "1.0", "license": "MIT",
                                    "source": "recorded", "requires": []}}
        env = {"distributions": {"onnxruntime": {"name": "onnxruntime", "version": "2.0",
                                                  "license": "Apache-2.0", "source": "installed",
                                                  "requires": []}}}
        direct, _, _ = inventory.python_component(["onnxruntime"], env, evidence)
        self.assertEqual((direct[0]["version"], direct[0]["source"]), ("2.0", "installed"))

    def test_every_recorded_licence_is_allowed_by_the_policy(self):
        policy = real_policy()
        for component, records in inventory.checked_python_evidence().items():
            for key, record in records.items():
                category, _ = policy.judge_expression(record["license"])
                self.assertEqual(category, "allow", f"{component} {key}: {record['license']}")

    def test_an_unavailable_transitive_dependency_is_reported(self):
        direct, transitive, missing = inventory.python_component(
            ["root-package==1"],
            {
                "distributions": {
                    "root-package": {
                        "name": "root-package",
                        "version": "1",
                        "license": "MIT",
                        "source": "test metadata",
                        "requires": ["missing-child>=1"],
                    }
                }
            },
        )
        self.assertEqual(len(direct), 1)
        self.assertEqual(transitive, [])
        self.assertEqual(missing, ["missing-child"])


@unittest.skipUnless(shutil.which("go"), "go is not on PATH")
class GoInventoryTests(unittest.TestCase):
    def test_reading_the_modules_leaves_go_sum_and_go_mod_alone(self):
        files = [ROOT / "proxy" / "go.sum", ROOT / "proxy" / "go.mod"]
        before = [hashlib.sha256(f.read_bytes()).hexdigest() for f in files]
        rows, info = inventory.go_modules()
        after = [hashlib.sha256(f.read_bytes()).hexdigest() for f in files]
        self.assertTrue(rows, info)
        self.assertEqual(before, after)


class FakeInventory:
    """Stands in for third-party-licenses.py in gate.collect."""

    def __init__(self, go_rows=None, missing=None):
        self.go_rows = go_rows or []
        self.missing = missing or []
        self.evidence_seen: dict[str, object] = {}
        self._components = iter(("analyzer", "classifier", "sdk-python"))

    def go_modules(self):
        return self.go_rows, "test go"

    @staticmethod
    def npm_packages(_project):
        return [], {"skipped_dev": 0}

    @staticmethod
    def checked_python_evidence():
        return {"analyzer": {"x": {}}, "classifier": {"y": {}}}

    @staticmethod
    def requirements_lines(_path):
        return []

    @staticmethod
    def pyproject_dependencies(_path):
        return []

    @staticmethod
    def dump_pip():
        return {"distributions": {}}

    def python_component(self, _lines, _env, evidence=None):
        self.evidence_seen[next(self._components)] = evidence
        return [], [], list(self.missing)


class PolicyAndSbomTests(unittest.TestCase):
    def policy(self):
        return gate.Policy(
            {
                "version": 1,
                "mode": "enforce",
                "unreadable": "fail",
                "allow": [{"licences": ["MIT", "Apache-2.0"], "reason": "test allow"}],
                "review": [{"licences": ["LGPL-3.0-or-later"], "reason": "test review"}],
                "deny": [{"licences": ["Proprietary"], "reason": "test deny"}],
                "decisions": [{"ecosystem": "npm", "package": "pending-*", "licence": "LGPL-3.0-or-later",
                               "decision": "pending"}],
            }
        )

    def test_unknown_unreadable_and_pending_licences_fail_in_enforce_mode(self):
        rows = [
            {"ecosystem": "python", "component": "classifier", "name": "unknown",
             "version": "1", "licence": "New-License", "source": "test"},
            {"ecosystem": "python", "component": "classifier", "name": "missing",
             "version": "", "licence": "NOT-INSPECTED", "source": "test"},
            {"ecosystem": "npm", "component": "dashboard", "name": "pending-lib",
             "version": "1", "licence": "LGPL-3.0-or-later", "source": "test"},
        ]
        items = gate.judge(rows, self.policy())
        self.assertEqual([item["category"] for item in items], ["unknown", "unreadable", "review"])
        self.assertTrue(all(item["outcome"] == "fail" for item in items))
        _, code = gate.render(items, [], self.policy(), "enforce")
        self.assertEqual(code, 1)
        _, code = gate.render(gate.judge(rows[:1], self.policy()), [], self.policy(), "report")
        self.assertEqual(code, 0)

    def test_go_module_not_linked_into_a_binary_is_not_judged(self):
        fake = FakeInventory(go_rows=[
            {"name": "example.invalid/runtime", "version": "v1.0.0", "license": "MIT",
             "source": "LICENSE", "direct": True, "linked": True},
            {"name": "example.invalid/test-only", "version": "v1.0.0", "license": "MIT",
             "source": "LICENSE", "direct": True, "linked": False},
        ])
        rows, notes = gate.collect(fake, {}, self.policy())
        go_names = [row["name"] for row in rows if row["ecosystem"] == "go"]
        self.assertEqual(go_names, ["example.invalid/runtime"])
        self.assertTrue(any("not in the binaries" in note for note in notes))

    def test_records_are_not_used_for_a_component_given_with_pip(self):
        fake = FakeInventory()
        gate.collect(fake, {"classifier": {"distributions": {}}}, self.policy())
        self.assertIsNone(fake.evidence_seen["classifier"])
        self.assertEqual(fake.evidence_seen["analyzer"], {"x": {}})

    def test_a_missing_transitive_dependency_is_judged_only_without_pip(self):
        fake = FakeInventory(missing=["absent"])
        rows, _ = gate.collect(fake, {"classifier": {"distributions": {}}}, self.policy())
        judged = {(row["component"], row["name"]) for row in rows if row["ecosystem"] == "python"}
        self.assertIn(("analyzer", "absent"), judged)
        self.assertNotIn(("classifier", "absent"), judged)

    def test_cyclonedx_uses_the_judged_inventory(self):
        rows = [
            {"ecosystem": "python", "component": "classifier", "name": "tokenizers",
             "version": "0.23.3", "licence": "Apache-2.0", "source": "wheel METADATA"},
            {"ecosystem": "python", "component": "classifier", "name": "Typing_Extensions",
             "version": "4.16.0 (differs from the pin)", "licence": "MIT", "source": "test"},
            {"ecosystem": "python", "component": "analyzer", "name": "vague",
             "version": "1", "licence": "BSD (unspecified)", "source": "test"},
            {"ecosystem": "go", "component": "proxy", "name": "example.invalid/Mod",
             "version": "v1.2.3", "licence": "MIT", "source": "LICENSE"},
        ]
        items = gate.judge(rows, self.policy())
        with tempfile.TemporaryDirectory() as temp:
            paths = gate.write_sboms(items, Path(temp), "v0.9.0")
            documents = {path.name: json.loads(path.read_text(encoding="utf-8")) for path in paths}
        self.assertEqual(sorted(documents), ["tamga-analyzer_v0.9.0.cdx.json", "tamga-classifier_v0.9.0.cdx.json",
                                             "tamga-proxy_v0.9.0.cdx.json"])
        classifier = documents["tamga-classifier_v0.9.0.cdx.json"]
        self.assertEqual((classifier["bomFormat"], classifier["specVersion"]), ("CycloneDX", "1.6"))
        self.assertEqual(classifier["metadata"]["component"]["version"], "v0.9.0")
        by_name = {c["name"]: c for c in classifier["components"]}
        self.assertEqual(by_name["tokenizers"]["licenses"], [{"expression": "Apache-2.0"}])
        self.assertEqual(by_name["tokenizers"]["properties"][0], {"name": "tamga:licence-category", "value": "allow"})
        self.assertEqual(by_name["Typing_Extensions"]["purl"], "pkg:pypi/typing-extensions@4.16.0")
        self.assertEqual(by_name["Typing_Extensions"]["version"], "4.16.0")
        vague = documents["tamga-analyzer_v0.9.0.cdx.json"]["components"][0]
        self.assertEqual(vague["licenses"], [{"license": {"name": "BSD (unspecified)"}}])
        proxy = documents["tamga-proxy_v0.9.0.cdx.json"]["components"][0]
        self.assertEqual(proxy["purl"], "pkg:golang/example.invalid/Mod@v1.2.3")


if __name__ == "__main__":
    unittest.main()
