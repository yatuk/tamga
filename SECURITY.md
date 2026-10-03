# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Tamga, please **do not**
open a public issue. Email security@tamga.dev with:

- Description of the vulnerability
- Steps to reproduce
- Affected version (e.g. v0.7.0)
- Your suggested fix (optional)

We aim to acknowledge reports within 48 hours and release a fix
within 14 days for critical issues.

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.7.x   | ✅ |
| < 0.7.0 | ❌ |

## Trust Assumptions

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
- The optional classifier service receives prompt text over plain gRPC and
  has no authentication. Run it on the internal network only. Its model is
  a file you download and mount; Tamga does not fetch or verify it, so pin
  the revision you measured.
- When a scanner errors, panics or is shed under load, `scan.on_error` decides:
  `block` (the shipped policy) answers 503, `pass` or no setting forwards the
  request. Watch `tamga_scan_degraded_total` and the `X-Tamga-Scan-Degraded`
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

We follow responsible disclosure. Once a fix is released, we credit
the reporter (unless they prefer to remain anonymous) in the
release notes.

## EU Cyber Resilience Act

Tamga complies with the EU Cyber Resilience Act (effective 11 September 2026).
See [ENISA Vulnerability Disclosure](docs/ENISA_VULNERABILITY_DISCLOSURE.md) for
our coordinated vulnerability disclosure procedure.

## Security Advisory Archive

Security advisories are published on our
[GitHub Security Advisories](https://github.com/yatuk/tamga/security/advisories) page.
