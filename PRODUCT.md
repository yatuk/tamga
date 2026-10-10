# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- Primary: SOC analysts, security engineers, and incident responders operating an LLM security gateway.
- Secondary: compliance and risk owners who need KVKK, BDDK, GDPR, and OWASP LLM evidence.
- Evaluators and platform engineers also use the console to check deployment health, policies, integrations, and cost controls.

These roles and jobs are inferred from the repository, product documentation, console routes, and commit history at the user's explicit direction to derive product context from the codebase. They have not been confirmed with users.

## Product Purpose

Tamga is a self-hosted LLM security proxy. It scans prompts and responses for personal data, secrets and prompt injection before a request is forwarded to a provider, applies a policy to what it finds, and keeps a record of what it did.

The console succeeds when an operator can read what the proxy stopped, redacted or let through, see what needs attention, and move into investigation or a fix without rebuilding the state from raw logs.

## Positioning

Tamga combines inline enforcement, Turkish and regulated-industry data controls (TC Kimlik, Turkish IBAN and phone formats), reversible tokenization, several providers behind one proxy, and a self-hosted audit trail. It is not LLM observability: it keeps regulated data in the customer's control and enforces policy before a request reaches a model.

## Operating Context

- Monitoring and incident triage at a desk: the overview, the incident queue, the event explorer, threat hunting.
- Analysis of traffic, trends, token costs, latency, and reports.
- Policy editing with proposals and a diff, a playground that runs a prompt against the policy, and custom patterns.
- Administration: proxy status, the scanner pool, API keys, integrations, the audit log, team roles, and settings.
- Regulated deployments where evidence, data residency, and plain reporting of failure matter.

## Capabilities and Constraints

- Next.js 15, React 19, TypeScript, IBM Carbon (`@carbon/react`) re-skinned in `dashboard/app/tamga.css`, TanStack Query, nuqs, Motion, Sonner, CodeMirror. No Tailwind and no shadcn/ui.
- One light theme, by decision. There is no theme toggle and no dark theme.
- The console has no sign-in of its own. It reads the proxy's management API with the admin key entered under Settings, which is kept in that browser. `/sign-in` and `/sign-up` say so.
- The proxy decides what a key or a signed-in account may do (viewer, analyst, admin). The console cannot know a key's role beforehand, so it offers the action and shows the proxy's refusal.
- The interface is English only.
- The console must stay useful when there is no admin key, when the proxy is down or refuses the key, and when there is no data. Each of those states is worded differently.
- It shows only what the proxy reported. There is no demo data in the console's source, and a value that is not known is said to be not known.
- The proxy's actions are `BLOCK`, `REDACT`, `STRIP`, `WARN`, `LOG`, and `PASS`. The console words them as Stopped, Redacted, Stripped, Warned, Logged, and Passed. Findings carry a severity of critical, high, medium, or low.
- Routes and the management API contract (`proxy/docs/openapi.yaml`) are to be preserved.

## Brand Commitments

- Product name: Tamga.
- In the console the mark is the wordmark `TAMGA.` with a red full stop, and the favicon `dashboard/app/icon.svg`. The T-shaped mark under `docs/logo/` is used by the README, not by the console.
- Red is the brand's colour and in the console it means a request was stopped.
- Voice: precise, operational, calm, and honest about coverage and limits.
- No fabricated customer, compliance, benchmark, or readiness claims.

## Evidence on Hand

- Product truth and architecture: `README.md`, `docs/`, `proxy/docs/openapi.yaml`.
- The design system: `DESIGN.md`.
- Logo assets: `docs/logo/tamga-mark.svg`, and `docs/logo/tamga-mark-dark.svg` for a dark page such as the README on GitHub.
- Benchmark and adversarial data: `docs/benchmarks/` and `tests/stress/baseline.json`.
- The console: pages in `dashboard/app/(dashboard)/dashboard/`, the kit in `dashboard/components/console/`, the API client in `dashboard/lib/api/`.
- No customer testimonials or external certification claims are present; future work must not invent them.

## Product Principles

1. Action before decoration: every surface says what happened, why it matters, and where to go next.
2. Evidence over reassurance: state the source, the window, and what is not known instead of implying certainty.
3. One figure per page: say the number the page is about in a sentence; the rest is supporting fact.
4. Secure by default: keys and configuration stay out of the way of routine monitoring.
5. Keep the operator's thread: filters, the time window, and the open request live in the URL.

## Accessibility & Inclusion

- Keyboard access to everything interactive, a visible focus ring, a skip link, and status said in words, never by colour alone.
- Reduced motion is honoured in part; the gaps are listed in `DESIGN.md`.
- The console is a desk tool. A narrow or zoomed window reflows without a sideways scroll; there is no phone design.
