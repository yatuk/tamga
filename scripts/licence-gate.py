#!/usr/bin/env python3
"""Judge the licences of Tamga's shipped dependencies against a policy file.

The dependency set and each licence come from scripts/third-party-licenses.py,
the generator of THIRD_PARTY_LICENSES.md: this script imports it and adds no
second way of reading a licence. The policy is .github/licence-policy.yml.

    python scripts/licence-gate.py
        Judge with the mode the policy file names (report or enforce).

    python scripts/licence-gate.py --mode enforce
        Exit 1 when something would be refused, whatever the file says.

    python scripts/licence-gate.py --pip analyzer=analyzer.json --pip classifier=classifier.json
        Read a Python component's licences from the output of
        `third-party-licenses.py dump-pip`, run inside the environment its
        requirements were installed into. Without it the metadata comes from
        the interpreter running this script and most packages are
        NOT-INSPECTED.

    python scripts/licence-gate.py --json findings.json
        Also write every judged dependency as JSON.

    python scripts/licence-gate.py --sbom-dir sboms --sbom-version v0.9.0
        Also write a CycloneDX 1.6 SBOM for each of the proxy, the analyzer
        and the classifier: the dependencies judged here, each with its
        licence and the category the policy gave it. They are written in
        report and enforce mode alike, before the exit code is decided.

Needs PyYAML for the policy file; everything else is the standard library.
No network is used. This is a check of declared licences against a list. It
is not legal advice.

Exit codes: 0 nothing refused, or report mode; 1 enforce mode and something
refused; 2 the policy file or the inventory could not be read, including, in
enforce mode, an ecosystem of which nothing could be read (the proxy's Go
modules, a console or SDK lockfile, a Python component's environment).
"""

from __future__ import annotations

import argparse
import fnmatch
import importlib.util
import json
import os
import re
import sys
from pathlib import Path
from urllib.parse import quote

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_POLICY = ROOT / ".github" / "licence-policy.yml"

# What the inventory writes when it has no licence id to give.
UNREADABLE = {"", "MISSING", "UNRECOGNISED", "NOT-INSPECTED", "UNKNOWN"}

# Worst first. An expression joined by AND is as bad as its worst part; one
# joined by OR is as good as its best alternative.
ORDER = ["deny", "unreadable", "unknown", "review", "allow"]

# Licence ids joined by AND, OR or WITH, as a CycloneDX `expression` needs.
SPDX_EXPRESSION = re.compile(r"[A-Za-z0-9.+-]+(?: (?:AND|OR|WITH) [A-Za-z0-9.+-]+)*")


