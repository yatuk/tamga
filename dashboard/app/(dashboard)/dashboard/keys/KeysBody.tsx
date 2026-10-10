"use client";

import { useRef, useState } from "react";
import {
  Button,
  CodeSnippet,
  DataTableSkeleton,
  Modal,
  RadioButton,
  RadioButtonGroup,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TextInput,
} from "@carbon/react";
import { Add } from "@carbon/icons-react";
import { counted, dayLabel } from "@/components/console/names";
import { DataNotice, PageHead, Remove } from "@/components/console/page";
import { formatSince } from "@/lib/utils/format";
import type { KeyScope } from "./_constants";
import type { useKeysPage } from "./useKeysPage";

type Props = ReturnType<typeof useKeysPage>;

const SCOPES: { value: KeyScope; name: string; can: string }[] = [
  { value: "proxy", name: "Application", can: "Identifies an application that sends requests through the proxy. Opens nothing in this console." },
  { value: "read", name: "Read", can: "Reads statistics and events through the management API." },
  { value: "write", name: "Write", can: "Also triages incidents and edits policies and patterns." },
  { value: "admin", name: "Admin", can: "Everything, including creating and revoking keys." },
];
const SCOPE_WORDS: Record<string, string> = {
  proxy: "Identify an application",
  read: "Read",
  write: "Read and change",
  admin: "Everything, keys included",
};

const STALE_AFTER_MS = 30 * 24 * 60 * 60 * 1000;
const olderThanThat = (when?: string | null) => !!when && Date.now() - new Date(when).getTime() > STALE_AFTER_MS;
/** Idle long enough to question: last used, or created and never used, over 30 days ago. */
const idle = (key: { last_used?: string | null; created_at: string }) => olderThanThat(key.last_used ?? key.created_at);

