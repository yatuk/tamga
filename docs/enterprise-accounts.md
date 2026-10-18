# Business accounts only

A company that pays for ChatGPT Enterprise or Team, Claude for Work, Gemini
in Google Workspace or a business Copilot wants its people to use that
account and not a personal one. The prompts are the same; the terms they are
held under, who can read them and who can delete them are not.

Tamga can hold this line in three places:

| Path | What Tamga sees | What it does | Status |
|---|---|---|---|
| API proxy | Requests your applications send to a provider through Tamga | Checks the provider account each request uses, and can send the organisation's own key in place of whatever the caller sent | Built: `provider_accounts` in the policy |
| Network (ICAP) | Every HTTP request of the network, from a TLS-inspecting gateway | Sets the tenant-restriction header the vendor documents; the vendor refuses other accounts | The header logic is built (`proxy/internal/tenantheaders`); the ICAP service that calls it is not part of this change |
| Browser extension | Requests the browser makes to chatgpt.com and claude.ai | Sets the same vendor header from a managed setting | Built: `allowedWorkspaces` |

The vendor does the refusing on the network and extension paths: Tamga only
says which tenants are allowed, in the form the vendor reads. Nothing guesses
from the page which account is signed in.

## What each vendor offers

Read on 2026-10-18 in the vendors' own documentation. "Verified" means the
vendor page was read and says what the row says. None of the headers has been
tried against the live service from here.

| Vendor and product | Mechanism | Where Tamga can enforce it | Verified |
|---|---|---|---|
| Microsoft Entra ID (Microsoft 365 Copilot and every other app that signs in with Entra) | Tenant restrictions v2. A proxy sets `sec-Restrict-Tenant-Access-Policy: <TenantId>:<policyGuid>` on `login.live.com`, `login.microsoft.com`, `login.microsoftonline.com` and `login.windows.net`. Through a proxy this covers the authentication plane only; the data plane needs Global Secure Access or Windows device policy. Without the header nothing is enforced. Needs TLS interception of the sign-in hosts, which Microsoft supports for this purpose. [1] | Network (ICAP) | Yes: header name, value, hosts and the auth-plane limit [1] |
| OpenAI ChatGPT Enterprise | Corporate network controls. A proxy sets `ChatGPT-Allowed-Workspace-Id` to one or more workspace ids (UUIDs), comma-separated with no space. Other workspaces, personal ones included, are filtered out; a user left with none gets 403. [2] | Network (ICAP), extension (chatgpt.com) | Partly. The help article answered 403 to the fetcher; the header name, format and behaviour are from the article's own text as the search index shows it [2]. The hosts to set it on are not confirmed; Tamga sets it on `chatgpt.com` and its subdomains |
| OpenAI API | `OpenAI-Organization` and `OpenAI-Project` request headers choose the organisation and project a request is billed to; the response carries `openai-organization`. [3] | API proxy: `organizations`, `projects`, `keys`, `replace_key_env` | Yes [3] |
| Anthropic Claude (claude.ai Enterprise, Console organisations) | Tenant restrictions. A proxy sets `anthropic-allowed-org-ids` to organisation UUIDs, comma-separated, no spaces. Longer lists use `;n=K` on the first header and `anthropic-allowed-org-ids1` … `K-1`; at most 10 lines and 500 ids. Hosts: `claude.ai`, `api.anthropic.com`, `claude.com`, `anthropic.com`. Covers API keys and OAuth tokens too. Refusal is 403 `permission_error` / `tenant_restriction_violation`; a malformed header set is 400. The proxy must overwrite, not add. [4] | Network (ICAP), extension (claude.ai) | Yes [4]. Where `;n=K` goes is read as "at the end of the first header's value"; not tried |
| Anthropic API | `anthropic-workspace-id` request header picks the workspace (required with a multi-workspace key). Responses carry `anthropic-organization-id` and `anthropic-workspace-id`. [5] | API proxy: `workspaces`, `keys`, `replace_key_env` | Yes [5] |
| Google Workspace (Gemini app, Gemini in Workspace) | A proxy sets `X-GoogApps-Allowed-Domains` on requests to `*.google.com`, a comma-separated list of allowed domains; the keyword `consumer_accounts` allows personal accounts, leaving it out blocks them. Blocks sign-in to services that need one; anonymous use is not necessarily blocked. Needs TLS interception. [6] Chrome sets the same header itself under the `AllowedDomainsForApps` policy [7]. | Network (ICAP). For the browser, the Chrome policy, not the extension | Partly. The header is verified [6]. Google's article does not name Gemini, so whether gemini.google.com honours it is not verified |
| Gemini API | API keys belong to a Google Cloud project. No vendor page read here names a request header that identifies the project of an API-key request. | API proxy: `keys`, `replace_key_env` | Not verified |
| GitHub (Copilot Business and Enterprise) | Enterprise access restrictions, Enterprise Managed Users only: a proxy sets `sec-GitHub-allowed-enterprise` to an enterprise id, or up to 20 comma-separated ids. [8][9] Copilot also routes by plan: business and enterprise traffic goes to their own `*.githubcopilot.com` hosts, so a gateway can block `*.individual.githubcopilot.com` to stop personal plans. [10] | Network (ICAP) for the header. Blocking the individual host is a plain URL rule in the gateway | Partly. Header and limit from GitHub's changelog [8][9]; the docs pages (hosts `api.github.com` and `*.githubcopilot.com` for the header, the individual host name) did not resolve from here and are from their search excerpts [10] |

