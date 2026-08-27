"use client";

import Link from "next/link";
import { useState } from "react";
import {
  ArrowLeft,
  BadgeCheck,
  Check,
  ChevronRight,
  Copy,
  ExternalLink,
  Plug,
  TriangleAlert,
} from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toast } from "@/lib/toast";
import { useTranslation } from "@/lib/i18n";
import { toUpperLocale } from "@/lib/utils/tr-string";
import type { IntegrationGuide } from "../_data/guides";

function CopyButton({ text }: { text: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setCopied(true);
          toast.success(t("guide.copied"));
          setTimeout(() => setCopied(false), 1400);
        } catch {
          toast.error("Copy failed");
        }
      }}
      className="inline-flex h-6 items-center gap-1 rounded-sm border border-border bg-surface-subtle px-2 text-[10px] uppercase tracking-wide text-fg-muted hover:bg-surface-card"
    >
      {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
      {copied ? t("guide.copied") : t("guide.copy")}
    </button>
  );
}

function CodeBlock({ lang, content }: { lang: string; content: string }) {
  return (
    <div className="mt-3 rounded-sm border border-border bg-surface-card">
      <div className="flex items-center justify-between border-b border-border px-3 py-1.5">
        <span className="text-[10px] uppercase tracking-[0.18em] text-fg-muted">{lang}</span>
        <CopyButton text={content} />
      </div>
      <pre className="overflow-x-auto px-3 py-2 text-[11px] leading-5 text-fg whitespace-pre-wrap break-words">
        {content}
      </pre>
    </div>
  );
}

