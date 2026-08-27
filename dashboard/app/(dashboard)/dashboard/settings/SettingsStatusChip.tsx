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
    ? "border-border-strong bg-surface-subtle text-fg-muted"
    : good
      ? "border-status-pass/40 bg-status-pass/10 text-status-pass"
      : "border-status-critical/40 bg-status-critical/10 text-status-critical";
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-sm border px-2 py-1 text-[10px] uppercase tracking-wide ${cls}`}
    >
      {label}
      <span className="text-fg-muted">·</span>
      <span>{value}</span>
    </span>
  );
}
