import { Info } from "lucide-react";

import { cn } from "@/lib/utils";

export type PostureScoreProps = {
  /** 0-100, higher = better */
  score: number | null;
  /** Change vs previous period, in percentage points */
  delta: number | null;
  /** Sparkline data (posture history) */
  series: { t: number; v: number }[];
  /** Shown in the info tooltip */
  methodology?: string;
};

const DEFAULT_METHODOLOGY =
  "Posture Score = 100 − weighted input risk. Input risk (0–100, higher is worse) is inverted; severity weights: critical > high > medium > low.";

type Tone = "pass" | "medium" | "critical";

// NOTE: use the baked-alpha `-bg` tokens (e.g. `from-status-pass-bg`) for the
// status tint. `bg-status-pass/10`-style opacity modifiers are dropped by this
// Tailwind setup because the tokens are bare `var(--...)` strings with no
// `<alpha-value>` placeholder. `--status-*-bg` already carries ~0.1 alpha.
const TONE: Record<Tone, { text: string; tint: string }> = {
  pass: { text: "text-status-pass", tint: "from-status-pass-bg" },
  medium: { text: "text-status-medium", tint: "from-status-medium-bg" },
  critical: { text: "text-status-critical", tint: "from-status-critical-bg" },
};

function toneForScore(score: number): Tone {
  if (score >= 90) return "pass";
  if (score >= 70) return "medium";
  return "critical";
}

function formatDelta(delta: number): string {
  const sign = delta > 0 ? "+" : "";
  return `${sign}${delta.toFixed(1)} pts`;
}

/** Maps a series onto a 100x100 viewBox; returns stroke + area-fill strings. */
function buildSparkline(series: PostureScoreProps["series"]): {
  line: string;
  area: string;
} {
  const W = 100;
  const H = 100;
  const padX = 3;
  const padY = 10;

  const values = series.map((p) => p.v);
  let min = Math.min(...values);
  let max = Math.max(...values);
  if (min === max) {
    const spread = Math.abs(min) * 0.02 || 1;
    min -= spread;
    max += spread;
  } else {
    const pad = (max - min) * 0.15;
    min -= pad;
    max += pad;
  }

  const n = series.length;
  const px = (i: number) =>
    n === 1 ? W / 2 : padX + (i / (n - 1)) * (W - padX * 2);
  const py = (v: number) =>
    H - padY - ((v - min) / (max - min)) * (H - padY * 2);

  const pts = series.map((p, i) => [px(i), py(p.v)] as const);
  const line = pts.map(([x, y]) => `${x.toFixed(2)},${y.toFixed(2)}`).join(" ");
  const area = [
    `M ${pts[0][0].toFixed(2)},${pts[0][1].toFixed(2)}`,
    ...pts
      .slice(1)
      .map(([x, y]) => `L ${x.toFixed(2)},${y.toFixed(2)}`),
    `L ${pts[n - 1][0].toFixed(2)},${H}`,
    `L ${pts[0][0].toFixed(2)},${H}`,
    "Z",
  ].join(" ");

  return { line, area };
}

