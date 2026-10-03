"use client";

import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Props = {
  draft: string;
  setDraft: (v: string) => void;
  saved: string;
  saveAdminKey: () => void;
};

export function SettingsAccessSection({ draft, setDraft, saved, saveAdminKey }: Props) {
  return (
    <>
      <Panel
        title="Admin key"
        aside={<StatusBadge tone={saved ? "pass" : "neutral"}>{saved ? "Stored in this browser" : "Not set"}</StatusBadge>}
      >
        <form
          className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start"
          onSubmit={(e) => {
            e.preventDefault();
            saveAdminKey();
          }}
        >
          <FormField
            label="Admin key"
            htmlFor="admin-key-input"
            className="flex-1"
            hint="Sent as X-Tamga-Admin-Key with every management API call. It is stored only in this browser."
          >
            <Input
              id="admin-key-input"
              name="admin-key"
              type="password"
              autoComplete="off"
              spellCheck={false}
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              className="font-mono"
            />
          </FormField>
          <Button type="submit" className="sm:mt-[1.375rem]" disabled={draft === saved}>
            Save Key
          </Button>
        </form>
      </Panel>

      <div className="flex flex-wrap items-center justify-between gap-3 border bg-card px-4 py-3">
        <p className="max-w-2xl text-sm text-muted-foreground">
          Client applications use their own scoped API keys, not the admin key. Webhooks and other alert destinations live under Integrations.
        </p>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline" size="sm">
          <Link href="/dashboard/keys">
            Manage API Keys
            <ArrowRight />
          </Link>
        </Button>
          <Button asChild variant="outline" size="sm">
            <Link href="/dashboard/integrations">
              Alert Destinations
              <ArrowRight />
            </Link>
          </Button>
        </div>
      </div>
    </>
  );
}
