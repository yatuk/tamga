"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, InlineNotification, SkeletonText, TextInput } from "@carbon/react";
import { Diff, diffLines, diffWords } from "@/components/console/diff";
import { clockTime, dayLabel } from "@/components/console/names";
import { Remove, Section } from "@/components/console/page";
import { api, type PolicyProposal } from "@/lib/api";
import { ChangedNotice, explainRefusal, RefusalNotice, type Refusal } from "./refusal";

const when = (iso?: string) => (iso ? `${dayLabel(iso)}, ${clockTime(iso)}` : "at a time that was not recorded");

/** Who the proxy verified a request as, in words. */
export function actorName(actor?: string): string {
  if (!actor) return "an unknown author";
  if (actor === "admin-key") return "the shared admin key";
  if (actor === "dev-mode") return "dev mode, without a key";
  if (actor === "policy-file") return "the policy file on disk";
  if (actor.startsWith("apikey:")) return `API key ${actor.slice("apikey:".length)}`;
  if (actor.startsWith("user:")) return `user ${actor.slice("user:".length)}`;
  return actor;
}

/** A proposal's text laid out as the running policy is, so the two can be compared line by line. */
function sameLayout(text: string): string | null {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return null;
  }
}

/** What a proposal would change against the policy the proxy is running. */
function ProposedChange({ proposal, running }: { proposal: PolicyProposal; running: string }) {
  const text = useMemo(() => sameLayout(proposal.yaml), [proposal.yaml]);
  const lines = useMemo(() => (text === null ? [] : diffLines(running, text)), [running, text]);
  if (text === null) {
    return (
      <details>
        <summary>Read the proposed policy</summary>
        <p className="t-note" style={{ marginTop: "0.75rem" }}>
          It was written as YAML, so it is shown whole and not compared with the running policy.
        </p>
        <div className="t-diff">
          <div>
            <pre style={{ paddingLeft: "1rem" }}>{proposal.yaml}</pre>
          </div>
        </div>
      </details>
    );
  }
  if (lines.length === 0) return <p className="t-note">It is the same as the policy the proxy is running.</p>;
  return (
    <details>
      <summary>What it would change: {diffWords(lines)}</summary>
      <Diff lines={lines} />
    </details>
  );
}

/**
 * Policy changes that wait for a second person, and the form that opens one
 * from the draft. Under dual control this is the only way to change the policy.
 */
