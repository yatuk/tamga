import json

import httpx
import pytest

from tamga import TamgaClient, TamgaError

# ---------------------------------------------------------------------------
# Helper to reduce boilerplate when injecting a mock transport
# ---------------------------------------------------------------------------


def _mock_client(json_response: object, status: int = 200):
    """Return a (TamgaClient, httpx.Client) pair wired to a MockTransport.

    The returned httpx.Client must be used as a context manager so the
    transport is properly closed.
    """
    transport = httpx.MockTransport(
        lambda request: httpx.Response(status, json=json_response)
    )
    hc = httpx.Client(transport=transport)
    c = TamgaClient.__new__(TamgaClient)
    c._base_url = "http://localhost:8080"
    c._headers = {"X-Tamga-Admin-Key": "sk-test"}
    c._client = hc
    return c, hc


def _mock_client_raw(raw_body: str, status: int = 200, content_type: str = "text/plain"):
    """Like _mock_client but returns raw text instead of JSON."""
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            status, content=raw_body.encode(), headers={"Content-Type": content_type}
        )
    )
    hc = httpx.Client(transport=transport)
    c = TamgaClient.__new__(TamgaClient)
    c._base_url = "http://localhost:8080"
    c._headers = {"X-Tamga-Admin-Key": "sk-test"}
    c._client = hc
    return c, hc


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def client() -> TamgaClient:
    return TamgaClient(base_url="http://localhost:8080", admin_key="sk-test-1234")


# ---------------------------------------------------------------------------
# Constructor validation
# ---------------------------------------------------------------------------


def test_init_empty_base_url_raises_value_error():
    with pytest.raises(ValueError, match="base_url"):
        TamgaClient(base_url="", admin_key="sk-123")


def test_init_empty_admin_key_raises_value_error():
    with pytest.raises(ValueError, match="admin_key"):
        TamgaClient(base_url="http://localhost:8080", admin_key="")


def test_init_whitespace_only_raises_value_error():
    with pytest.raises(ValueError):
        TamgaClient(base_url="   ", admin_key="sk-123")
    with pytest.raises(ValueError):
        TamgaClient(base_url="http://localhost:8080", admin_key="\t\n")


# ---------------------------------------------------------------------------
# Existing methods (backward compatibility)
# ---------------------------------------------------------------------------


def test_get_stats_returns_parsed_json():
    fake_response = {"total_requests": 42, "blocked_requests": 3}
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, json=fake_response)
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        result = c.get_stats(range="24h")
        assert result == fake_response


def test_get_events_with_pagination():
    fake_response = {"events": [], "total": 0}
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, json=fake_response)
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        result = c.get_events(page=3, limit=10)
        assert result == fake_response


def test_get_events_with_filters():
    """Ensure extra filter params are passed through."""
    fake_response = {"events": [], "total": 0}
    captured_request: dict = {}

    def capture(request: httpx.Request) -> httpx.Response:
        captured_request["url"] = str(request.url)
        return httpx.Response(200, json=fake_response)

    transport = httpx.MockTransport(capture)
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        result = c.get_events(
            action="BLOCK",
            severity="high",
            finding_type="pii",
            range="30d",
        )
        assert result == fake_response
        url = captured_request.get("url", "")
        assert "action=BLOCK" in url
        assert "severity=high" in url
        assert "finding_type=pii" in url
        assert "range=30d" in url


def test_get_findings_breakdown():
    fake_response = {"range": "30d", "by_type": {"pii": 5}}
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, json=fake_response)
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        result = c.get_findings_breakdown(range="30d")
        assert result["range"] == "30d"


def test_reload_policy():
    fake_response = {"ok": True, "name": "default"}
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, json=fake_response)
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        result = c.reload_policy()
        assert result["ok"] is True


