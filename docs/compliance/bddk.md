# BDDK mapping (Turkish Banking Regulator)

> This page is not legal advice. It shows which Tamga controls are relevant
> to which requirement. Whether an organisation complies depends on its own
> processes and on an assessment by a qualified person.

Tamga controls that are relevant to articles of the banking regulator's
information systems regulation, "Bankaların Bilgi Sistemleri ve Elektronik
Bankacılık Hizmetleri Hakkında Yönetmelik", for LLM traffic that is routed
through the proxy. Tamga does not see what a person types into a chat
website in a browser. Tamga runs on your own infrastructure, so its logs and
settings stay where you run it. A request that is not blocked is still
forwarded to the provider it names.

The Regulation is addressed to banks (Md. 1). It asks for processes,
committees, documents and contracts; software does not meet them. In a bank
Tamga is itself an information system that the bank's own controls
(inventory, access, change, logging, continuity) have to cover.

How to read the table: the first column gives the article's number and its
heading as printed in the Regulation, in Turkish. Turkish words in quotation
marks are the Regulation's own. The English next to them is this page's
gloss, not an official translation, and it shortens the provision. The first
column names the article; it does not say that the control in the second
column satisfies it. Read the article itself before relying on a row.

## Sources

- Bankaların Bilgi Sistemleri ve Elektronik Bankacılık Hizmetleri Hakkında
  Yönetmelik, Resmî Gazete 15/3/2020, sayı 31069, issued under Md. 93 of
  Bankacılık Kanunu No. 5411. Consolidated text at
  <https://www.mevzuat.gov.tr/mevzuat?MevzuatNo=34360&MevzuatTur=7&MevzuatTertip=5>,
  read on 2026-10-10. That text lists one amending regulation, Resmî Gazete
  20/6/2020, sayı 31161, which replaced Md. 46 (Yürürlük); no other
  amendment note appears in it. Whether a later amendment exists that the
  consolidated text does not yet show was not checked against the Resmî
  Gazete.
- Read for this page: the headings of Md. 1 to 47, and the text of Md. 1, 3,
  6, 9, 10, 11, 12, 13, 18, 24, 25 and 29. The other articles were not read
  beyond their headings.
