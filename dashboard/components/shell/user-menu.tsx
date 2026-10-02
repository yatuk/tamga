"use client";

import { toLowerEn } from "@/lib/utils/case";

const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
const clerkEnabled = Boolean(pk) && !toLowerEn(pk).includes("placeholder");

/** The signed-in user's menu. Renders nothing when Clerk is not configured. */
export function UserMenu() {
  if (!clerkEnabled) return null;
  // Loaded lazily so installs without Clerk never pull its client bundle in.
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { UserButton } = require("@clerk/nextjs") as typeof import("@clerk/nextjs");
  return <UserButton afterSignOutUrl="/sign-in" />;
}
