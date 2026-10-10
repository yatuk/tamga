"use client";

import type { ReactNode } from "react";
import { ActionableNotification, InlineNotification } from "@carbon/react";
import { ApiError, sessionRefused } from "@/lib/api/fetch-core";
import type { PolicyIssue } from "@/lib/api/types-extended";

/** What the person was doing when the proxy said no. It decides how "nothing happened" is worded. */
export type PolicyAction = "save" | "propose" | "approve" | "reject" | "entity-add" | "entity-delete" | "reload";

export interface Refusal {
  /** The proxy's code, or "" for a failure it gave no code for. */
  code: string;
  title: string;
  detail: string;
  /** admin_role_required: the sections this key may not change, in words and as written in the document. */
  sections?: { words: string; path: string }[];
  /** 422: one entry per field that broke a limit. */
  issues?: PolicyIssue[];
  /** dual_control_required on a draft: the draft can be proposed instead. */
  canPropose?: boolean;
}

const SECTION_WORDS: Record<string, string> = {
  providers: "Providers",
  exceptions: "Exceptions",
  governance: "Governance",
  data: "Data handling",
  pricing: "Pricing",
  vault: "The vault",
  cache: "The cache",
  body_limits: "Body limits",
  rate_limit: "Rate limits",
  cost: "Cost limits",
  scan: "Scan settings",
  operator_state: "Operator state",
  canary: "Canary settings",
};

/** A section the proxy named, in words: "rules.pii_detection.notify" is the alert destinations of that rule. */
export function sectionWords(path: string): string {
  const notify = path.match(/^rules\.(.+)\.notify$/);
  if (notify) return `Alert destinations of rule ${notify[1]}`;
  const known = SECTION_WORDS[path];
  if (known) return known;
  const words = path.replace(/[._]/g, " ");
  return words ? words[0].toUpperCase() + words.slice(1) : path;
}

// What did not happen, per action. Every write is all or nothing: the proxy
// checks the whole document before it touches the file.
const NOT_DONE: Record<PolicyAction, string> = {
  save: "Nothing was saved",
  propose: "The proposal was not opened",
  approve: "The proposal was not applied and is still open",
  reject: "The proposal is still open",
  "entity-add": "The entity was not added",
  "entity-delete": "The entity was not deleted",
  reload: "The policy was not reloaded",
};

const FAILED: Record<PolicyAction, string> = {
  save: "The policy could not be saved.",
  propose: "The proposal could not be opened.",
  approve: "The proposal could not be approved.",
  reject: "The proposal could not be rejected.",
  "entity-add": "The entity could not be added.",
  "entity-delete": "The entity could not be deleted.",
  reload: "The policy could not be reloaded.",
};

/** What to do about a masked alert address. Said before a write is tried, and again if the proxy refuses one. */
export const MASKED_WAY_OUT =
  "With an admin key, write each address in full or, better, as a reference such as ${SLACK_WEBHOOK_URL}: a reference is never masked.";

function issuesOf(error: ApiError): PolicyIssue[] {
  const inner = error.body.error;
  const details = inner && typeof inner === "object" ? (inner as { details?: unknown }).details : undefined;
  return Array.isArray(details) ? (details as PolicyIssue[]) : [];
}

/** Turns what the proxy answered into what happened and what the person can do about it. */
export function explainRefusal(error: unknown, action: PolicyAction): Refusal {
  const message = error instanceof Error ? error.message : "";
  if (!(error instanceof ApiError)) {
    return { code: "", title: FAILED[action], detail: message || "The proxy did not answer." };
  }
  const notDone = NOT_DONE[action];
  const entity = action === "entity-add" || action === "entity-delete";

  switch (error.code) {
    case "admin_role_required": {
      const raw = error.body.privileged_sections;
      const paths = Array.isArray(raw) ? raw.filter((p): p is string => typeof p === "string") : [];
      const rest =
        action === "save" || action === "propose"
          ? `${notDone}, including the parts this key may change. Take these sections out of the draft, or use an admin key under Settings.`
          : action === "reject"
            ? `${notDone}. Its author can withdraw it, or someone with an admin key can reject it.`
            : `${notDone}. Use an admin key under Settings.`;
      return {
        code: error.code,
        title: "The key in use is not an admin key, and this change touches parts only an admin may change.",
        detail: rest,
        sections: paths.map((path) => ({ words: sectionWords(path), path })),
      };
    }
    case "dual_control_required":
      return {
        code: error.code,
        title: "This proxy requires a second person for every policy change.",
        detail: entity
          ? `${notDone}. Make the change to custom_entities in the document under Editor and propose it, for someone else to approve.`
          : `${notDone}. Propose the draft instead, and someone else approves it.`,
        canPropose: !entity,
      };
    case "self_approval":
      return {
        code: error.code,
        title: "You cannot approve this proposal.",
        detail:
          "Under dual control a proposal is approved by someone other than its author, using a key of their own. The shared admin key counts as one person, whoever holds it.",
      };
    case "proposal_closed": {
      const status = error.body.status === "approved" || error.body.status === "rejected" ? error.body.status : "settled";
      return {
        code: error.code,
        title: `This proposal was already ${status}.`,
        detail: "There is nothing left to decide. The list below is up to date.",
      };
    }
    case "proposal_stale":
      return {
        code: error.code,
        title: "The policy changed after this proposal was written.",
        detail: "Approving it would undo that change, so it was not applied. Reject it and propose the change again from the current policy.",
      };
    case "policy_read_only":
      return {
        code: error.code,
        title: "The policy file is read-only where the proxy runs.",
        detail: `${notDone}. Edit the file there and choose Reload from disk, or mount it writable.`,
      };
    case "validation_failed": {
      const issues = issuesOf(error);
      const n = issues.length;
      return {
        code: error.code,
        title: n === 1 ? "One setting in the policy is not allowed." : `${n} settings in the policy are not allowed.`,
        detail: `${notDone}. Fix ${n === 1 ? "it" : "them"} in the document and try again.`,
        issues,
      };
    }
    case "masked_value": {
      // The proxy hides the address of an alert from every role below admin, and
      // takes the text it handed out that way back from nobody, an admin included.
      const raw = error.body.fields;
      const paths = Array.isArray(raw) ? raw.filter((p): p is string => typeof p === "string") : [];
      return {
        code: error.code,
        title: "The policy holds a masked alert address, which cannot be saved back.",
        detail: `${notDone}. ${MASKED_WAY_OUT}`,
        sections: paths.map((path) => ({ words: sectionWords(path), path })),
      };
    }
  }
  const session = sessionRefused(error);
  if (session) return { code: error.code, title: session.title, detail: `${notDone}. ${session.detail}` };
  if (error.code === "permission_denied") {
    const required = error.body.required_permission;
    return {
      code: error.code,
      title: "This key's role cannot do this.",
      detail: `${notDone}. ${
        typeof required === "string" && required ? `The proxy asks for the ${required} permission.` : "The proxy asks for a higher role."
      } Use a key with a higher role under Settings.`,
    };
  }
  if (error.status === 401) {
    return { code: "", title: "The proxy did not accept the admin key.", detail: `${notDone}. Check the key under Settings.` };
  }
  if (error.status === 403) {
    return { code: "", title: "The key in use may not do this.", detail: `${notDone}. The proxy said: ${message}` };
  }
  if (error.status === 400 && message.startsWith("policy parse:")) {
    return {
      code: "",
      title: "The proxy could not read the draft as a policy.",
      detail: `${notDone}. ${message.replace(/^policy parse:\s*/, "")}`,
    };
  }
  return { code: "", title: FAILED[action], detail: message };
}