def test_client_context_manager():
    """Sanity-check that the client works as a context manager."""
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, json={"ok": True})
    )
    c = TamgaClient.__new__(TamgaClient)
    c._base_url = "http://localhost:8080"
    c._headers = {"X-Tamga-Admin-Key": "sk-test"}
    c._client = httpx.Client(transport=transport)
    with c:
        r = c.reload_policy()
        assert r == {"ok": True}


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------


def test_http_error_raises_tamga_error():
    transport = httpx.MockTransport(
        lambda request: httpx.Response(503, text="Service Unavailable")
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        with pytest.raises(TamgaError) as exc_info:
            c.get_stats()
        assert exc_info.value.status_code == 503
        assert "Service Unavailable" in exc_info.value.body


def test_non_json_response_raises_tamga_error():
    """A 200 with a non-JSON body should raise TamgaError."""
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            content=b"not json at all",
            headers={"Content-Type": "text/plain"},
        )
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        with pytest.raises(TamgaError) as exc_info:
            c.get_events()
        assert exc_info.value.status_code == 200
        assert "not json at all" in exc_info.value.body


def test_http_404_error_raises_tamga_error():
    transport = httpx.MockTransport(
        lambda request: httpx.Response(404, json={"error": "not found"})
    )
    with httpx.Client(transport=transport) as hc:
        c = TamgaClient.__new__(TamgaClient)
        c._base_url = "http://localhost:8080"
        c._headers = {"X-Tamga-Admin-Key": "sk-test"}
        c._client = hc

        with pytest.raises(TamgaError) as exc_info:
            c.get_stats()
        assert exc_info.value.status_code == 404
        assert "not found" in exc_info.value.body


# ---------------------------------------------------------------------------
# Health
# ---------------------------------------------------------------------------


def test_health():
    c, hc = _mock_client({"status": "ok", "service": "tamga"})
    with hc:
        result = c.health()
        assert result == {"status": "ok", "service": "tamga"}


def test_health_detailed():
    fake = {"proxy": "ok", "database": "ok", "uptime_seconds": 3600}
    c, hc = _mock_client(fake)
    with hc:
        result = c.health_detailed()
        assert result["proxy"] == "ok"
        assert result["uptime_seconds"] == 3600


