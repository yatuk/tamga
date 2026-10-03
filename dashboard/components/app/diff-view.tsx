import { cn } from "@/lib/utils";

export type DiffLine = {
  /** "+" added, "-" removed, " " unchanged. */
  type: "+" | "-" | " ";
  text: string;
};

/** Counts of added and removed lines, for a summary next to the diff. */
export function diffStats(lines: DiffLine[]) {
  let added = 0;
  let removed = 0;
  for (const line of lines) {
    if (line.type === "+") added++;
    else if (line.type === "-") removed++;
  }
  return { added, removed };
}

/**
 * A unified line diff. The +/- marker carries the meaning; color only
 * reinforces it.
 */
export function DiffView({ lines, className }: { lines: DiffLine[]; className?: string }) {
  return (
    <div
      className={cn("max-h-[32rem] overflow-auto font-mono text-xs leading-5", className)}
      role="region"
      aria-label="Line diff"
      tabIndex={0}
    >
      {lines.map((line, i) => (
        <div
          key={i}
          className={cn(
            "flex min-w-max",
            line.type === "+" && "bg-status-pass-bg text-status-pass",
            line.type === "-" && "bg-status-critical-bg text-status-critical",
            line.type === " " && "text-muted-foreground",
          )}
        >
          <span className="w-8 shrink-0 text-center select-none" aria-hidden={line.type === " "}>
            {line.type === " " ? "" : line.type}
          </span>
          <pre className="pr-4 whitespace-pre">{line.text || " "}</pre>
        </div>
      ))}
    </div>
  );
}
