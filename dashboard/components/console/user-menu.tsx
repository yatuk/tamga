"use client";

import dynamic from "next/dynamic";
import { toLowerEn } from "@/lib/utils/case";

const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
const clerkEnabled = Boolean(pk) && !toLowerEn(pk).includes("placeholder");

// A dynamic import keeps Clerk's client code in its own chunk, which is only
// requested when the menu renders. (A require() here was bundled into the
// shell and shipped to every install, with or without Clerk.)
const UserButton = dynamic(() => import("@clerk/nextjs").then((m) => m.UserButton), { ssr: false });

/** The signed-in user's menu. Renders nothing when Clerk is not configured. */
export function UserMenu() {
  if (!clerkEnabled) return null;
  return <UserButton afterSignOutUrl="/sign-in" />;
}
