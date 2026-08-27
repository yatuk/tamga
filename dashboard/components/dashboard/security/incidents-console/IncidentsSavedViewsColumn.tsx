"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";

export function IncidentsSavedViewsColumn({ m }: { m: IncidentsConsoleModel }) {
  return (
    <Card className="h-full rounded-sm border-border bg-surface-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Saved Views</CardTitle>
        <CardDescription className="text-fg-muted">Hızlı triage filtre setleri</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Button
          type="button"
          className="h-8 w-full cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 text-xs text-fg-muted hover:bg-surface-card"
          onClick={() => m.saveCurrentView()}
        >
          Save current view
        </Button>
        {m.savedViews.length === 0 ? (
          <div className="text-xs text-fg-muted">Henüz kayıtlı görünüm yok.</div>
        ) : (
          m.savedViews.map((v) => (
            <div key={v.id} className="rounded-sm border border-border bg-surface-subtle p-2">
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={() => m.applySavedView(v)}
                  className="flex-1 cursor-pointer text-left text-xs text-fg hover:text-fg"
                >
                  {v.name}
                </button>
                <button
                  type="button"
                  onClick={() => m.renameSavedView(v.id)}
                  className="text-[10px] text-fg-muted hover:text-fg"
                >
                  edit
                </button>
                <button
                  type="button"
                  onClick={() => m.deleteSavedView(v.id)}
                  className="text-[10px] text-fg-muted hover:text-status-critical"
                >
                  del
                </button>
              </div>
              <div className="mt-1 text-[10px] text-fg-muted">
                {v.action}/{v.type}/{v.severity}/{v.range}/{v.triage}/{v.assignee}
              </div>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}
