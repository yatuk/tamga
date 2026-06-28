"use client";

import { Shield } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";
import type {
  ActionFilter,
  AssigneeFilter,
  DensityMode,
  ProviderFilter,
  SeverityFilter,
  TimeRange,
  TriageFilter,
  TypeFilter,
} from "@/lib/security/security-events-model";

export function IncidentsFiltersCard({ m }: { m: IncidentsConsoleModel }) {
  return (
    <Card className="rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Shield className="h-4 w-4 text-red-500" />
          Incidents Console
        </CardTitle>
        <CardDescription className="text-zinc-700 dark:text-zinc-300">
          Triage odakli olay kuyrugu, filtreleme ve detay inceleme.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300">Presets</Badge>
          <Button
            className="h-8 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-xs text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={() => m.applyPreset("critical-now")}
          >
            Critical Now
          </Button>
          <Button
            className="h-8 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-xs text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={() => m.applyPreset("block-focused")}
          >
            Block Focused
          </Button>
          <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300">Density</Badge>
          <div className="inline-flex overflow-hidden rounded-sm border border-zinc-300 dark:border-zinc-700">
            {(["comfortable", "compact"] as DensityMode[]).map((d) => (
              <button
                key={d}
                className={`px-3 py-1 text-xs capitalize ${m.density === d ? "bg-emerald-600 text-white" : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300"}`}
                onClick={() => m.setDensity(d)}
                type="button"
              >
                {d}
              </button>
            ))}
          </div>
          <input
            ref={m.searchInputRef}
            value={m.searchText}
            onChange={(e) => m.setSearchText(e.target.value)}
            placeholder="Search request/provider/model... (/)"
            className="h-8 min-w-[240px] rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-xs text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-600 dark:text-zinc-400"
          />
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-8">
          <Select
            value={m.actionFilter}
            onValueChange={(v) => m.setActionFilter(v as ActionFilter)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Aksiyon" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Aksiyon: Hepsi</SelectItem>
              <SelectItem value="BLOCK">BLOCK</SelectItem>
              <SelectItem value="REDACT">REDACT</SelectItem>
              <SelectItem value="WARN">WARN</SelectItem>
              <SelectItem value="LOG">LOG</SelectItem>
              <SelectItem value="PASS">PASS</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={m.typeFilter}
            onValueChange={(v) => m.setTypeFilter(v as TypeFilter)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Tür" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Tümü</SelectItem>
              <SelectItem value="pii">PII</SelectItem>
              <SelectItem value="secret">Gizli Anahtar</SelectItem>
              <SelectItem value="injection">Enjeksiyon</SelectItem>
              <SelectItem value="custom">Özel</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={m.severityFilter}
            onValueChange={(v) => m.setSeverityFilter(v as SeverityFilter)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Önem" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Tümü</SelectItem>
              <SelectItem value="critical">Kritik</SelectItem>
              <SelectItem value="high">Yüksek</SelectItem>
              <SelectItem value="medium">Orta</SelectItem>
              <SelectItem value="low">Düşük</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={m.timeRange}
            onValueChange={(v) => m.setTimeRange(v as TimeRange)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Zaman aralığı" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="1h">Son 1 saat</SelectItem>
              <SelectItem value="24h">Son 24 saat</SelectItem>
              <SelectItem value="7d">Son 7 gün</SelectItem>
              <SelectItem value="30d">Son 30 gün</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={m.triageFilter}
            onValueChange={(v) => m.setTriageFilter(v as TriageFilter)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Triaj Durumu" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Tümü</SelectItem>
              <SelectItem value="Open">Açık</SelectItem>
              <SelectItem value="In Progress">İşlemde</SelectItem>
              <SelectItem value="Closed">Kapalı</SelectItem>
              <SelectItem value="False Positive">Hatalı Tespit</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={m.assigneeFilter}
            onValueChange={(v) => m.setAssigneeFilter(v as AssigneeFilter)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Atanan" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Tümü</SelectItem>
              <SelectItem value="me">Ben</SelectItem>
              <SelectItem value="unassigned">Atanmamış</SelectItem>
            </SelectContent>
          </Select>

          <Select
            value={m.providerFilter}
            onValueChange={(v) => m.setProviderFilter(v as ProviderFilter)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Sağlayıcı" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Tümü</SelectItem>
              <SelectItem value="openai">OpenAI</SelectItem>
              <SelectItem value="anthropic">Anthropic</SelectItem>
              <SelectItem value="google">Google</SelectItem>
              <SelectItem value="azure">Azure</SelectItem>
              <SelectItem value="unknown">Bilinmeyen</SelectItem>
              <SelectItem value="shadow">Gölge (kurumsal değil)</SelectItem>
            </SelectContent>
          </Select>

          <input
            value={m.requestIdFilter}
            onChange={(e) => m.setRequestIdFilter(e.target.value.trim())}
            placeholder="İstek Kodu filtresi"
            className="h-10 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-3 text-xs text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-600 dark:text-zinc-400"
          />
        </div>
      </CardContent>
    </Card>
  );
}
