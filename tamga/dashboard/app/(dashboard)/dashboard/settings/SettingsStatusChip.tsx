export function SettingsStatusChip({
  label,
  value,
  good,
  neutral,
}: {
  label: string;
  value: string;
  good?: boolean;
  neutral?: boolean;
}) {
  const cls = neutral
    ? "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400"
    : good
      ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300"
      : "border-red-500/40 bg-red-500/10 text-red-400";
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-sm border px-2 py-1 text-[10px] uppercase tracking-wide ${cls}`}
    >
      {label}
      <span className="text-zinc-600 dark:text-zinc-400">·</span>
      <span>{value}</span>
    </span>
  );
}
