"use client";

import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { useTranslation } from "@/lib/i18n";

export default function TrustPage() {
  const { t } = useTranslation();
  return (
    <main className="mx-auto w-full max-w-5xl px-6 py-16 font-sans text-zinc-800 dark:text-zinc-200">
      <header className="mb-12">
        <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
          {t("trust.eyebrow")}
        </p>
        <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
          {t("trust.title")}
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-zinc-600 dark:text-zinc-400">
          {t("trust.lede")}
        </p>
      </header>

      <section className="mb-12 grid gap-4 md:grid-cols-3">
        <StatusCard
          label="SOC 2 Type II"
          status={t("trust.roadmap")}
          detail={t("trust.soc2_detail")}
        />
        <StatusCard
          label="ISO/IEC 27001"
          status={t("trust.roadmap")}
          detail={t("trust.iso_detail")}
        />
        <StatusCard
          label="KVKK"
          status={t("trust.active")}
          detail={t("trust.kvkk_detail")}
        />
      </section>

      <Section title={t("trust.residency")}>
        <ul className="space-y-3 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
          <li>
            <strong className="text-zinc-800 dark:text-zinc-200">TR:</strong>{" "}
            {t("trust.residency_tr")}
          </li>
          <li>
            <strong className="text-zinc-800 dark:text-zinc-200">EU:</strong>{" "}
            {t("trust.residency_eu")}
          </li>
          <li>
            <strong className="text-zinc-800 dark:text-zinc-200">Self-hosted:</strong>{" "}
            {t("trust.residency_self")}
          </li>
        </ul>
      </Section>

      <Section title={t("trust.encryption")}>
        <BulletList
          items={[
            t("trust.enc_tls"),
            t("trust.enc_storage"),
            t("trust.enc_kms"),
            t("trust.enc_ci"),
          ]}
        />
      </Section>

      <Section title={t("trust.data_control")}>
        <BulletList
          items={[
            t("trust.data_erase"),
            t("trust.data_retention"),
            t("trust.data_hash"),
            t("trust.data_dpa"),
          ]}
        />
      </Section>

      <Section title={t("trust.audit_trail")}>
        <p className="text-sm leading-6 text-zinc-600 dark:text-zinc-400">
          {t("trust.audit_text")}
        </p>
      </Section>

      <Section title={t("trust.disclosure")}>
        <p className="text-sm leading-6 text-zinc-600 dark:text-zinc-400">
          {t("trust.disclosure_text")}
        </p>
      </Section>

      <Section title={t("trust.certs")}>
        <div className="grid gap-3 text-sm md:grid-cols-2">
          <DocLink href="/trust/security" label={t("trust.certs_security")} />
          <DocLink href="/trust/kvkk" label={t("trust.certs_kvkk")} />
          <DocLink href="/dpa" label={t("trust.certs_dpa")} />
          <DocLink href="/docs" label={t("trust.certs_api")} />
        </div>
      </Section>

      <footer className="mt-16 border-t border-zinc-200 dark:border-zinc-800 pt-6 text-xs text-zinc-500 dark:text-zinc-400">
        {t("trust.updated")}: 2026-04-17 ·{" "}
        <Link className="text-zinc-700 dark:text-zinc-300 hover:text-white" href="/subprocessors">
          {t("trust.subprocessors")}
        </Link>
      </footer>
    </main>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-10 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40 p-6">
      <h2 className="mb-3 font-mono text-[11px] uppercase tracking-[0.2em] text-zinc-600 dark:text-zinc-400">
        {title}
      </h2>
      {children}
    </section>
  );
}

function StatusCard({
  label,
  status,
  detail,
}: {
  label: string;
  status: string;
  detail: string;
}) {
  const active = status === "AKTİF" || status === "ACTIVE";
  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-4">
      <div className="mb-2 flex items-center justify-between">
        <span className="font-mono text-[11px] uppercase tracking-wide text-zinc-700 dark:text-zinc-300">
          {label}
        </span>
        <span
          className={`rounded-sm border px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider ${
            active
              ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300"
              : "border-amber-500/40 bg-amber-500/10 text-amber-300"
          }`}
        >
          {status}
        </span>
      </div>
      <p className="text-xs text-zinc-500 dark:text-zinc-400">{detail}</p>
    </div>
  );
}

function BulletList({ items }: { items: string[] }) {
  return (
    <ul className="space-y-2 text-sm leading-6 text-zinc-600 dark:text-zinc-400">
      {items.map((item, idx) => (
        <li key={idx} className="flex gap-3">
          <span className="font-mono text-zinc-600 dark:text-zinc-400">{String(idx + 1).padStart(2, "0")}</span>
          <span>{item}</span>
        </li>
      ))}
    </ul>
  );
}

function DocLink({ href, label }: { href: string; label: string }) {
  return (
    <Link
      href={href}
      className="flex items-center justify-between rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/40 px-4 py-3 text-zinc-700 dark:text-zinc-300 transition hover:border-zinc-700 hover:bg-zinc-100 dark:hover:bg-zinc-900"
    >
      <span>{label}</span>
      <ArrowRight className="h-3.5 w-3.5 text-zinc-500 dark:text-zinc-400" aria-hidden />
    </Link>
  );
}