Sources, all read on 2026-10-18:

1. Microsoft Learn, "Set up tenant restrictions v2",
   <https://learn.microsoft.com/en-us/entra/external-id/tenant-restrictions-v2> (page updated 2026-06-11).
2. OpenAI Help Center, "Corporate network controls in ChatGPT Enterprise",
   <https://help.openai.com/en/articles/20001323-corporate-network-controls-in-chatgpt-enterprise>.
3. OpenAI API reference, overview, <https://developers.openai.com/api/reference/overview>.
4. Claude Help Center, "Enforce network-level access control with tenant restrictions",
   <https://support.claude.com/en/articles/13198485-enforce-network-level-access-control-with-tenant-restrictions>.
5. Claude API docs, "API overview", <https://platform.claude.com/docs/en/api/overview>.
6. Google Workspace Admin Help, "Block access to consumer accounts",
   <https://knowledge.workspace.google.com/admin/security/block-access-to-consumer-accounts>.
7. Microsoft Learn, Edge: block access to consumer accounts,
   <https://learn.microsoft.com/en-us/deployedge/edge-learnmore-block-access-consumer-accounts>
   (search excerpt: the policy is inherited from Chromium and adds the header on all google.com requests).
8. GitHub changelog, 2025-09-15, "Enterprise access restrictions with corporate proxies is now generally available",
   <https://github.blog/changelog/2025-09-15-enterprise-access-restrictions-with-corporate-proxies-is-now-generally-available/>.
9. GitHub changelog, 2025-10-06, "Enterprise access restrictions now supports multiple enterprises",
   <https://github.blog/changelog/2025-10-06-enterprise-access-restrictions-now-supports-multiple-enterprises/>.
10. GitHub Docs, "Restricting access to GitHub.com using a corporate proxy" and
    "Managing GitHub Copilot access to your enterprise's network",
    <https://docs.github.com/en/enterprise-cloud@latest/admin/configuring-settings/hardening-security-for-your-enterprise/restricting-access-to-githubcom-using-a-corporate-proxy>,
    <https://docs.github.com/en/copilot/how-tos/administer-copilot/manage-for-enterprise/manage-access/manage-network-access>.

### Reading the workspace from the page

None of the vendors documents a way for a page script or an extension to
read which workspace or plan is signed in. What a page shows (a workspace
name in a menu) or what its own internal endpoints return is not an
interface: it changes without notice, and an extension that calls a site's
internal endpoints on its own does something the site did not offer. The
extension therefore has no adapter that reads the workspace. It uses what the
vendors do document, the header, and leaves the decision to them.

## API path: `provider_accounts`

Off unless the section is present and `enabled: true`. A route without an
entry is not checked; to close a route, use `providers.allowed`.

```yaml
provider_accounts:
  enabled: true
  action: BLOCK                 # BLOCK (default) or LOG
  providers:
    openai:
      organizations: [org-EXAMPLE]
      projects: [proj_EXAMPLE]
      keys: ["sha256:<64 hex>"] # the keys that may be sent
    anthropic:
      workspaces: [wrkspc_EXAMPLE]
      replace_key_env: ANTHROPIC_API_KEY
```

| Field | Route | The request must |
|---|---|---|
| `keys` | any but bedrock | send only provider keys whose fingerprint is listed: every one in `Authorization: Bearer`, `x-api-key`, `api-key`, `x-goog-api-key` and the `key` query parameter |
| `organizations` | openai | send `OpenAI-Organization` once, with a listed value |
| `projects` | openai | send `OpenAI-Project` once, with a listed value |
| `workspaces` | anthropic | send `anthropic-workspace-id` once, with a listed value |
| `replace_key_env` | any but bedrock | carry a verified Tamga key; its provider credentials are dropped and the organisation's key is sent |

A header list makes the header required: without it the provider uses the
key's default account, which the proxy cannot see. The provider checks that
the key may act for the account the header names, so a listed organisation
with a personal key from elsewhere is refused by the provider.

A request that fails gets 403:

```json
{"error": {"type": "provider_account_not_allowed", "reason": "organization_not_allowed",
           "message": "...", "request_id": "..."}}
```

`reason` is one of `key_missing`, `key_not_allowed`, `organization_missing`,
`organization_not_allowed`, `project_missing`, `project_not_allowed`,
`workspace_missing`, `workspace_not_allowed`. Nothing is sent to the provider.
With `action: LOG` the request is forwarded and the proxy logs
`provider_account_not_allowed` with the reason. Use LOG to find out which
organisations and keys are in use before turning BLOCK on.

