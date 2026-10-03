"use client";

import Link from "next/link";
import { AuthCard } from "@/components/app/auth-card";
import { Button } from "@/components/ui/button";

const GITHUB_CLIENT_ID = process.env.NEXT_PUBLIC_GITHUB_CLIENT_ID || "";

function generateState(): string {
  const arr = new Uint8Array(16);
  crypto.getRandomValues(arr);
  return Array.from(arr, (b) => b.toString(16).padStart(2, "0")).join("");
}

function GitHubIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
      />
    </svg>
  );
}

export default function LoginPage() {
  const handleGitHubLogin = () => {
    // The state value is checked again on the callback page to make sure the
    // response belongs to the sign-in this browser started.
    const state = generateState();
    sessionStorage.setItem("tamga_oauth_state", state);

    const params = new URLSearchParams({
      client_id: GITHUB_CLIENT_ID,
      redirect_uri: `${window.location.origin}/auth/callback`,
      scope: "read:user user:email",
      state,
    });
    window.location.href = `https://github.com/login/oauth/authorize?${params.toString()}`;
  };

  if (!GITHUB_CLIENT_ID) {
    return (
      <AuthCard
        title="GitHub sign-in is not set up"
        description={
          <>
            Set <span className="font-mono text-xs">NEXT_PUBLIC_GITHUB_CLIENT_ID</span> for the dashboard and{" "}
            <span className="font-mono text-xs">TAMGA_GITHUB_CLIENT_SECRET</span> for the proxy to enable it.
          </>
        }
      >
        <p className="mb-3 text-sm text-muted-foreground">Until then, the console works with the admin key from Settings.</p>
        <Button asChild variant="outline" className="w-full">
          <Link href="/dashboard">Open the Console</Link>
        </Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard title="Sign in" description="Use your GitHub account to open the Tamga console.">
      <Button className="w-full" onClick={handleGitHubLogin}>
        <GitHubIcon />
        Sign in with GitHub
      </Button>
    </AuthCard>
  );
}
