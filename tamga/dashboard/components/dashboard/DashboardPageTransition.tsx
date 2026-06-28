"use client";

import { motion, useReducedMotion } from "framer-motion";
import { usePathname } from "next/navigation";

type TransitionVariant = "fade" | "slide-up" | "slide-left" | "none";

interface DashboardPageTransitionProps {
  children: React.ReactNode;
  variant?: TransitionVariant;
}

export function DashboardPageTransition({
  children,
  variant = "fade",
}: DashboardPageTransitionProps) {
  const pathname = usePathname();
  const reduce = useReducedMotion();

  if (reduce || variant === "none") {
    return <>{children}</>;
  }

  if (variant === "slide-up") {
    return (
      <motion.div
        key={pathname}
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.18, ease: [0.22, 0.61, 0.36, 1] }}
      >
        {children}
      </motion.div>
    );
  }

  if (variant === "slide-left") {
    return (
      <motion.div
        key={pathname}
        initial={{ opacity: 0, x: -12 }}
        animate={{ opacity: 1, x: 0 }}
        transition={{ duration: 0.18, ease: [0.22, 0.61, 0.36, 1] }}
      >
        {children}
      </motion.div>
    );
  }

  // Default: fade
  return (
    <motion.div
      key={pathname}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.12, ease: "easeOut" }}
    >
      {children}
    </motion.div>
  );
}
