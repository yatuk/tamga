"use client";

import { useEffect, useState } from "react";

const GITHUB_CLIENT_ID = process.env.NEXT_PUBLIC_GITHUB_CLIENT_ID || "";

function generateState(): string {
  const arr = new Uint8Array(16);
  crypto.getRandomValues(arr);
  return Array.from(arr, (b) => b.toString(16).padStart(2, "0")).join("");
}

export default function LoginPage() {
  const [configured, setConfigured] = useState(true);

  useEffect(() => {
    if (!GITHUB_CLIENT_ID) {
      setConfigured(false);
    }
  }, []);

  const handleGitHubLogin = () => {
    const state = generateState();
    sessionStorage.setItem("tamga_oauth_state", state);

    const callbackURL = `${window.location.origin}/auth/callback`;
    const params = new URLSearchParams({
      client_id: GITHUB_CLIENT_ID,
      redirect_uri: callbackURL,
      scope: "read:user user:email",
      state,
    });

    window.location.href = `https://github.com/login/oauth/authorize?${params.toString()}`;
  };

  if (!configured) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-subtle dark:bg-surface-base">
        <div className="w-full max-w-md rounded-lg border border-border bg-white p-8 text-center shadow-sm dark:border-border dark:bg-surface-subtle">
          <h1 className="mb-2 text-2xl font-bold text-fg">Tamga</h1>
          <p className="text-sm text-fg-subtle">
            GitHub OAuth is not configured. Set <code className="rounded bg-surface-subtle px-1 dark:bg-surface-elevated">NEXT_PUBLIC_GITHUB_CLIENT_ID</code> and{" "}
            <code className="rounded bg-surface-subtle px-1 dark:bg-surface-elevated">TAMGA_GITHUB_CLIENT_SECRET</code>.
          </p>
          <p className="mt-4 text-xs text-fg-subtle">
            Use admin key to access the dashboard in the meantime.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-subtle dark:bg-surface-base">
      <div className="w-full max-w-md rounded-lg border border-border bg-white p-8 shadow-sm dark:border-border dark:bg-surface-subtle">
        <div className="mb-6 text-center">
          <h1 className="text-2xl font-bold text-fg">Tamga</h1>
          <p className="mt-1 text-sm text-fg-subtle">AI Security Proxy</p>
        </div>

        <button
          onClick={handleGitHubLogin}
          className="flex w-full items-center justify-center gap-3 rounded-lg border border-border-strong bg-surface-subtle px-4 py-3 text-sm font-medium text-white transition-colors hover:bg-surface-elevated dark:border-border-strong dark:bg-surface-subtle dark:text-fg dark:hover:bg-surface-subtle"
          aria-label="Sign in with GitHub"
        >
          <svg className="h-5 w-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path
              fillRule="evenodd"
              d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"
              clipRule="evenodd"
            />
          </svg>
          Sign in with GitHub
        </button>

        <p className="mt-4 text-center text-xs text-fg-subtle">
          Authenticate with your GitHub account to access the Tamga dashboard.
        </p>
      </div>
    </div>
  );
}
