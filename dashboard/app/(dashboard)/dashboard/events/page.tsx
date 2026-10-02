import { Suspense } from "react";
import { SkeletonRows } from "@/components/app/states";
import { EventsView } from "./events-view";

export default function EventsPage() {
  // useSearchParams in the view needs a Suspense boundary during prerender.
  return (
    <Suspense fallback={<SkeletonRows rows={10} />}>
      <EventsView />
    </Suspense>
  );
}
