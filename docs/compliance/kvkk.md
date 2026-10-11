# KVKK mapping (Turkish Data Protection Law)

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Tamga controls that are relevant to KVKK articles. The controls apply only
to traffic that is routed through the proxy. Tamga does not see what a
person types into a chat website in a browser. Tamga runs on your own
infrastructure, so its logs and settings stay where you run it. A request
that is not blocked is still forwarded to the provider it names.

On this page: the articles of the law; which guidance of the authority on
AI exists; a mapping to its document on generative AI tools at work (the
"Gölge Yapay Zekâ" text), to its document on agentic AI ("Etken Yapay
Zekâ") and to its 2025 recommendations on AI; and a list of the controls
with where each is described.

How to read the citations: article headings and text in Turkish are taken
from the official text of the law. For the authority's documents, the Turkish
words in quotation marks are the document's own terms, given with the page
they are on so that the passage can be found; the English next to them is
this page's own gloss, not an official translation, and it shortens the
passage. Read the passage itself before relying on a row.

## Articles of the law (6698 sayılı Kanun)

Source: Kişisel Verilerin Korunması Kanunu, Kanun No. 6698, Resmî Gazete
7/4/2016, sayı 29677; consolidated text at
<https://www.mevzuat.gov.tr/MevzuatMetin/1.5.6698.pdf>, read on 2026-10-10.
Article headings below are the law's own. The first column names the
article; it does not say that the control in the second column satisfies it.

| KVKK article (heading in the law) | Tamga control |
|-----------------------------------|---------------|
| Md. 4: Genel ilkeler. Fıkra 2/ç: "İşlendikleri amaçla bağlantılı, sınırlı ve ölçülü olma" (gloss: related to, limited to and proportionate to the purpose; the law does not use the words "veri minimizasyonu") | PII redaction rule (`pii_detection`, action `REDACT`): masks the PII types listed in the rule before a proxied request is forwarded. Supports limiting what is sent, for traffic through the proxy. It masks only the types listed and only what the patterns recognise |
| Md. 4: Genel ilkeler. Fıkra 2/d: "…işlendikleri amaç için gerekli olan süre kadar muhafaza edilme" (gloss: kept only as long as needed) | Retention job (`TAMGA_RETENTION_ENABLED`, `TAMGA_RETENTION_REQUEST_LOGS_DAYS`, `TAMGA_RETENTION_AUDIT_DAYS`, `TAMGA_RETENTION_INCIDENT_CLOSED_DAYS`). It covers what Tamga stores, not what a provider stores. Request logs are dropped a monthly partition at a time, so a row can outlive the period by up to a month. The policy key `data.retention_days` has no effect |
| Md. 5: Kişisel verilerin işlenme şartları (fıkra 1: açık rıza; fıkra 2: the conditions under which consent is not required) | PII block rule (`pii`, action `BLOCK`): stops a proxied request that contains one of the identifiers listed in the rule (TC Kimlik, card number and SSN in the shipped policy). Tamga does not know whether consent was given or which condition of this article a processing rests on. The rule is one control toward limiting what is sent |
| Md. 7: Kişisel verilerin silinmesi, yok edilmesi veya anonim hâle getirilmesi | Erasure of a person's stored request logs (`DELETE /api/v1/events/subject`, admin only, final) and the retention job above. Both act on Tamga's own request logs only: not on the audit log, on alerts already sent, or on anything a provider holds. `REDACT` and the vault are not "anonim hâle getirme" as Md. 3/1-b defines it: they mask or replace the identifiers a pattern found and leave the rest of the text |
| Md. 9: Kişisel verilerin yurt dışına aktarılması (replaced by Kanun No. 7499 md. 34, 2/3/2024: yeterlilik kararı, uygun güvenceler such as the standart sözleşme, and the occasional cases of fıkra 6) | Provider allowlist (`providers.allowed`, `providers.blocked`): limits which providers a proxied request may go to. Provider pools may point only at the vendor's own host or a host listed in `TAMGA_UPSTREAM_ALLOW`. Self-hosted deployment: Tamga's own logs and settings stay on your infrastructure. A request forwarded to an external provider leaves your infrastructure, and Tamga does not know where that provider processes it or whether the transfer has a basis under this article. `data.residency` and `TAMGA_REGION` refuse nothing |
| Md. 11: İlgili kişinin hakları; Md. 13: Veri sorumlusuna başvuru (answer within thirty days, fıkra 2) | Subject access (`POST /api/v1/events/subject`, admin only): returns the stored request logs for one selector, `user_id` or `email` (compared with the user identifier stored on the request) or `tckn` (a national id, sent in the body and looked for in the stored findings by the forms the log holds it in: the masked match, or its keyed hash when `data.hash_findings` is on; another id with the same first and last two digits is found too), or `tckn_hash` (a stored hash as it is). Each call is written to the audit log as `privacy.subject_access` with the kind of selector, not its value. It answers for what Tamga stores about the sender of a request. It does not find a person who was only mentioned in a prompt, because prompt text is not stored. Receiving and answering an application is the organisation's own process |
| Md. 12: Veri güvenliğine ilişkin yükümlülükler (fıkra 1: "uygun güvenlik düzeyini temin etmeye yönelik gerekli her türlü teknik ve idari tedbirleri almak") | TLS on the listener (`TAMGA_TLS_CERT`, `TAMGA_TLS_KEY`, minimum TLS 1.2), optional client certificates (`TAMGA_MTLS_STRICT_VERIFY`), roles on the management API (admin, analyst, viewer), scoped API keys (read, write, admin, proxy) with expiry and revocation. Each is one control toward this article |
| Md. 12, fıkra 3: the controller's duty to audit ("gerekli denetimleri yapmak veya yaptırmak") | Request log (provider, model, action and findings for each proxied request, kept in PostgreSQL when `TAMGA_DB_URL` is set) and audit log hash chain (administrative actions such as policy, key, pattern and webhook changes, checked with `GET /api/v1/audit/verify`, which covers the most recent 512 entries the process holds, not the whole table). Both can be used as evidence in an audit. The law has no article named "işleme kaydı"; this row was labelled "Md. 7: İşleme kaydı" before 2026-10-10 |
| Md. 12, fıkra 5: notifying the Kurul and the person "en kısa sürede" when data is obtained by others unlawfully. The Kurul's decision of 24.01.2019, no. 2019/10, reads this as 72 hours ([announcement of 15.02.2019](https://www.kvkk.gov.tr/Icerik/5362/Veri-Ihlali-Bildirimi)) | Alerts: a blocked, stripped or warned proxied request is posted to the saved alert destinations (Slack, Teams, Splunk, Sentinel, QRadar and others) and to the `notify` URL of the rule that matched. The message names the request and the kinds of finding, not the text. No destination has been tried against the real service. These help a team notice an event. A blocked request is not by itself a breach, and a breach may happen where Tamga sees nothing. Notifying the authority is the organisation's own process. This row was labelled "Md. 13" before 2026-10-10 |
| Md. 10: Veri sorumlusunun aydınlatma yükümlülüğü; Md. 16: Veri Sorumluları Sicili (VERBİS) | No Tamga control |

## The authority's guidance on AI: what exists

Documents of the Kişisel Verileri Koruma Kurumu (kvkk.gov.tr) that bear on
the use of language models, as found on 2026-10-10. None of them is a
regulation; each describes itself as guidance, recommendations or matters
that may be taken into account.

| Document | Date | Read in full | Mapped below |
|----------|------|--------------|--------------|
| [İş Yerlerinde Üretken Yapay Zekâ Araçlarının Kullanımı](https://www.kvkk.gov.tr/Icerik/8674/is-yerlerinde-uretken-yapay-zeka-araclarinin-kullanimi) ([PDF](https://www.kvkk.gov.tr/SharedFolderServer/CMSFiles/MTY5YTdkNjdjNzJlMjM.pdf)). Its section 2 is the authority's text on "Gölge Yapay Zekâ" (Shadow AI) | Document dated Şubat 2026; announced 5 March 2026 | Yes, 15 numbered pages | Yes |
| [Etken Yapay Zekâ (Agentic AI)](https://www.kvkk.gov.tr/Icerik/8683/etken-yapay-zeka-agentic-ai) ([PDF](https://www.kvkk.gov.tr/SharedFolderServer/CMSFiles/MTY5YWE4ZDE0NWYwMWI.pdf)) | Document dated Şubat 2026; announced 12 March 2026 | Yes, 42 numbered pages | Yes |
| [Yapay Zekâ Alanında Kişisel Verilerin Korunmasına Dair Tavsiyeler](https://www.kvkk.gov.tr/Icerik/7048/Yapay-Zeka-Alaninda-Kisisel-Verilerin-Korunmasina-Dair-Tavsiyeler), KVKK Yayınları No: 76 | The edition read is dated Nisan 2025. Secondary sources date the first publication to 15 September 2021; that date was not checked on the authority's site | Yes, 17 numbered pages | Yes, briefly |
| [Sohbet Robotları (ChatGPT Örneği) Hakkında Bilgi Notu](https://www.kvkk.gov.tr/Icerik/8047/Sohbet-Robotlari-ChatGPT-Ornegi-Hakkinda-Bilgi-Notu) | 8 November 2024 | No: only the announcement page was opened | No |
| Üretken Yapay Zekâ ve Kişisel Verilerin Korunması Rehberi (15 Soruda). The workplace document points to it by this name (p. 10) | Secondary sources: 24 November 2025. The authority's own page for it was not found | No | No |

No mapping is written for a document that was not read. There is no
separate guide titled "Shadow AI" or "Gölge Yapay Zekâ": the term is the
subject of section 2 of the workplace document.

## İş Yerlerinde Üretken Yapay Zekâ Araçlarının Kullanımı ("Gölge Yapay Zekâ")

Kişisel Verileri Koruma Kurumu, Şubat 2026, announced 5 March 2026
([page](https://www.kvkk.gov.tr/Icerik/8674/is-yerlerinde-uretken-yapay-zeka-araclarinin-kullanimi),
[PDF](https://www.kvkk.gov.tr/SharedFolderServer/CMSFiles/MTY5YTdkNjdjNzJlMjM.pdf)).
The document addresses "şirket, kurum ve kuruluşlar" (p. 3) and is about
generative AI tools offered by third parties and open to the public, used at
work; it says it aims to raise awareness, point to risks and encourage
informed use (p. 3; gloss). Section 2 (pp. 5-10) defines "Gölge YZ" (Shadow
AI) as staff using such tools in their work outside the organisation's
knowledge, approval or control (p. 5; gloss) and lists six kinds of risk
(pp. 8-9). Section 3 (pp. 10-15) lists five matters that organisations may
take into account. The document also says that an approach resting on
banning the tools outright is not expected to give realistic results, because
it may push use out of sight (p. 10; gloss).

**What Tamga is in this picture.** Shadow AI is, by the document's own
definition, use that the organisation does not see. Tamga sees one thing: a
request that an application, an agent or a script sends to a model provider
through the proxy. A person who opens a chat website in a browser, a
personal account on a phone, a browser extension or a desktop assistant that
calls the provider directly: none of that passes through Tamga, and Tamga
does nothing about it. That is most of what the document describes. Closing
that gap needs things Tamga does not provide: an approved tool that staff
actually prefer, network controls (a secure web gateway, DNS or firewall
rules), managed devices and browsers, a written policy and training. A
browser extension that asks a local Tamga before a paste is planned and is
not in the repository. What Tamga can be is part of the approved route: the
internal assistant or integration that the organisation offers instead, with
its traffic sent through the proxy.

### Matters to take into account (section 3)

| What the document says | Relevant Tamga control | What the control does not do |
|------------------------|------------------------|------------------------------|
| A clear corporate policy or guidance: which tools, for which activities, which kinds of information may be given as input, how outputs are used ("kurumsal politika veya yönlendirme çerçevesi", p. 11) | The policy file, for proxied traffic: `providers.allowed` and `providers.blocked` name the providers a request may go to; `rules.pii_detection`, `rules.pii`, `rules.secret_detection` and `custom_entities` name kinds of data that are masked or stopped | It is not the organisation's policy and tells no employee anything. It expresses only what a pattern can detect. It says nothing about which activities a tool may be used for or how outputs are used |
| Employees take care with sensitive corporate information and personal data; anonymised, generalised or abstract wording is preferred; names, dates, places left out; more caution with health, financial and legal data ("anonimleştirilmiş, genelleştirilmiş ve soyut ifadelerin tercih edilmesi", pp. 12-13) | PII detection with `REDACT` or `BLOCK` before a proxied request is forwarded. Built-in types: `tc_kimlik` (checksum validated), `credit_card` (Luhn validated), `iban` (country length and mod-97 check), `email`, `phone_tr`, `phone`, `vkn`, `ssn`, `passport_number`, `date_of_birth`, `medical_record`, `ip_public`, `ip_private` and others in `proxy/internal/scanner/pii.go`; and four that are found only where the text names them: `mersis_no` (the embedded tax number or national id is validated), `imei` (Luhn validated), `passport_tr` (no check digit exists) and `license_plate_tr` (`pii_tr_ids.go`). Shipped policy: `REDACT` for IBAN, e-mail, phone, VKN, IP addresses, MERSİS number, IMEI and Turkish passport number; `BLOCK` for TC Kimlik, card number and SSN; a vehicle plate is found and not acted on. [Policy templates](../policy-templates.md) are starting points that list more; which products and laws name which identifier is in [research/turkish-dlp-landscape.md](../research/turkish-dlp-landscape.md). Your own identifiers as `custom_entities` (the shipped policy has a customer number, a file number and a staff number as examples). With `vault.enabled`, values under a `REDACT` rule go to the provider as numbered placeholders and are put back in the response | There is no built-in pattern for a person's name, a postal address or a place, and nothing recognises health, financial or legal content written as ordinary prose. Masking the identifiers a pattern found is not anonymisation: the rest of the text can still identify someone. The vault is reversible by design (originals are kept for the request, and encrypted in Redis for a short time when `TAMGA_VAULT_KEY` is set); it is off in the shipped policy. Nothing here applies to text typed into a tool outside the proxy |
| Trade secrets, source code and other confidential information shared with external tools (risk named on p. 8; care asked of employees on p. 12) | `rules.secret_detection` (API keys, tokens, private keys, connection strings and the other types listed in the shipped policy, action `BLOCK`); `custom_entities` and stored patterns (`POST /api/v1/patterns`) for project names, document markings and the like | Tamga does not know what a trade secret is. Source code, a product design or a strategy document passes unless a pattern you wrote matches it |
| Data security and access control: staff reach only the tools the organisation has designated, with conditions of use defined; access by duty and need; role-based approaches for which groups may use the tools ("yalnızca kuruluş tarafından belirlenen … ÜYZ araçlarına erişebilmesi", p. 14) | Provider allowlist as above. Provider pools may point only at the vendor's own host or a host in `TAMGA_UPSTREAM_ALLOW`. A Tamga key per application (`X-Tamga-Key`, scope `proxy`); with `TAMGA_REQUIRE_KEY=true` a request without a key gets 401. A key carries an organisation and may carry a role that policy exceptions are evaluated with. `TAMGA_IP_ALLOWLIST` limits which addresses may use the proxy routes. `rate_limit.max_requests_per_minute` and `rate_limit.max_tokens_per_day` are counted per key | These decide who may use the proxy and where the proxy may send, not who may reach an AI tool. A key identifies an application; the person behind a request is whatever the application says in `X-Tamga-User-Id`, unless the key itself names a user. Role-based access for employee groups is the identity provider's and the application's work |
| Controls that restrict access to external platforms at network level; which devices may be used, for example only corporate devices ("ağ düzeyinde sınırlandırılmasına yönelik kontroller", p. 14) | No Tamga control | Tamga is not a firewall, a web gateway or a device manager. It does not block a chat website and does not know what device a request came from |
| Visibility and auditability: for tools outside corporate recording and monitoring it is hard to establish afterwards which data was used for what (risk "Denetlenebilirlik ve hesap verebilirliğe ilişkin riskler", p. 8; visibility also pp. 5, 7) | Request log for each proxied request: provider, model, the action taken, the findings (matches stored masked, or as a keyed hash (HMAC-SHA-256) with `data.hash_findings: true`), the user identifier, the name and version of the policy that decided. Audit log with a hash chain for administrative actions, checked with `GET /api/v1/audit/verify`, which covers the most recent 512 entries the process holds, not the whole table. Alerts for `BLOCK`, `STRIP` and `WARN` decisions | The log does not hold the prompt or the response, so it shows that an e-mail address was masked in a request, not what was asked. Request logs need `TAMGA_DB_URL`; without it only the last 1000 requests are held in memory. Nothing is recorded for a tool used outside the proxy, which is the case the document is about |

### Matters in the document for which Tamga has no control

- Reading the privacy notices and policies of a tool: what it collects, for
  what purposes, with whom it shares, how long it keeps (p. 12).
- Over-reliance on outputs ("otomasyon ön yargısı"), keeping outputs under
  human review, and reviewing them for accuracy, suitability and bias
  (pp. 13-14). `output_rules` scan a response for the finding types listed
  in the policy (identifiers, secrets); they do not judge whether an answer
  is true, fair or fit for use.
- Announcing the policy, keeping it reachable, informing staff of changes,
  training, and a feedback channel for staff (pp. 14-15).
- The risks named under decision quality and under corporate reputation
  and trust (p. 8).
- Taking the authority's "Üretken Yapay Zekâ ve Kişisel Verilerin Korunması
  Rehberi (15 Soruda)" into account (p. 10).

## Etken Yapay Zekâ (Agentic AI)

Kişisel Verileri Koruma Kurumu, Şubat 2026, announced 12 March 2026
([page](https://www.kvkk.gov.tr/Icerik/8683/etken-yapay-zeka-agentic-ai),
[PDF](https://www.kvkk.gov.tr/SharedFolderServer/CMSFiles/MTY5YWE4ZDE0NWYwMWI.pdf)).
The document explains what agentic AI systems and "YZ Aracıları" (AI agents)
are, gives use cases and risks, and sets out in general terms matters that
may be taken into account for the protection of personal data over the life
cycle of such systems (p. 5; gloss). It names no single addressee; it speaks
of "geliştiriciler", "yerleştiriciler (deployers)" and other actors (pp. 30,
38). It uses "Etken YZ" for agentic AI and "YZ Aracısı" for an AI agent
(p. 4, footnotes 1 and 2). Section 5 (pp. 25-33) describes risks to personal
data; section 6 (pp. 34-40) lists seven matters to take into account. The
document says its assessments rest on present knowledge and may change
(p. 40; gloss).

**What Tamga sees of an agent.** An agent that calls its model through the
proxy sends, on every turn, the whole conversation: the system prompt, the
user's text, the results its tools returned, and the tool calls the model
made earlier. Tamga reads that request by message and by role (`system`,
`user`, `assistant`, `tool`, and `tool_definition` for tool descriptions),
and a rule can be limited to roles with `applies_to`. It does not run, see
or approve the tools themselves. Traffic between an agent and its tools or
MCP servers does not pass through Tamga; what a tool returned is seen one
step later, when the agent sends it to the model. In a streamed response the
model's prose is scanned when `output_rules.streaming.enabled` is set;
tool-call arguments and thinking in the stream are forwarded unread. Images,
audio and PDFs are not read. A full session of a real agent through Tamga
has not been recorded yet ([docs/agents.md](../agents.md)).

### Matters to take into account (section 6)

| What the document says | Relevant Tamga control | What the control does not do |
|------------------------|------------------------|------------------------------|
| A human-centred approach with sufficient and meaningful human involvement or oversight; say under which conditions and at which stages oversight comes in ("yeterli ve anlamlı insan katılımı veya gözetiminin sağlanması", p. 35). At deployment, extra human review for high-risk decisions or processing; after deployment, monitoring of behaviour (p. 36) | A `BLOCK` ends the agent's turn with a 403; `STRIP` removes a tool result that carries a finding and lets the session go on. `BLOCK`, `STRIP` and `WARN` decisions are sent to the alert destinations, so a person can be told. `rate_limit.max_requests_per_minute` and `rate_limit.max_tokens_per_day` (per key) and `cost.max_tokens_per_day` and `cost.max_cost_usd_per_day` (per organisation) bound how much an agent can do in a day. The `operator_state` scanner can refuse a request that refers to a decision which an external decisions file does not show in the required state | None of this is human oversight of an agent's actions. Tamga has no step where a person approves an action before it happens, and it does not see the action: a tool runs in the agent, not in the proxy. The limits are on requests and tokens, not on what a tool may do. The shipped policy has no `cost` block. The `operator_state` scanner reads files written by another system and is not an approval workflow |
| At development, people define which kinds of data the system may reach and test those limits (p. 36). Purpose limitation and data minimisation: an agent may reach more data than the task needs (risk, "amaçla sınırlılık ve veri minimizasyonu", pp. 26-28) | Role-aware rules: a PII rule with `applies_to: [tool]` masks or stops identifiers in tool results before they are forwarded to the provider; tool-call arguments in the request are scanned as `assistant` text. `POST /api/v1/scan` gives the policy's verdict for a text and a role without forwarding anything, for a check inside the agent or in CI | Tamga limits what reaches the model provider through the proxy. It does not limit what the agent's tools can read: the data is already in the agent when Tamga sees it. Which systems an agent may access is set in the agent and in those systems |
| Transparency, explainability and traceability of what the system did; mechanisms that let behaviour be monitored; technical limits and protective controls that keep a system within its intended purpose ("teknik sınırlamaların ve koruyucu kontrol mekanizmalarının", pp. 36-37; risk on pp. 23, 30) | One request-log row per model call: the findings with the role of the text they were in, the action, the policy name and version, the Tamga key's organisation and the user identifier. A key per agent (`X-Tamga-Key`) separates one agent's calls from another's. Audit log with a hash chain for changes to the policy, keys and patterns, with the verified actor | The log is a record of what Tamga decided about each model call. It is not a trace of the agent: it holds no prompt, no response, no tool call and no link between the steps of one task. It explains nothing about why the model chose an action. Calls between agents, and between an agent and its tools, are not in it |
| Security and resilience: manipulated inputs that make a system disclose data or go beyond its restrictions, with wider effects where it starts automatic actions (risk, "güvenlik ve sistem dayanıklılığı", p. 32) | Injection detection on the request, including tool results (`applies_to: [tool]`) and tool descriptions (`tool_definition`), with `STRIP` or `BLOCK`; the optional inline classifier (`scan.classifier`); canary tokens for a leaked system prompt (`canary`); secret detection on what is sent | Detection is by patterns and an optional model and does not catch every injection; measured figures are in `docs/benchmarks/`. A tool call that the model produces in a streamed response is forwarded unread, so Tamga does not stop an agent from carrying out a harmful call; it sees the call only when it comes back in the next request, after the tool has run. Content in images, audio and PDFs is not read |
| "Tasarımdan itibaren mahremiyet" and "varsayılan olarak mahremiyet"; integrating "mahremiyet artırıcı teknolojiler" (pp. 38-39) | Scanning happens before a request is forwarded. `REDACT` masks listed identifiers; with `vault.enabled` they are replaced by placeholders and restored in the response, in streams too. The request log holds findings and metadata, not the text; matches are stored masked, or as a keyed hash (HMAC-SHA-256) with `data.hash_findings: true` | These concern what Tamga forwards and stores. They do not make an agent's design privacy-preserving. The vault is reversible tokenisation of what a pattern found, not anonymisation; it rewrites the request body, so an agent that needs the body byte for byte should leave it off. Whether a technique counts as a privacy-enhancing technology in the document's sense is a judgement for the organisation |
| Governance: review existing data protection and governance mechanisms for agentic systems (p. 39) | For Tamga's own configuration: roles and scoped keys on the management API; with `governance.require_dual_control: true` a policy change is applied only when someone other than its author approves it; every change is in the audit log with the verified actor and the paths that changed | This governs changes to Tamga's policy, not the organisation's agents. An admin can create a second key and approve with it, and a change to the policy file on disk is outside it ([operations](../operations.md#two-people-for-a-change)) |

### Matters in the document for which Tamga has no control

- Keeping personal data accurate where outputs are used as inputs to later
  steps, and limiting the effect of wrong results (p. 37; risk on p. 31).
- Defining the roles of the actors (developers, deployers and others) and
  dividing duties, powers and responsibilities among them (pp. 37-38; risk
  on pp. 30-31).
- Risk assessment over the life cycle, and where suitable a "veri koruma
  etki değerlendirmesi" (p. 39). Tamga's logs can be one input to it.
- Awareness and training for those who use or operate the systems (p. 39).
- The legal basis of processing as an agent's purposes shift ("işleme
  şartı", p. 28); special categories of data (p. 29); inferences and
  profiles built by combining sources (p. 29); bias and discrimination
  (p. 24); a system's choice of unintended ways to reach a goal (p. 33).

## Yapay Zekâ Alanında Kişisel Verilerin Korunmasına Dair Tavsiyeler

Kişisel Verileri Koruma Kurumu, KVKK Yayınları No: 76, edition dated Nisan
2025
([document](https://www.kvkk.gov.tr/Icerik/7048/Yapay-Zeka-Alaninda-Kisisel-Verilerin-Korunmasina-Dair-Tavsiyeler)).
Recommendations under Law 6698 for developers, manufacturers, service
providers and decision makers working in the field of AI (p. 7; the document
gives this summary in English itself). It has three lists: general
recommendations (pp. 10-12, ten items), recommendations for "geliştiriciler,
üreticiler ve servis sağlayıcılar" (pp. 12-14, fourteen items) and for
"karar alıcılar" (pp. 14-16, twelve items). The booklet carries a notice
restricting reproduction, so it is not quoted here; page numbers are given
instead. It is written for those who build AI systems and set policy. Tamga
is a proxy in front of a model someone else built, so few items meet a
control.

| What the document recommends (gloss, with page) | Relevant Tamga control | What the control does not do |
|--------------------------------------------------|------------------------|------------------------------|
| Where the same result can be reached without processing personal data, prefer processing the data in anonymised form (general, p. 12); keep the use of personal data to the minimum (developers, p. 12) | `REDACT` rules and the vault, for the identifiers listed in the policy, before a proxied request is forwarded | Masking or tokenising the identifiers a pattern found is not anonymisation in the law's sense. Whether the task can be done without personal data is a design decision of the application |
| Develop and run systems under data protection by design from the first stage (general, p. 11) | Scanning in the request path before forwarding; findings stored masked; prompt text not stored | Tamga is one component. It does not make the system around it compliant by design, and a compliance programme for the project (same item) is the organisation's |
| Design a way for a user's data to be deleted, destroyed or anonymised (developers, p. 14) | `DELETE /api/v1/events/subject` and the retention job, for Tamga's own request logs | Nothing is deleted at the provider or in the application |
| Stricter technical and administrative measures where special categories of data are processed (general, p. 11) | `custom_entities` and the `medical_record` pattern can mark some such data in proxied traffic | There is no detector for special categories as such (health, religion, union membership and the rest of Md. 6 written in prose) |

The other thirty-one items have no Tamga control: respect for fundamental
rights and the general principles (p. 10-11); the data subject's control
over processing (p. 11); a privacy impact assessment where risk is high
(p. 11); fixing who is controller and who is processor (p. 12); preventing
discrimination and bias (p. 12); algorithms
used out of context (p. 13); consulting academic and independent experts
(p. 13); the right to object and not being subject to solely automated
decisions (p. 13); informing users and a consent mechanism (p. 14); less intrusive alternatives and accountability
across the life cycle (p. 14); and all twelve items for decision makers
(pp. 14-16).

## The controls, and where each is described

Each control named on this page, with the place its behaviour is written
down. Settings are in the policy file unless they start with `TAMGA_`, which
are environment variables.

| Control | Setting or endpoint | Described in |
|---------|---------------------|--------------|
| PII detection, built-in types and validation | `rules.pii_detection`, `rules.pii`; actions `BLOCK`, `STRIP`, `REDACT`, `WARN`, `LOG`, `PASS` | `proxy/internal/scanner/pii.go`, `pii_tr_ids.go`, [`proxy/tamga-policy.yaml`](../../proxy/tamga-policy.yaml), [policy templates](../policy-templates.md) |
| Custom entities and stored patterns | `custom_entities`; `POST /api/v1/policies/custom-entities`, `POST /api/v1/patterns` | [operations: custom entities](../operations.md#custom-entities) |
| PII tokenisation vault | `vault.enabled`, `TAMGA_VAULT_KEY`, `TAMGA_VAULT_TTL_SECONDS` | [operations: vault](../operations.md#vault--reversible-pii-tokenization) |
| Provider allowlist and pools | `providers.allowed`, `providers.blocked`, `providers.pools`, `TAMGA_UPSTREAM_ALLOW` | [operations: provider pools](../operations.md#provider-pools) |
| Response scanning | `output_rules` (`block_on`, `redact_on`, `streaming.enabled`); commented out in the shipped policy | [operations: streamed responses](../operations.md#scanning-streamed-responses) |
| Roles, scoped keys, verified actor | admin, analyst, viewer; key scopes `read`, `write`, `admin`, `proxy` | [operations: who may call what](../operations.md#who-may-call-what) |
| Two people for a policy change | `governance.require_dual_control` | [operations: two people for a change](../operations.md#two-people-for-a-change) |
| Audit log and its hash chain | `GET /api/v1/audit/verify` | [operations: who may change the policy](../operations.md#who-may-change-the-policy) |
| A person's stored data | `POST /api/v1/events/subject`, `DELETE /api/v1/events/subject` (admin) | `proxy/docs/openapi.yaml` |
| Retention | `TAMGA_RETENTION_ENABLED`, `TAMGA_RETENTION_REQUEST_LOGS_DAYS`, `TAMGA_RETENTION_AUDIT_DAYS`, `TAMGA_RETENTION_INCIDENT_CLOSED_DAYS` | [operations: configuration](../operations.md#configuration) |
| What a stored request holds; hashed findings | `TAMGA_DB_URL`, `data.hash_findings` | [operations: what a stored request holds](../operations.md#what-a-stored-request-holds) |
| Alerts and where they may go | alert destinations, a rule's `notify`, `TAMGA_ALERT_ALLOW_PRIVATE` | [operations: alerts](../operations.md#alerts) |
| Keys for applications and agents | `X-Tamga-Key`, `TAMGA_REQUIRE_KEY`, `TAMGA_IP_ALLOWLIST` | [operations: keys for applications](../operations.md#keys-for-applications) |
| Rate limits and daily caps | `rate_limit.*`, `cost.max_tokens_per_day`, `cost.max_cost_usd_per_day` | [operations: cost control](../operations.md#cost-control-and-budget-enforcement) |
| Roles of text in a request, `STRIP`, what is read for agents | `applies_to` | [agents.md](../agents.md) |
| Verdict for a text without forwarding it | `POST /api/v1/scan` | [operations: checking a text](../operations.md#checking-a-text-without-sending-it) |
| Operator-state scanner | `operator_state`, `TAMGA_OPERATOR_STATE_DECISIONS_PATH` | [`proxy/tamga-policy.yaml`](../../proxy/tamga-policy.yaml) |
