"use client";

import { json } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import CodeMirror from "@uiw/react-codemirror";

// Colors come from the theme tokens, so the editor follows light and dark.
const theme = EditorView.theme({
  "&": { backgroundColor: "var(--surface-card)", color: "var(--fg)", fontSize: "13px" },
  ".cm-content": { fontFamily: "var(--font-mono)", caretColor: "var(--fg)" },
  ".cm-gutters": {
    backgroundColor: "var(--surface-card)",
    color: "var(--fg-faint)",
    border: "none",
    borderRight: "1px solid var(--border)",
  },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "var(--surface-elevated)" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "color-mix(in oklch, var(--brand) 35%, transparent)",
  },
  "&.cm-focused": { outline: "2px solid var(--ring)", outlineOffset: "-2px" },
  ".cm-cursor": { borderLeftColor: "var(--fg)" },
});

const highlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: tags.propertyName, color: "var(--status-low)" },
    { tag: tags.string, color: "var(--status-pass)" },
    { tag: [tags.number, tags.bool, tags.null], color: "var(--status-high)" },
    { tag: tags.punctuation, color: "var(--fg-subtle)" },
  ]),
);

const extensions = [json(), theme, highlight, EditorView.lineWrapping];

/** CodeMirror with JSON mode. Loaded on demand; see policy-editor.tsx. */
export default function PolicyCodeEditor({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <CodeMirror
      value={value}
      height="460px"
      theme="none"
      extensions={extensions}
      onChange={onChange}
      aria-label="Policy document"
      basicSetup={{ foldGutter: true, highlightActiveLine: true, autocompletion: false }}
    />
  );
}
