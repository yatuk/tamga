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
    <main className="flex min-h-screen items-center justify-center bg-white dark:bg-zinc-950 px-4 text-zinc-900 dark:text-zinc-100">
      <div className="w-full max-w-md rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-6">
        <h1 className="mb-1 text-xl font-semibold tracking-tight">Create account</h1>
        <p className="mb-4 text-sm text-zinc-600 dark:text-zinc-400">Tamga early access hesabı oluşturun.</p>
        {enabled ? (
          <ClerkSignUp />
        ) : (
          <div className="space-y-3 font-mono text-xs text-zinc-600 dark:text-zinc-400">
            <p>Clerk devre dışı (demo mod). Demo için:</p>
            <Link
              href="/dashboard"
              className="inline-flex h-9 items-center rounded-sm bg-red-600 px-4 text-white hover:bg-red-700"
            >
              /dashboard
            </Link>
          </div>
        )}
      </div>
    </main>
  );
}
