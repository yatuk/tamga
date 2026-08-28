---
name: Tamga Dashboard
description: A forensic chain-of-custody workspace for live LLM security operations.
colors:
  tamga-red: "oklch(0.62 0.16 22)"
  ink: "oklch(0.155 0.008 255)"
  graphite: "oklch(0.205 0.009 255)"
  paper: "oklch(0.992 0.004 85)"
  muted-ink: "oklch(0.75 0.01 260)"
  rule: "oklch(0.29 0.012 255)"
  critical: "oklch(0.64 0.16 22)"
  caution: "oklch(0.78 0.12 90)"
  verified: "oklch(0.72 0.13 150)"
typography:
  headline:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  body:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.25
  measurement:
    fontFamily: "Fira Code, ui-monospace, monospace"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.25
rounded:
  sm: "0.75rem"
  md: "0.875rem"
  lg: "1rem"
spacing:
  xs: "0.5rem"
  sm: "0.75rem"
  md: "1rem"
  lg: "1.5rem"
  xl: "2rem"
components:
  evidence-seal:
    backgroundColor: "{colors.graphite}"
    textColor: "{colors.muted-ink}"
    typography: "{typography.measurement}"
    rounded: "{rounded.sm}"
    padding: "0.25rem 0.5rem"
  docket-container:
    backgroundColor: "{colors.graphite}"
    textColor: "{colors.paper}"
    rounded: "{rounded.sm}"
    padding: "1rem"
---

# Design System: Tamga Dashboard

## Overview

**Creative North Star: "The Evidence Docket"**

Tamga is designed as a chain-of-custody workspace, not a decorative cybersecurity dashboard. The interface is calm under pressure: strong rules, compact labels, explicit provenance, and status treatments that distinguish verified safety from unavailable evidence.

The visual world borrows from forensic records and regulated operations. Ink and paper surfaces carry the information; Tamga red is a scarce evidence seal for block, critical, or unverified states. Measurements use monospace only when fixed-width scanning improves comprehension.

**Key Characteristics:**

- Continuous evidence records with identity, source, time, action, and finding kept together.
- Flat, ruled surfaces with restrained tonal layering.
- Dense but strongly ordered information: disposition, evidence, measures, then supporting analysis.
- Honest unavailable, empty, error, and verified states.

## Colors

The palette is nearly neutral, with semantic color reserved for operational meaning.

### Primary

- **Tamga Evidence Red:** the product accent and the seal for critical, blocked, or unverified evidence.

### Secondary

- **Caution Amber:** degraded, incomplete, or attention-required states.
- **Verified Green:** confirmed healthy, pass, and complete states only.

### Neutral

- **Ink:** dark application ground.
- **Graphite:** records, panels, and navigation surfaces.
- **Paper:** light-mode records and high-contrast foreground.
- **Rule:** borders, dividers, and chain-of-custody row separation.
- **Muted Ink:** secondary labels and explanatory copy.

**The Evidence Color Rule.** Never use green to imply low risk when telemetry is absent. Missing evidence is amber and explicitly unverified.

## Typography

**Display Font:** Inter (system sans fallback)
**Body Font:** Inter (system sans fallback)
**Label/Mono Font:** Fira Code (system monospace fallback)

**Character:** Inter keeps dense operational language legible; Fira Code is limited to request IDs, timestamps, counts, and measurements.

### Hierarchy

- **Headline** (600, 1.5rem, 1.2): page identity.
- **Title** (600, 0.875–1rem): sections and record headers.
- **Body** (400, 0.875rem, 1.5): explanation and recovery guidance, kept under 75 characters per line.
- **Label** (500, 0.75rem): controls and short metadata.
- **Measurement** (500, 0.75rem): tabular values, timestamps, and immutable identifiers.

**The Measurement Rule.** Monospace means data, not “technical” decoration.

## Layout

The application shell uses a persistent 16rem desktop case index, a 3.5rem operational top bar, and a centered content canvas capped at 1600px. The overview sequence is disposition, live evidence, supporting findings, operational measures, control coverage, and deeper analytics.

Desktop evidence uses a 12-column composition: posture occupies four columns and the live ledger eight. At mobile widths, navigation moves into a sheet and evidence follows disposition immediately. No essential state or investigation link may depend on horizontal scrolling.

Spacing follows an 8px base rhythm. Related controls use 8–12px gaps; major operational sections use 28px vertical separation.

## Elevation & Depth

The system is flat by default. Borders, divider rules, and tonal surface shifts establish depth. Shadows are limited to elevated popovers and command surfaces; normal records never combine a border with a wide ambient shadow.

**The Flat Record Rule.** Evidence remains on the same visual plane until interaction genuinely elevates it.

## Shapes

Surfaces use gently curved 12px corners. Controls and records share the same geometry; pills are reserved for compact statuses. One-pixel rules divide evidence. Circular shapes are limited to status indicators, avatars, and icon-only controls.

## Components

### Buttons

- **Shape:** gently curved (12px) with a minimum 32px target for compact controls.
- **Primary:** high-contrast foreground fill for committed actions.
- **Hover / Focus:** tonal shift plus the shared two-pixel semantic focus ring.
- **Secondary:** flat surface with a rule border; never a low-contrast text-only mystery action.

### Chips

- **Style:** compact bordered seal with semantic foreground and translucent semantic background.
- **State:** text always names the state; color is never the only carrier.

### Cards / Containers

- **Corner Style:** 12px.
- **Background:** graphite in dark mode, paper in light mode.
- **Shadow Strategy:** none at rest.
- **Border:** one-pixel rule.
- **Internal Padding:** 12–16px, with continuous ledgers using row dividers instead of nested cards.

### Inputs / Fields

- **Style:** surface fill, rule border, 12px corners.
- **Focus:** two-pixel semantic outline with two-pixel offset.
- **Error / Disabled:** explanatory copy names the failure and recovery; reduced contrast alone is insufficient.

### Navigation

Navigation is a compact case index. Active items use a graphite record surface and a narrow verified rail; section labels are short, uppercase, and collapsible. Mobile uses a full-height sheet with the same grouping and runtime state.

### Live Evidence Ledger

The signature component preserves action, finding, request ID, source, and timestamp in one linked row. Selecting a row carries its request identity and observation window into the incident console. “No evidence” and “evidence unavailable” are separate states.

## Do's and Don'ts

### Do:

- **Do** lead operational pages with verified state and the next investigation action.
- **Do** keep request identity, provider, timestamp, decision, and finding together.
- **Do** use explicit `UNVERIFIED`, empty, loading, and error language.
- **Do** preserve keyboard focus, reduced motion, tabular numerals, and color-independent status labels.

### Don't:

- **Don't** infer “LOW” or safe posture from missing telemetry.
- **Don't** present illustrative or demo values as live production evidence.
- **Don't** flatten every metric into an equal-weight card.
- **Don't** use Tamga red decoratively; its scarcity gives evidence states authority.
