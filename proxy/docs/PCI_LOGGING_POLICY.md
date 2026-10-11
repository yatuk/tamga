# PCI-DSS Logging Policy

**Version:** 1.1.0  
**Compliance:** PCI-DSS v4.0 Requirements 3.4 and 3.5  
**Last updated:** 2026-10-11

## Policy

Tamga NEVER logs full Primary Account Numbers (PANs). All credit card numbers detected by the PII scanner are masked before storage or transmission.

## Masking Format

```
Input:  4532015112830366
Output: ************0366
```

**Rule:** The last 4 digits are preserved. Every other digit is replaced with `*`; separators are dropped.

PCI-DSS v4.0 lets at most the BIN and the last four digits of a PAN be displayed, and only to people with a business need to see more than that (Requirement 3.4.1), and asks that a stored PAN be unreadable (3.5.1), for which truncation is one method. A finding's match is not shown to a chosen few: it is written to the request log, the live feed, the console and every export. So it holds the least that still lets an operator tell which card a row is about, the last four digits, as a receipt does.

Beside the match, a card finding carries what the embedded issuer table says about the number: `card_brand`, `card_type`, `bank_name` and `country`. They are derived from the first six or eight digits; those digits themselves (`bin`) are not stored. Where an issuer has a single number range its name gives away what the BIN would. With `data.hash_findings: true` the match is a keyed hash and `bank_name` and `country` are dropped.

Until v0.9.0 the match was the first 6 and last 4 digits (`453201******0366`) and the metadata held `bin`. Rows written by those versions are not rewritten; they stay until retention deletes them (`TAMGA_RETENTION_REQUEST_LOGS_DAYS`).

This document describes what the code does; whether a deployment meets PCI-DSS is for its assessor to decide.

The masking applies to card numbers the PII scanner detects. A number the scanner does not recognise is not masked.

## Scope

This policy applies to:
- **PostgreSQL audit log** (`request_logs.findings` JSONB column)
- **SIEM webhook payloads** (Splunk, Sentinel, QRadar, etc.), which carry a finding's type, category and severity and never its match
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
    return strings.Repeat("*", len(d)-4) + d[len(d)-4:]
}
```

Fallback: If a card number is shorter than 13 digits (invalid PAN), generic `maskContent()` is used which replaces middle characters with `*`.

Two different cards with the same last four digits have the same match. While a request is scanned they are told apart by a fingerprint that is kept in memory for that scan only and never written out, so that the second is not taken for a copy of the first and left unmasked.

## Audit Verification

To verify compliance:
1. Search the proxy's logs for un-masked PANs. The proxy writes its log to standard error, so search wherever you collect it, for example: `docker compose --env-file .env -f deploy/docker-compose.yml logs proxy | grep -P '\b[3-6]\d{12,18}\b'`
2. Query PostgreSQL for un-masked PANs: `SELECT findings FROM request_logs WHERE findings::text ~ '\d{13,19}'`. For rows that still hold the old form: `SELECT count(*) FROM request_logs WHERE findings::text ~ '"match": ?"\d{6}\*+\d{4}"' OR output_findings::text ~ '"match": ?"\d{6}\*+\d{4}"'`
3. Verify maskPAN output, from `proxy/`: `go test -run MaskPAN ./internal/scanner/`

## Change History

| Date | Version | Change |
|------|---------|--------|
| 2026-06-12 | 1.0.0 | Initial PCI-DSS masking policy — maskPAN replaces generic maskContent for credit_card findings |
| 2026-10-11 | 1.1.0 | The match keeps the last 4 digits only (was first 6 and last 4); `bin` is no longer stored in the finding's metadata |