export function Proposals({
  adminKey,
  draft,
  running,
  dirty,
  readOnly = false,
  dualControl = false,
  onApplied,
}: {
  adminKey: string;
  draft: string;
  /** The policy the proxy is running, as the editor shows it. */
  running: string;
  dirty: boolean;
  readOnly?: boolean;
  dualControl?: boolean;
  /** Called once an approved proposal is in force. */
  onApplied?: () => void;
}) {
  const qc = useQueryClient();
  const [message, setMessage] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [opened, setOpened] = useState<PolicyProposal | null>(null);
  const [proposeRefusal, setProposeRefusal] = useState<Refusal | null>(null);
  const [refused, setRefused] = useState<{ id: string; refusal: Refusal } | null>(null);
  const [applied, setApplied] = useState<string[] | null>(null);

  const list = useQuery({
    queryKey: ["tamga-policy-proposals", adminKey],
    queryFn: () => api.listPolicyProposals(adminKey),
    enabled: !!adminKey,
    staleTime: 15_000,
  });
  const refresh = () => qc.invalidateQueries({ queryKey: ["tamga-policy-proposals", adminKey] });

  const propose = useMutation({
    mutationFn: () => api.createPolicyProposal(adminKey, draft, message.trim()),
    onSuccess: (proposal) => {
      setOpened(proposal);
      setMessage("");
      setSubmitted(false);
      void refresh();
    },
    onError: (e) => setProposeRefusal(explainRefusal(e, "propose")),
  });
  const approve = useMutation({
    mutationFn: (id: string) => api.approvePolicyProposal(adminKey, id),
    onSuccess: (res) => {
      setApplied(res.changed ?? []);
      void refresh();
      // The running policy is another one now: everything read from it is stale.
      for (const key of ["tamga-policy-history", "custom-entities", "policy-competitors"]) {
        void qc.invalidateQueries({ queryKey: [key, adminKey] });
      }
      onApplied?.();
    },
    onError: (e, id) => {
      setRefused({ id, refusal: explainRefusal(e, "approve") });
      void refresh();
    },
  });
  const reject = useMutation({
    mutationFn: (id: string) => api.rejectPolicyProposal(adminKey, id),
    onSuccess: () => void refresh(),
    onError: (e, id) => {
      setRefused({ id, refusal: explainRefusal(e, "reject") });
      void refresh();
    },
  });
  const busy = approve.isPending || reject.isPending;

  // A proposal that was just refused stays in view with the reason, even if it turned out to be settled.
  const waiting = useMemo(
    () =>
      [...(list.data ?? [])]
        .filter((p) => p.status === "open" || p.id === refused?.id)
        .sort((a, b) => new Date(b.timestamp || 0).getTime() - new Date(a.timestamp || 0).getTime()),
    [list.data, refused],
  );

  const messageError = message.trim() ? "" : "Say what this changes and why. The person who approves it reads this first.";

  function onPropose() {
    setSubmitted(true);
    setOpened(null);
    setProposeRefusal(null);
    if (messageError) return;
    try {
      JSON.parse(draft);
    } catch {
      setProposeRefusal({
        code: "",
        title: "The draft is not valid JSON.",
        detail: "The proposal was not opened. Fix the syntax error under Editor and propose again.",
      });
      return;
    }
    propose.mutate();
  }

  return (
    <div className="t-hunt" style={{ gridTemplateColumns: "minmax(0, 1fr) 24rem", marginTop: 0 }}>
      <Section
        id="proposals"
        title="Waiting for a decision"
        note={
          dualControl
            ? "This proxy requires a second person: the author of a proposal cannot approve it."
            : "Approving one puts it in force at once."
        }
      >
        {applied ? (
          <ChangedNotice
            title="Proposal approved. The proxy is now enforcing it."
            paths={applied}
            none="It matched the running policy, so nothing changed."
            onClose={() => setApplied(null)}
          />
        ) : null}
        {list.isPending ? (
          <div style={{ marginTop: "1rem" }}>
            <SkeletonText paragraph lineCount={3} />
          </div>
        ) : list.isError ? (
          <p className="t-empty">The proposals could not be read.</p>
        ) : waiting.length === 0 ? (
          <p className="t-empty">
            No proposal is waiting. {dirty ? "Propose the draft to open one." : "Edit the document under Editor, then propose the draft here."}
          </p>
        ) : (
          <ul className="t-proposals">
            {waiting.map((p) => (
              <li key={p.id}>
                <div className="t-proposal">
                  <div>
                    <p>{p.message || "No description was given."}</p>
                    <p className="t-note">
                      Proposed by {actorName(p.author)}, {when(p.timestamp)}. <span className="t-data">{p.id.slice(0, 8)}</span>
                    </p>
                  </div>
                  {p.status === "open" ? (
                    <span className="t-rowactions">
                      <Remove
                        label={`Reject proposal ${p.id.slice(0, 8)}`}
                        text="Reject"
                        verb="Reject it"
                        disabled={busy}
                        onConfirm={() => {
                          setRefused(null);
                          reject.mutate(p.id);
                        }}
                      />
                      <Button
                        kind="primary"
                        size="sm"
                        disabled={busy || readOnly}
                        onClick={() => {
                          setRefused(null);
                          setApplied(null);
                          approve.mutate(p.id);
                        }}
                      >
                        {approve.isPending && approve.variables === p.id ? "Applying" : "Approve and apply"}
                      </Button>
                    </span>
                  ) : null}
                </div>
                {refused?.id === p.id ? <RefusalNotice refusal={refused.refusal} onClose={() => setRefused(null)} /> : null}
                {p.status === "open" ? <ProposedChange proposal={p} running={running} /> : null}
              </li>
            ))}
          </ul>
        )}
        {readOnly ? (
          <p className="t-note" style={{ marginTop: "1rem" }}>
            The policy file is read-only where the proxy runs, so a proposal cannot be applied from here.
          </p>
        ) : null}
      </Section>

      <Section id="propose" title="Propose the draft" note={dirty ? undefined : "The draft has no changes"}>
        <form
          noValidate
          className="t-form"
          onSubmit={(e) => {
            e.preventDefault();
            onPropose();
          }}
        >
          <p className="t-note">
            {dirty
              ? "The draft from Editor is sent as it is now. It changes nothing until it is approved."
              : "The draft is the same as the running policy. Edit the document under Editor first."}
          </p>
          <TextInput
            id="proposal-message"
            labelText="What it changes and why"
            placeholder="Stop card numbers instead of redacting them"
            autoComplete="off"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            invalid={submitted && !!messageError}
            invalidText={messageError}
            disabled={!dirty}
          />
          <div className="t-buttons">
            <Button type="submit" kind="primary" size="md" disabled={!dirty || propose.isPending}>
              {propose.isPending ? "Proposing" : "Propose this change"}
            </Button>
          </div>
        </form>
        {proposeRefusal ? <RefusalNotice refusal={proposeRefusal} onClose={() => setProposeRefusal(null)} /> : null}
        {opened ? (
          <InlineNotification
            kind="success"
            lowContrast
            onCloseButtonClick={() => setOpened(null)}
            title="Proposal opened."
            subtitle={
              dualControl
                ? "It waits in the list for someone else to approve. The draft stays in this browser until you discard it."
                : "It waits in the list. The draft stays in this browser until you discard it."
            }
            style={{ marginTop: "1.5rem", maxWidth: "none" }}
          />
        ) : null}
      </Section>
    </div>
  );
}
