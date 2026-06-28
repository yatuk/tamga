"use client";

import * as React from "react";

interface RevealProps {
  index?: number;
  delay?: number;
  className?: string;
  as?: "div" | "section" | "article" | "li";
  children: React.ReactNode;
}

export function Reveal({
  index: _index = 0,
  delay: _delay,
  className,
  as = "div",
  children,
}: RevealProps) {
  const Comp = as;
  return <Comp className={className}>{children}</Comp>;
}
