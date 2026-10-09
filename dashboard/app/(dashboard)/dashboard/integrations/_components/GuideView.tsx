"use client";

import NextLink from "next/link";
import { Button, CodeSnippet, InlineNotification, Link } from "@carbon/react";
import { Add } from "@carbon/icons-react";
import { Arrive } from "@/components/console/motion";
import { PageHead, Section } from "@/components/console/page";
import type { IntegrationGuide } from "../_data/guides";

/** A setup guide for one destination: what you need, the steps in order, and what goes wrong. */
export function GuideView({ guide }: { guide: IntegrationGuide }) {
  return (
    <>
      <p style={{ marginBottom: "1rem" }}>
        <Link as={NextLink} href="/dashboard/integrations">
          All integrations
        </Link>
      </p>

      <PageHead title={`Setting up ${guide.name}`} lede={guide.overview}>
        <Button as={NextLink} kind="primary" size="sm" renderIcon={Add} href={`/dashboard/integrations?connect=${guide.kind}`}>
          Connect {guide.name}
        </Button>
      </PageHead>

      <div className="t-hunt">
        <div>
          {guide.prerequisites.length > 0 ? (
            <Section id="before" title="Before you start">
              <ul className="t-plainlist">
                {guide.prerequisites.map((p) => (
                  <li key={p}>{p}</li>
                ))}
              </ul>
            </Section>
          ) : null}

          <Section id="steps" title="Steps" note={`${guide.steps.length} in order`} style={{ marginTop: "2.5rem" }}>
            <ol className="t-steps">
              {guide.steps.map((s, i) => (
                <Arrive key={`${i}-${s.title}`} index={i}>
                  <span className="t-stepnumber" aria-hidden="true">
                    {i + 1}
                  </span>
                  <div>
                    <h3>{s.title}</h3>
                    <p>{s.body}</p>
                    {s.code ? (
                      <CodeSnippet type="multi" feedback="Copied" wrapText>
                        {s.code.content}
                      </CodeSnippet>
                    ) : null}
                    {s.note ? (
                      <InlineNotification kind="warning" lowContrast hideCloseButton title="" subtitle={s.note} style={{ maxWidth: "none", marginTop: "0.75rem" }} />
                    ) : null}
                  </div>
                </Arrive>
              ))}
            </ol>
          </Section>

          {guide.headers && guide.headers.length > 0 ? (
            <Section id="headers" title="Headers it needs" style={{ marginTop: "2.5rem" }}>
              <table className="t-kinds">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Value</th>
                    <th>Note</th>
                  </tr>
                </thead>
                <tbody>
                  {guide.headers.map((h) => (
                    <tr key={h.key}>
                      <td className="t-data">{h.key}</td>
                      <td className="t-data">{h.valueHint}</td>
                      <td className="t-note" style={{ whiteSpace: "normal" }}>{h.note ?? ""}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          ) : null}

          <Section id="payload" title="What Tamga sends" style={{ marginTop: "2.5rem" }}>
            <div style={{ marginTop: "1rem" }}>
              <CodeSnippet type="multi" feedback="Copied" maxCollapsedNumberOfRows={18} wrapText>
                {guide.payloadPreview.content}
              </CodeSnippet>
            </div>
          </Section>

          {guide.gotchas.length > 0 ? (
            <Section id="gotchas" title="What tends to go wrong" style={{ marginTop: "2.5rem" }}>
              <dl className="t-roles">
                {guide.gotchas.map((g) => (
                  <div key={g.title}>
                    <dt>{g.title}</dt>
                    <dd>{g.body}</dd>
                  </div>
                ))}
              </dl>
            </Section>
          ) : null}
        </div>

        <aside>
          <Section id="glance" title="At a glance">
            <dl className="t-roles">
              <div>
                <dt>URL looks like</dt>
                <dd className="t-data" style={{ overflowWrap: "anywhere" }}>{guide.urlHint}</dd>
              </div>
              <div>
                <dt>These steps were last checked</dt>
                <dd>{guide.lastVerified}</dd>
              </div>
            </dl>
          </Section>

          {guide.docsLinks.length > 0 ? (
            <Section id="vendor" title="The vendor's own documentation" style={{ marginTop: "2.5rem" }}>
              <ul className="t-plainlist" style={{ listStyle: "none", paddingLeft: 0 }}>
                {guide.docsLinks.map((d) => (
                  <li key={d.href}>
                    <Link href={d.href} target="_blank" rel="noreferrer">
                      {d.label}
                    </Link>{" "}
                    <span className="t-note">opens in a new tab</span>
                  </li>
                ))}
              </ul>
            </Section>
          ) : null}
        </aside>
      </div>
    </>
  );
}
