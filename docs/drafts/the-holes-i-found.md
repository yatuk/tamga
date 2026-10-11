> **Draft. Not published.** Written from the repository on 2026-10-11
> (branch `ui/carbon-pilot`, `CHANGELOG.md` under v0.9.0, the commits and
> tests named below). Every fact is to be checked against the repository
> again before this goes anywhere. Notes in [square brackets] are for the
> editor and must not survive.

# The holes I found in my own security proxy

Tamga is a proxy that sits between an application and a model provider. It
reads each request, and a policy file says what happens to what it finds: a
card number is blocked, an email address is masked, an instruction hidden in
a tool result is stripped. I wrote it, I wrote the policy format, and I wrote
the README that says what it does.

Before releasing v0.9.0 I went through the code and compared it with its own
claims. This is a list of eight places where the two disagreed. All eight
are fixed on the branch this was written from, and each has a test that
fails if it comes back. I am writing them down because the pattern in them
is more useful than the fixes.

I do not know how long each of these was in the code, and I have no record
of anyone being harmed by one. I am not going to guess at either.

## 1. A rule that said REDACT forwarded the value

**The claim.** A rule with `action: REDACT` masks what it matched and sends
the rest of the request on.

**What it did.** On a request, only PII findings and custom-entity findings
were masked. A secret under a REDACT rule, a competitor name, or a custom
entity reached through a `custom` rule went to the provider exactly as the
client sent it. The policy loaded, the validator had no complaint, and the
event said REDACT.

**How it was found.** By a matrix. I wrote a test that takes every kind of
finding the scanners make, every action that rewrites text, and both
directions (request and answer), and records one of four outcomes: masked,
stripped, blocked, or refused at load. It has a fifth outcome spelled in
capitals, `FORWARDED UNMASKED`, for the thing that must never happen. I ran
it against the code as it was, before changing anything, and that cell was
not empty. The commit message of `8dbf423` says so.

**The fix.** Every finding that has a position in the text is now masked,
whatever its kind, with a marker that names the kind and holds nothing of
the value (`MaskMarker` in `proxy/internal/scanner/mask.go`). A kind that
has nothing to mask, such as an injection finding, cannot be given REDACT:
the policy does not load, and the error names the actions that are possible.
If a REDACT verdict cannot be carried out at run time, the request is
blocked.

**What holds it.** `TestRewriteMatrix_Requests` and
`TestRewriteMatrix_Responses` in
`proxy/internal/proxy/redact_matrix_test.go`.

## 2. The management API allowed whatever it had not thought about

**The claim.** Three roles. A viewer reads, an analyst triages, an admin
changes things.

**What it did.** Each route checked its own permission, if it remembered to.
A route with no declared permission was open to every authenticated caller.
The check that a read-only caller cannot write ran for API keys and was
skipped for signed-in users. A viewer could run retention, reset a circuit
breaker, change incidents and saved hunts, and call the SCIM routes.

**How it was found.** [The repository does not record what started this.
What it does show is the method that made the gap impossible to miss: fill
in, or cut this paragraph.] Once every route is one row in one table, with a
column for the permission it needs, the empty cells are visible.

**The fix.** Routes are registered from a single table
(`proxy/internal/api/routes.go`). Each entry states its permission, or
states in a sentence why it is public. One guard applies that to every kind
of credential before the body is read. A route that declares nothing is
refused to everyone, the admin included. The default went from open to
closed.

This fix took things away from existing callers, and the changelog lists
them: viewers lose the event export, the audit log, the key list and the
webhook list; analysts lose policy reload, retention and circuit reset.

**What holds it.** `proxy/internal/api/authz_test.go` has a second copy of
the table, written by hand, of who may call what. A new route fails
`TestEveryRouteDeclaresItsPermission` until someone adds it there too.
`TestRoutesAreRegisteredOnlyFromTheTable` parses the package's own source
and fails if a handler is registered anywhere else, because, as its comment
says, the mux cannot be asked what it serves and the source can.
`TestUndeclaredRouteIsRefused` checks the closed default.

## 3. A rule in the shipped policy could never fire

**The claim.** The policy file that ships with the proxy had a rule named
`code_leakage`, with `action: WARN`. Anyone reading the file would conclude
that code leaking through the proxy raises a warning.

**What it did.** Nothing. The rule named findings that only the scan of a
response produces, and what happens to those is decided in another section,
`output_rules`. No finding could ever reach the rule. The operations guide
had a sample with the same fault under another name, `pii_critical`, so an
operator who copied the sample believed national ids and card numbers were
being stopped by a rule that matched nothing.

