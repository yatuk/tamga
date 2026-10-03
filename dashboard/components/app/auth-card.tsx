import * as React from "react";
import { TamgaMark } from "@/components/shell/tamga-mark";

interface AuthCardProps {
  title: string;
  description?: React.ReactNode;
  children?: React.ReactNode;
}

/** The centered card used by the sign-in, sign-up and OAuth callback pages. */
export function AuthCard({ title, description, children }: AuthCardProps) {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-background px-4 py-10 text-foreground">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center gap-2.5">
          <TamgaMark className="size-6" />
          <span className="font-display text-xl font-extrabold tracking-wide uppercase">Tamga</span>
        </div>
        <div className="border bg-card p-6">
          <h1 className="text-lg font-semibold text-balance">{title}</h1>
          {description ? <p className="mt-1 text-sm text-muted-foreground">{description}</p> : null}
          {children ? <div className="mt-6">{children}</div> : null}
        </div>
      </div>
    </main>
  );
}
