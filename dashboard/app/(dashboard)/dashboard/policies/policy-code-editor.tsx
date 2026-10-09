"use client";

import { json } from "@codemirror/lang-json";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import CodeMirror from "@uiw/react-codemirror";

// Ink on paper, as the rest of the console: keys in black, strings in grey, values in ochre.
const theme = EditorView.theme({
  "&": { backgroundColor: "var(--cds-layer)", color: "var(--cds-text-primary)", fontSize: "13px" },
  ".cm-content": { fontFamily: "var(--tamga-font-data)", caretColor: "var(--cds-text-primary)" },
  ".cm-gutters": {
    backgroundColor: "var(--cds-layer)",
    color: "var(--cds-text-helper)",
    border: "none",
    borderRight: "1px solid var(--tamga-rule)",
  },
  ".cm-activeLine, .cm-activeLineGutter": { backgroundColor: "#e8e8e8" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "#c6c6c6",
  },
  "&.cm-focused": { outline: "2px solid #161616", outlineOffset: "-2px" },
  ".cm-cursor": { borderLeftColor: "var(--cds-text-primary)" },
});

const highlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: tags.propertyName, color: "#161616" },
    { tag: tags.string, color: "#525252" },
    { tag: [tags.number, tags.bool, tags.null], color: "#8a5a00" },
    { tag: tags.punctuation, color: "var(--cds-text-secondary)" },
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
