# Tamga Architecture

All architecture diagrams in one place. D2 sources and rendered SVG/PNG
versions live alongside this file.

## System overview

![Tamga Architecture](tamga.svg)

View the [D2 source](tamga.d2).

| Component | Language | Role | Port |
|---|---|---|---|
| Proxy | Go | Inline scanners, policy engine, reverse proxy, rate limiter, management API. One listener for all of it | `8443` |
| Scanner service | Go | Optional. Runs the stateless scanners over gRPC when `TAMGA_SCANNER_SERVICE_ADDR` is set | `50052` |
| Classifier | Python | Optional. Local prompt-injection model, asked inline when the rules did not block | `50052` |
| Analyzer | Python | The PDF compliance reports. Optionally (`TAMGA_ANALYZER_SEND_CONTENT`) a second look at forwarded requests after the decision: Presidio NER, and an LLM judge when `TAMGA_LLM_JUDGE` turns it on. Its findings are logged as a count; they change no verdict and are not stored | `50051` gRPC, `8444` HTTP |
| Dashboard | Next.js | The console. It runs in the browser and calls the proxy's management API directly | `3000` |
| PostgreSQL | — | Optional persistent telemetry and audit storage | `5432` |
| Redis | — | Rate limiting, caching, distributed counters | `6379` |
| NATS | — | Optional JetStream event stream, used when `TAMGA_NATS_URL` is set | `4222` |
| Jaeger | — | OpenTelemetry tracing (optional) | `16686` UI, `4318` OTLP |

## Deployment topology

`deploy/docker-compose.yml` puts every service on one internal bridge network
(`deploy_internal`). The proxy (`:8443`) and the dashboard (`:3000`) are
published to the host for use; the file also publishes the Jaeger UI
(`:16686`) and the NATS monitoring page (`:8222`) for local development.
PostgreSQL, Redis, the analyzer and the scanner service are reachable only
inside the network. The diagram leaves out NATS, the scanner service and the
optional classifier.

```mermaid
%%{init: {
  'theme': 'dark',
  'themeVariables': {
    'primaryColor': '#1e3a5f',
    'primaryTextColor': '#dbeafe',
    'primaryBorderColor': '#3b82f6',
    'lineColor': '#6b7280',
    'background': '#0f172a',
    'mainBkg': '#1e293b'
  }
}}%%
graph TB
    subgraph "Your Infrastructure"
        Client[Client App]
    end

    subgraph "Tamga Stack (docker-compose)"
        subgraph "Edge"
            Proxy[Tamga Proxy<br/>:8443]
        end

        subgraph "Inspection"
            Analyzer[Analyzer<br/>:50051 gRPC<br/>:8444 HTTP]
        end

        subgraph "Data"
            Postgres[(PostgreSQL 16<br/>:5432)]
            Redis[(Redis 7<br/>:6379)]
        end

        subgraph "Observability"
            Jaeger[Jaeger<br/>:16686 UI<br/>:4318 OTLP]
        end

        subgraph "Management"
            Dashboard[Dashboard<br/>:3000 Next.js]
        end
    end

    Client -->|HTTP, or HTTPS with TAMGA_TLS_CERT| Proxy
    Proxy -->|gRPC| Analyzer
    Proxy -->|SQL| Postgres
    Proxy -->|cache + rate limit| Redis
    Proxy -->|OTLP traces| Jaeger
    Dashboard -->|management API, from the browser| Proxy

    classDef edge fill:#1e3a5f,stroke:#3b82f6,color:#dbeafe
    classDef inspection fill:#0d3a3a,stroke:#14b8a6,color:#ccfbf1
    classDef data fill:#2d1b4e,stroke:#a855f7,color:#e9d5ff

    class Proxy edge
    class Analyzer inspection
    class Postgres,Redis data
```

View the [full-size deployment diagram](deployment.svg) or [D2 source](deployment.d2).

## Request lifecycle

What happens when your app sends an LLM request through Tamga:

