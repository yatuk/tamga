"use client";

import { useCallback, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Copy, Search } from "lucide-react";
import { toLowerEn } from "@/lib/utils/tr-string";

// ── Types ──────────────────────────────────────────────────────────────────────

type JsonNode =
  | { type: "primitive"; value: string; kind: "string" | "number" | "boolean" | "null" }
  | { type: "object"; entries: { key: string; node: JsonNode }[] }
  | { type: "array"; items: JsonNode[] };

interface Props {
  data: unknown;
  className?: string;
  /** Maximum depth to auto-expand. Default 3. */
  autoExpandDepth?: number;
  /** Truncate strings longer than this. Default 200. */
  maxStringLen?: number;
}

// ── Parser ─────────────────────────────────────────────────────────────────────

function parseValue(val: unknown): JsonNode {
  if (val === null || val === undefined) return { type: "primitive", value: "null", kind: "null" };
  if (typeof val === "boolean") return { type: "primitive", value: String(val), kind: "boolean" };
  if (typeof val === "number") return { type: "primitive", value: String(val), kind: "number" };
  if (typeof val === "string") return { type: "primitive", value: val, kind: "string" };
  if (Array.isArray(val)) return { type: "array", items: val.map(parseValue) };
  if (typeof val === "object") {
    const entries = Object.entries(val as Record<string, unknown>).map(([key, v]) => ({
      key,
      node: parseValue(v),
    }));
    return { type: "object", entries };
  }
  return { type: "primitive", value: String(val), kind: "string" };
}

// ── Color tokens (OKLCH, dark-first) ───────────────────────────────────────────

const COLOR: Record<string, string> = {
  key: "text-[oklch(0.72_0.12_230)]",        // blue
  string: "text-[oklch(0.72_0.13_150)]",     // green
  number: "text-[oklch(0.74_0.13_55)]",      // amber
  boolean: "text-[oklch(0.64_0.16_22)]",     // red
  null: "text-zinc-500",                      // muted grey
  bracket: "text-zinc-500",                   // faint grey
  linenum: "text-zinc-600 dark:text-zinc-500", // line numbers
};

// ── Line renderer ──────────────────────────────────────────────────────────────

function JsonLine({
  text,
  depth,
  kind,
  searchTerm,
}: {
  text: string;
  depth: number;
  kind?: string;
  searchTerm: string;
}) {
  const highlight = searchTerm && toLowerEn(text).includes(toLowerEn(searchTerm));

  return (
    <div
      className={`pl-[calc(1.5rem*${depth}+2.5rem)] pr-2 py-px text-[11px] leading-relaxed whitespace-pre-wrap break-all font-mono ${COLOR[kind || "null"]} ${highlight ? "bg-amber-500/20" : ""}`}
    >
      {text}
    </div>
  );
}

// ── Node renderer (recursive) ──────────────────────────────────────────────────

