# Changelog

## v0.9.0 (unreleased)

### Licence
- **Tamga is open source only: AGPL-3.0, and nothing else.** There is no
  commercial licence and no paid edition. `LICENSE-COMMERCIAL.md` is removed,
  and so is the commercial appendix that followed the AGPL text in `LICENSE`.
  Packages declare `AGPL-3.0-only`.
- **The plan layer is gone.** A `pricing:` section in the policy selected a
  tier with a monthly request cap and a switch for custom entities. Nothing
  is limited by plan any more: the 429 `monthly_request_limit_exceeded`, the
  403 for custom entities on a tier, the `X-Tamga-Tier` headers, the `tier`
  fields in the health detail and `TAMGA_TIER` no longer exist. **A policy
  that still has a `pricing:` section is refused at load, with a message
  saying to remove it.** Limits are set under `rate_limit` and `cost`, as
  before.

### Deployment and operations
- **The PDF reports did not work in the compose stack**: the proxy looked
  for the analyzer on a port nothing listens on. `TAMGA_ANALYZER_HTTP_URL`
  now defaults to `http://localhost:8444` and compose sets it. Report errors
  carry a `code`; a timeout is 504 and a bad analyzer answer 502 (both were
  503). The caller's query is no longer passed on as written.
- **Migration 019 drops five unused tables** (`api_keys`, `policies`,
  `alerts`, `event_outbox`, `daily_stats`). **Upgrade the proxy first**, then
  apply it. `TAMGA_RETENTION_ALERTS_DAYS` and
  `TAMGA_RETENTION_DAILY_STATS_DAYS` are no longer read.
- The HashiCorp Vault settings (`TAMGA_VAULT_ADDR` and the others; not
  `TAMGA_VAULT_KEY`) are no longer read. The proxy connected to Vault and
  resolved nothing through it.
- The Helm chart was checked with Helm itself. Resource names are no longer
  doubled for a release called `tamga-proxy`, **which renames the resources
  of an existing release of that name**.
- CI runs on `main`; it triggered on a branch that does not exist, and the
  integration smoke test had never run.
- **Compose did not pass the session secret or the GitHub sign-in settings to
  the proxy**, so sign-in could not be turned on from `.env`. It now passes
  `TAMGA_JWT_SECRET`, `TAMGA_GITHUB_CLIENT_ID`, `TAMGA_GITHUB_CLIENT_SECRET`,
  `TAMGA_GITHUB_OAUTH_CALLBACK_URL` and `TAMGA_SSO_DEFAULT_ROLE`, each empty
  unless set. **A `.env` that already holds them turns sessions on at the
  next `up`**; one without them changes nothing. `.env.example` lists them,
  commented out and empty.
- The Helm chart has `analyzer.sendContent` (default `false`) for
  `TAMGA_ANALYZER_SEND_CONTENT`. A release that sets the variable under `env`
  keeps working; the render fails when it is set in both places.
- `.gitignore` ignored any directory called `tamga`, so a new source file
  under `proxy/cmd/tamga/` was left out of a commit without a word. The
  pattern is tied to the repository root; the built binaries are still
  ignored (`bin/`, `proxy/tamga`, `proxy/cmd/tamga/tamga`, `*.exe`).
- **`npm run lint` in `dashboard/` stopped with the build guard's error on a
  machine with a development key.** `next lint` loads `next.config.js` as a
  production build does, so `NEXT_PUBLIC_ADMIN_KEY` in `.env.local` or the
  environment tripped the guard that keeps the key out of a production
  bundle. The script is now `eslint app components lib`: the same
  configuration (`eslint.config.mjs`) over the directories `next lint`
  checked, without loading `next.config.js`, and `next lint` is deprecated
  in any case. CI runs `npm run lint`. The guard is unchanged: `next build`
  with the variable set still stops.

### Security
- **A rule that said REDACT could forward the value unmasked.** On a request
  only PII and custom-entity findings were masked. A secret, a competitor
  name, or a custom entity reached through a `custom` rule went to the
  provider as it was, with no warning. Every finding that has a place in the
  text is now masked, with a marker that holds no part of the value. A
  secret or a competitor name is never put in the vault, so it is not
  restored in the answer. If a REDACT verdict cannot be carried out, the
  request is blocked, not forwarded.
  **A policy that asks REDACT for a finding with nothing to mask (an
  injection, content moderation, operator state) no longer loads**, and says
  which actions are possible. The same for `output_rules.redact_on` naming
  such a kind.
- **The analyzer's LLM judge could send the text of a blocked request to
  Anthropic.** The proxy copied request bodies to the analyzer, blocked ones
  as they arrived, and the judge called out whenever a key was present,
  a placeholder included. The proxy now sends the analyzer no request text
  unless `TAMGA_ANALYZER_SEND_CONTENT=true`, never a blocked request, and
  only the body as forwarded. The judge runs only with
  `TAMGA_LLM_JUDGE=anthropic` and a real key. **Both are off by default: a
  deployment that relied on the analyzer's deep scan or the judge has to turn
  them on.** `.env.example` no longer ships a placeholder key.
- **A competitor entry's `action` decided nothing**, and an unknown `action`
  on a custom entity acted as PASS. Both are applied now; with a `custom` or
  `competitor` rule as well, the stronger wins. An `action` that is not an
  action name stops the load. **A competitor entry written with
  `action: block` now blocks.**
- STRIP replaced only the first part of an instruction split over two content
  parts, and stripped the wrong part for a finding tied to no segment. Both
  parts are stripped; an unlocated finding blocks.
- An answer masked in a stream could keep the part of a value that only one
  of two overlapping findings covered.
- **Erasing a person's data could erase everyone's.** The national-id hash
  selector of `DELETE /api/v1/events/subject` was matched as a pattern, so a
  selector of `%` deleted every stored request of the organisation. It must
  now be 64 hexadecimal characters. The response also said rows were
  soft-deleted; the delete is final, and it says so.
