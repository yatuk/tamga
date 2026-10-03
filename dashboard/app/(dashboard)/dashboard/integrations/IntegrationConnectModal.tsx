"use client";

import { useState, type Dispatch, type SetStateAction } from "react";
import Link from "next/link";
import type { UseMutationResult } from "@tanstack/react-query";
import { FormField } from "@/components/app/form-field";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { toUpperEn } from "@/lib/utils/case";
import type { IntegrationDraft } from "./integrationDraft";
import { INTEGRATION_PRESETS } from "./integrationPresets";

type Props = {
  /** The destination being connected; the dialog is open while this is set. */
  draft: IntegrationDraft | null;
  setDraft: Dispatch<SetStateAction<IntegrationDraft | null>>;
  createMut: UseMutationResult<unknown, Error, IntegrationDraft, unknown>;
};

/** What is missing before the destination can be created, by field. */
function validate(draft: IntegrationDraft): Partial<Record<"url" | "projectKey" | "authToken", string>> {
  const errors: Partial<Record<"url" | "projectKey" | "authToken", string>> = {};
  if (!draft.url.trim()) errors.url = "Enter the URL to send to.";
  if (draft.kind === "jira" && !draft.projectKey.trim()) errors.projectKey = "Jira needs a project key to create issues.";
  if (draft.kind === "pagerduty" && !draft.authToken.trim()) errors.authToken = "PagerDuty rejects events without a routing key.";
  if (draft.kind === "opsgenie" && !draft.authToken.trim()) errors.authToken = "Opsgenie rejects alerts without an API key.";
  return errors;
}

export function IntegrationConnectModal({ draft, setDraft, createMut }: Props) {
  return (
    <Dialog open={!!draft} onOpenChange={(open) => (open ? undefined : setDraft(null))}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto overscroll-contain sm:max-w-lg">
        {draft ? <ConnectForm draft={draft} setDraft={setDraft} createMut={createMut} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function ConnectForm({ draft, setDraft, createMut }: Omit<Props, "draft"> & { draft: IntegrationDraft }) {
  const [submitted, setSubmitted] = useState(false);
  const preset = INTEGRATION_PRESETS.find((p) => p.kind === draft.kind);
  const errors = submitted ? validate(draft) : {};
  const tokenLabel = draft.kind === "pagerduty" ? "Routing key" : "API key";

  return (
    <form
      noValidate
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault();
        setSubmitted(true);
        if (Object.keys(validate(draft)).length === 0) createMut.mutate(draft);
      }}
    >
      <DialogHeader>
        <DialogTitle>Connect {preset?.name ?? draft.kind}</DialogTitle>
        <DialogDescription>
          {preset?.blurb}.{" "}
          <Link href={`/dashboard/integrations/${draft.kind}`} className="text-foreground underline underline-offset-4">
            Setup guide
          </Link>
        </DialogDescription>
      </DialogHeader>

      <FormField label="Label" htmlFor="integration-label" hint="How this destination is named in the console.">
        <Input
          id="integration-label"
          name="label"
          autoComplete="off"
          value={draft.label}
          onChange={(e) => setDraft({ ...draft, label: e.target.value })}
        />
      </FormField>

      <FormField label="URL" htmlFor="integration-url">
        <Input
          id="integration-url"
          name="url"
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          className="font-mono text-xs"
          value={draft.url}
          onChange={(e) => setDraft({ ...draft, url: e.target.value })}
          placeholder={preset?.urlHint ? `${preset.urlHint}…` : undefined}
          aria-invalid={!!errors.url}
          aria-describedby={errors.url ? "integration-url-error" : undefined}
        />
        {errors.url ? (
          <p id="integration-url-error" role="alert" className="text-xs text-status-critical">
            {errors.url}
          </p>
        ) : null}
      </FormField>

      {draft.kind === "jira" ? (
        <div className="grid grid-cols-2 gap-3">
          <FormField label="Project key" htmlFor="integration-project">
            <Input
              id="integration-project"
              name="project_key"
              autoComplete="off"
              spellCheck={false}
              className="font-mono"
              value={draft.projectKey}
              onChange={(e) => setDraft({ ...draft, projectKey: toUpperEn(e.target.value) })}
              placeholder="SEC…"
              aria-invalid={!!errors.projectKey}
              aria-describedby={errors.projectKey ? "integration-project-error" : undefined}
            />
            {errors.projectKey ? (
              <p id="integration-project-error" role="alert" className="text-xs text-status-critical">
                {errors.projectKey}
              </p>
            ) : null}
          </FormField>
          <FormField label="Issue type" htmlFor="integration-issue-type">
            <Input
              id="integration-issue-type"
              name="issue_type"
              autoComplete="off"
              value={draft.issueType}
              onChange={(e) => setDraft({ ...draft, issueType: e.target.value })}
              placeholder="Task…"
            />
          </FormField>
        </div>
      ) : null}

      {draft.kind === "pagerduty" || draft.kind === "opsgenie" ? (
        <FormField
          label={tokenLabel}
          htmlFor="integration-token"
          hint={
            draft.kind === "pagerduty"
              ? "Sent in the request body as routing_key."
              : "Sent as the Authorization: GenieKey header."
          }
        >
          <Input
            id="integration-token"
            name="auth_token"
            type="password"
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            value={draft.authToken}
            onChange={(e) => setDraft({ ...draft, authToken: e.target.value })}
            aria-invalid={!!errors.authToken}
            aria-describedby={errors.authToken ? "integration-token-error" : undefined}
          />
          {errors.authToken ? (
            <p id="integration-token-error" role="alert" className="text-xs text-status-critical">
              {errors.authToken}
            </p>
          ) : null}
        </FormField>
      ) : null}

      <FormField label="Extra headers" htmlFor="integration-headers" hint="One per line, as Name: value.">
        <Textarea
          id="integration-headers"
          name="headers"
          className="min-h-20 resize-y font-mono text-xs"
          spellCheck={false}
          value={draft.headers}
          onChange={(e) => setDraft({ ...draft, headers: e.target.value })}
        />
      </FormField>

      <div className="flex items-center gap-3">
        <Switch id="integration-enabled" checked={draft.enabled} onCheckedChange={(on) => setDraft({ ...draft, enabled: on })} />
        <Label htmlFor="integration-enabled">Start sending right away</Label>
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => setDraft(null)}>
          Cancel
        </Button>
        <Button type="submit" disabled={createMut.isPending}>
          {createMut.isPending ? "Connecting…" : "Connect"}
        </Button>
      </DialogFooter>
    </form>
  );
}
