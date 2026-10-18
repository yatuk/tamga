# MCP gateway

On the model route Tamga sees what an MCP tool returned only later, as a
tool result in the agent's next model request, and it never sees the
traffic between the agent and the MCP server itself
([agents](agents.md#the-agents-own-environment)). The MCP gateway puts Tamga
in that path too. Each MCP server you list is served by Tamga at
`/mcp/<name>`. You give the agent that URL in place of the server's, and
every message between the two goes through Tamga. Tamga judges each
message with the same scanners and the same policy it uses for tool
results on the model route.

> Status: the gateway is tested against a fake MCP server written in Go
> (`proxy/internal/proxy/mcp_gateway_test.go`) and the stdio wrapper
> against a fake stdio server (`proxy/cmd/tamga/mcpwrap_test.go`). No
> session of a real MCP client against a real MCP server through Tamga has
> been recorded yet.

## Which revision of MCP

The gateway follows the Streamable HTTP transport of the MCP specification,
revision **2026-07-28**, the current one
([transport](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http),
read on 2026-10-18). It also handles the older form of the same transport,
used by revisions 2025-03-26 to
[2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports),
which most servers and clients still speak:

| | 2026-07-28 | 2025-03-26 to 2025-11-25 | The gateway |
|---|---|---|---|
| POST with one JSON-RPC message, answered as JSON or as an SSE stream | yes | yes | Reads each message, in both answer types |
| Version and capabilities | `_meta` on every request, `MCP-Protocol-Version` header | `initialize` handshake | Passes through unchanged |
| Session | none | `Mcp-Session-Id`, ended with DELETE | Forwards the header and the DELETE |
| GET for a stream the server opens | removed (405) | yes | Forwards it and reads the stream like a POST stream |
| Resuming a stream | removed | `Last-Event-ID`, event ids | Forwards the header and keeps each event's id |
| `Mcp-Method`, `Mcp-Name`, `Mcp-Param-*` headers | required | not defined | Forwards them. Checks `Mcp-Method` and `Mcp-Name` against the body |
| JSON-RPC batches | no | 2025-03-26 only | Refuses them (`-32600`) |

The gateway does not negotiate anything itself. It forwards `initialize`,
`server/discover` and every other method, and reads the answers.

## Setting it up

List the servers in a YAML file and point `TAMGA_MCP_CONFIG` at it. The
proxy reads the file once, at startup. If the file does not load, the
proxy does not start.

```yaml
servers:
  - name: github                         # served at /mcp/github
    url: https://mcp.example.com/mcp     # the server's MCP endpoint
    tools: [search_issues, "get_*"]      # optional: the only tools the agent sees and may call
    headers_from_env:                    # optional: the server's credential, from the environment
      Authorization: TAMGA_MCP_GITHUB_AUTH
    tool_changes: warn                   # off | warn (default) | block
  - name: files
    url: http://127.0.0.1:8931/mcp       # a local stdio server behind tamga mcp-wrap, below
allowed_origins: []                      # web origins whose pages may call the gateway
```

| Field | Meaning |
|---|---|
| `name` | 1 to 63 characters: lower-case letters, digits, `-` and `_`. It is the path segment, and it names the server in events. |
| `url` | An absolute `http` or `https` URL, without credentials, query or fragment. The gateway does not follow redirects. |
| `tools` | The tools the agent is shown in `tools/list` and may call. `*` stands for any run of characters. If you leave it out, every tool is allowed. |
| `headers_from_env` | Headers sent to the server. Each value is read from the named environment variable at startup. The load fails if the variable is empty. Transport headers, `Mcp-*` headers and `X-Tamga-*` headers cannot be set this way. |
| `forward_authorization` | Off by default. When on, the agent's own `Authorization` header goes to the server, and the server's `WWW-Authenticate` comes back to the agent. You cannot turn it on together with an `Authorization` header in `headers_from_env`. |
| `tool_changes` | What happens when a tool's definition changes in the middle of a session ([below](#tool-definitions-that-change)). |
| `max_message_bytes` | The largest message read in either direction. The default is 4 MiB, the ceiling is 32 MiB. A larger request gets a 413. A larger answer is not passed on: the agent gets error `-32014`. |

The server list lives in its own file, not in the policy, for a reason.
People who are not the operator can change the policy through the API.
This file names hosts to send traffic to and environment variables to send
along with it, and those are the operator's to decide (the same reasoning
as for [pool endpoints](operations.md)). What happens to the things the
scanners find is still set in the policy.

The gateway sits behind the same `TAMGA_IP_ALLOWLIST` as the model routes.
It authenticates agents the same way too. A Tamga key in `X-Tamga-Key`
decides which organisation the traffic is counted under. Without a key the
traffic is anonymous, unless `TAMGA_REQUIRE_KEY` is on, in which case it is
refused with a 401. For clients that can only send a few header names, the
key is also read from `X-Api-Key`. Neither header is passed to the server.

## Clients

### Claude Code

```bash
claude mcp add --transport http github https://tamga.internal/mcp/github \
  --header "X-Tamga-Key: tk_your_key_here"
```

or in `.mcp.json`:

```json
{
  "mcpServers": {
    "github": {
      "type": "http",
      "url": "https://tamga.internal/mcp/github",
      "headers": { "X-Tamga-Key": "${TAMGA_KEY}" }
    }
  }
}
```

Claude Code shows the server's tools to the model as
`mcp__<name>__<tool>`. If you use the same name in Claude Code as in the
gateway file, one `output_rules.tool_destinations` entry such as
`"mcp__github__*"` covers a tool call on both paths: in the model's answer
on the model route, and in the `tools/call` at the gateway
([below](#what-goes-to-the-server)).

### Claude Desktop

In Claude Desktop (and claude.ai) you add a remote server as a custom
connector: **Customize > Connectors > Add custom connector**, with the
gateway URL `https://tamga.example.com/mcp/github`. Choose **No sign-in**
for authentication, and under **Request headers** add `x-api-key` with your
Tamga key as the value. The gateway reads the Tamga key from that header.
Claude's documentation on
[custom connectors](https://claude.com/docs/connectors/custom/add-unlisted)
(read on 2026-10-18) says three things that matter here:

- Request headers are in beta and only some organisations have them.
- Only header names that Anthropic has approved are sent. `X-Tamga-Key`
  is not on the list it offers, which is why the gateway also accepts
  `X-Api-Key`.
- A connector added by URL is reached over the internet, so the gateway
  has to be reachable from the internet, or through the MCP tunnels that
  page describes.

If your Tamga is internal only, run the agent in Claude Code, which
connects from your machine.

### An SDK: the MCP Python SDK

The headers go on the HTTP client that you hand to the transport. The
example follows the SDK's
[transport documentation](https://py.sdk.modelcontextprotocol.io/client/transports/)
as read on 2026-10-18. It has not been run against the gateway.

```python
import os
import httpx2
from mcp import Client
from mcp.client.streamable_http import streamable_http_client

async def main():
    async with httpx2.AsyncClient(
        headers={"X-Tamga-Key": os.environ["TAMGA_KEY"]},
        timeout=httpx2.Timeout(30.0, read=300.0),
    ) as http:
        transport = streamable_http_client("https://tamga.internal/mcp/github", http_client=http)
        async with Client(transport) as client:
            tools = await client.list_tools()
            result = await client.call_tool("search_issues", {"query": "label:bug"})
```

## What comes back to the agent

The gateway recognises a message by its shape, not by the request it
answers. A response that arrives on a resumed stream is read the same way
as one that answers the POST it came with.

| Message | What is read | Read as |
|---|---|---|
| `tools/list` result | Each tool whole: name, title, description, input and output schemas, annotations | `tool_definition`, as a tool definition in a model request |
| `tools/call` result | Each `text` block, the `text` of an embedded resource, the title and description of a resource link, and `structuredContent` | `tool`, as a tool result |
| `resources/read` result | The `text` of each item in `contents` | `tool` |
| `prompts/get` result | The description and each message's text | `tool` |
| `initialize` result | `instructions`, which many clients add to the model's context | `tool` |

What is read as `tool` is read the way a tool result is read on the model
route. A result that is JSON is
[read decoded too](agents.md#tool-results-that-are-json), and
`scan.strip_scope: field` works as it does there. The policy acts on these
texts as it acts on a model request:

- **`STRIP`**: the text is replaced with
  `[Content removed by Tamga security policy.]` inside an otherwise
  unchanged JSON-RPC response, with the same id. A poisoned tool
  description becomes the placeholder, and the tool stays in the list. If
  `structuredContent` was stripped whole, the field is dropped and the
  placeholder stands in the text content. With `strip_scope: field`, only
  the stripped strings are replaced, and the field stays JSON.
- **`REDACT`**: the value is masked in place, as on the model route.
- **`BLOCK`** (and `ROUTE`, which the gateway cannot carry out): the
  response is replaced with a JSON-RPC error that has the same id.

A typical policy for this:

```yaml
rules:
  injection_detection:        # tool results, resources, prompts: strip
    action: STRIP
    sensitivity: low
    applies_to: [tool]
  injection:                  # a poisoned tool description: block the list
    action: BLOCK
    sensitivity: low
    applies_to: [tool_definition, user, system]
```

Errors from the server, notifications, and anything the server sends that
is not a result are passed on without being read. A message the gateway
cannot parse as a JSON-RPC object is not passed on at all.

### The errors the gateway returns

The codes and messages are fixed, and nothing the agent, the server or a
scanner wrote is in them. `data.tamga_request_id` points to the event.
`data.kinds` names the kinds of finding, never the text that matched. A
proxy key is told the type of a named finding and not its name.
`data.reason` is set when a REDACT or a STRIP could not be done and the
request was blocked instead.

| Code | Message | When |
|---|---|---|
| `-32010` | Blocked by Tamga security policy | A BLOCK verdict, in either direction |
| `-32011` | Tool not allowed by the Tamga MCP gateway | A `tools/call` for a tool that is not in `tools` |
| `-32012` | Tool definition changed during the session | `tool_changes: block`, and the tool's definition changed |
| `-32013` | Tamga could not complete the security scan | The scan failed, and `scan.on_error` is not `pass` |
| `-32014` | Message larger than the Tamga MCP gateway accepts | Over `max_message_bytes` |
| `-32015` | The MCP server could not be reached or sent a message Tamga cannot read | Connection failure (HTTP 502) |
| `-32016` | (the key error, as on the model route) | Missing or invalid Tamga key (HTTP 401), or a refused `Origin` (HTTP 403) |
| `-32020` | Header mismatch | `Mcp-Method` or `Mcp-Name` does not match the body (HTTP 400) |
| `-32600`, `-32700` | | A batch, a `tools/call` without an id, a body that is not one JSON object (HTTP 400) |

A refusal of a request that never reached the server is answered with HTTP
200 and the JSON-RPC error, as a server answers a request it refuses. A
failed scan stops a message unless `scan.on_error` is `pass`. If the key is
not set, MCP traffic counts as agent traffic and fails closed.

## What goes to the server

The arguments of a `tools/call` are read as the arguments of a tool call,
under the `assistant` role, which is how the model route reads them in a
model request:

- **Secrets and personal data.** A finding under a BLOCK rule stops the
  call: the server never receives it, and the agent gets `-32010`. A
  finding under a REDACT rule is masked in the arguments, for example
  `[email_REDACTED]`, and the call goes on. If the client mirrors an
  argument into an `Mcp-Param-*` header and something in the arguments was
  masked, the call is blocked instead. The header would carry the original
  value, and the server would refuse a header that no longer matches the
  body. A STRIP verdict on arguments is a block, because an argument
  cannot be replaced with a placeholder.
- **`output_rules.tool_destinations`.** Each call is held to the
  [list of where each tool may send data](agents.md#limit-where-tools-may-send-data).
  An entry may name the tool as the server does (`send_email`) or as
  Claude Code shows it to the model (`mcp__<name>__send_email`). A place
  under `block_on` stops the call. Under `redact_on` the address is masked
  and the call goes on with nowhere to send to.
- **The tools list.** A call to a tool that is not in `tools` is refused
  with `-32011` and does not reach the server.

Only `tools/call` arguments are read. Other parameters are not, and that
includes the answers a client sends back for an elicitation.

## Tool definitions that change

A server can list a harmless tool, and once the user has approved it,
change its description or its schema to something else: a rug pull. With
`tool_changes: warn`, the default, the gateway remembers each tool's
definition the first time the tool is listed in a session: its title,
description, input and output schemas and annotations. A later listing
that differs is recorded as a finding `mcp_tool_change` /
`tool_definition_changed`, and the action is `WARN`. The list itself
passes unchanged. With `tool_changes: block`, the changed tool is also
taken out of the list, and a `tools/call` for it gets `-32012` for the rest
of the session. The first definition stays the reference. `off` turns the
check off.

A session is a combination of three things: the server, the caller (its
Tamga key, or its address if it has no key), and the `Mcp-Session-Id` of
the older revisions. A 2026-07-28 client has no session id, so its session
is all of its use of that server. The gateway keeps up to 10,000 sessions,
for 24 hours after each one's last listing, and up to 4,096 tools per
session. A restart forgets them, and so does each replica of a scaled-out
proxy on its own.

## Authorization

MCP's
[authorization](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)
is OAuth 2.1. The MCP server is the resource server, and a token must be
issued for that server (its audience) and must not be passed on to anyone
else. The gateway stands between the two, and it does the following:

- The agent authenticates to Tamga with its Tamga key, as on the model
  route.
- The server's credential is the operator's. It is set in
  `headers_from_env`, for example a token that was issued to Tamga as the
  client. That is the setup that matches the specification: the token the
  server receives was issued for it, and the agent never holds it.
- `forward_authorization: true` passes the agent's own `Authorization`
  header through, along with the server's `WWW-Authenticate` challenge.
  This works for a client that you configure with a fixed bearer token for
  that server. It does not make OAuth discovery work through the gateway.
  An MCP client checks that the protected resource metadata names the URL
  it connected to, and that URL is Tamga's, not the server's. The gateway
  does not serve `/.well-known/oauth-protected-resource`, and it does not
  issue or check OAuth tokens.

## Local stdio servers: `tamga mcp-wrap`

Servers that use the stdio transport run as a child process of the client,
so no network gateway can sit in front of them. `tamga mcp-wrap` serves one
such server over Streamable HTTP, and the gateway can then point at it:

```bash
tamga mcp-wrap -listen 127.0.0.1:8931 -- npx -y your-stdio-mcp-server
```

and in the gateway file: `url: http://127.0.0.1:8931/mcp`.

It is a small bridge. Each POST carries one message. A request is written
to the child's stdin with an id of the bridge's own, so that two clients
cannot get each other's answers, and the response comes back as one JSON
object with the client's id put back in place. A notification is answered
with 202. If the HTTP client goes away first, the child is sent
`notifications/cancelled`. There is no GET stream and no session. Anything
the child sends that does not answer a request is dropped and counted on
stderr: progress and log notifications, and in the 2025 revisions the
server's own requests, such as sampling. The bridge listens on loopback
only, unless you pass `-allow-remote`. It refuses requests from web pages
(`Origin`), and it has no authentication of its own. Tamga in front of it
does the authentication.

A stdio server that expects exactly one `initialize` handshake can fail
when two clients share the bridge, because each client sends its own
handshake. Run one bridge per agent in that case.

## What is recorded

Each `tools/call` is recorded as an event, the same as a request on the
model route. Any other exchange is recorded only when something was found
or done. The event has the provider `mcp`, the endpoint
`/mcp/<name> <method> <tool>`, the verdict, the findings with the matches
the scanners masked, the organisation of the key, and the policy name and
version. It holds no text from the messages. Rule `notify` URLs are told,
as on the model route. Each exchange also writes one log line
(`mcp gateway exchange`) with the method, the tool, the action and the
counts.

## What is not covered

- **The `subscriptions/listen` stream and the notifications on it** are
  passed through. They carry no text for a model to read.
- **Server-to-client requests** are passed through without being read.
  That covers sampling, elicitation and roots: as requests on a stream in
  the 2025 revisions, and as `inputRequests` inside an `input_required`
  result in 2026-07-28. A sampling request's messages are meant for the
  client's model. If that model is behind Tamga's model route, they are
  read there.
- **`resources/list`, `resources/templates/list` and `prompts/list`** pass
  without being read. Their descriptions are usually shown to the user and
  not to the model.
- **Progress and log notifications** (`notifications/progress`,
  `notifications/message`) pass without being read.
- **Images, audio and blob resources** are not read, as on the model route.
- **The deprecated HTTP+SSE transport of 2024-11-05**, with a separate SSE
  endpoint and POST endpoint, is not supported.
- **OAuth through the gateway**, as described under
  [Authorization](#authorization).
- **A JSON-RPC batch** is refused, not split into its messages.
- **Rate limits**: `rate_limit` in the policy does not apply to the
  gateway.
- **The console** shows MCP events under the provider name `mcp`. It has no
  page of its own for the gateway.
