# Pattern Update Runbook

**Version:** 1.0.0  
**Last Updated:** 2026-06-12  
**Audience:** Tamga Engineering, Security Researchers  
**Component:** `internal/scanner/` (Aho-Corasick DFA + regex injection scanner)

## Overview

This runbook documents the end-to-end process for adding, modifying, or
removing injection detection patterns in Tamga. Patterns live in
`internal/scanner/injection.go:injectionPatterns`, with the Turkish, German
and Russian additions in `internal/scanner/injection_i18n.go`, and are
compiled into a shared Aho-Corasick DFA at proxy startup.

Every pattern change must go through **proposal → review → test → deploy**
to prevent false-positive regressions in production.

---

## 1. Proposing a New Pattern

### 1.1 Submission

Open a GitHub issue with the content below. The repository has no issue
template for it; paste the text.

```markdown
### Pattern
Exact phrase to match (lowercase, trimmed): `your pattern here`

### Category
[ ] instruction_override
[ ] role_manipulation
[ ] delimiter_injection
[ ] context_manipulation
[ ] jailbreak
[ ] tool_fetch
[ ] indirect_injection

### Language
[ ] English  [ ] Turkish  [ ] Language-agnostic  [ ] Other: ___

### Source
Where did this pattern come from?
- OWASP LLM Top 10 (link)
- Lakera Gandalf challenge (link)
- JailbreakChat.com entry (link)
- Tamga red-team finding (internal)
- Community contribution (GitHub user)

### License
[ ] CC0  [ ] MIT  [ ] CC-BY  [ ] Apache 2.0  [ ] Proprietary (Tamga internal)

### Proposed Confidence
0.00 – 1.00 (see confidence ranges in Section 3)

### Positive Test Case
Example prompt that MUST trigger this pattern:
```

### Negative Test Case
Example prompt that MUST NOT trigger this pattern (benign usage):
```

### Real-World Prevalence
Is this pattern observed in the wild? Links to reports/VDBs:
```

### 1.2 Triage

Tamga engineering triages within **2 business days**:
- **P0 (Emergency):** CVE or public jailbreak with active exploitation → 24h turnaround
- **P1 (High):** New jailbreak technique observed in the wild → 1 week
- **P2 (Medium):** Community contribution, red-team finding → next sprint
- **P3 (Low):** Theoretical bypass without known exploitation → backlog

---

## 2. Review Checklist

Before any pattern is merged, a Tamga engineer must verify:

### 2.1 License Compatibility

| License | Allowed in open-source build? | Notes |
|---------|:---:|-------|
| CC0 | ✅ | Public domain, no restrictions |
| MIT | ✅ | Attribution in source comments |
| CC-BY | ✅ | Attribution in source comments |
| Apache 2.0 | ✅ | Attribution in source comments |
| Proprietary | ❌ | Must be extracted to private module before public release |
| Unknown / Unlicensed | ❌ | Reject — cannot accept patterns without clear license |

### 2.2 False-Positive Risk Assessment

For each pattern, manually test against the **negative corpus**:
1. Run `go test -v -run Adversarial ./internal/scanner/` — all negative tests must pass
2. Manually test against 5 real-world prompts that contain the pattern's tokens in benign context
3. Check if the pattern substring-appears inside legitimate company names, product names, or common phrases

### 2.3 Category and Confidence Calibration

Start at the **lower end** of the category's confidence range. Only increase after 30 days of production telemetry shows zero false positives.

See `internal/scanner/patterns/README.md` for current category ranges.

---

## 3. Confidence Ranges

