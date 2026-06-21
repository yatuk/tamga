"use client";

import { useEffect, useRef, useState } from "react";
import { useReducedMotion } from "framer-motion";

// ── Easing ─────────────────────────────────────────────────────────────────────

function easeOutExpo(t: number): number {
  return t === 1 ? 1 : 1 - Math.pow(2, -10 * t);
}

// ── Hook ───────────────────────────────────────────────────────────────────────

function useAnimatedValue(target: number, durationMs = 600, active = true): number {
  const [current, setCurrent] = useState(target);
  const raf = useRef<number | null>(null);
  const startVal = useRef(current);
  const startTime = useRef(0);

  useEffect(() => {
    if (!active) {
      setCurrent(target);
      return;
    }

    startVal.current = current;
    startTime.current = 0;

    const animate = (timestamp: number) => {
      if (!startTime.current) startTime.current = timestamp;
      const elapsed = timestamp - startTime.current;
      const progress = Math.min(1, elapsed / durationMs);
      const eased = easeOutExpo(progress);

      setCurrent(
        startVal.current + (target - startVal.current) * eased,
      );

      if (progress < 1) {
        raf.current = requestAnimationFrame(animate);
      }
    };

    raf.current = requestAnimationFrame(animate);

    return () => {
      if (raf.current !== null) {
        cancelAnimationFrame(raf.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, durationMs, active]);

  return current;
}

// ── Component ──────────────────────────────────────────────────────────────────

interface AnimatedNumberProps {
  value: number;
  duration?: number;
  format?: "int" | "float1" | "float2" | "pct";
  prefix?: string;
  suffix?: string;
  className?: string;
  active?: boolean;
}

export function AnimatedNumber({
  value,
  duration = 600,
  format = "int",
  prefix = "",
  suffix = "",
  className = "",
  active = true,
}: AnimatedNumberProps) {
  const reduce = useReducedMotion();
  const animActive = active && !reduce;
  const current = useAnimatedValue(value, duration, animActive);

  let formatted: string;
  switch (format) {
    case "float1":
      formatted = current.toFixed(1);
      break;
    case "float2":
      formatted = current.toFixed(2);
      break;
    case "pct":
      formatted = `${Math.round(current)}%`;
      break;
    default:
      formatted = Math.round(current).toLocaleString("tr-TR");
  }

  return (
    <span className={`font-mono tabular-nums ${className}`}>
      {prefix}{formatted}{suffix}
    </span>
  );
}

// ── Compact ticker variant — flips individual digit columns ────────────────────

const DIGIT_HEIGHT = 20;

function DigitColumn({
  digit,
  animate,
}: {
  digit: string;
  prevDigit?: string;
  animate: boolean;
}) {
  // For non-numeric chars, just render static
  if (!/^\d$/.test(digit)) {
    return <span className="tabular-nums">{digit}</span>;
  }

  const d = parseInt(digit, 10);

  return (
    <span
      className="relative inline-block overflow-hidden tabular-nums"
      style={{ height: DIGIT_HEIGHT, width: "0.6em", verticalAlign: "top" }}
      aria-hidden
    >
      <span
        className="absolute inset-0 flex flex-col"
        style={{
          transform: `translateY(${animate ? -d * DIGIT_HEIGHT : 0}px)`,
          transition: animate ? "transform 0.45s cubic-bezier(0.22, 0.61, 0.36, 1)" : "none",
        }}
      >
        {Array.from({ length: 10 }, (_, i) => (
          <span
            key={i}
            className="flex items-center justify-center"
            style={{ height: DIGIT_HEIGHT }}
          >
            {i}
          </span>
        ))}
      </span>
    </span>
  );
}

interface TickerNumberProps {
  value: string;
  animate?: boolean;
  className?: string;
}

export function TickerNumber({ value, animate = true, className = "" }: TickerNumberProps) {
  const reduce = useReducedMotion();
  const [prevValue, setPrevValue] = useState(value);

  useEffect(() => {
    if (value !== prevValue) {
      setPrevValue(value);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  const shouldAnimate = animate && !reduce;

  return (
    <span className={`inline-flex items-baseline font-mono tabular-nums ${className}`}>
      {value.split("").map((char, i) => (
        <DigitColumn
          key={`${i}-${char}`}
          digit={char}
          prevDigit={prevValue[i] || char}
          animate={shouldAnimate}
        />
      ))}
    </span>
  );
}
