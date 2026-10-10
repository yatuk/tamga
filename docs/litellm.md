# Tamga as a LiteLLM guardrail

A team that already runs a [LiteLLM](https://github.com/BerriAI/litellm) proxy
can have Tamga check its traffic without putting Tamga's proxy in the path.
LiteLLM has a guardrail type, `generic_guardrail_api`, that posts the texts
of a request (and of the answer) to an HTTP endpoint and does what the reply
says. Tamga serves that endpoint.

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
| `REDACT` | `{"action":"GUARDRAIL_INTERVENED","texts":[…]}` | Goes on with the returned texts in place of the ones it sent. |
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
  `X-Tamga-Action` response header, and the count of findings by type is in
  Tamga's log line for the call.

**Answers** (`input_type: response`) are judged by `output_rules`, exactly as
the proxy judges an answer: `block_on` blocks, `redact_on` masks, and with
`output_rules` off every answer passes. The input rules are not applied to
answers.

**Roles.** The contract's `texts` carry no role. On LiteLLM's chat routes the
call also carries the conversation (`structured_messages`); when its texts
match `texts` one for one, each text is scanned in the role of its message,
so a rule with `applies_to: [tool]` covers tool results. In every other case
all texts are scanned as the user's.

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
- A body of at most 1 MiB, at most 256 texts, and at most 256 KiB of text in
  total.
- Scans share the scan API's slots; when all are in use the answer is 503
  with `tamga_scan_busy`.

## What is not supported over the bridge

- **No restore.** On its own proxy path Tamga can put masked values back in
  the answer (the vault). Over the bridge nothing is kept, so nothing can be
  put back: the model sees the marker and so does the caller. The contract
  does link an answer to its request (`litellm_call_id`), but restoring would
  mean keeping the original values in Tamga under an id the caller chooses.
- **Only `texts` is scanned.** Images, tool definitions and tool-call
  arguments (`images`, `tools`, `tool_calls`) are not read.
- **Streamed answers** are checked the way LiteLLM hands them over (by
  default every few chunks, block only). Tamga's own stream scanning is not
  involved, and with LiteLLM's default a masked answer is not what the client
  receives while streaming.
- **No request record.** A call is one log line and one count
  (`tamga_scan_api_requests_total`, outcomes prefixed `litellm_`). It does
  not appear in the console, the request log, the audit log or the alerts,
  because the scan API stores nothing either. LiteLLM's own logs hold the
  refusal.
- The `operator_state` scanner, the response cache, budgets and provider
  pools belong to the proxy path and play no part here.
- What LiteLLM says about its caller (`request_data`: key alias, user, team)
  is not read. Policy exceptions go by the role on the Tamga key.

## What leaves the machine

Nothing. Tamga receives what LiteLLM sends it, scans it in process, and
answers. It forwards the texts nowhere, stores none of them, and logs none
of them.

## What this was written and tested against

- LiteLLM's Generic Guardrail API as documented at
  `docs.litellm.ai/docs/adding_provider/generic_guardrail_api` and as
  implemented in LiteLLM **v1.104.2**
  (`litellm/proxy/guardrails/guardrail_hooks/generic_guardrail_api/generic_guardrail_api.py`
  and `litellm/types/proxy/guardrails/guardrail_hooks/generic_guardrail_api.py`),
  read on **2026-10-10**. LiteLLM marks the API as beta, and its path says so.
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
