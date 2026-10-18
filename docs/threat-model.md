# Threat model

What Tamga is in a deployment, what it holds, who might attack it, which
controls exist and where each is implemented, and what is left over. Written
on 2026-10-11 against `main` (v0.9.0, unreleased). Paths are relative to the
repository root; a test name is the test that fails if the control stops
working. A control listed here is in the code. It is not a claim that the
control cannot be defeated.

No independent penetration test or security audit of Tamga has been done.
The review behind this page is the maintainer's own.

## The system and its trust boundaries

```
 browser (console) ──────────────────┐ X-Tamga-Admin-Key / session
 browser extension ── POST /api/v1/scan ─┤
 LiteLLM proxy ── guardrail endpoint ────┤
                                         ▼
 applications, agents ── X-Tamga-Key ─► PROXY :8443 ── provider's own key ─► model provider
   (untrusted text inside:               │  policy file (YAML, on disk)
    user input, documents,               ├─► PostgreSQL  request log, audit log, keys, roles
    tool results)                        ├─► Redis       rate limits, cache, vault tokens
                                         ├─► NATS        event stream (7 days)
                                         ├─► analyzer    gRPC + HTTP, token (reports; text only if enabled)
                                         ├─► classifier  gRPC, token (optional)
                                         └─► alert destinations (webhooks, SIEM)
```

| Boundary | Who is on the other side | What crosses it |
|---|---|---|
| Caller → proxy path (`/v1`, `/anthropic`, …) | Applications and agents. Authenticated only if they send a Tamga key; required only with `TAMGA_REQUIRE_KEY=true` | Prompts, tool results, attachments, the caller's provider credential |
| Text inside a request | Whoever wrote the user input, the retrieved document or the tool result. Never trusted | Instructions and data aimed at the model |
| Operator → management API and console (`/api/v1`) | Admin key, API keys (read, write, admin), signed-in users with a role | Policy, keys, roles, alert destinations, stored events |
| Policy file | Whoever can write the file on the host, and the API roles above | Every decision the proxy makes |
| Proxy → PostgreSQL, Redis, NATS, analyzer, classifier, scanner service | The operator's own network. Plain connections in the Compose stack; the analyzer, classifier and scanner service take calls only with a token of their own, and TLS to them is optional | Request metadata, masked findings, vault tokens, events; request text to the analyzer, classifier or scanner service only when enabled |
| Proxy → model provider | A third party | Every request that is not blocked, masked where the policy masks |
| Proxy → alert destinations | Third-party services or internal receivers | The kind of finding and the request id, not the text |
| Browser extension → proxy | A browser on a staff machine, holding a `proxy`-scope key | The text of a message box, for a verdict |
| LiteLLM → proxy | A LiteLLM proxy holding a Tamga key | Texts, tool definitions and tool calls, for a verdict |
| Source → release artefact | GitHub Actions, GHCR, Sigstore, PyPI, npm, base images | The code that runs |

## Assets

1. Request and response text (personal data, secrets, business content). In
   memory while a request is handled; not written to the database.
2. Findings: what was detected, stored with a masked or hashed match.
3. Credentials: the admin key, Tamga API keys, the session secret
   (`TAMGA_JWT_SECRET`), provider keys in pools and passing through, webhook
   tokens, the vault key, the finding hash key, the canary token.
4. The policy: it decides what is blocked, masked and where traffic may go.
5. The audit log (administrative actions) and the request log (decisions).
6. The released binaries and images.

## Adversaries, controls and what remains

### 1. A user of an application: getting data out, or instructions in

| Control | Where |
|---|---|
| PII, secret and injection scanners on each request, by role of the text; Unicode, encoding and obfuscation readings | `proxy/internal/scanner/`, `proxy/internal/scanner/normalize/`; regression gate `.github/workflows/adversarial-gate.yml` with `tests/stress/` |
| REDACT masks every copy of a value, including obfuscated ones, or blocks | `proxy/internal/proxy/redact_matrix_test.go` (`TestRewriteMatrix_Obfuscated`, 873 cells) |
| A scan that fails or is cut short does not forward (shipped policy: `scan.on_error: block`); a masking that could not be completed is refused whatever the setting | `proxy/internal/proxy/scan_error_test.go`, `rewrite_incomplete_test.go` (`TestRequestRewrite_IncompleteScanIsRefused`) |
| A body two JSON parsers read differently is refused (`scan.on_malformed: block`) | `proxy/internal/extract/strict.go`, `strict_test.go` |
| Responses scanned when `output_rules` is on; canary token for system-prompt leaks | `proxy/internal/proxy/output_scan_test.go`; `canary` in the policy |

