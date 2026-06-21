"use client";

import { Suspense } from "react";
import { EventsBody } from "./EventsBody";
import { useEventsPage } from "./useEventsPage";

function EventsPageInner() {
  const p = useEventsPage();
  return <EventsBody {...p} />;
}

export default function EventsPage() {
  return (
    <Suspense
      fallback={
        <div className="space-y-2">
          <div className="h-[40px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
          <div className="h-[600px] animate-pulse rounded-sm bg-zinc-100 dark:bg-zinc-900/40" />
        </div>
      }
    >
      <EventsPageInner />
    </Suspense>
  );
}
