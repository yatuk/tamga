# GDPR mapping (EU)

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Tamga controls that are relevant to GDPR articles. The controls apply only
to traffic that is routed through the proxy. Tamga does not see what a
person types into a chat website in a browser. Tamga runs on your own
infrastructure, so its logs and settings stay where you run it. A request
that is not blocked is still forwarded to the provider it names.

How to read the table: the first column gives an article's number and its
heading as printed in the Regulation; words in single quotation marks are
the Regulation's own. Anything else in that column is this page's summary
and shortens the provision. The first column names the article; it does not
say that the control in the second column satisfies it. Read the article
itself before relying on a row.

## Sources

- Regulation (EU) 2016/679 of the European Parliament and of the Council of
  27 April 2016 on the protection of natural persons with regard to the
  processing of personal data and on the free movement of such data, and
  repealing Directive 95/46/EC (General Data Protection Regulation), OJ L
  119, 4.5.2016, pp. 1-88. Read at
  <https://eur-lex.europa.eu/eli/reg/2016/679/oj> on 2026-10-10. This is the
  text as published in the Official Journal. Corrigenda published later were
  not compared with it.
- Read for this page: recitals 26, 28 and 29; Articles 4, 5, 6, 12, 13, 15,
  17, 19, 24, 25, 28, 30, 32, 33, 34, 35, 37, 44, 45, 46 and 49; the
  headings of Articles 1 to 53. Not read: guidelines of the European Data
  Protection Board, decisions of supervisory authorities, case law, and the
  national laws that supplement the Regulation. Any of these can change how
  an article applies.

## Three things that hold for every row

**Masking is not anonymisation.** `REDACT` replaces the identifiers a
pattern found; the vault replaces them with numbered placeholders and puts
the originals back in the response. The rest of the text is left as it is
and may still identify someone. Article 4(5) defines 'pseudonymisation' as
processing after which the data "can no longer be attributed to a specific
data subject without the use of additional information", with that
information kept separately. Recital 26 says that pseudonymised data which
could be attributed to a person with additional information are to be
treated as information on an identifiable person, that identifiability is
judged by the means "reasonably likely to be used", and that the Regulation
does not apply to anonymous information. Whether what Tamga forwards after
masking is personal data, pseudonymised data or neither is a judgement
about the whole text and the context. Tamga does not make it.

**A request that is not blocked goes to the provider.** Tamga forwards it
to the provider the request names, if the policy allows that provider. It
does not know where the provider processes the data, under what contract,
or on what basis. `data.residency` in the policy and `TAMGA_REGION` refuse
nothing: they are compared once at startup and a mismatch is logged.

