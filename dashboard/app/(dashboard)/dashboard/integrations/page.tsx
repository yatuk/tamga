"use client";

import { useState } from "react";
import NextLink from "next/link";
import { AnimatePresence, motion } from "motion/react";
import {
  Button,
  Link,
  Modal,
  PasswordInput,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TextArea,
  TextInput,
  Toggle,
} from "@carbon/react";
import { EXPRESSIVE } from "@/components/console/motion";
import { counted } from "@/components/console/names";
import { DataNotice, PageHead, Remove, ScrollTable, Section } from "@/components/console/page";
import { formatSince } from "@/lib/utils/format";
import { type IntegrationDraft, openIntegrationDraft } from "./integrationDraft";
import { webhookHost } from "./integrationWebhookHelpers";
import { INTEGRATION_PRESETS, PRIMARY_PRESETS, SECONDARY_PRESETS, type IntegrationPreset } from "./integrationPresets";
import { useIntegrationsPage } from "./useIntegrationsPage";

const presetName = (kind: string) => INTEGRATION_PRESETS.find((p) => p.kind === kind)?.name ?? kind;
/** The proxy sends the year 1 for a destination nothing has been delivered to. */
const hasFired = (at?: string) => !!at && !at.startsWith("0001-");

/** What is missing before the destination can be created, by field. */
function validate(draft: IntegrationDraft): Partial<Record<"url" | "projectKey" | "authToken", string>> {
  const errors: Partial<Record<"url" | "projectKey" | "authToken", string>> = {};
  if (!draft.url.trim()) errors.url = "Enter the URL to send to.";
  if (draft.kind === "jira" && !draft.projectKey.trim()) errors.projectKey = "Jira needs a project key to create issues.";
  if (draft.kind === "pagerduty" && !draft.authToken.trim()) errors.authToken = "PagerDuty rejects events without a routing key.";
  if (draft.kind === "opsgenie" && !draft.authToken.trim()) errors.authToken = "Opsgenie rejects alerts without an API key.";
  return errors;
}

