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

This complies with PCI-DSS v4.0 Requirement 3.4 which mandates that PANs be rendered unreadable anywhere they are stored. The first 6 / last 4 format is the industry standard for audit trails and supports BIN-based issuer identification.

## Scope

This policy applies to:
- **PostgreSQL audit log** (`request_logs.findings` JSONB column)
- **SIEM webhook payloads** (Splunk, Sentinel, QRadar, etc.)
- **Dashboard display** (React UI — findings match field)
- **API responses** (`/api/v1/events`, `/api/v1/analyze`)
- **Log files** (structured JSON logs)

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
1. Search all log files for un-masked PANs: `grep -E '\b[34]\d{12,18}\b' /var/log/tamga/*.log`
2. Query PostgreSQL for un-masked PANs: `SELECT findings FROM request_logs WHERE findings::text ~ '\d{13,19}'`
3. Verify maskPAN output: `go test -run TestMaskPAN ./internal/scanner/`

## Change History

| Date | Version | Change |
|------|---------|--------|
| 2026-06-12 | 1.0.0 | Initial PCI-DSS masking policy — maskPAN replaces generic maskContent for credit_card findings |
