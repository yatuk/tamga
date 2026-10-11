"use client";

import { ActionableNotification, InlineNotification } from "@carbon/react";
import { useAdminKeySource } from "@/hooks/useAdminKey";
import type { AdminKeySource } from "@/lib/built-in-key";
import { toast } from "@/lib/toast";

export const REFUSED_KEY_TITLE = "The proxy did not accept the admin key.";
export const FORGET_STORED_KEY = "Forget the key stored in this browser";

/**
 * What to say after "the proxy did not accept the admin key": which key it
 * was, and what the person can do. `forget` is true when the key is the one
 * stored in this browser, which the notice can then offer to drop.
 *
 * A key saved under Settings wins over the one built into a development
 * build, so a stale stored key is refused on every page while a working key
 * sits unused. In a production build there is no built-in key to fall back on.
 */
export function refusedKeyWords(source: AdminKeySource, builtIn: boolean): { detail: string; forget: boolean } {
  if (source === "stored") {
    return {
      detail: builtIn
        ? "The key it refused is the one stored in this browser. Forgetting it makes this console use the key built into this page for development."
        : "The key it refused is the one stored in this browser. Forget it, then enter a valid one under Settings.",
      forget: true,
    };
  }
  if (source === "built-in") {
    return {
      detail:
        "The key it refused is the one built into this page for development, from NEXT_PUBLIC_ADMIN_KEY. Enter a valid key under Settings, or correct the variable and restart the development server.",
      forget: false,
    };
  }
  return { detail: "Check the key under Settings.", forget: false };
}

/** Which key this console sends, in words. The key itself is never shown. */
export const KEY_IN_USE: Record<AdminKeySource, string> = {
  stored: "In use: the one stored in this browser",
  "built-in": "In use: the one built into this page for development",
  none: "None in use",
};

/** What forgetting the stored key leads to, said before the button is pressed. */
export function storedKeyNote(builtIn: boolean): string {
  return builtIn
    ? "The key stored in this browser is the one this console sends. It takes the place of the key built into this page for development; forgetting it makes the console use that one again."
    : "The key stored in this browser is the one this console sends. Forgetting it leaves the console without a key until one is entered here.";
}

/** What a toast says once the stored key is gone: which key the console uses now. */
export function forgottenWords(builtIn: boolean): string {
  return builtIn ? "This console now uses the key built into this page for development." : "This console has no admin key now. Enter one under Settings.";
}

/**
 * The notice for a proxy that is up and refused the admin key. `lead` is the
 * sentence the place it stands in opens with ("It is up."). When the refused
 * key is the one stored in this browser the notice carries the button that
 * forgets it. No part of a key is shown.
 */
export function RefusedKeyNotice({ lead }: { lead: string }) {
  const { source, builtIn, forget } = useAdminKeySource();
  const words = refusedKeyWords(source, builtIn);
  const style = { marginTop: "1.5rem", maxWidth: "none" };
  const subtitle = `${lead} ${words.detail}`;
  if (!words.forget) {
    return <InlineNotification kind="error" lowContrast hideCloseButton title={REFUSED_KEY_TITLE} subtitle={subtitle} style={style} />;
  }
  return (
    <ActionableNotification
      kind="error"
      lowContrast
      inline
      hideCloseButton
      // A state of the page, not a dialog: it does not take the focus when it appears.
      role="status"
      title={REFUSED_KEY_TITLE}
      subtitle={subtitle}
      actionButtonLabel={FORGET_STORED_KEY}
      onActionButtonClick={() => {
        forget();
        toast.success("Stored key forgotten", forgottenWords(builtIn));
      }}
      style={style}
    />
  );
}
