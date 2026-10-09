"use client";

import dynamic from "next/dynamic";
import { Button, InlineNotification, SkeletonText, Tab, TabList, TabPanel, TabPanels, Tabs } from "@carbon/react";
import { Renew, Save } from "@carbon/icons-react";
import { dayLabel } from "@/components/console/names";
import { DataNotice, PageHead, Remove, Section } from "@/components/console/page";
import { useHealthDetail } from "@/hooks/useHealth";
import { POLICY_TABS } from "./_constants";
import { appendCustomEntity, appendRateLimitTemplate, strengthenInjection } from "./snippets";
import { Changes, Competitors, Entities, History, TryIt } from "./tabs";
import { usePoliciesPage } from "./usePoliciesPage";

// CodeMirror is large and touches the DOM on import, so the editor is its own
// chunk, loaded on the client when it is shown.
const PolicyCodeEditor = dynamic(() => import("./policy-code-editor"), {
  ssr: false,
  loading: () => <SkeletonText paragraph lineCount={12} />,
});

export default function PoliciesPage() {
  const m = usePoliciesPage();
  const { activePolicy } = m;
  // A proxy that cannot write its policy file says so; older ones say nothing and are assumed able.
  const readOnly = useHealthDetail().data?.policy_writable === false;
  const dirty = Boolean(m.originalYaml && m.draft && m.originalYaml !== m.draft);
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
            <Button kind="primary" size="sm" renderIcon={Save} onClick={() => void m.onSave()} disabled={m.saving || !dirty || readOnly}>
              {m.saving ? "Saving" : "Save and apply"}
            </Button>
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
              <TabPanel>{m.tab === "history" ? <History adminKey={m.adminKey} /> : null}</TabPanel>
              <TabPanel>{m.tab === "entities" ? <Entities adminKey={m.adminKey} readOnly={readOnly} /> : null}</TabPanel>
              <TabPanel>{m.tab === "competitors" ? <Competitors adminKey={m.adminKey} /> : null}</TabPanel>
            </TabPanels>
          </Tabs>
        </div>
      )}
    </>
  );
}
