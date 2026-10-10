# PCI-DSS Logging Policy

**Version:** 1.0.0  
**Compliance:** PCI-DSS v4.0 Requirement 3.4  
**Last updated:** 2026-06-12

## Policy

Tamga NEVER logs full Primary Account Numbers (PANs). All credit card numbers detected by the PII scanner are masked before storage or transmission.

## Masking Format

```
Input:  4532015112830366
Output: 453201******0366
```

**Rule:** First 6 digits (BIN/IIN) + last 4 digits preserved. Middle digits replaced with `*`.

PCI-DSS v4.0 Requirement 3.4 asks that a PAN be unreadable wherever it is stored. First 6 / last 4 is the truncation format commonly used for audit trails, and it keeps the BIN for issuer identification. This document describes what the code does; whether a deployment meets PCI-DSS is for its assessor to decide.

The masking applies to card numbers the PII scanner detects. A number the scanner does not recognise is not masked.

## Scope

This policy applies to:
- **PostgreSQL audit log** (`request_logs.findings` JSONB column)
- **SIEM webhook payloads** (Splunk, Sentinel, QRadar, etc.)
- **Dashboard display** (React UI — findings match field)
- **API responses** of the proxy that return findings (`/api/v1/events` and the event detail)
- **Logs** (the proxy writes its log to standard error; in a container that is the container log)

The analyzer is a separate service with its own endpoint (`/api/v1/analyze`); this policy describes the proxy.

## Technical Implementation

The `maskPAN()` function in `proxy/internal/scanner/pii.go` implements the masking:

```go
func maskPAN(pan string) string {
    d := digitsOnly(pan)
    if len(d) < 13 {
        return maskContent(pan) // fallback
    }
    return d[:6] + strings.Repeat("*", len(d)-10) + d[len(d)-4:]
}
```

Fallback: If a card number is shorter than 13 digits (invalid PAN), generic `maskContent()` is used which replaces middle characters with `*`.

## Audit Verification

To verify compliance:
1. Search the proxy's logs for un-masked PANs. The proxy writes its log to standard error, so search wherever you collect it, for example: `docker compose --env-file .env -f deploy/docker-compose.yml logs proxy | grep -P '\b[3-6]\d{12,18}\b'`
2. Query PostgreSQL for un-masked PANs: `SELECT findings FROM request_logs WHERE findings::text ~ '\d{13,19}'`
3. Verify maskPAN output, from `proxy/`: `go test -run MaskPAN ./internal/scanner/`

## Change History

| Date | Version | Change |
|------|---------|--------|
| 2026-06-12 | 1.0.0 | Initial PCI-DSS masking policy — maskPAN replaces generic maskContent for credit_card findings |
