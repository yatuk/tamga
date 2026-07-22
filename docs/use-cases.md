# Real-World Use Cases

## Bank with Shadow AI (primary fit)

**Problem:** Employees pasting customer TC Kimlik and IBAN into ChatGPT.
KVKK fines and BDDK audit exposure.

**Solution:** Tamga sits between the corporate proxy and LLM providers.
Inline detection blocks TC/IBAN, redacts customer names, logs every attempt
for BDDK audit. The KVKK officer gets a real-time webhook notification.

**Result:** Zero PII leak to LLM providers; complete audit trail;
compliance evidence ready for regulator review.

## SaaS company using multiple LLMs internally

**Problem:** Engineering uses Copilot, sales uses ChatGPT Enterprise,
support uses Claude. Three vendors, three dashboards, no unified cost view.

**Solution:** Tamga as a unified proxy. Single API key per team, daily
budget caps, provider-agnostic policy enforcement. One dashboard for all
LLM usage across the organization.

**Result:** cost reduction via budget caps and provider routing; unified
audit trail; policy consistency across teams.

## Healthcare provider with RAG application

**Problem:** RAG indexes patient records; risk of surfacing PHI in answers.
Indirect prompt injection in uploaded documents.

**Solution:** Tamga between RAG and LLM. Source-tagged content inspection on
RAG chunks. Indirect injection detector flags suspicious payloads. PHI
redaction on output before return to the user.

**Result:** HIPAA compliance pathway; auditable PHI handling; defense in
depth against indirect injection.