- **Reading a person's data left no trace.** `GET` and `POST
  /api/v1/events/subject` now write an audit entry with who asked, the kind
  of selector and the number of rows. Neither it nor the erase entry holds
  the selector's value any more; the erase entry used to.
- **The incident report printed whatever figures the caller sent.** The
  proxy computes them for the asked range, from the same source as the
  summary. `GET /api/v1/reports/incident/pdf` takes `range` only; counts in
  the query are ignored.
- **`POST /api/v1/auth/github/exchange` is removed.** It exchanged any OAuth
  code for a session with no state check, and nothing called it.
- The session endpoints answer 503 `sessions_unavailable` when no session
  secret is configured, instead of each failing in its own way.
- `/api/v1/ratelimit/stats` returned every key it had ever seen and listed
  daily token totals as request counts. It returns the top twenty.
- **A rule in the shipped policy could never fire.** `rules.code_leakage`
  named findings that only the scan of a response produces, which
  `output_rules` decides. It is removed with a note; the operations guide's
  sample had a `pii_critical` rule with the same fault, so a copied sample
  did not stop national ids or cards. A test now fails on a shipped or
  documented policy with a rule that cannot fire.
- The binary reported `v0.1.0` whatever it was built from. The version is set
  at build time (`VERSION`, `REVISION` build arguments); a build without one
  reports `dev` and the commit.
- `TAMGA_CLERK_CALLBACK_URL` is no longer read; nothing used it.
- **A stored secret finding kept the first eight characters of its match.**
  For a labelled secret that was the label, but for a connection string's
  password, a bare key or a secret after a short label it was eight
  characters of the value, in the request log, the console and the exports.
  A secret finding now keeps the vendor's marker when the kind has one
  (`sk-ant-****`, `AKIA****`, `ghp_****`) and `****` otherwise. A second
  secret of the same kind that only the normalised text reveals is no longer
  taken for a copy of the first. **Rows already stored are not rewritten:
  they keep the eight characters until retention deletes them.** **With
  `TAMGA_SCANNER_SERVICE_ADDR` set, upgrade scanner-service with the proxy**:
  against an older one, a request with a secret under a REDACT rule is
  blocked instead of masked.
- **A refused management credential left no trace.** A wrong or revoked key,
  a bad or expired session token, a revoked session, an account without a
  role, a proxy key on the management API and a role without the permission
  now write an `auth.refused` audit entry: the route, the reason, the source
  address, and never the credential or a part of it. The first refusal of a
  source for a reason is written; the others within the minute are counted
  in one `auth.refused.repeated` entry, and 32 sources are followed at a
  time, so refusals cannot fill the log. A request with no credential at all
  is not recorded. **The audit log gains these two kinds of entry.** Both
  are described, with their detail fields, the seven reasons and what is
  never recorded, in `proxy/docs/openapi.yaml` (`AuditEntry`) and in
  `docs/operations.md` ("Refused requests in the audit log"). The console's
  audit page shows the reason, the source and the count in words above the
  recorded detail.
- **The id of an API key was the first 12 characters of the key.** The id is
  the actor of audit entries (`apikey:<id>`) and `key_id` in the proxy's log
  and traces, so a quarter of every key (48 of its 192 random bits) was
  written where no part of a credential belongs. A new key's id
  is `key_` and 16 random hex characters, drawn apart from the key. A
  presented key was already found by its SHA-256 hash and never by its id;
  the stored hash is now also compared in constant time. The list still
  returns the first 8 characters of a key (`tk_` and five hex characters,
  20 bits), so that a key in hand can be matched to its row; that prefix
  goes nowhere but the key list. **Keys that exist keep working and keep
  their ids, which are still the start of the key: revoke such a key and
  create another to get an id that says nothing about it. An id that does
  not start with `key_` is one of these.** **The API keys page has an ID
  column**, the id the audit log names a key by. No change to the database.
- **A system-prompt leak finding carried the canary token.** The
  `output_scanned` event of a leak was handed to every subscriber of the
  event stream (NATS, the live feed, alerts, the analyzer) with the whole
  token as its match, and the request's event held its first two and last
  two characters. The token is a secret of the
  deployment: whoever knows it can have a model leave it out. The finding's
  match is now the fixed text `[canary token]` on every path. **Events
  already stored are not rewritten**; the token changes when
  `TAMGA_VAULT_KEY` does, or at a restart where that is not set.
- **The rate limiter kept a request count for every key it had ever seen**,
  one per client address for callers without a key, and every past day's
  token totals, until the process ended. A count is dropped after 24 hours
  without a request and a daily total when its day is over, and the limiter
  holds at most 50,000 keys of each kind, dropping the ones used longest
  ago. **`top_keys` of `/api/v1/ratelimit/stats` counts a key from its
  first request after such a drop, not from the start of the process.**
- **A failing scanner's error text was written to the process log**, at
  warn level, on every path: the proxy, `POST /api/v1/scan` and the LiteLLM
  route. A scanner, or the service that runs the scanners, the inline
  classifier or the analyzer, can quote what it was sent in the error it
  returns, and what it was sent is the request. The log now names the
  scanner, the reason and the kind of error (`timeout`, `cancelled`,
  `panic`, `grpc_unavailable`, or a Go type such as `*net.OpError`) and
  never the text; a panic's value is kept out the same way. The error the
  pipeline hands on carries the cause in its chain and not in its message,
  so `POST /api/v1/policies/simulate`, which answers with that message, no
  longer repeats it either, and a scanner's trace span records the kind in
  `error.type` instead of the message. **A log search or an alert on the
  `error` field of `scanner did not report`, `scanner error`, `gRPC scanner
  failed`, `classifier unavailable` or the analyzer's `deep scan failed`
  lines finds nothing now: the field is `error_kind`.**
- **The hash that `data.hash_findings` stores could be undone.** It was a
  plain SHA-256 of the finding's match. The match is what the scanner lets
  out, for PII the masked value (`10*******46`), so the hash was of that and
  not of the whole identifier; but a masked value has few possibilities, and
  whoever could read the table could recover it by hashing them. The option
  therefore kept nothing out of the table that is not there with it off.
  The hash is now HMAC-SHA-256 under a key of the deployment, stored as
  `hmac-sha256:…`: equal values still give equal hashes, and without the
  key a hash can be neither computed nor enumerated. The key is
  `TAMGA_FINDING_HASH_KEY` (base64, 32 bytes or more); when it is not set
  one is derived from `TAMGA_VAULT_KEY` with HKDF, so the vault's key is not
  used for a second purpose. **With `data.hash_findings` on and neither
  variable set, matches are no longer stored at all: the row holds
  `[not stored: no finding hash key]` in their place, and the proxy says so
  at startup and at error level when it first happens. The Compose file
  does not pass `TAMGA_VAULT_KEY` to the proxy, so a Compose deployment
  with `data.hash_findings` on has to set `TAMGA_FINDING_HASH_KEY`.** The
  key has to be the same on every replica and after a restart; rows hashed
  under another key are no longer found by a national id.
  **Rows written before this version keep the old, reversible `sha256:`
  hash until retention deletes them (`TAMGA_RETENTION_REQUEST_LOGS_DAYS`).
  They are not rewritten. An operator who wants them gone sooner deletes
  the rows older than the upgrade, or removes only their matches with the
  statement in [docs/operations.md](docs/operations.md#hashed-findings).**
  No migration: the hash is inside the findings, and its prefix tells old
  from new.
- **The subject routes take a national id, in the body.** `POST` and
  `DELETE /api/v1/events/subject` accept `tckn`, the id as the person gave
  it; the server computes the forms the log may hold it in (the masked
  match, its keyed hash, the unkeyed hash of older rows) and looks for
  exactly those in the findings of the request and of the response. The id
  is checked (11 digits, valid checksum), read from the body only (`GET`
  with `tckn` in the URL is 400), and is not logged, not written to the
  audit log, whose entry says `selector: tckn`, and not repeated in the
  answer. **`tckn_hash` was documented as "SHA-256 of the TCKN" and never
  found a row that way: what was stored was the hash of the masked match.
  It stays, as the 64 hex characters of a stored hash copied from a row;
  a caller that computed a hash itself sends `tckn` now.** The SDKs take
  `tckn` and send it by `POST`. **The log never held more than the masked
  form, so a lookup by national id also returns, and an erase also deletes,
  requests that mention another id with the same first and last two
  digits. Read before erasing.**
- **`data.hash_findings` hashed the database row and nothing else.** The
  same finding went, with its masked match (`10*******46`), to the events
  the proxy holds in memory (what the console's event list, the event
  detail and `GET /api/v1/events/export` read when there is no database, or
  when it cannot be reached), to the live feed (`/api/v1/live/events`), to
  the NATS envelope, which the stream keeps on disk for seven days, and,
  from the operator-state scanner, to the analyzer. The match is now hashed
  once, when the event is published and before any of them has it, so they
  all hold what the row holds. Masking and stripping a request are done
  before that, from the positions in the text, and are unchanged. Alerts,
  webhook payloads and SIEM lines never carried a match and still do not.
  **With the option on, the live feed, and the event list, detail and
  export when they are read from memory, show `hmac-sha256:…` where they
  showed the masked value, and a search of those events by a masked value
  finds nothing, as was already so for stored rows. Without a hash key
  they show `[not stored: no finding hash key]`. A consumer of the NATS
  stream gets the hash in `findings[].match`, and `findings_hashed: true`
  in the payload; envelopes already in the stream are not rewritten and
  age out in seven days.** The error-level line that says there is no key
  is written once per process, not once per writer.
- **With `data.hash_findings` on, a finding's `metadata` still described
  the value.** A card finding carried `bin` (the first six or eight digits
  of the number), `bank_name` and `country` beside the hash of its match; a
  national id on the list of known test numbers carried `denylist_reason`,
  which is different for every number and for one names its holder; an
  operator-state finding repeated its match in `decision_id` or `note_id`
  and in the free text of `message` and `detail`. With the option on those
  keys are dropped, `operator_id` is kept as a keyed hash (dropped without
  a key), and the keys that describe the detection stay (`detected_via`,
  `denylist_match`, `card_brand`, `card_type`, the counts and states: the
  list is in [docs/operations.md](docs/operations.md#hashed-findings)). A
  key that is not on that list is dropped, so a key a scanner adds later
  is not kept until it is listed. **With the option on, `bin`,
  `bank_name`, `country`, `denylist_reason`, `decision_id`, `note_id`,
  `message` and `detail` are gone from stored findings, exports and the
  live feed; with it off nothing changes. Rows already stored keep their
  metadata until retention deletes them.**
- **`tckn_hash` searched only the request's findings, as a substring.** A
  hash copied from a finding of the answer found nothing, and an erase by
  hash left those rows. It now finds the rows with a finding, of the
  request or of the response, whose match is exactly that hash, under the
  keyed scheme or the old one, as `tckn` does. A value that is not 64 hex
  characters is still refused with 400 before any query, and the store
  refuses it again. **A lookup or an erase by `tckn_hash` can return more
  rows than before (those where the value was in the answer), and no longer
  returns a row that only held those 64 characters somewhere else in its
  findings.**
- **A scanner that panicked in `workerpool` mode ended the process.** A job
  runs on a pool worker, which is not a request's goroutine, and nothing
  recovered there; in the other modes the pipeline does. The job is now
  answered with a failure of kind `panic`, counted and logged as one
  (`reason: panic`, never the panic's value), the scan is reported as
  degraded with what the other scanners found, and the worker stays.
  **A scan whose context was already over (a client that went away, a
  timeout) could hang its request for good in the same mode**: the pool
  refused such a job at random and the pipeline went on waiting for its
  result. Every job the pool accepts now gets exactly one result, and a job
  whose context is over is answered with that error without running the
  scanner.
- **A request whose scan jobs were still queued when the scanner pool shut
  down waited for good** (`workerpool` mode). Shutdown stopped the workers
  and left the queue as it was, and each queued job had a request waiting on
  its result. Shutdown now answers every queued job with an error of kind
  `pool_closed`, without running its scanner, and does so before it waits
  for the workers, so the queue is answered even when a worker outlasts the
  timeout. The pool refuses jobs from then on, and the pipeline takes any
  refusal for what it is: it used to count every error but "queue full" as
  an accepted job and wait for a result that was never sent. Such a scan
  comes back degraded (`reason: error`), and `scan.on_error` decides about
  the request as for any other scanner failure. A second call to Shutdown
  returns instead of panicking. **A panic in `workerpool` mode is now
  counted once in `tamga_scan_degraded_total`, under `panic`**; the last
  sentence of the entry above no longer holds, and an alert on
  `reason="error"` will see fewer events for the same failures.

### LiteLLM
- **Tamga can be added to a LiteLLM proxy as a guardrail**, without Tamga's
  proxy in the traffic path. `POST /api/v1/beta/litellm_basic_guardrail_api`
  speaks LiteLLM's Generic Guardrail API: a `generic_guardrail_api` guardrail
  with `api_base: https://<tamga>/api/v1` and a Tamga key as `api_key` is the
  whole setup. It is an adapter over the scan path of `POST /api/v1/scan`,
  not a second implementation. BLOCK is returned as `BLOCKED` with a reason
  that names the kind of finding and never the value; REDACT and STRIP as
  `GUARDRAIL_INTERVENED` with the texts masked by the proxy's own markers;
  WARN, LOG and PASS as `NONE`. Answers (`post_call`) are judged by
  `output_rules`. A scan that did not finish is a 503
  (`tamga_scan_unavailable`), never a verdict. Nothing is put in the vault,
  so nothing masked over the bridge is restored.
  Written against LiteLLM v1.104.2. **Tested against a fake caller that
  speaks the contract; the end-to-end test with a real LiteLLM container
  (`deploy/e2e/litellm/`, CI job `litellm-e2e`) is written and has not been
  run yet.** See [docs/litellm.md](docs/litellm.md).
