"use client";

import { Bookmark, Download, Pencil, Search, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";
import type {
  ActionFilter,
  AssigneeFilter,
  ProviderFilter,
  SeverityFilter,
  TimeRange,
  TriageFilter,
  TypeFilter,
} from "@/lib/security/security-events-model";

type Option = { value: string; label: string };

/** One labelled dropdown filter. The label is announced; the value shows it in the trigger. */
function Filter({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Option[];
}) {
  return (
    <label className="flex items-center gap-2 text-xs text-muted-foreground">
      {label}
      <NativeSelect size="sm" value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => (
          <NativeSelectOption key={o.value} value={o.value}>
            {o.label}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </label>
  );
}

const ALL: Option = { value: "all", label: "All" };

export function IncidentsToolbar({ m }: { m: IncidentsConsoleModel }) {
  return (
    <div className="space-y-3 border bg-card p-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1">
          <Search
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden
          />
          <Input
            ref={m.searchInputRef}
            name="incident-search"
            autoComplete="off"
            value={m.searchText}
            onChange={(e) => m.setSearchText(e.target.value)}
            placeholder="Search request, provider or model…"
            aria-label="Search incidents"
            className="h-8 pl-8"
          />
        </div>
        <Input
          name="request-id"
          autoComplete="off"
          spellCheck={false}
          value={m.requestIdFilter}
          onChange={(e) => m.setRequestIdFilter(e.target.value.trim())}
          placeholder="Request ID…"
          aria-label="Filter by request ID"
          className="h-8 w-48 font-mono"
        />
        <Button variant="outline" size="sm" onClick={() => m.applyPreset("critical-now")}>
          Critical Now
        </Button>
        <Button variant="outline" size="sm" onClick={() => m.applyPreset("block-focused")}>
          Blocked Only
        </Button>
        <SavedViews m={m} />
        <Button variant="outline" size="sm" onClick={m.exportIncidentsCsv}>
          <Download />
          Export CSV
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Filter
          label="Range"
          value={m.timeRange}
          onChange={(v) => m.setTimeRange(v as TimeRange)}
          options={[
            { value: "1h", label: "1 hour" },
            { value: "24h", label: "24 hours" },
            { value: "7d", label: "7 days" },
            { value: "30d", label: "30 days" },
          ]}
        />
        <Filter
          label="Action"
          value={m.actionFilter}
          onChange={(v) => m.setActionFilter(v as ActionFilter)}
          options={[ALL, ...["BLOCK", "STRIP", "REDACT", "WARN", "LOG", "PASS"].map((a) => ({ value: a, label: a }))]}
        />
        <Filter
          label="Type"
          value={m.typeFilter}
          onChange={(v) => m.setTypeFilter(v as TypeFilter)}
          options={[
            ALL,
            { value: "pii", label: "PII" },
            { value: "secret", label: "Secret" },
            { value: "injection", label: "Injection" },
            { value: "custom", label: "Custom" },
          ]}
        />
        <Filter
          label="Severity"
          value={m.severityFilter}
          onChange={(v) => m.setSeverityFilter(v as SeverityFilter)}
          options={[
            ALL,
            { value: "critical", label: "Critical" },
            { value: "high", label: "High" },
            { value: "medium", label: "Medium" },
            { value: "low", label: "Low" },
          ]}
        />
        <Filter
          label="Triage"
          value={m.triageFilter}
          onChange={(v) => m.setTriageFilter(v as TriageFilter)}
          options={[
            ALL,
            { value: "Open", label: "Open" },
            { value: "In Progress", label: "In progress" },
            { value: "Closed", label: "Closed" },
            { value: "False Positive", label: "False positive" },
          ]}
        />
        <Filter
          label="Assignee"
          value={m.assigneeFilter}
          onChange={(v) => m.setAssigneeFilter(v as AssigneeFilter)}
          options={[ALL, { value: "me", label: "Me" }, { value: "unassigned", label: "Unassigned" }]}
        />
        <Filter
          label="Provider"
          value={m.providerFilter}
          onChange={(v) => m.setProviderFilter(v as ProviderFilter)}
          options={[
            ALL,
            { value: "openai", label: "OpenAI" },
            { value: "anthropic", label: "Anthropic" },
            { value: "google", label: "Google" },
            { value: "azure", label: "Azure" },
            { value: "unknown", label: "Unknown" },
            { value: "shadow", label: "Shadow (not sanctioned)" },
          ]}
        />
      </div>
    </div>
  );
}

function SavedViews({ m }: { m: IncidentsConsoleModel }) {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm">
          <Bookmark />
          Views
          {m.savedViews.length > 0 ? (
            <span className="font-mono text-xs text-muted-foreground">{m.savedViews.length}</span>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0">
        <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
          <span className="text-sm font-medium">Saved Views</span>
          <Button variant="outline" size="xs" onClick={() => m.saveCurrentView()}>
            Save Current Filters
          </Button>
        </div>
        {m.savedViews.length === 0 ? (
          <p className="px-3 py-4 text-xs text-muted-foreground">
            No saved views yet. Set the filters you use most and save them here.
          </p>
        ) : (
          <ul className="max-h-72 divide-y overflow-y-auto overscroll-contain">
            {m.savedViews.map((v) => (
              <li key={v.id} className="flex items-center gap-1 px-1.5 py-1">
                <Button
                  variant="ghost"
                  onClick={() => m.applySavedView(v)}
                  className="h-auto min-w-0 flex-1 flex-col items-start gap-0 px-1.5 py-1 text-left font-normal whitespace-normal"
                >
                  <span className="block w-full truncate text-sm">{v.name}</span>
                  <span className="block w-full truncate font-mono text-xs text-muted-foreground">
                    {[v.range, v.action, v.type, v.severity, v.triage].filter((x) => x !== "all").join(" / ")}
                  </span>
                </Button>
                <Button variant="ghost" size="icon-xs" aria-label={`Rename ${v.name}`} onClick={() => m.renameSavedView(v.id)}>
                  <Pencil />
                </Button>
                <Button variant="ghost" size="icon-xs" aria-label={`Delete ${v.name}`} onClick={() => m.deleteSavedView(v.id)}>
                  <Trash2 />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
