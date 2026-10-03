"use client";

import Link from "next/link";
import { ArrowLeft, ExternalLink, Plus, TriangleAlert } from "lucide-react";
import { CopyButton } from "@/components/app/copy-button";
import { DetailList } from "@/components/app/detail-list";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { IntegrationGuide } from "../_data/guides";

function CodeBlock({ lang, content }: { lang: string; content: string }) {
  return (
    <div className="mt-3 border bg-background">
      <div className="flex items-center justify-between border-b py-1 pr-1 pl-3">
        <span className="font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase">{lang}</span>
        <CopyButton value={content} label={`${lang} snippet`} size="xs">
          Copy
        </CopyButton>
      </div>
      <pre className="overflow-x-auto p-3 font-mono text-xs leading-5" tabIndex={0} translate="no">
        {content}
      </pre>
    </div>
  );
}

function Note({ children }: { children: React.ReactNode }) {
  return (
    <p className="mt-3 flex items-start gap-2 border border-status-medium/40 bg-status-medium-bg p-3 text-sm text-status-medium">
      <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
      <span>{children}</span>
    </p>
  );
}

export function GuideView({ guide }: { guide: IntegrationGuide }) {
  const connectHref = `/dashboard/integrations?connect=${guide.kind}`;

  return (
    <div className="space-y-6">
      <Link
        href="/dashboard/integrations"
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" aria-hidden />
        All integrations
      </Link>

      <PageHeader
        title={`${guide.name} setup`}
        description={guide.overview}
        actions={
          <Button asChild size="sm">
            <Link href={connectHref}>
              <Plus />
              Connect {guide.name}
            </Link>
          </Button>
        }
      />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-6">
          {guide.prerequisites.length > 0 ? (
            <Panel title="Before you start">
              <ul className="list-disc space-y-1.5 py-4 pr-4 pl-8 text-sm text-fg-muted marker:text-fg-faint">
                {guide.prerequisites.map((p) => (
                  <li key={p}>{p}</li>
                ))}
              </ul>
            </Panel>
          ) : null}

          <Panel title="Steps" aside={`${guide.steps.length} steps`}>
            <ol className="divide-y">
              {guide.steps.map((s, i) => (
                <li key={`${i}-${s.title}`} className="flex gap-4 p-4">
                  <span className="font-mono text-sm text-fg-faint tabular-nums" aria-hidden>
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <div className="min-w-0 flex-1">
                    <h3 className="text-sm font-medium">{s.title}</h3>
                    <p className="mt-1 text-sm leading-relaxed text-fg-muted">{s.body}</p>
                    {s.code ? <CodeBlock lang={s.code.lang} content={s.code.content} /> : null}
                    {s.note ? <Note>{s.note}</Note> : null}
                  </div>
                </li>
              ))}
            </ol>
          </Panel>

          {guide.headers && guide.headers.length > 0 ? (
            <Panel title="Required headers">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Header</TableHead>
                    <TableHead>Value</TableHead>
                    <TableHead>Note</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {guide.headers.map((h) => (
                    <TableRow key={h.key}>
                      <TableCell className="font-mono text-xs">{h.key}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{h.valueHint}</TableCell>
                      <TableCell className="whitespace-normal text-muted-foreground">{h.note ?? "—"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Panel>
          ) : null}

          <Panel
            title="What Tamga sends"
            aside={<CopyButton value={guide.payloadPreview.content} label="example payload" size="xs">Copy</CopyButton>}
          >
            <pre className="overflow-x-auto p-4 font-mono text-xs leading-5" tabIndex={0} translate="no">
              {guide.payloadPreview.content}
            </pre>
          </Panel>

          {guide.gotchas.length > 0 ? (
            <Panel title="Things that go wrong">
              <ul className="divide-y">
                {guide.gotchas.map((g) => (
                  <li key={g.title} className="p-4">
                    <h3 className="text-sm font-medium">{g.title}</h3>
                    <p className="mt-1 text-sm leading-relaxed text-fg-muted">{g.body}</p>
                  </li>
                ))}
              </ul>
            </Panel>
          ) : null}
        </div>

        <aside className="space-y-6">
          <Panel title="At a glance">
            <DetailList
              items={[
                { label: "Kind", value: <StatusBadge>{guide.kind}</StatusBadge> },
                { label: "Instructions checked", value: guide.lastVerified, mono: true },
              ]}
            />
            <div className="border-t px-4 py-3">
              <p className="text-xs text-muted-foreground">URL pattern</p>
              <p className="mt-1 font-mono text-xs break-all text-fg-muted" translate="no">
                {guide.urlHint}
              </p>
            </div>
          </Panel>

          {guide.docsLinks.length > 0 ? (
            <Panel title="Vendor documentation">
              <ul className="divide-y text-sm">
                {guide.docsLinks.map((d) => (
                  <li key={d.href}>
                    <a
                      href={d.href}
                      target="_blank"
                      rel="noreferrer"
                      className="flex items-center justify-between gap-3 px-4 py-2.5 hover:bg-accent"
                    >
                      <span>{d.label}</span>
                      <ExternalLink className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
                      <span className="sr-only">(opens in a new tab)</span>
                    </a>
                  </li>
                ))}
              </ul>
            </Panel>
          ) : null}
        </aside>
      </div>
    </div>
  );
}