- A Tamga key is also read from `x-api-key` on that route, because that is
  where LiteLLM sends it. Nowhere else: the header is no credential on any
  other route, and the admin key is not read from it.
- **The bridge read only `texts`: a poisoned tool description, or a
  national id in a tool call's arguments, passed unread.** It now scans a
  request's `tools` in the role `tool_definition` and the arguments of
  `tool_calls` in the role `assistant`, as the proxy reads the same request,
  and an answer's tool calls under `output_rules`. A value masked or
  stripped in a tool definition goes back to LiteLLM in `tools`. The
  contract has nothing to return tool calls in, so **a value the policy
  would mask or strip in a tool call's arguments blocks the call**
  (`REDACT could not be carried out (tool_call)`), as does one in a tool's
  name. **Tools and tool calls now count toward the 256 KiB that is scanned
  per call, with at most 256 of each, so a request with very large tool
  definitions that passed before can be a 413**, and the message of that
  413 changed. Images stay unread, as on the proxy path.
- **Bridge calls are recorded.** A call was one log line and a count, so a
  team using Tamga only as a LiteLLM guardrail saw an empty console. Each
  request call is now a row in the request log and the live feed, with the
  provider `litellm` (shown as "LiteLLM bridge", and not counted as a
  shadow provider), the route and direction as its endpoint, the verdict,
  the findings with their masked matches and their place (`texts[1]`,
  `tools[0].function.description`), the key's organisation and the model
  LiteLLM named; an answer call is a row when something was found or done.
  **Blocks, redactions and warnings over the bridge now fire the alert
  destinations and the rules' `notify` URLs, and bridge calls count in the
  console's totals and in `tamga_requests_total` and
  `tamga_blocked_total`.**
  No text is stored, and no tokens or cost, which Tamga does not see here.
  `litellm_call_id` stays in the log line: the request log has no column
  for it. `POST /api/v1/scan` is unchanged and stores nothing. No migration.
- **A long answer over the bridge was a 503.** Each text of an answer was
  given `output_rules.scan_window_ms` to be scanned in, 200 ms by default,
  and one the scanners needed longer for (an answer near the route's size
  limit can) was answered with
  `tamga_scan_unavailable`, which LiteLLM turns into a refused request. That
  window is the proxy path's: it is how long a buffered answer is held back
  from a waiting client, and `output_rules.fail_open` says what happens when
  it runs out. Over the bridge LiteLLM already has the answer and waits for
  the verdict, and there is no `fail_open`. An answer call now has one
  budget of 10 seconds for all its texts and tool calls together, several
  times what the 256 KiB the route accepts takes to scan; a scan that
  reaches it is still a 503, never a verdict. **`scan_window_ms` no longer
  shortens the scan of an answer over the bridge; set above 10 seconds it
  is the bound. A `post_call` guardrail can therefore take longer to answer
  than 200 ms a text.** The proxy path is unchanged.

### Licences and packaging
- `NOTICE` gives the attributions the bundled material asks for, and
  `THIRD_PARTY_LICENSES.md` lists dependency licences as read from disk, with
  what could not be inspected said as such. The table of where
  prompt-injection phrases came from named licences nobody had checked: the
  OWASP material is CC BY-SA 4.0, not CC0, and three sources could not be
  verified at all and now say so.
- Both SDK packages carry the licence text. Container images are labelled
  with their licence and source.
- `SECURITY.md` and the disclosure document describe the project as it is:
  one maintainer, no service, fixes on `main`. **Private vulnerability
  reporting on GitHub is named as the first route and has to be enabled in
  the repository's settings.**
- Helm: the disruption budget is rendered only when it cannot block a node
  drain; `analyzer.grpcAddr` and `analyzer.httpUrl` set where the analyzer
  is. Terraform: the cluster version is a required variable, zones come from
  the region, and provider versions are locked.

### Console
- A page no longer shows old answers as current when the proxy stops
  answering: figures from the last answer are marked with its time or not
  shown, and "Needs an admin key" appears only when the proxy answered and
  withheld the detail.
- Text that wraps is no longer set solid; a selection in the policy editor is
  visible; toasts carry no icon; a report that could not be made says why
  next to its button; figures are not drawn as 0 while they load.
- Marked text was wrong in three cases: a literal pattern tried on a sample
  with a letter such as "İ" before the match marked the wrong characters; a
  playground prompt that was one match from end to end was not shown; a diff
  against an empty policy counted one removed line. A request list read anew
  no longer marks rows as new. The charts, the diff, new-row marking and
  match marking have unit tests; the mocked end-to-end tests written for the
  old interface are replaced by `e2e/states.spec.ts`; `npm run lint` covers
  `hooks/`.
- A long name with no place to break (a model, a key label, a provider, a
  version) no longer makes a page scroll sideways, nor does a table with a
  header written for screen readers; on a narrow screen the filters and form
  rows wrap. `e2e/layout.spec.ts` checks every page at 1280 and 375 px with
  such values. Changing a filter, the window or the page no longer marks rows
  as new. Two policies that differ only in line endings are said to be the
  same apart from line endings, instead of drawn as a diff with no change.
- The Redis and NATS URLs were written to the log at startup with their
  passwords. Only the host is logged.
- **Helm chart 0.2.0.** The chart set variables the proxy does not read, so
  an install had no database, and shipped a default policy the proxy refuses
  to start with. It now takes the admin key, database URL and Redis URL from
  a Secret (`secrets.adminKey`, `secrets.databaseUrl`, `secrets.redisUrl`,
  or `secrets.existingSecret`) and fails to render without an admin key;
  the port is 8443. **Values files written for the old chart must be
  changed: `env.TAMGA_ADMIN_KEY`, `env.TAMGA_POSTGRES_URL` and
  `env.TAMGA_REDIS_URL` are gone.**
- The proxy image's health check calls `/health`. It called the detailed
  endpoint and marked the container unhealthy when a dependency was down.
- **SDKs, breaking.** Python: `get_mttr` and `get_budget_stats` no longer
  take an organisation. TypeScript: `exportEvents` returns text,
  `openLiveEvents` is an async generator, and `updateWebhook`, `getSession`
  and `exportEventsUrl` are removed; none of the three reached anything.
- The Team list says whether an account's role was given by an admin
  (`assigned`) or is only the default for accounts without one. An account
  whose role was removed stays listed for 30 days, marked `role_removed`,
  with the moment its sessions ended, so it can be seen and given a role
  again. `DELETE /api/v1/team/{id}/role` returns that moment.
- The console no longer carries Clerk: the sign-in widgets, the user menu,
  the `@clerk/nextjs` dependency and the dashboard's Clerk variables in the
  compose file are gone. Nothing signed in through them.
- **A session could not be ended.** A token was good for 24 hours whatever
  happened to its owner. Changing or removing a role, `POST
  /api/v1/team/{id}/sessions/revoke` and `POST /api/v1/auth/logout` now end
  a user's sessions: at once on the replica that served the call, within 30
  seconds on the others.
- Session tokens carry an issuer and an audience and are checked for their
  algorithm and issue time; a token whose header said `alg: none` over a
  valid signature was accepted. **Every session token issued before this
  release is refused: users sign in again.**
- **An account nobody gave a role has no access**, at sign-in and on every
  request. Through the Clerk callbacks it became a viewer. To keep that, set
  `TAMGA_SSO_DEFAULT_ROLE=viewer`; it then applies to GitHub sign-in as well.
- The Clerk SAML and OIDC callbacks could not complete a sign-in and let the
  caller supply both halves of the state check. They answer 501, and the
  code behind them is removed.
- **A literal webhook URL in a rule's `notify` was shown to every role.** It
  is masked for everyone below admin in the policy, its history, revisions
  and proposals, and a policy that holds the mask cannot be saved.
