"use client";

import { json } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import CodeMirror from "@uiw/react-codemirror";

// Ink on paper, as the rest of the console. Every colour is a token read by the
// stylesheet: a CodeMirror theme is turned into CSS rules, so var() works in it.
// Nothing is red or ochre: those mean a request was stopped or changed. The
// parts of the document are told apart by the ink steps, weight and slope:
// keys in ink and bold, numbers and true/false/null in ink, strings in the
// second ink, comments in the second ink and sloped.
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
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
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

const extensions = [json(), EditorView.theme(theme), syntaxHighlighting(HighlightStyle.define(highlight)), EditorView.lineWrapping];

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
      basicSetup={{ foldGutter: true, highlightActiveLine: true, autocompletion: false }}
      />
      {/* Tab indents here, so it cannot also be the way out. */}
      <p className="t-editorhint">Tab indents. To leave the editor with the keyboard, press Esc and then Tab.</p>
    </>
  );
}
