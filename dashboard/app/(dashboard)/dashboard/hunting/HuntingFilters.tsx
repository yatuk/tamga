"use client";

import { useState } from "react";
import { BookmarkPlus, Search } from "lucide-react";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
import type { HuntFilterKey, HuntFilters } from "./useHuntingPage";

type Props = {
  filters: HuntFilters;
  setFilter: (key: HuntFilterKey, value: string) => void;
  shadow: boolean;
  setShadow: (on: boolean) => void;
  activeFilterCount: number;
  clearFilters: () => void;
  saveHunt: (name: string) => Promise<void>;
};

const ACTIONS = ["BLOCK", "REDACT", "WARN", "PASS"];
const SEVERITIES = ["critical", "high", "medium", "low"];

export function HuntingFilters({ filters, setFilter, shadow, setShadow, activeFilterCount, clearFilters, saveHunt }: Props) {
  return (
    <Panel
      title="Query"
      aside={
        <>
          {activeFilterCount > 0 ? (
            <Button variant="ghost" size="xs" onClick={clearFilters}>
              Clear {activeFilterCount} {activeFilterCount === 1 ? "filter" : "filters"}
            </Button>
          ) : null}
          <SaveHuntPopover onSave={saveHunt} />
        </>
      }
    >
      <div className="space-y-4 p-4">
        <FormField label="Search" htmlFor="hunt-q" hint="Matches the request ID and the text of findings.">
          <div className="relative">
            <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              id="hunt-q"
              name="q"
              type="search"
              autoComplete="off"
              spellCheck={false}
              className="pl-9 font-mono"
              placeholder="Request ID or text…"
              value={filters.q}
              onChange={(e) => setFilter("q", e.target.value)}
            />
          </div>
        </FormField>

        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <FormField label="Action" htmlFor="hunt-action">
            <NativeSelect
              id="hunt-action"
              name="action"
              value={filters.action}
              onChange={(e) => setFilter("action", e.target.value)}
            >
              <NativeSelectOption value="">Any action</NativeSelectOption>
              {ACTIONS.map((a) => (
                <NativeSelectOption key={a} value={a}>
                  {a}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FormField>
          <FormField label="Severity" htmlFor="hunt-severity">
            <NativeSelect
              id="hunt-severity"
              name="severity"
              value={filters.severity}
              onChange={(e) => setFilter("severity", e.target.value)}
            >
              <NativeSelectOption value="">Any severity</NativeSelectOption>
              {SEVERITIES.map((s) => (
                <NativeSelectOption key={s} value={s}>
                  {s}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </FormField>
          <FormField label="Finding type" htmlFor="hunt-finding-type">
            <Input
              id="hunt-finding-type"
              name="finding_type"
              autoComplete="off"
              spellCheck={false}
              placeholder="pii, secret, injection…"
              value={filters.finding_type}
              onChange={(e) => setFilter("finding_type", e.target.value)}
            />
          </FormField>
          <FormField label="Category contains" htmlFor="hunt-category">
            <Input
              id="hunt-category"
              name="category"
              autoComplete="off"
              spellCheck={false}
              placeholder="credit_card, jailbreak…"
              value={filters.category}
              onChange={(e) => setFilter("category", e.target.value)}
            />
          </FormField>
          <FormField label="Technique or OWASP code" htmlFor="hunt-technique">
            <Input
              id="hunt-technique"
              name="technique"
              autoComplete="off"
              spellCheck={false}
              placeholder="LLM01…"
              value={filters.technique}
              onChange={(e) => setFilter("technique", e.target.value)}
            />
          </FormField>
          <FormField label="Provider" htmlFor="hunt-provider">
            <Input
              id="hunt-provider"
              name="provider"
              autoComplete="off"
              spellCheck={false}
              placeholder="openai, anthropic…"
              value={filters.provider}
              disabled={shadow}
              onChange={(e) => setFilter("provider", e.target.value)}
            />
          </FormField>
        </div>

        <div className="flex items-center gap-3">
          <Switch id="hunt-shadow" checked={shadow} onCheckedChange={setShadow} />
          <Label htmlFor="hunt-shadow">Only providers outside the routing table (shadow AI)</Label>
        </div>
      </div>
    </Panel>
  );
}

function SaveHuntPopover({ onSave }: { onSave: (name: string) => Promise<void> }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="xs">
          <BookmarkPlus />
          Save Hunt
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!name.trim()) return;
            void onSave(name).then(() => {
              setName("");
              setOpen(false);
            });
          }}
        >
          <FormField label="Name" htmlFor="hunt-name" hint="The current filters and range are saved with it.">
            <Input
              id="hunt-name"
              name="hunt-name"
              autoComplete="off"
              placeholder="Leaked cloud keys…"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </FormField>
          <Button type="submit" size="sm" disabled={!name.trim()}>
            Save Hunt
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  );
}