**Remains.** Detection is pattern-based. Recall is measured only on corpora
the maintainer wrote (0.910 on the tuning corpus, 1.000 on a held-out set by
the same author; [benchmarks](benchmarks/README.md)); expect less on real
traffic. 6 of the 84 vectors of the adversarial suite were published as
bypasses ([tests/stress/baseline.json](../tests/stress/baseline.json)); the
scanners now read three of them and the suite has not been run again.
What the rules miss is semantic: fictional
framings, exfiltration requests, health data, addresses, people's names. The
optional classifier has not been measured on a set its author did not write.
Images, audio and PDFs are forwarded unread. `output_rules` is off in the
shipped policy, and streamed tool-call arguments are never scanned. Traffic
that does not pass through the proxy is not seen at all.

### 2. A poisoned document or tool result (indirect injection)

| Control | Where |
|---|---|
| Tool results, tool descriptions and attachments are scanned in their own role; `STRIP` removes an injected tool result and lets the session continue | `proxy/internal/extract/`, `proxy/internal/policy/strip_test.go`, `proxy/internal/proxy/segment_scan_test.go`; [agents.md](agents.md) |
| The same reading over the LiteLLM bridge (tools, tool calls) | `proxy/internal/proxy/litellm.go`, `litellm_test.go` |

**Remains.** The same detection limits as above. The role of a piece of text
is read from the request body, which the caller writes: a rule narrowed with
`applies_to` trusts the caller to label text correctly. Traffic between an
agent and its MCP servers does not pass through Tamga; a tool result is seen
only when it comes back in the next model request.

### 3. A caller with a valid Tamga key

| Control | Where |
|---|---|
| Organisation and role come from the key's record, not from headers | `proxy/internal/proxy/identity.go`, `identity_test.go` (`TestIdentity_ComesFromTheKeyNotTheHeaders`) |
| Keys stored as SHA-256, compared in constant time; ids say nothing about the key; a `proxy` key is refused by the management API | `proxy/internal/apikeys/store.go`; `proxy/internal/api/refusal_audit_test.go` (`TestRefusalAudit_ProxyKey`) |
| Per-key rate limit, daily token and cost cap, bounded limiter memory | `proxy/internal/ratelimit/`, `proxy/internal/budget/` |
| The response cache never serves one credential's answer to another | `proxy/internal/proxy/cache_identity_test.go` |
| Failover does not send a caller's credential to another provider; pools may point only at the vendor's host or `TAMGA_UPSTREAM_ALLOW` | `proxy/internal/upstream/egress.go`, `proxy/internal/proxy/pool_egress_test.go` |

**Remains.** By default a request with no key is accepted
(`TAMGA_REQUIRE_KEY=false`) and its `X-Tamga-Org-Id` and `X-Tamga-User-Id`
are taken as sent: until the setting is on, anyone who can reach port 8443
is a caller. A revoked key works on other replicas for up to 30 seconds, and
for up to five minutes while the database is unreachable. A key holder can
probe which texts pass (`POST /api/v1/scan` is an oracle by design). Without
Redis, limits are per replica.

### 4. Someone on the network

| Control | Where |
|---|---|
| TLS 1.2+ on the listener when `TAMGA_TLS_CERT` and `TAMGA_TLS_KEY` are set; optional client certificates | `proxy/cmd/tamga/main.go`, `proxy/internal/config/config.go` |
| In the Compose full profile the published ports are HTTPS, through a TLS edge (Caddy); plain HTTP only with `http://` site addresses | `deploy/docker-compose.yml`, `deploy/edge/Caddyfile` |
| A token per internal service (analyzer, classifier, scanner service), compared as hashes in constant time; required at start under `TAMGA_REQUIRE_SERVICE_TOKENS`, which Compose turns on; optional mTLS on those gRPC links | `proxy/internal/grpcauth/grpcauth_test.go`, `proxy/internal/config/service_tokens_test.go`, `analyzer/tests/test_service_auth.py`, `classifier/tests/test_service_auth.py` |
| Credentials are read from headers only, never from the URL | `proxy/internal/api/authz_test.go` (`TestKeyInQueryIsNotACredential`) |
| `X-Forwarded-For` is ignored unless the peer is in `TAMGA_TRUSTED_PROXIES`; CORS is an exact-match list | `proxy/internal/config/trusted_proxies.go`, `proxy/internal/api/router.go` |
| Alerts and provider pools cannot be pointed at internal addresses (SSRF): resolved addresses are judged at connect time, redirects are checked, rebinding is covered | `proxy/internal/egress/guard.go`, `guard_test.go` (`TestGuard_Rebinding`, `TestGuard_RedirectToBlockedIsRefused`); `proxy/internal/api/webhooks_guard_test.go`, `policy_pool_guard_test.go` |
| Without a credential every management path answers 401, existing or not | `authz_test.go` (`TestUnauthenticatedCallerCannotMapRoutes`) |

