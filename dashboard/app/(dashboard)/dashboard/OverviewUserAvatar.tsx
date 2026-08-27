"use client";

import { toLowerEn } from "@/lib/utils/tr-string";

export function OverviewUserAvatar() {
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const clerkEnabled = pk && !toLowerEn(pk).includes("placeholder");
  if (!clerkEnabled) {
    return (
      <div className="flex h-8 w-8 items-center justify-center rounded-full border border-border-strong bg-surface-subtle text-xs font-semibold text-fg-muted dark:border-border dark:bg-surface-subtle dark:text-fg">
        T
      </div>
    );
  }
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { UserButton } = require("@clerk/nextjs");
  return <UserButton afterSignOutUrl="/sign-in" />;
}