export default function IntegrationsPage() {
  const { adminKey, draft, setDraft, hooks, error, createMut, testMut, deleteMut } = useIntegrationsPage();
  const [showAll, setShowAll] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const testing = testMut.isPending ? testMut.variables : undefined;
  const sending = hooks.filter((h) => h.enabled).length;
  const connectedOf = (kind: string) => hooks.filter((h) => h.kind === kind).length;

  const preset = draft ? INTEGRATION_PRESETS.find((p) => p.kind === draft.kind) : undefined;
  const errors = draft && submitted ? validate(draft) : {};
  const close = () => {
    setDraft(null);
    setSubmitted(false);
  };

  const row = (p: IntegrationPreset) => {
    const n = connectedOf(p.kind);
    return (
      <li key={p.kind}>
        <div>
          <b>{p.name}</b>
          <span className="t-note">{p.blurb.replace(/ · /g, ", ")}</span>
        </div>
        <span className="t-note">{n > 0 ? `${n} connected` : ""}</span>
        <Link as={NextLink} href={`/dashboard/integrations/${p.kind}`}>
          Setup guide
        </Link>
        <Button kind="tertiary" size="sm" onClick={() => setDraft(openIntegrationDraft(p.kind, p.name))}>
          Connect<span className="cds--visually-hidden"> {p.name}</span>
        </Button>
      </li>
    );
  };

  return (
    <>
      <PageHead
        title="Integrations"
        lede={
          !adminKey || error
            ? "Where stopped requests and alerts are sent."
            : hooks.length === 0
              ? "Where stopped requests and alerts are sent. Nothing is connected yet, so they stay in this console."
              : `Stopped requests and alerts are sent to ${counted(hooks.length, "destination")}, ${sending} of them switched on.`
        }
      />

      <DataNotice adminKey={adminKey} error={error} what="the connected destinations" />

      {hooks.length > 0 ? (
        <Section id="connected" title="Connected" style={{ marginTop: "2rem" }}>
          <ScrollTable className="t-table">
            <Table size="md" aria-label="Connected destinations">
              <TableHead>
                <TableRow>
                  <TableHeader>Name</TableHeader>
                  <TableHeader>Sends to</TableHeader>
                  <TableHeader>Host</TableHeader>
                  <TableHeader>Last delivery</TableHeader>
                  <TableHeader>
                    <span className="cds--visually-hidden">Test or disconnect</span>
                  </TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {hooks.map((h) => (
                  <TableRow key={h.id}>
                    <TableCell>
                      {h.label}
                      {h.enabled ? null : <span className="t-note t-under">Switched off</span>}
                    </TableCell>
                    <TableCell>
                      {presetName(h.kind)}
                      {h.kind === "jira" && h.project_key ? (
                        <span className="t-note t-under">
                          {h.project_key}, {h.issue_type || "Task"}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell className="t-clip" title={webhookHost(h.url)}>
                      <span className="t-data">{webhookHost(h.url)}</span>
                    </TableCell>
                    <TableCell>{hasFired(h.last_fired) ? formatSince(h.last_fired) : "Never"}</TableCell>
                    <TableCell>
                      <span className="t-rowactions">
                        <Button kind="ghost" size="sm" onClick={() => testMut.mutate(h.id)} disabled={testing === h.id}>
                          {testing === h.id ? "Sending" : "Send a test"}
                        </Button>
                        <Remove label={`Disconnect ${h.label}`} verb="Disconnect the destination" onConfirm={() => deleteMut.mutate(h.id)} />
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </ScrollTable>
          <p className="t-footnote">
            Only the host of each destination is shown. The rest of the address and any token in it are kept by the proxy and are not sent to
            this console.
          </p>
        </Section>
      ) : null}

      <Section id="add" title="Add a destination" style={{ marginTop: "2.5rem" }}>
        <ul className="t-destinations">{PRIMARY_PRESETS.map(row)}</ul>
        {/* The rest opens in place, under the ones most used. */}
        <AnimatePresence initial={false}>
          {showAll ? (
            <motion.ul
              key="rest"
              className="t-destinations"
              style={{ overflow: "hidden" }}
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.3, ease: EXPRESSIVE }}
            >
              {SECONDARY_PRESETS.map(row)}
            </motion.ul>
          ) : null}
        </AnimatePresence>
        {SECONDARY_PRESETS.length > 0 ? (
          <Button kind="ghost" size="sm" onClick={() => setShowAll((v) => !v)} aria-expanded={showAll} style={{ marginTop: "0.5rem" }}>
            {showAll ? "Show fewer" : `Show ${SECONDARY_PRESETS.length} more`}
          </Button>
        ) : null}
      </Section>

      <Modal
        open={!!draft}
        size="sm"
        modalHeading={`Connect ${preset?.name ?? draft?.kind ?? ""}`}
        primaryButtonText={createMut.isPending ? "Connecting" : "Connect"}
        primaryButtonDisabled={createMut.isPending}
        secondaryButtonText="Cancel"
        onRequestClose={close}
        onRequestSubmit={() => {
          if (!draft) return;
          setSubmitted(true);
          if (Object.keys(validate(draft)).length === 0) createMut.mutate(draft, { onSuccess: () => setSubmitted(false) });
        }}
      >
        {draft ? (
          <>
            <p style={{ marginBottom: "1rem" }}>
              {preset?.blurb.replace(/ · /g, ", ")}.{" "}
              <Link as={NextLink} href={`/dashboard/integrations/${draft.kind}`}>
                Setup guide
              </Link>
            </p>
            <div className="t-form" style={{ paddingTop: 0 }}>
              <TextInput
                id="integration-label"
                labelText="Name"
                helperText="What this destination is called in the console."
                autoComplete="off"
                value={draft.label}
                onChange={(e) => setDraft({ ...draft, label: e.target.value })}
              />
              <TextInput
                id="integration-url"
                labelText="URL"
                type="url"
                autoComplete="off"
                spellCheck={false}
                data-modal-primary-focus
                placeholder={preset?.urlHint}
                value={draft.url}
                onChange={(e) => setDraft({ ...draft, url: e.target.value })}
                invalid={!!errors.url}
                invalidText={errors.url}
              />
              {draft.kind === "jira" ? (
                <div className="t-pair">
                  <TextInput
                    id="integration-project"
                    labelText="Project key"
                    placeholder="SEC"
                    autoComplete="off"
                    spellCheck={false}
                    value={draft.projectKey}
                    onChange={(e) => setDraft({ ...draft, projectKey: e.target.value.toUpperCase() })}
                    invalid={!!errors.projectKey}
                    invalidText={errors.projectKey}
                  />
                  <TextInput
                    id="integration-issue-type"
                    labelText="Issue type"
                    placeholder="Task"
                    autoComplete="off"
                    value={draft.issueType}
                    onChange={(e) => setDraft({ ...draft, issueType: e.target.value })}
                  />
                </div>
              ) : null}
              {draft.kind === "pagerduty" || draft.kind === "opsgenie" ? (
                <PasswordInput
                  id="integration-token"
                  labelText={draft.kind === "pagerduty" ? "Routing key" : "API key"}
                  helperText={
                    draft.kind === "pagerduty" ? "Sent in the request body as routing_key." : "Sent as the Authorization: GenieKey header."
                  }
                  autoComplete="off"
                  spellCheck={false}
                  value={draft.authToken}
                  onChange={(e) => setDraft({ ...draft, authToken: e.target.value })}
                  invalid={!!errors.authToken}
                  invalidText={errors.authToken}
                />
              ) : null}
              <TextArea
                id="integration-headers"
                data-code=""
                labelText="Extra headers"
                helperText="One per line, as Name: value."
                rows={3}
                spellCheck={false}
                value={draft.headers}
                onChange={(e) => setDraft({ ...draft, headers: e.target.value })}
              />
              <Toggle
                id="integration-enabled"
                size="sm"
                labelText="Start sending right away"
                labelA="No"
                labelB="Yes"
                toggled={draft.enabled}
                onToggle={(on) => setDraft({ ...draft, enabled: on })}
              />
            </div>
          </>
        ) : null}
      </Modal>
    </>
  );
}
