"use client";

import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";
import { animate, motion, MotionConfig, useReducedMotion } from "motion/react";

/**
 * Motion in the console says one of three things: a number changed, a thing
 * arrived, or where you are. Nothing moves for decoration.
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
 * A number that counts to its value, and from its old value to a new one.
 * The eye follows it there, which is the point for the one figure a page is
 * about; do not use it on every number.
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
  const shown = useRef(0);
  const reduced = useReducedMotion();

  useEffect(() => {
    const el = node.current;
    if (!el) return;
    if (reduced) {
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
 * A bar that grows to its share. On first paint it draws from nothing; when
 * the data changes it moves from where it was, so a change in the window
 * reads as a change and not as a new chart.
 */
export function Bar({
  share,
  axis = "x",
  delay = 0,
  className,
  style,
  ...rest
}: {
  share: number;
  axis?: "x" | "y";
  delay?: number;
  className?: string;
  style?: CSSProperties;
} & Record<`data-${string}`, string | undefined>) {
  const size = `${Math.max(0, Math.min(1, share)) * 100}%`;
  const prop = axis === "x" ? "width" : "height";
  return (
    <motion.span
      className={className}
      style={style}
      initial={{ [prop]: 0 }}
      animate={{ [prop]: size }}
      transition={{ duration: 0.55, ease: EXPRESSIVE, delay }}
      {...rest}
    />
  );
}

/** Rows of a list arriving one after another. Use for a handful, not a table. */
export function Arrive({ index = 0, children, as = "li", className }: { index?: number; children: ReactNode; as?: "li" | "div" | "tr"; className?: string }) {
  const Tag = motion[as];
  return (
    <Tag
      className={className}
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.24, ease: PRODUCTIVE, delay: 0.04 * index }}
    >
      {children}
    </Tag>
  );
}
