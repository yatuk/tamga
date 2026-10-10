---
name: Tamga Console
description: The operations console of the Tamga LLM security proxy. IBM Carbon, re-skinned in ink on paper, with one red and one ochre.
colors:
  ink: "#161616"
  ink-2: "#525252"
  ink-3: "#6f6f6f"
  ink-hover: "#393939"
  black: "#000000"
  paper: "#f4f4f4"
  sheet: "#ffffff"
  track: "#e0e0e0"
  hover: "#e8e8e8"
  rule: "#c6c6c6"
  mark-checking: "#8d8d8d"
  stamp: "#b82a18"
  stamp-fill: "#e04a35"
  stamp-tint: "#ffd7d0"
  stamp-on-ink: "#e5503b"
  altered: "#7a5000"
  altered-fill: "#d99e1c"
  altered-tint: "#fbe9b8"
  diff-added-tint: "#dff1e2"
  diff-added-text: "#0e4a20"
  diff-removed-tint: "#ffe3de"
  diff-removed-text: "#8f1d0f"
typography:
  figure:
    fontFamily: "Archivo, Arial Narrow, Arial, sans-serif"
    fontSize: "7.5rem"
    fontWeight: 700
    lineHeight: 0.9
    letterSpacing: "-0.02em"
  figure-narrow:
    fontFamily: "Archivo, Arial Narrow, Arial, sans-serif"
    fontSize: "5rem"
    fontWeight: 700
    lineHeight: 0.9
    letterSpacing: "-0.02em"
  verdict:
    fontFamily: "Archivo, Arial Narrow, Arial, sans-serif"
    fontSize: "4.5rem"
    fontWeight: 700
    lineHeight: 0.9
    letterSpacing: "-0.02em"
  title:
    fontFamily: "Archivo, Arial Narrow, Arial, sans-serif"
    fontSize: "2rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.01em"
  step-number:
    fontFamily: "Archivo, Arial Narrow, Arial, sans-serif"
    fontSize: "1.75rem"
    fontWeight: 700
    lineHeight: 1
  statement:
    fontFamily: "Atkinson Hyperlegible Next, Segoe UI, Arial, sans-serif"
    fontSize: "1.375rem"
    fontWeight: 400
    lineHeight: 1.3
  prose:
    fontFamily: "Atkinson Hyperlegible Next, Segoe UI, Arial, sans-serif"
    fontSize: "1.25rem"
    fontWeight: 400
    lineHeight: 1.45
  heading:
    fontFamily: "Archivo, Arial Narrow, Arial, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.3
  lede:
    fontFamily: "Atkinson Hyperlegible Next, Segoe UI, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
  body:
    fontFamily: "Atkinson Hyperlegible Next, Segoe UI, Arial, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.45
  note:
    fontFamily: "Atkinson Hyperlegible Next, Segoe UI, Arial, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.5
  data:
    fontFamily: "Atkinson Hyperlegible Mono, Consolas, Menlo, monospace"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Atkinson Hyperlegible Next, Segoe UI, Arial, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.33
rounded:
  none: "0"
spacing:
  3xs: "0.125rem"
  2xs: "0.25rem"
  xs: "0.5rem"
  sm: "0.75rem"
  md: "1rem"
  lg: "1.5rem"
  xl: "2rem"
  2xl: "2.5rem"
  3xl: "3rem"
  4xl: "4rem"
  5xl: "5rem"
components:
  button-primary:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.sheet}"
    rounded: "{rounded.none}"
  button-primary-hover:
    backgroundColor: "{colors.ink-hover}"
    textColor: "{colors.sheet}"
  button-primary-active:
    backgroundColor: "{colors.ink-2}"
    textColor: "{colors.sheet}"
  header:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    height: "3rem"
  side-nav:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink-2}"
    width: "16rem"
  notice:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
  detail-panel:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
    width: "31rem"
  action-mark-stopped:
    textColor: "{colors.stamp}"
    typography: "{typography.note}"
  action-mark-altered:
    textColor: "{colors.altered}"
    typography: "{typography.note}"
  confirm-irreversible:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.sheet}"
    rounded: "{rounded.none}"
  confirm-recoverable:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.none}"
  notice-proxy-down:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    typography: "{typography.heading}"
    rounded: "{rounded.none}"
  toast:
    backgroundColor: "{colors.sheet}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.none}"
---

# Design system: Tamga console

## Overview

The console is where a security operator reads what the Tamga proxy did with LLM traffic: which requests it stopped, which it redacted, which it let through, and whether the proxy itself is healthy. Its users are SOC analysts and security engineers at a desk, reading request IDs, keys and card numbers for hours. It is English only.

It is built on IBM Carbon (`@carbon/react`) and re-skinned in one stylesheet, `dashboard/app/tamga.css`. Carbon supplies the controls; the console supplies its own type, its own colour rules and a small kit of page parts in `dashboard/components/console`. Everything this document describes is in those two places.

**What is enforced, and where:**

