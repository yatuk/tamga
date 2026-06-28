"use client";

type Props = {
  open: boolean;
  onClose: () => void;
};

export function IncidentsShortcutsModal({ open, onClose }: Props) {
  if (!open) return null;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="w-full max-w-md rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 p-4"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="shortcuts-title"
      >
        <h3 id="shortcuts-title" className="mb-2 text-sm text-zinc-900 dark:text-zinc-100">
          Keyboard Shortcuts
        </h3>
        <div className="grid grid-cols-2 gap-2 text-xs">
          {[
            ["j / k", "next / previous row"],
            ["x", "select row"],
            ["Enter", "open details"],
            ["Shift+A", "assign to me"],
            ["Shift+C", "close incident"],
            ["Shift+F", "mark false positive"],
            ["/", "focus search"],
            ["?", "toggle this help"],
            ["Esc", "close panel/help"],
          ].map(([k, v]) => (
            <div key={k} className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 px-2 py-1">
              <span className="text-zinc-800 dark:text-zinc-200">{k}</span>
              <span className="ml-2 text-zinc-600 dark:text-zinc-400">{v}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