- Two demotions at the same moment could remove the last admin. An incident's
  triage, resolve or reopen could go unrecorded in the audit log when the
  read that followed failed.
- `POST /api/v1/events/subject` takes the person's selector in the body; the
  GET form, which puts it in the URL, still works. `DELETE
  /api/v1/team/{id}/role` removes a role; nothing could before.
- `POST /api/v1/policies/simulate` gave the scan two seconds, less than the
  largest sample it accepts needs on a busy machine, and then answered 503.
  It gives five, the proxy's own default for a scan.
- A key in the query string is no longer read anywhere. The live stream was
  the last route that took `?key=`; the console sends the header there too.
  **A client that opened `/api/v1/live/events?key=…` must send
  `X-Tamga-Admin-Key`.**
- The live stream sends a `: ping` comment every 15 seconds while it is
  quiet. The console reopens a stream that has gone silent for 45 seconds;
  before, a connection whose other end had gone looked like a quiet proxy.
- **The dashboard image ignored `NEXT_PUBLIC_API_URL`.** Compose passed it
  to the running container, where it does nothing: the value is compiled
  into the page at build time, so the console always called
  `http://localhost:8443`. It is now a build argument.
- **The console's live feed asked for no credential.** A route of the
  dashboard opened the proxy's event stream with a key held on the server, so
  anyone who could reach the console saw scanned and stopped requests with
  their findings. The route is gone: the browser reads the stream from the
  proxy with the key the person entered, in a header.
- **A dashboard built with `NEXT_PUBLIC_ADMIN_KEY` handed the admin key to
  every visitor**, and the image build copied `.env.local` into the builder.
  A production build now stops when the variable is set, the code that reads
  it is dead in production, and the image no longer takes `.env` files. **A
  production console starts with no key; each person enters one under
  Settings.** `next dev` keeps the built-in key and says so.
- **The management API allowed whatever it had not thought about.** A route
  with no declared permission was open to every authenticated caller, and
  the check that a read-only caller cannot write ran for API keys only. A
  signed-in viewer could run retention, reset a circuit, change incidents
  and saved hunts, and call the SCIM routes. Every route now declares the
  permission it needs, one check applies it to every kind of credential, and
  a route that declares nothing is refused to everyone. A test fails when a
  route is added without a declaration.
  **What existing callers lose:** viewers (read keys, viewer users) lose the
  event export, the audit log, the API key list, the webhook list and a
  person's data; analysts (write keys) lose policy reload, retention,
  circuit reset, the API key list, the webhook list and a person's data. See
  "Who may call what" in the operations guide.
- **Any GitHub account could sign in as a viewer.** An account with no role
  became a viewer and could read events, the policy and, until this release,
  alert destinations with their tokens. A GitHub account now needs a role
  before it can sign in; give the first with the admin key.
- `GET /api/v1/webhooks` returned each destination's token, headers and full
  URL to any authenticated caller. It is admin-only and returns none of them.
- A key in the query string was accepted on every route. `?key=` now works
  only on `GET /api/v1/live/events`. **Scripts and links that passed the key
  in the URL must send the header.**
- A user's role was fixed in the session token for 24 hours. It is read on
  every request. Nobody can change their own role, and the last admin cannot
  be demoted.
- `X-Tamga-User-Id` sent without a credential told a caller whether an id
  existed and whether it was an admin. It is no longer read.
- Saved hunts, `/mttr` and `/budget/stats` took the organisation from the
  caller; `/mttr` without one covered every organisation. They use the
  install's organisation.
- The admin key is compared in constant time.
- Audit entries for team roles, API keys, alert destinations, patterns and
  incidents name who did it; retention runs, circuit resets and saved-hunt
  changes are now recorded.
- **Who changed the policy was whatever the caller said.** `X-Tamga-Actor`
  and `X-Tamga-User-Id` set the author of a proposal, its approver and the
  actor in the audit log, so the author of a proposal could approve it under
  another name. The actor now comes from the credential: `admin-key`,
  `apikey:<id>` or `user:<id>`. Older records keep the names they had.
- **`require_dual_control` could be walked around with `PUT /policies`.**
  With it on, a direct write, a custom-entity change and a rollback now
  answer 409 and the change has to be approved by someone other than its
  author. A proposal cannot be approved twice or after the policy moved on.
  **A deployment with only the admin key cannot approve anything under dual
  control:** give each person a key. See "Who may change the policy" in the
  operations guide for what dual control does not cover.
- **A `write`-scope key could change any part of the policy.** Providers,
  exceptions, governance, the vault, the cache, limits, cost, the scan
  settings, the canary and where a rule sends its alerts now need the admin
  role; a write that touches them with an analyst key is refused with 403
  and the sections named. **A CI job that pushes the whole policy with a
  `write` key needs an admin key, or to leave those sections as they are.**
  Analysts can open, approve and reject proposals within their sections.
- Every policy change through the API, a reload or the file is in the audit
  log with the actor and the paths that changed, and in the policy history.
- **Text could be sent masked in part.** With `scan.on_error: pass`, a
  request with more than 1000 matches in one piece of text, or whose scan ran
  out of time, was forwarded with the rest unmasked. A request or response
  that was to be masked on an unfinished scan is now refused with 503
  `tamga_redaction_incomplete`, whatever `on_error` says. This includes a
  scanner that fails outright: a deployment on `on_error: pass` with a
  scanner that always fails gets 503 for every request that needed masking.
- **A response whose scan did not finish was sent anyway.** It is now
  withheld with 503 unless `output_rules.fail_open` is set, as a stream
  already was.
- **A cached answer outlived the rules it was scanned under.** The cache key
  now carries the policy's content, the compiled patterns and the scanner
  version: after a change every request misses once. All entries miss once
  on upgrade.
- The built-in scanners stop at the request's deadline instead of running to
  the end (1 MiB took over four seconds whatever the deadline). **With
  `on_error: block`, a large request that used to finish late now gets a 503
  at the deadline; the same for a response against
  `output_rules.scan_window_ms`.**
- The scanner service answered an unfinished scan with an empty result,
  which the proxy read as clean. It now returns an error.
- A stored pattern created on one replica was not compiled on the others
  until a restart or a policy reload. It is now picked up within 30 seconds.
- `PUT /policies` and `POST /policies/validate` answer a policy over a limit
  with 422 and one entry per field, not a single-message 400.
- **A compressed response passed the output scan unread.** The caller's
  `Accept-Encoding` was forwarded and the answer was not decoded, so
  `output_rules` scanned gzip bytes and found nothing in them. Most HTTP
  clients ask for gzip. The proxy now requests compression itself and reads
  the answer decoded; callers receive it uncompressed.
- **The response cache answered callers the provider had not checked.** The
  key held the organisation, provider, model and body, not the caller: a
  second caller with a wrong or revoked provider key, or none, got the first
  one's answer. An answer is now served only to a caller with the same
  provider credential, or the same Tamga key when no credential is sent.
  Expect a lower hit rate where callers with different keys shared answers.
  `cache.ttl_seconds` is capped at one hour.
- **The health endpoints told anyone the layout of the deployment.**
  `/api/v1/health/detailed` and `/health/detail` returned the policy path,
  pool and endpoint names, raw dial errors with addresses, the IP allowlist
  and the version to callers with no credential. Without a management key
  they now say only whether the proxy is up, with scan latency. **Scripts
  that read `database`, `redis`, `scanner_count` or `providers` must send
  `X-Tamga-Admin-Key`.** Status codes are unchanged. `failure_reason` is a
  fixed class, not the transport's error text.
- **Patterns had no size, count or time limit**, and one pattern could hold a
  request for over a minute. Custom, competitor and operator-state patterns
  are now bounded in length, compiled size and number, may not match the
  empty string, and their scans stop at the request's deadline. A policy
  over a limit does not load; see "Limits on patterns and settings" in the
  operations guide. **A custom entity name with a dash, a space or a
  non-ASCII letter no longer loads.**
- **A rule with `action: block` in lower case let everything through.** The
  action is compared as written; an unknown one now fails the load instead
  of acting as PASS. The same for `override_action`.
- Competitor matches are masked in events, the request log and simulate,
  as custom entity matches already were.
- **A policy could send the proxy's secrets, and a route's traffic, to any
  host.** `providers.pools` let the policy choose an upstream URL and name
  any environment variable as its key; the proxy sent that variable's value
  as a bearer token to that host, with the request and the caller's own
  credentials. The policy can be changed through the API by the analyst role
  and by write-scope keys. A pool endpoint must now be the route's own vendor
  host, the route's `TAMGA_<PROVIDER>_URL`, or a host listed in
  `TAMGA_UPSTREAM_ALLOW`, and `api_key_env` must be a `*_API_KEY` name, never
  a `TAMGA_` one. The rules hold on every way a policy is installed and again
  on every request. **If you run pools against a gateway, a private endpoint
  or a self-hosted model, list the host in `TAMGA_UPSTREAM_ALLOW` before
  upgrading, or the policy will not load and the proxy will not start.** A
  `${NAME}` in a rule's `notify:` is limited the same way
  (`*_WEBHOOK_URL`). Rollback and proposal approval now check a policy
  before writing it; they used to write first and could leave a file the
  proxy would not start with.
