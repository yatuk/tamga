# Tamga Architecture

All architecture diagrams in one place. D2 sources and rendered SVG/PNG
versions live alongside this file.

## System overview

![Tamga Architecture](tamga.svg)

View the [D2 source](tamga.d2) or [full-size PNG](tamga.png).

| Component | Language | Role | Port |
|---|---|---|---|
| Proxy | Go | Inline scanners, policy engine, reverse proxy, rate limiter, REST API | `8443` |
| Analyzer | Python | Deep PII/injection/toxicity analysis, compliance reports | `8444` |
| Dashboard | Next.js | Admin UI, incident hunting, integrations, OWASP coverage | `3000` |
| PostgreSQL | — | Optional persistent telemetry and audit storage | `5432` |
| Redis | — | Rate limiting, caching, distributed counters | `6379` |
| Jaeger | — | OpenTelemetry tracing (optional) | `16686` |

## Deployment topology

Six services on an internal `tamga_default` bridge network. Only Proxy
(`:8443`) and Dashboard (`:3000`) are exposed externally; all inter-service
communication stays inside Docker's network namespace.

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
            Proxy[Tamga Proxy<br/>:8443 TLS<br/>:8080 metrics]
        end

        subgraph "Inspection"
            Analyzer[Analyzer<br/>:50051 gRPC<br/>:8444 HTTP]
        end

        subgraph "Data"
            Postgres[(PostgreSQL 16<br/>:5432)]
            Redis[(Redis 7<br/>:6379)]
        end

        subgraph "Observability"
            Jaeger[Jaeger<br/>:16686 UI<br/>:4317 OTLP]
        end

        subgraph "Management"
            Dashboard[Dashboard<br/>:3000 Next.js]
        end
    end

    Client -->|HTTPS| Proxy
    Proxy -->|gRPC| Analyzer
    Proxy -->|SQL| Postgres
    Proxy -->|cache + rate limit| Redis
    Proxy -->|OTLP traces| Jaeger
    Dashboard -->|REST API| Proxy
    Dashboard -->|read events| Postgres

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
    participant Analyzer as Analyzer (Python)
    participant LLM as LLM Provider
    participant DB as PostgreSQL

    App->>Proxy: POST /v1/messages
    Note over Proxy: Authenticate API key<br/>Check rate limit (Redis)

    Proxy->>Scanner: Inline scan (input)
    Note over Scanner: 7 scanners run<br/>(~3-5ms)
    Scanner-->>Proxy: Findings + confidence

    alt Confidence 0.3-0.9 (uncertain)
        Proxy->>Analyzer: gRPC deep scan
        Note over Analyzer: Presidio NER<br/>+ LLM-as-judge
        Analyzer-->>Proxy: Enriched findings
    end

    Proxy->>Policy: Evaluate findings

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
```

Scan overhead is a small fraction of total request time — provider latency
is usually 500-3000ms. See [benchmarks](../benchmarks/README.md) for
measured scan-stage numbers.

View the [D2 source](request-lifecycle.d2) or [full-size SVG](request-lifecycle.svg).

## Scanner pipeline

Seven inline scanners run on every request. Hybrid design: fast scanners
run sequentially to avoid goroutine overhead, while slower scanners run in
parallel to hide their latency.

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

    Fast -.->|0.3ms| PII[PII<br/>email, TC, IBAN]
    Fast -.->|0.3ms| Secret[Secrets<br/>API keys, tokens]
    Fast -.->|0.2ms| Comp[Competitor<br/>brand names]
    Fast -.->|0.2ms| Custom[Custom Entity<br/>policy-driven]

    Slow -.->|1.5ms| Inject[Injection<br/>DFA + LLM judge]
    Slow -.->|1.2ms| Mod[Moderation<br/>toxicity, hate]
    Slow -.->|0.6ms| Jail[Jailbreak<br/>DAN, STAN]

    PII --> Merge[Findings<br/>Aggregator]
    Secret --> Merge
    Comp --> Merge
    Custom --> Merge
    Inject --> Merge
    Mod --> Merge
    Jail --> Merge

    Merge --> Confidence{Confidence<br/>Score}
    Confidence -->|< 0.3| Skip[Skip deep scan]
    Confidence -->|0.3-0.9| Deep[Analyzer gRPC]
    Confidence -->|> 0.9| Decisive[Use inline result]

    classDef fast fill:#1e3a5f,stroke:#3b82f6,color:#dbeafe
    classDef slow fill:#3a1f5f,stroke:#a855f7,color:#e9d5ff

    class PII,Secret,Comp,Custom fast
    class Inject,Mod,Jail slow
```

## Policy decisions

