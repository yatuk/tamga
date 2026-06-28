# TAMGA → Cortex XSOAR-Tier Enterprise Transformation Plan

**Author:** Principal UX Architect, Enterprise SOC Products  
**Date:** 2026-06-12  
**Target:** `dashboard/` (Next.js 15 + Tailwind CSS + React 19)  
**Status:** PLANNING — No code has been modified yet

---

## Executive Summary

After a line-by-line audit of every file in the `dashboard/` directory, the diagnosis is clear: Tamga currently presents itself as a **developer tool with a terminal cosplay aesthetic**, not an enterprise Security Operations Center (SOC) platform. The product has real engineering underneath — real API calls, TanStack virtualized tables, React Query caching, and a working policy engine. But the UI actively undermines this credibility with:

1. **Fake macOS traffic lights** on 31 TerminalFrame instances — the #1 visual signal of "hobbyist demo"
2. **Single-column card stack layout** — no split-pane, no "War Room" incident triage surface, no resizable panels
3. **Fira Sans as body font** — a monospace-adjacent typeface that reduces readability at scale; 275+ `font-mono` usages make the UI feel like a terminal, not a SIEM
4. **Gradient dividers, hover glow shadows, backdrop-blur on headers** — Vercel-template design language inappropriate for a SOC tool
5. **`p-8`/`p-10` empty states** — waste 30–40% of viewport real estate on padding

### Reference: XSOAR / CrowdStrike Design DNA

Cortex XSOAR and CrowdStrike Falcon share a design language that can be summarized as:

| Principle | XSOAR / CrowdStrike | Tamga (Current) |
|---|---|---|
| **Data density** | 40–60 rows visible, 4–8px cell padding | TerminalFrame chrome eats 36px header per card |
| **Layout** | Split-pane: list left, detail right (War Room) | Single-column vertical stack |
| **Typography** | Inter/SF Pro body, monospace ONLY for code/data | Fira Sans (mono-adjacent) everywhere |
| **Corners** | `2px` (`rounded-sm`) universally | ✅ Already correct |
| **Shadows** | None on cards; subtle border-only elevation | Hover glow shadows (MetricStat), gradient fades |
| **Chrome** | Zero decorative chrome; every pixel carries data | Red/amber/green dots, gradient divider, top fade |
| **Color** | Neutral grey palette, accent reserved for severity | ✅ OKLCH tokens already well-designed |
| **Icons** | Used sparingly in nav; text labels + status dots in tables | 17 icons in nav config alone |

**The transformation is not cosmetic.** It signals to enterprise buyers that Tamga is a serious operational tool, not a side project.

---

## SPRINT 0: Typography Foundation (Prerequisite)

**Why first:** Font choice cascades into every pixel of every component. Fira Sans was designed as a coding font companion — it has wide letterforms, tall x-height, and a "monospaced feel" that reduces reading speed for dense data tables. Inter is the industry standard for SOC dashboards (CrowdStrike, Sentinel, Wiz all use it).

### 0.1 Swap Fira Sans → Inter for body text
- **File:** `app/layout.tsx` — replace `Fira_Sans` import with `Inter` from `next/font/google`
- **File:** `app/globals.css` — update `--font-fira-sans` → `--font-inter` (or keep variable name, swap source)
- **Keep:** `Fira_Code` for all monospace surfaces (code blocks, log lines, IDs, hashes)
- **Result:** Body text becomes Inter (crisp, narrow, highly readable at 11–13px); monospace stays Fira Code

### 0.2 Remove `font-mono` from non-data surfaces
- **Target:** ~275 occurrences across `app/dashboard/` and `components/dashboard/`
- **Rule:** `font-mono` stays ONLY on: code blocks, JSON payloads, request IDs, timestamps, log lines, table data cells, keyboard shortcuts
- **Remove `font-mono` from:** Page subtitles, card descriptions, button labels, empty state messages, eyebrow text, nav labels, tooltips
- **Verification:** `grep -r "font-mono" app/dashboard/ components/dashboard/ | wc -l` should drop from ~275 to ~80–100

**Effort:** 2–3 hours | **Files:** ~40 | **Risk:** Low (visual-only change)

---

## SPRINT 1: "The Purge" — Slop Eradication

**Goal:** Delete every element that signals "hobbyist demo" to a security professional. This is the highest-impact sprint for credibility.

### 1.1 REMOVE: Fake macOS traffic light dots from TerminalFrame

**Files:**
- `components/dashboard/TerminalFrame.tsx` (lines 35–37) — the primary offender
- `components/dashboard/DashboardErrorCard.tsx` (lines 39–41) — terminal header with dots
- `components/landing/Hero.tsx` (lines 332–335) — landing terminal chrome