- **Streamed responses through a provider pool were cut off** once the
  response headers arrived. A pool endpoint's `timeout` now bounds the wait
  for the answer to begin, not the whole answer.
- **Alert destinations could point the proxy at internal addresses.** A saved
  destination or a policy rule's `notify:` URL was posted to wherever it
  pointed, redirects included, so whoever could save one could make the proxy
  send requests to loopback, the private ranges or the cloud metadata
  address, and the Test button reported the answer. Every alert request now
  goes through one guard that judges the address at connect time. **If you
  alert to a receiver inside your network (Splunk, QRadar, a sibling
  container), list it in `TAMGA_ALERT_ALLOW_PRIVATE`** or its alerts stop and
  are counted as `blocked`. Plain `http` destinations are refused unless the
  host is on that list. Alerts no longer use `HTTP_PROXY`. A redirect to
  another host is not followed. The Test endpoint returns a fixed error class
  in place of the transport's error text.
- **Default policy did not enforce prompt injection.** On `main` since the
  vault/canary merge (2026-08-02), the `injection`, `code_leakage` and
  `operator_state` rules in `proxy/tamga-policy.yaml` were nested under
  `canary:` and silently dropped; injection findings evaluated to PASS. The
  file is fixed, policy YAML is now parsed strictly (an unknown or misplaced key
  is a load error), and startup/reload/validate warn when no rule acts on PII,
  secret or injection findings. If you run a copy of the default policy from
  that period, re-check its indentation.
- **Cross-provider failover forwarded caller credentials.** When the addressed
  provider returned 429/5xx, the request — including `Authorization` /
  `x-api-key` — was retried against the other of OpenAI/Anthropic. Retries now
  stay on the addressed provider; use `providers.pools` for same-vendor
  failover.
- **Policy exceptions trusted a caller-supplied role.** `X-Tamga-Role` is now
  ignored unless `TAMGA_TRUST_ROLE_HEADER=true` (for deployments behind an
  authenticating gateway). Caller-supplied `X-Tamga-*` headers are no longer
  forwarded to the provider.
- **`X-Forwarded-For` was trusted from anyone.** The first entry of the header
  was taken as the client address, so a caller could pass `TAMGA_IP_ALLOWLIST`
  by writing an allowed address into it, and a caller without a key could
  reset its rate limit on every request. The header is now read only when the
  connection comes from `TAMGA_TRUSTED_PROXIES`, and from the right. **If
  Tamga runs behind a load balancer, set `TAMGA_TRUSTED_PROXIES` to its
  address range**; without it the allowlist and the limiter see the load
  balancer's address.
- **Rate-limit buckets were named after the provider key.** The caller's
  `Authorization` / `X-API-Key` value was used as the bucket name, stored in
  Redis and returned by `GET /api/v1/ratelimit/stats` under `top_keys`. The
  name is now a truncated SHA-256 of the key.
- **Blocked responses could be served from cache.** The response cache was
  written before the output scan; it is now written after, and only for
  responses with no output findings.
- **Large responses were truncated.** With output scanning on, a non-stream
  response above `output_rules.buffer_bytes` was cut at the limit. It is now
  forwarded whole with `X-Tamga-Output-Scan: skipped-too-large`; the default
  limit is raised from 256 KB to 1 MB.

### Console data
- **The request log did not hold what a request cost or what its response
  scan found.** Tokens were always 0 and the cost was never written, so the
  cost breakdown was empty. A stored request now carries its tokens and cost
  when the proxy read the response, the cache status, the findings, action
  and risk of the response scan, and the name and version of the policy that
  decided it. What is not known is left empty and is absent from the API, not
  0. **Tokens and cost are only read when `output_rules` is on.**
- Scan and total time are stored to a fraction of a millisecond; a scan under
  one millisecond used to store 0, which emptied the Latency page.
- With NATS on, output hints and provider state changes were stored as
  requests, a redelivered event added a second row, and
  `data.hash_findings` was ignored. All three are fixed.
- A request's detail is found through a new index on the request id
  (migration 017). **Start the new proxy before applying the migration; the
  index build holds back inserts on a large table.**
- **A request from a caller without a key was not recorded when its
  `X-Tamga-Org-Id` was not a UUID**, so any caller could keep its requests,
  stopped ones included, out of the record. It is now stored under the
  default organisation. A key whose `org_id` is not a UUID still produces no
  rows; they are counted in `tamga_request_log_unrecorded_total`.
- SSO settings are kept in PostgreSQL and reach other replicas within 30
  seconds; they were lost on restart. **Changing them now needs the admin
  role** (any signed-in caller could) and is written to the audit log.
- Migration 018 removes the row-level security policy on `request_logs` and
  `daily_stats`. It compared against a setting nothing set and did not apply
  to the role the proxy connects as, so it protected nothing.
- **After a restart the console showed no traffic and older requests would
  not open**, although the requests were in PostgreSQL. The summary, the
  charts, the findings breakdown, model shares, a request's detail and the
  CSV export read only the last 1000 requests in the proxy's memory. They now
  read the stored request logs when there is a database: the same after a
  restart and on every replica, a few seconds behind the live feed.
- `GET /api/v1/stats` honours `?range` for every field. The counts were
  process-lifetime counters whatever the range.
- `GET /api/v1/events/export` takes the event list's filters and exports up
  to 50,000 rows instead of 1000, marking a cut with
  `X-Tamga-Export-Truncated`.
- New response header `X-Tamga-Data-Source: database|memory` on these
  endpoints.
- A request log written through the in-process bus is dated by the request,
  not by the moment its batch was flushed.
- `daily_stats` is no longer read; nothing ever filled it. The earlier entry
  about the day-bucket timeseries reading it is superseded.
- **"Shadow only" gave different answers from the database and from
  memory.** The filter was written three times: a list of provider names in
  the SQL of the event search, another in the export, and a function for the
  events held in memory. They disagreed about a request that names no
  provider (shadow in the database, not in memory), about a provider
  recorded as `OpenAI` (shadow in memory, not in the database), and the
  export, answered from memory, counted a LiteLLM bridge call as shadow.
  There is one list now, in the `providers` package; the search is given it
  as an argument and the two filters over memory call the same function.
  **A request without a provider is not shadow AI**: shadow says traffic
  went to a provider outside the approved ones, and such a row says nothing
  about where anything went. **`?shadow=true` and `?provider=shadow` on a
  deployment with a database return fewer rows than before if it holds rows
  with an empty provider.**

### Agents
- **A long tool result or file no longer stalls the proxy.** Scanning a large
  piece of text did not finish: the secret scanner's separator normalisation
  copied the rest of the text at every character, so its cost grew with the
  square of the length, and 64 KB took minutes. It is linear now.
- **Long segments are scanned in parallel.** A segment over 32 KB is cut into
  overlapping chunks that are scanned on every core; what is found and where
  does not depend on the cuts. A megabyte of new text takes about a second
  on 16 cores (four seconds on one).
- **Repeated text is not scanned again.** The scanners that depend only on
  the text (PII, secrets, injection, jailbreak, content moderation) keep
  their findings for each piece of 2 KB or more for ten minutes. An agent
  resends its whole history every turn; only the new part is scanned.
  Custom patterns, competitor names and operator-state checks are never
  cached.
- The shipped policy accepts up to 32 MB on the Anthropic route (was 2 MB),
  the API's own limit.
- Tests pin what an agent needs from a gateway against a stand-in for the
  Anthropic API: `count_tokens` passes, `anthropic-*` headers are forwarded,
  a request and response with nothing to find pass byte for byte, and a
  stream is not held back.
- **New action `STRIP`.** Replaces the whole piece of text a finding is in
  with a placeholder and forwards the request, so an injection in a tool
  result does not end an agent's session. Ranks between `BLOCK` and
  `REDACT`. Other findings in the request are still redacted under their own
  rules; if the text cannot be replaced the request is blocked
  (`X-Tamga-Strip-Fallback`). The response carries `X-Tamga-Stripped-Count`.
- **Streamed responses are scanned, redacted and restored as they stream.**
  With `output_rules.streaming` on, the model's text is reassembled from the
  events and its last 64 characters are kept back until nothing in them can
  still be growing, so a value the provider cut across two events is found.
  `redact_on` masks it in place; `block_on` ends the stream. Before, each
  chunk of raw bytes was scanned on its own, a value split across events was
  missed, and a redaction ended the stream.
- **Vault and canary no longer hold a streamed response.** Both collected the
  whole stream before sending any of it. Placeholders are now restored and
  the canary token looked for as the text goes by.
- **The canary token is stable per organisation and key.** A new token on
  every request made every request body unique, which defeated the response
  cache and the provider's prompt cache.
- New: [docs/agents.md](docs/agents.md).

### Inline classifier (optional)
- **A model on the decision path.** The analyzer's semantic checks ran after
  the response and could not change a verdict. A new classifier service runs
  a local ONNX prompt-injection model, and the proxy asks it inline about a
  request its rules did not block. A score at or above the threshold becomes
  an `injection` finding of category `classifier`, with the role and path of
  the text, and goes through the policy like any other finding.
