# Coding assistants in editors

A coding assistant in an editor is an ordinary client of a model API, so it
passes through Tamga only if it can be told to send its requests to Tamga's
address instead of the vendor's. Some can, some can for chat but not for
inline completion, and some cannot. This page lists which, with the setting
for each, and says what Tamga does and does not see afterwards.

> Status: the Tamga side (routes, headers, scanning) is the same as for any
> other client and is described in [agents](agents.md) and
> [operations](operations.md). None of the client configurations below has
> been run against a Tamga instance by the project. Each was read from the
> vendor's documentation on the date given next to it (2026-10-17). Where the
> documentation did not say, the table says "not verified" rather than
> guessing. Vendors change these settings often: check the page before you
> roll a configuration out.

## Summary

"Seen" means the text reaches Tamga's scanners and the verdict applies.

| Client | Can be pointed at Tamga | Seen | Not seen |
|---|---|---|---|
| [Claude Code](#claude-code) (CLI, VS Code extension, JetBrains plugin) | Yes | Every model request: prompts, file contents, tool results, tool arguments | The assistant's own telemetry and update checks, which do not go to the model route; traffic to MCP servers |
| [Continue](#continue) | Yes | Chat, edit and autocomplete, per model, if the model's `apiBase` is Tamga | A model configured without `apiBase`; embeddings and rerank models you did not point at Tamga |
| [Cline](#cline) | Yes | Chat and agent requests through its OpenAI Compatible provider, or its Anthropic provider with a custom base URL | Custom headers: the page does not document a field, so no Tamga key can be sent (not verified) |
| [Roo Code](#roo-code) | Yes | Chat and agent requests through its OpenAI Compatible provider | Custom headers: not documented (not verified); the Anthropic provider's base URL was not verified |
| [Aider](#aider) | Yes | Every request, over the OpenAI-compatible interface | Tamga key header: not verified |
| [GitHub Copilot](#github-copilot) | Partly | Chat and agent requests to a model added as a Custom Endpoint in VS Code | Inline suggestions, which go to GitHub; semantic search and embeddings; Copilot's own models |
| [Cursor](#cursor) | Partly | Chat with your own key and an overridden OpenAI base URL, when Tamga is reachable from the internet | Tab completion; everything that does not use your key. Cursor's servers see the request before Tamga does |
| [JetBrains AI Assistant](#jetbrains-ai-assistant) | Partly | AI Chat with an OpenAI-compatible provider; inline completion if its own section is pointed at Tamga | Features left on JetBrains' models; whether the plugin calls the endpoint directly or through JetBrains was not stated (not verified) |
| [Zed](#zed) | Partly | Agent and inline assistant through an OpenAI-compatible provider; edit prediction through `open_ai_compatible_api` | Zed's default edit prediction (Zeta) and Copilot as a provider; the Anthropic `api_url` was not verified |
| [Windsurf](#windsurf) | Not verified | Nothing is known | The vendor documentation read did not describe a custom endpoint. Treat as no until it does |

## What Tamga offers a client

Two base URLs cover almost every client:

| API the client speaks | Base URL | Path the client then adds |
|---|---|---|
| OpenAI (chat completions, completions, Responses) | `https://tamga.internal/v1` | `/chat/completions`, `/completions`, `/responses` |
| Anthropic Messages | `https://tamga.internal/anthropic` | `/v1/messages`, `/v1/messages/count_tokens` |

`tamga.internal` stands for your own host; `http://localhost:8443` for a
local one. Further routes (`/openai`, `/mistral`, `/local`, `/azure`,
`/gemini`, `/bedrock`) are listed under
[Provider pools](operations.md#provider-pools).

Keys and headers:

- The provider's own key is sent as the client already sends it
  (`Authorization: Bearer` for OpenAI-style, `x-api-key` for Anthropic) and
  is forwarded to the provider. Tamga does not issue it.
- A Tamga key goes in **`X-Tamga-Key`** only. Tamga does not read a Tamga key
  from `Authorization` or `x-api-key`. It decides which organisation the
  request is counted under and which rate limit applies
  ([keys](operations.md#keys-for-applications)).
- Many editor clients have no field for an extra header. Such a client sends
  no Tamga key and is anonymous. That works while `TAMGA_REQUIRE_KEY` is off.
  With it on, those clients get 401. Either leave it off until the clients
  you use can send the header, or put a small reverse proxy in front of
  Tamga that adds `X-Tamga-Key` per user, for example one that maps each
  developer's network identity or client certificate to a Tamga key.
- Anonymous clients share a rate-limit bucket per client address. Behind a
  load balancer that is one bucket for everyone unless
  `TAMGA_TRUSTED_PROXIES` is set
  ([configuration](operations.md#configuration)).
- Tamga must be reachable from the developer's machine, over HTTPS if it
  crosses a network. The exception is Cursor, below.

## Claude Code

Sources read 2026-10-17:
[Connect Claude Code to an LLM gateway](https://code.claude.com/docs/en/llm-gateway-connect),
[Use Claude Code in VS Code](https://code.claude.com/docs/en/vs-code).

Can the base URL and key be set: yes. The variable is `ANTHROPIC_BASE_URL`;
the provider credential is `ANTHROPIC_API_KEY` (sent as `x-api-key`) or
`ANTHROPIC_AUTH_TOKEN` (sent as `Authorization: Bearer`). Extra headers are
set in `ANTHROPIC_CUSTOM_HEADERS`, one `Name: Value` per line.

CLI, in a shell or in the `env` block of `~/.claude/settings.json`
(`%USERPROFILE%\.claude\settings.json` on Windows):

```bash
export ANTHROPIC_BASE_URL="https://tamga.internal/anthropic"
export ANTHROPIC_CUSTOM_HEADERS="X-Tamga-Key: tk_your_key_here"
```

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://tamga.internal/anthropic",
    "ANTHROPIC_CUSTOM_HEADERS": "X-Tamga-Key: tk_your_key_here"
  }
}
```

VS Code extension: the vendor says to set the variables in VS Code's user
settings, because the extension checks credentials from there before it
starts the process:

```json
{
  "claudeCode.environmentVariables": [
    { "name": "ANTHROPIC_BASE_URL", "value": "https://tamga.internal/anthropic" },
    { "name": "ANTHROPIC_CUSTOM_HEADERS", "value": "X-Tamga-Key: tk_your_key_here" }
  ]
}
```

JetBrains plugin: the page read did not describe how the plugin takes
environment variables. Not verified. A variable exported in the environment
the IDE was started from, or the `env` block of `~/.claude/settings.json`
(which the CLI the plugin runs reads), are the likely routes; test with
`/status` in the session, which shows the base URL in use.

Notes from the vendor page that matter here:

- With only `ANTHROPIC_BASE_URL` set, a saved claude.ai login stays the
  active credential and its limits and billing apply. The traffic still goes
  through Tamga. Set a credential variable to replace the login.
- A gateway credential variable or `apiKeyHelper` makes the session
  unavailable for Remote Control and voice dictation.
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` stops update checks and
  similar background traffic to other hosts; it disables auto-updates.
- The vendor documents that a firewall in front of a gateway can block
  prompts that look like cross-site scripting. If Tamga sits behind a WAF,
  exempt the `/anthropic/v1/messages` path from body inspection.

What Tamga sees: every model request, with the whole session in it. Tool
results and tool arguments are scanned by role ([agents](agents.md#what-is-scanned)).
Agent sessions need the settings in
[Settings to change for agents](agents.md#settings-to-change-for-agents).

## Continue

Sources read 2026-10-17:
[OpenAI provider](https://docs.continue.dev/customize/model-providers/top-level/openai),
[config.yaml reference](https://docs.continue.dev/reference),
[autocomplete](https://docs.continue.dev/customize/deep-dives/autocomplete).

Can the base URL and key be set: yes, per model, with `apiBase` and `apiKey`
in `config.yaml`. Headers go in `requestOptions.headers`. Roles decide what
a model is used for.

```yaml
name: Tamga
version: 0.0.1
schema: v1

models:
  - name: Chat through Tamga
    provider: openai
    model: gpt-4o
    apiBase: https://tamga.internal/v1
    apiKey: <provider key>
    roles: [chat, edit, apply]
    requestOptions:
      headers:
        X-Tamga-Key: tk_your_key_here
  - name: Autocomplete through Tamga
    provider: openai
    model: <a model that supports fill-in-the-middle>
    apiBase: https://tamga.internal/v1
    apiKey: <provider key>
    roles: [autocomplete]
    requestOptions:
      headers:
        X-Tamga-Key: tk_your_key_here
```

For an Anthropic model use `provider: anthropic` with
`apiBase: https://tamga.internal/anthropic`. The reference page does not
state whether the Anthropic provider takes `apiBase` the same way: not
verified.

What Tamga sees: whatever each model sends. Chat requests carry the
conversation and the files the user attached. Autocomplete requests carry
the text around the cursor. `useLegacyCompletionsEndpoint: true` makes
Continue use `/v1/completions`; Tamga reads the `prompt` field of that
request as user text.

## Cline

Sources read 2026-10-17:
[OpenAI Compatible](https://docs.cline.bot/provider-config/openai-compatible),
[Anthropic](https://docs.cline.bot/provider-config/anthropic).

Can the base URL and key be set: yes. In settings choose API Provider
"OpenAI Compatible", then fill Base URL (`https://tamga.internal/v1`), API
Key and Model. The Anthropic provider page says: check "Use custom base URL"
and enter the URL (`https://tamga.internal/anthropic`). There is no settings
file key for this in the pages read; it is set in the settings panel.

The OpenAI Compatible page documents no custom header field, so a Tamga key
cannot be sent from Cline (not verified for the current release). Cline is
then anonymous at Tamga.

What Tamga sees: every request of the agent loop, including the tool
results it sends back.

## Roo Code

Source read 2026-10-17:
[OpenAI Compatible](https://roocodeinc.github.io/Roo-Code/providers/openai-compatible).

Can the base URL and key be set: yes. API Provider "OpenAI Compatible",
Base URL `https://tamga.internal/v1`, API Key, Model. The page says the model
must support native tool calling. It documents no custom header field (not
verified), so the client is anonymous at Tamga. It does not cover the
Anthropic provider, so a custom Anthropic base URL is not verified.

What Tamga sees: every request of the agent loop.

## Aider

Sources read 2026-10-17:
[OpenAI-compatible APIs](https://aider.chat/docs/llms/openai-compat.html),
[Anthropic](https://aider.chat/docs/llms/anthropic.html),
[.env options](https://aider.chat/docs/config/dotenv.html).

Can the base URL and key be set: yes for OpenAI-compatible endpoints, with
`OPENAI_API_BASE` and `OPENAI_API_KEY`, and the model name prefixed with
`openai/`:

```bash
export OPENAI_API_BASE=https://tamga.internal/v1
export OPENAI_API_KEY=<provider key>
aider --model openai/gpt-4o
```

The `.env` page lists `AIDER_OPENAI_API_BASE` as the base-URL option. The
Anthropic page read says nothing about a base URL, so aider's Anthropic path
through `/anthropic` is not verified. Sending `X-Tamga-Key` was not found in
the pages read (not verified), so aider is anonymous at Tamga.

What Tamga sees: every request aider makes, including the repository map and
the files it adds to the chat.

## GitHub Copilot

Sources read 2026-10-17:
[Language models in VS Code](https://code.visualstudio.com/docs/copilot/customization/language-models),
[Use your own API keys (GitHub Docs)](https://docs.github.com/en/copilot/how-tos/administer-copilot/manage-for-organization/use-your-own-api-keys).

Can it be pointed at Tamga: partly, in VS Code. VS Code has a Custom
Endpoint provider (Manage Language Models, Add Models, Custom Endpoint). It
is stored in `chatLanguageModels.json`, with `vendor: "customendpoint"`, an
`apiType` of `chat-completions`, `responses` or `messages`, an `apiKey`, and
per model a `url` and `requestHeaders`:

```json
[
  {
    "name": "Tamga (Anthropic)",
    "vendor": "customendpoint",
    "apiKey": "${input:providerKey}",
    "apiType": "messages",
    "models": [
      {
        "id": "claude-sonnet-4-6",
        "name": "Claude through Tamga",
        "url": "https://tamga.internal/anthropic/v1/messages",
        "toolCalling": true,
        "requestHeaders": { "X-Tamga-Key": "tk_your_key_here" }
      }
    ]
  }
]
```

For an OpenAI-style model use `"apiType": "chat-completions"` and
`"url": "https://tamga.internal/v1/chat/completions"`. Without an explicit
path VS Code appends the path for the API type.

What still goes to GitHub, per the vendor page: "BYOK applies to the chat
experience and utility tasks only." Inline suggestions, semantic search and
embedding-based features still use the Copilot service and need a GitHub
account. Inline completion therefore cannot be pointed at Tamga. The page
does not say what chat data, if any, GitHub still receives when a custom
model is used: not verified. Utility tasks (chat titles, commit messages)
use built-in models unless `chat.utilityModel` and `chat.utilitySmallModel`
name a custom one. Administrators of Copilot Business and Enterprise can
disable the bring-your-own-key policy, and can add organisation-wide custom
models; the GitHub page read did not say how an endpoint is set there for an
OpenAI-compatible provider, so that path is not verified. JetBrains and other
Copilot clients: not verified.

What Tamga sees: chat and agent requests to the custom model.

## Cursor

Sources read 2026-10-17:
[Cursor documentation, API keys](https://cursor.com/docs/settings/api-keys)
(the old address redirects to the docs index); forum threads on the base URL
override ([one](https://forum.cursor.com/t/problem-reaching-openai-error-on-a-local-model-with-overriden-base-url/3975),
[two](https://forum.cursor.com/t/connecting-local-ai-server-to-cursor-does-not-work/152940/3)).
The forum threads are user reports with replies from Cursor staff, not
official documentation.

Can it be pointed at Tamga: partly. Cursor Settings, Models, lets you add
your own keys for OpenAI, Anthropic, Google, Azure OpenAI and AWS Bedrock,
and has an Override OpenAI Base URL field. The documentation page read does
not mention the override field; it is described by the forum threads.

The vendor page says custom keys work only with chat models and that Tab
completion continues to use Cursor's own models. It also says "all requests
are routed through Cursor's servers for final prompt building", with the key
sent to Cursor's backend on every request.

What that requires of Tamga:

- The request goes from Cursor's servers to the base URL, not from the
  developer's machine. Tamga must therefore be reachable from the internet
  at a public HTTPS address. The forum replies from Cursor staff say that
  localhost and private network addresses do not work for this reason.
  Opening Tamga to the internet is a decision for you to make with its
  consequences: put `TAMGA_REQUIRE_KEY=true` on it, and note that Cursor
  offers no header field for a Tamga key (not verified), so the provider key
  is all Tamga can use to tell callers apart; restrict by source address to
  Cursor's published ranges if it publishes any (not verified).
- Cursor has already seen the full prompt, with the project context it built,
  before Tamga does. Tamga cannot keep the text away from Cursor. It can
  block or redact it only on its way to the model provider.

If sending the text to Cursor is itself the concern, Cursor is a client Tamga
does not help with.

## JetBrains AI Assistant

Source read 2026-10-17:
[Use custom models](https://www.jetbrains.com/help/ai-assistant/use-custom-models.html).
The AI completion page I tried (`ai-completion.html`) returned 404 and was
not read.

Can it be pointed at Tamga: partly. In Settings, Tools, AI Assistant,
Providers & API keys, choose OpenAI-compatible under Third-party AI
providers, enter the endpoint URL (`https://tamga.internal/v1`) and the key,
then Test Connection. The page names LiteLLM as an example endpoint. No
custom header field was described, so the client is anonymous at Tamga.

- Models from such a provider appear in AI Chat. For other features they must
  be assigned by hand in Models Assignment; until then only AI Chat uses
  them.
- AI completion (inline and next edit suggestions) is configured separately
  in the AI Completion section, with its own base URL, key and model. The
  model should support fill-in-the-middle.
- The page says agents cannot use local models, and that AI Assistant cannot
  call MCP tools with local models. It does not mention Junie or the Claude
  agent by name; whether either honours these settings, and whether requests
  go from the IDE directly or through JetBrains' servers, is not verified.
  A search found no documented base-URL setting for them.

What Tamga sees: AI Chat and, if configured, inline completion.

## Zed

Sources read 2026-10-17:
[LLM providers](https://zed.dev/docs/ai/llm-providers),
[Use API access](https://zed.dev/docs/ai/use-api-access),
[Edit prediction](https://zed.dev/docs/ai/edit-prediction).

Can it be pointed at Tamga: partly. For an OpenAI-compatible provider the
setting is `api_url` under `language_models.openai_compatible`, and the key
comes from an environment variable named after the provider id in upper case
plus `_API_KEY` (`tamga` gives `TAMGA_API_KEY`):

```json
{
  "language_models": {
    "openai_compatible": {
      "tamga": {
        "api_url": "https://tamga.internal/v1"
      }
    }
  }
}
```

The pages read give `ANTHROPIC_API_KEY` and `OPENAI_API_KEY` for the built-in
providers but show no `api_url` for them, so the Anthropic route is not
verified. No header setting was described, so the client is anonymous at
Tamga.

Edit prediction (inline completion) has a provider setting. The default is
Zed's own (`zed`); GitHub Copilot and Codestral are other choices and have no
URL setting. `open_ai_compatible_api` takes a URL, which must serve the
legacy completions API:

```json
{
  "edit_predictions": {
    "provider": "open_ai_compatible_api",
    "open_ai_compatible_api": {
      "api_url": "https://tamga.internal/v1/completions",
      "model": "<a base model that supports completions>",
      "prompt_format": "infer",
      "max_output_tokens": 512
    }
  }
}
```

What Tamga sees: agent and inline-assistant requests, and prediction
requests if the provider above is chosen. A prediction request is sent very
often; see the next sections.

## Windsurf

Source read 2026-10-17:
[Models](https://docs.devin.ai/desktop/cascade/models) (the old
docs.windsurf.com address redirects there). The page was fetched; it did not
describe a custom API key or base-URL option, and a web search found only
unofficial and contradictory reports. Not verified. Do not plan on Windsurf
being routable through Tamga.

## Clients that cannot be pointed at Tamga

For a client that cannot be pointed at Tamga, or for the parts marked as not
seen (Copilot and Cursor completions, JetBrains' own models, Windsurf):

- The [browser extension](browser-extension.md) does not help. It runs on
  three chat websites and does nothing in an editor.
- Tamga has no network-level control. A check at the company's web proxy over
  ICAP, so that traffic from an editor to a vendor is scanned by Tamga
  without the editor knowing, is planned and **not built**. There is no ICAP
  server in Tamga today, and traffic from these clients to their vendor is
  not seen. Even when it exists, a request that is encrypted end to end to
  the vendor can be read only if the web proxy terminates TLS.
- What you can do today is a policy decision outside Tamga: allow in the
  editor only the clients and settings in this page, and block the vendor
  endpoints of the others at the firewall, so that the clients that remain
  are the ones that go through Tamga. Tamga does not enforce or discover
  that.
- Tell developers what the proxy sees and what it does not. A table like the
  one at the top, adapted to your policy, is a good start.

## Inline completion: volume and latency

Inline completion sends a request on pauses in typing, often several a
second, each carrying a window of the file around the cursor and usually a
few other open files. Compared with chat:

- **Volume.** The requests are small but numerous. The default
  `rate_limit.max_requests_per_minute` is 60 per Tamga key
  ([sample policy](operations.md#sample-policy-yaml)), and an editor on
  autocomplete reaches that within a minute. A client that sends no Tamga key
  shares a bucket per client address. Give such clients their own limit or
  key, or raise the figure, and set `rate_limit.max_tokens_per_day` with
  completion in mind: it counts tokens, and completion contexts repeat
  mostly the same text. `action_on_exceed: BLOCK` answers 429, which the
  editor shows as a failed completion.
- **Latency.** Scanning adds under a millisecond for a short text and grows
  with its length (about 250 KB a second per core;
  [speed](agents.md#speed)). Text a request repeats from the one before is
  answered from memory for ten minutes, which suits completion, where the
  window moves a few characters between requests. The extra latency that
  matters is the network hop and, if `scan.classifier` is on
  ([inline classifier](operations.md#inline-classifier)), a call of about 15
  to 30 ms on CPU for new text, with `timeout_ms: 150` in the example. Keep
  the classifier off a completion route, or limit `scan.classifier.roles`,
  unless you have measured it. Measured completion latency through
  Tamga has not been recorded.
- **Body size.** `TAMGA_MAX_BODY_BYTES` defaults to 1 MiB, and the shipped
  policy's `body_limits` allow 1 MiB on the OpenAI route and 32 MiB on the
  Anthropic route. A chat request from an editor that attaches several
  files can exceed 1 MiB on `/v1`; Tamga then answers 413. Raise
  `body_limits.per_provider.openai.max_bytes` (the policy accepts at most
  64 MiB) if that happens.
- **Fields.** Tamga reads `messages`, `input`, `instructions` and `prompt`
  by role. Other request fields, such as the `suffix` of a fill-in-the-middle
  request, are read as request parameters, not as user text.
- **Choose where completion goes.** A reasonable arrangement is to route chat
  and agent traffic through Tamga and leave inline completion on a local or
  in-house model that never leaves the machine, instead of through the
  proxy. Which is right depends on where the completion model runs.

A response is scanned only if `output_rules` is on; for a streamed one, only
with `output_rules.streaming`. See
[Scanning streamed responses](operations.md#scanning-streamed-responses).