- **One light theme.** There is no theme toggle and no dark theme. The page is Carbon's `g10`; only the header is `g100` (`shell.tsx`). The toaster is fixed to light.
- **Controls are ink.** Every Carbon colour that marks something as interactive, and Carbon's error, success, info and danger colours, are mapped to ink at the top of `tamga.css`. Nothing is coloured for being clickable.
- **Red means stopped.** See [the stamp rule](#the-stamp-rule).
- **Ochre means changed.** See [the ochre rule](#the-ochre-rule).
- **A request that passed gets no colour and no weight.**
- **Square corners.** The only round thing drawn by the console is the 8px status dot.
- **No decoration.** No gradients as surface, no drop shadows, no emoji, no pictograms beside headings. Icons are Carbon's and sit only inside buttons.
- **Nothing animates for arriving on the page.** Motion marks a change: a number changed, a thing arrived, or where you are. With reduced motion nothing tweens at all.
- **A number is said in a sentence, with its unit and its window.** What is not known is said to be not known, never shown as 0.
- **Copy is short, in sentence case, and says what happened and what to do.**
- **Everything that can be clicked can be reached and worked from the keyboard, with a visible focus ring.**
- **Colour literals are not written in components.** The lint rule `tamga/no-hardcoded-color` (`dashboard/eslint.config.mjs`) warns on any hex or colour function outside the stylesheet, and `components/console/design-rules.test.tsx` fails on one. The same test holds the stylesheet to these rules: literals only on `:root`, red and ochre only on the selectors listed below, no radius, no dropped shadow.

## Colors

Tokens are custom properties on `:root` in `tamga.css`, named `--tamga-*`. The front matter carries the same values under the same names without the prefix. Inside the console the Carbon `g10` tokens resolve to the same greys, and both spellings are in use: `--cds-text-primary` is `ink`, `--cds-text-secondary` is `ink-2`, `--cds-text-helper` is `ink-3`, `--cds-background` is `paper`, `--cds-layer` is `sheet`.

### Neutrals

| Token | Meaning | Where |
| --- | --- | --- |
| `ink` | Text, controls, rules that close a head, the focus ring, the header | Everywhere |
| `ink-2` | Secondary text | Ledes, notes, column heads, labels, passed and logged actions |
| `ink-3` | Helper text and placeholders | Navigation group labels, field placeholders, the editor gutter |
| `ink-hover` | An ink button under the pointer | Carbon button tokens only |
| `black` | A link under the pointer | `--cds-link-primary-hover` only |
| `paper` | The page | `body`, the side list |
| `sheet` | A surface raised off the page by a border, not a shadow | The detail panel, notices, code samples, the diff, the editor, toasts |
| `hover` | A row under the pointer, selected, or just arrived; the line being edited | Rows that open a panel, the palette, modal secondary button, `t-arrived`, the editor's active line |
| `track` | The empty part of a bar; the column being read in a chart; text a pattern caught | `.t-split`, `.t-splitkey`, `.t-day[data-at]`, `.t-sample mark` |
| `rule` | Hairlines between rows; the passed part of a column chart; the proxy state in the header; selected text in the editor | Borders, `.t-day span` |
| `mark-checking` | The status dot before the first answer | `.t-dot` only |

### The stamp rule

Red is `stamp` for text and thin marks, `stamp-fill` for bars, `stamp-tint` behind matched text. It says that a request was stopped, and nothing else. It is used in exactly these places:

- The `Stopped` action mark (`.t-action[data-action="BLOCK"]`) and the playground verdict (`.t-verdict[data-action="BLOCK"]`).
- The stopped share of a bar or column (`data-kind="blocked"` in `.t-split`, `.t-splitkey` and `.t-day`).
- Text in a prompt that the policy stopped (`.t-sample mark[data-action="BLOCK"]`).
- The figure of a `Statement` given `stopped`: the count of stopped requests on the overview, when it is not zero.

Two things are red without being a stopped request, and there is no third:

- **The mark.** The full stop of the wordmark is `stamp-fill`, and `stamp-on-ink` in the header, where the darker red would fall under 4.5:1. The red of the mark is where the stamp colour comes from.
- **The diff.** See [the diff](#the-diff).

Red is never used for a destructive button, a validation error, a failed save, a required field, a missed target or a part of the system that is down. Those are said in ink, in words and weight:

| State | How it is said |
| --- | --- |
| Latency above its target (Latency) | The figure stays ink. The line under it ends with "That is above the 2 s target." in 600. On the chart the target is a dashed ink rule with its name beside it in ink, 600. |
| Token budget used up (Token costs) | The bar is ink and full. The key beside it says "Nothing left: requests are being refused" in ink, 600. |
| Proxy down (Proxy status) | The figure is the word "Down" in ink at 7.5rem; the sentence says the proxy is not answering its health check. |
| Proxy not answering (every page) | The notice above the page. See [notices](#notices-and-toasts). The header dot is hollow and the words beside it say "Proxy unreachable". |
| Text a pattern caught (Patterns) | `track` behind the text with a 2px ink underline. A pattern only detects; what happens to a match is the policy's decision. |
| Something removed for good | The solid ink button, whose words name what will be gone. See [the two-step confirm](#the-two-step-confirm). |

Carbon's `danger` button is not used; its tokens, and Carbon's error, success and info colours, are all ink here, so a Carbon part cannot arrive in red.

### The ochre rule

Ochre is `altered` for text, `altered-fill` for bars, `altered-tint` behind text. It says that something was changed: a request redacted or stripped (`REDACT`, `STRIP`), the redacted share of a bar, text that would be redacted, the paths a policy write changed (`.t-changed`), and `Unsaved changes` on the policy editor. It is not a warning colour. A warned request is ink with a hollow square. It is not a syntax colour either: the policy editor tells keys, strings and numbers apart in ink.

### State without colour

Where you are is marked in ink: a 3px bar beside the current section, an ink rule above and below the selected row. A part of the system that is failing gets a solid ink square; one that is struggling, and a warned request, get a hollow one. The header dot is filled when the proxy is up and hollow when it is down, and the words beside it say which.

### The diff

The diff is the one sanctioned exception to both colour rules. Removed lines are red and added lines are green by a convention older than this console, and a reader who has seen a diff before expects it. The four tokens `diff-added-tint`, `diff-added-text`, `diff-removed-tint` and `diff-removed-text` are used only by `.t-diff`; it is the one place green appears, and the one red that is not a stopped request or the mark. The `+` and `-` in the gutter carry the meaning; the tint backs them up.

### Contrast

Computed with the WCAG 2.1 formula from the token values. AA is 4.5:1 for text and 3:1 for graphics.

| Text | On | Ratio |
| --- | --- | --- |
| `ink` | `paper` / `sheet` / `hover` | 16.45 / 18.10 / 14.77 |
| `ink-2` | `paper` / `sheet` / `hover` | 7.10 / 7.81 / 6.38 |
| `ink-3` | `paper` / `sheet` | 4.57 / 5.02 |
| `stamp` | `paper` / `sheet` / `hover` | 5.64 / 6.21 / 5.07 |
| `altered` | `paper` / `sheet` / `hover` / `altered-tint` | 6.42 / 7.06 / 5.76 / 5.87 |
| `ink` | `stamp-tint` / `altered-tint` | 13.69 / 15.04 |
| `sheet` | `ink` / `ink-hover` / `ink-2` (buttons) | 18.10 / 11.55 / 7.81 |
| `paper`, `rule` | `ink` (header) | 16.45 / 10.59 |
| `stamp-on-ink` | `ink` (the wordmark's full stop in the header) | 4.78 |
| `ink` | `track` (text a pattern caught) | 13.71 |
| `ink` / `ink-2` | `rule` (selected text in the editor) | 10.59 / 4.57 |
| `diff-added-text` | `diff-added-tint` | 8.82 |
| `diff-removed-text` | `diff-removed-tint` | 7.36 |

No text is under AA in any state:

- A row that has just arrived starts on `hover`, not on `rule`, and fades to nothing over 2.4 s. At its darkest the ground is `hover`, where `stamp` is 5.07, `altered` 5.76 and `ink-2` 6.38; with reduced motion that ground is held for the 2.4 s and then gone. Before, the ground was `rule`: `stamp` 3.63, `altered` 4.13.
- The full stop of the wordmark in the header is `stamp-on-ink`, 4.78 on `ink`. Before, it was `stamp-fill`, 4.48. On the sign-in frame it is `stamp-fill` on `paper`, 3.67, at 1.25rem and 700, which is large text.
- `ink-3` on `hover` is 4.10, so helper text is never placed on a hovered row or on the editor's active line. The editor's line numbers are `ink-2` for that reason (6.38 on the active line).

In the policy editor, on `sheet` / on the active line (`hover`) / inside a selection (`rule`): keys and numbers in `ink` are 18.10 / 14.77 / 10.59; strings, comments and punctuation in `ink-2` are 7.81 / 6.38 / 4.57.

Fills are not asked to reach 3:1 against the page or each other; a 1px `ink` hairline is, and does:

| Fill | Against | Ratio | What closes it |
| --- | --- | --- | --- |
| `stamp-fill` | `paper` | 3.67 | Passes alone |
| `altered-fill` | `paper` / `track` | 2.16 / 1.80 | An ink hairline: 7.63 against the fill |
| `stamp-fill` | `altered-fill` beside it | 1.70 | An ink hairline between them: 4.48 and 7.63 |
| `rule` (passed) | `paper` | 1.55 | An ink hairline on top of the part: 10.59 |
| `track` | `paper` | 1.20 | An ink hairline around the track: 13.71 against the track, 16.45 against the page |

So `.t-split` has a 1px ink outline and each part of it ends in an ink hairline; each part of a column (`.t-day span`) is capped by one; and the squares of a key (`.t-splitkey`) are outlined. The two meaning colours are unchanged. No chart depends on its fills in any case: each has a readout line, a key in words with the percentages, or an `aria-label` that states the numbers.

Still under 3:1, and not needed to read anything: the `track` ground behind the column being pointed at (`.t-day[data-at]`, 1.20 on `paper`), which repeats what the readout line says; the sides of a column, which carry no value; and the `hover` ground of a row (1.11 on `paper`), which comes with the pointer or the 2px focus ring.

## Typography

Three faces, loaded with `next/font/google` in `dashboard/app/layout.tsx` as variable fonts and exposed as `--font-atkinson`, `--font-archivo` and `--font-atkinson-mono`. IBM Plex is not loaded; `tamga.css` replaces it on every Carbon class and sets Carbon's letter-spacing to 0.

- **Atkinson Hyperlegible Next** (`--tamga-font-text`): all text. Chosen because it keeps 0/O and 1/l/I apart.
- **Archivo**, set narrow with `font-stretch` (`--tamga-font-head`): `h1`, `h2`, figures, step numbers, the wordmark. Width is 85% for the title, 88% for headings, 80% for the wordmark and 75% for figures.
- **Atkinson Hyperlegible Mono** (`--tamga-font-data`): only where the text is data. Request IDs, key prefixes, paths, patterns, code, the diff. Use the class `t-data`.

Numbers in table cells use tabular figures. Times and dates are formatted `en-GB`, numbers `en`.

### Scale

| Step | Size | Weight | Line height | Use |
| --- | --- | --- | --- | --- |
| `title` | 2rem | 600 | 1.15 | The page's one `h1` |
| `step-number` | 1.75rem | 700 | 1 | The number of a step in a guide |
| `statement` | 1.375rem | 400 | 1.3 | The sentence beside a figure |
| `prose` | 1.25rem | 400 | 1.45 | A written summary (Reports); the wordmark on the sign-in frame |
| `heading` | 1.125rem | 600 | 1.3 | `h2`; the wordmark in the header (700) |
| `lede` | 1rem | 400 | 1.5 | The sentence under the title; the line under a statement; a step's title (600) |
| `body` | 0.875rem | 400 | 1.45 | Lists, navigation, guide steps, toasts. Carbon's own body size |
| `note` | 0.8125rem | 400, or 600 for a state | 1.5 where set | Notes, footnotes, readouts, action marks, the proxy state |
| `data` | 0.8125rem | 400 | 1.5 where set | Mono: samples, the diff, IDs in rows, the policy editor |
| `label` | 0.75rem | 400 | 1rem where set | Column heads, navigation group labels, axis labels, `h3` in the detail panel |

Body text that runs to more than one line is set at 1.45: the navigation, the role list, plain lists, the steps of a guide and toasts. Two roles at 0.875rem keep another value because they are a single line whose height comes from the row, not from the text: a table row (Carbon's compact 1.2857, and 1rem in `.t-kinds`) and the value on a `Scale` mark (1.2, stacked over its label). Rows that set no line height of their own (`.t-facts`, `.t-queue`, `.t-hunts`, `.t-destinations`, the palette) inherit Carbon's `body { line-height: 1 }`; see [known gaps](#known-gaps).

Weights in use are 400, 600 and 700. 600 is for a heading, the current section, a state said in words and a value inside a sentence. 700 is for figures, the wordmark and the diff gutter. There is no uppercase and no letter-spacing on labels.

### Named exceptions

- **`figure`, 7.5rem.** The one number or word a page is about, in a `Statement`. One per page.
- **`figure-narrow`, 5rem.** The same figure below 72rem.
- **`verdict`, 4.5rem.** The playground's one-word answer, which is a word and has to fit beside its sentence.

Anything else at these sizes is a mistake. A new text size is a change to this table first.

## Layout

### The shell

`TamgaShell` (`components/console/shell.tsx`) draws three things and every page goes inside it:

- **Header**, fixed, 3rem, ink. The wordmark `TAMGA.` on the left; on the right the proxy state in words (`Proxy up, scanning in 4 ms`) and the search button that opens the palette. The first tab stop is a skip link to `#main`.
- **Section list**, fixed, 16rem, always open. Five groups from `nav.ts`: Overview, Triage, Analytics, Policy, System. It does not collapse and has no icons. To add a page, add it to `navGroups`; the palette picks it up from there.
- **Main**, `margin-left: 16rem`, padding `5rem 2rem 4rem`, at most 106rem wide. When the proxy is not answering, the notice `t-proxydown` sits above every page.

`html` has `scroll-padding-top: 4rem` so anything scrolled to clears the header.

### Breakpoints

The console is a desk tool. The breakpoints keep a narrow or zoomed window readable; there is no phone design.

| At or below | What changes |
| --- | --- |
| 90rem | The detail panel is 26rem, not 31rem; clipped cells narrow to 12rem while it is open |
| 72rem | `Statement`, `.t-columns` and `.t-hunt` become one column with a 2.5rem gap; the page head stacks; the figure drops to 5rem; hunt fields go from three columns to two |
| 56rem | The section list leaves the side and sits above the page in columns; main loses its left margin and pads `2rem 1rem 4rem`; figure lines and control rows wrap. Reached by zooming a desk window to 200% |

### Space

Space is written as rem literals on Carbon's spacing steps, in the stylesheet and in the inline `style` of a component alike; an inline margin or padding is always one of the steps below. The step names in the front matter exist only in this document; the stylesheet has no spacing variables.

| Step | Value | Typical use |
| --- | --- | --- |
| `3xs` | 0.125rem | Inside a chip |
| `2xs` | 0.25rem | Between a control and the note under it |
| `xs` | 0.5rem | Mark to its word; cell padding; a label to its chart |
| `sm` | 0.75rem | Under a section head; rows of a list |
| `md` | 1rem | The default gap; panel padding; between fields |
| `lg` | 1.5rem | Under the page head; above a table or a notice |
| `xl` | 2rem | Page side padding; between the title and its controls |
| `2xl` | 2.5rem | Above a statement; between stacked columns |
| `3xl` | 3rem | Above a section that follows another; an empty state's padding |
| `4xl` | 4rem | Between two columns; the foot of the page |
| `5xl` | 5rem | The top of the page, to clear the header |

Also in use between steps: 0.375rem and 0.625rem for tight rows, 1.25rem and 1.75rem around the statement and its bar. Buttons that belong together are 1px apart (`.t-buttons`, `.t-confirm`), and columns of a time chart 2px.

Text measures: a lede is at most 68ch, a footnote 76ch, a step 72ch, prose 38rem.

### How a page is put together

A page is its head, an optional notice, the statement if it has one figure to state, then sections. Taken from `traffic/TrafficBody.tsx`:

```tsx
// No key: the title, what the page is for, and why there is nothing.
if (!adminKey) return <NoData title="Traffic" lede="…" adminKey={adminKey} />;

return (
  <>
    <PageHead title="Traffic" lede="How much went through the proxy in the last 7 days, and where it was headed.">
      <RangeSwitch value={range} onChange={setRange} />        {/* the window, kept in the URL */}
      <Button kind="tertiary" size="sm">Export CSV</Button>     {/* the page's own actions */}
    </PageHead>

    <DataNotice adminKey={adminKey} failed={hasError} />        {/* renders nothing unless the read failed */}

    <Statement
      loading={isLoading}
      figure={<Count value={totalRequests} />}                  {/* the one number */}
      sentence="requests went through the proxy in the last 7 days."
      facts={[["Average per hour", "…"], ["Providers in use", "…"]]}
    />

    <Section id="volume" title="Requests by day">
      <Columns label="…" idle="…" columns={…} />
    </Section>

    <div className="t-columns" data-even="true">               {/* two sections side by side */}
      <Section id="providers" title="Requests by provider">
        <Shares head={["Provider", "Requests"]} rows={…} empty="No request reached a provider in this window." />
      </Section>
      <Section id="models" title="Requests by model family">…</Section>
    </div>
  </>
);
```

A section that follows another section is given `marginTop: "3rem"` inline. `.t-columns` is 9:7 by default and 1:1 with `data-even="true"`.

## Elevation & depth

There is none. Surfaces are told apart by a 1px border and by `sheet` on `paper`. `box-shadow` appears in the stylesheet only as an inset, to draw a rule or a mark without changing layout: the ink lines above and below a selected row, the 3px bar on the selected palette entry and on a row that has just arrived, the 2px underline on matched text, the hairlines that close the parts of a bar, the hollow square and the hollow dot. The shadows that Carbon and the toast library ship are removed: a dropdown menu is set apart by a 1px ink outline, a dialog by a 1px ink border on Carbon's overlay, a toast by a 1px ink border.

The one `linear-gradient` (`.t-scaleline`) draws a 1px ink line through the middle of a box. It is a rule, not a surface.

Borders:

- **1px `ink`**: closes a page head, a table head and a chart's baseline; frames a notice, the detail panel, the editor, a toast, a dialog, an open menu and the track of a bar; separates the parts of a bar.
- **1px `rule`**: between rows, under a section head, around a code sample and the diff.
- **2px `ink`**: the stem of a `Scale` mark; the focus ring.
- **3px `ink`**: the current section; the top of the notice that the proxy is not answering; the leading edge of a row that has just arrived.

## Shapes

Corners are square: `rounded.none` is the only step, and `tamga.css` squares what Carbon ships rounded: the content switcher, the toggle, the checkbox, the tooltip and the copy confirmation. The exceptions are the status dot (`.t-dot`, 0.5rem, `border-radius: 50%`) and whatever Carbon draws round by nature, such as a radio button.

The mark is the wordmark `TAMGA.` in Archivo 700 with its full stop in `stamp-fill` (`stamp-on-ink` in the header). The favicon (`dashboard/app/icon.svg`) is a white T on an ink square with a red square at its foot.

An action is a mark, not a pill: a 0.625rem square in the colour of the action, then the word. Passed and logged have no square.

## Components

### The kit

All in `dashboard/components/console`.

- **`PageHead`** (`page.tsx`). The title, one sentence on what the page answers, and the page's controls on the right. One per page; it holds the only `h1`. The lede names the window when the page has one.
- **`RangeSwitch`**. 24 hours, 7 days, 30 days. Use it with `useRangeParam` so the window is in the URL. Not for any other choice of three.
- **`Section`**. An `h2` with an optional note on the same line, closed by a hairline, then the content. The note is for what the reader must know to read the section ("Findings, not requests"), not for a count.
- **`Statement`**. The one figure the page is about at 7.5rem, a sentence that completes it, an optional second line and bar, and a plain list of facts beside it. Not a row of tiles: if a page has four equal numbers, they are facts or a table. The figure is ink. Pass `stopped` only where the figure counts stopped requests and is not zero; a bad answer of any other kind stays ink.
- **`Shares`**. A short ranked table with each row's share drawn as an ink bar in the row. For up to a dozen rows of one measure. Not for anything with actions in the row; use a Carbon table.
- **`Columns`** (`charts.tsx`). One column per hour or day, stacked by stopped, redacted, passed, or a single ink series. Pointing or arrow keys read a column out in the line above.
- **`Trace`**. One measure over time as a line, with an optional limit drawn as a dashed ink rule with its name. Gaps where nothing was measured. One measurement is said as a sentence instead.
- **`Scale`**. A handful of values of the same measure as marks on one line, where the distance between them is the point (the latency percentiles). Not for more than about five marks.
- **`Count`** (`motion.tsx`). A number that counts to its new value. For the figure of a `Statement` only.
- **`Bar`**. A bar as long as its share, which moves from where it was when the data changes. Every bar in the console is this. With reduced motion it is a plain element at its share.
- **`Arriving` / `Arrive`**. A short list whose rows may join while it is on screen. Rows present at first draw do not move.
- **`EventTable`, `ActionMark`, `Pager`** (`events.tsx`). Requests as rows with the open one in a panel beside them; the mark for what was done to a request; previous and next.
- **`DataNotice`, `NoData`, `Refused`, `HealthDetailNotice`**. See states below.
- **`Remove`**. See the two-step confirm below.
- **`Verdict`** (`verdict.tsx`) and **`Diff`** (`diff.tsx`). The playground's answer; a unified line diff for policy changes.
- **`CommandPalette`** (`palette.tsx`). Ctrl K or the header's search button. Pages, a few filtered views and common actions; `incident <id>` and `provider <name>` jump straight there.
- **`names.ts`**. The words: `ACTION_WORDS` (BLOCK is "Stopped"), `kindName`, `providerName`, `counted(2, "request")`, `RANGE_WORDS`, the date formats. Use these; do not show an API enum.

### States

- **Loading.** A route shows `SkeletonText` where the title, lede and body will be (`loading.tsx`). A `Statement` takes `loading`. A table shows `DataTableSkeleton` with no head and no toolbar. There are no spinners; the playground shows Carbon's `ProgressBar` with a count while a batch of prompts runs.
- **Empty.** One sentence in `<p className="t-empty">` that says what is absent in this window, and what to do if something can be done: "No keys yet. Create one for each application that sends requests through the proxy." No illustration, no icon.
- **No admin key.** `NoData` replaces the body: the page's head, then an info notice, "No admin key in this browser. Add one under Settings to read the proxy's data." The page never draws empty figures in this state. A single value the proxy keeps back reads `Needs an admin key`.
- **Read failed.** `DataNotice` under the page head. It tells apart a proxy that did not answer, one that is up and refused the key, one that is up and did not return this page's data, a role too low to read the page, and a session that has ended, and words each differently.
- **Refused.** The console cannot know a key's role beforehand, so the button is there for everyone. When the proxy refuses an action, `Refused` puts what it said beside the button that asked, in ink, as `role="alert"`.
- **Action failed.** A sentence in `.t-failed` under the control ("That did not save. The proxy refused the change. Try again."), or a toast.
- **Page threw.** `error.tsx` keeps the shell and says the page failed, not the proxy, with a "Try again" button.
- **Not known.** `Not known`, `Not recorded`, `Not reported`, `Not measured`, `No data`. Never 0 and never a dash.

### Notices and toasts

A notice is Carbon's `InlineNotification` or `ActionableNotification`, always `lowContrast`, drawn as a 1px ink box on `sheet` with no icon and no thicker edge. Its `kind` is kept for assistive technology and makes no visual difference, so the title has to say what kind of news it is. The title is a full sentence; the subtitle says what to do.

One notice is heavier than the rest: `t-proxydown`, which the shell puts above every page while the proxy is not answering. It is not red, because nothing was stopped. It is the same ink box on `sheet` with a 3px ink rule across its top, its title ("The proxy is not answering.") in the `heading` step, its second line ("Requests may be going out unscanned.") in 600, and a button to Proxy status. Nothing else in the console uses that rule or that size in a notice.

A toast (`lib/toast.ts`, Sonner, bottom right) reports a save, a failure or a copy: an ink box on `sheet`, 0.875rem, no colour. Use a toast for the result of an action the user just took; use a notice for a state of the page.

### Tables

Carbon's `Table`, redrawn: no grey band and no white slabs. The head is `label` text in `ink-2` closed by an ink rule; rows are ink on the page with a hairline between them. Wrap in `TableContainer className="t-table"`; cells do not wrap unless given `t-under`, long cells take `t-clip`, numbers take `t-right`. Size `sm` for rows that are only read or that open a panel, `md` for rows with controls. The request table sits in `.t-work` instead of `.t-table`, which adds the panel's column.

Rows that open a panel (`rows.ts`):

- `useRows` makes the table one tab stop. Up, Down, Home and End move between rows; Enter or Space opens the row. The focused row has the 2px ink ring; the open row has the `hover` ground with ink rules and `aria-current`.
- `usePanelFocus` moves focus into the panel when it opens and back to the row when it closes.
- `useEscape` closes the panel, unless a dialog is open or a field is being typed in.
- The open request is in the URL as `?request_id=`. While the panel is open the table drops the columns the panel repeats.
- `useSwap` says whether a table body holds a later set of rows. The first rows a table shows are drawn still; `data-swap`, which the stylesheet fades on, is set only after the window, a filter or the page has changed.
- A row hovers only where it opens something (`.t-work`).

Charts are focusable (`role="img"`, `tabIndex={0}`); Left, Right, Home and End read each bucket into the `aria-live` line above.

### The two-step confirm

`Remove` asks in place, without a dialog. The bin, or a worded ghost button, turns into two buttons 1px apart: the answer and the way out. Focus lands on the way out, so a second Enter keeps the thing; Escape does the same; afterwards focus returns to where it was asked.

Neither answer is red. The two kinds are told apart by the button and by its words:

| The thing | The answer | Its words |
| --- | --- | --- |
| Is gone for good (the default) | Solid ink (`primary`) | Name what will be gone: "Delete the pattern", "Delete the entity", "Delete the search", "Disconnect the destination", "Revoke the key for good", "Reject the proposal", "Discard the draft", "Remove access and sign out" |
| Can be had back (`recoverable`) | Outlined ink (`tertiary`) | The verb: "Sign out" |

The way out is always a ghost button: "Keep", or what is kept where that would not say enough ("Keep signed in"). `verb` is required, so no answer is left as a bare "Delete".

A dialog (`Modal`) is for a form, or for something that must be answered, such as a new key shown once (`t-reveal`, which has no close button). In a dialog footer the way out is quieter than the way on.

### Buttons and fields

Carbon's, in three kinds. `primary`, ink fill: the one action a page or panel is for, and the answer that cannot be taken back. `tertiary`, ink outline: a second action, and the answer that can. `ghost`: row actions, paging, the way out. `danger` is not used. Sizes are `sm` in a page head and in rows, `md` in a panel or a form, `lg` for the one button on the sign-in frame. An icon appears only inside a button, from `@carbon/icons-react`. Links are underlined, in ink.

Fields are Carbon's with their visible labels. Filters above a table go in `.t-filters`; a form beside a list goes in `.t-form`, with `.t-pair` for two fields on a line and `.t-buttons` for its actions.

## Motion

Motion says one of three things: a number changed, a thing arrived, or where you are. A page that has just loaded has not changed, so it is drawn as it is: every `AnimatePresence` has `initial={false}`, `Bar`, `Trace` and `Scale` start at their value, and the first rows of a table are still (`useSwap`). The one table whose rows fade the first time is the playground's batch result, which does not exist until a run has been asked for: it is the answer to an action, not a page arriving.

Two curves, both Carbon's, exported from `motion.tsx`: `PRODUCTIVE` `cubic-bezier(0.2, 0, 0.38, 0.9)` for small, frequent moves and `EXPRESSIVE` `cubic-bezier(0.4, 0.14, 0.3, 1)` for the few that should be noticed.

| What | Duration | Curve |
| --- | --- | --- |
| Navigation link hover | 110ms | productive |
| Panel leaving; triage state swap | 160ms | productive |
| A later set of rows fading in (`t-rows`, on `data-swap`); panel content changing | 200ms | productive |
| A row or a verdict arriving | 240ms | productive |
| Detail panel opening, and its grid track | 280ms | expressive |
| The section mark travelling | 320ms | expressive |
| A bar, a line or a scale mark moving to a new value | 400ms | expressive |
| The figure counting | 900ms | expressive |
| The ring on the status dot, once per answer from the proxy | 1.6s | productive |
| The mark on a row that arrived while you were looking: the `hover` ground and a 3px ink bar on its leading edge | 2.4s | productive |

**Reduced motion.** With `prefers-reduced-motion: reduce` nothing tweens; everything is drawn at its end state.

- `Motion` wraps the shell in `MotionConfig reducedMotion="user" skipAnimations`, so every `motion` element inside it jumps: the panel, its content, the triage state, a row or a verdict arriving, the saved-search form's height, the section mark.
- `Bar` and `Scale` draw plain elements at their value, `Count` writes the new value at once, and `Trace` redraws with no transition.
- The stylesheet turns off the navigation hover fade, the panel's track transition, the row fade and the dot's ring.
- The mark on a new row is the one thing kept, because it is information: it is held for 2.4 s and then gone, with no fade, on a ground where every action mark is above 4.5:1.

Carbon's own transitions (the dialog, a menu opening) carry Carbon's reduced-motion rules, and the toast library turns its own off.

## Writing

The voice is a colleague reading the log aloud: what happened, in the past tense, then what to do. Sentence case everywhere, including buttons and headings. Full sentences end with a full stop; labels do not. No exclamation marks, no "please", no "oops".

- Say the thing, not its code. `BLOCK` is "Stopped", `tc_kimlik` is "TC identity number", `timeout` is "timed out".
- A number comes with its unit and its window: "of 1,204 requests were stopped before they reached a provider", in "the last 7 days".
- Say who did it. "The proxy did not accept the admin key", not "Authentication failed".
- A button says what it does: "Close as handled", "Revoke the key for good", "Open the queue".
- When nothing is known, say so: "Not known", "No request has been scanned since the proxy started."

Before, from the previous console, and after, from this one:

| Before | After |
| --- | --- |
| Admin key required | No admin key in this browser. Add one under Settings to read the proxy's data. |
| Could not load this data | The proxy did not answer. Check that it is running and that this console points at it. |
| No API keys | No keys yet. Create one for each application that sends requests through the proxy. |
| Proxy healthy | Proxy up, scanning in 4 ms |
| Open command palette | Search or jump to a page (Ctrl K) |

## Do's and don'ts

### Do

- **Do** open a page with what it answers, then the one figure, then the evidence.
- **Do** use `NoData` when there is no key or the first read failed, so empty figures are never read as "nothing happened".
- **Do** put the window and the open row in the URL.
- **Do** give every chart a readout or a key in words.
- **Do** use the `t-` classes and the kit. If a page needs a new part, add it to `tamga.css` and to this document.

### Don't

- **Don't** use red for delete, error, required, over a limit or down. Red is a stopped request.
- **Don't** use ochre for a warning or for syntax. Ochre is a changed request or a changed document.
- **Don't** colour a control to show it is clickable, or a row to show it passed.
- **Don't** add a radius, a drop shadow, a gradient, an emoji or an illustration.
- **Don't** animate something for appearing. Animate it for changing, and not at all under reduced motion.
- **Don't** show 0, a dash or an empty chart for something the proxy did not report.
- **Don't** write a colour literal in a component, or a text size that is not in the scale.
- **Don't** put more than one figure at 7.5rem on a page, or use `Count` on ordinary numbers.

## Known gaps

- **No dark theme**, by decision.
- **No phone layout.** Below 56rem the page reflows for a zoomed desk window; it is not designed for touch.
- **Fills under 3:1.** The meaning colours of a bar do not reach 3:1 against the page or each other; ink hairlines close them instead. What is still under 3:1 is listed at the end of [Contrast](#contrast).
- **Rows with no line height of their own.** Carbon sets `body { line-height: 1 }` and `.tamga` does not replace it, so text in `.t-facts`, `.t-queue`, `.t-hunts`, `.t-destinations`, `.t-issues`, `.t-empty`, `.t-note` and the palette is set solid. It reads as intended while it stays on one line and is cramped where it wraps (a long note, an empty state in a narrow column). Setting 1.45 on `.tamga` would fix it and would also make every such row about 6px taller, which is a change to every page and has not been made.
- **Space is not tokenised.** Values are rem literals, in the stylesheet and in about 110 inline `style` props, most of them a `marginTop` on a `Section` or a notice. All are on the scale; none is checked by a tool.
- **Toasts carry the library's icon.** A success or failure toast shows Sonner's small glyph before its text, in ink. The rule is that icons sit only inside buttons.
- **The editor's selection hides on the active line.** CodeMirror draws the selection behind the text, and the active line's `hover` ground is opaque, so a selection is not visible on the line the cursor is on. It is visible on every other line.
- **Proxy status keeps the last answer while a check fails.** The figure says "Down" and the HTTP server row says "Failing", but the uptime line, the facts and the other rows are the last ones the proxy gave, and are not marked as old.
- **Native menus.** `Select` opens the operating system's own menu, which this stylesheet cannot draw.
- **Carbon's warning colour is not mapped.** No field or notice in the console uses `warn` or `kind="warning"`; one that did would arrive in Carbon's yellow.