**How it was found.** In an audit of the code before the release; the commit
that fixed it (`8ec4ee4`) is titled "close what the audits left open".

**The fix.** The rule is removed from the shipped policy, with a note in its
place, and the sample in the guide is corrected.

**What holds it.** `TestShippedAndSamplePoliciesHaveNoDeadRules` in
`proxy/internal/policy/never_fire_test.go` loads the shipped policy and
every policy sample in the documentation and fails on a rule that no finding
can match.

## 4. Text the proxy had blocked was sent to a provider anyway

**The claim.** A blocked request does not reach a model provider. That is
the first sentence anyone would write about this product.

**What it did.** Tamga has an optional second service, the analyzer, which
looks at requests more slowly after the proxy has decided. The proxy copied
request bodies to it, blocked ones included, as they had arrived. One of the
analyzer's scanners is an "LLM judge" that asks a model whether a text is an
injection. It called out whenever an API key was present in its environment.
A placeholder key counted as present, and the example environment file
shipped one. So the first 2,000 characters of a request the proxy had just
refused to send to a provider could be sent to a provider, by Tamga.

**How it was found.** [Not recorded. In the history the fix (`2b5ec40`)
comes directly after the commit that added the migration guide from LLM
Guard, and that guide has a section called "Where secrets and data go". If
writing that section is what raised the question, say so here; otherwise
cut.]

**The fix.** The proxy sends the analyzer no request text unless
`TAMGA_ANALYZER_SEND_CONTENT=true`, never the text of a blocked request, and
only the body as it was forwarded, so what REDACT masked is already gone.
The judge runs only with `TAMGA_LLM_JUDGE=anthropic` and a key that is not a
placeholder. Both are off by default, which means a deployment that relied
on them has to turn them on. `.env.example` no longer ships a placeholder.

**What holds it.** `TestAnalyzerHandler_BlockedRequestIsNotSent` in
`proxy/internal/events/analyzer_content_test.go`, and on the analyzer's side
`test_judge_is_off_unless_asked_for` and
`test_judge_is_not_called_for_blocked_content` in
`analyzer/tests/test_injection_llm.py`.

## 5. A secret finding stored eight characters of the secret

**The claim.** Findings are stored masked. The log tells you a secret was
there and does not hold it.

**What it did.** The stored match of a secret finding was the first eight
characters of what the pattern matched, followed by dots. For a secret with
a long label in front, those eight characters were the label. For the
password of a connection string, a bare key, or a secret after a short
label, they were eight characters of the value. That went into the request
log, the console and the exports.

**How it was found.** [Not recorded: fill in, or cut.]

**The fix.** A secret finding keeps the vendor's marker when the kind has
one, such as `AKIA****` or `ghp_****`, and `****` otherwise.

It had a consequence I did not expect. The code that decides whether two
findings are the same value had been comparing those matches. With no
characters left in them, a second secret of the same kind would have been
taken for a repeat of the first and dropped, and a finding that is dropped
is not masked. Secrets now carry a fingerprint that exists only while the
request is being scanned and is never written anywhere.

Rows stored before the fix are not rewritten. They keep their eight
characters until retention deletes them, and the changelog says that in
bold.

**What holds it.** `proxy/internal/scanner/secrets_mask_test.go`:
`TestSecretMatch_NoMarkerNoCharacters` for the kinds that used to leak, and
`TestSecretDedupe_SameKindDifferentValues` for the consequence.

## 6. Row-level security that never ran

**The claim.** Migration 011 enabled row-level security on the request log.
Its own comment said it "enforces org-scoped access at the database level so
that a compromised query or bug cannot leak cross-tenant data".

**What it did.** The policy compared each row's organisation with a database
setting, `app.tenant_id`. Nothing in the proxy ever set it. The proxy also
connects as the role that owns the tables, and PostgreSQL does not apply a
policy to a table's owner unless the table is forced. So the policy applied
to nobody. Had it applied, every query would have failed, because the
setting was empty.

**How it was found.** The tell was in the test helpers: they switched the
policy off so that the tests could run at all (commit `0839aff` removed
those lines from `proxy/internal/store/testhelpers_test.go`).

