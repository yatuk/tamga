from typing import Any, Iterator, Optional

import httpx

from tamga.errors import TamgaError

_DEFAULT_TIMEOUT = 30.0


class TamgaClient:
    """Thin wrapper around the Tamga proxy REST API.

    Every request is authenticated with the ``X-Tamga-Admin-Key`` header.
    Callers must supply the proxy base URL (e.g. ``http://localhost:8080``)
    and a valid admin API key.
    """

    def __init__(self, base_url: str, admin_key: str) -> None:
        """Create a new Tamga API client.

        Args:
            base_url: Root URL of the Tamga proxy (e.g. ``http://localhost:8080``).
            admin_key: Admin API key for the proxy.

        Raises:
            ValueError: If *base_url* or *admin_key* is empty or whitespace-only.
        """
        if not base_url or not base_url.strip():
            raise ValueError("base_url must be a non-empty string")
        if not admin_key or not admin_key.strip():
            raise ValueError("admin_key must be a non-empty string")

        self._base_url = base_url.rstrip("/")
        self._headers = {
            "X-Tamga-Admin-Key": admin_key,
            "Accept": "application/json",
        }
        self._client = httpx.Client(
            headers=self._headers,
            timeout=_DEFAULT_TIMEOUT,
        )

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self._client.close()

    def __enter__(self) -> "TamgaClient":
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    def _request(self, method: str, path: str, **kwargs: object) -> Any:  # type: ignore[no-any-return]
        """Issue an HTTP request and return the parsed JSON body.

        Raises:
            TamgaError: On any HTTP error or when the response body is not
                valid JSON.
        """
        url = self._base_url + path
        response = self._client.request(method, url, **kwargs)
        try:
            response.raise_for_status()
        except httpx.HTTPStatusError as exc:
            raise TamgaError(exc.response.status_code, exc.response.text) from exc

        try:
            return response.json()
        except ValueError:
            raise TamgaError(
                response.status_code,
                response.text or "(empty response body)",
            )

    def _request_raw(self, method: str, path: str, **kwargs: object) -> str:
        """Issue an HTTP request and return the raw response text.

        Useful for endpoints that return text/plain or text/csv.

        Raises:
            TamgaError: On any HTTP error.
        """
        url = self._base_url + path
        response = self._client.request(method, url, **kwargs)
        try:
            response.raise_for_status()
        except httpx.HTTPStatusError as exc:
            raise TamgaError(exc.response.status_code, exc.response.text) from exc
        return response.text

    def _build_query(self, **kwargs: object) -> dict[str, object]:
        """Build a query-params dict, dropping None values."""
        return {k: v for k, v in kwargs.items() if v is not None}

    # ==================================================================
    # Health
    # ==================================================================

    def health(self) -> dict:
        """Basic health check (no auth required).

        Corresponds to ``GET /health``.
        """
        return self._request("GET", "/health")

    def health_detailed(self) -> dict:
        """Detailed health status with provider pool snapshots.

        Corresponds to ``GET /api/v1/health/detailed``.
        """
        return self._request("GET", "/api/v1/health/detailed")

    def health_detail(self) -> dict:
        """Runtime operational profile (TLS, mTLS, Redis, Postgres, tier, etc.).

        Corresponds to ``GET /api/v1/health/detail``.
        """
        return self._request("GET", "/api/v1/health/detail")

    # ==================================================================
    # Stats
    # ==================================================================

    def get_stats(self, range: str = "7d") -> dict:
        """Retrieve dashboard overview statistics.

        Corresponds to ``GET /api/v1/stats``.

        Args:
            range: Time window (``"24h"``, ``"7d"``, or ``"30d"``).
        """
        return self._request("GET", "/api/v1/stats", params={"range": range})

    def get_model_stats(self, range: str = "7d") -> dict:
        """Per-model and per-family request counts.

        Corresponds to ``GET /api/v1/stats/models``.

        Args:
            range: Time window (``"24h"``, ``"7d"``, or ``"30d"``).
        """
        return self._request(
            "GET", "/api/v1/stats/models", params={"range": range}
        )

    # ==================================================================
    # Events
    # ==================================================================

    def get_events(
        self,
        page: int = 1,
        limit: int = 50,
        *,
        action: Optional[str] = None,
        provider: Optional[str] = None,
        shadow: Optional[bool] = None,
        finding_type: Optional[str] = None,
        severity: Optional[str] = None,
        category: Optional[str] = None,
        technique: Optional[str] = None,
        q: Optional[str] = None,
        range: Optional[str] = None,
        since: Optional[str] = None,
        until: Optional[str] = None,
    ) -> dict:
        """List security events with pagination and optional filters.

        Corresponds to ``GET /api/v1/events``.

        Args:
            page: Page number (1-indexed).
            limit: Items per page (max 200).
            action: Filter by action (BLOCK, REDACT, WARN, PASS, ...).
            provider: Filter by provider name (lowercase) or ``"shadow"``.
            shadow: If True, return only non-enterprise providers.
            finding_type: Filter by finding type (e.g. ``"pii"``).
            severity: Filter by severity (e.g. ``"high"``).
            category: Substring match on finding category.
            technique: Substring match on OWASP code within findings JSON.
            q: Free-text search on request_id or findings text.
            range: Time window (``"24h"``, ``"7d"``, ``"30d"``).
            since: RFC3339 start time (overrides *range*).
            until: RFC3339 end time.
        """
        params = self._build_query(
            page=page,
            limit=limit,
            action=action,
            provider=provider,
            shadow=shadow,
            finding_type=finding_type,
            severity=severity,
            category=category,
            technique=technique,
            q=q,
            range=range,
            since=since,
            until=until,
        )
        return self._request("GET", "/api/v1/events", params=params)

    def get_event_detail(self, request_id: str) -> dict:
        """Get a single security event by request ID.

        Corresponds to ``GET /api/v1/events/{request_id}``.

        Args:
            request_id: The unique request ID.
        """
        return self._request("GET", f"/api/v1/events/{request_id}")

    def export_events(
        self,
        format: str = "csv",
        *,
        action: Optional[str] = None,
        provider: Optional[str] = None,
        range: Optional[str] = None,
        request_id: Optional[str] = None,
    ) -> str:
        """Export filtered events as CSV or JSON.

        Corresponds to ``GET /api/v1/events/export``.

        Args:
            format: ``"csv"`` or ``"json"``. Defaults to ``"csv"``.
            action: Filter by action.
            provider: Filter by provider.
            range: Time window.
            request_id: Filter by request ID.

        Returns:
            Raw response text (CSV string or JSON string).
        """
        params = self._build_query(
            format=format,
            action=action,
            provider=provider,
            range=range,
            request_id=request_id,
        )
        return self._request_raw("GET", "/api/v1/events/export", params=params)

    def get_subject_events(
        self,
        *,
        user_id: Optional[str] = None,
        email: Optional[str] = None,
        tckn_hash: Optional[str] = None,
    ) -> dict:
        """GDPR Art. 15 / KVKK madde 11 subject access request.

        Corresponds to ``GET /api/v1/events/subject``.

        Args:
            user_id: Subject user ID.
            email: Subject email address.
            tckn_hash: SHA-256 of the subject's TCKN.
        """
        params = self._build_query(user_id=user_id, email=email, tckn_hash=tckn_hash)
        return self._request("GET", "/api/v1/events/subject", params=params)

    def erase_subject(
        self,
        *,
        user_id: Optional[str] = None,
        email: Optional[str] = None,
        tckn_hash: Optional[str] = None,
    ) -> dict:
        """KVKK/GDPR subject erase.

        Corresponds to ``DELETE /api/v1/events/subject``.

        Args:
            user_id: Subject user ID.
            email: Subject email address.
            tckn_hash: SHA-256 of the subject's TCKN.
        """
        body: dict[str, str] = {}
        if user_id:
            body["user_id"] = user_id
        elif email:
            body["email"] = email
        elif tckn_hash:
            body["tckn_hash"] = tckn_hash
        return self._request("DELETE", "/api/v1/events/subject", json=body)

    # ==================================================================
    # Timeseries
    # ==================================================================

    def get_timeseries(
        self,
        range: str = "7d",
        bucket: Optional[str] = None,
    ) -> dict:
        """Bucketed time series for the overview chart.

        Corresponds to ``GET /api/v1/timeseries``.

        Args:
            range: Time window (``"24h"``, ``"7d"``, ``"30d"``).
            bucket: ``"hour"`` or ``"day"``. Defaults to hour for 24h, day otherwise.
        """
        params = self._build_query(range=range, bucket=bucket)
        return self._request("GET", "/api/v1/timeseries", params=params)

    # ==================================================================
    # Findings
    # ==================================================================

    def get_findings_breakdown(self, range: str = "7d") -> dict:
        """Fetch a breakdown of findings by type, category, and severity.

        Corresponds to ``GET /api/v1/findings/breakdown``.

        Args:
            range: Time window (``"24h"``, ``"7d"``, or ``"30d"``).
        """
        return self._request(
            "GET",
            "/api/v1/findings/breakdown",
            params={"range": range},
        )

    # ==================================================================
    # Incidents
    # ==================================================================

    def list_incidents(self, limit: int = 200) -> dict:
        """List incidents.

        Corresponds to ``GET /api/v1/incidents``.

        Args:
            limit: Maximum number of incidents to return.
        """
        return self._request(
            "GET", "/api/v1/incidents", params={"limit": limit}
        )

    def get_incident(self, request_id: str) -> dict:
        """Get a single incident by request ID.

        Corresponds to ``GET /api/v1/incidents/{request_id}``.

        Args:
            request_id: The unique request ID.
        """
        return self._request("GET", f"/api/v1/incidents/{request_id}")

    def patch_incident(
        self,
        request_id: str,
        *,
        status: Optional[str] = None,
        assignee: Optional[str] = None,
        reason: Optional[str] = None,
        tags: Optional[list[str]] = None,
        add_comment: Optional[dict[str, str]] = None,
    ) -> dict:
        """Patch an incident (status, assignee, tags, comment).

        Corresponds to ``PATCH /api/v1/incidents/{request_id}``.

        Args:
            request_id: The unique request ID.
            status: New status (Open, In Progress, Closed, False Positive).
            assignee: Assignee name.
            reason: Reason for the change.
            tags: List of tags.
            add_comment: Dict with ``author`` and ``text`` keys.
        """
        body: dict[str, object] = {}
        if status is not None:
            body["status"] = status
        if assignee is not None:
            body["assignee"] = assignee
        if reason is not None:
            body["reason"] = reason
        if tags is not None:
            body["tags"] = tags
        if add_comment is not None:
            body["add_comment"] = add_comment
        return self._request("PATCH", f"/api/v1/incidents/{request_id}", json=body)

    def triage_incident(self, request_id: str, assignee: Optional[str] = None) -> dict:
        """Triage an incident (assign + set In Progress).

        Corresponds to ``POST /api/v1/incidents/{request_id}/triage``.

        Args:
            request_id: The unique request ID.
            assignee: Assignee name.
        """
        body: dict[str, str] = {}
        if assignee is not None:
            body["assignee"] = assignee
        return self._request("POST", f"/api/v1/incidents/{request_id}/triage", json=body)

    def resolve_incident(
        self,
        request_id: str,
        resolution: str = "true_positive",
        notes: Optional[str] = None,
    ) -> dict:
        """Resolve an incident.

        Corresponds to ``POST /api/v1/incidents/{request_id}/resolve``.

        Args:
            request_id: The unique request ID.
            resolution: One of ``"true_positive"``, ``"false_positive"``,
                ``"accepted_risk"``, ``"remediated"``.
            notes: Optional resolution notes.
        """
        body: dict[str, str] = {"resolution": resolution}
        if notes is not None:
            body["notes"] = notes
        return self._request("POST", f"/api/v1/incidents/{request_id}/resolve", json=body)

    def reopen_incident(self, request_id: str) -> dict:
        """Reopen a resolved incident.

        Corresponds to ``POST /api/v1/incidents/{request_id}/reopen``.

        Args:
            request_id: The unique request ID.
        """
        return self._request("POST", f"/api/v1/incidents/{request_id}/reopen")

    # ==================================================================
    # MTTR
    # ==================================================================

    def get_mttr(
        self,
        range: str = "7d",
        org_id: Optional[str] = None,
    ) -> dict:
        """Mean Time to Resolution statistics.

        Corresponds to ``GET /api/v1/mttr``.

        Args:
            range: Time window (``"24h"``, ``"7d"``, ``"30d"``).
            org_id: Optional organisation ID filter.
        """
        params = self._build_query(range=range, org_id=org_id)
        return self._request("GET", "/api/v1/mttr", params=params)

    # ==================================================================
    # Audit
    # ==================================================================

    def get_audit_log(self, limit: int = 200) -> dict:
        """List audit log entries.

        Corresponds to ``GET /api/v1/auditlog``.

        Args:
            limit: Maximum number of entries to return.
        """
        return self._request(
            "GET", "/api/v1/auditlog", params={"limit": limit}
        )

    def verify_audit_chain(self) -> dict:
        """Verify the audit hash-chain integrity.

        Returns ``chain_ok`` and ``broken_at`` (if tampered).

        Corresponds to ``GET /api/v1/audit/verify``.
        """
        return self._request("GET", "/api/v1/audit/verify")

    # ==================================================================
    # Live
    # ==================================================================

    def open_live_events(self) -> Iterator[str]:
        """Open a Server-Sent Events stream of scanned/blocked requests.

        Corresponds to ``GET /api/v1/live/events``.

        Yields SSE event data lines as they arrive. The caller must iterate
        to consume the stream and handle ``httpx.RemoteProtocolError`` on
        disconnect.

        Example::

            for line in client.open_live_events():
                if line.startswith("data:"):
                    print(line)
        """
        url = self._base_url + "/api/v1/live/events"
        with self._client.stream("GET", url) as response:
            try:
                response.raise_for_status()
            except httpx.HTTPStatusError as exc:
                raise TamgaError(exc.response.status_code, exc.response.text) from exc
            for line in response.iter_lines():
                yield line

    # ==================================================================
    # Policies
    # ==================================================================

    def get_policies(self) -> list:
        """Get the active policy document.

        Corresponds to ``GET /api/v1/policies``.
        """
        return self._request("GET", "/api/v1/policies")

    def put_policy(self, yaml: str) -> dict:
        """Overwrite the on-disk policy YAML and reload.

        Corresponds to ``PUT /api/v1/policies``.

        Args:
            yaml: Raw YAML policy document string.
        """
        return self._request("PUT", "/api/v1/policies", json={"yaml": yaml})

    def reload_policy(self) -> dict:
        """Trigger a policy reload from disk without changing content.

        Corresponds to ``POST /api/v1/policies/reload``.
        """
        return self._request("POST", "/api/v1/policies/reload")

    def validate_policy(self, yaml: str) -> dict:
        """Validate policy YAML/JSON without persisting (dry-run).

        Corresponds to ``POST /api/v1/policies/validate``.

        Args:
            yaml: Raw YAML policy document string to validate.
        """
        return self._request("POST", "/api/v1/policies/validate", json={"yaml": yaml})

    def simulate_policy(self, sample_text: str, yaml: Optional[str] = None) -> dict:
        """Dry-run scan against a policy with sample text.

        Corresponds to ``POST /api/v1/policies/simulate``.

        Args:
            sample_text: Text to scan.
            yaml: Optional policy YAML. If omitted, the active policy is used.
        """
        body: dict[str, str] = {"sample_text": sample_text}
        if yaml is not None:
            body["yaml"] = yaml
        return self._request("POST", "/api/v1/policies/simulate", json=body)

    def list_policy_revisions(self) -> list:
        """List policy revision history.

        Corresponds to ``GET /api/v1/policies/history``.
        """
        return self._request("GET", "/api/v1/policies/history")

    def get_policy_revision(self, revision_id: str) -> dict:
        """Get a single policy revision by ID.

        Corresponds to ``GET /api/v1/policies/revisions/{id}``.

        Args:
            revision_id: The revision ID.
        """
        return self._request("GET", f"/api/v1/policies/revisions/{revision_id}")

    def rollback_policy(self, revision_id: str) -> dict:
        """Roll back to a previous policy revision.

        Corresponds to ``POST /api/v1/policies/rollback/{id}``.

        Args:
            revision_id: The revision ID to roll back to.
        """
        return self._request("POST", f"/api/v1/policies/rollback/{revision_id}")

    def list_custom_entities(self) -> dict:
        """List custom entities from the active policy.

        Corresponds to ``GET /api/v1/policies/custom-entities``.
        """
        return self._request("GET", "/api/v1/policies/custom-entities")

    def create_custom_entity(
        self,
        name: str,
        pattern: str,
        *,
        description: Optional[str] = None,
        severity: Optional[str] = None,
        action: Optional[str] = None,
        confidence: Optional[float] = None,
    ) -> dict:
        """Add a custom entity to the policy.

        Corresponds to ``POST /api/v1/policies/custom-entities``.

        Args:
            name: Entity name (unique).
            pattern: Regex pattern.
            description: Optional description.
            severity: Severity level (critical, high, medium, low).
            action: Action to take on match.
            confidence: Confidence score.
        """
        body: dict[str, object] = {"name": name, "pattern": pattern}
        if description is not None:
            body["description"] = description
        if severity is not None:
            body["severity"] = severity
        if action is not None:
            body["action"] = action
        if confidence is not None:
            body["confidence"] = confidence
        return self._request("POST", "/api/v1/policies/custom-entities", json=body)

    def delete_custom_entity(self, name: str) -> dict:
        """Remove a custom entity by name and reload.

        Corresponds to ``DELETE /api/v1/policies/custom-entities/{name}``.

        Args:
            name: The entity name to delete.
        """
        return self._request("DELETE", f"/api/v1/policies/custom-entities/{name}")

    def list_policy_proposals(self) -> list:
        """List pending policy proposals.

        Corresponds to ``GET /api/v1/policies/proposals``.
        """
        return self._request("GET", "/api/v1/policies/proposals")

    def create_policy_proposal(self, message: str, yaml: str) -> dict:
        """Create a policy proposal (requires two-person approval).

        Corresponds to ``POST /api/v1/policies/proposals``.

        Args:
            message: Description of the proposed change.
            yaml: Proposed policy YAML.
        """
        return self._request(
            "POST",
            "/api/v1/policies/proposals",
            json={"message": message, "yaml": yaml},
        )

    def approve_policy_proposal(self, proposal_id: str) -> dict:
        """Approve a pending policy proposal.

        Corresponds to ``POST /api/v1/policies/proposals/{id}/approve``.

        Args:
            proposal_id: The proposal ID.
        """
        return self._request(
            "POST", f"/api/v1/policies/proposals/{proposal_id}/approve"
        )

    def reject_policy_proposal(self, proposal_id: str) -> dict:
        """Reject a pending policy proposal.

        Corresponds to ``POST /api/v1/policies/proposals/{id}/reject``.

        Args:
            proposal_id: The proposal ID.
        """
        return self._request(
            "POST", f"/api/v1/policies/proposals/{proposal_id}/reject"
        )

    # ==================================================================
    # API Keys
    # ==================================================================

    def list_api_keys(self) -> dict:
        """List scoped API keys (prefix only, raw key not returned).

        Corresponds to ``GET /api/v1/apikeys``.
        """
        return self._request("GET", "/api/v1/apikeys")

    def create_api_key(self, label: str, scope: str = "read") -> dict:
        """Create a new scoped API key.

        The response includes ``raw_key`` — this is the only time the
        full key is revealed.

        Corresponds to ``POST /api/v1/apikeys``.

        Args:
            label: Human-readable label for the key.
            scope: ``"read"``, ``"write"``, or ``"admin"``. Defaults to ``"read"``.
        """
        return self._request(
            "POST",
            "/api/v1/apikeys",
            json={"label": label, "scope": scope},
        )

    def delete_api_key(self, key_id: str) -> dict:
        """Revoke an API key.

        Corresponds to ``DELETE /api/v1/apikeys/{id}``.

        Args:
            key_id: The key ID to revoke.
        """
        return self._request("DELETE", f"/api/v1/apikeys/{key_id}")

    # ==================================================================
    # Webhooks
    # ==================================================================

    def list_webhooks(self) -> dict:
        """List configured webhooks.

        Corresponds to ``GET /api/v1/webhooks``.
        """
        return self._request("GET", "/api/v1/webhooks")

    def create_webhook(
        self,
        label: str,
        kind: str,
        url: str,
        *,
        enabled: Optional[bool] = None,
        rule: Optional[dict] = None,
        headers: Optional[dict[str, str]] = None,
        payload_template: Optional[str] = None,
        project_key: Optional[str] = None,
        issue_type: Optional[str] = None,
        auth_token: Optional[str] = None,
    ) -> dict:
        """Create a new webhook destination.

        Corresponds to ``POST /api/v1/webhooks``.

        Args:
            label: Human-readable label.
            kind: Integration kind (slack, teams, splunk, pagerduty, etc.).
            url: Webhook URL.
            enabled: Whether the webhook is active.
            rule: Dict with ``blocks_per_minute`` and/or ``severity_at_least``.
            headers: Custom HTTP headers to send.
            payload_template: Custom payload template string.
            project_key: Jira project key (for Jira kind).
            issue_type: Issue type (for Jira kind).
            auth_token: Auth token for the destination.
        """
        body: dict[str, object] = {"label": label, "kind": kind, "url": url}
        if enabled is not None:
            body["enabled"] = enabled
        if rule is not None:
            body["rule"] = rule
        if headers is not None:
            body["headers"] = headers
        if payload_template is not None:
            body["payload_template"] = payload_template
        if project_key is not None:
            body["project_key"] = project_key
        if issue_type is not None:
            body["issue_type"] = issue_type
        if auth_token is not None:
            body["auth_token"] = auth_token
        return self._request("POST", "/api/v1/webhooks", json=body)

    def test_webhook(self, webhook_id: str) -> dict:
        """Send a test event to a webhook.

        Corresponds to ``POST /api/v1/webhooks/{id}/test``.

        Args:
            webhook_id: The webhook ID.
        """
        return self._request("POST", f"/api/v1/webhooks/{webhook_id}/test")

    def delete_webhook(self, webhook_id: str) -> dict:
        """Delete a webhook.

        Corresponds to ``DELETE /api/v1/webhooks/{id}``.

        Args:
            webhook_id: The webhook ID.
        """
        return self._request("DELETE", f"/api/v1/webhooks/{webhook_id}")

    # ==================================================================
    # Metrics
    # ==================================================================

    def get_metrics_text(self) -> str:
        """Get Prometheus text-format metrics snapshot.

        Returns raw Prometheus exposition format text.

        Corresponds to ``GET /api/v1/metrics``.
        """
        return self._request_raw("GET", "/api/v1/metrics")

    def get_histograms(self) -> dict:
        """Get Prometheus histogram metrics as structured JSON.

        Corresponds to ``GET /api/v1/metrics/histograms``.
        """
        return self._request("GET", "/api/v1/metrics/histograms")

    # ==================================================================
    # Rate Limit
    # ==================================================================

    def get_ratelimit_stats(self) -> dict:
        """Rate limiter statistics snapshot.

        Corresponds to ``GET /api/v1/ratelimit/stats``.
        """
        return self._request("GET", "/api/v1/ratelimit/stats")

    # ==================================================================
    # Patterns
    # ==================================================================

    def list_patterns(self) -> dict:
        """List custom detection patterns.

        Corresponds to ``GET /api/v1/patterns``.
        """
        return self._request("GET", "/api/v1/patterns")

    def create_pattern(
        self,
        name: str,
        kind: str,
        pattern: str,
        severity: str,
        enabled: Optional[bool] = None,
    ) -> dict:
        """Create a new detection pattern.

        Corresponds to ``POST /api/v1/patterns``.

        Args:
            name: Pattern name.
            kind: ``"regex"`` or ``"literal"``.
            pattern: The pattern string.
            severity: ``"low"``, ``"medium"``, ``"high"``, or ``"critical"``.
            enabled: Whether the pattern is active.
        """
        body: dict[str, object] = {
            "name": name,
            "kind": kind,
            "pattern": pattern,
            "severity": severity,
        }
        if enabled is not None:
            body["enabled"] = enabled
        return self._request("POST", "/api/v1/patterns", json=body)

    def update_pattern(
        self,
        pattern_id: str,
        *,
        name: Optional[str] = None,
        kind: Optional[str] = None,
        pattern: Optional[str] = None,
        severity: Optional[str] = None,
        enabled: Optional[bool] = None,
    ) -> dict:
        """Update an existing detection pattern.

        Corresponds to ``PUT /api/v1/patterns/{id}``.

        Args:
            pattern_id: The pattern ID.
            name: New name.
            kind: ``"regex"`` or ``"literal"``.
            pattern: New pattern string.
            severity: ``"low"``, ``"medium"``, ``"high"``, ``"critical"``.
            enabled: Whether the pattern is active.
        """
        body: dict[str, object] = {}
        if name is not None:
            body["name"] = name
        if kind is not None:
            body["kind"] = kind
        if pattern is not None:
            body["pattern"] = pattern
        if severity is not None:
            body["severity"] = severity
        if enabled is not None:
            body["enabled"] = enabled
        return self._request("PUT", f"/api/v1/patterns/{pattern_id}", json=body)

    def delete_pattern(self, pattern_id: str) -> dict:
        """Delete a detection pattern.

        Corresponds to ``DELETE /api/v1/patterns/{id}``.

        Args:
            pattern_id: The pattern ID.
        """
        return self._request("DELETE", f"/api/v1/patterns/{pattern_id}")

    # ==================================================================
    # Saved Hunts
    # ==================================================================

    def list_saved_hunts(self) -> dict:
        """List saved threat-hunting queries.

        Corresponds to ``GET /api/v1/saved-hunts``.
        """
        return self._request("GET", "/api/v1/saved-hunts")

    def create_saved_hunt(self, name: str, query_json: dict) -> dict:
        """Save a new threat-hunting query.

        Corresponds to ``POST /api/v1/saved-hunts``.

        Args:
            name: Display name for the saved hunt.
            query_json: The hunt query object.
        """
        return self._request(
            "POST",
            "/api/v1/saved-hunts",
            json={"name": name, "query_json": query_json},
        )

    def update_saved_hunt(
        self,
        hunt_id: str,
        *,
        name: Optional[str] = None,
        query_json: Optional[dict] = None,
    ) -> dict:
        """Update a saved hunt.

        Corresponds to ``PUT /api/v1/saved-hunts/{id}``.

        Args:
            hunt_id: The saved hunt ID.
            name: New display name.
            query_json: Updated query object.
        """
        body: dict[str, object] = {}
        if name is not None:
            body["name"] = name
        if query_json is not None:
            body["query_json"] = query_json
        return self._request("PUT", f"/api/v1/saved-hunts/{hunt_id}", json=body)

    def delete_saved_hunt(self, hunt_id: str) -> dict:
        """Delete a saved hunt.

        Corresponds to ``DELETE /api/v1/saved-hunts/{id}``.

        Args:
            hunt_id: The saved hunt ID.
        """
        return self._request("DELETE", f"/api/v1/saved-hunts/{hunt_id}")

    # ==================================================================
    # Team
    # ==================================================================

    def list_team(self) -> dict:
        """List team members with roles.

        Corresponds to ``GET /api/v1/team``.
        """
        return self._request("GET", "/api/v1/team")

    def set_team_role(self, user_id: str, role: str) -> dict:
        """Set a team member's role.

        Corresponds to ``PUT /api/v1/team/{id}/role``.

        Args:
            user_id: The user ID.
            role: ``"admin"``, ``"analyst"``, or ``"viewer"``.
        """
        return self._request(
            "PUT",
            f"/api/v1/team/{user_id}/role",
            json={"role": role},
        )

    # ==================================================================
    # Billing
    # ==================================================================

    def get_pricing(self) -> dict:
        """List active model pricing entries.

        Corresponds to ``GET /api/v1/billing/pricing``.
        """
        return self._request("GET", "/api/v1/billing/pricing")

    def get_costs_breakdown(self, range: str = "7d") -> dict:
        """Per-model cost breakdown for a time range.

        Corresponds to ``GET /api/v1/billing/costs/breakdown``.

        Args:
            range: Time window (``"24h"``, ``"7d"``, ``"30d"``).
        """
        return self._request(
            "GET",
            "/api/v1/billing/costs/breakdown",
            params={"range": range},
        )

    # ==================================================================
    # Budget
    # ==================================================================

    def get_budget_stats(self, org: Optional[str] = None) -> dict:
        """Daily token and cost counters for an organisation.

        Corresponds to ``GET /api/v1/budget/stats``.

        Args:
            org: Organisation ID.
        """
        params = self._build_query(org=org)
        return self._request("GET", "/api/v1/budget/stats", params=params)

    # ==================================================================
    # Maintenance
    # ==================================================================

    def run_retention(self) -> dict:
        """Trigger a retention maintenance cycle.

        Corresponds to ``POST /api/v1/maintenance/retention``.
        """
        return self._request("POST", "/api/v1/maintenance/retention")

    def reset_circuit(self, pool: str, endpoint: str) -> dict:
        """Reset a circuit breaker for an upstream pool/endpoint.

        Corresponds to ``POST /api/v1/maintenance/circuit-reset``.

        Args:
            pool: The upstream pool name.
            endpoint: The endpoint name within the pool.
        """
        return self._request(
            "POST",
            "/api/v1/maintenance/circuit-reset",
            json={"pool": pool, "endpoint": endpoint},
        )

    # ==================================================================
    # Providers
    # ==================================================================

    def list_providers(self) -> list:
        """List supported providers and models (with pricing).

        Corresponds to ``GET /api/v1/providers``.
        """
        return self._request("GET", "/api/v1/providers")