def load_inventory():
    """Import third-party-licenses.py, whose file name is not a module name."""
    path = ROOT / "scripts" / "third-party-licenses.py"
    spec = importlib.util.spec_from_file_location("third_party_licenses", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


class PolicyError(Exception):
    pass


class Policy:
    def __init__(self, data: dict):
        if not isinstance(data, dict) or data.get("version") != 1:
            raise PolicyError("policy file must be a mapping with `version: 1`")
        self.mode = data.get("mode", "report")
        if self.mode not in ("report", "enforce"):
            raise PolicyError("`mode` must be report or enforce")
        self.unreadable = data.get("unreadable", "fail")
        if self.unreadable not in ("fail", "warn"):
            raise PolicyError("`unreadable` must be fail or warn")
        self.judge_outside = bool(data.get("judge_go_modules_outside_binaries", False))
        # (category, pattern in upper case, reason); deny is looked at first.
        self.rules: list[tuple[str, str, str]] = []
        seen: dict[str, str] = {}
        for category in ("deny", "review", "allow"):
            for i, group in enumerate(data.get(category) or []):
                if not isinstance(group, dict) or not group.get("licences"):
                    raise PolicyError(f"`{category}[{i}]` needs `licences`")
                reason = " ".join(str(group.get("reason", "")).split())
                if not reason:
                    raise PolicyError(f"`{category}[{i}]` needs a `reason`")
                for lic in group["licences"]:
                    pat = str(lic).strip().upper()
                    if pat in seen and seen[pat] != category:
                        raise PolicyError(f"`{lic}` is in both `{seen[pat]}` and `{category}`")
                    seen[pat] = category
                    self.rules.append((category, pat, reason))
        self.decisions: list[dict] = []
        for i, d in enumerate(data.get("decisions") or []):
            if not isinstance(d, dict) or not d.get("package") or not d.get("ecosystem"):
                raise PolicyError(f"`decisions[{i}]` needs `ecosystem` and `package`")
            if d.get("decision") not in ("pending", "allow", "deny"):
                raise PolicyError(f"`decisions[{i}].decision` must be pending, allow or deny")
            self.decisions.append(d)

    def judge_id(self, lic: str) -> tuple[str, str]:
        """Category and reason for one licence id."""
        u = lic.strip().upper()
        if u in UNREADABLE:
            return "unreadable", "the licence could not be read"
        # `MIT+` and `GPL-2.0+` are older spellings of "or later".
        candidates = [u] + ([u[:-1], u[:-1] + "-OR-LATER"] if u.endswith("+") else [])
        for category, pat, reason in self.rules:
            if any(fnmatch.fnmatchcase(c, pat) for c in candidates):
                return category, reason
        return "unknown", "this licence id is in none of the policy's sets"

    def judge_expression(self, expr: str) -> tuple[str, list[tuple[str, str, str]]]:
        """Category of an SPDX-like expression, and the parts that decided it.

        Parentheses are not nested in anything the inventory produces beyond
        one level, so they are dropped and OR binds looser than AND.
        """
        cleaned = re.sub(r"[()]", " ", expr or "").strip()
        if cleaned.upper() in UNREADABLE:
            return "unreadable", [("unreadable", expr or "MISSING", "the licence could not be read")]
        best: tuple[str, list[tuple[str, str, str]]] | None = None
        for alternative in re.split(r"\s+OR\s+", cleaned, flags=re.I):
            parts = [p.strip() for p in re.split(r"\s+(?:AND|WITH)\s+|\s*[/,;]\s*", alternative, flags=re.I) if p.strip()]
            judged = [(*self.judge_id(p), p) for p in parts] or [("unreadable", "the licence could not be read", expr)]
            worst = min((j[0] for j in judged), key=ORDER.index)
            detail = [(c, p, r) for c, r, p in judged if c == worst]
            if best is None or ORDER.index(worst) > ORDER.index(best[0]):
                best = (worst, detail)
        assert best is not None
        return best

    def decision_for(self, row: dict, review_ids: list[str]) -> dict | None:
        """The `decisions` entry that covers this package, if there is one.

        An entry covers a package when its glob matches and it names every
        licence id that put the package in review: a package whose licence
        changes to something the entry did not name is in review again.
        """
        for d in self.decisions:
            if d["ecosystem"] != row["ecosystem"]:
                continue
            if d.get("component") and d["component"] != row["component"]:
                continue
            if not fnmatch.fnmatchcase(row["name"], str(d["package"])):
                continue
            named = {t.upper() for t in re.split(r"\s+(?:OR|AND|WITH)\s+|\s*[/,;]\s*", str(d.get("licence", "")), flags=re.I) if t.strip()}
            if named and not {r.upper() for r in review_ids} <= named:
                continue
            return d
        return None


def collect(inv, pip_envs: dict[str, dict], policy: Policy) -> tuple[list[dict], list[str], list[str]]:
    """Every dependency to judge, notes about what was left out, and the
    ecosystems or components that could not be read at all.

    A package that cannot be read is a NOT-INSPECTED row and is judged. An
    ecosystem that cannot be read gives no rows to judge, so it is returned
    separately and fails the gate on its own (see render).
    """
    rows: list[dict] = []
    notes: list[str] = []
    unread: list[str] = []

    go_rows, go_info = inv.go_modules()
    if not go_rows:
        unread.append(f"go proxy: no modules read ({go_info})")
    elif not any(r["linked"] for r in go_rows):
        unread.append("go proxy: no module is linked into the binaries, so `go list -deps ./cmd/...` read nothing")
    outside = 0
    for r in go_rows:
        shipped = r["linked"]
        if not shipped and not policy.judge_outside:
            outside += 1
            continue
        rows.append({"ecosystem": "go", "component": "proxy", "name": r["name"], "version": r["version"],
                     "licence": r["license"] or "MISSING", "source": r["source"]})
    if outside:
        notes.append(f"go: {outside} modules are in the module graph only (tests, tools), not in the binaries; not judged")

    for component, rel in (("dashboard", "dashboard"), ("sdk-typescript", "sdk/typescript/tamga-client")):
        project = ROOT / rel
        if not (project / "package-lock.json").is_file():
            unread.append(f"npm {component}: no package-lock.json in {rel}")
            continue
        npm_rows, info = inv.npm_packages(project)
        seen = set()
        for r in npm_rows:
            key = (r["name"], r["version"], r["license"])
            if key in seen:
                continue
            seen.add(key)
            rows.append({"ecosystem": "npm", "component": component, "name": r["name"], "version": r["version"],
                         "licence": r["license"] or "MISSING", "source": r["source"]})
        notes.append(f"npm {component}: {info['skipped_dev']} development entries of the lockfile left out")

    evidence = inv.checked_python_evidence()
    components = (
        ("analyzer", lambda: inv.requirements_lines(ROOT / "analyzer/requirements.txt")),
        ("classifier", lambda: inv.requirements_lines(ROOT / "classifier/requirements.txt")),
        ("sdk-python", lambda: inv.pyproject_dependencies(ROOT / "sdk/python/pyproject.toml")),
    )
    local_env = None
    for component, load in components:
        if component in pip_envs:
            env = pip_envs[component]
            if not env.get("distributions"):
                unread.append(f"python {component}: the --pip file lists no distributions")
                continue
        else:
            if local_env is None:
                local_env = inv.dump_pip()
            env = local_env
            if not env.get("distributions"):
                unread.append(f"python {component}: the interpreter running this script lists no distributions")
                continue
            notes.append(f"python {component}: licences read from the interpreter running this script, not from "
                         f"an environment built for it (pass --pip {component}=FILE)")
        direct, trans, missing = inv.python_component(load(), env,
                                                      None if component in pip_envs else evidence.get(component))
        for r in direct + trans:
            rows.append({"ecosystem": "python", "component": component, "name": r["name"],
                         "version": r.get("version", ""), "licence": r["license"] or "MISSING",
                         "source": r.get("source", "")})
        if missing and component in pip_envs:
            # Not installed in an environment built from the component's own
            # requirements: an optional or other-platform dependency, not
            # something the component ships.
            notes.append(f"python {component}: required but not installed where inspected, so not judged: "
                         + ", ".join(sorted(missing)))
        elif missing:
            # The interpreter running this script says nothing about what the
            # component installs, so a gap here is a gap in the inventory.
            notes.append(f"python {component}: required but not installed where inspected: "
                         + ", ".join(sorted(missing)))
            for name in missing:
                rows.append({"ecosystem": "python", "component": component, "name": name,
                             "version": "", "licence": "NOT-INSPECTED",
                             "source": "required by an installed distribution but not installed"})
    return rows, notes, unread


def judge(rows: list[dict], policy: Policy) -> list[dict]:
    out = []
    for row in rows:
        category, detail = policy.judge_expression(row["licence"])
        item = dict(row, category=category, why="; ".join(sorted({f"{p}: {r}" for _, p, r in detail})))
        item["decision"] = ""
        if category == "review":
            d = policy.decision_for(row, [p for _, p, _ in detail])
            if d is not None:
                item["decision"] = d["decision"]
                item["note"] = " ".join(str(d.get("note", "")).split())
        # What enforce mode does with it.
        if category == "allow":
            item["outcome"] = "pass"
        elif category == "review":
            item["outcome"] = {"allow": "pass", "deny": "fail"}.get(item["decision"], "fail")
        elif category == "unreadable":
            item["outcome"] = "fail" if policy.unreadable == "fail" else "warn"
        else:
            item["outcome"] = "fail"
        out.append(item)
    return out


def label(item: dict) -> str:
    return f"{item['ecosystem']} {item['component']}: {item['name']} {item['version']}".rstrip()


def explain(item: dict) -> str:
    if item["category"] == "review":
        state = {"": "no decision recorded", "pending": "decision pending",
                 "allow": "allowed by a recorded decision", "deny": "refused by a recorded decision"}[item["decision"]]
        return f"in review, {state}"
    return {"deny": "licence is in the deny set", "unknown": "licence is in no set of the policy",
            "unreadable": "licence could not be read", "allow": "allowed"}[item["category"]]


def render(items: list[dict], notes: list[str], policy: Policy, mode: str,
           unread: list[str] | None = None) -> tuple[str, int]:
    unread = unread or []
    failing = [i for i in items if i["outcome"] == "fail"]
    warning = [i for i in items if i["outcome"] == "warn"]
    decided = [i for i in items if i["category"] == "review" and i["outcome"] == "pass"]
    lines = [f"Licence gate, mode: {mode}",
             f"Judged {len(items)} dependencies: {sum(1 for i in items if i['category'] == 'allow')} allowed, "
             f"{len(decided)} allowed by a recorded decision, {len(failing)} would be refused, {len(warning)} warned."]
    for n in notes:
        lines.append(f"  note: {n}")
    for title, group in (("Would be refused" if mode == "report" else "Refused", failing), ("Warnings", warning)):
        if not group:
            continue
        lines += ["", f"{title}:"]
        for i in sorted(group, key=lambda i: (ORDER.index(i["category"]), label(i).lower())):
            lines.append(f"  {label(i)}: {i['licence']} ({explain(i)})")
        # Each reason and each note once, not once per package.
        reasons = sorted({i["why"] for i in group if i["category"] != "unreadable"})
        if reasons:
            lines += ["", "  What the policy says about these licences:"] + [f"    {r}" for r in reasons]
        policy_notes = sorted({i["note"] for i in group if i.get("note")})
        if policy_notes:
            lines += ["", "  Notes recorded with the pending decisions:"] + [f"    {n}" for n in policy_notes]
    if unread:
        # Nothing of these was judged, so a pass would say nothing about them.
        lines += ["", "Not read at all, so not judged:"] + [f"  {u}" for u in unread]
    stale = unused_decisions(items, policy)
    if stale:
        lines += ["", "Decisions in the policy that match no dependency any more (remove them):"]
        lines += [f"  {d['ecosystem']} {d.get('component', '*')}: {d['package']} ({d.get('licence', '')})" for d in stale]
    lines.append("")
    if mode == "report":
        lines.append("Report mode: exit 0. Set `mode: enforce` in the policy file to refuse the above."
                     if failing or unread else "Report mode: nothing would be refused.")
        return "\n".join(lines), 0
    if unread:
        lines.append(f"Enforce mode: failing; {len(unread)} part(s) of the inventory could not be read (listed above).")
        return "\n".join(lines), 2
    lines.append("Enforce mode: failing." if failing else "Enforce mode: nothing refused.")
    return "\n".join(lines), 1 if failing else 0


def unused_decisions(items: list[dict], policy: Policy) -> list[dict]:
    out = []
    for d in policy.decisions:
        hit = any(i["ecosystem"] == d["ecosystem"]
                  and (not d.get("component") or d["component"] == i["component"])
                  and fnmatch.fnmatchcase(i["name"], str(d["package"])) for i in items)
        if not hit:
            out.append(d)
    return out


def summary_markdown(items: list[dict], mode: str, unread: list[str] | None = None) -> str:
    rows = [i for i in items if i["outcome"] != "pass"]
    out = [f"### Dependency licences ({mode} mode)", "",
           f"{len(items)} dependencies judged, {len(rows)} not passing.", ""]
    if unread:
        out += ["Not read at all, so not judged:", ""] + [f"- {u}" for u in unread] + [""]
    if rows:
        out += ["| Dependency | Licence | Finding |", "|---|---|---|"]
        for i in sorted(rows, key=lambda i: (ORDER.index(i["category"]), label(i).lower())):
            out.append(f"| `{label(i)}` | {i['licence'].replace('|', '/')} | {explain(i)} |")
        out.append("")
    if mode == "report":
        out.append("Report mode: this job does not fail. The policy is `.github/licence-policy.yml`.")
    return "\n".join(out) + "\n"


def write_sboms(items: list[dict], directory: Path, version: str) -> list[Path]:
    """Write checked CycloneDX inventories for the shipped service dependencies."""
    directory.mkdir(parents=True, exist_ok=True)
    written = []
    for component in ("proxy", "analyzer", "classifier"):
        components = []
        seen = set()
        for item in sorted((i for i in items if i["component"] == component),
                           key=lambda i: (i["name"].lower(), i["version"])):
            key = (item["ecosystem"], item["name"], item["version"])
            if key in seen:
                continue
            seen.add(key)
            name = item["name"]
            # The inventory appends " (differs from the pin)" to a version.
            pkg_version = item["version"].split(" ", 1)[0]
            if item["ecosystem"] == "go":
                package_url = f"pkg:golang/{quote(name, safe='/')}"
            else:
                # The purl spec's pypi names are lower case with `-` for `_`.
                package_url = f"pkg:pypi/{quote(name.lower().replace('_', '-'), safe='')}"
            if pkg_version:
                package_url += f"@{quote(pkg_version, safe='.+-')}"
            dependency = {
                "type": "library",
                "bom-ref": package_url,
                "name": name,
                "version": pkg_version,
                "purl": package_url,
                "properties": [
                    {"name": "tamga:licence-category", "value": item["category"]},
                    {"name": "tamga:licence-source", "value": item["source"]},
                ],
            }
            if item["licence"] not in UNREADABLE:
                if SPDX_EXPRESSION.fullmatch(item["licence"]):
                    dependency["licenses"] = [{"expression": item["licence"]}]
                else:
                    # "BSD (unspecified)" and the like are not SPDX expressions.
                    dependency["licenses"] = [{"license": {"name": item["licence"]}}]
            components.append(dependency)
        document = {
            "bomFormat": "CycloneDX",
            "specVersion": "1.6",
            "version": 1,
            "metadata": {
                "component": {
                    "type": "application",
                    "name": f"tamga-{component}",
                    "version": version,
                },
                "properties": [{"name": "tamga:licence-policy", "value": ".github/licence-policy.yml"}],
            },
            "components": components,
        }
        path = directory / f"tamga-{component}_{version}.cdx.json"
        path.write_text(json.dumps(document, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        written.append(path)
    return written


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--policy", type=Path, default=DEFAULT_POLICY)
    ap.add_argument("--mode", choices=("report", "enforce"), help="override the policy file's mode")
    ap.add_argument("--pip", action="append", default=[], metavar="NAME=FILE",
                    help="NAME is analyzer, classifier or sdk-python; FILE is the output of third-party-licenses.py dump-pip")
    ap.add_argument("--json", type=Path, help="write every judged dependency to this file")
    ap.add_argument("--sbom-dir", type=Path,
                    help="write checked CycloneDX SBOMs for the proxy, analyzer and classifier")
    ap.add_argument("--sbom-version", default="snapshot", help="component version written into the SBOMs")
    args = ap.parse_args()

    try:
        import yaml
    except ImportError:
        print("licence-gate: PyYAML is needed to read the policy file (pip install pyyaml)", file=sys.stderr)
        return 2
    try:
        policy = Policy(yaml.safe_load(args.policy.read_text(encoding="utf-8")))
    except (OSError, yaml.YAMLError, PolicyError) as exc:
        print(f"licence-gate: {args.policy}: {exc}", file=sys.stderr)
        return 2
    mode = args.mode or policy.mode

    envs = {}
    for item in args.pip:
        name, _, file = item.partition("=")
        if name not in ("analyzer", "classifier", "sdk-python") or not file:
            print(f"licence-gate: --pip {item}: expected analyzer|classifier|sdk-python=FILE", file=sys.stderr)
            return 2
        try:
            envs[name] = json.loads(Path(file).read_text(encoding="utf-8-sig"))
        except (OSError, ValueError) as exc:
            print(f"licence-gate: --pip {item}: {exc}", file=sys.stderr)
            return 2

    try:
        inv = load_inventory()
        rows, notes, unread = collect(inv, envs, policy)
    except Exception as exc:  # the inventory reads files and runs `go list`
        print(f"licence-gate: could not read the dependency inventory: {exc}", file=sys.stderr)
        return 2
    if not rows:
        print("licence-gate: the inventory is empty; refusing to call that a pass", file=sys.stderr)
        return 2

    items = judge(rows, policy)
    text, code = render(items, notes, policy, mode, unread)
    print(text)
    if args.json:
        args.json.write_text(json.dumps(items, indent=1, sort_keys=True), encoding="utf-8")
    if args.sbom_dir:
        write_sboms(items, args.sbom_dir, args.sbom_version)
    step_summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if step_summary:
        with open(step_summary, "a", encoding="utf-8") as fh:
            fh.write(summary_markdown(items, mode, unread))
    return code


if __name__ == "__main__":
    sys.exit(main())
