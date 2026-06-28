import type { Transition, Variants } from "framer-motion";

export const springDefault = { type: "spring" as const, damping: 25, stiffness: 250 };

export const viewportOnce = { once: true as const, margin: "-50px" as const, amount: 0.2 };

export function fadeUpTransition(reduced: boolean): Transition {
  if (reduced) return { duration: 0.15, ease: "easeOut" };
  return springDefault;
}

export function fadeOnlyTransition(reduced: boolean): Transition {
  return { duration: reduced ? 0.12 : 0.2, ease: "easeOut" };
}

export const staggerFast = (reduced: boolean) => (reduced ? 0 : 0.08);
export const staggerCards = (reduced: boolean) => (reduced ? 0 : 0.12);

export function containerStagger(reduced: boolean, gap = 0.08): Variants {
  return {
    hidden: {},
    show: {
      transition: { staggerChildren: reduced ? 0 : gap, delayChildren: reduced ? 0 : 0.02 },
    },
  };
}

export function itemFadeUp(reduced: boolean): Variants {
  return {
    hidden: { opacity: 0, y: reduced ? 0 : 20 },
    show: {
      opacity: 1,
      y: 0,
      transition: fadeUpTransition(reduced),
    },
  };
}
