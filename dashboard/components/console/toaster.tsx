"use client";

import { Toaster as Sonner } from "sonner";

/** Where saves, failures and copies are reported. Styled in tamga.css as the console's own. */
export function Toaster() {
  return <Sonner position="bottom-right" theme="light" className="t-toasts" />;
}
