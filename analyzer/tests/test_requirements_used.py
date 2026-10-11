"""Every line of requirements.txt is a package the analyzer uses.

These tests read the tree; nothing is installed or imported. A requirement
fails the check when no Python file under ``analyzer/`` imports a module it
provides, unless it is in ``_NOT_IMPORTED`` below with the reason it is
installed all the same. A package nobody imports is download time, image size
and one more licence to keep track of.
"""

import ast
import re
from pathlib import Path

ANALYZER_ROOT = Path(__file__).resolve().parent.parent

_SKIP_DIRS = {"__pycache__", ".venv", "venv", ".pytest_cache", ".ruff_cache", ".mypy_cache"}

# The module a distribution is imported as, where that is not its name with
# "-" turned into "_".
_MODULE_OF = {
    "grpcio": "grpc",
    "grpcio-tools": "grpc_tools",
}

# Requirements no file imports, and why each is installed. An entry that
# starts to be imported, or that leaves requirements.txt, has to leave this
# list too: test_the_exceptions_are_still_exceptions fails otherwise.
_NOT_IMPORTED = {
    "grpcio-tools": (
        "regenerates analyzer/v1/tamga_pb2*.py from the .proto, and is the requirement that "
        "brings protobuf, which those generated files import as google.protobuf"
    ),
    "spacy": (
        "presidio-analyzer's NLP engine; the image build runs `python -m spacy download "
        "en_core_web_sm` (Dockerfile) and this line holds the version that model is for"
    ),
    "httpx": "starlette's TestClient, which the endpoint tests use, does not work without it",
}

_REQUIREMENT = re.compile(r"^\s*([A-Za-z0-9][A-Za-z0-9._-]*)")


def _normalise(name: str) -> str:
    """PyPI treats "-", "_" and "." in a project name as the same character."""
    return re.sub(r"[-_.]+", "-", name).lower()


def _requirements(text: str) -> list[str]:
    names: list[str] = []
    for line in text.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line or line.startswith("-"):
            continue
        match = _REQUIREMENT.match(line)
        assert match, f"cannot read the requirement line: {line!r}"
        names.append(_normalise(match.group(1)))
    return names


def _module_of(requirement: str) -> str:
    return _MODULE_OF.get(requirement, requirement.replace("-", "_"))


def _imported_top_level(source: str, filename: str = "<source>") -> set[str]:
    """The top-level module of every absolute import, wherever in the file it stands."""
    found: set[str] = set()
    for node in ast.walk(ast.parse(source, filename=filename)):
        if isinstance(node, ast.Import):
            found.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module and node.level == 0:
            found.add(node.module.split(".")[0])
    return found


def _python_files() -> list[Path]:
    return sorted(
        path
        for path in ANALYZER_ROOT.rglob("*.py")
        if path.is_file() and not _SKIP_DIRS.intersection(path.parts)
    )


def _imported_anywhere() -> set[str]:
    found: set[str] = set()
    for path in _python_files():
        found |= _imported_top_level(path.read_text(encoding="utf-8"), str(path))
    return found


def _declared() -> list[str]:
    return _requirements((ANALYZER_ROOT / "requirements.txt").read_text(encoding="utf-8"))


def test_the_tree_is_where_the_tests_expect_it():
    """Guard against the checks below passing because they looked at nothing."""
    assert len(_python_files()) > 20
    declared = _declared()
    assert len(declared) >= 10
    assert "fastapi" in declared and "grpcio" in declared
    assert {"fastapi", "grpc", "structlog"} <= _imported_anywhere()


def test_every_requirement_is_imported_somewhere():
    imported = _imported_anywhere()
    unused = [
        name
        for name in _declared()
        if _module_of(name) not in imported and name not in _NOT_IMPORTED
    ]
    assert unused == [], (
        f"requirements.txt lists {unused}, which no file under analyzer/ imports. "
        "Remove the line, or add it to _NOT_IMPORTED with the reason it is installed."
    )


def test_the_exceptions_are_still_exceptions():
    declared = set(_declared())
    imported = _imported_anywhere()
    for name, reason in _NOT_IMPORTED.items():
        assert name == _normalise(name)
        assert len(reason) > 20, f"{name}: say why it is installed"
        assert name in declared, f"{name} is no longer in requirements.txt: take it out of _NOT_IMPORTED"
        assert _module_of(name) not in imported, f"{name} is imported now: take it out of _NOT_IMPORTED"


def test_the_reasons_given_for_the_exceptions_hold():
    """Each reason names something in the tree; it fails when that thing goes."""
    generated = (ANALYZER_ROOT / "analyzer" / "v1" / "tamga_pb2.py").read_text(encoding="utf-8")
    assert "google.protobuf" in generated
    assert "protobuf" not in _declared(), "protobuf is pinned itself now: grpcio-tools no longer has to bring it"

    dockerfile = (ANALYZER_ROOT / "Dockerfile").read_text(encoding="utf-8")
    assert "python -m spacy download" in dockerfile
    assert "presidio_analyzer" in _imported_anywhere()

    uses_test_client = [
        path.name
        for path in _python_files()
        if "tests" in path.parts and "fastapi.testclient" in path.read_text(encoding="utf-8") and path != Path(__file__).resolve()
    ]
    assert uses_test_client != []


def test_the_packages_removed_as_unused_stay_out():
    """openai, presidio-anonymizer, sqlalchemy and tenacity were listed and imported nowhere."""
    declared = set(_declared())
    imported = _imported_anywhere()
    for name in ("openai", "presidio-anonymizer", "sqlalchemy", "tenacity"):
        if name in declared:
            assert _module_of(name) in imported, f"{name} is back in requirements.txt and still imported nowhere"


def test_the_checks_recognise_what_they_look_for():
    text = "grpcio==1.81.1\nuvicorn[standard]==0.31.0  # server\n\n# a comment\n-r other.txt\nPresidio_Analyzer>=2\nsqlalchemy[asyncio]==2.0.35\n"
    assert _requirements(text) == ["grpcio", "uvicorn", "presidio-analyzer", "sqlalchemy"]
    assert _module_of("grpcio") == "grpc"
    assert _module_of("grpcio-tools") == "grpc_tools"
    assert _module_of("pydantic-settings") == "pydantic_settings"

    source = (
        "import grpc.aio\n"
        "from presidio_analyzer.nlp_engine import NlpEngineProvider\n"
        "from . import local\n"
        "from .sibling import thing\n"
        "def late():\n"
        "    import uvicorn\n"
        "try:\n"
        "    import asyncpg as pg\n"
        "except ImportError:\n"
        "    pg = None\n"
    )
    assert _imported_top_level(source) == {"grpc", "presidio_analyzer", "uvicorn", "asyncpg"}
    # A name in a comment or a string is not an import.
    assert _imported_top_level("# import openai\nx = 'import sqlalchemy'\n") == set()
