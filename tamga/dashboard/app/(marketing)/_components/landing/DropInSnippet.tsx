"use client";

import { useState, useCallback } from "react";
import { Copy, Check } from "lucide-react";

type Tab = "python" | "go";

const SNIPPETS: Record<Tab, { label: string; lines: { kind: "+" | "-" | "="; text: string }[] }> = {
  python: {
    label: "Python SDK",
    lines: [
      { kind: "=", text: 'import openai' },
      { kind: "=", text: "" },
      { kind: "=", text: 'client = openai.OpenAI(' },
      { kind: "-", text: '    api_key = "sk-...",' },
      { kind: "+", text: '    base_url = "https://proxy.tamga.dev/v1",' },
      { kind: "+", text: '    api_key = "sk-...",' },
      { kind: "=", text: ")" },
      { kind: "=", text: "" },
      { kind: "=", text: "# Every request now passes through Tamga" },
      { kind: "=", text: "# PII redaction · prompt injection defense · audit" },
      { kind: "=", text: 'response = client.chat.completions.create(' },
      { kind: "=", text: '    model = "gpt-4",' },
      { kind: "=", text: '    messages = [{"role": "user", "content": "Hello"}]' },
      { kind: "=", text: ")" },
    ],
  },
  go: {
    label: "Go SDK",
    lines: [
      { kind: "=", text: 'import "github.com/openai/openai-go"' },
      { kind: "=", text: "" },
      { kind: "=", text: "client := openai.NewClient(" },
      { kind: "-", text: '    openai.WithAPIKey("sk-..."),' },
      { kind: "+", text: '    openai.WithBaseURL("https://proxy.tamga.dev/v1"),' },
      { kind: "+", text: '    openai.WithAPIKey("sk-..."),' },
      { kind: "=", text: ")" },
      { kind: "=", text: "" },
      { kind: "=", text: "// Every request now passes through Tamga" },
      { kind: "=", text: "// PII redaction · prompt injection defense · audit" },
      { kind: "=", text: "resp, err := client.Chat.Completions.New(" },
      { kind: "=", text: '    context.TODO(), openai.ChatCompletionNewParams{' },
      { kind: "=", text: '        Model: openai.F("gpt-4"),' },
      { kind: "=", text: '        Messages: openai.F([]openai.ChatCompletionMessageParamUnion{' },
      { kind: "=", text: '            openai.UserMessage("Hello"),' },
      { kind: "=", text: "        })," },
      { kind: "=", text: "    })" },
    ],
  },
};

const LINE_COLORS: Record<string, string> = {
  "=": "text-zinc-400 dark:text-zinc-500",
  "+": "text-emerald-400 dark:text-emerald-300",
  "-": "text-red-400 dark:text-red-300/80 line-through decoration-red-500/40",
};

const LINE_BG: Record<string, string> = {
  "=": "",
  "+": "bg-emerald-500/5 border-l-2 border-emerald-500/40",
  "-": "bg-red-500/5 border-l-2 border-red-500/40",
};

export function DropInSnippet() {
  const [tab, setTab] = useState<Tab>("python");
  const [copied, setCopied] = useState(false);

  const snippet = SNIPPETS[tab];

  const copyAll = useCallback(() => {
    const text = snippet.lines
      .filter((l) => l.kind !== "-")
      .map((l) => l.text)
      .join("\n");
    navigator.clipboard.writeText(text).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [snippet]);

  return (
    <div className="overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-[#0a0a0a]">
      {/* Header with tabs */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/50 px-4 py-2">
        <div className="flex items-center gap-1">
          {(["python", "go"] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3 py-1 text-[11px] rounded-sm transition-colors ${
                tab === t
                  ? "bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 border border-zinc-200 dark:border-zinc-700"
                  : "text-zinc-500 hover:text-zinc-300"
              }`}
            >
              {SNIPPETS[t].label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-3">
          <span className="text-[10px] text-zinc-500">
            <span className="text-emerald-400">+</span> add ·{" "}
            <span className="text-red-400 line-through">-</span> remove
          </span>
          <button
            onClick={copyAll}
            className="flex items-center gap-1.5 rounded-sm px-2 py-1 text-[10px] text-zinc-500 hover:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800 transition-colors"
          >
            {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
            {copied ? "Copied" : "Copy"}
          </button>
        </div>
      </div>

      {/* Code */}
      <div className="p-4">
        <div className="text-[13px] leading-relaxed font-mono">
          {snippet.lines.map((line, i) => (
            <div
              key={i}
              className={`flex ${LINE_BG[line.kind]} ${line.kind === "+" ? "px-4 -mx-4" : ""} ${line.kind === "-" ? "px-4 -mx-4" : ""}`}
            >
              <span className="shrink-0 w-6 text-right mr-4 select-none text-zinc-700 dark:text-zinc-600 text-[11px]">
                {i + 1}
              </span>
              <span className={LINE_COLORS[line.kind]}>
                {line.text || " "}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