- Not read, and not mapped: Bankaların Destek Hizmeti Almalarına İlişkin
  Yönetmelik (Resmî Gazete 5/11/2011, sayı 28106; the Regulation refers to
  it); Bankaların İç Sistemleri ve İçsel Sermaye Yeterliliği Değerlendirme
  Süreci Hakkında Yönetmelik (Resmî Gazete 11/7/2014, sayı 29057; the
  Regulation takes its definitions of "birincil sistemler" and "ikincil
  sistemler" from Md. 3 of it); Sır Niteliğindeki Bilgilerin Paylaşılması
  Hakkında Yönetmelik (secondary sources give Resmî Gazete 4/6/2021, sayı
  31501; its official text was not opened); Bankacılık Kanunu No. 5411
  itself, including its provision on customer secrets; and the circulars
  and Board decisions of the BDDK. Any of these can bear on sending bank
  data to a language-model provider. This page was labelled "BDDK Bilgi
  Sistemleri Yönetmeliği" before 2026-10-10; no regulation has that title.

## Two things that hold for every row

**A request that is not blocked goes to the provider.** Tamga forwards it
to the provider the request names, if the policy allows that provider. The
provider is a third party and may be outside Türkiye. Tamga does not know
where the provider processes the data. `data.residency` in the policy and
`TAMGA_REGION` refuse nothing: they are compared once at startup and a
mismatch is logged.

**Tamga recognises patterns, not categories of the law.** It does not know
what "müşteri sırrı", "sır kapsamındaki veri" or "hassas veri" is. It
detects the identifier types listed in the policy and the patterns you
write. A customer's name, an account history or a credit decision written
as ordinary prose passes unless a pattern of yours matches it.

## Articles for which Tamga has a relevant control

| Article (heading in the Regulation) | Relevant Tamga control | What the control does not do |
|-------------------------------------|------------------------|------------------------------|
| Md. 6: Bilgi varlıkları envanteri ve sınıflandırılması (fıkra 6: the security class of data is set by criteria that include whether it is "hassas veri, kişisel veri ya da sır kapsamındaki veri") | Custom entities (`custom_entities` in the policy file) and stored patterns (`POST /api/v1/patterns`): your own patterns, each with a severity and an action. They can mark, in proxied traffic, identifiers that belong to a class you have defined (the shipped policy has a customer number, a file number and a staff number as examples) | Tamga keeps no asset inventory and classifies nothing. The classification guide, the owners and the review every two years (fıkra 4, 5) are the bank's. A pattern marks a format, not a security class. This row was labelled "Operasyonel Risk: Veri sınıflandırma" before 2026-10-10 |
| Md. 9: Veri gizliliği (gloss: confidentiality of data in transfer, processing and storage; encryption and key management; end-to-end secure communication and encrypted storage for "hassas veri") | TLS on the listener (`TAMGA_TLS_CERT`, `TAMGA_TLS_KEY`, minimum TLS 1.2), optional client certificates (`TAMGA_MTLS_STRICT_VERIFY`). Secret detection (`rules.secret_detection`, action `BLOCK` in the shipped policy: passwords, API keys, tokens, private keys, connection strings) and PII rules (in the shipped policy `REDACT` for IBAN, e-mail, phone, VKN, IP addresses, MERSİS number, IMEI and Turkish passport number; `BLOCK` for TC Kimlik, card number and SSN) on what is sent. The request log stores findings masked, or as a keyed hash (HMAC-SHA-256) with `data.hash_findings: true`, and not the prompt | The listener serves plain HTTP until a certificate is set. Tamga does not encrypt its database or backups, and does not manage encryption keys or their lifetimes (fıkra 2). Masking the identifiers a pattern found does not make the rest of a prompt confidential. Scanning servers for sensitive data in clear text (fıkra 3) is not something Tamga does |
| Md. 10: Verilerin paylaşılması (fıkra 1, gloss: without a provable request from the customer, and outside the exceptions in the Law, a bank may not share "müşteri sırrı niteliğindeki bilgileri" with third parties in Türkiye or abroad, or transfer it to them) | PII rules and custom entities with `BLOCK` or `REDACT`, before a proxied request is forwarded. The [`bddk-bank.yaml` template](../policy-templates.md) is a starting point that blocks TC Kimlik, card number, IBAN, VKN, MERSİS number and Turkish passport number and carries two example patterns for a customer number and an account number; what was read of Bankacılık Kanunu Md. 73 for it is in [research/turkish-dlp-landscape.md](../research/turkish-dlp-landscape.md). Provider allowlist (`providers.allowed`, `providers.blocked`); provider pools may point only at the vendor's own host or a host listed in `TAMGA_UPSTREAM_ALLOW`. With `TAMGA_REQUIRE_KEY=true`, only applications holding a Tamga key can use the proxy | Tamga does not decide whether a request may be shared. It does not know whether a text is a customer secret, whether the customer asked, or whether an exception applies. `REDACT` and the vault are not anonymisation: what is left of the text may still be a customer secret. A forwarded request has been shared with the provider. Whether that is permitted for a given use is a legal question for the bank |
| Md. 11: Kimlik ve erişim yönetimi (fıkra 1: access by "görevler ayrılığı prensibi" and through "roller ve/veya profiller"; fıkra 5: a critical operation is not to be started, approved and completed by one person) | For Tamga's own management API: three roles (admin, analyst, viewer); API keys with a scope (read, write, admin, proxy), expiry and revocation; sign-in only for accounts that were given a role (unless `TAMGA_SSO_DEFAULT_ROLE=viewer` is set); a session can be ended (`POST /api/v1/team/{id}/sessions/revoke`), and changing or removing a role ends that user's sessions; nobody can change their own role and the last admin cannot be demoted. With `governance.require_dual_control: true` a policy change is applied only when someone other than its author approves it. The actor of a change is taken from the credential, not from a header | These govern access to Tamga, not to the bank's systems or to an AI tool. The admin key is a shared secret: one actor whoever holds it (compare fıkra 10 on shared accounts). Tamga has no multi-factor step of its own; a session token lasts 24 hours unless ended. Dual control is off in the shipped policy, covers policy changes only, can be got around by an admin who creates a second key, and does not cover the policy file on disk ([operations](../operations.md#two-people-for-a-change)). The items of fıkra 4, 7, 11, 12 and 14 (blocking after failed attempts, idle sessions, password rules, account reports, usage profiles) were not checked against Tamga one by one. This row was labelled "Bilgi Sistemleri: Erişim kontrolü" before 2026-10-10 |
| Md. 13: İz kayıtlarının oluşturulması ve takibi (fıkra 3: the iz kayıtları it lists are kept "asgari beş yıl boyunca banka nezdinde"; fıkra 6: techniques that prevent, and detect, loss of integrity of the records) | Request log: one row per proxied request with the time, provider, model, action, findings, user identifier and the policy's name and version, in PostgreSQL when `TAMGA_DB_URL` is set. Audit log: administrative actions (policy, key, pattern, role, webhook and SSO changes, incident triage, reading or erasing a person's stored data) with the time, the verified actor, the kind and the target, linked in a hash chain (`GET /api/v1/audit/verify`). Alerts in CEF and LEEF can copy decisions to a SIEM | Retention is shorter than five years unless you set it: when the retention job is on, request logs are kept 30 days and audit entries 365 days by default (`TAMGA_RETENTION_REQUEST_LOGS_DAYS`, `TAMGA_RETENTION_AUDIT_DAYS`); when it is off nothing is deleted. Whether Tamga's records are among those fıkra 3 lists is the bank's assessment. The request log holds no prompt and no response. A refused credential on the management API is answered with 401 or 403 and is not written to the audit log; ordinary reads of the request log are not written either. `GET /api/v1/audit/verify` checks the entries the running process holds in memory (the most recent 512), not the whole table, and the chain is an unkeyed hash: someone who can rewrite the table can rewrite the chain. `DELETE /api/v1/events/subject` removes request-log rows for good. Without `TAMGA_DB_URL` both logs are in memory only. Backup, regular review and reporting of the records (fıkra 5, 7) are the bank's. This row was labelled "Bilgi Sistemleri: Log yönetimi" before 2026-10-10 |
| Md. 18: Siber olay yönetimi, sızma testi ve siber istihbarat paylaşımı | Alerts for blocked, stripped and warned requests to the saved destinations (CEF for Splunk and Sentinel, LEEF for QRadar, and others) and to a rule's `notify` URL; the message names the request and the kinds of finding, not the text. Incident queue (triage, resolve and reopen in the dashboard and under `/api/v1/incidents`) | These can feed the bank's own incident process; they are not that process. Tamga is not a SOME, reports nothing to the Kurum or to the Sektörel SOME, classifies no incident by severity under the bank's criteria, and does no penetration test. A blocked request is not by itself a cyber incident, and an incident may happen where Tamga sees nothing. No destination has been tried against the real service. This row was labelled "Acil Durum: Olay yönetimi" before 2026-10-10 |
| Md. 24: Değişiklik yönetimi | For Tamga's policy: every change made through the API is a revision in the policy history and an audit entry with the verified actor, where it came from and the paths that changed; a revision can be rolled back; with `governance.require_dual_control: true` a change is a proposal that someone else approves | This covers changes to Tamga's policy only, not changes to Tamga itself (upgrades, environment variables) or to the bank's systems. A change to the policy file on disk is recorded without an actor. Testing, risk assessment and the bank's approval steps are outside it |
| Md. 25: Birincil ve ikincil sistemler (fıkra 1: "Bankaların birincil ve ikincil sistemlerini yurt içinde bulundurmaları zorunludur."; fıkra 5, gloss: where an outside service or cloud service is taken for an activity within those systems, the provider's systems and their backups count as such and are kept in the country) | Self-hosted deployment: the proxy, its database and its logs run where the bank runs them. A provider route can point at a model the bank hosts itself (`TAMGA_LOCAL_URL`, or a pool host listed in `TAMGA_UPSTREAM_ALLOW`). The provider allowlist limits which providers a request may go to | Tamga does not know which of the bank's systems are primary or secondary, or whether a use of a language model falls under fıkra 5. A request forwarded to an external provider is processed wherever that provider processes it. The allowlist names providers, not countries. `data.residency` and `TAMGA_REGION` refuse nothing |

## Articles for which Tamga has no control

A reader may expect these on a page like this one. Tamga does nothing about
them.

| Article (heading in the Regulation) | Tamga control |
|-------------------------------------|---------------|
| Md. 4: Yönetim gözetimi, roller ve sorumluluklar; Md. 5: Bilgi sistemleri politika, prosedür ve süreç dokümanları; Md. 7: Bilgi sistemleri risk yönetim süreci; Md. 8: Bilgi güvenliği organizasyonu, roller ve sorumluluklar | No Tamga control |
| Md. 12: Bütünlük kontrolleri | No Tamga control for banking transactions and records. The audit log's hash chain concerns Tamga's own log and is described under Md. 13 |
| Md. 14: Ağ güvenliği; Md. 15: Güvenlik konfigürasyonu yönetimi; Md. 16: Güvenlik açıkları ve yama yönetimi; Md. 17: Fiziksel güvenlik kontrolleri; Md. 19: Bilgi güvenliği farkındalığını artırma | No Tamga control. Tamga is not a firewall or a web gateway and does not block a chat website. `TAMGA_IP_ALLOWLIST` limits which addresses may use the proxy routes, nothing more |
| Md. 26: BT operasyon yönetimi; Md. 27: Erişilebilirlik yönetimi ve yedekleme; Md. 28: Bilgi sistemleri sürekliliğinin sağlanması | No Tamga control. Backups of Tamga's database and its availability are the operator's work |
| Md. 29: Dış hizmet alımı sürecinin yönetimi | No Tamga control. Choosing a model provider, the contract with it, its audit, the exit strategy and the rules on cloud services (fıkra 11) are the bank's. Tamga is open-source software the bank runs itself; it comes with no service contract. The provider allowlist limits where requests may go; it is not a contract and assesses no provider |
| Md. 30: Bilgi sistemleri iç kontrol faaliyetleri; Md. 31: Bilgi sistemleri iç denetim faaliyetleri; Md. 32: Bulguların takibi ve güvence sağlanması | No Tamga control. The request log and the audit log can be used as evidence in an audit |
| Üçüncü Kısım: Elektronik Bankacılık Hizmetleri (from Md. 34, among them Md. 34: Kimlik doğrulama ve işlem güvenliği) | No Tamga control. These concern how a bank authenticates its customers and secures their transactions |

## Where each control is described

The controls named on this page are the ones listed, with the place each is
described, at the end of the [KVKK page](kvkk.md#the-controls-and-where-each-is-described).
The incident routes and the session routes are in `proxy/docs/openapi.yaml`;
roles and sessions are in
[operations: who may call what](../operations.md#who-may-call-what).
