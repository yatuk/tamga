"use client";

import dynamic from "next/dynamic";
import { AuthFrame, NoAccounts } from "@/components/console/auth";

const ClerkSignUp = dynamic(
  async () => {
    try {
      const mod = await import("@clerk/nextjs");
      return { default: mod.SignUp };
    } catch {
      return { default: () => null };
    }
  },
  { ssr: false },
);

export default function SignUpPage() {
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const enabled = pk && !pk.toLowerCase().includes("placeholder");

  if (enabled) {
    return (
      <AuthFrame>
        <h1 className="cds--visually-hidden">Create a Tamga account</h1>
        <ClerkSignUp />
      </AuthFrame>
    );
  }

  return (
    <NoAccounts title="Create an account">
      No identity provider is set up for this deployment, so there are no accounts to create. The console is open and uses the admin key from Settings.
    </NoAccounts>
  );
}
