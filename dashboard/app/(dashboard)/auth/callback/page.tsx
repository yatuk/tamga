"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { AuthCard } from "@/components/app/auth-card";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { API_BASE } from "@/lib/api/fetch-core";

type Session = {
  token: string;
  user: { id: string; email: string; name: string; avatar: string; role: string };
};

function Callback() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const code = searchParams.get("code");
    const state = searchParams.get("state");

    if (!code) {
      setError("GitHub did not return an authorization code.");
      return;
    }

    // The response must carry the state this browser generated before it was
    // sent to GitHub. A missing value is treated the same as a wrong one, so
    // a response that was not started here cannot sign anyone in.
    const storedState = sessionStorage.getItem("tamga_oauth_state");
    sessionStorage.removeItem("tamga_oauth_state");
    if (!state || !storedState || state !== storedState) {
      setError("This sign-in response does not match a request started in this browser.");
      return;
    }

    fetch(`${API_BASE}/api/v1/auth/github/exchange`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    })
      .then(async (res) => {
        if (!res.ok) {
          const body = (await res.json().catch(() => ({}))) as { error?: string };
          throw new Error(body.error || "The proxy could not complete the sign-in.");
        }
        return res.json() as Promise<Session>;
      })
      .then((data) => {
        sessionStorage.setItem("tamga_session_token", data.token);
        sessionStorage.setItem("tamga_session_user", JSON.stringify(data.user));
        router.push("/dashboard");
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "The sign-in did not complete.");
      });
  }, [searchParams, router]);

  if (error) {
    return (
      <AuthCard title="Sign-in failed" description={error}>
        <div role="alert" className="sr-only">
          Sign-in failed. {error}
        </div>
        <Button asChild className="w-full">
          <Link href="/login">Try Again</Link>
        </Button>
      </AuthCard>
    );
  }

  return <Pending />;
}

function Pending() {
  return (
    <AuthCard title="Completing sign-in…">
      <div role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner />
        Checking with the proxy
      </div>
    </AuthCard>
  );
}

export default function AuthCallbackPage() {
  // useSearchParams suspends during prerender.
  return (
    <Suspense fallback={<Pending />}>
      <Callback />
    </Suspense>
  );
}
