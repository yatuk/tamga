"use client";

import { ChevronRight } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";
import { EVENTS_FETCH_LIMIT } from "@/lib/security/security-events-model";

export function IncidentsPaginationCard({ m }: { m: IncidentsConsoleModel }) {
  return (
    <Card className="rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
      <CardContent className="flex items-center justify-between pt-6">
        <div className="text-sm text-[var(--text-secondary)]">
          Sunucuda {m.total} olay · yuklu {m.eventsFeed.length}
          {m.hasNextPage ? ` · daha fazla var (cekim basina max ${EVENTS_FETCH_LIMIT})` : " · tum sayfalar yuklendi"}
        </div>
        <div className="flex gap-2">
          <Button
            className="cursor-pointer bg-[var(--accent)] text-[var(--accent-foreground)] hover:opacity-90"
            disabled={!m.hasNextPage || m.isFetchingNextPage}
            onClick={() => void m.fetchNextPage()}
          >
            {m.isFetchingNextPage ? "Yukleniyor…" : "Daha fazla yukle"}
            <ChevronRight className="ml-1 h-4 w-4" />
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
