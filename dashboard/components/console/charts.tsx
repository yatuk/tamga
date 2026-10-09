"use client";

import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import { motion, useReducedMotion } from "motion/react";
import { Bar, EXPRESSIVE } from "./motion";

export interface Column {
  key: string;
  /** Under the column, when there is room. */
  label: string;
  /** The column read out in words, shown while it is pointed at. */
  says: string;
  /** A part with no kind is the only series: it is drawn in ink. */
  parts: { kind?: "blocked" | "altered" | "passed"; value: number }[];
}

/** Left and right, Home and End walk the buckets of a chart that has the focus. */
function walk(e: KeyboardEvent, at: number | null, count: number, set: (at: number) => void) {
  const to =
    e.key === "ArrowRight"
      ? Math.min(count - 1, (at ?? -1) + 1)
      : e.key === "ArrowLeft"
        ? Math.max(0, (at ?? count) - 1)
        : e.key === "Home"
          ? 0
          : e.key === "End"
            ? count - 1
            : null;
  if (to === null || count === 0) return;
  e.preventDefault();
  set(to);
}

/**
 * One column per hour or day. Pointing at a column, or walking the chart with
 * the arrow keys, reads it out in the line above; with nothing pointed at,
 * that line says what `idle` says.
 */
export function Columns({ columns, idle, label }: { columns: Column[]; idle: string; label: string }) {
  const [at, setAt] = useState<number | null>(null);
  const top = Math.max(1e-9, ...columns.map((c) => c.parts.reduce((sum, p) => sum + p.value, 0)));
  // Labels are dropped evenly when every column cannot carry one.
  const every = Math.ceil(columns.length / 12);
  const pointed = at !== null ? columns[at] : undefined;

  return (
    <div className="t-chart">
      <p className="t-readout" aria-live="polite">
        {pointed ? pointed.says : idle}
      </p>
      <div
        className="t-days"
        role="img"
        tabIndex={0}
        aria-label={
          columns.length > 0
            ? `${label}, ${columns[0].label} to ${columns[columns.length - 1].label}. The arrow keys read each one.`
            : label
        }
        onPointerLeave={() => setAt(null)}
        onBlur={() => setAt(null)}
        onKeyDown={(e) => walk(e, at, columns.length, setAt)}
      >
        {columns.map((c, i) => (
          <div className="t-day" key={c.key} data-at={at === i} onPointerEnter={() => setAt(i)}>
            {c.parts.map((p, j) => (
              <Bar key={j} axis="y" data-kind={p.kind} share={p.value / top} />
            ))}
          </div>
        ))}
      </div>
      <div className="t-daylabels" aria-hidden="true">
        {columns.map((c, i) => (
          <span key={c.key}>{i % every === 0 ? c.label : ""}</span>
        ))}
      </div>
    </div>
  );
}

export interface TracePoint {
  key: string;
  /** For the axis and the readout: "Tue 6 Oct" or "14:00". */
  name: string;
  /** null where nothing was measured: the line breaks there. */
  value: number | null;
}

/** The next round number at or above v: 1, 2 or 5 times a power of ten. */
function ceiling(v: number) {
  if (v <= 0) return 1;
  const unit = 10 ** Math.floor(Math.log10(v));
  const m = v / unit;
  return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * unit;
}

const HEIGHT = 176;
const PAD = 6;

/**
 * One measure over time, as a line. Pointing at the
 * chart, or walking it with the arrow keys, reads a bucket out in the line
 * above it. One measurement is not a line: it is said as a sentence.
 */