**Action on TerminalFrame.tsx:**
Remove the three `<span>` dots and replace with a bare filename label:
```tsx
// BEFORE (lines 33–43):
<div className="flex items-center justify-between border-b ...">
  <div className="flex items-center gap-1.5">
    <span className="h-2 w-2 rounded-full bg-red-500" aria-hidden />
    <span className="h-2 w-2 rounded-full bg-amber-500" aria-hidden />
    <span className="h-2 w-2 rounded-full bg-emerald-500" aria-hidden />
    {filename ? <span className="ml-2 font-mono text-[11px] ...">{filename}</span> : null}
  </div>
  {status ? <div className="shrink-0">{status}</div> : null}
</div>

// AFTER:
<div className="flex items-center justify-between border-b ... px-3 py-1.5">
  {filename ? <span className="font-mono text-[11px] ...">{filename}</span> : <span />}
  {status ? <div className="shrink-0">{status}</div> : null}
</div>
```

**Action on DashboardErrorCard.tsx:**
Remove the entire "Terminal header" (lines 38–49). Replace with a simple red-left-border error card with icon + title.

**Action on Hero.tsx:**
Keep for landing (marketing page, not dashboard). Landings can use terminal aesthetic.

### 1.2 REMOVE: `bg-gradient-to-b` top fade in TerminalFrame

**File:** `components/dashboard/TerminalFrame.tsx` (lines 45–49)

The top fade is a purely decorative element. Remove the `withTopFade` prop, the `<div>` with `bg-gradient-to-b`, and simplify the component.

```tsx
// REMOVE lines 45–49 and the `withTopFade` prop entirely
// The body wrapper becomes:
<div ref={bodyRef} className={cn("relative", bodyClassName)}>
  {children}
</div>
```

Search for all `withTopFade` usages (31 TerminalFrame instances) — most pass `withTopFade={false}` already. Remove the prop from all call sites.

### 1.3 REMOVE: Gradient divider in PageHeader

**File:** `components/dashboard/PageHeader.tsx` (line 33)

```tsx
// BEFORE:
<div className="h-px w-full bg-gradient-to-r from-transparent via-red-600/40 to-transparent" />

// AFTER:
<div className="h-px w-full bg-zinc-200 dark:bg-zinc-800" />
```

A simple `border-b` on the header wrapper is sufficient. The red gradient is a Vercel-template cliché.

### 1.4 REMOVE: Hover glow shadows on MetricStat

**File:** `components/dashboard/MetricStat.tsx` (lines 30–34)

```tsx
// REMOVE the `glowClass` object entirely (lines 30–34)
// Remove the `glowClass[accent]` from the className (line 63)
// Keep the border + subtle hover state change (bg shift on hover is fine)
```

The `hover:shadow-[0_0_20px_-6px_rgba(...)]` pattern on stat cards is pure decoration. Replace with a simple border-color lightening on hover.

### 1.5 REMOVE: backdrop-blur from sticky headers

**File:** `components/dashboard/DashboardLayoutShell.tsx` (lines 123, 149)

```tsx
// BEFORE:
className="sticky top-0 z-20 flex items-center justify-between border-b bg-white dark:bg-zinc-950/95 px-3 py-2 backdrop-blur md:hidden"

// AFTER:
className="sticky top-0 z-20 flex items-center justify-between border-b bg-white dark:bg-zinc-950 px-3 py-2 md:hidden"
```

`bg-zinc-950` is fully opaque — no transparency, no `backdrop-blur` needed. The `/95` alpha and blur are decorative artifacts.

### 1.6 REDUCE: Oversized padding in empty states

**Files:**
- `components/dashboard/security/incidents-console/IncidentsQueueTableCard.tsx:229` — `p-10` → `p-6`
- `components/dashboard/security/SeverityTriagePanel.tsx:95` — `p-8` → `p-5`
- `components/dashboard/DashboardErrorCard.tsx:103` — `py-8` → `py-5`

### 1.7 REDUCE: Icon density in nav config

**File:** `components/dashboard/dashboard-nav-config.tsx` — 17 icon imports

No action needed for now — nav icons serve a real scannability purpose. But flag for Sprint 2 where we may want a 2-level hierarchy that reduces some icon-only leaf nodes to text labels.

### 1.8 STANDARDIZE: Page content padding

**File:** `components/dashboard/DashboardLayoutShell.tsx` (line 188)

```tsx
// BEFORE:
<div className={`transition-colors duration-200 ${focusMode ? "p-2 md:p-3" : "p-3 md:p-4"}`}>

// AFTER:
<div className={focusMode ? "p-2" : "p-4"}>
```

Remove the `transition-colors` — it animates on every focus mode toggle, which is distracting.

