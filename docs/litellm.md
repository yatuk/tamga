# Tamga as a LiteLLM guardrail

A team that already runs a [LiteLLM](https://github.com/BerriAI/litellm) proxy
can have Tamga check its traffic without putting Tamga's proxy in the path.
LiteLLM has a guardrail type, `generic_guardrail_api`, that posts the texts
of a request (and of the answer) to an HTTP endpoint, with its tool
definitions and tool calls, and does what the reply says. Tamga serves that
endpoint.

It is the same scan and the same policy as on Tamga's own proxy path: the
endpoint is an adapter over [`POST /api/v1/scan`](operations.md#checking-a-text-without-sending-it),
not a second implementation.

## Setting it up

Make a Tamga key for LiteLLM. A `proxy` key is enough, and it opens nothing
else in the management API:

```bash
curl -s -X POST "https://tamga.internal/api/v1/apikeys" \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"label":"litellm","scope":"proxy"}'
```

Add the guardrail to LiteLLM's `config.yaml`. The two lines that are Tamga's
are `api_base` and `api_key`:

```yaml
guardrails:
  - guardrail_name: tamga
    litellm_params:
      guardrail: generic_guardrail_api
      mode: [pre_call, post_call]
      api_base: https://tamga.internal/api/v1
      api_key: os.environ/TAMGA_KEY
      default_on: true
```

LiteLLM appends `/beta/litellm_basic_guardrail_api` to `api_base` and sends
`api_key` in the `x-api-key` header. `pre_call` checks requests, `post_call`
checks answers; leave one out to check only the other. Do not set LiteLLM's
`unreachable_fallback: fail_open` or `fail_on_error: false` unless you mean
traffic to pass unchecked when Tamga cannot answer (see below).

LiteLLM's guardrail quick start puts `guardrails:` at the top level of
`config.yaml`, as above. Its page on this guardrail type shows the same list
under `litellm_settings:`. If the guardrail does not load, try the other
place.

## What Tamga returns

| Policy action | Reply to LiteLLM | What LiteLLM does |
|---|---|---|
| `BLOCK` | `{"action":"BLOCKED","blocked_reason":"…"}` | Refuses the request. Its own caller gets an error, HTTP 400, with the reason as the message. |
| `REDACT` | `{"action":"GUARDRAIL_INTERVENED","texts":[…]}`, with `"tools":[…]` when a tool definition was masked | Goes on with the returned texts, and tools, in place of the ones it sent. |
| `STRIP` | the same, with the stripped text replaced by `[Content removed by Tamga security policy.]` | The same. |
| `WARN`, `LOG`, `PASS` | `{"action":"NONE"}` | Goes on unchanged. |

- `blocked_reason` names the kinds of finding the policy blocks on and the
  Tamga request id, for example `Request blocked by Tamga policy:
  pii/tc_kimlik. Tamga request id 0199….` It never holds what was matched,
  masked or not: LiteLLM shows it to whoever sent the request.
- `texts` has one text for each one LiteLLM sent, in the same order. Values
  are masked with the markers the proxy uses, `[email_REDACTED]`,
  `[tc_kimlik_REDACTED]`.
- A `REDACT` or `STRIP` that cannot be carried out is a `BLOCKED`, as on the
  proxy path. The reason then says so.
- The reply has no field for findings that do not change the outcome, so a
  `WARN` looks like a `PASS` to LiteLLM. The policy's own action is in the
  `X-Tamga-Action` response header, and the findings are in the request
  record (below).

**Answers** (`input_type: response`) are judged by `output_rules`, exactly as
the proxy judges an answer: `block_on` blocks, `redact_on` masks, and with
`output_rules` off every answer passes. The input rules are not applied to
answers.

**Roles.** The contract's `texts` carry no role. On LiteLLM's chat routes the
call also carries the conversation (`structured_messages`); when its texts
match `texts` one for one, each text is scanned in the role of its message,
so a rule with `applies_to: [tool]` covers tool results. In every other case
all texts are scanned as the user's.

## Tools and tool calls

A request through LiteLLM carries more than its texts, and Tamga reads it
as its own proxy reads the same request
([what is scanned](AGENTS.md#what-is-scanned)):

| Field | Read as | What a rule does there |
|---|---|---|
| `tools` (a request's tool definitions) | Role `tool_definition`: every string in a definition but its `type`, so the name, the description and the parameters' descriptions | `BLOCK` blocks. `REDACT` masks the value and `STRIP` replaces the string, and all the tools go back in `tools`. |
| `tool_calls` (the tool calls in the conversation, or the model's in an answer) | Role `assistant`: the strings in the arguments, as the proxy reads the arguments the model gave a tool. The function's name is not read, as on the proxy path. | `BLOCK` blocks. `REDACT` and `STRIP` block as well: see below. |
| `images` | Not read. Tamga's proxy does not read images either. | |

So a poisoned tool description is caught by a rule that covers
`tool_definition`, and a national id in the arguments of a tool call by the
rule that would catch it in a message:

```yaml
rules:
  injection:
    action: BLOCK
    sensitivity: low
    applies_to: [tool, tool_definition]
```

On an answer the tool calls are judged by `output_rules`, like its texts.
Tool definitions are a request's: `tools` sent with an answer is not read.

**What is rewritten and what is not.** The contract's reply has `texts` and
`tools`, and nothing for tool calls. So:

- A value masked or stripped in a **tool definition** goes back: the reply
  carries every tool, in the order received, changed only where the policy
  changed it. LiteLLM v1.104.2 puts the returned list in place of the
  request's own on its chat completions, Anthropic messages and Responses
  routes (read in
  `litellm/llms/openai/chat/guardrail_translation/handler.py`, the
  `anthropic/chat` one and the `openai/responses` one). Its other routes
  were not read; if one sends `tools` and does not take them back, a
  `REDACT` there is not applied, so use `BLOCK` for tool definitions if you
  serve such a route. When no tool was changed the reply has no `tools`, and
  LiteLLM keeps its own.
- A value that the policy would mask or strip in a **tool call's
  arguments** cannot be returned, in a request or in an answer. The call is
  `BLOCKED`, the reason reads `REDACT could not be carried out (tool_call)`
  (or `STRIP …`), and the response header `X-Tamga-Redact-Fallback` (or
  `X-Tamga-Strip-Fallback`) is `block:tool_call`. This is the proxy's own
  rule for a `REDACT` it cannot carry out: sending the value on unmasked is
  the one outcome `REDACT` must not have.
- A tool's **name** is never rewritten, because LiteLLM matches the tools
  it gets back to its own by name. A value to mask there blocks, with
  `tool_name` in the same places.

A policy that masks e-mail addresses will therefore block a conversation
whose history holds a tool call with an address in its arguments. If that
is too strict for your traffic, scope the rule with `applies_to` so that it
does not cover `assistant`.

## When Tamga cannot give a verdict

A scan that did not run to the end is answered with HTTP 503 and the code
`tamga_scan_unavailable`, never with an action. This is so whatever
`scan.on_error` and `output_rules.fail_open` say: those settings are for
Tamga's own proxy path. LiteLLM treats a 503 as the guardrail being
unreachable and, by default (`unreachable_fallback: fail_closed`,
`fail_on_error: true`), refuses the request. Whether to fail open instead is
LiteLLM's setting, and it then applies to every failure, including Tamga
being down.

Other refusals are also plain HTTP errors with a `tamga_…` code: 401 for a
missing or invalid key, 429 over the rate limit, 413 over a size limit, 400
for a body that is not the contract's. LiteLLM handles each as a failed
guardrail call.

## Limits

- 600 calls a minute per key, unless the policy sets
  `rate_limit.max_requests_per_minute`. Each request through LiteLLM is one
  call, and one more with `post_call`. The count is apart from the key's
  scans and proxy requests.
- A body of at most 1 MiB; at most 256 texts, 256 tools and 256 tool calls;
  and at most 256 KiB of what is scanned in total: the texts, and the tools
  and tool calls as LiteLLM sent them. Over any of these the answer is 413.
  LiteLLM sends the tool definitions with every request, so a request with
  very large tool definitions can reach the limit with little text.
- Scans share the scan API's slots; when all are in use the answer is 503
  with `tamga_scan_busy`.
- The scan of an answer, all its texts and tool calls together, may take up
  to 10 seconds; one that is not done by then is a 503 with
  `tamga_scan_unavailable`. The size limit above is what keeps a scan well
  inside that: the scanners read about 250 KB a second on one core.
  `output_rules.scan_window_ms` does not apply here, unless it is set longer
  than 10 seconds, and then it is the bound. That window is how long Tamga's
  own proxy path holds a buffered answer back from a waiting client; over
  the bridge LiteLLM is already waiting for the verdict. The scanner
  pipeline's own timeout (`TAMGA_SCANNER_PIPELINE_TIMEOUT_MS`) applies to an
  answer as it does to a request.

## What is recorded

A call over the bridge is real traffic, so it is recorded as a request on
Tamga's proxy path is. It appears in the console (the request list, the
live feed, the counts and the findings breakdown), in the stored request
log, and in the NATS stream when that is on:

| Recorded | Value |
|---|---|
| Provider | `litellm`, shown as "LiteLLM bridge". Tamga is not told which provider LiteLLM sends the request to; this names the way the request came. Filter with `?provider=litellm`. It is not counted as a shadow provider. |
| Endpoint | `/api/v1/beta/litellm_basic_guardrail_api (request)` or `… (response)` |
| Action | The policy's action for the call: `BLOCK`, `REDACT`, `STRIP`, `WARN`, `LOG`, `PASS` |
| Findings | Type, category, severity, role, the scanners' masked match (hashed with `data.hash_findings`), and where it was: `texts[1]`, `tools[0].function.description`, `tool_calls[0].function.arguments#q` |
| Organisation | The Tamga key's. A call with the admin key goes to the default organisation. |
| Model | The `model` LiteLLM named, when it looks like a model name |
| Policy, time, latency | As for any request |

Not recorded, because Tamga never sees them here: tokens, cost, cache
status, the user. No text is recorded, in the row or in the log.

- **Every request call is a row**, a clean one included, as every request on
  the proxy path is.
- **An answer call is a row when something was found or done.** LiteLLM
  calls once more for every answer, and for a streamed answer once every few
  chunks; a row for each clean one would be several rows per request that
  say nothing. A row for an answer has the verdict as its action and the
  findings as its findings (role `assistant`), and the same again in its
  output fields (`output_action`, `output_findings`).
- A request and its answer are two calls and two rows. `litellm_call_id`
  links them in Tamga's log line for each call (`litellm bridge call`, with
  the Tamga request id); there is no column for it in the request log.
- **Alerts fire as on the proxy path**: a `BLOCK`, `STRIP`, `REDACT` or
  `WARN` goes to the alert destinations, with the direction `output` for an
  answer, and a rule's own `notify` URLs are told about a request.
- A call that got no verdict (a 503, a 401, a 429, a 413, a 400) is a log
  line and a count (`tamga_scan_api_requests_total`, outcomes prefixed
  `litellm_`), not a row.
- Recording does not hold up the reply: the event is published after the
  reply is written and dropped if the event queue is full
  (`tamga_events_dropped_total`).

`POST /api/v1/scan` is different and stays unrecorded: it answers what the
policy would say about a text, and stores nothing.

## What is not supported over the bridge

- **No restore.** On its own proxy path Tamga can put masked values back in
  the answer (the vault). Over the bridge nothing is kept, so nothing can be
  put back: the model sees the marker and so does the caller. The contract
  does link an answer to its request (`litellm_call_id`), but restoring would
  mean keeping the original values in Tamga under an id the caller chooses.
- **Images are not read** (`images`). Tamga's own proxy does not read them
  either.
- **Tool-call arguments are never rewritten**, only passed or blocked (see
  [Tools and tool calls](#tools-and-tool-calls)).
- **Streamed answers** are checked the way LiteLLM hands them over (by
  default every few chunks, block only). Tamga's own stream scanning is not
  involved, and with LiteLLM's default a masked answer is not what the client
  receives while streaming.
- **No audit log entry.** The audit log is for changes to Tamga and
  refused management credentials; a guardrail call is a request, and is in
  the request log.
- The `operator_state` scanner, the response cache, budgets and provider
  pools belong to the proxy path and play no part here.
- What LiteLLM says about its caller (`request_data`: key alias, user, team)
  is not read. Policy exceptions go by the role on the Tamga key.

## What leaves the machine

Nothing. Tamga receives what LiteLLM sends it, scans it in process, and
answers. It forwards the texts, tools and tool calls nowhere, stores none
of them, and logs none of them. What it keeps of a call is the record
above: the verdict and the findings with their masked matches. An alert
names the kinds of finding and carries no match.

## What this was written and tested against

- LiteLLM's Generic Guardrail API as documented at
  `docs.litellm.ai/docs/adding_provider/generic_guardrail_api` and as
  implemented in LiteLLM **v1.104.2**
  (`litellm/proxy/guardrails/guardrail_hooks/generic_guardrail_api/generic_guardrail_api.py`
  and `litellm/types/proxy/guardrails/guardrail_hooks/generic_guardrail_api.py`),
  read on **2026-10-10**. LiteLLM marks the API as beta, and its path says so.
  What LiteLLM does with returned `tools`, and that it reads no `tool_calls`
  from a reply, was read in the same files and in the three route handlers
  named above, at the same tag.
- The Go tests call the endpoint as a fake caller that speaks that contract.
  **It has not yet been run against a real LiteLLM container.** The
  end-to-end test that does this is written (`deploy/e2e/litellm/`, run by
  the `litellm-e2e` CI job) and, until it has passed once, the statements on
  this page about what LiteLLM does with a reply (the 400 its caller gets,
  the texts it writes back, the `guardrails:` entry above) come from reading
  LiteLLM's documentation and source, not from watching it.

To run the end-to-end test yourself (Docker with the compose plugin):

```bash
bash deploy/e2e/litellm/run.sh
```
