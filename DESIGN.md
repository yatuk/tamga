---
name: Tamga Dashboard
description: The operations console of the Tamga LLM security proxy. One identity with tamgaproxy.com.
colors:
  ink: "#10120f"
  ink-subtle: "#171a16"
  ink-card: "#1d211b"
  ink-elevated: "#242921"
  paper: "#f1f1e8"
  muted-paper: "#c0c5ba"
  rule: "#343a31"
  tamga-red: "#e45e4f"
  critical: "#e5604f"
  high: "#d9903f"
  medium: "#d2a63c"
  low: "#6fa8cf"
  pass: "#75c9a4"
typography:
  display:
    fontFamily: "Barlow Condensed, Arial Narrow, sans-serif"
    fontSize: "1.875rem"
    fontWeight: 800
    lineHeight: 1
    letterSpacing: "0.025em"
  body:
    fontFamily: "Barlow, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "JetBrains Mono, ui-monospace, monospace"
    fontSize: "0.6875rem"
    fontWeight: 500
    lineHeight: 1.25
    letterSpacing: "0.1em"
  measurement:
    fontFamily: "JetBrains Mono, ui-monospace, monospace"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.25
rounded:
  sm: "0"
  md: "0"
  lg: "0"
spacing:
  xs: "0.5rem"
  sm: "0.75rem"
  md: "1rem"
  lg: "1.5rem"
  xl: "2rem"
components:
  panel:
    backgroundColor: "{colors.ink-card}"
    textColor: "{colors.paper}"
    rounded: "{rounded.sm}"
    padding: "1rem"
  status-badge:
    backgroundColor: "{colors.ink-subtle}"
    textColor: "{colors.muted-paper}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 0.375rem"
---

# Design System: Tamga Dashboard

## Overview

