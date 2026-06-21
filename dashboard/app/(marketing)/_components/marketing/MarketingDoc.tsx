import type { ReactNode } from "react";

interface MarketingDocProps {
  eyebrow: string;
  title: string;
  lastUpdated?: string;
  lastUpdatedLabel?: string;
  intro?: ReactNode;
  children: ReactNode;
}

export function MarketingDoc({
  eyebrow,
  title,
  lastUpdated,
  lastUpdatedLabel = "Son güncelleme",
  intro,
  children,
}: MarketingDocProps) {
  return (
    <main className="mx-auto w-full max-w-4xl px-6 py-16 font-sans text-zinc-800 dark:text-zinc-200">
      <header className="mb-12 border-b border-zinc-200 dark:border-zinc-800 pb-10">
        <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
          {eyebrow}
        </p>
        <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
          {title}
        </h1>
        {lastUpdated ? (
          <p className="mt-3 font-mono text-[11px] uppercase tracking-[0.18em] text-zinc-500 dark:text-zinc-400">
            {lastUpdatedLabel} {"//"} {lastUpdated}
          </p>
        ) : null}
        {intro ? (
          <div className="mt-4 max-w-2xl text-base leading-7 text-zinc-600 dark:text-zinc-400">
            {intro}
          </div>
        ) : null}
      </header>
      <article className="prose prose-invert max-w-none space-y-6 text-[15px] leading-7 text-zinc-700 dark:text-zinc-300 [&_h2]:mt-12 [&_h2]:border-l-2 [&_h2]:border-red-500/60 [&_h2]:pl-3 [&_h2]:text-xl [&_h2]:font-semibold [&_h2]:text-white [&_h3]:mt-6 [&_h3]:text-base [&_h3]:font-semibold [&_h3]:text-zinc-100 [&_a]:text-red-400 hover:[&_a]:text-red-300 [&_li]:marker:text-zinc-600 dark:text-zinc-400 [&_table]:border-collapse [&_table]:text-sm [&_th]:border [&_th]:border-zinc-800 [&_th]:bg-zinc-900 [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:font-mono [&_th]:text-[11px] [&_th]:uppercase [&_th]:tracking-[0.14em] [&_th]:text-zinc-500 dark:text-zinc-400 [&_td]:border [&_td]:border-zinc-800 [&_td]:px-3 [&_td]:py-2 [&_td]:align-top [&_td]:text-zinc-300">
        {children}
      </article>
    </main>
  );
}
