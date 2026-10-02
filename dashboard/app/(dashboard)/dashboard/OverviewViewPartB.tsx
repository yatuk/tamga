"use client";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { OVERVIEW_PALETTE, overviewTrafficBarConfig } from "./overviewConstants";
import { OverviewProviderPie, OverviewTrafficChart } from "./overviewDynamicCharts";
import { useOverviewContext } from "./OverviewContext";
import { toUpperEn } from "@/lib/utils/case";
import { humanizeFindingType } from "@/lib/humanize";

export function OverviewViewPartB() {
  const { range, derived } = useOverviewContext();
  const { sevenDayData, providerPieData, providerPieConfig, topProviders, topFindingTypes } = derived;

  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <div className="text-[10px] uppercase tracking-[0.18em] text-fg-muted">TRAFFIC // {toUpperEn(range)}</div>
          <CardTitle>Traffic trend</CardTitle>
          <CardDescription>Requests, blocked and redacted over time</CardDescription>
        </CardHeader>
        <CardContent>
          <OverviewTrafficChart data={sevenDayData} config={overviewTrafficBarConfig} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Risk distribution</CardTitle>
          <CardDescription>By provider and by finding</CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="providers">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="providers">Provider</TabsTrigger>
              <TabsTrigger value="findings">Findings</TabsTrigger>
            </TabsList>
            <TabsContent value="providers" className="space-y-3">
              {providerPieData.length === 0 ? (
                <p className="py-10 text-center text-sm text-muted-foreground">No data.</p>
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
                  className="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm dark:border-border"
                >
                  <span>{humanizeFindingType(item.name)}</span>
                  <Badge className="border-border-strong bg-surface-subtle text-fg-muted dark:border-border dark:bg-surface-subtle dark:text-fg">
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
