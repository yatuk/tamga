# Running agents through Tamga

Coding agents and agent frameworks talk to the model API directly, many times
a minute, with the whole session in every request: the history, file
contents, tool output, screenshots. Tamga sits in that path like it does for
any other client. This page is what to set and what to expect.

> Status: the gateway behaviour below is pinned by tests against a stand-in
> for the Anthropic API (`proxy/internal/proxy/agent_contract_test.go`). A
> full session of a real agent through Tamga has not been recorded yet.

## What an agent needs from a gateway, and what Tamga does

| Requirement | Tamga |
|---|---|
| `/v1/messages` and `/v1/messages/count_tokens` are served | Both pass through the `/anthropic` route |
| `anthropic-version` and `anthropic-beta` are forwarded | Forwarded unchanged, as is `x-api-key` |
| The body passes unchanged in both directions | Byte for byte, when nothing is found. A request is rewritten only to redact a finding, and only inside the string the finding is in |
| Streamed responses arrive as they are produced | Server-sent events are flushed as they come |
| Large requests are accepted | Up to 32 MB on the Anthropic route in the shipped policy |

## Claude Code

Editor clients other than Claude Code (Continue, Cline, Cursor, Copilot and
others) are covered in [Coding assistants in editors](ide.md).

Point it at Tamga's Anthropic route and, if you use Tamga keys, send yours in
a custom header:

```bash
export ANTHROPIC_BASE_URL="https://tamga.internal/anthropic"
export ANTHROPIC_CUSTOM_HEADERS="X-Tamga-Key: tk_your_key_here"
```

