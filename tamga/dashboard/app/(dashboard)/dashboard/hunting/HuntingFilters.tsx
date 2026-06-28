"use client";

import type { ChangeEvent } from "react";
import { BookmarkPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";

import type { TimeRange } from "@/lib/types";

type Props = {
  action: string;
  setAction: (v: string) => void;
  provider: string;
  setProvider: (v: string) => void;
  shadow: boolean;
  setShadow: (v: boolean) => void;
  findingType: string;
  setFindingType: (v: string) => void;
  severity: string;
  setSeverity: (v: string) => void;
  category: string;
  setCategory: (v: string) => void;
  technique: string;
  setTechnique: (v: string) => void;
  q: string;
  setQ: (v: string) => void;
  range: TimeRange;
  setRange: (v: TimeRange) => void;
  resetPage: () => void;
  saveHunt: () => void;
  total: number;
  page: number;
  isLoading: boolean;
  isFetching: boolean;
};

export function HuntingFilters({
  action,
  setAction,
  provider,
  setProvider,
  shadow,
  setShadow,
  findingType,
  setFindingType,
  severity,
  setSeverity,
  category,
  setCategory,
  technique,
  setTechnique,
  q,
  setQ,
  range,
  setRange,
  resetPage,
  saveHunt,
  total,
  page,
  isLoading,
  isFetching,
}: Props) {
  return (
    <TerminalFrame title="Arama Sorgusu">
      <div className="space-y-3 p-3">
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Action</span>
            <Input
              className="h-8 focus:border-red-500"
              placeholder="BLOCK, REDACT…"
              value={action}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                setAction(e.target.value);
                resetPage();
              }}
            />
          </label>
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Provider</span>
            <input
              className="h-8 w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 text-xs text-zinc-900 dark:text-zinc-100 outline-none focus:border-red-500 disabled:opacity-50"
              placeholder="openai, shadow…"
              value={provider}
              disabled={shadow}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                setProvider(e.target.value);
                resetPage();
              }}
            />
          </label>
          <label className="flex items-end gap-2 pb-1">
            <input
              type="checkbox"
              checked={shadow}
              onChange={(e) => {
                setShadow(e.target.checked);
                if (e.target.checked) setProvider("");
                resetPage();
              }}
              className="accent-red-500"
            />
            <span className="text-[11px] text-zinc-600 dark:text-zinc-400">Shadow providers only</span>
          </label>
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Range</span>
            <select
              className="h-8 w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 text-xs text-zinc-800 dark:text-zinc-200"
              value={range}
              onChange={(e) => {
                setRange(e.target.value as TimeRange);
                resetPage();
              }}
            >
              <option value="24h">24h</option>
              <option value="7d">7d</option>
              <option value="30d">30d</option>
            </select>
          </label>
        </div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Finding type</span>
            <Input
              className="h-8 focus:border-red-500"
              placeholder="pii, injection…"
              value={findingType}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                setFindingType(e.target.value);
                resetPage();
              }}
            />
          </label>
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Severity</span>
            <Input
              className="h-8 focus:border-red-500"
              placeholder="high, critical…"
              value={severity}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                setSeverity(e.target.value);
                resetPage();
              }}
            />
          </label>
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Category</span>
            <Input
              className="h-8 focus:border-red-500"
              placeholder="substring"
              value={category}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                setCategory(e.target.value);
                resetPage();
              }}
            />
          </label>
          <label className="space-y-1">
            <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Technique / OWASP</span>
            <Input
              className="h-8 focus:border-red-500"
              placeholder="LLM01, metadata…"
              value={technique}
              onChange={(e: ChangeEvent<HTMLInputElement>) => {
                setTechnique(e.target.value);
                resetPage();
              }}
            />
          </label>
        </div>
        <label className="block space-y-1">
          <span className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">Q (request_id / payload)</span>
          <Input
              className="h-8 focus:border-red-500"
            placeholder="req_… veya findings içinde ara"
            value={q}
            onChange={(e: ChangeEvent<HTMLInputElement>) => {
              setQ(e.target.value);
              resetPage();
            }}
          />
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" size="sm" onClick={saveHunt}>
            <BookmarkPlus className="h-3.5 w-3.5" />
            Save hunt
          </Button>
          <span className="text-[10px] text-zinc-600 dark:text-zinc-400">
            {isLoading || isFetching ? "Loading…" : `${total} eşleşme (sayfa ${page})`}
          </span>
        </div>
      </div>
    </TerminalFrame>
  );
}
