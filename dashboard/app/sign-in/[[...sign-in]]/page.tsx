"use client";

import dynamic from "next/dynamic";
import { AuthFrame, NoAccounts } from "@/components/console/auth";

const ClerkSignIn = dynamic(
  async () => {
    try {
      const mod = await import("@clerk/nextjs");
      return { default: mod.SignIn };
    } catch {
      return { default: () => null };
    }
  },
  { ssr: false },
);

export default function SignInPage() {
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const enabled = pk && !pk.toLowerCase().includes("placeholder");

  if (enabled) {
    return (
      <AuthFrame>
        <h1 className="cds--visually-hidden">Sign in to Tamga</h1>
        <ClerkSignIn />
      </AuthFrame>
    );
  }

  return (
    <NoAccounts title="Sign in">
      No identity provider is set up for this deployment. The console is open and uses the admin key from Settings.
    </NoAccounts>
  );
}
