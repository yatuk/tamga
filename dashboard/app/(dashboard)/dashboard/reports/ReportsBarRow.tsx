export function ReportsBarRow({
  label,
  value,
  total,
  color,
}: {
  label: string;
  value: number;
  total: number;
  color: string;
}) {
  const pct = total > 0 ? Math.round((value / total) * 100) : 0;
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-[11px]">
        <span className="truncate text-zinc-700 dark:text-zinc-300">{label}</span>
        <span className="tabular-nums text-zinc-600 dark:text-zinc-400">
          {value} <span className="text-zinc-600 dark:text-zinc-400">({pct}%)</span>
        </span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-sm bg-zinc-100 dark:bg-zinc-900">
        <div className={`h-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}