/** Issues of a policy check, each under the path of the field it is about. */
export function IssueList({ issues }: { issues: PolicyIssue[] }) {
  if (issues.length === 0) return null;
  return (
    <ul className="t-issues">
      {issues.map((issue, i) => (
        <li key={`${issue.field ?? ""}-${i}`}>
          <span className="t-data">{issue.field || "policy"}</span>
          <span>{issue.message || issue.rule || "Not allowed."}</span>
        </li>
      ))}
    </ul>
  );
}

/** A refusal, shown where the action was taken. */
export function RefusalNotice({
  refusal,
  onPropose,
  onClose,
  children,
}: {
  refusal: Refusal;
  /** Where to go to propose the draft; offered only for a refusal that allows it. */
  onPropose?: () => void;
  onClose?: () => void;
  children?: ReactNode;
}) {
  // A notice with a way on is Carbon's actionable one: the plain notice takes no button.
  if (refusal.canPropose && onPropose) {
    return (
      <ActionableNotification
        kind="error"
        lowContrast
        inline
        hasFocus={false}
        hideCloseButton={!onClose}
        onCloseButtonClick={onClose}
        title={refusal.title}
        subtitle={refusal.detail}
        actionButtonLabel="Propose this change"
        onActionButtonClick={onPropose}
        style={{ marginTop: "1.5rem", maxWidth: "none" }}
      />
    );
  }
  return (
    <InlineNotification
      kind="error"
      lowContrast
      hideCloseButton={!onClose}
      onCloseButtonClick={onClose}
      title={refusal.title}
      subtitle={refusal.detail}
      className="t-refusal"
      style={{ marginTop: "1.5rem", maxWidth: "none" }}
    >
      {refusal.sections && refusal.sections.length > 0 ? (
        <ul className="t-issues">
          {refusal.sections.map((s) => (
            <li key={s.path}>
              <span className="t-data">{s.path}</span>
              <span>{s.words}</span>
            </li>
          ))}
        </ul>
      ) : null}
      {refusal.issues ? <IssueList issues={refusal.issues} /> : null}
      {children}
    </InlineNotification>
  );
}

/** The paths a write changed, as the proxy names them. Ochre, as everything that was changed. */
export function ChangedPaths({ paths }: { paths: string[] }) {
  if (paths.length === 0) return null;
  return (
    <ul className="t-changed" aria-label="What changed">
      {paths.map((path) => (
        <li key={path} className="t-data">
          {path}
        </li>
      ))}
    </ul>
  );
}

/** Says a write went through and names what it changed. */
export function ChangedNotice({
  title,
  paths,
  none,
  warnings,
  onClose,
}: {
  title: string;
  paths: string[];
  /** What to say when the write changed nothing. */
  none: string;
  warnings?: PolicyIssue[];
  onClose?: () => void;
}) {
  const n = paths.length;
  return (
    <InlineNotification
      kind="success"
      lowContrast
      hideCloseButton={!onClose}
      onCloseButtonClick={onClose}
      title={title}
      subtitle={n === 0 ? none : `${n === 1 ? "One part" : `${n} parts`} of the policy changed:`}
      className="t-refusal"
      style={{ marginTop: "1.5rem", maxWidth: "none" }}
    >
      <ChangedPaths paths={paths} />
      {warnings && warnings.length > 0 ? (
        <>
          <p className="t-note" style={{ marginTop: "0.75rem" }}>
            {warnings.length === 1 ? "The proxy has one remark on it:" : `The proxy has ${warnings.length} remarks on it:`}
          </p>
          <IssueList issues={warnings} />
        </>
      ) : null}
    </InlineNotification>
  );
}
