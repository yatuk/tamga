"use client";

import { useLayoutEffect, useState, type KeyboardEvent, type PointerEvent } from "react";
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

/** The clear space kept between two labels on an axis, in pixels. */
export const LABEL_GAP = 8;
/** The space between two columns, in pixels: the gap of `.t-chart .t-days`. */
const COLUMN_GAP = 2;
/** The most labels written under columns: a dozen, and the last. */
const MOST_COLUMN_LABELS = 13;

/** Where a label is drawn on its axis: its left and its right edge, in pixels. */
export type Span = [left: number, right: number];

/**
 * How wide a label is drawn, in em, give or take a pixel that LABEL_GAP
 * covers. The text is not measured, so the answer is the same on the server
 * and before the font has loaded: a letter or a figure is taken as 0.62em,
 * which is a tabular figure of the text face and more than most letters, and
 * a space or a point as 0.3em.
 */
export function labelWidth(text: string) {
  let em = 0;
  for (const ch of text) em += " .,:;".includes(ch) ? 0.3 : 0.62;
  return em;
}

/**
 * Which of `count` labels on an axis are written, as their indices in order.
 * `span` says where label i would be drawn. The first and the last always
 * are. Between them every n-th is, counted from the first, for the smallest n
 * at which no more than `most` are written and no two come within LABEL_GAP
 * of each other; one that would stand nearer to the last than a step is left
 * out, so the labels are evenly spaced up to the last. An axis of no width
 * keeps the two ends.
 */
export function keptLabels(count: number, span: (i: number) => Span, most: number): number[] {
  if (count <= 0) return [];
  if (count === 1) return [0];
  const last = count - 1;
  const clear = (a: number, b: number) => span(a)[1] + LABEL_GAP <= span(b)[0];
  for (let step = Math.max(1, Math.ceil(last / Math.max(1, most - 1))); step < last; step++) {
    const picked: number[] = [];
    for (let i = 0; i < last; i += step) picked.push(i);
    if (last - picked[picked.length - 1] < step) picked.pop();
    picked.push(last);
    if (picked.every((p, k) => k === 0 || clear(picked[k - 1], p))) return picked;
  }
  return [0, last];
}

/** Whether the last label under columns ends with the axis: it does when its column is narrower than it is. */
const endsWithAxis = (widths: number[], room: number) =>
  widths.length > 0 && widths[widths.length - 1] > (room + COLUMN_GAP) / widths.length - COLUMN_GAP;

/**
 * Labels under columns `room` wide in all: each starts where its column
 * does, and the last ends with the axis when it would otherwise run past it.
 */
export function underColumns(widths: number[], room: number): (i: number) => Span {
  const pitch = (room + COLUMN_GAP) / Math.max(1, widths.length);
  const end = endsWithAxis(widths, room);
  return (i) => (end && i === widths.length - 1 ? [room - widths[i], room] : [i * pitch, i * pitch + widths[i]]);
}

/**
 * Labels under the points of a line `room` wide: the first starts with the
 * axis, the last ends with it, and one between them is centred on its point.
 */
export function underPoints(widths: number[], room: number): (i: number) => Span {
  const last = widths.length - 1;
  return (i) => {
    if (i === 0) return [0, widths[0]];
    if (i === last) return [room - widths[i], room];
    const x = (i / last) * room;
    return [x - widths[i] / 2, x + widths[i] / 2];
  };
}

/**
 * The labels written under a line: the two ends of its window, and the point
 * in the middle of it where there is room for a third.
 */
export function lineLabels(widths: number[], room: number): number[] {
  const last = widths.length - 1;
  const ends = last < 0 ? [] : [...new Set([0, Math.floor(last / 2), last])];
  const span = underPoints(widths, room);
  return keptLabels(ends.length, (i) => span(ends[i]), ends.length).map((i) => ends[i]);
}

/**
 * The width of a chart and the size of its axis labels, in pixels, with the
 * ref to put on the element that is as wide as the axis. Both are read before
 * the first paint, so the labels a chart is first seen with are the ones that
 * fit, and again whenever the element changes size. The width is 0 until it
 * is known: on the server, and where nothing is laid out.
 */
