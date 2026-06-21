"use client";

import type { Dispatch, SetStateAction } from "react";
import Link from "next/link";
import { BookOpen } from "lucide-react";
import type { UseMutationResult } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toast } from "@/lib/toast";
import { toUpperEn } from "@/lib/utils/tr-string";
import { INTEGRATION_PRESETS } from "./integrationPresets";
import { integrationKindBadge } from "./integrationWebhookHelpers";
import type { IntegrationDraft } from "./integrationDraft";

type Props = {
  draft: IntegrationDraft;
  setDraft: Dispatch<SetStateAction<IntegrationDraft | null>>;
  createMut: UseMutationResult<unknown, Error, IntegrationDraft, unknown>;
};

export function IntegrationConnectModal({ draft, setDraft, createMut }: Props) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div className="w-full max-w-md rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4 shadow-xl">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">CONNECT</div>
            <div className="text-sm text-zinc-900 dark:text-zinc-100">{draft.kind}</div>
          </div>
          <div className="flex items-center gap-2">
            <Link
              href={`/dashboard/integrations/${draft.kind}`}
              className="inline-flex items-center gap-1 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-[10px] uppercase tracking-wide text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            >
              <BookOpen className="h-3 w-3" /> Guide
            </Link>
            <Badge className={`rounded-sm border text-[10px] uppercase ${integrationKindBadge(draft.kind)}`}>
              {draft.kind}
            </Badge>
          </div>
        </div>
        <div className="space-y-3">
          <div>
            <label className="text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">Label</label>
            <input
              className="mt-1 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-900 dark:text-zinc-100 focus:border-red-500/40 focus:outline-none"
              value={draft.label}
              onChange={(e) => setDraft({ ...draft, label: e.target.value })}
            />
          </div>
          <div>
            <label className="text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">URL</label>
            <input
              className="mt-1 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-900 dark:text-zinc-100 focus:border-red-500/40 focus:outline-none"
              value={draft.url}
              onChange={(e) => setDraft({ ...draft, url: e.target.value })}
              placeholder={INTEGRATION_PRESETS.find((p) => p.kind === draft.kind)?.urlHint}
            />
          </div>
          {draft.kind === "jira" ? (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">Project key</label>
                <input
                  className="mt-1 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-900 dark:text-zinc-100 focus:border-red-500/40 focus:outline-none"
                  value={draft.projectKey}
                  onChange={(e) => setDraft({ ...draft, projectKey: toUpperEn(e.target.value) })}
                  placeholder="SEC"
                />
              </div>
              <div>
                <label className="text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">Issue type</label>
                <input
                  className="mt-1 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-900 dark:text-zinc-100 focus:border-red-500/40 focus:outline-none"
                  value={draft.issueType}
                  onChange={(e) => setDraft({ ...draft, issueType: e.target.value })}
                  placeholder="Task"
                />
              </div>
            </div>
          ) : null}
          {draft.kind === "pagerduty" || draft.kind === "opsgenie" ? (
            <div>
              <label className="text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">
                {draft.kind === "pagerduty" ? "Routing key (integration key)" : "API key (GenieKey)"}
              </label>
              <input
                type="password"
                className="mt-1 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-900 dark:text-zinc-100 focus:border-red-500/40 focus:outline-none"
                value={draft.authToken}
                onChange={(e) => setDraft({ ...draft, authToken: e.target.value })}
                placeholder={
                  draft.kind === "pagerduty" ? "R0A1B2C3D4E5F6789ABCDEF012345678" : "xxxxxxxxxxxxxxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                }
                autoComplete="off"
              />
              <p className="mt-1 text-[10px] text-zinc-600 dark:text-zinc-400">
                {draft.kind === "pagerduty"
                  ? "Injected into the JSON body as routing_key (Events API v2 requirement)."
                  : "Injected as Authorization: GenieKey <token> at request time."}
              </p>
            </div>
          ) : null}
          <div>
            <label className="text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">
              Extra Headers (one per line, &ldquo;Key: Value&rdquo;)
            </label>
            <textarea
              className="mt-1 block min-h-[70px] w-full resize-y rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-1.5 text-[11px] text-zinc-900 dark:text-zinc-100 focus:outline-none"
              value={draft.headers}
              onChange={(e) => setDraft({ ...draft, headers: e.target.value })}
            />
          </div>
          <label className="flex cursor-pointer items-center gap-2 text-[11px] text-zinc-700 dark:text-zinc-300">
            <input
              type="checkbox"
              checked={draft.enabled}
              onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
              className="h-3.5 w-3.5 cursor-pointer accent-red-600"
            />
            enabled
          </label>
        </div>
        <div className="mt-4 flex items-center justify-end gap-2">
          <Button
            className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={() => setDraft(null)}
          >
            Cancel
          </Button>
          <Button
            className="cursor-pointer rounded-sm bg-red-600 text-white hover:bg-red-700"
            onClick={() => {
              if (!draft.url.trim()) {
                toast.error("URL required");
                return;
              }
              if (draft.kind === "jira" && !draft.projectKey.trim()) {
                toast.error("Project key required", "Jira Cloud v3 rejects creates without it");
                return;
              }
              if (draft.kind === "pagerduty" && !draft.authToken.trim()) {
                toast.error("Routing key required", "PagerDuty Events API v2 rejects requests without routing_key");
                return;
              }
              if (draft.kind === "opsgenie" && !draft.authToken.trim()) {
                toast.error("API key required", "Opsgenie returns 401 without GenieKey");
                return;
              }
              createMut.mutate(draft);
            }}
            disabled={createMut.isPending}
          >
            Connect
          </Button>
        </div>
      </div>
    </div>
  );
}
