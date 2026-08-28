# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- Primary: SOC analysts, security engineers, and incident responders operating an LLM security gateway.
- Secondary: compliance and risk owners who need defensible KVKK, BDDK, GDPR, and OWASP LLM evidence.
- Evaluators and platform engineers also use the dashboard to validate deployment health, policies, integrations, and cost controls.

These roles and jobs are inferred from the repository, product documentation, dashboard routes, and commit history at the user's explicit direction to derive product context from the codebase.

## Product Purpose

Tamga is a self-hosted, provider-agnostic LLM security proxy. It inspects prompts and responses before sensitive data leaves a customer's infrastructure, enforces policy through block/redact/warn/pass actions, and leaves an auditable operational record.

Dashboard success means an operator can understand current risk posture, identify what needs attention, and move into investigation or remediation without reconstructing the state from raw telemetry.

## Positioning

Tamga combines inline enforcement, Turkish and regulated-industry data controls, reversible tokenization, provider independence, and a self-hosted audit trail. Its differentiator is not generic LLM observability: it is keeping regulated data in the customer's control while enforcing policy before model ingress and after model egress.

## Operating Context

- Continuous SOC monitoring and incident triage.
- Investigation across security events, incidents, threat hunting, trends, traffic, latency, and cost.
- Policy authoring and simulation, custom pattern management, API-key administration, integrations, audit review, and runtime health checks.
- Regulated deployments where evidence, data residency, and transparent failure reporting matter.

## Capabilities and Constraints

- Next.js 15, React 19, TypeScript, Tailwind, Recharts, TanStack Query, and Framer Motion.
- The dashboard consumes the Tamga proxy management API and must remain useful when the admin key is missing, the proxy is offline, or datasets are empty.
- Core actions and terminology are `BLOCK`, `REDACT`, `WARN`, and `PASS`; risk severity is critical, high, medium, low, and informational/pass where relevant.
- Existing functional routes, API contracts, authentication behavior, light/dark themes, and real product data must be preserved.
- Demo data may support development states but must never masquerade as live production telemetry.

## Brand Commitments

- Product name: Tamga.
- Preserve the existing Tamga mark and the security-operator character of the product.
- Voice should be precise, operational, calm under pressure, and honest about coverage and limitations.
- Avoid fabricated customer, compliance, benchmark, or readiness claims.

## Evidence on Hand

- Product truth and architecture: `README.md`, `docs/`, `proxy/docs/openapi.yaml`.
- Existing logo assets: `dashboard/public/tamga-logo.png`, `docs/tamga_logo.png`, and `dashboard/components/TamgaLogo.tsx`.
- Published benchmark and adversarial data: `docs/benchmarks/` and `tests/stress/baseline.json`.
- Existing dashboard implementation and API-driven states: `dashboard/app/(dashboard)/dashboard/`, `dashboard/components/dashboard/`, and `dashboard/lib/api/`.
- No customer testimonials or external certification claims are present; future UI must not invent them.

## Product Principles

1. Action before decoration: every operational surface should clarify what changed, why it matters, and where to go next.
2. Evidence over reassurance: expose source, time range, state, and limitations instead of implying certainty.
3. Calm density: support expert scanning without flattening every metric into equal visual weight.
4. Secure by default: sensitive configuration and credentials should not dominate routine monitoring workflows.
5. Preserve operator context: filters, navigation, and drill-downs should maintain the user's investigative thread.

## Accessibility & Inclusion

- Maintain keyboard access, visible focus, reduced-motion support, semantic status text, and color-independent severity cues.
- Responsive behavior must support desktop SOC workstations and mobile incident checks without hiding critical state.
