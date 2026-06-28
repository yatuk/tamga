"""Plugin discovery — auto-load custom scanners from a directory.

Set TAMGA_CUSTOM_SCANNERS_DIR to point at a directory of Python files.
Each file should contain one or more BaseScanner subclasses.
The discovery module imports them and makes them available for the scan flow.

Scanner isolation: each custom scanner runs in the same process as built-in
scanners.  If a custom scanner raises an exception, it is caught and logged
(fail-open per scanner) — one custom scanner cannot take down the analyzer.
"""

from __future__ import annotations

import importlib.util
import os
from pathlib import Path
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from app.scanners.base import BaseScanner

logger = structlog.get_logger()

# In-memory registry of discovered custom scanner classes.
_custom_scanner_classes: list[type[BaseScanner]] = []

# Module cache — keyed by file path, so hot-reload just re-imports.
_module_cache: dict[str, list[type[BaseScanner]]] = {}


def _env_dir() -> str:
    """Return the configured custom scanners directory (env or default)."""
    return os.environ.get(
        "TAMGA_CUSTOM_SCANNERS_DIR",
        os.path.join(os.path.dirname(__file__), "..", "..", "custom_scanners"),
    )


def discover_scanners(scanner_dir: str | None = None) -> list[type[BaseScanner]]:
    """Discover and import all BaseScanner subclasses from *scanner_dir*.

    Args:
        scanner_dir: Directory containing scanner plugin .py files.
                     Defaults to ``TAMGA_CUSTOM_SCANNERS_DIR`` or
                     ``<project>/custom_scanners/``.

    Returns:
        List of scanner *classes* (not instances).  Callers instantiate them.
    """
    global _custom_scanner_classes

    if scanner_dir is None:
        scanner_dir = _env_dir()

    path = Path(scanner_dir).resolve()
    if not path.is_dir():
        logger.debug("custom_scanners_dir not found, skipping", path=str(path))
        return []

    # Check cache: same directory, non-empty cache → return cached.
    cache_key = str(path)
    if cache_key in _module_cache and _module_cache[cache_key]:
        _custom_scanner_classes = _module_cache[cache_key]
        return _custom_scanner_classes

    from app.scanners.base import BaseScanner

    discovered: list[type[BaseScanner]] = []

    for py_file in sorted(path.glob("*.py")):
        if py_file.name.startswith("_"):
            continue  # skip __init__.py, private modules

        module_name = f"tamga_custom_{py_file.stem}"

        try:
            spec = importlib.util.spec_from_file_location(module_name, str(py_file))
            if spec is None or spec.loader is None:
                logger.warning("cannot load custom scanner", file=str(py_file))
                continue

            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)

            # Collect BaseScanner subclasses defined in this module.
            found_in_file = 0
            for attr_name in dir(module):
                attr = getattr(module, attr_name)
                if (
                    isinstance(attr, type)
                    and issubclass(attr, BaseScanner)
                    and attr is not BaseScanner
                ):
                    discovered.append(attr)
                    found_in_file += 1

            if found_in_file > 0:
                logger.info(
                    "custom scanner loaded",
                    file=str(py_file),
                    scanners=found_in_file,
                )

        except Exception as exc:
            logger.warning(
                "failed to load custom scanner (fail-open, skipping)",
                file=str(py_file),
                error=str(exc),
            )
            continue

    # Update cache and global registry.
    _module_cache[cache_key] = discovered
    _custom_scanner_classes = discovered

    logger.info(
        "custom scanner discovery complete",
        total=len(discovered),
        dir=str(path),
    )
    return discovered


def get_custom_scanners() -> list[type[BaseScanner]]:
    """Return the currently discovered custom scanner classes.

    If discovery hasn't run yet, returns an empty list.
    Call ``discover_scanners()`` first.
    """
    return list(_custom_scanner_classes)
