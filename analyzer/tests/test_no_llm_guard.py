"""The analyzer does not depend on the archived llm-guard package.

These tests read the tree; nothing is installed or imported. They fail if
``llm_guard`` is imported anywhere under ``analyzer/`` (statically, or by
name through importlib), or if the package is named in a requirements or
build file there.
"""

import ast
import re
from pathlib import Path

ANALYZER_ROOT = Path(__file__).resolve().parent.parent

_SKIP_DIRS = {"__pycache__", ".venv", "venv", ".pytest_cache", ".ruff_cache", ".mypy_cache"}

# PyPI treats "-", "_" and "." in a project name as the same character.
_PACKAGE = re.compile(r"llm[-_.]guard", re.IGNORECASE)
# A module name handed to importlib.import_module / __import__ as a string.
_DYNAMIC_IMPORT = re.compile(r"""["']llm_guard(\.[\w.]*)?["']""")


def _files(*patterns: str) -> list[Path]:
    found: list[Path] = []
    for pattern in patterns:
        for path in ANALYZER_ROOT.rglob(pattern):
            if path.is_file() and not _SKIP_DIRS.intersection(path.parts):
                found.append(path)
    return sorted(set(found))


def _imported_modules(tree: ast.AST) -> list[tuple[int, str]]:
    names: list[tuple[int, str]] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            names.extend((node.lineno, alias.name) for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            names.append((node.lineno, node.module))
    return names


def test_the_tree_is_where_the_tests_expect_it():
    """Guard against the checks below passing because they looked at nothing."""
    assert (ANALYZER_ROOT / "requirements.txt").is_file()
    assert (ANALYZER_ROOT / "app" / "main.py").is_file()
    assert len(_files("*.py")) > 20


def test_no_python_file_imports_llm_guard():
    offenders: list[str] = []
    this_file = Path(__file__).resolve()
    for path in _files("*.py"):
        source = path.read_text(encoding="utf-8")
        rel = path.relative_to(ANALYZER_ROOT).as_posix()
        for lineno, module in _imported_modules(ast.parse(source, filename=str(path))):
            if module == "llm_guard" or module.startswith("llm_guard."):
                offenders.append(f"{rel}:{lineno} imports {module}")
        if path != this_file:
            for lineno, line in enumerate(source.splitlines(), start=1):
                if _DYNAMIC_IMPORT.search(line):
                    offenders.append(f"{rel}:{lineno} names the module in a string: {line.strip()}")
    assert offenders == []


def test_no_requirements_or_build_file_names_llm_guard():
    files = _files(
        "requirements*.txt", "constraints*.txt", "*.lock", "pyproject.toml",
        "setup.py", "setup.cfg", "Pipfile", "Dockerfile*",
    )
    assert ANALYZER_ROOT / "requirements.txt" in files
    offenders: list[str] = []
    for path in files:
        rel = path.relative_to(ANALYZER_ROOT).as_posix()
        for lineno, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
            if _PACKAGE.search(line):
                offenders.append(f"{rel}:{lineno}: {line.strip()}")
    assert offenders == []


def test_the_checks_recognise_what_they_look_for():
    """The patterns above match the forms llm-guard was used in."""
    tree = ast.parse("import llm_guard.output_scanners\nfrom llm_guard.output_scanners import Toxicity\n")
    assert [m for _, m in _imported_modules(tree)] == ["llm_guard.output_scanners"] * 2
    assert _DYNAMIC_IMPORT.search('importlib.import_module("llm_guard.output_scanners")')
    assert _DYNAMIC_IMPORT.search("__import__('llm_guard')")
    for line in ("llm-guard==0.3.14", "llm_guard>=0.3", "LLM.Guard", "pip install llm-guard[onnxruntime]"):
        assert _PACKAGE.search(line)
    assert not _PACKAGE.search("presidio-analyzer==2.2.355")
