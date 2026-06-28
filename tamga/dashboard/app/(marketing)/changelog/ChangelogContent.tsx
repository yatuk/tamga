"use client";

import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";
import { useTranslation } from "@/lib/i18n";

type Entry = {
  version: string;
  date: string;
  highlights: string[];
};

export function ChangelogContent() {
  const { t } = useTranslation();

  const ENTRIES: Entry[] = [
    {
      version: t("changelog.v1_title"),
      date: "2026-06-12",
      highlights: [
        t("changelog.v1_h1"),
        t("changelog.v1_h2"),
        t("changelog.v1_h3"),
        t("changelog.v1_h4"),
        t("changelog.v1_h5"),
        t("changelog.v1_h6"),
        t("changelog.v1_h7"),
        t("changelog.v1_h8"),
        t("changelog.v1_h9"),
        t("changelog.v1_h10"),
        t("changelog.v1_h11"),
        t("changelog.v1_h12"),
        t("changelog.v1_h13"),
      ],
    },
    {
      version: t("changelog.v0_title"),
      date: "2026-02-14",
      highlights: [
        t("changelog.v0_h1"),
        t("changelog.v0_h2"),
        t("changelog.v0_h3"),
        t("changelog.v0_h4"),
        t("changelog.v0_h5"),
        t("changelog.v0_h6"),
        t("changelog.v0_h7"),
        t("changelog.v0_h8"),
        t("changelog.v0_h9"),
        t("changelog.v0_h10"),
      ],
    },
  ];

  return (
    <>
      <main className="mx-auto max-w-3xl px-6 py-16 sm:py-20">
        <header className="mb-10 border-b border-zinc-200 dark:border-zinc-800 pb-6">
          <h1 className="text-3xl font-semibold tracking-tight">
            {t("changelog.title")}
          </h1>
          <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
            {t("changelog.lede")}
          </p>
        </header>

        <ol className="space-y-8">
          {ENTRIES.map((e) => (
            <li
              key={e.version}
              className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4"
            >
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h2 className="font-mono text-sm text-zinc-900 dark:text-zinc-100">
                  {e.version}
                </h2>
                <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">
                  {e.date}
                </span>
              </div>
              <ul className="mt-3 list-disc space-y-1 pl-5 text-sm text-zinc-700 dark:text-zinc-300">
                {e.highlights.map((h, i) => (
                  <li key={i}>{h}</li>
                ))}
              </ul>
            </li>
          ))}
        </ol>
      </main>
      <MarketingFooter />
    </>
  );
}