function useAxis() {
  const [el, ref] = useState<HTMLDivElement | null>(null);
  const [axis, setAxis] = useState({ width: 0, em: 12 });

  useLayoutEffect(() => {
    if (!el) return;
    const read = (measured: number) => {
      const width = Math.round(measured);
      // An axis label is the label step, 0.75rem.
      const em = 0.75 * (parseFloat(getComputedStyle(document.documentElement).fontSize) || 16);
      setAxis((was) => (was.width === width && was.em === em ? was : { width, em }));
    };
    read(el.getBoundingClientRect().width);
    const observer = new ResizeObserver(([entry]) => read(entry.contentRect.width));
    observer.observe(el);
    return () => observer.disconnect();
  }, [el]);

  return [ref, axis.width, axis.em] as const;
}

/**
 * One column per hour or day. Pointing at a column, or walking the chart with
 * the arrow keys, reads it out in the line above; with nothing pointed at,
 * that line says what `idle` says.
 */
export function Columns({ columns, idle, label }: { columns: Column[]; idle: string; label: string }) {
  const [at, setAt] = useState<number | null>(null);
  const [axis, width, em] = useAxis();
  const top = Math.max(1e-9, ...columns.map((c) => c.parts.reduce((sum, p) => sum + p.value, 0)));
  // Labels are dropped evenly where the axis is too narrow for every column to carry one.
  const widths = columns.map((c) => labelWidth(c.label) * em);
  const written = new Set(keptLabels(columns.length, underColumns(widths, width), MOST_COLUMN_LABELS));
  const lastEnds = endsWithAxis(widths, width);
  const pointed = at !== null ? columns[at] : undefined;

  return (
    <div className="t-chart" data-chart="columns">
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
      <div className="t-daylabels" aria-hidden="true" ref={axis}>
        {columns.map((c, i) =>
          written.has(i) ? (
            <span key={c.key} data-label="true" data-end={lastEnds && i === columns.length - 1 ? "true" : undefined}>
              {c.label}
            </span>
          ) : (
            <span key={c.key} />
          ),
        )}
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
  const [frame, width, em] = useAxis();
  const [at, setAt] = useState<number | null>(null);
  const still = useReducedMotion();

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
  // The two ends of the window, and the middle of it where there is room for a third label.
  const widths = points.map((p) => labelWidth(p.name) * em);
  const written = lineLabels(widths, width);

  if (measured.length === 1) {
    const only = points.find((p) => p.value !== null);
    return (
      <p className="t-readout">
        One measurement so far, at {only?.name}: {format(measured[0])}. A line needs two.
      </p>
    );
  }

  return (
    <div className="t-chart" data-chart="trace">
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
        <span className="t-scale" data-label="true" style={{ top: PAD }}>{format(top)}</span>
        <span className="t-scale" data-label="true" style={{ top: PAD + (HEIGHT - PAD) / 2 }}>{format(top / 2)}</span>
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
          <span className="t-scale" data-label="true" data-limit="true" style={{ top: y(limit.value) }}>
            {limit.name}
          </span>
        ) : null}
      </div>
      {points.length > 1 ? (
        <div className="t-tracelabels" aria-hidden="true">
          {written.map((i) => (
            // The first and the last are set against the ends of the axis by the stylesheet; one between them stands under its point.
            <span key={points[i].key} data-label="true" style={i > 0 && i < points.length - 1 ? { left: `${(i / (points.length - 1)) * 100}%` } : undefined}>
              {points[i].name}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
}

/**
 * Marks on one scale. For a handful of values of the same measure, where the
 * distance between them is the point; a mark slides when its value changes.
 * With reduced motion it is simply at its value.
 */
export function Scale({ marks, format, label }: { marks: { name: string; value: number }[]; format: (n: number) => string; label: string }) {
  const top = Math.max(1e-9, ...marks.map((m) => m.value)) * 1.08;
  const still = useReducedMotion();
  return (
    <div className="t-scaleline" data-chart="scale" role="img" aria-label={label}>
      {marks.map((m) => {
        const left = `${(m.value / top) * 100}%`;
        // Past the middle the words go to the left of the tick: on the right they would run off the line.
        const side = m.value / top > 0.5 ? "left" : "right";
        const words = (
          <>
            <b>{format(m.value)}</b>
            <span>{m.name}</span>
          </>
        );
        return still ? (
          <div key={m.name} className="t-mark" data-label="true" data-side={side} style={{ left }}>
            {words}
          </div>
        ) : (
          <motion.div key={m.name} className="t-mark" data-label="true" data-side={side} initial={false} animate={{ left }} transition={{ duration: 0.4, ease: EXPRESSIVE }}>
            {words}
          </motion.div>
        );
      })}
    </div>
  );
}
