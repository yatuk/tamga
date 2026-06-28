// Marketing route skeleton. Keeps the zinc-950 shell visible so the
// nav doesn't "flash" during route transitions between /docs,
// /compare, /trust etc.
export default function MarketingLoading() {
  return (
    <main className="w-full bg-white dark:bg-zinc-950" aria-busy aria-live="polite">
      <section className="mx-auto max-w-6xl px-4 py-16">
        <div className="space-y-4">
          <div className="h-3 w-40 animate-pulse rounded-sm bg-zinc-200 dark:bg-zinc-800/60" />
          <div className="h-10 w-3/4 animate-pulse rounded-sm bg-zinc-200 dark:bg-zinc-800/80" />
          <div className="h-4 w-2/3 animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/80" />
          <div className="h-4 w-1/2 animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/80" />
        </div>
        <div className="mt-10 grid gap-3 md:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div
              key={i}
              className="h-28 animate-pulse rounded-sm border border-zinc-200 dark:border-zinc-800/80 bg-zinc-100 dark:bg-zinc-900/40"
            />
          ))}
        </div>
      </section>
      <span className="sr-only">Yükleniyor…</span>
    </main>
  );
}
