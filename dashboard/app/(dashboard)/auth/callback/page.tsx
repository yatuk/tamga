"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";

export default function AuthCallbackPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const code = searchParams.get("code");
    const state = searchParams.get("state");

    if (!code) {
      setError("Missing authorization code from GitHub.");
      return;
    }

    // Verify state to prevent CSRF
    const storedState = sessionStorage.getItem("tamga_oauth_state");
    if (state && storedState && state !== storedState) {
      setError("Invalid state parameter. Possible CSRF attack.");
      return;
    }
    sessionStorage.removeItem("tamga_oauth_state");

    // Exchange code for JWT via the proxy
    fetch(`${API_BASE}/api/v1/auth/github/exchange`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    })
      .then((res) => {
        if (!res.ok) {
          return res.json().then((err) => {
            throw new Error(err.error || "Token exchange failed");
          });
        }
        return res.json();
      })
      .then(
        (data: { token: string; user: { id: string; email: string; name: string; avatar: string; role: string } }) => {
          // Store session
          sessionStorage.setItem("tamga_session_token", data.token);
          sessionStorage.setItem("tamga_session_user", JSON.stringify(data.user));
          // Redirect to dashboard overview
          router.push("/dashboard");
        },
      )
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Authentication failed");
      });
  }, [searchParams, router]);

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950">
        <div className="w-full max-w-md rounded-lg border border-red-200 bg-white p-8 text-center shadow-sm dark:border-red-900 dark:bg-zinc-900">
          <div className="mb-4 text-4xl" aria-hidden="true">&#x26A0;</div>
          <h1 className="mb-2 text-xl font-bold text-red-700 dark:text-red-400">Authentication Failed</h1>
          <p className="text-sm text-zinc-600 dark:text-zinc-400">{error}</p>
          <button
            onClick={() => router.push("/login")}
            className="mt-6 rounded-lg bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 dark:bg-zinc-100 dark:text-zinc-900"
          >
            Try Again
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-zinc-50 dark:bg-zinc-950">
      <div className="text-center">
        <div className="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-4 border-zinc-300 border-t-zinc-900 dark:border-zinc-700 dark:border-t-zinc-100" />
        <p className="text-sm text-zinc-500">Completing sign-in...</p>
      </div>
    </div>
  );
}