**Effort:** 3–4 hours | **Files:** ~40 | **Risk:** Medium (TerminalFrame is a shared component — 31 consumers)

---

## SPRINT 2: Layout & IA Overhaul — The Dense Grid

**Goal:** Replace the vertical card-stack layout with a multi-column CSS grid that maximizes data density on 1080p/1440p screens. This is the single biggest UX improvement.

### 2.1 Design the 12-column enterprise grid

Create a reusable grid system:

```css
/* In globals.css or a new dashboard-grid.css */
.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 0.5rem; /* 8px — XSOAR-grade density */
}

/* At 1920px, no gap increase — keep density */
@media (min-width: 1920px) {
  .dashboard-grid {
    gap: 0.75rem;
  }
}
```

Grid span tokens:
```
col-span-3  — Small stat card (MetricStat)
col-span-4  — Medium panel
col-span-6  — Half-width table/chart
col-span-8  — 2/3 width (main content)
col-span-12 — Full-width table
```

### 2.2 Rebuild the Overview page into a grid

**Files:** `app/dashboard/DashboardOverviewClient.tsx`, `OverviewViewPartA.tsx`, `OverviewViewPartB.tsx`, `OverviewViewPartC.tsx`

Current layout: 3 stacked `<div>` sections (PartA → PartB → PartC), each a vertical stack.

Target layout:
```
┌──────────────┬──────────────┬──────────────┬──────────────┐
│ MetricStat   │ MetricStat   │ MetricStat   │ MetricStat   │  ← row 1: 4 stat cards (col-span-3 each)
│ (requests)   │ (blocks)     │ (latency)    │ (tokens)     │
├──────────────┴──────────────┼──────────────┴──────────────┤
│ ExecutiveRiskBanner         │ ThreatFlowOverview          │  ← row 2: risk banner + flow (col-span-6 each)
│ (col-span-6)                │ (col-span-6)                │
├─────────────────────────────┼─────────────────────────────┤
│ ThreatCorrelationGraph      │ QuickActionsPanel            │  ← row 3: graph + policy status (col-span-6 each)
│ (col-span-6)                │ (col-span-6)                │
├─────────────────────────────┴─────────────────────────────┤
│ Recent Incidents Table (col-span-12)                       │  ← row 4: full-width virtualized table
└───────────────────────────────────────────────────────────┘
```

Remove all `space-y-4` wrappers. Replace with `dashboard-grid`.

### 2.3 Convert TerminalFrame to plain Card

**File:** `components/dashboard/TerminalFrame.tsx`

Rename to `DashboardCard` (or keep name, change internals). The frame should be a thin-bordered container with NO decorative chrome:

```tsx
export function DashboardCard({ title, status, className, children }) {
  return (
    <div className={cn("overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950", className)}>
      {title && (
        <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-3 py-1.5">
          <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400 uppercase tracking-wider">{title}</span>
          {status}
        </div>
      )}
      {children}
    </div>
  );
}
```

- **Remove:** `filename` prop → `title` (more generic, less terminal)
- **Remove:** `withTopFade` prop entirely
- **Remove:** `bodyRef`/`bodyClassName` (consumers that need ref can pass it directly)
- **Keep:** `rounded-sm`, `border`, `bg-white dark:bg-zinc-950`

### 2.4 Deepen sidebar hierarchy

**File:** `components/dashboard/dashboard-nav-config.tsx`

Current: 5 flat groups, 17 items total. Works for now, but needs collapsible sections for future scale.

Add collapsible group headers:
```tsx
export type DashboardNavGroup = {
  label?: string;
  defaultOpen?: boolean;  // NEW
  items: DashboardNavItem[];
};
```

Groups POLICY and SYSTEM should default to collapsed for analysts who only use TRIAGE + ANALYTICS. Persist collapse state in localStorage.

Remove "//" suffix from group labels — it's terminal-larp:
```
// BEFORE: "TRIAGE //" → AFTER: "TRIAGE"
// BEFORE: "ANALYTICS //" → AFTER: "ANALYTICS"
```

### 2.5 Remove `font-mono` from nav labels and group headers

**Files:** `DashboardLayoutShell.tsx`, `DashboardNavList.tsx`, `dashboard-nav-config.tsx`

Navigation should use Inter (body font). Only the version badge (`v0.1.1`) and keyboard shortcut (`Ctrl+K`) retain monospace.

### 2.6 Keyboard-First UX: Global Command Palette hardening

**File:** `components/dashboard/DashboardCommandPalette.tsx` (already exists)

The Ctrl+K palette already exists. Harden it for power users:

