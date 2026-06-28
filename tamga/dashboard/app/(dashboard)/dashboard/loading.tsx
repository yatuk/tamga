// Dashboard route skeleton. Served during segment transitions so the
// user sees instant feedback while the next route's JS chunk streams.
// Mirrors the mission-control aesthetic (mono eyebrow + dense grid).
export default function DashboardLoading() {
  return (
    <div className="space-y-2" aria-busy aria-live="polite">
      <div className="flex flex-col gap-2">
        <div className="h-3 w-48 animate-pulse rounded-sm bg-zinc-200 dark:bg-zinc-800/60" />
        <div className="h-7 w-72 animate-pulse rounded-sm bg-zinc-200 dark:bg-zinc-800/80" />
        <div className="h-3 w-64 animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/80" />
      </div>
      <div className="grid gap-3 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-24 animate-pulse rounded-sm border border-zinc-200 dark:border-zinc-800/80 bg-zinc-100 dark:bg-zinc-900/40"
          />
        ))}
      </div>
      <div className="h-72 animate-pulse rounded-sm border border-zinc-200 dark:border-zinc-800/80 bg-zinc-100 dark:bg-zinc-900/40" />
      <span className="sr-only">Yükleniyor…</span>
    </div>
  );
}
