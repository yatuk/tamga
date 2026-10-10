"use client";

import { json } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { Decoration, EditorView, ViewPlugin, type DecorationSet, type ViewUpdate } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import CodeMirror from "@uiw/react-codemirror";

// Ink on paper, as the rest of the console. Every colour is a token read by the
// stylesheet: a CodeMirror theme is turned into CSS rules, so var() works in it.
// Nothing is red or ochre: those mean a request was stopped or changed. The
// parts of the document are told apart by the ink steps, weight and slope:
// keys in ink and bold, numbers and true/false/null in ink, strings in the
// second ink, comments in the second ink and sloped.
export const SELECTION =
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, & > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-content ::selection";

export const theme = {
  "&": { backgroundColor: "var(--tamga-sheet)", color: "var(--tamga-ink)", fontSize: "0.8125rem" },
  ".cm-content": { fontFamily: "var(--tamga-font-data)", caretColor: "var(--tamga-ink)" },
  ".cm-gutters": {
    backgroundColor: "var(--tamga-sheet)",
    // The second ink, not the third: a line number also sits on the active line's grey.
    color: "var(--tamga-ink-2)",
    border: "none",
    borderRight: "1px solid var(--tamga-rule)",
  },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "var(--tamga-hover)" },
  // Spelled as CodeMirror spells its own rule for a focused editor, which a
  // shorter selector loses to: the selection was drawn in the library's
  // lavender. The text inside keeps its own ink (see `::selection` in tamga.css).
  [SELECTION]: {
    backgroundColor: "var(--tamga-rule)",
  },
  "&.cm-focused": { outline: "2px solid var(--tamga-ink)", outlineOffset: "-2px" },
  ".cm-cursor": { borderLeftColor: "var(--tamga-ink)" },
};

export const highlight = [
  { tag: tags.propertyName, color: "var(--tamga-ink)", fontWeight: "600" },
  { tag: [tags.number, tags.bool, tags.null], color: "var(--tamga-ink)" },
  { tag: tags.string, color: "var(--tamga-ink-2)" },
  { tag: tags.comment, color: "var(--tamga-ink-2)", fontStyle: "italic" },
  { tag: tags.punctuation, color: "var(--tamga-ink-2)" },
];

/**
 * Where the ground of the line being edited is drawn: the line of each
 * cursor, and nowhere once anything is selected.
 *
 * CodeMirror draws a selection behind the text and the active line's ground
 * on the line itself, so the ground, which is opaque, covered the selection
 * on the one line the cursor was on. A line is either the one being typed
 * on or part of what is selected, not both: with a selection there is no
 * ground, and the selection shows on every line. The line number in the
 * gutter keeps its mark either way.
 *
 * `ranges` are the selection's; `lineStart` gives the start of the line a
 * position is on. Returns the line starts to mark, in document order.
 */
export function cursorLines(ranges: readonly { from: number; to: number; head: number }[], lineStart: (pos: number) => number): number[] {
  if (ranges.some((r) => r.from !== r.to)) return [];
  return [...new Set(ranges.map((r) => lineStart(r.head)))].sort((a, b) => a - b);
}

const activeLine = Decoration.line({ class: "cm-activeLine" });
const marked = (view: EditorView) =>
  Decoration.set(cursorLines(view.state.selection.ranges, (pos) => view.state.doc.lineAt(pos).from).map((from) => activeLine.range(from)));

/** In place of CodeMirror's own active-line highlight, which is switched off below. */
const cursorLine = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = marked(view);
    }
    update(update: ViewUpdate) {
      if (update.docChanged || update.selectionSet) this.decorations = marked(update.view);
    }
  },
  { decorations: (plugin) => plugin.decorations },
);

const extensions = [
  json(),
  EditorView.theme(theme),
  syntaxHighlighting(HighlightStyle.define(highlight)),
  EditorView.lineWrapping,
  cursorLine,
];

/** CodeMirror with JSON mode. Loaded on demand; see policy-editor.tsx. */
export default function PolicyCodeEditor({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <>
      <CodeMirror
      value={value}
      height="460px"
      theme="none"
      extensions={extensions}
      onChange={onChange}
      aria-label="Policy document"
      // The active line is drawn by cursorLine above, so that it never covers a selection.
      basicSetup={{ foldGutter: true, highlightActiveLine: false, autocompletion: false }}
      />
      {/* Tab indents here, so it cannot also be the way out. */}
      <p className="t-editorhint">Tab indents. To leave the editor with the keyboard, press Esc and then Tab.</p>
    </>
  );
}
