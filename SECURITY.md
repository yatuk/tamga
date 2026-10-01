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

Tamga does not yet authenticate callers on the proxy path. Until it does,
deploy it where only trusted applications can reach it, and note:

- `X-Tamga-Role` is ignored by default. Set `TAMGA_TRUST_ROLE_HEADER=true`
  only when an authenticating gateway in front of Tamga strips the header
  from client requests and sets it itself.
- `X-Tamga-Org-Id`, `X-Tamga-User-Id` and the operator-state headers
  (`X-Tamga-Operator-Id`, `X-Tamga-Active-Decisions`,
  `X-Tamga-Last-Verifiable-By`) are taken from the request as sent. They drive
  budget attribution and operator-state checks, so the same gateway rule
  applies if callers are not fully trusted.
- The proxy fails open when a scanner errors, panics or is shed under load.
  Watch `tamga_scan_degraded_total` and the `X-Tamga-Scan-Degraded` response
  header.

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
