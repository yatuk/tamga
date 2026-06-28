"use client";

import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { HuntingFilters } from "./HuntingFilters";
import { HuntingResults } from "./HuntingResults";
import { SavedHuntsPanel } from "./SavedHuntsPanel";
import { useHuntingPage } from "./useHuntingPage";

export default function HuntingPage() {
  const {
    page,
    setPage,
    action,
    setAction,
    provider,
    setProvider,
    shadow,
    setShadow,
    findingType,
    setFindingType,
    severity,
    setSeverity,
    category,
    setCategory,
    technique,
    setTechnique,
    q,
    setQ,
    range,
    setRange,
    savedHunts,
    data,
    isLoading,
    error,
    refetch,
    isFetching,
    applyHunt,
    saveHunt,
    deleteHunt,
  } = useHuntingPage();

  const events = data?.events ?? [];
  const total = data?.total ?? 0;

  return (
    <div className="space-y-2">
      <PageHeader
        title="Threat hunting"
        subtitle="Sunucu tarafı filtreler (PostgreSQL veya in-memory buffer). Sonuçları Incidents ile birleştirmek için satırdan derin link kullanın."
        actions={
          <Button variant="outline" size="sm" className="gap-1" onClick={() => refetch()} disabled={isFetching}>
            Yenile
          </Button>
        }
      />

      <div className="grid gap-3 lg:grid-cols-[1fr_220px]">
        <HuntingFilters
          action={action}
          setAction={setAction}
          provider={provider}
          setProvider={setProvider}
          shadow={shadow}
          setShadow={setShadow}
          findingType={findingType}
          setFindingType={setFindingType}
          severity={severity}
          setSeverity={setSeverity}
          category={category}
          setCategory={setCategory}
          technique={technique}
          setTechnique={setTechnique}
          q={q}
          setQ={setQ}
          range={range}
          setRange={setRange}
          resetPage={() => setPage(1)}
          saveHunt={saveHunt}
          total={total}
          page={page}
          isLoading={isLoading}
          isFetching={isFetching}
        />

        <SavedHuntsPanel savedHunts={savedHunts} onApply={applyHunt} onDelete={deleteHunt} />
      </div>

      <HuntingResults
        events={events}
        total={total}
        page={page}
        setPage={setPage}
        isLoading={isLoading}
        error={error as Error | null}
      />
    </div>
  );
}
