# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Tamga, please **do not**
open a public issue. Report it privately:

1. **GitHub private vulnerability reporting (preferred).** Open the
   repository's **Security** tab and choose **Report a vulnerability**, or go
   to <https://github.com/yatuk/tamga/security/advisories/new>.
2. **E-mail**, if you cannot use GitHub or the form is not offered to you:
   security@tamgaproxy.com. No PGP key is published for this address.

Include:

- Description of the vulnerability
- Steps to reproduce
- What you tested: the commit, or the version named in `CHANGELOG.md`
- Your suggested fix (optional)

Tamga is maintained by its author; there is no security team. The
maintainer's targets are to acknowledge a report within 48 hours and to have
a fix on `main` within 14 days of confirming a critical issue. They are
targets, not a service level or a guarantee. What happens to a report is
described in
[Vulnerability Handling and Coordinated Disclosure](docs/ENISA_VULNERABILITY_DISCLOSURE.md).

## Supported Versions

No release has been tagged in the repository, and the project publishes no
container image or package. `CHANGELOG.md` names v0.7.0 (2026-06-20) and
v0.8.0-rc1 (2026-07-21) and lists v0.9.0 as unreleased. Security fixes are
made on `main` only; nothing is backported.

A release workflow that signs what it builds is in the repository and has
not run yet. What a release will contain and how to check its signatures,
provenance and SBOMs is in
[Verifying a release](docs/verifying-a-release.md).

| Version | Security fixes |
|---------|-----------|
| `main` | ✅ |
| v0.8.0-rc1 and earlier (changelog entries; no tag or branch exists) | ❌ Update to `main` |

## Verifying a Release

Require the release workflow's exact keyless signing identity and GitHub's
OIDC issuer. For a release file, verify its direct cosign bundle and then the
signed checksum list:

```bash
VERSION=v0.9.0
IDENTITY="https://github.com/yatuk/tamga/.github/workflows/release.yml@refs/tags/${VERSION}"
ISSUER="https://token.actions.githubusercontent.com"
FILE="tamga_${VERSION}_linux_amd64.tar.gz"

cosign verify-blob "$FILE" --bundle "${FILE}.sigstore.json" \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
sha256sum --check --strict SHA256SUMS
```

For an image, use the digest in the signed `image-digests_<version>.txt`, not
the movable tag:

```bash
cosign verify "ghcr.io/yatuk/tamga@sha256:<digest>" \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
```

No release exists yet, so these commands have not been run against one.
[Verifying a release](docs/verifying-a-release.md) has the full steps,
including `sha256sum --check --ignore-missing` for a partial download, the
SBOM attestations and the build provenance (`gh attestation verify`).

## Management API

