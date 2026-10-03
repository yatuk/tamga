"use client";

import Link from "next/link";
import dynamic from "next/dynamic";
import { AuthCard } from "@/components/app/auth-card";
import { Button } from "@/components/ui/button";
import { toLowerEn } from "@/lib/utils/case";

const ClerkSignUp = dynamic(
  async () => {
    try {
      const mod = await import("@clerk/nextjs");
      return { default: mod.SignUp };
    } catch {
      return { default: () => null };
    }
  },
  { ssr: false }
);

export default function SignUpPage() {
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const enabled = pk && !toLowerEn(pk).includes("placeholder");

  if (enabled) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-background px-4 py-10">
        <h1 className="sr-only">Create a Tamga account</h1>
        <ClerkSignUp />
      </main>
    );
  }

  return (
    <AuthCard
      title="Create an account"
      description="No identity provider is configured for this deployment, so there are no accounts to create. The console is open and uses the admin key from Settings."
    >
      <Button asChild className="w-full">
        <Link href="/dashboard">Open the Console</Link>
      </Button>
    </AuthCard>
  );
}