### Key fingerprints

Keys are never written into the policy. A fingerprint is `sha256:` and the
lower-case hex SHA-256 of the key exactly as sent:

```bash
printf %s "$OPENAI_API_KEY" | sha256sum | awk '{print "sha256:" $1}'
```

The policy refuses to load a `keys` entry that is not in this form, so a key
pasted by mistake is not stored.

### Sending the organisation's key

With `replace_key_env`, the proxy reads the organisation's key from that
environment variable and sends it in the header the route's provider reads:
`Authorization: Bearer` (openai, mistral, local), `x-api-key` (anthropic),
`x-goog-api-key` (gemini), `api-key` (azure). Every credential the caller sent
is removed, the `key` query parameter included. Personal keys never leave the
proxy, and callers do not need a provider key at all.

- Only a request with a verified Tamga key (`X-Tamga-Key`) is served; one
  without gets 401 `tamga_key_required`. Otherwise anyone who can reach the
  proxy could use the organisation's account.
- The variable is judged as a pool endpoint's `api_key_env` is: a name ending
  in `_API_KEY` (or `_API_KEY_<SUFFIX>`), or one listed in
  `TAMGA_UPSTREAM_KEY_ENVS`; never a `TAMGA_` variable.
- If the variable is empty the request gets 503 `provider_key_unavailable`.
  The caller's own key is not used as a fallback.
- `keys` is not consulted (the caller's keys are dropped); the header lists
  still are.
- A pool endpoint on the same route that names `api_key_env` is refused at
  load: say which key the route sends in one place.
- The response cache names such a request by its Tamga key, not by the shared
  organisation key, so two callers do not share cached answers.
- A request that a ROUTE rule sends to an internal pool does not carry the
  organisation's key: internal endpoints get no provider credential.
- Bedrock is not supported: its requests are signed with the host's AWS
  credentials and carry no key.

The section is privileged: only an admin may change it, as with `providers`.

## Network path: tenant-restriction headers

`proxy/internal/tenantheaders` holds the header logic for a mode in which
Tamga reads the requests of a whole network. It does no I/O:

```go
cfg := tenantheaders.Config{
    MicrosoftTenantID: "...", MicrosoftPolicyID: "...",
    ChatGPTWorkspaces: []string{"..."},
    AnthropicOrgs:     []string{"..."},
    GoogleDomains:     []string{"example.com"},
    GitHubEnterprises: []string{"..."},
}
if err := cfg.Validate(); err != nil { ... }      // once, at start
names := cfg.Apply(host, header)                   // per request
```

`Apply` looks at the request's host, and for a vendor that reads a header
there, removes any copy the client sent and sets the configured value. It
returns the names it set, or nil when the host is not one it knows or the
vendor is not configured; then the headers are not touched.

How it fits the ICAP mode (built separately): the gateway does TLS inspection
and sends each request to Tamga's ICAP service with REQMOD. The service reads
the encapsulated request line and headers, takes the host from `Host` (or the
absolute request URI), and calls `Apply`. When `Apply` returns names, it
answers `200 OK` with the modified request headers and the body untouched;
when it returns nil, it answers `204 No Content` if the gateway sent
`Allow: 204`, and otherwise the request as it came. The configuration would
come from the operator's environment, not the policy: it is a network setting
and is read once at start. Nothing here decides which hosts the gateway sends
to ICAP; send at least the hosts in the table above.

A gateway that cannot inspect TLS cannot add any of these headers, from Tamga
or anything else. Microsoft's proxy route covers sign-in only.

## Browser extension: `allowedWorkspaces`

A managed setting (policy only; a value in the options page's storage is
ignored):

```json
{
  "allowedWorkspaces": {
    "chatgpt": ["<ChatGPT workspace id>"],
    "claude": ["<Claude organisation id>"]
  }
}
```

For each site listed, the extension's service worker installs a
`declarativeNetRequest` rule that sets the vendor header on every request the
browser makes to that site (`ChatGPT-Allowed-Workspace-Id` on chatgpt.com,
`anthropic-allowed-org-ids` on claude.ai, with continuation lines as above).
The rule follows the policy: it is replaced when the managed setting changes,
and removed when the setting lists nothing. Ids that are not UUIDs are dropped,
and a site with more than 500 is left out.

This needs the permission `declarativeNetRequestWithHostAccess`, which acts
only on the hosts the extension already has (the three chat sites) and shows
no warning at install. The setting does not depend on `sites`: the scan and
the header are separate.

For Gemini, set the Chrome policy `AllowedDomainsForApps` instead; the browser
then adds Google's header itself.

Not verified: that the vendors accept the header when a browser extension sets
it rather than a proxy (they check the header, not who set it, but this has
not been tried), and that Chrome sets these header names without change.