```mermaid
%%{init: {
  'theme': 'dark',
  'themeVariables': {
    'primaryColor': '#1e3a5f',
    'primaryTextColor': '#dbeafe',
    'primaryBorderColor': '#3b82f6',
    'lineColor': '#6b7280',
    'background': '#0f172a',
    'mainBkg': '#1e293b'
  }
}}%%
sequenceDiagram
    autonumber
    participant App as Your App
    participant Proxy as Tamga Proxy
    participant Scanner as Scanner Pipeline
    participant Policy as Policy Engine
    participant Classifier as Classifier (optional)
    participant LLM as LLM Provider
    participant DB as PostgreSQL
    participant Analyzer as Analyzer (Python)

    App->>Proxy: POST /v1/messages
    Note over Proxy: Verify X-Tamga-Key<br/>Check rate limit

    Proxy->>Scanner: Inline scan, per message
    Note over Scanner: Deterministic scanners<br/>(under 1 ms)
    Scanner-->>Proxy: Findings

    Proxy->>Policy: Evaluate findings

    opt scan.classifier on, and not already blocked
        Proxy->>Classifier: gRPC Classify (user and tool text)
        Note over Classifier: Local ONNX model<br/>about 15-30 ms, 150 ms deadline
        Classifier-->>Proxy: Score per text
        Note over Proxy: A score over the threshold<br/>is one more finding
    end

    alt action = block
        Policy-->>Proxy: BLOCK
        Proxy-->>App: 403 Forbidden + reasons
        Proxy->>DB: Log incident (async)
    else action = redact
        Policy-->>Proxy: REDACT
        Note over Proxy: Mask findings in payload
        Proxy->>LLM: Forward redacted request
        LLM-->>Proxy: Response
        Proxy->>Scanner: Output scan
        Proxy-->>App: 200 OK + scan headers
    else action = pass
        Policy-->>Proxy: PASS
        Proxy->>LLM: Forward original
        LLM-->>Proxy: Response
        Proxy->>Scanner: Output scan
        Proxy-->>App: 200 OK
    end

    Proxy->>DB: Persist request_log (batched)
    Proxy-)Analyzer: Forwarded request, if content analysis is on (async, after the response)
    Note over Analyzer: Presidio NER, optional LLM judge<br/>findings are logged as a count,<br/>not stored, never change the verdict
```

Scan overhead is a small fraction of total request time — provider latency
is usually 500-3000ms. See [benchmarks](../benchmarks/README.md) for
measured scan-stage numbers.

View the [D2 source](request-lifecycle.d2) or [full-size SVG](request-lifecycle.svg).

## Scanner pipeline

Seven inline scanners run on every request, and `operator_state` as an eighth
when it is configured. In the default `adaptive` mode the fast scanners run in
sequence and the slow ones in parallel. The diagram gives no timings; see
[benchmarks](../benchmarks/README.md) for measured numbers.

```mermaid
%%{init: {
  'theme': 'dark',
  'themeVariables': {
    'primaryColor': '#1e3a5f',
    'primaryTextColor': '#dbeafe',
    'primaryBorderColor': '#3b82f6',
    'lineColor': '#6b7280',
    'background': '#0f172a',
    'mainBkg': '#1e293b'
  }
}}%%
graph LR
    Input[Request Content] --> Normalize[Unicode<br/>Normalization]

    Normalize --> Fast{Fast<br/>Scanners}
    Normalize --> Slow{Slow<br/>Scanners}

    Fast -.-> PII[PII<br/>email, TC, IBAN]
    Fast -.-> Secret[Secrets<br/>API keys, tokens]
    Fast -.-> Comp[Competitor<br/>brand names]
    Fast -.-> Custom[Custom Entity<br/>policy-driven]

    Slow -.-> Inject[Injection<br/>phrases and patterns]
    Slow -.-> Mod[Moderation<br/>toxicity, hate]
    Slow -.-> Jail[Jailbreak<br/>DAN, STAN]

    PII --> Merge[Findings<br/>Aggregator]
    Secret --> Merge
    Comp --> Merge
    Custom --> Merge
    Inject --> Merge
    Mod --> Merge
    Jail --> Merge

    Merge --> Verdict{Policy:<br/>blocked?}
    Verdict -->|yes| Decisive[403]
    Verdict -->|no, classifier on| Cls[Classifier gRPC<br/>local model]
    Verdict -->|no, classifier off| Forward[Forward]
    Cls --> Forward

    classDef fast fill:#1e3a5f,stroke:#3b82f6,color:#dbeafe
    classDef slow fill:#3a1f5f,stroke:#a855f7,color:#e9d5ff

    class PII,Secret,Comp,Custom fast
    class Inject,Mod,Jail slow
```

