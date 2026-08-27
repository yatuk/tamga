"use client";

import Link from "next/link";
import dynamic from "next/dynamic";
import { toLowerEn } from "@/lib/utils/tr-string";

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

  return (
    <main className="flex min-h-screen items-center justify-center bg-surface-card px-4 text-fg">
      <div className="w-full max-w-md rounded-sm border border-border bg-surface-card p-6">
        <h1 className="mb-1 text-xl font-semibold tracking-tight">Create account</h1>
        <p className="mb-4 text-sm text-fg-muted">Tamga early access hesabı oluşturun.</p>
        {enabled ? (
          <ClerkSignUp />
        ) : (
          <div className="space-y-3 font-mono text-xs text-fg-muted">
            <p>Clerk devre dışı (demo mod). Demo için:</p>
            <Link
              href="/dashboard"
              className="inline-flex h-9 items-center rounded-sm bg-status-critical px-4 text-white hover:bg-status-critical"
            >
              /dashboard
            </Link>
          </div>
        )}
      </div>
    </main>
  );
}
