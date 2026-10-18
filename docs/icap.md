# Tamga behind a company web proxy (ICAP)

Tamga's proxy sees a request only when the client is pointed at it. A
browser without the [extension](browser-extension.md), a desktop app, or an
editor assistant that calls its vendor directly is never pointed at it. In
many companies all of that traffic already goes through a web proxy (Squid,
or a commercial secure web gateway) that terminates TLS. Such a proxy can ask
another server about each request over ICAP (RFC 3507) and do what it is
told. Tamga can be that server.

The web proxy sends Tamga the request it is about to forward. Tamga reads
the ones that go to an AI service it knows, runs the same scanners and the
same policy as on its proxy path, and answers: send it on unchanged, send it
on with the body rewritten (REDACT, STRIP), or give the user this 403
instead (BLOCK). The request itself never passes through Tamga; the web
proxy forwards it.

> Status: the ICAP server and the decisions are covered by tests in which
> the test is the ICAP client (`proxy/internal/icap`, and
> `proxy/internal/proxy/icap_test.go`). **No Squid, no commercial gateway
> and no live vendor traffic have been run against it.** The Squid
> configuration below was written from Squid's documentation and was not
> run. The request formats of the web applications (chatgpt.com, claude.ai,
> gemini.google.com) were not verified at all: see
> [Format drift](#format-drift).

## Turning it on

The ICAP server is off unless `TAMGA_ICAP_ADDR` is set. It has its own
port; the standard ICAP port is 1344.

| Variable | Default | Meaning |
|---|---|---|
| `TAMGA_ICAP_ADDR` | empty (off) | Address to listen on, e.g. `:1344` or `10.0.5.20:1344` |
| `TAMGA_ICAP_ALLOWED_CLIENTS` | empty: loopback only | Comma-separated IPs and CIDR ranges of the web proxies that may connect. A connection from anywhere else is closed without a reply. A bad entry stops the proxy at startup |
| `TAMGA_ICAP_MAX_CONNECTIONS` | `256` | Connections served at once, and the `Max-Connections` Tamga tells the web proxy. One more is answered `503` and closed |
| `TAMGA_MAX_BODY_BYTES` and the policy's `body_limits` | 1 MiB | The largest request body read, per provider, as on the proxy path |
| `TAMGA_REQUIRE_KEY` | off | When on, the web proxy must send a Tamga key (below), or every request it asks about is refused |

The service path is `/reqmod`: `icap://<tamga-host>:1344/reqmod`.

ICAP has no authentication and no encryption of its own, and Tamga's ICAP
server does not speak ICAP over TLS. Between the web proxy and Tamga travel
the users' prompts and the provider credentials in their requests
(`Authorization`, `x-api-key`), in the clear. Run Tamga next to the web
proxy, on the same host or on a network segment only the two share, and set
`TAMGA_ICAP_ALLOWED_CLIENTS` to the web proxy's address. Tamga does not log
or store request headers.

The Compose files do not publish the ICAP port. Add a port mapping for it if
the web proxy runs outside the Compose network.

### A Tamga key from the web proxy

Optional. A proxy key sent as the ICAP header `X-Tamga-Key` is verified as
on the proxy path, and the requests are recorded under that key's
organisation. In Squid that is `adaptation_meta`. Without a key the requests
are recorded under the default organisation (`TAMGA_ORG_ID`).

## Squid

Not run; written from Squid's documentation. Check each directive against
the documentation of your Squid version.

```squid
# TLS interception is required: see the next section.
http_port 3128 ssl-bump tls-cert=/etc/squid/company-ca.pem generate-host-certificates=on

acl ai_tls ssl::server_name api.openai.com api.anthropic.com \
    generativelanguage.googleapis.com api.mistral.ai .openai.azure.com \
    chatgpt.com chat.openai.com claude.ai gemini.google.com
acl step1 at_step SslBump1
ssl_bump peek step1
ssl_bump bump ai_tls
ssl_bump splice all

icap_enable on
icap_preview_enable on
icap_persistent_connections on
adaptation_send_client_ip on
adaptation_send_username on
icap_client_username_header X-Client-Username
icap_client_username_encode off

# bypass=off: when Tamga cannot be reached, requests to these hosts fail
# instead of going out unchecked.
icap_service tamga_req reqmod_precache icap://127.0.0.1:1344/reqmod bypass=off

acl ai_hosts dstdomain api.openai.com api.anthropic.com \
    generativelanguage.googleapis.com api.mistral.ai .openai.azure.com \
    chatgpt.com chat.openai.com claude.ai gemini.google.com
acl post method POST
adaptation_access tamga_req allow ai_hosts post
adaptation_access tamga_req deny all

# Optional: a Tamga proxy key, so requests are recorded under its
# organisation.
# adaptation_meta X-Tamga-Key "tk_..."
```

Notes:

- `adaptation_access` decides what Squid sends at all. Keep it to the AI
  hosts: everything else would only cost a round trip, since Tamga answers
  204 to anything it does not recognise. Tamga checks the host and the path
  itself, so a broader list is not a hole, only a cost.
- Tamga's OPTIONS answer says `Preview: 0`, so Squid sends the request
  headers first. A request Tamga does not read gets a `204` before its body
  is sent; one it reads gets `100 Continue` and then the body.
- The user name Squid sends in `X-Client-Username` is recorded as the
  request's user. Tamga cannot check it; it is the web proxy's word. A
  gateway that sends `X-Authenticated-User` (base64) instead is read too.
- A commercial secure web gateway is configured the same way in its own
  terms: an ICAP REQMOD service at `icap://<tamga-host>:1344/reqmod`, sent
  only the AI hosts, failing closed. None was tried.

## What TLS interception requires

The request bodies Tamga must read are inside TLS to the vendor. Only a web
proxy that terminates that TLS (Squid's `ssl_bump bump`) has them to send.
That means:

- The company's own certification authority signs the certificates the web
  proxy presents for the vendor's hosts, and every client trusts that CA:
  the operating system store, and the stores of runtimes that keep their own
  (Firefox, Java, Node.js with `NODE_EXTRA_CA_CERTS`, Python with
  `REQUESTS_CA_BUNDLE` or `SSL_CERT_FILE`, some Electron apps).
- An app that pins the vendor's certificate refuses the web proxy's and
  stops working. Such apps have to be spliced (passed through unread), which
  means Tamga does not see them, or blocked.
- An app that does not use the system proxy settings bypasses the web proxy
  unless the network forces traffic through it.
- Interception is itself a decision with legal and works-council sides in
  many places (KVKK and employment law in Turkey, GDPR in the EU). Tamga does
  not make it for you.

## What is covered

Only `POST` requests to these endpoints are read. Everything else passes
unread with a `204`, including other requests to the same hosts.

| Host | Paths | Read as | Can be rewritten |
|---|---|---|---|
| `api.openai.com` | `/v1/chat/completions`, `/v1/completions`, `/v1/responses` | OpenAI request, by the extractor the proxy path uses | Yes |
| `api.anthropic.com` | `/v1/messages`, `/v1/messages/count_tokens` | Anthropic Messages | Yes |
| `generativelanguage.googleapis.com` | `/v1`, `/v1beta`, `/v1alpha` `…/models/<m>:generateContent`, `:streamGenerateContent`, `:countTokens` | Gemini | Yes |
| `api.mistral.ai` | `/v1/chat/completions`, `/v1/agents/completions` | OpenAI shape | Yes |
| `<resource>.openai.azure.com` | `/openai/deployments/<d>/chat/completions` and `/completions`, `/openai/v1/chat/completions`, `/openai/v1/responses`, `/openai/responses` | OpenAI shape | Yes |
| `chatgpt.com`, `chat.openai.com` | `/backend-api/conversation`, `/backend-api/f/conversation` | Every string in the JSON body | No: REDACT and STRIP block |
| `claude.ai` | `/api/organizations/<id>/chat_conversations/<id>/completion` and `/retry_completion` | Every string in the JSON body | No |
| `gemini.google.com` | `/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate` | Every string in the form field `f.req`, JSON inside strings included | No |

What each verdict does:

| Policy action | ICAP reply | What the user's client gets |
|---|---|---|
| `PASS`, `WARN`, `LOG` | `204 No Content` | The vendor's answer, as without Tamga |
| `REDACT`, `STRIP` | `200` with the rewritten request (`Content-Length` set, `Transfer-Encoding` removed) | The vendor's answer to the rewritten request |
| `BLOCK` | `200` with an HTTP `403` response | `{"error":{"code":"tamga_blocked","message":"Request blocked by Tamga security policy","type":"security_violation","request_id":"…","findings_count":1,"finding_types":["pii"]}}`, or a short HTML page when the client asks for `text/html` only. Finding types, never the matched text or the categories (a category can name an entry on your watch list) |
| `ROUTE` | as `BLOCK` | The web proxy cannot send a request to an internal pool. The response carries `X-Tamga-Route-Fallback: block:icap` |

And when there is no verdict, the request is not let through unread:

| Case | HTTP response in the ICAP reply |
|---|---|
| A REDACT or STRIP that cannot be carried out (a web application's body, an API body the extractor does not know, a malformed body read with `on_malformed: raw_scan`) | `403`, with `X-Tamga-Redact-Fallback` or `X-Tamga-Strip-Fallback`: `block:unverified_format`, `block:unknown_shape`, `block:malformed_json`, or the proxy path's own reasons |
| The body is over the limit | `413` `body_too_large` |
| `Content-Encoding` other than `gzip` or `identity` | `415` `tamga_unreadable_body`. A gzip body is decoded, scanned, and sent on decoded when it is rewritten |
| Ambiguous JSON (duplicate keys, invalid UTF-8) and `scan.on_malformed: block` (the default) | `400` `tamga_invalid_json` |
| The scan did not complete, the classifier or topic model was needed and not there, no policy | `503` `tamga_scan_unavailable` |
| No free scan slot within 5 seconds | `503` `tamga_scan_busy` |
| The web proxy's Tamga key was refused, or none was sent with `TAMGA_REQUIRE_KEY` on | `503` `tamga_icap_key` |

ICAP-level errors (a malformed message: `400`; an unknown service: `404`;
`RESPMOD`: `405`; an unknown method: `501`; another ICAP version: `505`; a
message that stops arriving: `408`) are answered as ICAP statuses. What Squid
then does depends on `bypass`.

Each request Tamga reads and judges is recorded like a request on the proxy
path: an event on the bus, so a row in the request log, the live feed and
the alerts, with the policy's name and version. The provider is the vendor
the request was going to (`openai`, `anthropic`, `gemini`, `mistral`,
`azure`); the endpoint is `icap ` and the route's name from the table, with
the identifiers in the path left out (for example
`icap claude.ai/api/organizations/*/chat_conversations/*/completion`); the
user is the one the web proxy named. No text and no body for the analyzer
is on the event. A request refused without a verdict (the cases in the
table above) is logged and not recorded.

## What is not covered

- **Answers.** Tamga implements REQMOD only; RESPMOD is answered `405`. The
  vendor's answer reaches the user unscanned, so `output_rules`, the canary
  and the vault do not apply over ICAP.
- **What the proxy path does around the scan**: rate limits, the daily
  token and cost caps, the response cache, the vault's placeholders (a
  REDACT over ICAP writes the plain markers), canary tokens, routing to an
  internal pool.
- **Traffic the web proxy does not intercept**: certificate-pinning apps,
  apps that ignore the system proxy, HTTP/3 (QUIC over UDP 443, which
  Chrome uses with Google's hosts unless UDP 443 is blocked so that it
  falls back to TCP), and anything spliced.
- **WebSockets and other non-POST traffic**: voice modes, the OpenAI
  Realtime API, and any web app that sends its prompts over a WebSocket.
- **Uploads**: files a web app uploads to its own upload endpoint before
  the message refers to them are not on the paths above and are not read.
- **Other vendors and back-ends**: Amazon Bedrock (its requests are signed
  with SigV4 over the body, so a rewrite would break the signature), Google
  Vertex AI (`aiplatform.googleapis.com`), GitHub Copilot, Cursor's and
  Windsurf's own servers, Microsoft Copilot, and any other service not in
  the table. Requests to them pass with a `204`.
- **A request body streamed in pieces** is read as a whole before the
  verdict, up to the body limit. Nothing goes on until the verdict.

## Format drift

The API shapes are the ones Tamga's proxy path reads, documented by each
vendor and covered by the extractor's tests. The vendors add to them: a
content block of a type the extractor does not know is still scanned, but a
new place for text elsewhere in the body could be missed until the
extractor learns it.

The web applications' back-ends are another matter. **None of the three web
formats was verified**: OpenAI, Anthropic and Google do not document the
requests chatgpt.com, claude.ai and gemini.google.com send, Tamga's
extension reads the pages and not their network requests, and no traffic
was captured to check them. The paths in the table are what those
applications were believed to use when this was written, and they change
without notice. Because the format is unknown:

- the body is read generically: every string in it, so identifiers,
  settings and model names are scanned along with the prompt, and can cause
  a false positive;
- nothing is ever rewritten: a REDACT or a STRIP is a BLOCK;
- a request to a path that has moved is not recognised and passes unread
  with a `204`. Nothing tells you this happened, except that the console
  stops showing requests from that endpoint. Watch for that, and test the
  web applications through your web proxy after each change you hear of.
