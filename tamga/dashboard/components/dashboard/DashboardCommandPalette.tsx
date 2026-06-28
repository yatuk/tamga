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
      className="fixed inset-0 z-[70] flex items-start justify-center bg-black/60 p-4 pt-24"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="w-full max-w-2xl overflow-hidden rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
      >
        <div className="border-b border-zinc-200 dark:border-zinc-800 p-2">
          <input
            autoFocus
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Jump to… (inc, pol, play, set)  ·  incident <id>  ·  provider <name>"
            aria-label="Command palette search"
            className="h-9 w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-3 font-mono text-xs text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-600 dark:text-zinc-400"
          />
        </div>
        <div className="max-h-[24rem] overflow-auto p-1">
          {commandsLength === 0 ? (
            <div className="px-2 py-6 text-center text-xs text-zinc-600 dark:text-zinc-400">
              <div className="text-[10px] uppercase tracking-[0.18em] mb-2">No matches</div>
              <div>Try: <span className="text-zinc-900 dark:text-zinc-200">inc</span> · <span className="text-zinc-900 dark:text-zinc-200">pol</span> · <span className="text-zinc-900 dark:text-zinc-200">play</span> · <span className="text-zinc-900 dark:text-zinc-200">set</span> · <span className="text-zinc-900 dark:text-zinc-200">incident &lt;id&gt;</span></div>
            </div>
          ) : (
            grouped.map((g) => (
              <div key={g.label} className="mb-2">
                <div className="px-2 py-1 text-[9px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">{g.label}</div>
                {g.items.slice(0, 12).map((cmd) => (
                  <button
                    key={cmd.id}
                    type="button"
                    onClick={() => run(cmd)}
                    className="group relative flex w-full cursor-pointer items-center justify-between rounded-sm px-2 py-2 text-left text-xs text-zinc-800 dark:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
                  >
                    <span className="pointer-events-none absolute left-0 top-1.5 h-[calc(100%-12px)] w-0.5 scale-y-0 bg-emerald-500 transition-transform duration-150 group-hover:scale-y-100" />
                    <span>{cmd.label}</span>
                    {cmd.hint ? <span className="font-mono text-[10px] text-zinc-600 dark:text-zinc-400">{cmd.hint}</span> : null}
                  </button>
                ))}
              </div>
            ))
          )}
        </div>
        {!query && commandsLength > 0 && (
          <div className="border-t border-zinc-200 dark:border-zinc-800 px-3 py-1.5 text-[10px] text-zinc-600 dark:text-zinc-400 flex gap-3">
            <span>↑↓ navigate</span>
            <span>↵ select</span>
            <span>esc close</span>
          </div>
        )}
      </div>
    </div>
  );
}