- Off by default. Needs the classifier service (`docker compose --profile
  classifier`), a model you download, `TAMGA_CLASSIFIER_ADDR`, and
  `scan.classifier.enabled: true`. No model ships with Tamga and none is
  named in the code. See docs/operations.md, "Inline classifier".
- `scan.classifier`: `timeout_ms` (150), `threshold` (0.98), `roles`
  (`user`, `tool`), `max_chars` (6000).
- A classifier that cannot answer is a failed scan: `X-Tamga-Scan-Degraded:
  classifier`, and `scan.on_error` decides between 503 and forwarding. A
  breaker stops calls for ten seconds after five failures in a row.
- Text seen in the last ten minutes is answered from memory, so a
  conversation pays for its new message only.
- New metrics: `tamga_classifier_calls_total`, `_errors_total`,
  `_cached_texts_total`, `_short_circuited_total`. New response header
  `X-Tamga-Classifier: ok | partial`.
- The classifier is a separate service, not part of the analyzer: it is on
  the request path, and the analyzer's pinned dependencies cannot read
  current model files.
- `cmd/redteam -classifier <addr> [-threshold N] [-sweep]` measures rules and
  classifier together and reports what the classifier added.
- Stress suite: `--classifier` runs it with the classifier on.
- Architecture diagrams corrected: the analyzer was drawn on the request
  path; it runs after the response.

### Keys
- **API keys are stored in PostgreSQL.** They were kept in memory and lost on
  every restart, and differed between replicas. With a database configured
  they now live in `virtual_keys` (migration 015; the proxy also creates the
  table at startup, so an existing database needs no manual step). Without a
  database the in-memory store remains and a warning is logged at startup.
- Only a SHA-256 hash and an 8-character prefix are stored; the value is
  returned once, at creation.
- Revoking a key marks it instead of deleting the row. It stops working at
  once on the replica that revoked it and within 30 seconds on the others.
- A key can carry `org_id`, `role`, `user_id` and an expiry
  (`expires_in_days`).
- New scope `proxy`: a key for an application on the proxy path. It is refused
  by the management API, so a leaked application key does not expose events.
- **Keys now authenticate the proxy path.** A request that sends a Tamga key
  in `X-Tamga-Key` takes its organisation and role from the key; the
  `X-Tamga-Org-Id` and `X-Tamga-Role` it sends are ignored. Budget, cache
  partition, events and policy exceptions all use that identity, and the
  request is rate-limited per key. An unknown, revoked or expired key gets
  401 `tamga_key_invalid`.
- `TAMGA_REQUIRE_KEY=true` refuses requests without a key (401
  `tamga_key_required`). Off by default: requests without a key behave as
  before.
- Dashboard: the keys page creates application keys with an organisation.

### Settings storage
- **Custom patterns, team roles and webhooks are stored in PostgreSQL.** They
  were kept in memory: lost on restart and different on every replica. With
  a database they are now written to `stored_documents` (migration 016, also
  created at startup) before a change takes effect, loaded at startup, and
  reloaded every 30 seconds so a change on one replica reaches the others.
- **Webhooks are stored encrypted** (AES-256-GCM, `TAMGA_VAULT_KEY`), because
  a webhook URL and its token are credentials. Without `TAMGA_VAULT_KEY`
  webhooks stay in memory and a warning is logged at startup. Keep the key:
  stored webhooks cannot be read with a different one.
- A change the database could not take is answered 503 and not applied.
  Previously every store error was a 400.

### Core Proxy (fixes)
- Scans that lose coverage — scanner panic or error, worker-pool shedding,
  oversized response — are no longer silent: counted in
  `tamga_scan_degraded_total{reason}`, logged, and flagged to the caller with
  `X-Tamga-Scan-Degraded: true`. The proxy still fails open in these cases.
- Provider allowlist names resolve aliases (`azure_openai` → `azure`,
  `google_vertex` → `gemini`); the default allowlist previously rejected the
  `/azure/` and `/gemini/` routes with 403.
- CI: Go tests and the adversarial gate now also run on pushes to `main`.
- **Fresh installs came up with only the first migration applied.** Migrations
  002 and 006 used `CREATE INDEX CONCURRENTLY` on the partitioned
  `request_logs` table, which PostgreSQL rejects; the database init aborted at
  002 and, after a restart, skipped 003–013. Existing deployments created this
  way are missing the audit log, retention log, pricing, outbox, RLS, incident
  lifecycle and saved-hunt tables — recreate the volume or apply the missing
  migrations by hand.
- **Retention never ran on a database that already had request logs.** The
  schema starts with only a default partition, so rows logged before a
  month's partition exists land there, and PostgreSQL refuses to create a
  partition over them. The maintenance cycle failed at its first step, before
  dropping or purging anything. Typical case: retention switched on after the
  proxy had been running. Partition creation now moves those rows into the
  new partition in the same transaction.
- **Retention never dropped expired `request_logs` partitions.** The partition
  bound was parsed as a bare date, but PostgreSQL renders it as a timestamp
  with offset, so every partition was skipped.
- **Delegating scanning to scanner-service switched off three scanners.**
  Custom entities, competitors and operator_state need the proxy's policy and
  the request, which the remote service does not have. They now keep running
  in-process and their findings are merged. `TAMGA_SCANNER_SERVICE_ADDR` alone
  now enables delegation (it previously also required a worker pool size), and
  the compose file makes it opt-in.
- Docker Compose: the Quick Start now passes `--env-file .env`; Compose does
  not read the repo-root `.env` on its own, which left the database password
  empty. `TAMGA_MOCK_UPSTREAM` is now passed through to the proxy container.
- **Model prices come from one place and are no longer guessed.** Three
  separate price lists and three matching rules are replaced by
  `internal/pricing`. A model is priced only by a row that names it; dates and
  revision numbers aside, `gpt-4o-mini` is no longer priced as `gpt-4o`, nor
  `claude-opus-4-7` as `claude-opus-4`. Behaviour changes:
  - A model without a price is reported as such: `priced: false` and an
    `unpriced` list in `GET /api/v1/billing/costs/breakdown`, no
    `X-Tamga-Cost-USD` header, and one warning in the log per model. Its tokens
    still count against the token budget; the cost budget cannot see it.
  - Rows stored under provider `google` now price the `/gemini/` route.
  - Models on the `/local/` route are always priced at zero.
  - Per-model totals in the cost breakdown summed only the first day of the
    range; they now cover the whole range.
  - `GET /api/v1/providers` lists the models seen in recent traffic on each
    route instead of a fixed list.
  - The built-in list now covers the current OpenAI, Anthropic and Gemini
    models, checked against the providers' price pages on 2026-10-03. It
    carries base rates only: prompts above a provider's long-context
    threshold are under-estimated.
  - Model families are derived from the name for models the proxy does not
    know (`gpt-5-mini` → `gpt-5`, `qwen3:32b` → `qwen3`).

### Request parsing
- **Requests are scanned by message, not as JSON bytes.** For OpenAI (chat,
  responses, legacy completions), Anthropic and Gemini bodies the scanners now
  read the decoded text of each message, tool call, tool result, document and
  tool description, with the role it speaks for. This closes three evasions
  that relied on the JSON encoding: text written in `\u` escapes, an ID number
  after an escaped line break, and an instruction split by a line break or
  over two content parts. Other bodies are scanned as bytes, as before;
  `X-Tamga-Scan-Mode` (`segments` or `raw`) says which, and
  `tamga_scan_mode_total` counts both.
  - **One value, one finding.** A value used to be reported once per view of
    the text it was found in: one e-mail address gave 3 findings, a message
    with four values gave 24. It now gives 1 and 4, so finding counts in
    events, metrics and `X-Tamga-Redacted-Count` drop accordingly.
  - Findings carry `role` and `path` (`messages[2].content[0].content`).
  - **Redaction keeps the body valid.** Masks and vault placeholders are
    written into the decoded text and the string is re-encoded; every other
    byte of the body is left as the client sent it. This also covers text
    inside tool call arguments (JSON in a string) and base64 text attachments.
    The result is extracted again and compared before it is sent.
  - **REDACT blocks when it cannot redact.** A value found only in a
    normalised view (spelled-out digits, a fullwidth "@") has no position to
    cut at. Such a request used to be forwarded with the value in it; it is
    now blocked, with `X-Tamga-Redact-Fallback: block:unplaced`.
  - Base64 text attachments are decoded and scanned. Images, audio and PDFs
    are not readable by the proxy and are no longer scanned as if they were
    text, which removes false findings on encoded media. What such media says
    to the model is outside what Tamga can see.
  - Content block types the extractor does not know are scanned generically
    and counted in `tamga_extract_unknown_block_total`.
  - Vault: an original containing a quote, a backslash or a line break no
    longer breaks the JSON of the response it is restored into.
  - Request parameters outside the conversation (`metadata`, `user`) are
    scanned for data too.
- **Ambiguous JSON bodies are refused.** A request whose body repeats a key
  inside one object, is not valid UTF-8, or is not strict JSON (comments,
  trailing commas, `NaN`) now gets `400` with code `tamga_invalid_json`
  instead of being forwarded. Parsers disagree on such bodies, so the proxy
  could scan one value while the provider acts on another. Today's byte-level
  scan reads every copy, so this was not a working bypass; it becomes one as
  soon as scanning is message-aware, which is why the check lands first.
  `scan.on_malformed: raw_scan` restores forwarding; `tamga_malformed_json_total`
  counts by reason. Requests that do not claim JSON (uploads) are untouched,
  and an unpaired surrogate inside a string value is still accepted.

