# Real-world use cases

## Bank with shadow AI

**Problem:** Employees pasting customer TC Kimlik and IBAN into ChatGPT.
The bank is concerned about KVKK fines and BDDK audit findings.

**What Tamga covers:** the bank's own applications, RAG pipelines and
agents that call a model API and are pointed at the proxy. With the shipped
policy a request carrying a TC Kimlik or a card number is blocked, and an
IBAN is redacted before the request is forwarded. Each decision is written
to the request log in the bank's own Postgres. A blocked request is posted
to the alert destinations the bank has connected ([alerts](operations.md#alerts)).

**What it does not cover:** text typed or pasted into chatgpt.com in a
browser never reaches the proxy. Tamga does not see or stop it; see
[What Tamga does not cover](../README.md#what-tamga-does-not-cover).

**Result:** on the traffic routed through Tamga, the identifiers the
scanners recognise are blocked or masked, and each decision is on record to
show an auditor. The scanners miss things (the README publishes the
misses), and the KVKK and BDDK exposure remains the bank's to assess.

## SaaS company using multiple LLMs internally

**Problem:** Engineering uses Copilot, sales uses ChatGPT Enterprise,
support uses Claude. Three vendors, three dashboards, no unified cost view.

**Solution:** Tamga as one proxy in front of the model APIs the company's
own applications call. A Tamga key per application, a daily token and USD
cap per organisation, and one policy for every provider route. One
dashboard for the LLM usage that goes through the proxy. The vendors' own
clients (ChatGPT Enterprise or Claude in a browser, Copilot in an editor)
talk to the vendor directly and are not seen unless they can be pointed at
the proxy.

**Result:** spend through the proxy stops at the daily cap; one request log
and one policy across teams for the traffic Tamga sees.

## Healthcare provider with RAG application

**Problem:** RAG indexes patient records; risk of surfacing PHI in answers.
Indirect prompt injection in uploaded documents.

**Solution:** Tamga between RAG and LLM. Retrieved chunks that the
application sends as tool results or attached documents are scanned as
`tool` text, and a rule can be limited to that role ([agents](agents.md)).
Injection wording found there can block the request or strip the chunk. A
chunk pasted into the user message is scanned as user text. With
`output_rules` on, the response is scanned for the PII types the scanners
know, and a finding is masked or blocks the response.

**Limits:** there is no scanner for health data. The README lists health
data among what the rules miss. Apart from one pattern for a labelled
medical record number, a diagnosis or a medication in a prompt or an answer
is not recognised. A custom entity can match an identifier with a fixed
format, such as a patient number.

**Result:** one layer against indirect injection in retrieved text, and a
record of each decision. Tamga does not by itself make PHI handling meet
HIPAA.