export function GuideView({ guide }: { guide: IntegrationGuide }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
        <Link href="/dashboard/integrations" className="inline-flex items-center gap-1 hover:text-fg-subtle">
          <ArrowLeft className="h-3 w-3" /> {t("guide.back")}
        </Link>
        <ChevronRight className="h-3 w-3 text-fg-muted" />
        <span className="text-fg-muted">{guide.name}</span>
      </div>

      <PageHeader
        eyebrow={`ADMINISTRATION // INTEGRATIONS // ${toUpperLocale(guide.kind)}`}
        title={`${guide.name} setup guide`}
        subtitle={guide.overview}
        actions={
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center gap-1 rounded-sm border border-status-pass/60 bg-status-pass/30 px-2 py-1 text-[10px] uppercase tracking-wide text-status-pass">
              <BadgeCheck className="h-3 w-3" /> verified {guide.lastVerified}
            </span>
            <Link href={`/dashboard/integrations?connect=${guide.kind}`} className="inline-flex">
              <Button className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical">
                <Plug className="mr-1 h-3.5 w-3.5" /> {t("guide.connect_cta")}
              </Button>
            </Link>
          </div>
        }
      />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-4">
          <div>
            <section className="rounded-sm border border-border bg-surface-card p-4">
              <div className="mb-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                Overview
              </div>
              <p className="text-sm leading-6 text-fg-muted">{guide.overview}</p>
              {guide.docsLinks.length > 0 ? (
                <div className="mt-3 flex flex-wrap gap-2">
                  {guide.docsLinks.map((d) => (
                    <a
                      key={d.href}
                      href={d.href}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 rounded-sm border border-border bg-surface-subtle px-2 py-1 text-[11px] text-fg-muted hover:bg-surface-card"
                    >
                      {d.label} <ExternalLink className="h-3 w-3" />
                    </a>
                  ))}
                </div>
              ) : null}
            </section>
          </div>

          {guide.prerequisites.length > 0 ? (
            <div>
              <section className="rounded-sm border border-border bg-surface-card p-4">
                <div className="mb-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                  {t("guide.prereq")}
                </div>
                <ul className="space-y-1.5 text-sm text-fg-muted">
                  {guide.prerequisites.map((p) => (
                    <li key={p} className="flex items-start gap-2">
                      <Check className="mt-0.5 h-3.5 w-3.5 flex-none text-status-pass" />
                      <span>{p}</span>
                    </li>
                  ))}
                </ul>
              </section>
            </div>
          ) : null}

          <div>
            <section className="space-y-3">
              <div className="text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                {t("guide.steps")}
              </div>
              <ol className="space-y-3">
                {guide.steps.map((s, i) => (
                  <li
                    key={`${i}-${s.title}`}
                    className="rounded-sm border border-border bg-surface-card p-4"
                  >
                    <div className="flex items-start gap-3">
                      <div className="mt-0.5 inline-flex h-6 w-6 flex-none items-center justify-center rounded-sm border border-border-strong bg-surface-subtle text-[11px] text-fg-muted">
                        {String(i + 1).padStart(2, "0")}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium text-fg">{s.title}</div>
                        <p className="mt-1 text-sm leading-6 text-fg-muted">{s.body}</p>
                        {s.code ? <CodeBlock lang={s.code.lang} content={s.code.content} /> : null}
                        {s.note ? (
                          <div className="mt-3 flex items-start gap-2 rounded-sm border border-status-medium/50 bg-status-medium/20 p-2 text-[12px] text-status-medium">
                            <TriangleAlert className="mt-0.5 h-3.5 w-3.5 flex-none" />
                            <span>{s.note}</span>
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </li>
                ))}
              </ol>
            </section>
          </div>

          {guide.headers && guide.headers.length > 0 ? (
            <div>
              <section className="rounded-sm border border-border bg-surface-card p-4">
                <div className="mb-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                  {t("guide.headers")}
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full text-left text-xs">
                    <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
                      <tr>
                        <th className="px-3 py-1.5">Key</th>
                        <th className="px-3 py-1.5">Value hint</th>
                        <th className="px-3 py-1.5">Note</th>
                      </tr>
                    </thead>
                    <tbody>
                      {guide.headers.map((h) => (
                        <tr key={h.key} className="border-t border-border">
                          <td className="px-3 py-1.5 text-fg">{h.key}</td>
                          <td className="px-3 py-1.5 text-fg-muted">{h.valueHint}</td>
                          <td className="px-3 py-1.5 text-fg-muted">{h.note ?? "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </div>
          ) : null}

          <div>
            <TerminalFrame
              filename={`payload.${guide.payloadPreview.lang}`}
              status={
                <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                  preview
                </span>
              }

            >
              <pre className="overflow-x-auto px-3 py-3 text-[11px] leading-5 text-fg whitespace-pre-wrap break-words">
                {guide.payloadPreview.content}
              </pre>
            </TerminalFrame>
          </div>

          <div>
            <section className="rounded-sm border border-border bg-surface-card p-4">
              <div className="mb-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                {t("guide.gotchas")}
              </div>
              <ul className="space-y-3">
                {guide.gotchas.map((g) => (
                  <li key={g.title} className="rounded-sm border border-border bg-black/40 p-3">
                    <div className="flex items-start gap-2">
                      <TriangleAlert className="mt-0.5 h-3.5 w-3.5 flex-none text-status-medium" />
                      <div>
                        <div className="text-sm font-medium text-fg">{g.title}</div>
                        <p className="mt-1 text-sm leading-6 text-fg-muted">{g.body}</p>
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          </div>
        </div>

        <aside className="space-y-4">
          <div>
            <section className="rounded-sm border border-border bg-surface-card p-4">
              <div className="mb-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                Summary
              </div>
              <div className="space-y-2 text-xs">
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-fg-muted">kind</span>
                  <div>
                    <Badge className={`rounded-sm border text-[10px] uppercase ${guide.badge}`}>
                      {guide.kind}
                    </Badge>
                  </div>
                </div>
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-fg-muted">url pattern</span>
                  <div className="mt-1 break-all rounded-sm border border-border bg-black/40 px-2 py-1 text-[11px] text-fg-muted">
                    {guide.urlHint}
                  </div>
                </div>
                <div>
                  <span className="text-[10px] uppercase tracking-wide text-fg-muted">last verified</span>
                  <div className="text-[11px] text-fg-muted">{guide.lastVerified}</div>
                </div>
              </div>
            </section>
          </div>

          <div>
            <Link
              href={`/dashboard/integrations?connect=${guide.kind}`}
              className="inline-flex w-full cursor-pointer items-center justify-center gap-2 rounded-sm bg-status-critical px-3 py-2 text-xs uppercase tracking-wide text-white hover:bg-status-critical"
            >
              <Plug className="h-3.5 w-3.5" /> {t("guide.connect_cta")}
            </Link>
          </div>
        </aside>
      </div>
    </div>
  );
}
