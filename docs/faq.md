# FAQ

**Q: Does data leave our network?**
A: Tamga runs on your own servers and sends prompts to no service of its
own. The request to the LLM provider still leaves your network, after the
policy has been applied to what the scanners found: in the shipped policy
national IDs, cards and secrets block the request and other PII is redacted.
What the scanners miss is forwarded as it is.

One other path can carry request text out, and it is off unless you turn on
both of its switches. With `TAMGA_ANALYZER_SEND_CONTENT=true` the proxy
copies forwarded requests (as forwarded: masked where the policy masked
them, never a blocked one) to the analyzer; with `TAMGA_LLM_JUDGE=anthropic`
and an `ANTHROPIC_API_KEY` the analyzer's LLM judge then sends up to the
first 2,000 characters of such a request to Anthropic. An Anthropic key on
its own turns nothing on. Details and what the analyzer downloads are in
[Operations](operations.md#analyzer).

**Q: How does Tamga compare to built-in provider guardrails?**
A: Provider guardrails (Anthropic, OpenAI) are provider-specific and
limited to their models. Tamga applies one policy, which you write, to the
provider routes it serves, and records each decision in your own Postgres
request log: provider, model, action, masked findings, token counts and
latency. Prompt and response bodies are not stored. Whether that record
satisfies BDDK or KVKK is for you and your auditor to decide; the
[compliance pages](compliance/) map requirements to controls.

**Q: What if Tamga goes down?**
A: Tamga is in the request path. While the proxy itself is down, the
applications pointed at it get no answer. When the proxy is up and the scan
fails (a scanner errors or times out, or is shed under load),
`scan.on_error` decides. A policy without that key forwards a plain chat request on
the findings of the scanners that did run, with
`X-Tamga-Scan-Degraded: true`, unless those findings call for masking: text
is not sent masked in part, and that request gets a 503. A request with tool
results or tool definitions gets a 503 as well. The shipped `proxy/tamga-policy.yaml` sets
`on_error: block`, which answers 503. A circuit breaker and retries handle
failures of the upstream provider.

**Q: Does scanning add latency?**
A: Yes. The static scan stage (Aho-Corasick DFA plus validators) runs in
the request path and its cost grows with the size of the text. The analyzer
(Presidio NER, and the LLM judge when it is turned on) runs after the
decision and only when `TAMGA_ANALYZER_SEND_CONTENT` is set, for forwarded
requests in a middle band of risk score. It does not change the verdict, and
what it finds is logged by the proxy as a count and stored nowhere. The measured figures
for the scan stage and for the whole proxy path are in
[benchmarks](benchmarks/README.md), the
[README](../README.md#benchmarks) and `tests/stress/baseline.json`; they
are not repeated here.

**Q: Can I run Tamga without Docker?**
A: Yes. The Go proxy is a single binary. PostgreSQL and Redis are optional
(in-memory fallbacks work for development). See `proxy/README.md` for
bare-metal deployment.

**Q: Is Tamga production-ready for regulated environments?**
A: Tamga is pre-1.0. It has IP allowlists, optional mTLS, role checks on the
management API, a hash-chained audit log of administrative actions, and
pages that map KVKK, BDDK and GDPR requirements to controls. The hash chain
does not cover request decisions: those are in the request log, which is
not chained. mTLS as the proxy sets it up has no test. The
[README](../README.md#project-status) lists what is tested end to end and
what is not. Evaluate in your own staging environment before production.