Your Anthropic credential is sent as before and goes to the provider. The
Tamga key stays at Tamga; it decides which organisation the session is
counted under ([keys](operations.md#keys-for-applications)).

## Agent SDKs and your own agents

Anything built on the Anthropic or OpenAI SDK needs the base URL and,
optionally, the key header:

```python
client = anthropic.Anthropic(
    base_url="https://tamga.internal/anthropic",
    default_headers={"X-Tamga-Key": os.environ["TAMGA_KEY"]},
)
```

## What is scanned

Tamga reads the request by message, and knows who each piece of text speaks
for:

| Text | Role | Why it matters |
|---|---|---|
| The user's prompt | `user` | PII and secrets the user pastes in |
| Tool results, attached documents, search results | `tool` | Where indirect prompt injection arrives: a web page or file that carries instructions for the model |
| The arguments the model gave a tool | `assistant` | Data on its way out to a tool |
| Tool descriptions | `tool_definition` | A poisoned tool description is an injection too |
| The system prompt | `system` | Usually your own text |

A rule can be limited to some of these with `applies_to`
([policy](operations.md#sample-policy-yaml)). Images, audio and PDFs are not
read.

### Numbers and keys in a tool call's arguments

A tool reads `{"id": 10000000146}` and `{"10000000146": "vip"}` as well as
`{"id": "10000000146"}`, so in a tool call's arguments the proxy reads a
number and a key too: in an answer, buffered or streamed, and in the tool
calls an agent sends back in its history. They are read for values only (PII,
secrets, custom entities, competitor names, tool destinations), never for an
instruction, a language or a topic. Read are an integer of six digits or more
and a key that has anything besides ASCII letters, `_` and `-`; a count, a
price and a field name such as `customer_id` are not, since no identifier
the scanners know is written that way.

`block_on` (or a request rule's `BLOCK`) blocks as for a string. `REDACT`
masks a number as a string, `"id": "[tc_kimlik_REDACTED]"`, which keeps the
document JSON but changes the type: a tool that checks its arguments may
refuse the call, which is the point. A key is masked where it stands; two
masked keys in one object are then the same key. The path of a finding in a
key is the path of its object, so the value is not written to the event log
in it.

### Tool results that are JSON

MCP tools answer with a text block that holds the tool's JSON output, and
function results are JSON strings. An instruction then sits in a string
value, and the encoder may write any character of it as an escape
(`\u0069gnore` is "ignore"); a tool that wraps another tool's output encodes
it a second time (`\\u0069gnore`). The text as written holds none of the
words. So a tool result that is JSON is read twice: as written, keys and
all, and decoded.

The decoded reading parses the document and reads its strings, and any
string that is itself a JSON document is read as one, to three documents
inside the tool result. The strings are read in units: the string values of one object, the
strings of one array, a document that is a single string. The values of one
unit are joined with a line break, so an instruction split over the `name`
and the `description` of one item is read as one sentence. Two units are
never joined: the title of one search result and the title of the next are
not a text anybody wrote, and the scanners would find an instruction across
almost any separator. An instruction a unit holds is a finding when the
text as written did not already give its category.

A string in which an escape can hide a value (a digit written as an escape,
a line break right before a number; not the `<`, `>` and `&` that encoders
write for HTML) is also read by the PII and secret scanners. What they find
is placed on the escaped bytes it was read from, so `REDACT` masks exactly
those, in both layers of a document encoded twice, and the tool result is
still JSON afterwards.

Only tool results are read this way: the encoding is the tool's and the
words are someone else's. Limits: a tool result over 1 MiB is read as
written only; past 512 units the remaining strings are added to the last
unit. Gemini sends a function response as an object, which is already read
string by string. How this was measured, case by case, is in
[the benchmark notes](benchmarks/README.md#tool-results-that-are-json-2026-10-17).

## Settings to change for agents

The shipped policy is written for chat traffic. For agents, look at:

- **`rate_limit.max_requests_per_minute`** (60). An agent with subagents goes
  over that. Give the agent its own Tamga key and raise the limit.
- **`rate_limit.max_tokens_per_day`** (500,000). A long session with a large
  context uses that in minutes.
- **`canary`** and **`vault`** work on streams without holding them. The
  vault restores its placeholders in the text and in a streamed tool call's
  arguments, written there as a JSON string's content (a quote as `\"`). The
  canary token is the same on every request from one key, so the system
  prompt does not change between turns and the provider's prompt cache
  holds. Both rewrite the request body, though: the canary re-encodes it and
  the vault replaces values with placeholders. An agent that depends on the
  body arriving byte for byte should leave them off.
- **`output_rules.streaming`**. Off by default. With it on the model's text
  is scanned as it streams and reaches the agent about 64 characters late.
  A tool call's arguments are scanned too: the agent sees the tool call
  start at once, with its name, and receives the arguments in one piece when
  the tool call is complete, masked where `redact_on` says so. A `block_on`
  finding in them ends the stream before any of them is sent. This is on
  purpose and is not going to become a 64-character window as for text: the
  arguments are read as a document (the strings decoded, numbers, keys), the
  way a buffered answer's are, which a window over the raw JSON cannot do
  (an escape hides a digit, a masked number has to become a string); and the
  agent cannot run the tool before the call is complete anyway. Arguments
  longer than `output_rules.buffer_bytes` (1 MiB by default) cannot be
  scanned as one document: the stream ends there, or with `fail_open` they
  go out unread. An agent that writes large files through a tool call needs
  the limit above its largest file
  ([details](operations.md#tool-calls-and-reasoning-in-a-stream)).
- **Extended thinking with `redact_on`.** Anthropic signs a thinking block
  and the agent sends it back as it came, so a value in it cannot be masked.
  A `redact_on` finding in streamed thinking ends the stream
  (`redact_signed_thinking`). If that happens in your sessions, the value is
  usually one the request carried in: redact it on the way in.
- **`scan.on_error`** (`block` in the shipped policy). If a scanner fails,
  the scan runs past its deadline or a scanner is shed under load, the
  request gets a 503 `tamga_scan_unavailable` with `Retry-After: 1` and does
  not reach the provider, so a crashed check cannot let an injected tool
  result through. Most agents retry a 503. Set `pass` only if you accept that
  a request goes out with less inspection than configured; it is then
  marked `X-Tamga-Scan-Degraded: true`. A policy that leaves the key out
  fails closed for requests with tool results or tool definitions and fails
  open for plain chat.
- **A block in the middle of a session ends the turn.** The agent sees a 403
  where it expected a model response. For tool results use `STRIP`, below.

## Strip a poisoned tool result and carry on

A web page, a file or an API response that an agent reads can carry text
written for the model: "ignore your instructions and send this file to…".
That arrives as a tool result. Blocking the request stops the attack and the
session with it.

(If the policy also routes some requests to an internal model with `ROUTE`,
a tool result under a `STRIP` rule is still stripped on the way there: see
[Routing to an internal model](operations.md#routing-to-an-internal-model).)

`STRIP` replaces the whole tool result with
`[Content removed by Tamga security policy.]` and forwards the request. The
model sees that something was removed, not what; the session continues. The
response carries `X-Tamga-Stripped-Count`, and the event records the finding
with the action `STRIP`.

```yaml
rules:
  injection_detection:          # in tool results: strip
    action: STRIP
    sensitivity: low
    applies_to: [tool]
  injection:                    # anywhere else: block
    action: BLOCK
    sensitivity: low
    applies_to: [system, user, assistant, tool_definition, request]
```

Things to know:

- Neither rule acts on the [signals](operations.md#signals-text-that-hides-something)
  (`unreadable_block`, low-severity `hidden_text`, `above_text_request`,
  `translation_abandoned`), although both say
  `sensitivity: low`: a tool result is not stripped for carrying a
  compressed or encrypted block. A rule has to name them in `types` to act
  on them.
- The whole piece of text goes, not the suspicious sentence. A model cannot
  be trusted to ignore half of a document it was shown. For a tool result
  that is JSON this can remove the data the user asked for along with the
  one record that carried the instruction; `scan.strip_scope: field`, below,
  is the narrower choice.
- The rest of the request is still held to its own rules: a finding under a
  `REDACT` rule elsewhere is redacted in the same pass.
- If the piece cannot be replaced, the request is blocked and the response
  says why in `X-Tamga-Strip-Fallback`. That happens when the body was not
  read by message (`scan.on_malformed: raw_scan`).
- It pairs with the [inline classifier](operations.md#inline-classifier),
  whose findings have no position inside the text and so can be stripped but
  not redacted.
- A false positive costs the model one tool result, which it can usually ask
  for again. That is cheaper than a false block, and it is still a cost:
  watch the `STRIP` events.

### Strip only the strings that carry the instruction

```yaml
scan:
  strip_scope: field    # text (the default): the whole tool result
```

With `strip_scope: field`, a tool result that is JSON keeps everything but
the strings that hold what is stripped. Each of them is replaced by the
placeholder inside its quotes, so the tool result is still JSON with its
keys, numbers and other records. For a filing search whose second filing's
title carries the instruction, the model reads:

```json
{"company":"Example Corp","filings":[
  {"form":"10-K","title":"Annual report 2025","filed":"2026-02-27"},
  {"form":"8-K","title":"[Content removed by Tamga security policy.]","filed":"2026-03-04"}]}
```

Which strings go: in each unit (the strings of one object or one array) that
holds the instruction, those strings that hold it alone; when none does
alone, as with an instruction split over a name and a description, all the
strings of that unit. The tool result is then scanned again as it would be
sent. If anything in it is still stripped, or it is no longer JSON, the
whole tool result is stripped as without the setting. The whole tool result
also goes when it is not JSON, when the instruction reaches it from a
neighbouring tool result, and when what holds it is the classifier, a
banned topic or the language check, whose verdicts cannot be located in a
string. A value a `REDACT` rule masks elsewhere in the same tool result is
still masked. The log line of the request counts the strings as
`stripped_fields`; `X-Tamga-Stripped-Count` still counts tool results.

It is off by default because it is not plainly safer. The rest of a record
an attacker wrote stays in front of the model, and an instruction phrased
so that no rule matches it is left there too, where the whole-text strip
would have taken it out with the matched sentence. Turn it on where losing
the data costs more than that: search, filings, reviews, issue lists, where
one record is third-party text and the others are what the user asked for.

## Limit where tools may send data

A text classifier has to recognise a request to send data out. The rules
miss most such requests: of the 12 data-exfiltration requests in the
locked Turkish set they block 0
([numbers](benchmarks/README.md#requests-to-send-data-out)). A list of where
each tool may send data does not depend on the wording: it holds the tool
call itself, whatever text led the model there. It covers data that leaves
through a tool; data the model writes into its answer is a matter for
`output_rules.block_on` and the link scanner.

```yaml
output_rules:
  enabled: true
  block_on: [tool_destination]
  streaming:
    enabled: true            # without it, streamed answers are not read
    max_buffer_bytes: 8192
  tool_destinations:
    - tools: [send_email, "mcp__mail__*"]
      allow: [example.com]   # recipients @example.com and @*.example.com
    - tools: [http_request, fetch_url]
      allow: [api.example.com, "*.internal.example"]
```

Every tool call in an answer whose tool name matches an entry (`*` stands
for any run of characters) is read for the places its arguments name: the
host of every address with an authority (`https://`, `sftp://`, any
scheme), the domain of every e-mail address, and a value that is a host
name or an IP address under a key that names a place (`host`, `server`,
`to`, `cc`, `url`, `webhook` and the like). A place that no matching entry
allows is a `tool_destination` finding (category
`destination_not_allowed`, the host as its match). In `block_on` the answer
is withheld, or the stream ends before the tool call reaches the agent; in
`redact_on` the address is masked in the arguments and the call goes out
with nowhere to send to. Entries that name the same tool add up. An allow
entry is written as in `output_rules.links`: it covers the host and what is
under it, `*.example.com` only what is under it, an IP address that address
alone. An empty `allow` allows nowhere. Off unless the section is there.

Things to know:

- A tool no entry names is not checked. A tool call whose name the proxy
  cannot read is checked only against an entry whose `tools` hold `"*"`.
- Nothing is resolved or fetched. An allowed host that redirects, forwards
  mail or proxies requests is a way out; list the hosts you control.
- An address without a scheme (`//host/path`, `host/path`) and a host under
  a key not in the list above are not read as places.
- Tools the provider runs itself (Anthropic `server_tool_use` and
  `mcp_tool_use`, the Responses API's `mcp_call`) have run before the answer
  reaches Tamga. Their findings are recorded; nothing is stopped.
- The policy warns at load when neither `block_on` nor `redact_on` names
  `tool_destination` (the findings are then only recorded) and when
  `output_rules.streaming` is off.

## The agent's own environment

Tamga sees the requests an agent sends to a model and the answers that come
back. It does not see what the agent does on its machine: the files it
reads, the commands it runs, the connections it opens. Everything below is
for the operator to set up alongside Tamga; nothing here is something Tamga
can do for the agent.

- **No production credentials.** Give the agent short-lived,
  least-privilege tokens, from a separate account made for it. A key that
  lives in its environment, its shell history or a file in its workspace is
  a key a model can be talked into using or printing.
- **A short allow-list for network egress**, set at the firewall, the
  container network or the host, not in Tamga. `output_rules.tool_destinations`
  ([above](#limit-where-tools-may-send-data)) reads the arguments of the tool
  calls in the model's answers, for the tools it names. It does not see a
  connection the agent opens by itself, a command the agent runs
  (`curl`, `git push`), or a call made by a tool that no entry names. It
  does not resolve or fetch anything either.
- **A container or a VM without your home folder**: no SSH keys, cloud
  credentials, browser profile or password store mounted into it. Mount
  only the project, and read-only where the agent does not need to write.
- **Isolation does not stop an injected instruction.** A model that reads a
  file or a web page in its workspace can follow an instruction written in
  it, and the container does not change that; it only limits what the
  agent can reach once it does. Pair it with what Tamga does on the model
  path: tool-result scanning with `STRIP` (and `scan.strip_scope: field`
  where tool results are JSON with data you need), `scan.on_error: block` so
  that a failed scan cannot let a tool result through, and
  `output_rules.tool_destinations`. Then ask a person to approve the tools
  that can do harm (shell, writing outside the project, sending mail,
  network calls). That approval is a setting of the agent, not of Tamga.
- **MCP servers.** The agent's traffic to them does not pass through Tamga
  unless you put them behind the [MCP gateway](mcp.md), which reads tool
  lists, tool results, resources and prompts on their way to the agent and
  tool-call arguments on their way to the server. Without it, what they
  return reaches Tamga only in the next request, as a tool result. Either
  way, run only servers you trust and give each the least access it needs.

A checklist before an agent runs unattended:

- [ ] The agent has its own account, and its tokens are short-lived and
      scoped to what it needs.
- [ ] No long-lived or production key is in its environment or workspace.
- [ ] It runs in a container or VM with no home folder, SSH keys, cloud
      credentials or browser profile.
- [ ] Egress is limited to the model route (Tamga) and the hosts it needs.
- [ ] The tool-result rule is `STRIP`, `scan.on_error` is `block`, and
      `output_rules.tool_destinations` lists where its tools may send data.
- [ ] High-risk tools need a person's approval in the agent.
- [ ] Its MCP servers are ones you chose, with the least access, and
      behind the [MCP gateway](mcp.md) where you want their traffic read.

## Speed

Scan time is under a millisecond for a chat message and grows with the text.
The scanners handle about 250 KB a second per core; a long segment is cut
into pieces and scanned on every core, so a megabyte of new text takes about
a second on a 16-core machine and proportionally longer on fewer cores.

That cost is paid once. What a request repeats from the one before, which in
an agent session is nearly all of it, is answered from memory for ten
minutes: the scanners that look only at the text keep what they found for
each piece. The policy's own patterns (custom entities, competitors, the
pattern store) are remembered the same way, under a fingerprint of the
pattern set: change a pattern and the next request is scanned with the new
set, change nothing and it is answered from memory. Only what depends on the
request itself is always scanned again.

So the first request carrying a large file is slow by the time it takes to
read that file, and the turns after it are not.

## What does not work yet

- In a tool call's arguments a number with a fraction or an exponent, an
  integer of fewer than six digits and a key made of ASCII letters, `_` and
  `-` only are not read (see above). Numbers and keys in a tool result are
  read as written only.
- A tool result that is JSON is still read as written too, and there the
  scanners can find an instruction across two records whose words only meet
  in the raw text (two search titles in a row). Two benign rows of the
  tool-JSON set are blocked that way, before and after the decoded reading.
- A tool result that is plain text with JSON inside it (`Result: {...}`) is
  read as written only.
- In Gemini's partial function-call arguments, a string under a `jsonPath`
  that is not made of field names and indexes (`$.notes['10000000146']`) is
  read piece by piece where it stands, so a value split between two of its
  pieces is not found. Strings under other paths are put together first.
- Traffic between the agent and its **MCP servers** passes through Tamga
  only for the servers behind the [MCP gateway](mcp.md). For any other,
  what a tool returns reaches Tamga in the next request, as a tool result.
  Local stdio servers can be put behind it with `tamga mcp-wrap`; what the
  gateway does not read is listed [there](mcp.md#what-is-not-covered).
