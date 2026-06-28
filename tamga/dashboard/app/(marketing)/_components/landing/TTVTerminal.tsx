"use client";

import { useState, useCallback } from "react";
import { Copy, Check, Terminal } from "lucide-react";

const COMMANDS = [
  { prompt: "$", text: "git clone https://github.com/tamga/tamga.git", delay: 0 },
  { prompt: "$", text: "cd tamga && docker compose up -d", delay: 600 },
  { prompt: ">", text: "[SUCCESS] Tamga Proxy running on :8443", delay: 1200, green: true },
  { prompt: ">", text: "[INFO] Admin API listening on :9090", delay: 1600 },
  { prompt: ">", text: "[READY] Accepting LLM traffic — 0ms cold start", delay: 2000, green: true },
];

export function TTVTerminal() {
  const [copied, setCopied] = useState(false);

  const copyAll = useCallback(() => {
    const text = COMMANDS.map((c) => `${c.prompt} ${c.text}`).join("\n");
    navigator.clipboard.writeText(text).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, []);

  return (
    <div className="relative overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-[#0a0a0a]">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2.5 bg-zinc-100 dark:bg-zinc-900/50">
        <div className="flex items-center gap-2">
          <Terminal className="h-3.5 w-3.5 text-zinc-500" />
          <span className="text-[11px] text-zinc-600 dark:text-zinc-400">Deploy in 5 minutes</span>
        </div>
        <button
          onClick={copyAll}
          className="flex items-center gap-1.5 rounded-sm px-2 py-1 text-[10px] text-zinc-500 hover:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800 transition-colors"
        >
          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
          {copied ? "Copied" : "Copy all"}
        </button>
      </div>

      {/* Commands */}
      <div className="p-4 space-y-2">
        {COMMANDS.map((cmd, i) => (
          <div
            key={i}
            className="flex items-start gap-3 text-[13px] leading-relaxed"
            style={{ animationDelay: `${cmd.delay}ms` }}
          >
            <span className="shrink-0 select-none text-zinc-500 dark:text-zinc-600 w-4 text-right">
              {cmd.prompt}
            </span>
            <span
              className={
                cmd.green
                  ? "text-emerald-400 dark:text-emerald-300"
                  : "text-zinc-800 dark:text-zinc-200"
              }
            >
              {cmd.text}
            </span>
          </div>
        ))}
      </div>

      {/* Subtle glow-bottom */}
      <div className="absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-emerald-500/30 to-transparent" />
    </div>
  );
}