def test_health_detail():
    fake = {"version": "0.5.0", "tls_enabled": True, "tier": "enterprise"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.health_detail()
        assert result["version"] == "0.5.0"
        assert result["tier"] == "enterprise"


# ---------------------------------------------------------------------------
# Stats
# ---------------------------------------------------------------------------


def test_get_model_stats():
    fake = {"range": "7d", "by_model": {"gpt-4": 100}, "by_family": {"gpt": 100}}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_model_stats(range="7d")
        assert result["by_model"]["gpt-4"] == 100


# ---------------------------------------------------------------------------
# Events (new)
# ---------------------------------------------------------------------------


def test_get_event_detail():
    fake = {"request_id": "req-123", "action": "BLOCK", "findings": []}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_event_detail("req-123")
        assert result["request_id"] == "req-123"
        assert result["action"] == "BLOCK"


def test_export_events_csv():
    csv_body = "request_id,action,timestamp\nreq-1,BLOCK,2025-01-01\n"
    c, hc = _mock_client_raw(csv_body, content_type="text/csv")
    with hc:
        result = c.export_events(format="csv", range="7d")
        assert result == csv_body
        assert "req-1" in result


def test_get_subject_events():
    fake = {"subject": {"user_id": "u1"}, "rows": [], "count": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_subject_events(user_id="u1")
        assert result["subject"]["user_id"] == "u1"


def test_erase_subject():
    fake = {"ok": True, "deleted": 5}
    c, hc = _mock_client(fake)
    with hc:
        result = c.erase_subject(user_id="u1")
        assert result["ok"] is True
        assert result["deleted"] == 5


# ---------------------------------------------------------------------------
# Timeseries
# ---------------------------------------------------------------------------


def test_get_timeseries():
    fake = {"range": "7d", "bucket": "day", "points": []}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_timeseries(range="7d", bucket="day")
        assert result["range"] == "7d"


# ---------------------------------------------------------------------------
# Incidents
# ---------------------------------------------------------------------------


def test_list_incidents():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_incidents(limit=50)
        assert result["total"] == 0


def test_get_incident():
    fake = {"request_id": "req-1", "status": "Open", "assignee": "alice"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_incident("req-1")
        assert result["status"] == "Open"


def test_patch_incident():
    fake = {"request_id": "req-1", "status": "In Progress", "assignee": "bob"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.patch_incident(
            "req-1",
            status="In Progress",
            assignee="bob",
            tags=["pii"],
            add_comment={"author": "admin", "text": "Looking into it"},
        )
        assert result["status"] == "In Progress"


def test_triage_incident():
    fake = {"request_id": "req-1", "status": "In Progress", "assignee": "alice"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.triage_incident("req-1", assignee="alice")
        assert result["assignee"] == "alice"


def test_resolve_incident():
    fake = {"request_id": "req-1", "status": "Closed", "resolution": "true_positive"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.resolve_incident("req-1", resolution="true_positive", notes="Fixed")
        assert result["resolution"] == "true_positive"


def test_reopen_incident():
    fake = {"request_id": "req-1", "status": "Open"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.reopen_incident("req-1")
        assert result["status"] == "Open"


# ---------------------------------------------------------------------------
# MTTR
# ---------------------------------------------------------------------------


def test_get_mttr():
    fake = {"overall_mttr_minutes": 12.5, "trend": "improving"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_mttr(range="30d", org_id="org-1")
        assert result["overall_mttr_minutes"] == 12.5


# ---------------------------------------------------------------------------
# Audit
# ---------------------------------------------------------------------------


def test_get_audit_log():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_audit_log(limit=100)
        assert result["total"] == 0


def test_verify_audit_chain():
    fake = {"chain_ok": True, "entries": 42, "broken_at": None}
    c, hc = _mock_client(fake)
    with hc:
        result = c.verify_audit_chain()
        assert result["chain_ok"] is True


# ---------------------------------------------------------------------------
# Policies
# ---------------------------------------------------------------------------


def test_get_policies():
    fake = [{"name": "default", "version": "1.0"}]
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_policies()
        assert len(result) == 1
        assert result[0]["name"] == "default"


def test_put_policy():
    fake = {"ok": True, "name": "custom", "version": "2.0"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.put_policy("rules:\n  - name: block_pii\n")
        assert result["ok"] is True


def test_validate_policy():
    fake = {"valid": True, "warnings": []}
    c, hc = _mock_client(fake)
    with hc:
        result = c.validate_policy("rules:\n  - name: test\n")
        assert result["valid"] is True


def test_simulate_policy():
    fake = {
        "policy_name": "default",
        "policy_version": "1.0",
        "action": "BLOCK",
        "findings": [],
    }
    c, hc = _mock_client(fake)
    with hc:
        result = c.simulate_policy("Hello, my SSN is 123-45-6789")
        assert result["action"] == "BLOCK"


def test_list_policy_revisions():
    fake = [{"id": "rev-1", "message": "initial"}]
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_policy_revisions()
        assert len(result) == 1


def test_get_policy_revision():
    fake = {"id": "rev-1", "message": "initial", "yaml": "rules: []"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_policy_revision("rev-1")
        assert result["id"] == "rev-1"


def test_rollback_policy():
    fake = {"ok": True, "revision": "rev-1"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.rollback_policy("rev-1")
        assert result["ok"] is True


def test_list_custom_entities():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_custom_entities()
        assert result["total"] == 0


def test_create_custom_entity():
    fake = {"ok": True, "entity": {"name": "my_pattern", "pattern": "\\d{3}"}}
    c, hc = _mock_client(fake)
    with hc:
        result = c.create_custom_entity(
            "my_pattern", "\\d{3}", severity="high", action="BLOCK"
        )
        assert result["entity"]["name"] == "my_pattern"


def test_delete_custom_entity():
    fake = {"ok": True}
    c, hc = _mock_client(fake)
    with hc:
        result = c.delete_custom_entity("my_pattern")
        assert result["ok"] is True


def test_create_policy_proposal():
    fake = {"id": "prop-1", "status": "pending", "message": "Add rule"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.create_policy_proposal("Add rule", "rules: []")
        assert result["id"] == "prop-1"


def test_approve_policy_proposal():
    fake = {"ok": True, "revision_id": "rev-2"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.approve_policy_proposal("prop-1")
        assert result["ok"] is True


def test_reject_policy_proposal():
    fake = {"id": "prop-1", "status": "rejected"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.reject_policy_proposal("prop-1")
        assert result["status"] == "rejected"


# ---------------------------------------------------------------------------
# API Keys
# ---------------------------------------------------------------------------


def test_list_api_keys():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_api_keys()
        assert result["total"] == 0


def test_create_api_key():
    fake = {
        "id": "key-1",
        "label": "my-key",
        "scope": "write",
        "prefix": "sk-tg-",
        "raw_key": "sk-tg-secret",
    }
    c, hc = _mock_client(fake)
    with hc:
        result = c.create_api_key("my-key", scope="write")
        assert result["raw_key"] == "sk-tg-secret"


def test_delete_api_key():
    fake = {"ok": True}
    c, hc = _mock_client(fake)
    with hc:
        result = c.delete_api_key("key-1")
        assert result["ok"] is True


# ---------------------------------------------------------------------------
# Webhooks
# ---------------------------------------------------------------------------


def test_list_webhooks():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_webhooks()
        assert result["total"] == 0


def test_create_webhook():
    fake = {
        "id": "wh-1",
        "label": "Slack alerts",
        "kind": "slack",
        "url": "https://hooks.slack.com/...",
        "enabled": True,
    }
    c, hc = _mock_client(fake)
    with hc:
        result = c.create_webhook(
            "Slack alerts",
            "slack",
            "https://hooks.slack.com/...",
            enabled=True,
            rule={"severity_at_least": "high"},
        )
        assert result["id"] == "wh-1"


def test_test_webhook():
    fake = {"ok": True, "status_code": 200}
    c, hc = _mock_client(fake)
    with hc:
        result = c.test_webhook("wh-1")
        assert result["ok"] is True


def test_delete_webhook():
    fake = {"ok": True}
    c, hc = _mock_client(fake)
    with hc:
        result = c.delete_webhook("wh-1")
        assert result["ok"] is True


# ---------------------------------------------------------------------------
# Metrics
# ---------------------------------------------------------------------------


def test_get_metrics_text():
    prom_text = (
        "# HELP tamga_requests_total Total requests\n"
        "# TYPE tamga_requests_total counter\n"
        "tamga_requests_total 1234\n"
    )
    c, hc = _mock_client_raw(prom_text, content_type="text/plain")
    with hc:
        result = c.get_metrics_text()
        assert "tamga_requests_total" in result


def test_get_histograms():
    fake = {"histograms": [{"name": "scan_latency", "count": 100, "sum": 500.0}]}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_histograms()
        assert len(result["histograms"]) == 1


# ---------------------------------------------------------------------------
# Rate Limit
# ---------------------------------------------------------------------------


def test_get_ratelimit_stats():
    fake = {"enabled": True, "total_requests": 5000, "limited_requests": 42}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_ratelimit_stats()
        assert result["enabled"] is True


# ---------------------------------------------------------------------------
# Patterns
# ---------------------------------------------------------------------------


def test_list_patterns():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_patterns()
        assert result["total"] == 0


def test_create_pattern():
    fake = {
        "id": "pat-1",
        "name": "iban",
        "kind": "regex",
        "pattern": "[A-Z]{2}\\d{2}",
        "severity": "high",
    }
    c, hc = _mock_client(fake)
    with hc:
        result = c.create_pattern("iban", "regex", "[A-Z]{2}\\d{2}", "high")
        assert result["id"] == "pat-1"


def test_update_pattern():
    fake = {
        "id": "pat-1",
        "name": "iban-updated",
        "kind": "regex",
        "pattern": "[A-Z]{2}\\d{2,}",
        "severity": "critical",
    }
    c, hc = _mock_client(fake)
    with hc:
        result = c.update_pattern("pat-1", name="iban-updated", severity="critical")
        assert result["name"] == "iban-updated"


def test_delete_pattern():
    fake = {"ok": True}
    c, hc = _mock_client(fake)
    with hc:
        result = c.delete_pattern("pat-1")
        assert result["ok"] is True


# ---------------------------------------------------------------------------
# Saved Hunts
# ---------------------------------------------------------------------------


def test_list_saved_hunts():
    fake = {"items": [], "total": 0}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_saved_hunts()
        assert result["total"] == 0


def test_create_saved_hunt():
    fake = {"id": "hunt-1", "name": "my-hunt", "query": {"action": "BLOCK"}}
    c, hc = _mock_client(fake)
    with hc:
        result = c.create_saved_hunt("my-hunt", {"action": "BLOCK"})
        assert result["id"] == "hunt-1"


def test_update_saved_hunt():
    fake = {"id": "hunt-1", "name": "updated-hunt", "query": {"action": "WARN"}}
    c, hc = _mock_client(fake)
    with hc:
        result = c.update_saved_hunt("hunt-1", name="updated-hunt")
        assert result["name"] == "updated-hunt"


def test_delete_saved_hunt():
    fake = {"ok": True}
    c, hc = _mock_client(fake)
    with hc:
        result = c.delete_saved_hunt("hunt-1")
        assert result["ok"] is True


# ---------------------------------------------------------------------------
# Team
# ---------------------------------------------------------------------------


def test_list_team():
    fake = {"items": [], "total": 0, "clerk": False}
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_team()
        assert result["clerk"] is False


def test_set_team_role():
    fake = {"user_id": "u1", "email": "a@b.com", "name": "Alice", "role": "admin"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.set_team_role("u1", "admin")
        assert result["role"] == "admin"


# ---------------------------------------------------------------------------
# Billing
# ---------------------------------------------------------------------------


def test_get_pricing():
    fake = {"pricing": [], "currency": "USD", "updated_at": "2025-01-01T00:00:00Z"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_pricing()
        assert result["currency"] == "USD"


def test_get_costs_breakdown():
    fake = {"range": "7d", "breakdown": [], "total_usd": 12.50}
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_costs_breakdown(range="30d")
        assert result["total_usd"] == 12.50


# ---------------------------------------------------------------------------
# Budget
# ---------------------------------------------------------------------------


def test_get_budget_stats():
    fake = {
        "org_id": "org-1",
        "day": "2025-01-01",
        "tokens_today": 50000,
        "cost_today_usd": 1.23,
        "limit_tokens": 1000000,
        "limit_cost_usd": 100.0,
    }
    c, hc = _mock_client(fake)
    with hc:
        result = c.get_budget_stats(org="org-1")
        assert result["org_id"] == "org-1"


# ---------------------------------------------------------------------------
# Maintenance
# ---------------------------------------------------------------------------


def test_run_retention():
    fake = {"ok": True, "last_run": "2025-01-01T00:00:00Z"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.run_retention()
        assert result["ok"] is True


def test_reset_circuit():
    fake = {"ok": True, "pool": "openai", "endpoint": "api.openai.com"}
    c, hc = _mock_client(fake)
    with hc:
        result = c.reset_circuit("openai", "api.openai.com")
        assert result["pool"] == "openai"


# ---------------------------------------------------------------------------
# Providers
# ---------------------------------------------------------------------------


def test_list_providers():
    fake = [
        {"id": "openai", "label": "OpenAI", "path": "/v1", "models": []}
    ]
    c, hc = _mock_client(fake)
    with hc:
        result = c.list_providers()
        assert len(result) == 1
        assert result[0]["id"] == "openai"
