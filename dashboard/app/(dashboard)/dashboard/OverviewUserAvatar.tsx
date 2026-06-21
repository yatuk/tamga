"use client";

import { toLowerEn } from "@/lib/utils/tr-string";

export function OverviewUserAvatar() {
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const clerkEnabled = pk && !toLowerEn(pk).includes("placeholder");
  if (!clerkEnabled) {
    return (
      <div className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-300 bg-slate-100 text-xs font-semibold text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
        T
      </div>
    );
  }
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { UserButton } = require("@clerk/nextjs");
  return <UserButton afterSignOutUrl="/sign-in" />;
}