**Remains.** The Compose minimal profile serves plain HTTP on 8443 unless the
proxy is given a certificate. The full profile's edge uses a local CA by
default, which each client has to be told to trust; an operator who skips
that and turns off verification has no protection against interception. The
edge talks plain HTTP to the proxy and the console on the Compose network.
Connections from the proxy to PostgreSQL, Redis and NATS are not encrypted
in that stack, and NATS has no authentication. The service tokens cross the
Compose network in clear unless gRPC TLS is configured, and the analyzer's
HTTP API has no TLS option; anyone who can read that network's traffic can
take a token. Outside Compose the services accept every call unless they are
given a token. The Jaeger UI (16686) and the NATS monitor (8222) are bound to
`127.0.0.1` and unauthenticated. mTLS on the proxy's own listener has no
test. The Helm chart has no NetworkPolicy and no secret key for the
analyzer token.

### 5. A compromised dependency, image or build

| Control | Where |
|---|---|
| Release artefacts signed with Sigstore keyless, with SLSA provenance and an SBOM per binary archive and image; the build jobs hold no signing identity; actions pinned by commit | `.github/workflows/release.yml`; [verifying-a-release.md](verifying-a-release.md). **The workflow has not run: no release exists and nothing is signed yet** |
| Known-vulnerability checks: `govulncheck` and `gosec` (failing), npm audit and pip-audit (report only) | `.github/workflows/proxy-ci.yml`, `vulnerability-report.yml` |
| Dependency licences judged against a policy, enforced in the release and the weekly licence workflow; the release's CycloneDX dependency SBOMs are written from the same check | `scripts/licence-gate.py`, `.github/licence-policy.yml` |
| The bundled BIN dataset is checked against a recorded SHA-256 | `scripts/verify-bin-data.sh` |
| Containers run as a non-root user; the chart runs with a read-only root filesystem and no capabilities | the Dockerfiles; `deploy/helm/tamga-proxy/templates/deployment.yaml` |

**Remains.** Base images and the Compose images are referenced by tag, not
digest: the digests are still to be filled in (each place is marked `TODO`),
after which Dependabot keeps them current. Python and npm
dependencies below the direct ones are not pinned by hash, and the Go
toolchain and modules are trusted as fetched. The scanner-service image
downloads `grpc_health_probe` at build time without checking a checksum. The
classifier's model is a file the operator downloads; Tamga does not verify
it. The workflows other than the three named above reference actions by
tag. Signing rests on GitHub Actions and Sigstore's public instance.

### 6. A malicious or mistaken administrator

| Control | Where |
|---|---|
| Every management route declares the permission it needs; a route that declares none is refused to everyone | `proxy/internal/api/routes.go`; `authz_test.go` (`TestEveryRouteDeclaresItsPermission`, `TestUndeclaredRouteIsRefused`, `TestAuthorisationMatrix`, `TestPathVariantsDoNotBypass`) |
| Privileged policy sections need the admin role; with `governance.require_dual_control` a second person approves, and turning it off needs one too | `proxy/internal/api/policy_change.go`; `policy_governance_test.go` (`TestPrivilegeSplitOnPut`, `TestDirectWritesRefusedUnderDualControl`, `TestTurningDualControlOffNeedsASecondPerson`) |
| Every policy change, key change and refused credential is written to the audit log with the actor taken from the credential; entries are hash-chained | `policy_governance_test.go` (`TestEveryPolicyChangeIsAudited`, `TestActorComesFromTheCredential`); `proxy/internal/api/refusal_audit.go`; `proxy/internal/incidents/audit.go`, `audit_test.go` |
| Reading or erasing a person's stored data is admin-only and audited without the selector's value | `proxy/internal/api/privacy.go`, `privacy_audit_test.go` |
| Sessions end when a role changes; nobody changes their own role; the last admin cannot be demoted | `proxy/internal/api/session_test.go`, `authz_test.go` (`TestTeamRoleGuards`) |
| The policy mount is read-only by default in Compose and always in the chart | `deploy/docker-compose.yml`; `proxy/internal/api/policy_file_test.go` |