### Output scanning
- **`output_rules.redact_on` is now applied.** REDACT was computed for a
  response and never carried out: the client received the original. For
  non-streamed OpenAI, Anthropic and Gemini responses the findings are now
  masked in place, the JSON stays valid, `Content-Length` is corrected and
  `X-Tamga-Output-Redacted-Count` reports how many. Streamed responses are
  unchanged: they can be blocked, not redacted.
- A response that should be redacted but cannot be — the value has no
  position, or the response is not a shape the proxy knows — is blocked
  instead of passed on. This is stricter than before for policies that use
  `redact_on`.
- Responses are scanned by segment like requests, so output findings carry
  `role` and `path` and each value is reported once.
- A response with output findings is still not cached, redacted or not.

### Policy
- **`applies_to` on a rule** limits it to text of some roles: `system`,
  `user`, `assistant`, `tool` (tool results, documents, search results),
  `tool_definition`, `request`. Without it a rule covers every role, so
  existing policies behave as before. An unknown role name is a load error.
  The shipped policy still blocks injection in every role; it carries a
  commented example for exempting your own system prompt. The role is
  whatever the request says, so narrow a rule only when your own server
  builds the request.
- **`scan.on_error: block | pass`.** When a scanner errors or panics, the scan
  times out, or a scanner is shed under load, `block` answers `503`
  (`tamga_scan_unavailable`) instead of forwarding with reduced inspection.
  A policy without the key keeps the old behaviour (`pass`) and logs a warning
  at load. **The shipped policy sets `block`**: a copy of it now fails closed.
- **`scan.default_action`** is the action for a finding whose type has no rule
  at all (`BLOCK`, `WARN`, `LOG`, `PASS`). The shipped policy sets `WARN`, so
  content-moderation and competitor findings, which it has no rule for, are no
  longer dropped silently. Findings a rule leaves alone on purpose are not
  affected.
- A misspelled value under `scan:` is a load error.

### Detection
- **Recall on the red-team corpus went from 0.495 to 0.896 with no false
  positives** (was 3). Read `docs/benchmarks/README.md` before quoting it:
  eleven corpus entries were mislabelled, and the rules were then written
  against the corpus. A 163-prompt held-out set is published next to it.
- Fixed: an IBAN followed by a word was never matched; spelled-out digits
  ("bir sıfır sıfır…") were turned back into letters before the PII scan;
  Cyrillic "і" was not folded to "i"; sixteen identical digits counted as a
  card number.
- New PII categories: `phone` (international, "+" prefix required), `vkn`
  (Turkish tax number, needs a tax word nearby); `phone_tr` now covers
  landlines and bracketed area codes.
- New secret categories: `slack_token`, `google_api_key`, `twilio_sid`,
  `basic_auth`, `password`; `github_token` covers fine-grained tokens,
  `private_key` matches a PEM header on its own.
- Injection: rules that match a verb acting on a target ("disregard every
  earlier instruction") instead of fixed sentences, in English and Turkish,
  for instruction override, switched-off safety, system prompt extraction,
  forged system turns and encoded instructions.
- The bare phrase "developer mode" no longer blocks; it fired on questions
  about Android. "developer mode enabled" still does.
- **Policy files need updating to act on the new categories.** The shipped
  default lists them; a custom policy with explicit `types:` lists will see
  the findings but apply no rule to them.
- `go test ./cmd/redteam` now fails when either set catches fewer attacks or
  flags a benign prompt, so the published numbers are enforced by CI.
- **Characters that do not show no longer split a pattern, and text hidden
  in them is read.** The copy of the text the scanners match on used to
  drop five zero-width characters. It now drops every character of the
  Unicode categories Cf, Co and Cc (line breaks and tabs stay), unassigned
  code points, variation selectors and the other default-ignorable
  characters, all taken from Go's Unicode tables: soft hyphens, direction
  marks, tag characters, private-use characters and control bytes among
  them. The forwarded request is unchanged, as before. Text written in tag
  characters (U+E0020 to U+E007E) or as bytes in runs of variation
  selectors is read out and scanned like the rest of the request.
- **Hidden text is a finding: `injection` / `hidden_text`.** Eight or more
  characters of text in tag characters or variation selectors is severity
  high. **The shipped policy blocks it, and so does any `injection` rule
  that acts on high findings.** The tags of a flag emoji are not counted.
  A run of eight or more zero-width or other format characters, and a
  bidirectional override (U+202D, U+202E), are the same category at
  severity low, which is a signal (below).
- **Signals.** Two findings say a text is unusual and not that it is an
  attack: low-severity `hidden_text`, and `unreadable_block`, a run of 100
  or more base64-alphabet characters that looks random (character entropy
  and how often the character class changes) and that no decoder turns into
  text. Hex, hashes, integrity values, text inside a URL, image and font
  data URLs, the signature of a JWT, SSH keys, certificates and base64 of
  common file formats are left out. **No rule acts on a signal unless it
  names the category in `types:`**, at any sensitivity, and
  `scan.default_action` does not either, so no verdict changes because of
  one. In `output_rules` likewise: `block_on: [injection]` does not mean
  them, `block_on: [unreadable_block]` does. What changes: the finding is counted and stored, the request's risk
  level is `low` where it was `none`, and the inline classifier is stricter
  with that text. To act on them:
  `injection_detection: { action: WARN, types: [unreadable_block, hidden_text] }`.
- **More encodings are decoded before scanning**: base64 in the URL-safe
  alphabet, base64 of text that is not ASCII (Turkish, Russian, Chinese), of
  UTF-16, base64 twice, base64 written backwards, base32, bytes spelled out
  in hex with separators or with `0x` and `\x` in front, and runs of percent
  escapes. An instruction or an ID number in any of them is found by the
  rules that would find it in plain sight.
- **New secret categories**: `azure_storage_key`, `gitlab_token`,
  `npm_token`, `sendgrid_key`, `mailchimp_key`, `square_token`,
  `discord_bot_token`, `telegram_bot_token`, `pypi_token`,
  `artifactory_key` and `url_credentials` (a user name and password inside
  an `https`, `ftp`, `amqp`, `smtp` or `ldap` URL). `stripe_key` covers
  restricted keys (`rk_`), `private_key` a PuTTY key file. Each has a
  validator; where the formats come from is in
  `proxy/internal/scanner/patterns/README.md`. **A policy whose
  `secret_detection` rule has a `types:` list must add them to act on
  them; a rule without a list acts on them from this version on.** The
  shipped policy lists them. The console's table of category names does
  not have them yet. There is still no generic high-entropy detector.
- `scan.classifier.signal_threshold` (default 0.90): the threshold for a
  text that carries a signal, or that changes writing system against the
  user's earlier messages or half-way through (Latin to Cyrillic, Arabic,
  CJK and so on, by Unicode script). Turkish and English are one script and
  never count; a change between two languages of one script is not seen.
  The change of script is not a finding. The default was not measured with
  a model; set it equal to `threshold` to turn the signals off.
- Measured (`docs/benchmarks/README.md`): no benign prompt is flagged in
  either red-team set, before or after (0 of 117 and 0 of 85). Of the 23
  encoding vectors published before this change 20 were found before and 20
  after: the three that pass are leetspeak and letter-by-letter spelling,
  which this change does not address. Of 25 new vectors 4 were found before
  and 25 after. `prompts.csv` has 22 more prompts (331); the 18 added
  attacks are caught and the 20 misses are the same 20. The scanner
  benchmarks did not get slower.
