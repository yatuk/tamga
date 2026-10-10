"use client";

import dynamic from "next/dynamic";
import { Button, InlineNotification, SkeletonText, Tab, TabList, TabPanel, TabPanels, Tabs } from "@carbon/react";
import { Renew, Save } from "@carbon/icons-react";
import { dayLabel } from "@/components/console/names";
import { DataNotice, PageHead, Remove, Section } from "@/components/console/page";
import { useHealthDetail } from "@/hooks/useHealth";
import { POLICY_TABS } from "./_constants";
import { appendCustomEntity, appendRateLimitTemplate, strengthenInjection } from "./snippets";
import { Proposals } from "./proposals";
import { maskedNotifyRules, NOTIFY_MASK } from "./policyUtils";
import { ChangedNotice, MASKED_WAY_OUT, RefusalNotice, sectionWords } from "./refusal";
import { Changes, Competitors, Entities, History, TryIt } from "./tabs";
import { usePoliciesPage } from "./usePoliciesPage";

// CodeMirror is large and touches the DOM on import, so the editor is its own
// chunk, loaded on the client when it is shown.
const PolicyCodeEditor = dynamic(() => import("./policy-code-editor"), {
  ssr: false,
  loading: () => <SkeletonText paragraph lineCount={12} />,
});

/**
 * Said before anything is tried: the document holds an alert address the
 * proxy masked, so a write of the whole document will be refused.
 */
function MaskedNotice({ running, draft, dualControl }: { running: string[]; draft: string[]; dualControl: boolean }) {
  const blocked = draft.length > 0;
  const button = dualControl ? "Propose this change" : "Save and apply";
  return (
    <InlineNotification
      kind="info"
      lowContrast
      hideCloseButton
      title={running.length > 0 ? "This key cannot see where alerts are sent." : "The draft holds a masked alert address."}
      subtitle={[
        running.length > 0
          ? `The proxy shows the address of an alert to an admin only; here each reads as its host and ${NOTIFY_MASK}.`
          : "It was read with a key that is not an admin key, which hides where alerts are sent.",
        blocked
          ? `A masked address cannot be saved back, so the proxy refuses a save or a proposal of this document, and ${button} is off.`
          : "The draft no longer holds one, but changing where alerts are sent is for an admin key.",
        MASKED_WAY_OUT,
      ].join(" ")}
      className="t-refusal"
      style={{ marginTop: "1.5rem", maxWidth: "none" }}
    >
      <ul className="t-issues" aria-label="Rules with a masked alert address">
        {(blocked ? draft : running).map((path) => (
          <li key={path}>
            <span className="t-data">{path}</span>
            <span>{sectionWords(path)}</span>
          </li>
        ))}
      </ul>
    </InlineNotification>
  );
}

