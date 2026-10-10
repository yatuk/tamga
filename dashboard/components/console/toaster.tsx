"use client";

import { Toaster as Sonner } from "sonner";

/**
 * No glyph before the text: an icon sits only inside a button. The library
 * leaves the icon out, and the space it would take, for a kind whose icon is
 * null. A toast says what it is in its words ("Could not save the hunt").
 */
export const NO_ICONS = { success: null, error: null, info: null, warning: null, loading: null };

/** Where saves, failures and copies are reported. Styled in tamga.css as the console's own. */
export function Toaster() {
  return <Sonner position="bottom-right" theme="light" className="t-toasts" icons={NO_ICONS} />;
}