**The request log is itself processing of personal data by whoever runs
Tamga.** One row is stored per proxied request: the organisation, the
request id, the time, the provider, model and endpoint, the action taken,
the findings (type, category, severity and the match, stored masked, or as
its SHA-256 with `data.hash_findings: true`), the user identifier the
application sent in `X-Tamga-User-Id`, the name and version of the policy,
and, when the response was read, tokens, cost and the findings of the
response. The text of the prompt and of the response is not stored. A
masked match keeps its first and last characters, and a user identifier is
often an e-mail address, so the log is personal data in most deployments.
It needs its own purpose, basis, retention period and place in the
organisation's records. It is kept in PostgreSQL when `TAMGA_DB_URL` is
set; Tamga does not encrypt that database. See
[operations: what a stored request holds](../operations.md#what-a-stored-request-holds).

## Articles for which Tamga has a relevant control

| GDPR article (heading in the Regulation) | Relevant Tamga control | What the control does not do |
|------------------------------------------|------------------------|------------------------------|
| Art. 5: Principles relating to processing of personal data. Paragraph 1(c), 'data minimisation' | PII redaction rule (`pii_detection`, action `REDACT`): masks the PII types listed in the rule before a proxied request is forwarded. PII block rule (`pii`, action `BLOCK`): stops a request that contains one of the identifiers listed. Shipped policy: `REDACT` for IBAN, e-mail, phone, VKN and IP addresses; `BLOCK` for TC Kimlik, card number and SSN | It masks only the types listed and only what the patterns recognise. There is no built-in pattern for a person's name or a postal address. Whether the data sent is limited to what the purpose needs is a decision about the application, not about a pattern. See "Masking is not anonymisation" above |
| Art. 5, paragraph 1(e), 'storage limitation' | Retention job (`TAMGA_RETENTION_ENABLED`, `TAMGA_RETENTION_REQUEST_LOGS_DAYS` default 30, `TAMGA_RETENTION_AUDIT_DAYS` default 365, `TAMGA_RETENTION_INCIDENT_CLOSED_DAYS` default 0): once a day it drops expired request-log partitions, deletes older audit entries and, when the last setting is above 0, deletes closed incidents | It is off unless enabled, and needs a database. It covers what Tamga stores, not what a provider or the application stores. Request logs are dropped a monthly partition at a time, so a row can outlive the period by up to a month. With the default 0, closed incidents are kept. Database backups are outside it. The policy key `data.retention_days` has no effect. This row said the job deletes "alerts" before 2026-10-10; it does not: an alert that was sent is in the receiving system |
| Art. 5, paragraph 1(f), 'integrity and confidentiality'; Art. 32: Security of processing | TLS on the listener (`TAMGA_TLS_CERT`, `TAMGA_TLS_KEY`, minimum TLS 1.2), optional client certificates (`TAMGA_MTLS_STRICT_VERIFY`), roles on the management API (admin, analyst, viewer), scoped API keys (read, write, admin, proxy) with expiry and revocation, sessions that can be ended (`POST /api/v1/team/{id}/sessions/revoke`), secret detection on what is sent (`rules.secret_detection`). Each is one control toward this article, for Tamga's own surface and for proxied traffic | The listener serves plain HTTP until a certificate is set. Tamga does not encrypt its database or its backups. Art. 32(1) asks for a level of security "appropriate to the risk" across the whole processing, and names among other things the ability to restore availability and a process for regularly testing the measures: these are the organisation's. The security of the application, of the provider and of the network is outside Tamga |
| Art. 5, paragraph 2, 'accountability'; Art. 24: Responsibility of the controller | Request log (as described above) and audit log with a hash chain (administrative actions such as policy, key, pattern, role and webhook changes, with the verified actor; `GET /api/v1/audit/verify`). Both can be used as evidence of what was done to proxied traffic and who changed Tamga's settings | They do not demonstrate compliance. They record Tamga's decisions, not the organisation's reasons. `GET /api/v1/audit/verify` checks the entries the running process holds in memory (the most recent 512), not the whole stored table, and the chain is an unkeyed hash: it shows an accidental or partial change, not one made by someone who can rewrite the table. Without `TAMGA_DB_URL` the logs are in memory only |
| Art. 15: Right of access by the data subject (to be answered within the time limits of Art. 12(3): one month, extendable) | Subject access (`POST /api/v1/events/subject`, admin only): returns up to 500 stored request-log rows, newest first, for one selector. `user_id` and `email` are compared with the user identifier stored on the request, that is, with the sender. `tckn_hash` (a SHA-256) is looked for in the stored findings, which hold hashes only when `data.hash_findings` is on. Each call is written to the audit log as `privacy.subject_access` with the kind of selector and the number of rows, not the selector's value | It is one input to answering a request, not an answer to it. It finds the sender of a request, not a person who is named inside a prompt: prompt text is not stored, and no selector searches for a name. It returns Tamga's request log only: not what the application, the provider or other systems hold, and not the information Art. 15(1) lists (purposes, recipients, storage period, source). Identifying the requester, deciding what may be disclosed without affecting others (Art. 15(4)) and replying are the organisation's process |
| Art. 17: Right to erasure (‘right to be forgotten’) | Erasure of stored request logs for one selector (`DELETE /api/v1/events/subject`, admin only, same selectors as above). The rows are deleted and cannot be restored through Tamga. The call is written to the audit log as `privacy.subject_erase` with the kind of selector and the number of rows deleted. The retention job above also removes rows with time | It acts on Tamga's request logs only: not on the audit log, the incident queue, alerts already sent, database backups, or anything the application or a provider holds. It does not decide whether a ground of Art. 17(1) applies or an exception of Art. 17(3) (for example legal claims) stands in the way, and there is no second approval before the delete. Telling recipients (Art. 19) is the organisation's process. `REDACT` and the vault are not erasure |
| Art. 25: Data protection by design and by default | Scanning happens in the request path, before a request is forwarded. The request log stores metadata and findings, not the prompt; matches are stored masked, or as SHA-256 with `data.hash_findings: true`. With `vault.enabled`, values under a `REDACT` rule go to the provider as placeholders | Tamga is one component. It does not make the system around it designed for data protection, and the "by default" part of Art. 25(2) is about what the organisation's own processing collects and keeps. The shipped defaults are not the strictest: the vault, response scanning (`output_rules`), `data.hash_findings`, the retention job and `TAMGA_REQUIRE_KEY` are off until set. This row was labelled "Privacy by design" before 2026-10-10 |
| Art. 33: Notification of a personal data breach to the supervisory authority (paragraph 1: "without undue delay and, where feasible, not later than 72 hours after having become aware of it") | Alerts: a blocked, stripped or warned proxied request is posted to the saved alert destinations (Slack, Teams, Splunk, Sentinel, QRadar and others) and to the `notify` URL of the rule that matched. The message names the request and the kinds of finding, not the text. Incidents can be triaged, resolved and reopened in the incident queue (`/api/v1/incidents`) | An alert is not a breach notification and Tamga sends nothing to an authority. A blocked request is not by itself a personal data breach as Art. 4(12) defines one: the request was stopped. A breach may happen where Tamga sees nothing, and a request whose personal data no pattern recognised raises no alert. No destination has been tried against the real service, and an alert is dropped when the queue is full. Assessing an event, the 72-hour notification, its content (Art. 33(3)) and the record of breaches (Art. 33(5)) are the organisation's process |
| Chapter V: Transfers of personal data to third countries or international organisations (Arts. 44 to 50; Art. 44: General principle for transfers; Art. 45: Transfers on the basis of an adequacy decision; Art. 46: Transfers subject to appropriate safeguards; Art. 49: Derogations for specific situations) | Provider allowlist (`providers.allowed`, `providers.blocked`): limits which providers a proxied request may go to. Provider pools may point only at the vendor's own host or a host listed in `TAMGA_UPSTREAM_ALLOW`. Self-hosted deployment: Tamga's own logs and settings stay on your infrastructure | Tamga does not decide whether a transfer is lawful. A request forwarded to an external provider leaves your infrastructure; Tamga does not know in which country the provider processes it, whether an adequacy decision or a safeguard of Art. 46 covers it, or whether the provider passes it on. The allowlist names providers, not countries or regions. `data.residency` and `TAMGA_REGION` refuse nothing. Alert destinations are a second route out of your infrastructure, without request text |

## Articles for which Tamga has no control

A reader may expect these on a page like this one. Tamga does nothing about
them.

| GDPR article (heading in the Regulation) | Tamga control |
|------------------------------------------|---------------|
| Art. 5, paragraph 1(a), 'lawfulness, fairness and transparency'; Art. 6: Lawfulness of processing | No Tamga control. Tamga does not know whether a processing has one of the bases of Art. 6(1), or whether consent was given. A `BLOCK` rule stops listed identifiers; it says nothing about lawfulness |
| Art. 5, paragraph 1(b), 'purpose limitation'; paragraph 1(d), 'accuracy' | No Tamga control. Tamga does not know the purpose of a request and does not check whether a model's output is correct |
| Art. 9: Processing of special categories of personal data | No Tamga control for special categories as such. `custom_entities` and the `medical_record` pattern can mark some identifiers; nothing recognises health, belief or similar content written as ordinary prose |
| Art. 12: Transparent information, communication and modalities for the exercise of the rights of the data subject; Art. 13: Information to be provided where personal data are collected from the data subject; Art. 14: Information to be provided where personal data have not been obtained from the data subject | No Tamga control. Tamga shows nothing to the people whose data passes through it. The privacy notice, which should also cover the request log, is the organisation's |
| Art. 16: Right to rectification; Art. 18: Right to restriction of processing; Art. 20: Right to data portability; Art. 21: Right to object; Art. 22: Automated individual decision-making, including profiling | No Tamga control |
| Art. 28: Processor | No Tamga control. Tamga is software the organisation runs itself; nobody operates it on the organisation's behalf. The model provider a request is forwarded to may be a processor or a controller of its own. Choosing it, and the contract Art. 28(3) asks for, are the organisation's. The provider allowlist limits where requests may go; it is not a contract |
| Art. 30: Records of processing activities | No Tamga control. The record is the controller's (and the processor's) own document, with the content Art. 30(1) lists: purposes, categories of data subjects and of data, recipients, transfers, time limits, security measures. A request log is not that record. It can be a source of facts for it, for example which providers were called. This row cited the request log and the audit log before 2026-10-10 |
| Art. 34: Communication of a personal data breach to the data subject | No Tamga control |
| Art. 35: Data protection impact assessment; Art. 36: Prior consultation | No Tamga control. Tamga's logs and its benchmark figures (`docs/benchmarks/`) can be one input to an assessment |
| Art. 37: Designation of the data protection officer; Art. 38: Position of the data protection officer; Art. 39: Tasks of the data protection officer | No Tamga control |

## Where each control is described

The controls named on this page are the ones listed, with the place each is
described, at the end of the [KVKK page](kvkk.md#the-controls-and-where-each-is-described).
The subject access and erasure routes are in `proxy/docs/openapi.yaml`
under `/api/v1/events/subject`; a deprecated `GET` form of subject access
puts the selector in the URL and should not be used.