- **Ranking:** Recently used commands float to top (persist in localStorage, max 10 entries)
- **Aliases:** `inc` → Incidents, `pol` → Policies, `play` → Playground, `set` → Settings
- **Quick actions:** "Ack all critical," "Assign to me," "Export CSV" — direct action dispatch, not just navigation
- **Keyboard:** `Esc` closes, `Enter` selects first result, `↑`/`↓` navigate, `Ctrl+K` toggles
- **Empty state:** When no query, show "Jump to…" with top 5 pages by visit frequency

**Effort:** 6–8 hours | **Files:** ~15 | **Risk:** High (layout restructure touches every page)

---

## SPRINT 3: "The War Room" — Split-Pane Incident Triage

**Goal:** Transform the incidents page from a simple table into a split-pane "War Room" interface — the signature XSOAR UX pattern. This is the feature that wins enterprise deals.

### 3.1 Architecture: Left Panel (Incident List) + Right Panel (Detail/Action)

```
┌────────────────────────────────┬──────────────────────────────┐
│ INCIDENT QUEUE (60% width)     │ INCIDENT DETAIL (40% width)  │
│ ┌────────────────────────────┐ │ ┌──────────────────────────┐ │
│ │ [Filters: severity ▾       │ │ │ REQUEST ID: abc123...    │ │
│ │  action ▾  status ▾]       │ │ │ TIMESTAMP: 2026-06-12... │ │
│ │ [Search...            ]    │ │ │ PROVIDER: openai / gpt-4 │ │
│ ├────────────────────────────┤ │ │ SEVERITY: ██ CRITICAL    │ │
│ │ ☐ CRIT  BLOCK  openai     │ │ ├──────────────────────────┤ │
│ │ ☐ HIGH   REDACT anthropic │ │ │ FINDINGS (3)             │ │
│ │ ☐ MED    WARN   openai    │ │ │ • jailbreak:direct (95%) │ │
│ │ ☐ LOW    PASS   cohere    │ │ │ • pii:ssn (88%)          │ │
│ │ ☐ CRIT  BLOCK  openai     │ │ │ • toxicity:harassment    │ │
│ │ ... (virtualized, 40+ rows)│ │ ├──────────────────────────┤ │
│ │                           │ │ │ RAW PAYLOAD              │ │
│ │                           │ │ │ { JSON tree view }       │ │
│ │                           │ │ ├──────────────────────────┤ │
│ │                           │ │ │ ACTIONS                  │ │
│ │                           │ │ │ [Ack] [Assign] [Close]   │ │
│ │                           │ │ │ [Open in Playground]     │ │
│ └────────────────────────────┘ │ └──────────────────────────┘ │
└────────────────────────────────┴──────────────────────────────┘
```

### 3.2 Create resizable split-pane primitive

**New file:** `components/dashboard/ResizableSplitPane.tsx`

A thin wrapper using CSS `resize` or a lightweight drag handle (no dependency needed):

```tsx
export function ResizableSplitPane({ left, right, defaultLeftWidth = "60%" }) {
  // Uses CSS grid with a draggable divider
  // Persists width ratio to localStorage per page
}
```

**Requirements:**
- Minimum left panel: 320px
- Minimum right panel: 280px
- Drag handle: 4px wide, visible on hover, `cursor: col-resize`
- Width ratio persisted in localStorage keyed by page route

### 3.3 Refactor IncidentsQueueTableCard into left panel

**File:** `components/dashboard/security/incidents-console/IncidentsQueueTableCard.tsx`

- Remove the `Card` wrapper → becomes just the table (no double-border)
- Reduce row height: `py-2` → `py-1.5` (saves ~2px per row × 40 rows = 80px more data)
- Remove the `<TerminalFrame>` wrapper → plain `<div>` with scroll
- Remove the "Export CSV" button from top-right → move to a toolbar
- Remove checkbox column on narrow screens (add "Select All" in toolbar instead)
- Remove the radial-gradient dot pattern on empty state (lines 230–238) → plain empty message

### 3.4 Build the right detail panel

**New file:** `components/dashboard/security/incidents-console/IncidentDetailPanel.tsx`

This replaces the current `IncidentsEventDetailSheet` (a slide-out sheet). XSOAR-style War Room detail is ALWAYS visible — no toggling.

**Content zones (top to bottom):**
1. **Metadata bar:** Request ID, timestamp, provider/model, user (if available) — compact single row
2. **Severity + Action badges:** Prominent at top
3. **Findings list:** Each finding with type, category, confidence bar, OWASP code
4. **Advanced JSON Inspector** (see 3.4a below)
5. **Action buttons:** Ack, Assign, Close, False Positive, Open in Playground — compact button row
6. **Timeline:** Mini audit log for this incident (status changes, assignments)

### 3.4a ADVANCED JSON INSPECTOR: Collapsible Syntax-Highlighted Payload Viewer