## Policy decisions

How Tamga determines whether to block, redact, warn, or pass a request.
Three checks come before any scanning and do not depend on findings: the
provider allowlist, the daily budget and the rate limit. The policy rules are
applied to the findings after that.

```mermaid
%%{init: {
  'theme': 'dark',
  'themeVariables': {
    'primaryColor': '#1e3a5f',
    'primaryTextColor': '#dbeafe',
    'primaryBorderColor': '#3b82f6',
    'lineColor': '#6b7280',
    'background': '#0f172a',
    'mainBkg': '#1e293b'
  }
}}%%
flowchart TD
    Start([Request received]) --> CheckProvider{Provider<br/>allowed?}

    CheckProvider -->|No| Block403([403<br/>provider_not_allowed])
    CheckProvider -->|Yes| CheckBudget{Daily budget<br/>OK?}

    CheckBudget -->|Exceeded| Block402([402<br/>budget_exceeded])
    CheckBudget -->|OK| CheckRate{Rate limit<br/>OK?}

    CheckRate -->|Exceeded| Block429([429<br/>Rate limited])
    CheckRate -->|OK| Scan[Scan] --> HasFindings{Any findings?}

    HasFindings -->|No| Pass([PASS<br/>Forward to LLM])
    HasFindings -->|Yes| EvalRules{Match policy<br/>rules?}

    EvalRules -->|No rule matched| DefaultAction([No rule for the finding<br/>forward])
    EvalRules -->|Rule matched| RuleAction{Rule action?}

    RuleAction -->|block| Block403b([BLOCK 403<br/>Policy violation])
    RuleAction -->|redact| Mask[Mask findings in payload] --> Forward([Forward to LLM])
    RuleAction -->|warn| Webhook[Queue alert<br/>continue request] --> Forward
    RuleAction -->|pass| Forward

    classDef pass fill:#0d3a3a,stroke:#14b8a6,color:#ccfbf1
    classDef block fill:#3a0d0d,stroke:#ef4444,color:#fecaca
    classDef warn fill:#3a2d0a,stroke:#f59e0b,color:#fef3c7

    class Pass,Forward,DefaultAction pass
    class Block403,Block403b,Block402,Block429 block
    class Mask,Webhook warn
```

Order of the checks, as `handleProxy` in `proxy/internal/proxy/handler.go`
makes them:

1. The caller's key, when one is sent or `TAMGA_REQUIRE_KEY=true`: 401 if it is refused
2. Provider not allowed by the policy: 403 `provider_not_allowed`
3. Daily token or cost budget of the organisation exceeded: 402 `budget_exceeded`
4. Rate limit exceeded: 429, when `rate_limit.action_on_exceed` is to block
5. Scan, then the policy rules on the findings: block (403), redact (mask and
   forward), warn (forward), pass. Block, strip and warn each queue an alert

## Integrations

Twelve kinds of alert destination are defined in
`proxy/internal/webhooks/store.go`: Slack, Teams, Splunk (two kinds, JSON
and CEF), Sentinel, QRadar, Datadog, Jira, PagerDuty, Opsgenie, ServiceNow
and a generic webhook. Tamga can post alerts to a destination of one of
these kinds. The diagram shows six of them, each with the format it is
sent, next to sign-in and observability:

