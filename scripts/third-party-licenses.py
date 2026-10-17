#!/usr/bin/env python3
"""Generate THIRD_PARTY_LICENSES.md from what is already on disk.

No network is used. Standard library only (Python 3.11+).

    python scripts/third-party-licenses.py generate
        Rewrite THIRD_PARTY_LICENSES.md at the repository root.

    python scripts/third-party-licenses.py generate --check
        Exit 1 when the file on disk differs from what would be generated.
        The result depends on what is installed (module cache, node_modules,
        pip environment), so run it where the file was generated.

    python scripts/third-party-licenses.py generate --stdout --strict
        Exit 1 when a direct or shipped dependency has no licence, could not
        be inspected, or is outside the permissive set. The list is printed
        on stderr.

    python scripts/third-party-licenses.py dump-pip > env.json
        Print the licence metadata of every distribution installed in the
        interpreter that runs the script. Run it inside the environment a
        requirements file was installed into (a venv, a container image), then
        hand the result to `generate`:

    python scripts/third-party-licenses.py generate \
        --pip analyzer=analyzer.json --pip classifier=classifier.json

Where the data comes from:

  Go      `go list -m -json all` in proxy/ with GOPROXY=off, and
          `go list -deps ./cmd/...` to tell the modules that are compiled into
          the binaries from the ones that are only in the module graph (tests,
          tools). Go modules do not declare a licence: the id is DETECTED by
          matching phrases in the LICENSE/COPYING file in the module cache.
  npm     package-lock.json for the dependency set (entries marked `dev` or
          `devOptional` are left out), and the `license` field of each
          package's own package.json under node_modules; the lockfile's
          `license` field is the fallback for packages not installed on this
          platform.
  Python  the requirements file (or pyproject) for the direct set, and
          distribution metadata (License-Expression, License, classifiers) for
          the licences. Without `--pip NAME=FILE` the metadata is read from
          the interpreter running the script, which may hold other versions or
          lack the package: both cases are written into the table. A package
          that interpreter lacks is taken from the records in
          scripts/third-party-licence-data.json when there is one (the
          "recorded wheel metadata" rows); --pip does not use the records.

This is an inventory, not legal advice. The sections between the MANUAL
markers in the output file are kept as they are across regenerations.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "THIRD_PARTY_LICENSES.md"
EVIDENCE = ROOT / "scripts" / "third-party-licence-data.json"

MANUAL_BEGIN = "<!-- BEGIN MANUAL: kept across regenerations -->"
MANUAL_END = "<!-- END MANUAL -->"

# Licences the summary treats as "plainly permissive" (weak file-level
# copyleft MPL-2.0 is included, as the task that defined this report asked).
PERMISSIVE = {
    "MIT", "MIT-0", "BSD-2-CLAUSE", "BSD-3-CLAUSE", "ISC", "APACHE-2.0", "0BSD",
    "UNLICENSE", "CC0-1.0", "MPL-2.0", "PYTHON-2.0", "PSF-2.0", "BLUEOAK-1.0.0",
    "ZLIB", "BSL-1.0", "HPND", "MIT-CMU", "UNICODE-3.0", "UNICODE-DFS-2016",
}

# Loose names found in pip metadata, mapped to an SPDX id. Anything not here
# and not an SPDX id is reported verbatim and lands on the review list.
ALIASES = {
    "mit license": "MIT", "mit": "MIT", "the mit license": "MIT", "mit-cmu": "MIT-CMU",
    "bsd license": "BSD (unspecified)", "bsd": "BSD (unspecified)",
    "bsd-3-clause license": "BSD-3-Clause", "bsd 3-clause": "BSD-3-Clause",
    "bsd 3-clause license": "BSD-3-Clause", "3-clause bsd license": "BSD-3-Clause",
    "new bsd license": "BSD-3-Clause", "new bsd": "BSD-3-Clause", "modified bsd license": "BSD-3-Clause",
    "bsd-2-clause license": "BSD-2-Clause", "bsd 2-clause license": "BSD-2-Clause",
    "simplified bsd": "BSD-2-Clause",
    "apache software license": "Apache-2.0", "apache 2.0": "Apache-2.0", "apache-2": "Apache-2.0",
    "apache license 2.0": "Apache-2.0", "apache license, version 2.0": "Apache-2.0",
    "apache license version 2.0": "Apache-2.0", "apache 2": "Apache-2.0", "apache": "Apache-2.0",
    "apache 2.0 license": "Apache-2.0", "apache-2.0 license": "Apache-2.0",
    "isc license": "ISC", "isc license (iscl)": "ISC",
    "python software foundation license": "PSF-2.0", "psf": "PSF-2.0", "psf-2.0": "PSF-2.0",
    "mozilla public license 2.0 (mpl 2.0)": "MPL-2.0", "mpl 2.0": "MPL-2.0", "mpl-2.0": "MPL-2.0",
    "the unlicense (unlicense)": "Unlicense", "the unlicense": "Unlicense", "public domain": "Public-Domain (unspecified)",
    "zlib/libpng license": "Zlib",
    "gnu lesser general public license v3 (lgplv3)": "LGPL-3.0-only",
    "gnu lesser general public license v3 or later (lgplv3+)": "LGPL-3.0-or-later",
    "gnu lesser general public license v2 or later (lgplv2+)": "LGPL-2.0-or-later",
    "gnu library or lesser general public license (lgpl)": "LGPL (unspecified)",
    "gnu general public license v3 (gplv3)": "GPL-3.0-only",
    "gnu general public license v2 (gplv2)": "GPL-2.0-only",
    "gnu general public license (gpl)": "GPL (unspecified)",
    "gnu affero general public license v3": "AGPL-3.0-only",
    "historical permission notice and disclaimer (hpnd)": "HPND",
}


def md_escape(s: str) -> str:
    return s.replace("|", "\\|").replace("\n", " ").strip()


def tokens(expr: str) -> list[str]:
    """Licence ids in an SPDX-like expression."""
    cleaned = re.sub(r"[()]", " ", expr)
    parts = re.split(r"\s+(?:OR|AND|WITH|or|and)\s+|\s*/\s*|\s*,\s*|\s*;\s*", cleaned)
    return [p.strip() for p in parts if p.strip()]


def is_permissive(expr: str) -> bool:
    toks = tokens(expr)
    return bool(toks) and all(t.upper() in PERMISSIVE for t in toks)


def summarise(rows: list[dict]) -> tuple[list[tuple[str, int]], list[dict]]:
    counts = Counter(r["license"] or "MISSING" for r in rows)
    review = [r for r in rows if not r["license"] or not is_permissive(r["license"])]
    return sorted(counts.items(), key=lambda kv: (-kv[1], kv[0])), review


# --------------------------------------------------------------------------- Go

GO_LICENSE_FILES = re.compile(r"^(LICEN[SC]E|COPYING|UNLICENSE|COPYRIGHT)([.\-_].*)?$", re.I)


def detect_license_text(text: str) -> list[str]:
    t = re.sub(r"\s+", " ", text)
    found: list[str] = []

    def has(p: str) -> bool:
        return re.search(p, t, re.I) is not None

    # The GNU licences are recognised by their title only, at the head of the
    # file: other licences (MPL-2.0 for one) name them in their body.
    head = t[:300]
    if re.search(r"GNU AFFERO GENERAL PUBLIC LICENSE", head, re.I):
        found.append("AGPL-3.0")
    elif re.search(r"GNU LESSER GENERAL PUBLIC LICENSE", head, re.I):
        found.append("LGPL")
    elif re.search(r"GNU GENERAL PUBLIC LICENSE", head, re.I):
        found.append("GPL")
    if has(r"Apache License,? Version 2\.0") or has(r"apache\.org/licenses/LICENSE-2\.0"):
        found.append("Apache-2.0")
    if has(r"Mozilla Public License,? (Version|v\.?) ?2\.0"):
        found.append("MPL-2.0")
    if has(r"Permission is hereby granted, free of charge, to any person obtaining a copy"):
        found.append("MIT")
    if has(r"Permission to use, copy, modify, and(/or)? distribute this software for any purpose"):
        found.append("ISC")
    if has(r"Redistribution and use in source and binary forms"):
        if has(r"must display the following acknowledge?ment"):
            found.append("BSD-4-Clause")
        elif has(r"Neither the name|may not be used to endorse or promote|names of (its|any) contributors"):
            found.append("BSD-3-Clause")
        else:
            found.append("BSD-2-Clause")
    if has(r"This is free and unencumbered software released into the public domain"):
        found.append("Unlicense")
    if has(r"CC0 1\.0 Universal|Creative Commons Zero"):
        found.append("CC0-1.0")
    if has(r"Business Source License"):
        found.append("BUSL-1.1")
    if has(r"Server Side Public License"):
        found.append("SSPL-1.0")
    if has(r"Elastic License"):
        found.append("Elastic-2.0")
    if has(r"Eclipse Public License"):
        found.append("EPL")
    if has(r"COMMON DEVELOPMENT AND DISTRIBUTION LICENSE"):
        found.append("CDDL")
    return found


def go_modules() -> tuple[list[dict], str]:
    proxy = ROOT / "proxy"
    try:
        # `go list -m` reports a module's directory only when go.sum holds the
        # hash of its zip, and -mod=mod adds missing hashes. It writes them to
        # a copy of go.mod and go.sum (-modfile), so proxy/go.sum is untouched.
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copyfile(proxy / "go.mod", Path(tmp) / "go.mod")
            if (proxy / "go.sum").is_file():
                shutil.copyfile(proxy / "go.sum", Path(tmp) / "go.sum")
            env = dict(os.environ, GOPROXY="off", GOFLAGS=f"-mod=mod -modfile={Path(tmp) / 'go.mod'}",
                       GOOS="linux", GOARCH="amd64", CGO_ENABLED="0")
            raw = subprocess.run(["go", "list", "-m", "-json", "all"], cwd=proxy, env=env,
                                 capture_output=True, text=True, check=True).stdout
            linked_raw = subprocess.run(
                ["go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", "./cmd/..."],
                cwd=proxy, env=env, capture_output=True, text=True, check=True).stdout
        goversion = subprocess.run(["go", "version"], capture_output=True, text=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError) as exc:
        return [], f"go list failed: {exc}"
    linked = {line.strip() for line in linked_raw.splitlines() if line.strip()}

    mods = []
    dec = json.JSONDecoder()
    i = 0
    while i < len(raw):
        while i < len(raw) and raw[i].isspace():
            i += 1
        if i >= len(raw):
            break
        obj, i = dec.raw_decode(raw, i)
        mods.append(obj)

    rows = []
    for m in mods:
        if m.get("Main"):
            continue
        eff = m.get("Replace") or m
        d = eff.get("Dir")
        lic, src = "", ""
        if d and Path(d).is_dir():
            files = sorted(p for p in Path(d).iterdir() if p.is_file() and GO_LICENSE_FILES.match(p.name))
            ids: list[str] = []
            for f in files:
                for x in detect_license_text(f.read_text(encoding="utf-8", errors="replace")):
                    if x not in ids:
                        ids.append(x)
            if ids:
                lic = " AND ".join(ids)
                src = ", ".join(f.name for f in files)
            elif files:
                lic = "UNRECOGNISED"
                src = ", ".join(f.name for f in files)
            else:
                src = "no licence file in module root"
        else:
            lic = "NOT-INSPECTED"
            src = "module not in the local module cache"
        rows.append({
            "name": m["Path"], "version": eff.get("Version", m.get("Version", "")),
            "license": lic, "source": src,
            "direct": not m.get("Indirect", False), "linked": m["Path"] in linked,
        })
    return rows, goversion


# -------------------------------------------------------------------------- npm

def npm_license_of(pkg_json: Path) -> str:
    try:
        d = json.loads(pkg_json.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return ""
    lic = d.get("license")
    if isinstance(lic, dict):
        lic = lic.get("type", "")
    if not lic and isinstance(d.get("licenses"), list):
        lic = " OR ".join(x.get("type", "") if isinstance(x, dict) else str(x) for x in d["licenses"])
    return str(lic or "")


def npm_packages(project: Path) -> tuple[list[dict], dict]:
    lock = json.loads((project / "package-lock.json").read_text(encoding="utf-8"))
    root = lock["packages"].get("", {})
    direct = set(root.get("dependencies", {}))
    rows = []
    skipped_dev = 0
    for path, meta in lock["packages"].items():
        if not path:
            continue
        if meta.get("dev") or meta.get("devOptional"):
            skipped_dev += 1
            continue
        if meta.get("link"):
            continue
        name = meta.get("name") or path.split("node_modules/")[-1]
        pj = project / path / "package.json"
        if pj.is_file():
            lic, src = npm_license_of(pj), "node_modules package.json"
        else:
            lic, src = str(meta.get("license") or ""), "package-lock.json (not installed on this platform)"
        rows.append({
            "name": name, "version": meta.get("version", ""), "license": lic, "source": src,
            # Only the top-level copy is the direct dependency; a nested copy
            # of the same name belongs to whichever package required it.
            "direct": name in direct and path == f"node_modules/{name}",
            "optional": bool(meta.get("optional")), "path": path,
        })
    info = {"direct_declared": root.get("dependencies", {}), "skipped_dev": skipped_dev,
            "lockfile_version": lock.get("lockfileVersion")}
    return rows, info


# ----------------------------------------------------------------------- Python

def norm(name: str) -> str:
    return re.sub(r"[-_.]+", "-", name).lower()


def pip_license(meta_get, meta_get_all) -> tuple[str, str]:
    expr = meta_get("License-Expression")
    if expr:
        return expr.strip(), "License-Expression"
    classifiers = [c.split("::")[-1].strip() for c in (meta_get_all("Classifier") or [])
                   if c.startswith("License ::")]
    classifiers = [c for c in classifiers if c not in ("OSI Approved",)]
    lic = (meta_get("License") or "").strip()
    if lic and "\n" not in lic and len(lic) <= 80 and lic.upper() != "UNKNOWN":
        return ALIASES.get(lic.lower(), lic), "License field"
    if classifiers:
        mapped = [ALIASES.get(c.lower(), c) for c in classifiers]
        return " AND ".join(dict.fromkeys(mapped)), "Trove classifier"
    if lic:
        ids = detect_license_text(lic)
        if ids:
            return " AND ".join(ids), "License field (full text, detected)"
        return "UNRECOGNISED", "License field (free text)"
    return "", "no licence metadata"


def dump_pip() -> dict:
    from importlib import metadata
    out = {}
    for dist in metadata.distributions():
        name = dist.metadata.get("Name")
        if not name:
            continue
        lic, src = pip_license(dist.metadata.get, dist.metadata.get_all)
        files = []
        try:
            for f in dist.files or []:
                s = str(f).replace("\\", "/")
                if ".dist-info/" in s and re.search(r"(LICEN[SC]E|COPYING|NOTICE)", s.rsplit("/", 1)[-1], re.I):
                    files.append(s.rsplit("/", 1)[-1])
        except OSError:
            pass
        if (lic in ("", "UNRECOGNISED") or "(unspecified)" in lic) and files:
            ids: list[str] = []
            for f in dist.files or []:
                s = str(f).replace("\\", "/")
                if ".dist-info/" in s and re.search(r"(LICEN[SC]E|COPYING)", s.rsplit("/", 1)[-1], re.I):
                    try:
                        for x in detect_license_text(Path(str(f.locate())).read_text(encoding="utf-8", errors="replace")):
                            if x not in ids:
                                ids.append(x)
                    except (OSError, ValueError):
                        pass
            if ids:
                was = f"; metadata says only \"{lic}\"" if lic else ""
                lic, src = " AND ".join(ids), f"licence file in dist-info (detected{was})"
        out[norm(name)] = {
            "name": name, "version": dist.version, "license": lic, "source": src,
            "requires": dist.metadata.get_all("Requires-Dist") or [],
        }
    return {"python": sys.version.split()[0], "distributions": out}


def checked_python_evidence() -> dict[str, dict[str, dict]]:
    """Recorded dump-pip entries per component, from scripts/third-party-licence-data.json.

    They fill in packages the interpreter running this script does not have,
    so the file can be regenerated where those packages are not installed.
    They are not used for a component given with --pip: an environment built
    from the requirements is the better source.
    """
    data = json.loads(EVIDENCE.read_text(encoding="utf-8"))
    if data.get("version") != 1 or not isinstance(data.get("python"), dict):
        raise ValueError(f"{EVIDENCE} must contain version 1 Python evidence")
    return data["python"]


REQ_NAME = re.compile(r"^\s*([A-Za-z0-9][A-Za-z0-9._-]*)\s*(\[[^\]]*\])?\s*([^;#]*)")


def parse_requirement(line: str) -> tuple[str, set[str], str, str] | None:
    line = line.split("#", 1)[0].strip()
    if not line or line.startswith("-"):
        return None
    marker = ""
    if ";" in line:
        line, marker = line.split(";", 1)
    m = REQ_NAME.match(line)
    if not m:
        return None
    extras = {e.strip().lower() for e in (m.group(2) or "").strip("[]").split(",") if e.strip()}
    return m.group(1), extras, (m.group(3) or "").strip(), marker.strip()


def marker_applies(marker: str, extras: set[str]) -> bool:
    """Deliberately small: honours `extra == "x"`, skips other-platform markers."""
    if not marker:
        return True
    wanted = re.findall(r"""extra\s*==\s*['"]([^'"]+)['"]""", marker)
    if wanted and not any(w.lower() in extras for w in wanted):
        return False
    if re.search(r"""(sys_platform|platform_system)\s*==\s*['"](win32|Windows|darwin|Darwin|emscripten|ios)""", marker):
        return False
    if re.search(r"""python_version\s*<\s*['"]3\.(\d+)['"]""", marker):
        if int(re.search(r"""python_version\s*<\s*['"]3\.(\d+)['"]""", marker).group(1)) <= 11:
            return False
    return True


def python_component(direct_lines: list[str], env: dict,
                     evidence: dict[str, dict] | None = None) -> tuple[list[dict], list[dict], list[str]]:
    dists = env["distributions"]
    evidence = evidence or {}
    available = dict(evidence)
    available.update(dists)
    direct_rows, missing = [], []
    seen: dict[str, bool] = {}
    queue: list[tuple[str, set[str]]] = []
    for line in direct_lines:
        req = parse_requirement(line)
        if not req:
            continue
        name, extras, spec, _ = req
        key = norm(name)
        d = available.get(key)
        direct_rows.append({
            "name": name, "declared": spec or "(any)",
            "version": (d["version"] + (" (differs from the pin)" if spec.startswith("==") and spec[2:].strip() != d["version"] else "")) if d else "",
            "license": d["license"] if d else "NOT-INSPECTED",
            "source": d["source"] if d else "not installed in the inspected environment",
        })
        seen[key] = True
        if d:
            queue.append((key, extras))
    trans: dict[str, dict] = {}
    while queue:
        key, extras = queue.pop()
        for r in available[key]["requires"]:
            req = parse_requirement(r)
            if not req:
                continue
            name, sub_extras, _, marker = req
            if not marker_applies(marker, extras):
                continue
            k = norm(name)
            if k in seen:
                continue
            seen[k] = True
            d = available.get(k)
            if not d:
                missing.append(name)
                continue
            trans[k] = {"name": d["name"], "version": d["version"], "license": d["license"], "source": d["source"]}
            queue.append((k, sub_extras))
    return direct_rows, sorted(trans.values(), key=lambda r: r["name"].lower()), sorted(set(missing), key=str.lower)


def requirements_lines(path: Path) -> list[str]:
    return path.read_text(encoding="utf-8").splitlines()


def pyproject_dependencies(path: Path) -> list[str]:
    import tomllib
    return list(tomllib.loads(path.read_text(encoding="utf-8"))["project"].get("dependencies", []))


# ----------------------------------------------------------------------- output

def table(headers: list[str], rows: list[list[str]]) -> list[str]:
    out = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    out += ["| " + " | ".join(md_escape(str(c)) for c in r) + " |" for r in rows]
    return out


def summary_block(rows: list[dict], what: str) -> list[str]:
    counts, review = summarise(rows)
    out = [f"{what}: {len(rows)}.", ""]
    out += table(["Licence", "Count"], [[k, str(v)] for k, v in counts])
    out.append("")
    if review:
        out.append("Not plainly permissive, unrecognised, not inspected or missing:")
        out.append("")
        out += table(["Package", "Version", "Licence", "Read from"],
                     [[r["name"], r["version"], r["license"] or "MISSING", r.get("source", "")]
                      for r in sorted(review, key=lambda r: r["name"].lower())])
    else:
        out.append("None outside the permissive set.")
    out.append("")
    return out


DEFAULT_MANUAL = """\
## Models and datasets

(Not generated. Edit by hand.)
"""


def generate(pip_envs: dict[str, dict]) -> tuple[str, list[str]]:
    problems: list[str] = []
    existing = OUT.read_text(encoding="utf-8").replace("\r\n", "\n") if OUT.is_file() else ""
    m = re.search(re.escape(MANUAL_BEGIN) + r"\n(.*?)" + re.escape(MANUAL_END), existing, re.S)
    manual = m.group(1) if m else DEFAULT_MANUAL

    L: list[str] = []
    L += [
        "# Third-party licences",
        "",
        "An inventory of the licences that Tamga's dependencies declare. Tamga itself is",
        "AGPL-3.0-only (see `LICENSE`). This file is generated by",
        "`scripts/third-party-licenses.py` from what is on disk (Go module cache,",
        "`node_modules`, pip metadata); the section on models and datasets is written by",
        "hand. It records what each package declares and where that was read. It is not",
        "a legal opinion and makes no statement about licence compatibility.",
        "",
        "\"Plainly permissive\" below means every licence id in the package's expression",
        "is one of: " + ", ".join(sorted(PERMISSIVE)) + ".",
        "Everything else is listed by name for a person to review, including packages",
        "whose licence could not be read.",
        "",
        "Regenerate: `python scripts/third-party-licenses.py generate` (see the script's",
        "header for the `--pip` option that reads a container's or a venv's metadata).",
        "",
    ]

    # Go
    go_rows, go_info = go_modules()
    L += ["## Go modules (`proxy/`)", ""]
    if not go_rows:
        L += [f"Not generated: {go_info}", ""]
        problems.append("go: " + go_info)
    else:
        L += [
            f"Source: `go list -m -json all` and `go list -deps ./cmd/...` (GOOS=linux, GOPROXY=off), {go_info}.",
            "Go modules declare no licence in their metadata. The id below is detected by",
            "matching phrases in the LICENSE/COPYING file in the module cache; the file is",
            "named in the last column. \"In binaries\" says whether any package of the",
            "module is compiled into `cmd/tamga`, `cmd/scanner-service` or `cmd/redteam`;",
            "the others are in the module graph for tests or tools only.",
            "",
            "### Direct",
            "",
        ]
        direct = sorted((r for r in go_rows if r["direct"]), key=lambda r: r["name"])
        L += table(["Module", "Version", "Licence (detected)", "In binaries", "Read from"],
                   [[r["name"], r["version"], r["license"] or "MISSING", "yes" if r["linked"] else "no", r["source"]]
                    for r in direct])
        L.append("")
        indirect = [r for r in go_rows if not r["direct"]]
        linked = [r for r in indirect if r["linked"]]
        unlinked = [r for r in indirect if not r["linked"]]
        L += ["### Transitive, compiled into the binaries", ""]
        L += summary_block(linked, "Modules")
        L += table(["Module", "Version", "Licence (detected)"],
                   [[r["name"], r["version"], r["license"] or "MISSING"] for r in sorted(linked, key=lambda r: r["name"])])
        L.append("")
        L += ["### Transitive, in the module graph only (tests, tools; not in the binaries)", ""]
        L += summary_block(unlinked, "Modules")
        for r in direct + linked:
            if not r["license"] or r["license"] in ("UNRECOGNISED", "NOT-INSPECTED") or not is_permissive(r["license"].replace(" AND ", " OR ")):
                problems.append(f"go: {r['name']} {r['version']}: {r['license'] or 'MISSING'}")

    # npm
    for title, rel in (("npm: console (`dashboard/`)", "dashboard"),
                       ("npm: TypeScript SDK (`sdk/typescript/tamga-client/`)", "sdk/typescript/tamga-client")):
        project = ROOT / rel
        L += [f"## {title}", ""]
        if not (project / "package-lock.json").is_file():
            L += ["Not generated: no package-lock.json.", ""]
            continue
        rows, info = npm_packages(project)
        L += [
            f"Source: `{rel}/package-lock.json` (lockfileVersion {info['lockfile_version']}) for the set,",
            "each package's own `package.json` `license` field for the licence.",
            f"Development dependencies are left out ({info['skipped_dev']} lockfile entries marked `dev` or `devOptional`).",
            "",
            "### Direct (production)",
            "",
        ]
        direct = sorted((r for r in rows if r["direct"]), key=lambda r: r["name"])
        if direct:
            L += table(["Package", "Declared range", "Installed", "Licence (declared)", "Read from"],
                       [[r["name"], info["direct_declared"].get(r["name"], ""), r["version"], r["license"] or "MISSING", r["source"]]
                        for r in direct])
        else:
            L.append("None. The package has no runtime dependencies.")
        L.append("")
        trans = [r for r in rows if not r["direct"]]
        L += ["### Transitive (production)", ""]
        if trans:
            L += summary_block(trans, "Packages (lockfile entries, a package may appear at more than one version)")
        else:
            L += ["None.", ""]
        for r in rows:
            if not r["license"] or not is_permissive(r["license"]):
                problems.append(f"npm {rel}: {r['name']} {r['version']}: {r['license'] or 'MISSING'}")

    # Python
    components = (
        ("analyzer", "Python: analyzer (`analyzer/requirements.txt`)", lambda: requirements_lines(ROOT / "analyzer/requirements.txt")),
        ("classifier", "Python: classifier (`classifier/requirements.txt`)", lambda: requirements_lines(ROOT / "classifier/requirements.txt")),
        ("sdk-python", "Python: SDK (`sdk/python/pyproject.toml`)", lambda: pyproject_dependencies(ROOT / "sdk/python/pyproject.toml")),
    )
    local_env = None
    evidence = checked_python_evidence()
    for key, title, load in components:
        L += [f"## {title}", ""]
        if key in pip_envs:
            env = pip_envs[key]
            L += [f"Licences read from the pip metadata of an environment the requirements were installed into "
                  f"(`--pip {key}=...`, Python {env.get('python', '?')}).", ""]
        else:
            if local_env is None:
                local_env = dump_pip()
            env = local_env
            L += ["Licences read from the pip metadata of the interpreter that ran the generator, NOT from an",
                  "environment built from this requirements file: versions may differ from the pins, and",
                  "packages that are not installed are marked NOT-INSPECTED unless",
                  "`scripts/third-party-licence-data.json` records them (\"recorded wheel metadata\").", ""]
        direct, trans, missing = python_component(load(), env, None if key in pip_envs else evidence.get(key))
        L += ["### Direct", ""]
        L += table(["Package", "Declared", "Inspected version", "Licence (declared)", "Read from"],
                   [[r["name"], r["declared"], r["version"] or "-", r["license"] or "MISSING", r["source"]] for r in direct])
        L.append("")
        L += ["### Transitive", ""]
        if trans:
            L += summary_block(trans, "Distributions reached through Requires-Dist")
        else:
            L += ["None found in the inspected environment.", ""]
        if missing:
            L += ["Required by the above but not installed in the inspected environment (not inspected; some are",
                  "optional or for another platform): " + ", ".join(f"`{m}`" for m in missing) + ".", ""]
        for r in direct + trans:
            if not r["license"] or not is_permissive(r["license"]):
                problems.append(f"python {key}: {r['name']} {r.get('version', '')}: {r['license'] or 'MISSING'}")

    L += [MANUAL_BEGIN, manual.rstrip("\n"), MANUAL_END, ""]
    return "\n".join(L), problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("dump-pip")
    g = sub.add_parser("generate")
    g.add_argument("--pip", action="append", default=[], metavar="NAME=FILE",
                   help="NAME is analyzer, classifier or sdk-python; FILE is the output of dump-pip")
    g.add_argument("--check", action="store_true", help="exit 1 if the file on disk is out of date")
    g.add_argument("--strict", action="store_true", help="exit 1 if anything is on the review list")
    g.add_argument("--stdout", action="store_true", help="print instead of writing the file")
    args = ap.parse_args()

    if args.cmd == "dump-pip":
        json.dump(dump_pip(), sys.stdout, indent=1, sort_keys=True)
        return 0

    envs = {}
    for item in args.pip:
        name, _, file = item.partition("=")
        envs[name] = json.loads(Path(file).read_text(encoding="utf-8-sig"))
    text, problems = generate(envs)
    if args.stdout:
        sys.stdout.write(text)
    elif args.check:
        current = OUT.read_text(encoding="utf-8").replace("\r\n", "\n") if OUT.is_file() else ""
        if current != text:
            print("THIRD_PARTY_LICENSES.md is out of date; run: python scripts/third-party-licenses.py generate", file=sys.stderr)
            return 1
    else:
        OUT.write_text(text, encoding="utf-8", newline="\n")
        print(f"wrote {OUT}", file=sys.stderr)
    if problems:
        print("\nFor review (not plainly permissive, unrecognised, not inspected or missing):", file=sys.stderr)
        for p in problems:
            print("  " + p, file=sys.stderr)
        if args.strict:
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