**New file:** `components/dashboard/security/incidents-console/JsonInspector.tsx`

A `<pre>` tag is amateur-hour. The War Room detail panel must render JSON payloads (request body, prompt messages, findings arrays, raw API response) using a purpose-built inspector:

**Feature requirements:**

1. **Collapsible tree view:** Nested objects/arrays are collapsed by default at depth ≥ 3. Click to expand/collapse any node. Expand-all / collapse-all toggles in the toolbar.

2. **Syntax color coding (OKLCH-based, dark-first):**
   - Keys: `oklch(0.72 0.12 230)` (blue) — property names
   - Strings: `oklch(0.72 0.13 150)` (green) — values
   - Numbers: `oklch(0.74 0.13 55)` (amber) — numeric values
   - Booleans: `oklch(0.64 0.16 22)` (red) — `true`/`false`
   - Null: `oklch(0.6 0.01 260)` (muted grey) — `null`
   - Brackets/braces: `oklch(0.48 0.01 260)` (faint grey) — structural

3. **Line numbers:** Monospace, right-aligned, muted, every 5th line labeled.

4. **Search within payload:** Ctrl+F inside the inspector highlights all matches. Enter jumps to next match.

5. **Copy path:** Right-click a key → "Copy JSON path" (e.g., `$.messages[0].content`) and "Copy value."

6. **Truncation guard:** Strings longer than 200 chars are truncated with "Show more…" toggle. Prevents a single base64 image from blowing out the layout.

7. **Performance:** Use `react-virtual` or a lightweight virtualizer if payload exceeds 5,000 lines.

**Do NOT import a heavy dependency.** No `react-json-view`, no `monaco-editor`. Build with vanilla React + Tailwind. The inspector is a read-only viewer, not an editor.

### 3.5 Keyboard navigation for Incident list

**File:** `components/dashboard/security/incidents-console/IncidentsQueueTableCard.tsx`

SOC analysts live on the keyboard. Add Vim-style row navigation to the incident table:

- **`j` / `↓`:** Move selection down one row
- **`k` / `↑`:** Move selection up one row
- **`Enter`:** Open selected incident in detail panel (same as click)
- **`x`:** Toggle checkbox on selected row
- **`Shift+A`:** Assign selected to me
- **`Shift+C`:** Close selected incident
- **`Shift+F`:** Mark selected as False Positive
- **`Esc`:** Clear selection, return focus to search/filter bar

**Implementation:**
- `tabIndex={0}` on the table container with a `onKeyDown` handler
- Visual indicator: selected row gets a 2px left accent border (`border-l-2 border-l-accent`) + subtle background shift
- Keyboard shortcut hint bar at the bottom of the table: `j/k navigate · Enter detail · x select · Shift+A assign · Shift+C close · Esc clear`
- All shortcuts documented in the global Command Palette (Ctrl+K → "Keyboard shortcuts")

### 3.6 Remove the IncidentsEventDetailSheet

**File:** `components/dashboard/security/IncidentsEventDetailSheet.tsx`

After the split-pane detail panel is built, the slide-out sheet is redundant. Delete it.

### 3.7 Update the incidents page layout

**File:** `app/dashboard/security/page.tsx`

```tsx
export default function SecurityPage() {
  return (
    <div className="space-y-2">
      <PageHeader eyebrow="TRIAGE" title="Incidents" subtitle="Real-time security event queue" />
      <ResizableSplitPane
        left={<IncidentsQueueTableCard />}
        right={<IncidentDetailPanel />}
        defaultLeftWidth="62%"
      />
    </div>
  );
}
```

**Effort:** 8–12 hours | **Files:** ~12 (4 new, 7 modified, 1 deleted) | **Risk:** High (core user workflow + new JSON inspector component)

---

## SPRINT 4: Enterprise Polish — Tables, Contrast, Data Density

**Goal:** Systematic refinement of every data surface for SOC-level density and readability.

### 4.1 Standardize table density

**Affected files:** Every component with `<table>` or data grid (~8 files)

Rules:
- Header row: `text-[10px]` uppercase, `py-1.5`
- Data rows: `text-[12px]`, `py-1.5` (max 28px row height)
- Cell padding: `px-2` horizontal (not `px-3`)
- No icon in timestamp column → text only
- Status column: colored left-border accent (4px) instead of full badge background

### 4.2 Remove unnecessary icons from table cells