How Tamga determines whether to block, redact, warn, or pass a request.
Decision order matters: the earliest deny wins.

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
    Start([Request received]) --> HasFindings{Any findings?}

    HasFindings -->|No| Pass([PASS<br/>Forward to LLM])
    HasFindings -->|Yes| CheckProvider{Provider<br/>allowed?}

    CheckProvider -->|No| Block403([BLOCK 403<br/>Provider not allowed])
    CheckProvider -->|Yes| EvalRules{Match policy<br/>rules?}

    EvalRules -->|No rule matched| DefaultAction([Apply default action<br/>usually PASS])
    EvalRules -->|Rule matched| RuleAction{Rule action?}

    RuleAction -->|block| Block403b([BLOCK 403<br/>Policy violation])
    RuleAction -->|redact| Mask[Mask findings in payload<br/>e.g. 4532 -> masked]
    RuleAction -->|warn| Webhook[Fire webhook<br/>continue request]
    RuleAction -->|pass| PassRule([PASS])

    Mask --> CheckBudget{Daily budget<br/>OK?}
    Webhook --> CheckBudget
    PassRule --> CheckBudget

    CheckBudget -->|Exceeded| Block429([BLOCK 429<br/>Budget exceeded])
    CheckBudget -->|OK| CheckRate{Rate limit<br/>OK?}

    CheckRate -->|Exceeded| Block429b([BLOCK 429<br/>Rate limited])
    CheckRate -->|OK| Forward([Forward to LLM])

    classDef pass fill:#0d3a3a,stroke:#14b8a6,color:#ccfbf1
    classDef block fill:#3a0d0d,stroke:#ef4444,color:#fecaca
    classDef warn fill:#3a2d0a,stroke:#f59e0b,color:#fef3c7

    class Pass,PassRule,Forward pass
    class Block403,Block403b,Block429,Block429b block
    class Mask,Webhook warn
```

Decision order (first match wins):

1. Provider allowed? Not in allowlist: BLOCK 403
2. Policy rules matched? No rule: default action
3. Rule action block (critical PII, secrets): BLOCK 403
4. Rule action redact: mask and continue
5. Rule action warn: fire webhook, pass
6. Daily budget exceeded: BLOCK 429
7. Rate limit exceeded: BLOCK 429
8. Default: PASS

## Integrations

12 pre-configured integrations. Tamga plugs into your existing security and
observability stack:

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
        SAML[SAML / OIDC<br/>Enterprise]
    end

    subgraph "Tamga Core"
        Proxy[Tamga Proxy]
        Auth[Auth Layer<br/>JWT + Bearer]
    end

    subgraph "SIEM / Security Tools"
        Splunk[Splunk HEC]
        Sentinel[Microsoft Sentinel]
        Elastic[Elastic / OpenSearch]
        Datadog[Datadog Logs]
    end

    subgraph "Notification"
        Slack[Slack Webhooks]
        Teams[MS Teams]
        PagerDuty[PagerDuty]
        Email[Email/SMTP]
    end

    subgraph "Observability"
        OTLP[OTLP Collector]
        Prometheus[Prometheus /metrics]
        Jaeger[Jaeger UI]
    end

    Github --> Auth
    SAML --> Auth
    Auth --> Proxy

    Proxy -.->|HEC events| Splunk
    Proxy -.->|Log Analytics| Sentinel
    Proxy -.->|JSON logs| Elastic
    Proxy -.->|datadog-go| Datadog

    Proxy -.->|webhook| Slack
    Proxy -.->|webhook| Teams
    Proxy -.->|events| PagerDuty
    Proxy -.->|smtp| Email

    Proxy -.->|traces| OTLP
    Proxy -.->|metrics| Prometheus
    OTLP --> Jaeger

    classDef idp fill:#1e3a5f,stroke:#3b82f6,color:#dbeafe
    classDef siem fill:#3a1f5f,stroke:#a855f7,color:#e9d5ff
    classDef notif fill:#3a2d0a,stroke:#f59e0b,color:#fef3c7
    classDef obs fill:#0d3a3a,stroke:#14b8a6,color:#ccfbf1

    class Github,SAML idp
    class Splunk,Sentinel,Elastic,Datadog siem
    class Slack,Teams,PagerDuty,Email notif
    class OTLP,Prometheus,Jaeger obs
```

## Database schema

Tamga uses PostgreSQL 16 with 13 migrations. Core tables and their
relationships:

```mermaid
erDiagram
    request_logs ||--o{ findings : "has many"
    request_logs ||--o| incidents : "may trigger"
    request_logs }o--|| api_keys : "authenticated by"
    request_logs }o--|| model_pricing : "priced by"

    api_keys ||--o{ rate_limits : "tracks usage"

    policies ||--o{ policy_history : "versioned"
    policies ||--o{ custom_entities : "defines"

    audit_log ||--o| policy_history : "tracks changes"

    incidents ||--o{ incident_events : "audit trail"

    request_logs {
        uuid id PK
        text request_id UK
        text api_key_id FK
        text provider
        text model
        text action "block redact warn pass"
        int input_tokens
        int output_tokens
        decimal cost_usd
        jsonb findings
        timestamptz created_at
    }

    findings {
        uuid id PK
        uuid request_log_id FK
        text type "pii secret injection..."
        text category
        text match "redacted or hashed"
        text severity
        decimal confidence
        text scanner
        text dataset_version
    }

    api_keys {
        text id PK
        text name
        text key_hash UK
        text tier "community team business enterprise"
        int rate_limit
        timestamptz created_at
        timestamptz expires_at
    }

    model_pricing {
        int id PK
        text provider
        text model_family
        text model_version
        decimal input_per_1k
        decimal output_per_1k
        char currency
        timestamptz effective_from
        timestamptz effective_to "NULL=active"
    }

    policies {
        int id PK
        text version
        jsonb config
        boolean active
        timestamptz created_at
    }

    incidents {
        uuid id PK
        text request_id FK
        text severity
        text status "open ack resolved closed"
        text assignee
        text tags
    }

    audit_log {
        uuid id PK
        text actor
        text action
        text resource_type
        jsonb before
        jsonb after
        text hash "chain"
        timestamptz created_at
    }
```

Retention: `request_logs` partitioned by month, default 90-day retention,
configurable per tier.