export default function PostureScore({
  score,
  delta,
  series,
  methodology = DEFAULT_METHODOLOGY,
}: PostureScoreProps) {
  const empty = score === null || !Number.isFinite(score);
  const tone: Tone | null = empty ? null : toneForScore(score);
  const spark = series.length >= 2 ? buildSparkline(series) : null;
  const first = series.length > 0 ? series[0].v : null;
  const last = series.length > 0 ? series[series.length - 1].v : null;

  return (
    <div className="docket-surface relative flex h-full min-h-[280px] flex-col overflow-hidden rounded-sm border border-border text-fg">
      {/* Full-card status tint (neutral when empty) */}
      {tone ? (
        <div
          aria-hidden
          className={cn(
            "pointer-events-none absolute inset-0 bg-gradient-to-b to-transparent",
            TONE[tone].tint,
          )}
        />
      ) : null}

      {/* Header */}
      <div className="relative flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <span className="text-sm font-semibold text-fg">
          Posture score
        </span>
        <span title={methodology} aria-label={methodology}>
          <Info
            size={14}
            className="shrink-0 cursor-help text-fg-subtle hover:text-fg-muted"
          />
        </span>
      </div>

      {/* Body: sparkline behind a centered hero number */}
      <div className="relative flex-1 bg-surface-card/80">
        {spark ? (
          <svg
            viewBox="0 0 100 100"
            preserveAspectRatio="none"
            aria-hidden
            className={cn(
              "absolute inset-0 h-full w-full",
              tone ? TONE[tone].text : "text-fg-subtle",
            )}
          >
            <path
              d={spark.area}
              fill="currentColor"
              fillOpacity={0.12}
              stroke="none"
            />
            <polyline
              points={spark.line}
              fill="none"
              stroke="currentColor"
              strokeOpacity={0.7}
              strokeWidth={1.5}
              strokeLinecap="round"
              strokeLinejoin="round"
              vectorEffect="non-scaling-stroke"
            />
          </svg>
        ) : null}

        {/* Period start / end labels */}
        {first !== null ? (
          <span className="absolute left-3 top-2 font-mono text-[10px] tabular-nums text-fg-subtle">
            {first.toFixed(1)}%
          </span>
        ) : null}
        {last !== null ? (
          <span className="absolute bottom-2 right-3 font-mono text-[10px] tabular-nums text-fg-subtle">
            {last.toFixed(1)}%
          </span>
        ) : null}

        <div className="absolute inset-0 flex items-center justify-center">
          {empty ? (
            <div className="flex flex-col items-center gap-1.5">
              <span className="font-mono text-4xl font-semibold leading-none tabular-nums text-fg-subtle">
                —
              </span>
              <span className="text-xs text-fg-muted">
                Connect admin access to calculate posture
              </span>
            </div>
          ) : (
            <span
              className={cn(
                "font-mono text-4xl font-semibold leading-none tabular-nums sm:text-5xl",
                tone ? TONE[tone].text : "text-fg",
              )}
            >
              {score.toFixed(1)}
            </span>
          )}
        </div>
      </div>

      {/* Footer: delta vs prior period */}
      <div className="relative flex min-h-10 items-center justify-between gap-2 border-t border-border bg-surface-card/90 px-4 py-2 text-[10px]">
        <span className="text-fg-muted">100 − average input risk</span>
        {!empty && typeof delta === "number" ? (
          <span className="inline-flex items-center gap-1.5">
            <span
              className={cn(
                "font-mono tabular-nums",
                delta > 0
                  ? "text-status-pass"
                  : delta < 0
                    ? "text-status-critical"
                    : "text-fg-muted",
              )}
            >
              {formatDelta(delta)}
            </span>
            <span className="text-fg-muted">vs prior</span>
          </span>
        ) : <span className="text-fg-faint">Current snapshot</span>}
      </div>
    </div>
  );
}

export const DEMO_POSTURE: PostureScoreProps = {
  score: 88.5,
  delta: 1.3,
  series: [
    { t: 0, v: 86.2 },
    { t: 1, v: 86.8 },
    { t: 2, v: 86.1 },
    { t: 3, v: 87.0 },
    { t: 4, v: 86.5 },
    { t: 5, v: 87.6 },
    { t: 6, v: 87.1 },
    { t: 7, v: 88.2 },
    { t: 8, v: 87.8 },
    { t: 9, v: 88.9 },
    { t: 10, v: 88.3 },
    { t: 11, v: 89.2 },
    { t: 12, v: 88.7 },
    { t: 13, v: 89.0 },
    { t: 14, v: 88.4 },
    { t: 15, v: 88.9 },
    { t: 16, v: 88.6 },
    { t: 17, v: 88.5 },
  ],
  methodology:
    "Posture Score = 100 − weighted input risk. Input risk (0–100, higher is worse) is inverted; severity weights: critical > high > medium > low.",
};
