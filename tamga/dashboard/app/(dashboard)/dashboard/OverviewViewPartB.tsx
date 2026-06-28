"use client";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OVERVIEW_PALETTE, overviewTrafficBarConfig } from "./overviewConstants";
import { OverviewProviderPie, OverviewTrafficChart } from "./overviewDynamicCharts";
import { useOverviewContext } from "./OverviewContext";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { humanizeFindingType } from "@/lib/humanize";

export function OverviewViewPartB() {
  const { range, derived } = useOverviewContext();
  const { sevenDayData, providerPieData, providerPieConfig, topProviders, topFindingTypes } = derived;

  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">TRAFFIC // {toUpperLocale(range)}</div>
          <CardTitle>Trafik trendi</CardTitle>
          <CardDescription>İstek, engellenen ve maskelenen trendi</CardDescription>
        </CardHeader>
        <CardContent>
          <OverviewTrafficChart data={sevenDayData} config={overviewTrafficBarConfig} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Risk dağılımı</CardTitle>
          <CardDescription>Provider ve finding görünümü</CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="providers">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="providers">Provider</TabsTrigger>
              <TabsTrigger value="findings">Findings</TabsTrigger>
            </TabsList>
            <TabsContent value="providers" className="space-y-3">
              {providerPieData.length === 0 ? (
                <p className="py-10 text-center text-sm text-muted-foreground">Veri yok.</p>
              ) : (
                <OverviewProviderPie data={providerPieData} config={providerPieConfig} />
              )}
              {topProviders.map((item, idx) => (
                <div key={item.name} className="flex items-center justify-between text-sm">
                  <span className="flex items-center gap-2">
                    <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: OVERVIEW_PALETTE[idx % OVERVIEW_PALETTE.length] }} />
                    {item.name}
                  </span>
                  <span className="font-semibold">{item.value}</span>
                </div>
              ))}
            </TabsContent>
            <TabsContent value="findings" className="space-y-2">
              {topFindingTypes.map((item) => (
                <div
                  key={item.name}
                  className="flex items-center justify-between rounded-md border border-slate-200 px-3 py-2 text-sm dark:border-slate-800"
                >
                  <span>{humanizeFindingType(item.name)}</span>
                  <Badge className="border-slate-300 bg-slate-100 text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
                    {item.value}
                  </Badge>
                </div>
              ))}
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  );
}