export function KeysBody({
  adminKey,
  isLoading,
  hasError,
  listError,
  apiKeys,
  createOpen,
  setCreateOpen,
  createMutation,
  deleteMutation,
  revealedKey,
  dismissReveal,
}: Props) {
  const [label, setLabel] = useState("");
  const [scope, setScope] = useState<KeyScope>("proxy");
  const [orgId, setOrgId] = useState("");
  const newKey = useRef<HTMLButtonElement>(null);

  const admins = apiKeys.filter((k) => k.scope === "admin").length;
  const apps = apiKeys.filter((k) => k.scope === "proxy").length;
  const unused = apiKeys.filter(idle).length;
  const never = apiKeys.filter((k) => !k.last_used && !idle(k)).length;

  const create = () => {
    if (!label.trim()) return;
    // The form is emptied only once the key exists, so a failed attempt keeps what was typed.
    createMutation.mutate(
      { label: label.trim(), scope, orgId: scope === "proxy" ? orgId.trim() || undefined : undefined },
      {
        onSuccess: () => {
          setLabel("");
          setScope("proxy");
          setOrgId("");
        },
      },
    );
  };

  return (
    <>
      <PageHead
        title="API keys"
        lede={
          apiKeys.length === 0
            ? "An application key identifies a caller on the proxy, sent as X-Tamga-Key. Read, write and admin keys open the management API."
            : `${counted(apiKeys.length, "key")}: ${apps} identify applications on the proxy, ${admins} can do everything. ${
                unused > 0
                  ? `${unused} ${unused === 1 ? "has" : "have"} not been used for 30 days and could be revoked.`
                  : never > 0
                    ? `${never} ${never === 1 ? "has" : "have"} not been used yet.`
                    : "All were used in the last 30 days."
              }`
        }
      >
        <Button ref={newKey} kind="primary" size="sm" renderIcon={Add} onClick={() => setCreateOpen(true)} disabled={!adminKey}>
          New key
        </Button>
      </PageHead>

      <DataNotice adminKey={adminKey} failed={hasError} error={listError} what="API keys" />

      {!adminKey || hasError ? null : isLoading ? (
        <div style={{ marginTop: "1.5rem" }}>
          <DataTableSkeleton columnCount={5} rowCount={5} showHeader={false} showToolbar={false} />
        </div>
      ) : apiKeys.length === 0 ? (
        <p className="t-empty">No keys yet. Create one for each application that sends requests through the proxy.</p>
      ) : (
        <TableContainer className="t-table" style={{ marginTop: "1.5rem" }}>
          <Table size="md" aria-label="API keys">
            <TableHead>
              <TableRow>
                <TableHeader>Name</TableHeader>
                <TableHeader>What it can do</TableHeader>
                <TableHeader>Starts with</TableHeader>
                <TableHeader>Created</TableHeader>
                <TableHeader>Last used</TableHeader>
                <TableHeader>
                  <span className="cds--visually-hidden">Revoke</span>
                </TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              {apiKeys.map((key) => (
                <TableRow key={key.id}>
                  <TableCell>
                    {key.label}
                    {key.scope === "proxy" ? <span className="t-note t-under">Organisation {key.org_id || "default"}</span> : null}
                  </TableCell>
                  <TableCell>{SCOPE_WORDS[key.scope] ?? key.scope}</TableCell>
                  <TableCell className="t-data">{key.prefix}</TableCell>
                  <TableCell title={key.created_at}>{dayLabel(key.created_at)}</TableCell>
                  <TableCell title={key.last_used ?? undefined}>
                    {key.last_used ? formatSince(key.last_used) : "Never"}
                    {idle(key) ? <span className="t-note"> Idle for over 30 days</span> : null}
                  </TableCell>
                  <TableCell>
                    <span className="t-rowactions">
                      <Remove
                        label={`Revoke ${key.label}`}
                        text="Revoke"
                        verb="Revoke the key for good"
                        disabled={deleteMutation.isPending}
                        onConfirm={() => deleteMutation.mutate(key.id)}
                      />
                    </span>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Modal
        open={createOpen}
        size="sm"
        modalHeading="New API key"
        launcherButtonRef={newKey}
        primaryButtonText={createMutation.isPending ? "Creating" : "Create key"}
        primaryButtonDisabled={!label.trim() || createMutation.isPending}
        secondaryButtonText="Cancel"
        onRequestClose={() => setCreateOpen(false)}
        onRequestSubmit={create}
      >
        <p style={{ marginBottom: "1rem" }}>The key is shown once, right after you create it.</p>
        <div className="t-form" style={{ paddingTop: 0 }}>
          <TextInput
            id="new-key-name"
            labelText="Name (required)"
            helperText="Name it after the application that will use it."
            placeholder="billing-service"
            autoComplete="off"
            data-modal-primary-focus
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
          <RadioButtonGroup
            legendText="What it can do"
            name="scope"
            orientation="vertical"
            valueSelected={scope}
            onChange={(value) => setScope(value as KeyScope)}
          >
            {SCOPES.map((s) => (
              <RadioButton key={s.value} id={`scope-${s.value}`} value={s.value} labelText={`${s.name}. ${s.can}`} />
            ))}
          </RadioButtonGroup>
          {scope === "proxy" ? (
            <TextInput
              id="new-key-org"
              labelText="Organisation (optional)"
              helperText="Requests made with this key are counted under it, whatever the request says."
              placeholder="finance"
              autoComplete="off"
              value={orgId}
              onChange={(e) => setOrgId(e.target.value)}
            />
          ) : null}
        </div>
      </Modal>

      {/* The full key is shown once. This closes only by its own button. */}
      <Modal
        open={!!revealedKey}
        size="sm"
        modalHeading="Copy the key now"
        primaryButtonText="I have saved it"
        preventCloseOnClickOutside
        onRequestClose={() => undefined}
        onRequestSubmit={dismissReveal}
        className="t-reveal"
      >
        <p style={{ marginBottom: "1rem" }}>
          This is the only time the full key for <b>{revealedKey?.label}</b> is shown. Put it in your secret manager.
        </p>
        <CodeSnippet type="multi" feedback="Copied" wrapText>
          {revealedKey?.rawKey ?? ""}
        </CodeSnippet>
      </Modal>
    </>
  );
}
