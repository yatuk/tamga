"use client";

import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";
import { animate, AnimatePresence, motion, MotionConfig, useReducedMotion } from "motion/react";

/**
 * Motion in the console says one of three things: a number changed, a thing
 * arrived, or where you are. Nothing moves for decoration, and a page that
 * has just loaded has not changed: it is drawn as it is, with no entrance.
 *
 * The curves are Carbon's: "productive" for the small, frequent moves and
 * "expressive" for the few that should be noticed.
 */
export const PRODUCTIVE = [0.2, 0, 0.38, 0.9] as const;
export const EXPRESSIVE = [0.4, 0.14, 0.3, 1] as const;

/** Honours the operating system's reduced-motion setting for everything inside. */
export function Motion({ children }: { children: ReactNode }) {
  return <MotionConfig reducedMotion="user">{children}</MotionConfig>;
}

/**
 * A number that counts from its old value to a new one. The eye follows it
 * there, which is the point for the one figure a page is about; do not use it
 * on every number. The first value is simply shown.
 */
const whole = (n: number) => Math.round(n).toLocaleString("en");

export function Count({
  value,
  duration = 0.9,
  format = whole,
}: {
  value: number;
  duration?: number;
  /** How to write the number at every step of the count. Whole numbers by default. */
  format?: (n: number) => string;
}) {
  const node = useRef<HTMLSpanElement>(null);
  const shown = useRef(value);
  const reduced = useReducedMotion();

  useEffect(() => {
    const el = node.current;
    if (!el) return;
    if (reduced || shown.current === value) {
      shown.current = value;
      el.textContent = format(value);
      return;
    }
    const controls = animate(shown.current, value, {
      duration,
      ease: EXPRESSIVE,
      onUpdate: (v) => {
        shown.current = v;
        el.textContent = format(v);
      },
    });
    return () => controls.stop();
    // format is a display detail; a new function identity must not restart the count.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, duration, reduced]);

  // The real value is in the markup from the first render: a reader that
  // does not run the effect still gets the number.
  return (
    <span ref={node} aria-label={format(value)}>
      {format(value)}
    </span>
  );
}

/**
 * A bar as long as its share. When the data changes it moves from where it
 * was, so a change in the window reads as a change and not as a new chart.
 */
export function Bar({
  share,
  axis = "x",
  className,
  style,
  ...rest
}: {
  share: number;
  axis?: "x" | "y";
  className?: string;
  style?: CSSProperties;
} & Record<`data-${string}`, string | undefined>) {
  const size = `${Math.max(0, Math.min(1, share)) * 100}%`;
  const prop = axis === "x" ? "width" : "height";
  return (
    <motion.span
      className={className}
      style={style}
      initial={false}
      animate={{ [prop]: size }}
      transition={{ duration: 0.4, ease: EXPRESSIVE }}
      {...rest}
    />
  );
}

/**
 * A short list whose rows may arrive while it is on screen. The rows that are
 * there when the list is first drawn do not move; one that joins later rises
 * into place. Put `Arrive` rows inside it.
 */
export function Arriving({ children }: { children: ReactNode }) {
  return <AnimatePresence initial={false}>{children}</AnimatePresence>;
}

export function Arrive({ children, as = "li", className }: { children: ReactNode; as?: "li" | "div" | "tr"; className?: string }) {
  const Tag = motion[as];
  return (
    <Tag className={className} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.24, ease: PRODUCTIVE }}>
      {children}
    </Tag>
  );
}
