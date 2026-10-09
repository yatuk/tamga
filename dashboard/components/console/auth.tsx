import type { ReactNode } from "react";
import NextLink from "next/link";
import { Button, Theme } from "@carbon/react";

/** The frame of the sign-in and sign-up pages: the mark, and one thing to do. */
export function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <div className="tamga">
      <Theme theme="g10">
        <main className="t-auth">
          <p className="t-brand t-authbrand">
            TAMGA<i>.</i>
          </p>
          {children}
        </main>
      </Theme>
    </div>
  );
}

/** Shown when the deployment has no identity provider: there is nothing to sign in to. */
export function NoAccounts({ title, children }: { title: string; children: ReactNode }) {
  return (
    <AuthFrame>
      <h1>{title}</h1>
      <p className="t-lede">{children}</p>
      <Button as={NextLink} href="/dashboard" kind="primary" size="lg" style={{ marginTop: "2rem" }}>
        Open the console
      </Button>
    </AuthFrame>
  );
}
