> Override rules for this page. Falls back to ../MASTER.md for anything not specified here.

# Dashboard Page Overrides

## Layout and Density

- Prefer dark-background cards for operator focus and reduced glare.
- Use data-dense layout patterns with compact spacing for analytics.
- Use monospace font for numeric KPIs, request IDs, and log-like values.

## Chart Palette

Derived from `../MASTER.md` primary/secondary/accent:

1. `#0369A1` (accent / CTA)
2. `#0F172A` (primary)
3. `#334155` (secondary)
4. `#0EA5E9` (accent tint)
5. `#64748B` (secondary tint)
6. `#1E293B` (primary tint)

## Stat Card Icon Colors

- Total requests: blue
- Blocked: red
- Redacted: yellow
- Warned: orange

## Table Behavior

- Row hover must use subtle background change.
- Apply left-border accent by severity level:
  - critical/high: red accent
  - medium: yellow accent
  - low: slate/neutral accent

## Anti-Patterns for this page

- Avoid heavy gradients.
- Avoid oversized whitespace that reduces scanability.
- Avoid decorative animations unrelated to monitoring tasks.