`/api/v1/*` is opened by `TAMGA_ADMIN_KEY` (role admin), by an API key of
scope read, write or admin (viewer, analyst, admin), or by a session token.
The key is read from the `X-Tamga-Admin-Key` header only, never from the
URL. Every route names the permission it needs; a role that lacks it gets
403. `GET /health` is public. `GET /api/v1/health/detailed` and
`/api/v1/health/detail` answer anyone, but describe the deployment
(database, Redis, analyzer, scanner count, provider pools) only to a caller
with a management credential. See
[Who may call what](docs/operations.md#who-may-call-what).

## Trust Assumptions

The [threat model](docs/threat-model.md) lists the trust boundaries, the
controls with the code and tests behind each, and the risks that remain.
The points below are the ones an operator has to act on.

Callers on the proxy path are authenticated only when they send a Tamga key
in `X-Tamga-Key`, and only required to when `TAMGA_REQUIRE_KEY=true`. The
default accepts requests without one, so an existing deployment keeps
working; until the setting is on, deploy Tamga where only trusted
applications can reach it. Note:

- A request with a valid key gets its organisation and role from the key's
  record. `X-Tamga-Org-Id` and `X-Tamga-Role` sent with it are ignored.
  `X-Tamga-User-Id` is ignored when the key names a user; a key that names
  none belongs to a service acting for many users, and that service's own
  `X-Tamga-User-Id` is kept as its attribution.
- A request without a key is anonymous. `X-Tamga-Org-Id` and
  `X-Tamga-User-Id` are then taken as sent and drive budget attribution, and
  `X-Tamga-Role` is ignored unless `TAMGA_TRUST_ROLE_HEADER=true`, which is
  only safe when an authenticating gateway in front of Tamga strips the
  header from client requests and sets it itself.
- The operator-state headers (`X-Tamga-Operator-Id`,
  `X-Tamga-Active-Decisions`, `X-Tamga-Last-Verifiable-By`) are taken from
  the request as sent in both cases.
- Keys are stored as a SHA-256 hash. A key with the `proxy` scope is refused
  by the management API. Revocation reaches other replicas within 30
  seconds, and a key that verified before is accepted for up to five minutes
  while the database is unreachable.
- `X-Forwarded-For` is ignored unless the connection comes from an address
  in `TAMGA_TRUSTED_PROXIES`. The IP allowlist and the per-address rate limit
  use the connecting address otherwise. List only proxies that append to the
  header; a proxy that passes it through unchanged makes it forgeable again.
- The optional classifier service receives prompt text over gRPC, plain
  unless TLS is configured. Like the analyzer and the scanner service it
  refuses calls without its token once one is set (`TAMGA_CLASSIFIER_TOKEN`;
  Compose requires all three), and without a token it accepts any caller.
  Run it on the internal network only. Its model is
  a file you download and mount; Tamga does not fetch or verify it, so pin
  the revision you measured.
- The Compose full stack publishes HTTPS through a TLS edge whose default
  certificate comes from a local CA. Give clients that CA's root rather
  than turning certificate checks off.
- When a scanner errors, panics or is shed under load, `scan.on_error` decides:
  `block` (the shipped policy) answers 503, `pass` forwards the
  request. With no setting, a request that carries tool results or tool
  definitions gets the 503 and a plain chat request is forwarded. Watch `tamga_scan_degraded_total` and the `X-Tamga-Scan-Degraded`
  response header either way.
- The role of a piece of text (system, user, tool, …) is read from the request
  body, which the caller writes. A rule narrowed with `applies_to` trusts the
  caller not to mislabel text; only do so when your own server builds the
  request.
- Images, audio and PDFs attached to a request are not readable by the proxy.
  Instructions or personal data inside them reach the model unseen. Text
  attachments are decoded and scanned.
- A JSON body that parsers can read differently (a repeated key, invalid
  UTF-8, non-standard JSON) is refused with 400 by default. Setting
  `scan.on_malformed: raw_scan` forwards it instead; `tamga_malformed_json_total`
  counts both cases.

## Fixed on `main` (unreleased)

Found in an internal review on 2026-10-01; see the Security section of
[CHANGELOG.md](CHANGELOG.md) for details.

| Issue | Affected |
|-------|----------|
| Default policy did not enforce the prompt-injection rule | `main` from 2026-08-02 |
| Failover forwarded caller credentials to a different provider | ≤ 0.8.0-rc1 |
| Policy exceptions honoured a caller-supplied `X-Tamga-Role` | ≤ 0.8.0-rc1 |
| Output-blocked responses could be served from the cache | ≤ 0.8.0-rc1 |
| Responses above the output scan buffer were truncated | ≤ 0.8.0-rc1 |

## Disclosure Policy

Disclosure is coordinated with the reporter. Once a fix is on `main`, a
GitHub Security Advisory and an entry in the Security section of
`CHANGELOG.md` are published, and the reporter is credited in both unless
they prefer not to be named. The project has no list of who runs Tamga, so
those two are the only announcement.

## EU Cyber Resilience Act

How the project handles vulnerabilities (private reports, fixes on `main`,
published advisories, a changelog) is described in
[Vulnerability Handling and Coordinated Disclosure](docs/ENISA_VULNERABILITY_DISCLOSURE.md).
That document does not state whether or how the Act applies to Tamga, its
maintainer, or anyone who deploys or redistributes it; that is for each of
them to determine. Tamga has not been assessed against the Act, and the
project has no arrangement for notifying ENISA or a national CSIRT.

## Security Advisory Archive

Security advisories are published on the repository's
[GitHub Security Advisories](https://github.com/yatuk/tamga/security/advisories) page.