function JsonNodeView({
  node,
  depth,
  path,
  autoExpandDepth,
  maxStringLen,
  searchTerm,
}: {
  node: JsonNode;
  depth: number;
  path: string;
  autoExpandDepth: number;
  maxStringLen: number;
  searchTerm: string;
}) {
  const [collapsed, setCollapsed] = useState(depth >= autoExpandDepth);
  const [truncated, setTruncated] = useState(true);

  const toggleCollapse = useCallback(() => setCollapsed((v) => !v), []);

  const copyPath = useCallback(() => {
    navigator.clipboard.writeText(path).catch(() => {});
  }, [path]);

  if (node.type === "primitive") {
    const display =
      node.kind === "string" && node.value.length > maxStringLen && truncated
        ? JSON.stringify(node.value.slice(0, maxStringLen)) + "…"
        : node.kind === "string"
          ? JSON.stringify(node.value)
          : node.value;

    return (
      <div
        className="group flex items-center hover:bg-zinc-100 dark:hover:bg-zinc-900/50"
        onDoubleClick={copyPath}
        title={`${path}\nDouble-click to copy path`}
      >
        <JsonLine text={display} depth={depth} kind={node.kind} searchTerm={searchTerm} />
        {node.kind === "string" && node.value.length > maxStringLen && (
          <button
            className="shrink-0 text-[10px] text-zinc-500 hover:text-zinc-300 ml-1"
            onClick={() => setTruncated((v) => !v)}
          >
            {truncated ? "more…" : "less"}
          </button>
        )}
        <button
          className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity ml-1"
          onClick={(e) => {
            e.stopPropagation();
            navigator.clipboard
              .writeText(node.kind === "string" ? node.value : node.value)
              .catch(() => {});
          }}
          title="Copy value"
        >
          <Copy className="h-3 w-3 text-zinc-500 hover:text-zinc-300" />
        </button>
      </div>
    );
  }

  const isArray = node.type === "array";
  const openBracket = isArray ? "[" : "{";
  const closeBracket = isArray ? "]" : "}";
  const entries = isArray
    ? node.items.map((item, i) => ({ key: String(i), node: item, comma: i < node.items.length - 1 }))
    : node.entries.map((e, i) => ({ ...e, comma: i < node.entries.length - 1 }));

  if (collapsed) {
    const count = isArray ? node.items.length : node.entries.length;
    return (
      <div
        className="group flex items-center hover:bg-zinc-100 dark:hover:bg-zinc-900/50 cursor-pointer"
        onClick={toggleCollapse}
        onDoubleClick={copyPath}
        title={`${path}\nClick to expand · Double-click to copy path`}
      >
        <span className="shrink-0 inline-block w-[2.5rem] text-right pr-1">
          <ChevronRight className="inline h-3 w-3 text-zinc-500" />
        </span>
        <span className="font-mono text-[11px]">
          <span className={COLOR.bracket}>{openBracket}</span>
          <span className="text-zinc-500 ml-1">{count} item{count !== 1 ? "s" : ""}</span>
          <span className={COLOR.bracket}>{closeBracket}</span>
        </span>
      </div>
    );
  }

  return (
    <div>
      {/* Open bracket */}
      <div
        className="group flex items-center hover:bg-zinc-100 dark:hover:bg-zinc-900/50 cursor-pointer"
        onClick={toggleCollapse}
        onDoubleClick={copyPath}
      >
        <span className="shrink-0 inline-block w-[2.5rem] text-right pr-1">
          <ChevronDown className="inline h-3 w-3 text-zinc-500" />
        </span>
        <span className={`font-mono text-[11px] ${COLOR.bracket}`}>{openBracket}</span>
      </div>

      {/* Entries */}
      {entries.map((entry, i) => (
        <div key={i}>
          <div className="flex items-center hover:bg-zinc-100 dark:hover:bg-zinc-900/50">
            <span className="shrink-0 w-[2.5rem] text-right pr-2 text-[10px] text-zinc-600 dark:text-zinc-500 tabular-nums font-mono">
              {/* line numbers every 5th */}
              {(i + 1) % 5 === 0 ? i + 1 : ""}
            </span>
            {!isArray && (
              <span className={`font-mono text-[11px] ${COLOR.key}`}>
                {JSON.stringify(entry.key)}
                <span className={COLOR.bracket}>: </span>
              </span>
            )}
            <JsonNodeView
              node={entry.node}
              depth={depth + 1}
              path={isArray ? `${path}[${entry.key}]` : `${path}.${entry.key}`}
              autoExpandDepth={autoExpandDepth}
              maxStringLen={maxStringLen}
              searchTerm={searchTerm}
            />
            {entry.comma && <span className={`font-mono text-[11px] ${COLOR.bracket}`}>,</span>}
          </div>
        </div>
      ))}

      {/* Close bracket */}
      <div className="flex items-center">
        <span className="shrink-0 w-[2.5rem]" />
        <span className={`font-mono text-[11px] ${COLOR.bracket}`}>{closeBracket}</span>
      </div>
    </div>
  );
}

// ── Main export ────────────────────────────────────────────────────────────────

export function JsonInspector({
  data,
  className = "",
  autoExpandDepth = 3,
  maxStringLen = 200,
}: Props) {
  const [searchTerm, setSearchTerm] = useState("");
  const [expandAll, setExpandAll] = useState(false);

  const node = useMemo(() => parseValue(data), [data]);

  const rawJson = useMemo(() => {
    try {
      return JSON.stringify(data, null, 2);
    } catch {
      return String(data);
    }
  }, [data]);

  const copyAll = useCallback(() => {
    navigator.clipboard.writeText(rawJson).catch(() => {});
  }, [rawJson]);

  return (
    <div className={`flex flex-col ${className}`}>
      {/* Toolbar */}
      <div className="flex items-center gap-2 border-b border-zinc-200 dark:border-zinc-800 px-2 py-1">
        <div className="relative flex-1">
          <Search className="absolute left-1.5 top-1/2 h-3 w-3 -translate-y-1/2 text-zinc-500" />
          <input
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            placeholder="Search payload…"
            className="h-7 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 pl-6 pr-2 text-[11px] font-mono text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-500"
          />
        </div>
        <button
          className="text-[10px] text-zinc-500 hover:text-zinc-300 transition-colors"
          onClick={() => setExpandAll((v) => !v)}
        >
          {expandAll ? "Collapse all" : "Expand all"}
        </button>
        <button
          className="text-[10px] text-zinc-500 hover:text-zinc-300 transition-colors flex items-center gap-1"
          onClick={copyAll}
        >
          <Copy className="h-3 w-3" />
          Copy
        </button>
      </div>

      {/* Tree */}
      <div className="flex-1 overflow-auto">
        <div className="py-1">
          <JsonNodeView
            node={node}
            depth={0}
            path="$"
            autoExpandDepth={expandAll ? 99 : autoExpandDepth}
            maxStringLen={maxStringLen}
            searchTerm={searchTerm}
          />
        </div>
      </div>
    </div>
  );
}
