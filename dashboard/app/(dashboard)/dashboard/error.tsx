"use client";

import { useEffect } from "react";
import { Button } from "@carbon/react";
import { PageHead } from "@/components/console/page";

/** A page that threw while drawing. The shell stays, so the rest of the console is one click away. */
export default function DashboardError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <>
      <PageHead title="This page could not be drawn" lede="Something in the page failed, not in the proxy. Your requests are still being scanned.">
        <Button kind="primary" size="sm" onClick={reset}>
          Try again
        </Button>
      </PageHead>
      <p className="t-footnote">
        If it keeps failing, the browser console has the error{error.digest ? <>, and the server log has it under {error.digest}</> : null}.
      </p>
    </>
  );
}