The dashboard shares one identity with [tamgaproxy.com](https://tamgaproxy.com): warm ink and paper surfaces, square corners, a condensed display face, and Tamga red kept for what was blocked or is critical. It is a working tool for SOC analysts, so density and legibility come before decoration.

**Key characteristics:**

- Flat, ruled surfaces. Depth comes from borders and four surface tones, not shadows.
- Dense but ordered: state first, then the numbers, then the evidence.
- Request identity, provider, time, decision and finding stay together in every row.
- Empty, loading, failed and unauthorised states are designed, not left blank.

Tokens live in `dashboard/app/globals.css`. Shared building blocks are in `dashboard/components/app`, the shell in `components/shell`, and shadcn/ui primitives in `components/ui`.

## Colors

Two themes with the same structure; dark is the default. The values in the front matter are the dark theme; the light theme mirrors them (`#e7e8e2` page, `#10120f` text).

- **Surfaces:** base (page), subtle (sidebar), card (panels), elevated (popovers, dialogs).
- **Text:** `fg`, `fg-muted`, `fg-subtle`, `fg-faint`. Every text color reaches 4.5:1 on every surface in both themes.
- **Brand (Tamga red):** the active navigation rail, the focus ring and selection. Not for buttons that are not destructive.
- **Status:** critical, high, medium, low, pass. Actions map onto them: BLOCK is critical, REDACT is medium, WARN is high, PASS is pass.

**The evidence color rule.** Never show green for missing data. No traffic, no evaluation or no admin key is a neutral state with its own wording, not a low-risk reading.

Semantic names follow shadcn/ui (`background`, `card`, `muted`, `accent`, `primary`, `destructive`), so generated components work unmodified. `accent` is the hover surface; the red is `brand`.

## Typography

- **Display:** Barlow Condensed 800, uppercase. Page titles only.
- **Body:** Barlow 400 to 600 at 14px; small text 12px. Nothing below 11px.
- **Label / mono:** JetBrains Mono. Uppercase with letter-spacing for column heads, panel titles and stat labels (11px); plain for IDs, timestamps and numbers.

**The measurement rule.** Monospace means data: request IDs, counts, latencies, timestamps. Numbers in columns use tabular figures. Dates are `en-GB` (24-hour), numbers `en-US`.

Titles and buttons use Title Case; descriptions are full sentences. No em-dashes in interface copy; `…` for loading and placeholders.

## Layout

A collapsible sidebar (16rem, icons only when collapsed, a drawer on mobile), a 3.5rem header with the breadcrumb, proxy status, command palette and theme toggle, and a content column capped at 1600px. A page is: `PageHeader`, then a `StatGrid` if it has headline numbers, then `Panel`s.

Spacing follows a 4px base. Sections are 16 to 24px apart; panel padding is 16px; table cells 8px by 16px.

## Shapes and depth

Corners are square everywhere; `rounded-full` is for dots and avatars only. Surfaces are flat with one-pixel rules. Shadows are limited to popovers, dialogs and sheets.

## Components

- **PageHeader:** title, one-sentence description, actions on the right. The section is in the header breadcrumb, so pages carry no eyebrow.
- **Panel:** the standard surface. Mono uppercase title, optional description, and an aside for counts or small actions.
- **Stat / StatGrid:** label, value, optional delta, sparkline, hint and tooltip. Stats share dividers inside one ruled block instead of sitting in separate cards. Tone (critical, warn, pass) is for state, not decoration.
- **StatusBadge, ActionBadge, SeverityBadge:** square, mono, uppercase. The text always names the state; color is never the only carrier.
- **Tables:** shadcn `Table` for short lists; a virtualised ARIA grid (TanStack Virtual) for queues with keyboard navigation. Long lists paginate with an infinite query.
- **States:** `EmptyState`, `ErrorState`, `AdminKeyRequired`, `SkeletonRows`.
- **ConfirmButton:** every destructive action asks first, naming the item and the consequence. Revoking an API key goes further and asks for the key name to be typed.
- **TimeRangeToggle:** the 24h / 7d / 30d window, stored in the URL as `?range=`.
- **PageTabs:** the underlined tab row that splits a page into sections. The active tab is a URL parameter (`?tab=`).
- **TimeSeriesChart:** the one chart for anything plotted over time, as areas or bars. Pages pass data and series; axes, grid, legend and tooltip stay the same everywhere. It is the stock shadcn chart on Recharts 3, loaded on demand.
- **BarList:** a ranked list with proportional bars (providers, models, finding types). One bar color: rank is carried by order and length, not by a rainbow.
- **DiffView:** a unified line diff for policy changes. The +/- marker carries the meaning; color only reinforces it.
- **DetailList:** label and value rows for configuration and status read-outs.
- **FormField:** a control with its visible label above and an optional hint below. Every input goes through it.
- **CopyButton:** copies a value and confirms with a check mark.
- **CircuitBadge:** circuit breaker state. Closed is healthy; open means the upstream is out of rotation.
- **Dialogs and sheets:** shadcn `Dialog` for forms, `Sheet` for detail of a row. No hand-rolled overlays; they need the focus trap.
- **Buttons:** primary is the high-contrast fill; outline and ghost for everything else; destructive only for delete and revoke. No per-call color overrides, and no raw `<button>` outside `components/ui`.

**The measured-data rule.** A number, a status or a chart is shown only if the proxy reported it. No interpolated percentiles, no composite health scores, no placeholder bars, no column that can only show a dash. When something is not configured, say so in neutral wording.

**One place per job.** API keys are managed on the Keys page, alert destinations on Integrations, custom entities in the policy. Other pages link there instead of repeating the form.

## Accessibility

- Visible focus ring on every interactive element; no `outline-none` without a replacement.
- Skip link to `#main`; one `h1` per page.
- Form controls have a visible label or an accessible name.
- Pointer-only interactions have a keyboard equivalent; `prefers-reduced-motion` is respected.
- Filters, tabs, search text and the time window that change what a page shows belong in the URL (`hooks/useUrlState.ts`, `hooks/useRangeParam.ts`).
- Every page is checked at 375px and 1440px for horizontal overflow by the end-to-end suite, which runs against fixtures, not a live proxy.

## Do and don't

- **Do** lead with state and the next action, then numbers, then rows.
- **Do** keep request identity, provider, timestamp, decision and finding together.
- **Do** say plainly when data is missing, loading or failed.
- **Don't** show a safe or low reading without telemetry behind it.
- **Don't** present demo values as live data.
- **Don't** use Tamga red decoratively or for ordinary primary buttons.
- **Don't** add one-off borders, radii, shadows or text sizes to a component; change the shared primitive instead.