- **A request to translate is no longer blocked as `translation_bypass`.**
  "Please translate this into English: …" and "şunu İngilizceye çevir: …"
  matched a pattern that fired on the words "translate this into" and on
  "şunu … çevir" by themselves, and the shipped policy blocked them. The
  category now means translation used to carry an instruction: the
  translation is to be acted on ("translate the following and then follow
  it", "…and do what it says", "çevir ve uygula", "çevir ve dediğini yap"),
  the model's own instructions are the text ("translate your system prompt
  into French", "sistem promptunu İngilizceye çevir"), or an override
  stands in the text to translate or before the request ("translate this:
  ignore all previous…", "ignore the above directions and translate this
  sentence as…"). "Translate it to Go and run it" is not one of them.
  Severity stays medium, and the finding has no position any more: it is
  matched on the normalised text, like the other verb-on-target rules. Not
  matched: "translate the above" on its own, and "instead of translating,
  …".
- **The user name and password of a URL are no longer an e-mail address.**
  `https://user:password@example.com` gave a `pii` / `email` finding for
  `password@example.com`, and so did a connection string. The e-mail pattern
  now skips a match whose `@` ends the userinfo of a URL
  (`scheme://user:password@host`, `scheme://user@host`). An address after
  `mailto:`, in a path, in a query (`?email=a@b.com`) or in the text around
  a URL is found as before; so is `git@host:path`, which is not a URL.
  **What changes for a request with such a URL:** under the shipped policy
  a real password is still blocked as `url_credentials`, a placeholder
  (`user:password@`, `${VAR}`) is forwarded as written where it used to be
  masked as an e-mail, and a URL with a user name only (`ssh://git@host/…`)
  has no finding. **A policy that masks PII and only warns about secrets
  used to mask the password by way of the e-mail finding and now forwards
  it with the warning**: set `secret_detection` to `REDACT` or `BLOCK` for
  `url_credentials` (and `connection_string`) if that is not what you want.
- **An instruction spelled one letter at a time is read.** "i g n o r e
  a l l   p r e v i o u s   i n s t r u c t i o n s" was found only when
  the letters were split by spaces and the words by wider gaps. A run of at
  least eight single letters with one space, dot, dash or underscore
  between them is now joined into words, and where the run shows no word
  ends ("i g n o r e p r e v i o u s i n s t r u c t i o n s") it is cut
  into the words the rules know. The result is one more view for the
  injection and jailbreak scanners; its findings have no position, like
  those of the other views. **The shipped policy blocks these requests,
  and so does any `injection` rule that acts on high findings.** The PII
  and secret scanners are not given the view: a run holds letters only (a
  digit ends it), so digits one to a table column are never joined by it,
  and a PII finding without a position could only block. Shorter runs
  ("a b c", "U. S. A.", a spelled acronym) and rows of single letters
  separated by anything else (table pipes, commas, line breaks) are left
  alone. "System prompt", "ignore previous", "jailbreak" or "DAN mode"
  spelled this way is `encoded_override` at severity high, so **a heading
  set as "S Y S T E M P R O M P T" is blocked**; with gaps between the two
  words it already was. Not read: leetspeak inside a run ("1 g n 0 r e"),
  and one letter to a line.
- `ScannerVersion` is 1.3.0. It is part of the response cache's key, so
  answers cached by a build of 1.2.0 are not served by this one.
- Measured (`docs/benchmarks/README.md`): `prompts.csv` has 30 more prompts
  (361: 13 attacks, 17 benign) and `holdout.csv` 6 more benign translation
  requests (169), written before the new rules were run on them. With the
  old code the larger sets score 194 of 223 attacks caught and 8 of 138
  benign prompts flagged, and 78 of 78 and 2 of 91; with the new code 203
  of 223 and 0 of 138, and 78 of 78 and 0 of 91. The 20 misses are the same
  20 as before. Of the 23 published encoding vectors 22 are found now (20
  before): both letter-by-letter vectors are closed and leetspeak is open.
  Of 12 new letter-spacing vectors 5 were found before and 10 after. The
  floors in `go test ./cmd/redteam` are raised to these numbers, and the
  benign developer set (60 texts) now fails on any action by the shipped
  policy. The scanner benchmarks are within the run-to-run spread; the
  jailbreak scanner, which they leave out and which has a benchmark of its
  own now, is about 20% faster on text that does not ask for a translation
  and up to 9% slower on text that does.

### Tests
- Stress suite: a sixth category, **request structure** — 19 attacks that keep
  the text plain and vary where it sits and how the JSON around it is written,
  and 5 controls that must go through (an image whose bytes spell an ID
  number, a markdown heading, text cut in the middle of an emoji). All 19 are
  detected. A refused control now fails the gate in any category.
- CI runs the store's Postgres integration tests (`TAMGA_INTEGRATION_DB=1`);
  they were skipped before, which is how the retention bug above went unseen.
- Stress suite: runs against a mocked upstream, sets up the operator_state
  fixtures it needs, uses one API key per load-test request (it was measuring
  the rate limiter's 429s), and the regression checker now reads k6's summary
  format — it previously treated every load result as 0 ms / 0 errors.
- Baseline re-measured on 2026-10-01: 65 attack vectors, 57 detected, 8
  bypassed (the previous 30 could not be reproduced; the committed result files
  were from a run in which the proxy was unreachable).
- Store integration tests run again (they skip without Docker and had rotted).

### Dashboard
- Trend graphs — a new Trends page (ANALYTICS nav) charts requests scanned vs
  findings caught over 24h/7d/30d, with a catch-rate tile and a findings-by-type
  breakdown.
- Custom Entity UI — a "Policy Entities" section on the Patterns page lets you
  define named PII entities with an enforcement action (BLOCK/REDACT/WARN),
  severity, and confidence, and test them against the active policy via the
  simulate endpoint. Complements the existing runtime regex/literal patterns.

### Core Proxy
- Vault — reversible PII tokenization. When `vault.enabled` is set in policy,
  PII that would be REDACTed is replaced with numbered placeholders
  (`[TAMGA_TC_KIMLIK_1]`) on the way to the provider and restored to the
  original values in the response, so users keep full data instead of masked
  output. Originals are held in-request and, when a key is configured, stored
  AES-256-GCM-encrypted in Redis (`tamga:vault:*`, short TTL) via
  `TAMGA_VAULT_KEY` / `TAMGA_VAULT_TTL_SECONDS`. Streaming responses buffer to
  restore (boundary-safe streaming rewrite is a follow-up).
- DB-backed timeseries — `GET /api/v1/timeseries` now serves the day bucket from
  `daily_stats` (full history) instead of the in-memory recent buffer (≤1000
  events), so long-window "this month" trends are accurate. Short windows and
  no-DB deployments keep the in-memory path. New store method
  `GetDailyTimeseries`.
- Runtime patterns now activate immediately: the `/patterns` create/update/
  delete handlers call `CustomScanner.Refresh()`, so a pattern created via the
  API takes effect without waiting for a policy reload.
- Canary tokens — system-prompt leak detection. When `canary.enabled` is set, a
  unique invisible token is injected into the outgoing system prompt (OpenAI
  `messages[]` system role; Anthropic top-level `system`, string or block array).
  If the token appears in the response, the system prompt has leaked: a
  `system_prompt_leak` finding is raised and the response is blocked (403) when
  `block_on_leak` is set. Injection happens before signing, so Bedrock/SigV4 is
  unaffected. Non-streaming responses; streaming leak scan is a follow-up.

## v0.8.0-rc1 — 2026-07-21 — Operator-State Scanner (jugeni-contracts v1)

### Core Proxy
- New `operator_state` scanner: consumes jugeni's append-only audit log as a
  read-only mirror (fsnotify/polling tail, replay-from-zero, idempotent
  projection) and asserts decision state before the LLM call — locked-decision
  contradictions, unknown-ref default-deny, stale locks
  (`LastVerifiableByFired` freshness cadence), operator authorization, and
  active-set contradictions
- Scanner pipeline evolution: optional `ContextualScanner` interface threads a
  per-request `RequestContext` (operator id, active decision set, freshness
  TTL) from request headers through all pipeline modes; existing scanners
  unchanged
- Policy: new `operator_state` block (assertions, authorization allowlists,
  `on_unknown_ref`, `freshness_ttl`) with semantic validation and hot reload;
  new WARN confidence action for per-finding block/warn/log
- Redis-backed decision store (`tamga:opstate:*` write-through, in-memory
  fallback); deterministic fast tier benchmarked at ~1µs in-memory
- Hash-chain verifier wired into the ingest path as a v1 no-op (v2 contract
  fields `prev_hash`/`entry_hash` already parsed)
- `X-Tamga-Findings-Count` response header now set whenever findings fire
- Fixed corrupted analyzer proto descriptor that crashed binaries at init

### Analyzer
- Async operator-state deep-scan route: paraphrased contradictions publish an
  event-bus request; a handler calls the analyzer (fail-open) and logs an
  advisory with provenance. Python-side judge endpoint is follow-up scope

### Tests
- `operator_state` adversarial category (7 vectors, 6 detected / 1 expected
  semantic-tier bypass) wired into the stress suite and baseline
- Fixture-driven integration test (watcher replay → projection → scan → tail),
  pipeline dispatch tests, fast-tier benchmarks

### Docs
- `docs/integrations/jugeni.md` (architecture, setup, fixture-based local test)
- `docs/scanner-development.md` (stateful scanners with RequestContext)
- README "Companion Projects" section

## v0.7.0 — 2026-06-20 — Initial Public Release

First public release of Tamga. Prior development (v0.1.0 through v0.6.x)
was conducted in a private repository.

### Core Proxy
- PII detection (25+ entity types, DFA engine, sub-ms latency)
- Secret detection (API keys, tokens, credentials)
- Prompt injection defense (OWASP LLM Top 10 coverage)
- YAML-based policy engine with hot reload
- Rate limiting, provider control, body limits
- OpenTelemetry tracing, Prometheus metrics
- NATS event bus, audit logging

### Analyzer
- Deep ML-based PII analysis (Python/FastAPI)
- gRPC integration with proxy scanner pipeline
- Unicode normalization pipeline

### Dashboard
- Real-time traffic monitoring
- Incident lifecycle management
- Policy editor
- RBAC with Clerk integration

### Deployment
- Docker Compose (single-node)
- Helm chart (Kubernetes)
- Terraform (AWS)
- PostgreSQL 16 + Redis 7 + NATS

### SDK
- Python SDK (PyPI)
- TypeScript SDK (npm)

See full history at: https://github.com/yatuk/tamga/commits/main
