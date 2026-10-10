"""The SDK carries its own copy of the licence text so that a wheel or sdist
built from this directory alone still ships it. This test keeps that copy
identical to the LICENSE at the repository root."""

from pathlib import Path

import pytest

SDK_DIR = Path(__file__).resolve().parent.parent
SDK_LICENSE = SDK_DIR / "LICENSE"
ROOT_LICENSE = SDK_DIR.parent.parent / "LICENSE"


def _text(path: Path) -> str:
    # Compare text, not bytes: a checkout may convert line endings.
    # read_text() opens in text mode, which already maps CRLF to LF.
    return path.read_text(encoding="utf-8")


def test_sdk_ships_a_license_file():
    assert SDK_LICENSE.is_file(), "sdk/python/LICENSE is missing"
    assert "GNU AFFERO GENERAL PUBLIC LICENSE" in _text(SDK_LICENSE)


def test_license_copy_matches_repository_root():
    if not ROOT_LICENSE.is_file():
        pytest.skip("repository root LICENSE not present (built outside the repository)")
    assert _text(SDK_LICENSE) == _text(ROOT_LICENSE), (
        "sdk/python/LICENSE differs from the repository root LICENSE; copy the root file over it"
    )