**The fix.** I removed it (migration 018). I did not make it work. What
keeps organisations apart is the `org_id` condition in each query, which is
tested with two organisations. A real policy needs the proxy to set the
tenant on every transaction and a second database role for maintenance, and
that work is not done. The migration's comment gives the reason for removing it: a
policy that reads as protection and does nothing is worse than none.

**What holds it.** `TestMigrations_LeaveNoRowLevelSecurity` in
`proxy/internal/store/rls_integration_test.go` fails if any table has
row-level security after the migrations have run. If it ever comes back, it
has to come back with the code that makes it real.

## 7. Erasing one person's data could erase everyone's

**The claim.** `DELETE /api/v1/events/subject` removes the stored requests
that mention one person, selected by the hash of their national id. It
exists for erasure requests under data-protection law.

**What it did.** The selector was matched as a pattern. A selector
consisting of the wildcard character matched every row, so one call deleted
every stored request of the organisation. The response then said the rows
had been soft-deleted, which was also untrue: the delete was final.

**How it was found.** In the same audit as the dead rule; the fix is in the
same commit.

**The fix.** The selector must be exactly 64 hexadecimal characters, checked
before any query runs and again in the store. Later work replaced the
pattern match with an exact comparison. The response says the delete is
final. Reading a person's data, which left no trace before, now writes an
audit entry, and neither that entry nor the erase entry holds the selector's
value.

**What holds it.** `TestSubjectRequestsThatAreRefused` in
`proxy/internal/api/privacy_audit_test.go` sends the wildcard, among other
malformed selectors, and expects a 400 and an untouched store.

## 8. Patterns you could save and try, and that were never scanned for

**The claim.** The console lets an analyst add a pattern of their own. There
are two kinds, a regular expression and a literal. Both can be saved, and
both can be tried on a sample text in the console, which marks the match.

**What it did.** When the proxy built the list of patterns for its scanners,
it skipped every pattern of kind literal. The line that skipped them had a
comment saying literals are compiled somewhere else. There was no somewhere
else. An analyst could add a project code name as a literal, watch the
console mark it in a sample, and no request containing it was ever touched.

**How it was found.** By reading that comment and going to look for the
place it described (commit `d050da0`).

**The fix.** A literal is handed to the scanners as its own text with every
metacharacter escaped, matched without regard to case, which is how the
console tries it. The commit message warns that a deployment with literal
patterns will see them fire for the first time.

**What holds it.** `TestExpression` in
`proxy/internal/patterns/store_test.go`.

## What these have in common

Five of the eight are the same fault. The rule that could never fire, the
row-level security, the literal patterns, the REDACT that did not mask and
the roles that did not restrict were all things the product displayed and
did not do. None of them crashed or logged an error. Each one would have
passed a demo.

The tests that hold them now have a shape in common too. They do not test a
function. They enumerate something (every finding kind against every action,
every route, every policy sample in the docs, every table after the
migrations) and fail on the member nobody thought about. Most of these holes
were in the cell of a table that nobody had drawn.

## What is still not verified

This list is of what I found. It says nothing about what I did not, and the
repository is specific about where nobody has looked yet:

- Nothing has been run against a real provider or a real third-party
  service as part of the tests. The other side is always a stand-in
  (`README.md`, "Project status").
- The accuracy figures in the README come from prompts written for this
  repository; no independent set has been run. The adversarial suite is, in
  the README's words, "a regression suite, not an independent evaluation",
  and it has not been run since 2026-10-03 (`docs/benchmarks/README.md`).
- The optional classifier has not been measured on a set its author did not
  write.
- The LiteLLM guardrail endpoint has not yet been run against a real LiteLLM
  container (`docs/litellm.md`). A full session of a real coding agent
  through the proxy has not been recorded (`docs/agents.md`).
- The Helm chart has not been tested: nothing in the repository lints,
  renders or installs it.
- Images, audio and PDFs are forwarded without being read, and traffic that
  does not pass through the proxy is not seen at all (`README.md`, "What
  Tamga does not cover").
- Secret findings stored before fix 5 keep their eight characters until
  retention deletes them.

[Editor: a ninth candidate was left out because it is open. A value that is
written in a form only the normaliser reads is fixed and tested for two
cases, a second secret and a second card number (`CHANGELOG.md`;
`proxy/internal/scanner/secrets_mask_test.go`,
`proxy/internal/scanner/pii_card_mask_test.go`). The wider matrix for it,
`TestRewriteMatrix_Obfuscated`, is uncommitted work in progress and was
failing when this draft was written. Publish nothing about it until it is
committed, passing and in the changelog.]
