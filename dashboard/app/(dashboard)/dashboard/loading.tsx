import { SkeletonText } from "@carbon/react";

/** Shown while a console route loads: a title, a sentence and a block, where the page will be. */
export default function DashboardLoading() {
  return (
    <div role="status" aria-label="Loading page">
      <SkeletonText heading width="16rem" />
      <SkeletonText width="28rem" />
      <div style={{ marginTop: "3rem" }}>
        <SkeletonText paragraph lineCount={8} />
      </div>
    </div>
  );
}
