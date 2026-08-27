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
          <div className="h-[40px] animate-pulse rounded-sm bg-surface-subtle" />
          <div className="h-[600px] animate-pulse rounded-sm bg-surface-subtle" />
        </div>
      }
    >
      <EventsPageInner />
    </Suspense>
  );
}