export function Trace({
  points,
  format,
  idle,
  label,
  limit,
}: {
  points: TracePoint[];
  format: (n: number) => string;
  idle: string;
  label: string;
  /** A line the measure should stay under. Drawn only when it is in view. */
  limit?: { value: number; name: string };
}) {
  const frame = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [at, setAt] = useState<number | null>(null);
  const still = useReducedMotion();

  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => setWidth(Math.round(entry.contentRect.width)));
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const measured = points.flatMap((p) => (p.value === null ? [] : [p.value]));
  const top = ceiling(Math.max(0, ...measured));
  const x = (i: number) => (points.length > 1 ? (i / (points.length - 1)) * width : width / 2);
  const y = (v: number) => HEIGHT - (v / top) * (HEIGHT - PAD);

  let d = "";
  const alone: number[] = [];
  points.forEach((p, i) => {
    if (p.value === null) return;
    const starts = i === 0 || points[i - 1].value === null;
    if (starts && (i === points.length - 1 || points[i + 1].value === null)) alone.push(i);
    d += `${starts ? "M" : "L"}${x(i).toFixed(1)} ${y(p.value).toFixed(1)}`;
  });

  const onMove = (e: PointerEvent<HTMLDivElement>) => {
    if (width === 0 || points.length === 0) return;
    const px = e.clientX - e.currentTarget.getBoundingClientRect().left;
    setAt(Math.max(0, Math.min(points.length - 1, Math.round((px / width) * (points.length - 1)))));
  };

  const pointed = at !== null ? points[at] : undefined;
  const limitInView = limit && limit.value <= top;

  if (measured.length === 1) {
    const only = points.find((p) => p.value !== null);
    return (
      <p className="t-readout">
        One measurement so far, at {only?.name}: {format(measured[0])}. A line needs two.
      </p>
    );
  }

  return (
    <div className="t-chart">
      <p className="t-readout" aria-live="polite">
        {pointed ? `${pointed.name}: ${pointed.value === null ? "nothing measured" : format(pointed.value)}` : idle}
      </p>
      <div
        className="t-trace"
        ref={frame}
        role="img"
        tabIndex={0}
        aria-label={
          measured.length > 0
            ? `${label}, ${points[0].name} to ${points[points.length - 1].name}, highest ${format(Math.max(...measured))}. The arrow keys read each one.`
            : label
        }
        onPointerMove={onMove}
        onPointerLeave={() => setAt(null)}
        onBlur={() => setAt(null)}
        onKeyDown={(e) => walk(e, at, points.length, setAt)}
      >
        <span className="t-scale" style={{ top: PAD }}>{format(top)}</span>
        <span className="t-scale" style={{ top: PAD + (HEIGHT - PAD) / 2 }}>{format(top / 2)}</span>
        {width > 0 ? (
          <svg width={width} height={HEIGHT} aria-hidden="true">
            <line className="t-grid" x1={0} x2={width} y1={y(top)} y2={y(top)} />
            <line className="t-grid" x1={0} x2={width} y1={y(top / 2)} y2={y(top / 2)} />
            {limitInView ? <line className="t-limit" x1={0} x2={width} y1={y(limit.value)} y2={y(limit.value)} /> : null}
            {d ? (
              <motion.path
                // A new window is a new line, drawn as it is; the same window refreshed moves in place.
                key={`${points.length}|${points[0]?.key}`}
                className="t-line"
                initial={false}
                animate={{ d }}
                transition={{ duration: still ? 0 : 0.4, ease: EXPRESSIVE }}
              />
            ) : null}
            {alone.map((i) => (
              <circle key={points[i].key} className="t-spot" cx={x(i)} cy={y(points[i].value ?? 0)} r={2.5} />
            ))}
            {pointed ? (
              <>
                <line className="t-cursor" x1={x(at ?? 0)} x2={x(at ?? 0)} y1={0} y2={HEIGHT} />
                {pointed.value !== null ? <circle className="t-spot" cx={x(at ?? 0)} cy={y(pointed.value)} r={3.5} /> : null}
              </>
            ) : null}
          </svg>
        ) : null}
        {limitInView ? (
          <span className="t-scale" data-limit="true" style={{ top: y(limit.value) }}>
            {limit.name}
          </span>
        ) : null}
      </div>
      {points.length > 1 ? (
        <div className="t-tracelabels" aria-hidden="true">
          <span>{points[0].name}</span>
          <span>{points[Math.floor((points.length - 1) / 2)].name}</span>
          <span>{points[points.length - 1].name}</span>
        </div>
      ) : null}
    </div>
  );
}

/**
 * Marks on one scale. For a handful of values of the same measure, where the
 * distance between them is the point; a mark slides when its value changes.
 */
export function Scale({ marks, format, label }: { marks: { name: string; value: number }[]; format: (n: number) => string; label: string }) {
  const top = Math.max(1e-9, ...marks.map((m) => m.value)) * 1.08;
  return (
    <div className="t-scaleline" role="img" aria-label={label}>
      {marks.map((m) => (
        <motion.div
          key={m.name}
          className="t-mark"
          initial={false}
          animate={{ left: `${(m.value / top) * 100}%` }}
          transition={{ duration: 0.4, ease: EXPRESSIVE }}
        >
          <b>{format(m.value)}</b>
          <span>{m.name}</span>
        </motion.div>
      ))}
    </div>
  );
}