**File:** `IncidentsQueueTableCard.tsx`
- `Clock3` icon next to timestamp → remove, just show relative time
- `BadgeCheck` icon for high confidence → replace with subtle green dot
- `FlaskConical` icon for "Test in Playground" → keep (it's an action button, not a decoration)
- `Eye` icon for "Incele" → keep (action button)
- `Download` icon for "Export CSV" → keep (action button)

### 4.3 Apply dense content wrapper globally

Reduce the layout shell padding:
- `DashboardLayoutShell.tsx:188`: `p-4` → `p-3` (12px instead of 16px)
- All `space-y-4` → `space-y-2` (8px gap between sections)
- Card internal padding: standardize to `p-3` (12px) for content areas, `px-3 py-1.5` for headers

### 4.4 Dark mode audit (already good, but verify)

The OKLCH token system is already well-implemented. Verify:
- `--fg-muted` contrast against `--surface-card` ≥ 4.5:1 (currently `oklch(0.75 0.01 260)` on `oklch(0.225 0.009 260)` — needs measurement)
- Status badge text on status badge background ≥ 4.5:1
- Border contrast: `--border` at `oklch(0.3 0.01 260)` may be too subtle — consider `oklch(0.33 0.01 260)` for better definition

### 4.5 Final `font-mono` audit

Run: `grep -r "font-mono" app/dashboard/ components/dashboard/ --include="*.tsx"`

Every remaining `font-mono` must be on one of:
- Code/pre blocks
- JSON payload displays
- Request IDs, trace IDs, hashes
- Timestamps in tables
- Keyboard shortcut hints
- Log lines

If it's on a card title, description, button label, nav item, or empty state — remove.

### 4.6 LOCALIZATION: Turkish İ/ı bug — Tailwind `uppercase` + remnant JS `.toUpperCase()` audit

**Context:** A previous sprint (June 2026) created `lib/tr-string.ts` and fixed 67 JavaScript `.toUpperCase()`/`.toLowerCase()` calls across 42 files. However, two attack surfaces remain:

#### 4.6a Tailwind `uppercase` class audit

The CSS `text-transform: uppercase` applied by Tailwind's `uppercase` class IS locale-aware in all modern browsers (Chrome 110+, Firefox 120+, Safari 16+). This means `uppercase` applied via Tailwind is **safe** — it correctly renders Turkish `İ` (İ) and `ı` (I) based on the page's `lang` attribute.

**Action:**
- Verify `<html lang="tr">` is set in `app/layout.tsx` (currently may be missing)
- Audit all `uppercase` Tailwind classes (~220 instances found in previous scan) — confirm they are all CSS-based, not JS-based
- Mark Tailwind `uppercase` as SAFE in code review checklist. Do NOT replace them.

#### 4.6b Remnant JavaScript `.toUpperCase()`/`.toLowerCase()` hunt

Since the original fix (42 files), new code may have introduced fresh unsafe calls.

**Action:**
```bash
# Hunt for any .toUpperCase() or .toLowerCase() NOT using the tr-string utility
grep -rn "\.toUpperCase\(\)\|\.toLowerCase\(\)" app/dashboard/ components/dashboard/ lib/ hooks/ \
  --include="*.ts" --include="*.tsx" | \
  grep -v "tr-string\|toUpperEn\|toLowerEn\|toUpperLocale\|toLowerLocale\|\.test\.\|node_modules"
```

- Any match found → replace with `toUpperEn()` / `toLowerEn()` (comparison) or `toUpperLocale()` / `toLowerLocale()` (display) from `@/lib/tr-string`
- Add a pre-commit checklist item: "No bare `.toUpperCase()`/`.toLowerCase()` — always locale-explicit"
- Add ESLint rule (optional): `no-restricted-syntax` to warn on bare `.toUpperCase()`/`.toLowerCase()`

#### 4.6c Turkish display text audit

Turkish UI strings are scattered inline across the dashboard (e.g., `"Yukleniyor..."`, `"Filtreye uygun olay yok."`, `"API anahtar yönetimi"`). These are fine for v0.1.1 but should eventually move to an i18n solution (next-i18next or similar). Note this as tech debt, not Sprint 4 scope.

### 4.7 STREAM CONTROL: Live data pause-on-hover + micro-action clipboard

**Goal:** Enterprise SOC dashboards have live-updating data streams. Tamga's overview and incident queue poll for new data. Two micro-interactions are mandatory for operator trust:

#### 4.7a Auto-pause live updates on hover

**Affected components:** `MetricStat` (live pulse), `IncidentsQueueTableCard` (infinite scroll polling), any `useQuery` with `refetchInterval`

**Behavior:**
- When the user's cursor enters a data card/table, pause `refetchInterval`/polling for that component
- Show a subtle indicator: the card border shifts from `border-zinc-200` to `border-amber-500/40` (amber = "paused"), and a small `⏸ PAUSED` badge appears in the top-right corner
- On cursor leave, resume polling immediately (no delay)
- Exception: Do NOT pause if the user is actively scrolling (scroll → no pause; hover without scroll → pause)

**Implementation:**
```tsx
// Hook: usePauseOnHover(refetch: () => void, enabled: boolean)
// - Wraps onMouseEnter/onMouseLeave on a container ref
// - When paused, skips refetch() calls but keeps the query cache warm
// - Restores refetch interval on mouse leave
```

#### 4.7b Invisible-until-hover "Copy to Clipboard" buttons

**Problem:** SOC analysts frequently copy IoCs (request IDs, IPs, prompt snippets, API keys) from the dashboard. Right-click → Copy is slow. A visible "Copy" button on every data cell is visual noise. The XSOAR pattern: copy buttons are invisible until hover.

**Affected surfaces:**
- Request IDs in incident table rows
- IP addresses in event explorer
- Prompt text snippets in playground/findings
- API keys in Settings → Access
- Any monospace data cell containing a copyable token

**Behavior:**
- Each copyable value is wrapped in a `<span className="group relative">`
- On hover: a small `📋` icon (or `Copy` text at `text-[10px]`) fades in at `opacity-0 group-hover:opacity-100` with `transition-opacity duration-75` (fast, not animated)
- On click: copies to clipboard via `navigator.clipboard.writeText()`, icon briefly changes to `✓` for 800ms, then reverts
- The copy button is `absolute right-0 top-0` with a `bg-zinc-100 dark:bg-zinc-900` background so it covers the underlying text (prevents overlap)
- Toast notification via `sonner` (already installed): "Copied `abc123...` to clipboard"

**CSS pattern:**
```tsx
<span className="group relative inline-flex items-center">
  <span className="font-mono text-xs">{truncatedValue}</span>
  <button
    className="absolute right-0 top-1/2 -translate-y-1/2 ml-1 rounded-sm bg-zinc-100 dark:bg-zinc-900 px-1 py-0.5 text-[10px] text-zinc-500 opacity-0 transition-opacity duration-75 group-hover:opacity-100 hover:text-zinc-900 dark:hover:text-zinc-200"
    onClick={() => copyToClipboard(fullValue)}
  >
    Copy
  </button>
</span>
```

**Effort:** 5–7 hours | **Files:** ~22 | **Risk:** Medium (global spacing changes + new interaction patterns)

---

## SPRINT 5: The "No LARP" Final Pass

**Goal:** One final sweep to catch any remaining terminal-larp, hobbyist, or web-dev-cliché elements that survived Sprints 1–4.

### 5.1 Audit checklist

- [ ] Zero `bg-gradient-to-*` in `app/dashboard/` and `components/dashboard/`
- [ ] Zero `backdrop-blur` in `app/dashboard/` and `components/dashboard/`
- [ ] Zero `shadow-[0_*` custom glow shadows in dashboard components
- [ ] Zero `rounded-full` on anything larger than a status dot (12px max)
- [ ] Zero `font-mono` on non-data text (verified in 4.5)
- [ ] Zero `p-8`, `p-10`, `p-12` in dashboard components
- [ ] Zero `aria-hidden` on decorative dots (they're all gone after Sprint 1)
- [ ] Zero `motion.div` stagger animations on dashboard content (keep page transitions only)

### 5.2 Landing page firewall

**Principle:** The landing page (`components/landing/`, `app/(marketing)/`) can use terminal aesthetic, gradients, animations, glassmorphism. The dashboard CANNOT.

Ensure no shared components leak landing visual language into the dashboard:
- `TerminalFrame` → if still used in landing, keep the dots there but strip them from dashboard variant
- `Reveal` → already removed from dashboard (Sprint 4, previous session), verify no regressions
- `Hero.tsx` → landing-only, no changes needed

### 5.3 Version badge cleanup

**File:** `DashboardLayoutShell.tsx:65`
```
// BEFORE:
<div className="... font-mono text-[9px] uppercase ...">AI PROXY // v0.1.1</div>

// AFTER:
<div className="... text-[10px] font-medium uppercase tracking-wider text-zinc-500">v0.1.1</div>
```

Remove "AI PROXY //" prefix, remove `font-mono`, simpler version string.

**Effort:** 1–2 hours | **Files:** ~10 | **Risk:** Low

---

## Dependency Graph

```
Sprint 0 (Typography)
  └─► Sprint 1 (The Purge)
        └─► Sprint 2 (Layout Grid)
              └─► Sprint 3 (War Room)
                    └─► Sprint 4 (Polish)
                          └─► Sprint 5 (Final Pass)
```

Sprints 0+1 can be done in parallel (typography changes don't conflict with deleting dots). Sprints 2+3 are sequential (War Room depends on grid system). Sprints 4+5 are cleanup passes.

---

## Effort Estimates

| Sprint | Description | Hours | Files | Risk |
|--------|-------------|-------|-------|------|
| 0 | Typography Foundation (Inter + font-mono reduction) | 2–3h | ~40 | Low |
| 1 | The Purge (dots, gradients, shadows, padding) | 3–4h | ~40 | Medium |
| 2 | Layout & IA Overhaul (grid, card rename, sidebar, keyboard palette) | 6–8h | ~15 | High |
| 3 | The War Room (split-pane, JSON inspector, keyboard nav) | 8–12h | ~12 | High |
| 4 | Enterprise Polish (tables, contrast, density, i18n, stream control, copy) | 5–7h | ~22 | Medium |
| 5 | Final Pass (no-larp audit) | 1–2h | ~10 | Low |
| **Total** | | **25–36h** | **~140** | |

---

## Success Metrics

After all sprints are complete, the dashboard should feel like a tool an enterprise CISO would trust:

1. **Data density:** Overview page shows 4 stat cards + risk banner + correlation graph + policy status + recent incidents table — all above the fold at 1080p (no scrolling for key metrics)
2. **Typography:** Inter body text at 11–13px, Fira Code for data surfaces only, no monospace on UI chrome
3. **Chrome:** Zero decorative elements — no dots, no gradients, no glow shadows, no blur
4. **War Room:** Incidents page loads with split-pane; clicking a row instantly populates the detail panel; JSON payloads rendered with collapsible syntax-highlighted tree view
5. **Keyboard-first:** j/k row navigation in incident list, global Ctrl+K palette with recently-used ranking and action dispatch, all shortcuts documented
6. **Turkish-safe:** Zero bare `.toUpperCase()`/`.toLowerCase()` calls; `<html lang="tr">` set; all string transforms are locale-explicit via `lib/tr-string.ts`
7. **Stream control:** Live data tables auto-pause on hover with amber "PAUSED" indicator; invisible-until-hover copy buttons on all IoCs and identifiers
8. **WCAG AA:** All text meets 4.5:1 contrast ratio in both themes
9. **First impression:** A security engineer seeing the dashboard for the first time should not be able to tell it was built with Tailwind/Next.js/React — it should look purpose-built

---

## Execution Protocol

Copy and paste the following prompt to begin execution:

> Execute **Sprint 0** and **Sprint 1** of `TAMGA_XSOAR_TRANSFORMATION_PLAN.md` in order.
>
> **Sprint 0 — Typography Foundation:**
> 1. Swap `Fira_Sans` → `Inter` in `app/layout.tsx`. Keep `Fira_Code` for monospace. Update `globals.css` variable name `--font-fira-sans` → `--font-inter`.
> 2. Verify `<html lang="tr">` is set in the root layout (this is critical for CSS `text-transform: uppercase` to render Turkish İ/ı correctly in Tailwind's `uppercase` classes).
> 3. Systematically reduce `font-mono` usage — keep ONLY on: code blocks, JSON payloads, request IDs, timestamps, log lines, table data cells, keyboard shortcuts. REMOVE from: page subtitles, card descriptions, button labels, empty state messages, eyebrow text, nav labels, tooltips. Target: reduce from ~275 to ~80–100 occurrences.
>
> **Sprint 1 — The Purge:**
> 1. Remove fake macOS traffic light dots from `TerminalFrame.tsx` (lines 35–37) — replace with bare filename label.
> 2. Remove the terminal header (dots + "ERROR") from `DashboardErrorCard.tsx` (lines 38–49) — replace with simple red-left-border error card.
> 3. Remove `bg-gradient-to-b` top fade from `TerminalFrame.tsx` and delete the `withTopFade` prop. Clean up all 31 call sites.
> 4. Remove gradient divider from `PageHeader.tsx` (line 33) — replace `bg-gradient-to-r from-transparent via-red-600/40 to-transparent` with solid `bg-zinc-200 dark:bg-zinc-800`.
> 5. Remove hover glow shadows from `MetricStat.tsx` (lines 30–34) — delete `glowClass` object, remove from className.
> 6. Remove `backdrop-blur` + `/95` alpha transparency from sticky headers in `DashboardLayoutShell.tsx` (lines 123, 149).
> 7. Reduce oversized padding: `p-10` → `p-6` in `IncidentsQueueTableCard.tsx:229`, `p-8` → `p-5` in `SeverityTriagePanel.tsx:95`, `py-8` → `py-5` in `DashboardErrorCard.tsx:103`.
> 8. Remove `transition-colors duration-200` from content wrapper in `DashboardLayoutShell.tsx` (line 188).
>
> Commit each sprint separately with descriptive messages. Run `npx tsc --noEmit` after each sprint to verify no TypeScript regressions.
