# FAQ

**Q: Does data leave our network?**
A: No. Tamga is self-hosted. Only the actual LLM API call leaves your
network, and only after policy enforcement (PII redacted, secrets blocked).

**Q: How does Tamga compare to built-in provider guardrails?**
A: Provider guardrails (Anthropic, OpenAI) are provider-specific and
limited to their models. Tamga is provider-agnostic, supports custom
policies, and provides audit logs that meet BDDK/KVKK requirements.

**Q: What if Tamga goes down?**
A: Fail-open by default — the proxy returns 200 if the scanner pipeline
fails, to avoid blocking your application. Fail-closed mode is available for
high-security deployments. A circuit breaker handles upstream provider
failures with automatic retry.

**Q: Does scanning add latency?**
A: The static scan stage runs at p95 0.52ms (Aho-Corasick DFA). Deep
analysis (LLM judge, Presidio NER) runs asynchronously for medium-confidence
findings. End-to-end proxy overhead is p95 5.5ms at 100 RPS on consumer
hardware — see [benchmarks](benchmarks/README.md).

**Q: Can I run Tamga without Docker?**
A: Yes. The Go proxy is a single binary. PostgreSQL and Redis are optional
(in-memory fallbacks work for development). See `proxy/README.md` for
bare-metal deployment.

**Q: Is Tamga production-ready for regulated environments?**
A: Tamga v0.7.0 includes mTLS, IP allowlists, hash-chain audit logs, RBAC,
and KVKK/BDDK/GDPR compliance mappings. Evaluate in your own staging
environment before production.
