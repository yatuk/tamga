"""A removed setting or scan type is reported in the log once, not silently ignored."""

import pytest
import structlog

from app import removed


@pytest.fixture(autouse=True)
def _fresh_state():
    """Each test starts as a new process would: nothing reported yet."""
    saved_env, saved_types = removed._env_reported, set(removed._scan_types_reported)
    removed._env_reported = False
    removed._scan_types_reported.clear()
    yield
    removed._env_reported = saved_env
    removed._scan_types_reported.clear()
    removed._scan_types_reported.update(saved_types)


def _warnings(logs: list[dict]) -> list[dict]:
    return [entry for entry in logs if entry["log_level"] == "warning"]


def test_ban_topics_is_reported_once_when_set():
    env = {"TAMGA_BAN_TOPICS": "weapons, self-harm"}
    with structlog.testing.capture_logs() as logs:
        assert removed.report_removed_env(env) == ["TAMGA_BAN_TOPICS"]
        assert removed.report_removed_env(env) == []

    warnings = _warnings(logs)
    assert len(warnings) == 1
    assert "TAMGA_BAN_TOPICS is set and is no longer read" in warnings[0]["event"]
    # The topic list itself is not written to the log.
    assert "weapons" not in repr(warnings[0])


@pytest.mark.parametrize("env", [{}, {"TAMGA_BAN_TOPICS": ""}, {"TAMGA_BAN_TOPICS": "  "}])
def test_nothing_is_logged_when_ban_topics_is_unset_or_empty(env):
    with structlog.testing.capture_logs() as logs:
        assert removed.report_removed_env(env) == []
    assert _warnings(logs) == []


def test_removed_env_reads_the_process_environment_by_default(monkeypatch):
    monkeypatch.setenv("TAMGA_BAN_TOPICS", "weapons")
    with structlog.testing.capture_logs() as logs:
        assert removed.report_removed_env() == ["TAMGA_BAN_TOPICS"]
    assert len(_warnings(logs)) == 1


def test_toxicity_scan_type_is_reported_once():
    with structlog.testing.capture_logs() as logs:
        assert removed.report_removed_scan_types({"pii", "toxicity"}) == ["toxicity"]
        assert removed.report_removed_scan_types(["toxicity"]) == []

    warnings = _warnings(logs)
    assert len(warnings) == 1
    assert "scan type 'toxicity' was requested and no longer exists" in warnings[0]["event"]


def test_current_scan_types_are_not_reported():
    with structlog.testing.capture_logs() as logs:
        assert removed.report_removed_scan_types(["pii", "injection", "custom", "operator_state"]) == []
    assert _warnings(logs) == []