export default function PoliciesPage() {
  const m = usePoliciesPage();
  const { activePolicy } = m;
  // A proxy that cannot write its policy file says so; older ones say nothing and are assumed able.
  const readOnly = useHealthDetail().data?.policy_writable === false;
  const dirty = Boolean(m.originalYaml && m.draft && m.originalYaml !== m.draft);
  // Below admin the proxy hands out the address of an alert masked, and takes a
  // masked address back from nobody. The policy as this key reads it says which
  // case this is; the draft says whether a write of it would be refused.
  const maskedRunning = maskedNotifyRules(m.originalYaml);
  const maskedDraft = maskedNotifyRules(m.draft);
  const masked = maskedDraft.length > 0;
  // The running policy says whether a change needs a second person. Then nothing is
  // written directly, and the way to change it is a proposal.
  const governance = activePolicy?.governance as { require_dual_control?: boolean } | null | undefined;
  const dualControl = governance?.require_dual_control === true;
  const version = typeof activePolicy?.version === "string" ? activePolicy.version : null;
  const updatedAt = typeof activePolicy?.updated_at === "string" ? activePolicy.updated_at : null;
  const index = Math.max(0, POLICY_TABS.findIndex((t) => t.id === m.tab));

  return (
    <>
      <PageHead
        title="Policies"
        lede={
          <>
            The rules that decide what is stopped, redacted, warned about or let through.
            {version ? ` The proxy is running version ${version}` : ""}
            {version && updatedAt ? `, loaded ${dayLabel(updatedAt)}.` : version ? "." : ""}
          </>
        }
      >
        {m.adminKey ? (
          <>
            {dirty ? <span className="t-unsaved">Unsaved changes</span> : null}
            <Remove
              label="Discard the draft"
              text="Discard"
              verb="Discard the draft"
              disabled={!dirty}
              onConfirm={() => m.setDraft(m.originalYaml)}
            />
            <Button kind="tertiary" size="sm" renderIcon={Renew} onClick={() => void m.onReload()}>
              Reload from disk
            </Button>
            {dualControl ? (
              <Button kind="primary" size="sm" onClick={() => m.setTab("proposals")} disabled={!dirty || masked}>
                Propose this change
              </Button>
            ) : (
              <Button kind="primary" size="sm" renderIcon={Save} onClick={() => void m.onSave()} disabled={m.saving || !dirty || readOnly || masked}>
                {m.saving ? "Saving" : "Save and apply"}
              </Button>
            )}
          </>
        ) : null}
      </PageHead>

      <DataNotice adminKey={m.adminKey} failed={!!m.error} />

      {readOnly ? (
        <InlineNotification
          kind="info"
          lowContrast
          hideCloseButton
          title="The policy file is read-only where the proxy runs."
          subtitle="You can read it here, try a draft against a prompt and see what it would change, but not save. Edit the file there and choose Reload from disk, or mount it writable."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}

      {maskedRunning.length > 0 || masked ? (
        <MaskedNotice running={maskedRunning} draft={maskedDraft} dualControl={dualControl} />
      ) : null}

      {dualControl ? (
        <InlineNotification
          kind="info"
          lowContrast
          hideCloseButton
          title="This proxy requires a second person for every policy change."
          subtitle="Nothing is saved directly. Propose the draft under Proposals, and someone else approves it with a key of their own."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}

      {m.outcome?.kind === "refused" ? (
        <RefusalNotice refusal={m.outcome.refusal} onPropose={() => m.setTab("proposals")} onClose={() => m.setOutcome(null)} />
      ) : m.outcome ? (
        <ChangedNotice
          title={m.outcome.kind === "saved" ? "Policy saved. The proxy is now enforcing it." : "Policy reloaded from disk."}
          paths={m.outcome.changed}
          none={
            m.outcome.kind === "saved"
              ? "It matched the running policy, so nothing changed."
              : "The file matched the running policy, so nothing changed."
          }
          warnings={m.outcome.warnings}
          onClose={() => m.setOutcome(null)}
        />
      ) : null}

      {!m.adminKey || m.error ? null : m.isLoading ? (
        <div style={{ marginTop: "2rem" }}>
          <SkeletonText paragraph lineCount={10} />
        </div>
      ) : (
        <div className="t-tabs">
          <Tabs selectedIndex={index} onChange={({ selectedIndex }) => m.setTab(POLICY_TABS[selectedIndex].id)}>
            <TabList aria-label="Parts of the policy">
              {POLICY_TABS.map((t) => (
                <Tab key={t.id}>{t.label}</Tab>
              ))}
            </TabList>
            {/* Each part is built only while it is the one shown. */}
            <TabPanels>
              <TabPanel>
                {m.tab === "editor" ? (
                  <Section id="document" title="The policy document" note={`${m.draft.split("\n").length} lines of JSON`}>
                    <div className="t-examples" style={{ margin: "0.75rem 0" }}>
                      <span className="t-note">Insert into the draft:</span>
                      <Button kind="ghost" size="sm" onClick={() => m.setDraft(appendCustomEntity(m.draft))}>
                        A custom entity
                      </Button>
                      <Button kind="ghost" size="sm" onClick={() => m.setDraft(appendRateLimitTemplate(m.draft))}>
                        A rate limit
                      </Button>
                      <Button kind="ghost" size="sm" onClick={() => m.setDraft(strengthenInjection(m.draft))}>
                        Stop prompt injection
                      </Button>
                    </div>
                    <div className="t-editor">
                      <PolicyCodeEditor value={m.draft} onChange={m.setDraft} />
                    </div>
                  </Section>
                ) : null}
              </TabPanel>
              <TabPanel>{m.tab === "diff" ? <Changes original={m.originalYaml} draft={m.draft} /> : null}</TabPanel>
              <TabPanel>
                {m.tab === "simulate" ? (
                  <TryIt sample={m.sample} onSample={m.setSample} running={m.simulating} onRun={() => void m.onSimulate()} result={m.simResult} />
                ) : null}
              </TabPanel>
              <TabPanel>
                {m.tab === "proposals" ? (
                  <Proposals
                    adminKey={m.adminKey}
                    draft={m.draft}
                    running={m.originalYaml}
                    dirty={dirty}
                    masked={masked}
                    readOnly={readOnly}
                    dualControl={dualControl}
                    onApplied={() => void m.refetch()}
                  />
                ) : null}
              </TabPanel>
              <TabPanel>{m.tab === "history" ? <History adminKey={m.adminKey} /> : null}</TabPanel>
              <TabPanel>
                {m.tab === "entities" ? <Entities adminKey={m.adminKey} readOnly={readOnly} dualControl={dualControl} /> : null}
              </TabPanel>
              <TabPanel>{m.tab === "competitors" ? <Competitors adminKey={m.adminKey} /> : null}</TabPanel>
            </TabPanels>
          </Tabs>
        </div>
      )}
    </>
  );
}
