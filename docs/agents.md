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

## Settings to change for agents

The shipped policy is written for chat traffic. For agents, look at:

- **`rate_limit.max_requests_per_minute`** (60). An agent with subagents goes
  over that. Give the agent its own Tamga key and raise the limit.
- **`rate_limit.max_tokens_per_day`** (500,000). A long session with a large
  context uses that in minutes.
- **`canary`**. Leave it off. It adds a fresh token to the system prompt on
  every request, which changes the prompt each time and defeats the
  provider's prompt cache; with it on, a streamed response is also held
  until it is complete.
- **`vault`**. Restoring vaulted values in the response holds a streamed
  response until it is complete.
- **`output_rules`**. Streamed responses are not scanned on the way back;
  only non-streamed ones are.
- **A block in the middle of a session ends the turn.** The agent sees a 403
  where it expected a model response. For tool results a rule on `WARN` is
  the gentler setting until the strip action lands (planned).

## Speed

Scan time is under a millisecond for a chat message and grows with the text.
The scanners handle about 250 KB a second per core; a long segment is cut
into pieces and scanned on every core, so a megabyte of new text takes about
a second on a 16-core machine and proportionally longer on fewer cores.

That cost is paid once. What a request repeats from the one before, which in
an agent session is nearly all of it, is answered from memory for ten
minutes: the scanners that look only at the text keep what they found for
each piece. Custom patterns and anything that depends on the request or the
policy are always scanned again.

So the first request carrying a large file is slow by the time it takes to
read that file, and the turns after it are not.

## What does not work yet

- Scanning or redacting **streamed responses**.
- A **strip** action that removes a suspicious tool result and lets the
  session continue, in place of blocking the request.
- Traffic between the agent and its **MCP servers** does not pass through
  Tamga. What a tool returns does, in the next request, as a tool result.
