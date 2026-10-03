import { Skeleton } from "@/components/ui/skeleton";

/**
 * Shown while a dashboard route loads. It has the shape of a page (header,
 * stats, a panel) so the layout does not jump when the content arrives.
 */
export default function DashboardLoading() {
  return (
    <div className="space-y-6" role="status" aria-label="Loading page">
      <div className="space-y-2 border-b pb-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-4 w-96 max-w-full" />
      </div>
      <div className="grid grid-cols-2 gap-px border bg-border lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <div key={i} className="space-y-3 bg-card p-4">
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-6 w-16" />
          </div>
        ))}
      </div>
      <Skeleton className="h-72 w-full" />
    </div>
  );
}