```mermaid
%%{init: {
  'theme': 'dark',
  'themeVariables': {
    'primaryColor': '#1e3a5f',
    'primaryTextColor': '#dbeafe',
    'primaryBorderColor': '#3b82f6',
    'lineColor': '#6b7280',
    'background': '#0f172a',
    'mainBkg': '#1e293b'
  }
}}%%
graph TB
    subgraph "Identity Providers"
        Github[GitHub OAuth]
    end

    subgraph "Tamga Core"
        Proxy[Tamga Proxy]
        Auth[Management API auth<br/>admin key, API key or session token]
    end

    subgraph "SIEM / Security Tools"
        Splunk[Splunk HEC]
        Sentinel[Microsoft Sentinel]
        Datadog[Datadog]
    end

    subgraph "Notification"
        Slack[Slack Webhooks]
        Teams[MS Teams]
        PagerDuty[PagerDuty]
    end

    subgraph "Observability"
        OTLP[OTLP Collector]
        Prometheus[Prometheus<br/>GET /api/v1/metrics, with a key]
        Jaeger[Jaeger UI]
    end

    Github --> Auth
    Auth --> Proxy

    Proxy -.->|JSON or CEF over HEC| Splunk
    Proxy -.->|CEF over HTTP| Sentinel
    Proxy -.->|JSON POST| Datadog

    Proxy -.->|webhook| Slack
    Proxy -.->|webhook| Teams
    Proxy -.->|Events API v2 JSON| PagerDuty

    Proxy -.->|traces| OTLP
    Proxy -.->|metrics| Prometheus
    OTLP --> Jaeger

    classDef idp fill:#1e3a5f,stroke:#3b82f6,color:#dbeafe
    classDef siem fill:#3a1f5f,stroke:#a855f7,color:#e9d5ff
    classDef notif fill:#3a2d0a,stroke:#f59e0b,color:#fef3c7
    classDef obs fill:#0d3a3a,stroke:#14b8a6,color:#ccfbf1

    class Github idp
    class Splunk,Sentinel,Datadog siem
    class Slack,Teams,PagerDuty notif
    class OTLP,Prometheus,Jaeger obs
```

## Database schema

PostgreSQL is optional. The schema is the 19 numbered migrations in
[`deploy/migrations/`](../../deploy/migrations/); read them for the columns.
The tables they create:

| Table | Migration | Holds |
|---|---|---|
| `organizations` | 001 | Organisations; request rows refer to one |
| `request_logs` | 001, 003, 006, 017 | One row per request: metadata, action and masked findings. Partitioned by month on `created_at`, with a default partition |
| `audit_log` | 004 | The hash-chained log of administrative actions |
| `policy_revisions`, `policy_proposals` | 004 | Policy history and two-person proposals |
| `retention_run_log` | 005 | One row per run of the retention job |
| `analyzer_findings` | 007 | Written only by a direct call of the analyzer's `POST /api/v1/analyze` with `DATABASE_URL` set. Nothing the proxy sends the analyzer is stored here |
| `model_pricing` | 008, 009 | Price per model, used for cost |
| `incident_lifecycle` | 012, 014 | Triage state of incidents |
| `saved_hunts` | 013 | Saved event queries |
| `virtual_keys` | 015 | Tamga's own API keys, as a SHA-256 hash and a prefix |
| `stored_documents` | 016 | Custom patterns, team roles, SSO settings and webhooks, one JSON document each |

Migration 011 added row-level security to `request_logs` and `daily_stats`
and 018 removed it; it never had an effect. Migration 019 dropped five tables
that nothing read or wrote: `api_keys`, `policies`, `alerts` and `daily_stats`
from 001 and `event_outbox` from 010. See
[Upgrading the database](../operations.md#upgrading-the-database).

Retention: with `TAMGA_RETENTION_ENABLED=true` a daily job creates the monthly
partitions of `request_logs` and drops old data. Request logs are kept 30
days by default and the audit log 365
(`TAMGA_RETENTION_*_DAYS`).