The lowest and highest confidence in use per category are listed in the
[pattern catalogue](../internal/scanner/patterns/README.md#categories), which
is written from the code. Keep a new pattern inside its category's range
unless the pull request says why not. Two things to know:

- `delimiter_injection` is low on purpose (0.35 to 0.45): ordinary Markdown
  matches it. Do not raise it above 0.50.
- `act as` (0.72) and `disregard` (0.72) are the low ends of their categories
  because both appear in benign prompts.

---

## 4. Implementation

### 4.1 Add the pattern

```go
// internal/scanner/injection.go — injectionPatterns slice

// Category: jailbreak | Source: Lakera Gandalf (MIT) | Added: 2026-06-12
{"your new pattern here", "jailbreak", 0.88},
```

### 4.2 Add tests

```go
// internal/scanner/injection_adversarial_test.go

func TestInjection_YourNewPattern(t *testing.T) {
    // Positive: must detect
    s := NewInjectionScanner()
    findings, err := s.Scan(context.Background(), []byte("your new pattern here"))
    if err != nil { t.Fatal(err) }
    if len(findings) == 0 { t.Fatal("expected detection") }

    // Negative: must NOT detect in benign context
    findings, _ = s.Scan(context.Background(), []byte("legitimate text that contains some tokens"))
    for _, f := range findings {
        if f.Category == "your_category" {
            t.Fatalf("false positive: %s", f.Match)
        }
    }
}
```

### 4.3 Update documentation

1. Add pattern to category table in `internal/scanner/patterns/README.md`
2. Increment pattern count in the header comment of `injection.go`
3. Update `Version` and `Last updated` in `injection.go` header comment

### 4.4 Run full test suite

```bash
cd proxy
go test -v -run Adversarial ./internal/scanner/
go test -v -run Injection ./internal/scanner/
go test ./...                           # All packages
go test -fuzz=FuzzValidLuhn -fuzztime=30s ./internal/scanner/
go test -bench=. -benchmem ./internal/scanner/
```

All must pass before merging.

---

## 5. Deployment

### 5.1 Standard deployment

Pattern changes are compiled into the binary. Deployment follows the normal CI/CD pipeline:

1. Pull request merged to `main`.
2. CI (`.github/workflows/proxy-ci.yml`) runs govulncheck, gosec, the SBOM
   job, `go test -race` and the scanner benchmarks. The fuzz job runs only
   on the weekly schedule or when started by hand. The adversarial gate
   (`adversarial-gate.yml`) runs on pull requests that touch `proxy/`.
3. No workflow in this repository builds an image or deploys. Build the
   image from `proxy/Dockerfile` and roll it out the way you deploy the proxy.

### 5.2 Emergency deployment (P0)

For CVE or active-exploitation patterns:

1. Add pattern directly to `main` (no PR for speed)
2. CI green → build emergency release
3. Deploy to production within 24h
4. Backfill PR and review checklist within 48h

### 5.3 Hot-reload (future)

Patterns are compiled into the binary: a new pattern needs a new build and a
restart. `scanner.ReloadDFA()` exists and the proxy calls it when the policy
file changes, but it rebuilds the automaton from the same compiled-in list,
so it does not bring in new patterns. Reloading patterns from a file is not
implemented.

---

## 6. Removing / Deprecating Patterns

### 6.1 When to remove

- **False-positive rate > 1%** over 30-day telemetry window
- **Pattern source is retracted** (e.g., Lakera Gandalf dataset updated, old challenges removed)
- **Attack technique is patched** at the model level (all major providers reject it)
- **Pattern is obsolete** — no instances observed in production for 90 days

### 6.2 Process

1. Mark pattern with `// DEPRECATED: reason (date)` comment
2. Keep in source for one release cycle (allows rollback)
3. Remove in next release
4. Update `injection.go` header comment with new pattern count

---

## 7. Telemetry and Monitoring

After deployment, monitor:

The proxy exposes these at `GET /api/v1/metrics` (viewer role):

| What | Metric |
|------|--------|
| DFA compile time | `tamga_dfa_build_seconds` |
| Approximate size of the patterns | `tamga_dfa_pattern_bytes` |
| Number of patterns in the DFA | `tamga_dfa_patterns_total` |

There is no metric for injection blocks alone and the repository ships no
alert rules. Blocked requests are in the event list (`GET /api/v1/events`)
with their findings. The DFA refuses to build above 50,000 patterns and logs
a warning above 10,000.

### 7.1 False-positive triage

If a customer reports a blocked legitimate prompt:

1. Get the **exact prompt text** (redact sensitive parts)
2. Add to `injection_adversarial_test.go` negative corpus
3. If the pattern is the sole match → lower confidence or remove
4. If cumulative scoring caused the block → adjust `cumulativeBoostPerExtra`
5. Deploy fix within 1 business day
6. Postmortem: document the incident and resolution

---

## 8. Pattern Update Cadence

| Frequency | Action | Owner |
|-----------|--------|-------|
| **Weekly** | Check Lakera Gandalf leaderboard for new jailbreak techniques | Security team |
| **Monthly** | Review OWASP LLM Top 10 for new categories | Engineering |
| **Quarterly** | Full audit: FP telemetry review, stale pattern removal, confidence recalibration | Engineering + Security |
| **On CVE** | Emergency pattern within 24h | On-call engineer |

---

## 9. Related Documentation

- [Pattern Catalog](../internal/scanner/patterns/README.md)
- [Adversarial Test Suite](../internal/scanner/injection_adversarial_test.go)
- [DFA Benchmarks](../internal/scanner/dfa_bench_test.go)
- [PCI Logging Policy](PCI_LOGGING_POLICY.md)
