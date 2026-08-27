"use client";

const RECENCY_KEY = "tamga-cmd-recent";
const MAX_RECENT = 10;

function recordRecency(id: string) {
  if (typeof window === "undefined") return;
  try {
    const raw = localStorage.getItem(RECENCY_KEY);
    const ids: string[] = raw ? (JSON.parse(raw) as string[]) : [];
    const next = [id, ...ids.filter((x) => x !== id)].slice(0, MAX_RECENT);
    localStorage.setItem(RECENCY_KEY, JSON.stringify(next));
  } catch { /* ignore */ }
}

type Cmd = { id: string; label: string; hint: string; run: () => void };

type Group = { label: string; items: Cmd[] };

type Props = {
  open: boolean;
  onClose: () => void;
  query: string;
  onQueryChange: (q: string) => void;
  grouped: Group[];
  commandsLength: number;
};

export function DashboardCommandPalette({ open, onClose, query, onQueryChange, grouped, commandsLength }: Props) {
  if (!open) return null;

  function run(cmd: Cmd) {
    recordRecency(cmd.id);
    cmd.run();
    onClose();
  }

  return (
    <div
      className="fixed inset-0 z-[70] flex items-start justify-center bg-surface-overlay p-4 pt-24"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="w-full max-w-2xl overflow-hidden rounded-sm border border-border-strong bg-surface-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
      >
        <div className="border-b border-border p-2">
          <input
            autoFocus
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Jump to… (inc, pol, play, set)  ·  incident <id>  ·  provider <name>"
            aria-label="Command palette search"
            className="h-9 w-full rounded-sm border border-border-strong bg-surface-subtle px-3 font-mono text-xs text-fg placeholder:text-fg-muted"
          />
        </div>
        <div className="max-h-[24rem] overflow-auto p-1">
          {commandsLength === 0 ? (
            <div className="px-2 py-6 text-center text-xs text-fg-muted">
              <div className="text-[10px] uppercase tracking-[0.18em] mb-2">No matches</div>
              <div>Try: <span className="text-fg">inc</span> · <span className="text-fg">pol</span> · <span className="text-fg">play</span> · <span className="text-fg">set</span> · <span className="text-fg">incident &lt;id&gt;</span></div>
            </div>
          ) : (
            grouped.map((g) => (
              <div key={g.label} className="mb-2">
                <div className="px-2 py-1 text-[9px] uppercase tracking-[0.18em] text-fg-muted">{g.label}</div>
                {g.items.slice(0, 12).map((cmd) => (
                  <button
                    key={cmd.id}
                    type="button"
                    onClick={() => run(cmd)}
                    className="group relative flex w-full cursor-pointer items-center justify-between rounded-sm px-2 py-2 text-left text-xs text-fg hover:bg-surface-subtle"
                  >
                    <span className="pointer-events-none absolute left-0 top-1.5 h-[calc(100%-12px)] w-0.5 scale-y-0 bg-status-pass transition-transform duration-150 group-hover:scale-y-100" />
                    <span>{cmd.label}</span>
                    {cmd.hint ? <span className="font-mono text-[10px] text-fg-muted">{cmd.hint}</span> : null}
                  </button>
                ))}
              </div>
            ))
          )}
        </div>
        {!query && commandsLength > 0 && (
          <div className="border-t border-border px-3 py-1.5 text-[10px] text-fg-muted flex gap-3">
            <span>↑↓ navigate</span>
            <span>↵ select</span>
            <span>esc close</span>
          </div>
        )}
      </div>
    </div>
  );
}
