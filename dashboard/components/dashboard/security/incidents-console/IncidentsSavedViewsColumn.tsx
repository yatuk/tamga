"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";

export function IncidentsSavedViewsColumn({ m }: { m: IncidentsConsoleModel }) {
  return (
    <Card className="h-full rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Saved Views</CardTitle>
        <CardDescription className="text-zinc-600 dark:text-zinc-400">Hızlı triage filtre setleri</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Button
          type="button"
          className="h-8 w-full cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-xs text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
          onClick={() => m.saveCurrentView()}
        >
          Save current view
        </Button>
        {m.savedViews.length === 0 ? (
          <div className="text-xs text-zinc-600 dark:text-zinc-400">Henüz kayıtlı görünüm yok.</div>
        ) : (
          m.savedViews.map((v) => (
            <div key={v.id} className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 p-2">
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={() => m.applySavedView(v)}
                  className="flex-1 cursor-pointer text-left text-xs text-zinc-800 dark:text-zinc-200 hover:text-zinc-100"
                >
                  {v.name}
                </button>
                <button
                  type="button"
                  onClick={() => m.renameSavedView(v.id)}
                  className="text-[10px] text-zinc-600 dark:text-zinc-400 hover:text-zinc-200"
                >
                  edit
                </button>
                <button
                  type="button"
                  onClick={() => m.deleteSavedView(v.id)}
                  className="text-[10px] text-zinc-600 dark:text-zinc-400 hover:text-red-400"
                >
                  del
                </button>
              </div>
              <div className="mt-1 text-[10px] text-zinc-600 dark:text-zinc-400">
                {v.action}/{v.type}/{v.severity}/{v.range}/{v.triage}/{v.assignee}
              </div>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}