**Remains.** The audit chain is an unkeyed SHA-256 chain: whoever can write
the audit table can rewrite entries and recompute it. `GET
/api/v1/audit/verify` checks the 512 most recent entries the process holds
in memory, not the table. The request log is not chained at all. Nothing is
anchored outside the deployment. The admin key is one shared actor: with
only that key in use, the log cannot say who acted, and dual control cannot
work. An admin can create a second key and approve with it; dual control
stops a mistake or one stolen key, not a determined administrator. Editing
the policy file on disk is outside both, and the log records only that the
file changed. Whoever controls the host, the database or the environment
variables controls everything. The console keeps the key a person enters in
that browser's local storage.

### 7. The model provider

| Control | Where |
|---|---|
| Blocked requests are not sent; masked values are replaced before sending; the vault can restore them only on the way back | `proxy/internal/proxy/handler.go`, `proxy/internal/vault/` |
| Provider allowlist; pools limited to vendors' hosts | `providers` in the policy; `proxy/internal/policy/upstream_rules.go` |
| Nothing goes to the analyzer's LLM judge unless two settings are on, and never a blocked request | `TAMGA_ANALYZER_SEND_CONTENT`, `TAMGA_LLM_JUDGE`; `proxy/internal/events/handlers.go` |

**Remains.** Everything that is not blocked or masked reaches the provider,
which may store it under its own terms. Tamga does not know where a provider
processes data. A response is trusted unless `output_rules` is on.

## What Tamga itself stores and logs

| Control | Where |
|---|---|
| Prompt and response text is not written to the database | [operations: what a stored request holds](operations.md#what-a-stored-request-holds) |
| A scanner's error text, which can quote the request, is not logged: only its kind | `proxy/internal/errkind/`; `proxy/internal/scanner/error_text_test.go` (`TestPipeline_ScannerErrorTextIsNotLogged`) |
| Stored matches are masked (a card keeps its last four digits, a secret its vendor prefix); with `data.hash_findings` they are HMAC-SHA-256 under a deployment key | `proxy/internal/store/finding_hash.go`, `finding_hash_test.go` (`TestHashFindingMatches_IsKeyed`) |
| Request text does not stay in memory after dispatch; the canary token is never in an event | `proxy/internal/events/` |
| Retention job for request logs, audit entries and closed incidents | `TAMGA_RETENTION_*` |

**Remains.** A masked match still narrows a value (first and last two
characters of a national id). Rows written before an upgrade keep their old
form until retention deletes them. With `data.hash_findings` on and no key
set, matches are not stored at all. The database is not encrypted by Tamga.

## The browser extension and the LiteLLM bridge

The extension keeps its key in the service worker, which makes the only
network request, and content scripts get a verdict only
(`extension/src/background/`, `extension/test/`). With no verdict a send is
held unless an administrator sets `failMode: open`. **Remains:** it has been
run on the live sites once, unpacked, in one Chromium browser, and only a
blocked send and a redacted text were seen there
([extension/README.md](../extension/README.md) records what was and was not
tried); the sites change their pages without notice, and an adapter that
stops matching checks nothing. It is in no store; it does not cover uploads,
images, voice, other browsers or the sites' apps; and a user who can remove
an extension can remove it.

The LiteLLM bridge is an adapter over the same scan path
(`proxy/internal/proxy/litellm.go`). **Remains:** the end-to-end test with a
real LiteLLM container is written and has not been run; whether LiteLLM
calls the guardrail at all is LiteLLM's configuration, outside Tamga.

## The project

Tamga is maintained by one person. Reports are answered on a best-effort
target, fixes land on `main` only, and nothing is backported
([SECURITY.md](../SECURITY.md)). No tests run against a real provider or a
real third-party service; the other side is always a stand-in. Several parts
have no end-to-end test (the Azure, Bedrock, Mistral and local routes, SCIM,
mTLS, Redis and NATS against real servers; see the README's project status).

To report a way past a control on this page, follow
[SECURITY.md](../SECURITY.md).
