import { Globe, Landmark, Layers, Shield, type LucideIcon } from "lucide-react";

import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export type ComplianceFramework = {
  id: string;
  name: string;
  standard: string;
  icon: "shield" | "landmark" | "globe" | "layers";
  readyPct: number; // 0-100
  readyControls: number;
  totalControls: number; // = readyControls + remaining
};

export type ComplianceReadinessProps = {
  frameworks: ComplianceFramework[];
};

const FRAMEWORK_ICONS: Record<ComplianceFramework["icon"], LucideIcon> = {
  shield: Shield,
  landmark: Landmark,
  globe: Globe,
  layers: Layers,
};

export const DEMO_FRAMEWORKS: ComplianceFramework[] = [
  {
    id: "kvkk",
    name: "KVKK",
    standard: "Law No. 6698",
    icon: "shield",
    readyPct: 100,
    readyControls: 229,
    totalControls: 229,
  },
  {
    id: "bddk",
    name: "BDDK",
    standard: "IT Governance Circular",
    icon: "landmark",
    readyPct: 64,
    readyControls: 118,
    totalControls: 184,
  },
  {
    id: "gdpr",
    name: "GDPR",
    standard: "Regulation (EU) 2016/679",
    icon: "globe",
    readyPct: 82,
    readyControls: 201,
    totalControls: 245,
  },
  {
    id: "owasp-llm",
    name: "OWASP LLM Top 10",
    standard: "v1.1",
    icon: "layers",
    readyPct: 0,
    readyControls: 0,
    totalControls: 10,
  },
];

function FrameworkCard({ framework }: { framework: ComplianceFramework }) {
  const Icon = FRAMEWORK_ICONS[framework.icon] ?? Shield;
  const evaluated =
    framework.readyPct != null && framework.readyPct > 0;
  const pct = evaluated
    ? Math.min(100, Math.max(0, framework.readyPct))
    : 0;

  return (
    <Card className="flex flex-col gap-4 p-4">
      {/* Header: circular icon + name/standard */}
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-border bg-surface-subtle">
          <Icon className="h-5 w-5 text-fg-muted" aria-hidden />
        </div>
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold text-fg">
            {framework.name}
          </div>
          <div className="truncate text-xs text-fg-subtle">
            {framework.standard}
          </div>
        </div>
      </div>

      {/* Full-width thin readiness line */}
      <div>
        <div className="h-1 w-full overflow-hidden rounded-sm bg-surface-subtle">
          <div
            className={cn("h-full", evaluated ? "bg-status-pass" : "bg-surface-subtle")}
            style={{ width: `${pct}%` }}
          />
        </div>
        <div className="mt-2 text-[10px] tracking-[0.14em] text-fg-subtle">
          {evaluated ? "FRAMEWORK READINESS" : "NOT EVALUATED YET"}
        </div>
      </div>

      {/* Ready controls + requirements */}
      <div className="grid grid-cols-2 gap-3">
        <div>
          <div className="text-[10px] tracking-[0.14em] text-fg-subtle">
            READY CONTROLS
          </div>
          <div
            className={cn(
              "mt-1 font-mono text-2xl font-semibold tabular-nums",
              evaluated ? "text-status-pass" : "text-fg-muted",
            )}
          >
            {pct}%
          </div>
        </div>
        <div>
          <div className="text-[10px] tracking-[0.14em] text-fg-subtle">
            REQUIREMENTS
          </div>
          <div className="mt-1 font-mono text-2xl font-semibold tabular-nums text-fg">
            {framework.totalControls}
          </div>
        </div>
      </div>

      {/* Footer: controls total */}
      <div className="mt-auto flex items-baseline justify-between border-t border-border-subtle pt-3">
        <span className="text-[10px] tracking-[0.14em] text-fg-subtle">
          CONTROLS
        </span>
        <span className="font-mono text-sm tabular-nums text-fg-muted">
          {framework.totalControls}
        </span>
      </div>
    </Card>
  );
}

export default function ComplianceReadiness({
  frameworks,
}: ComplianceReadinessProps) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      {frameworks.map((framework) => (
        <FrameworkCard key={framework.id} framework={framework} />
      ))}
    </div>
  );
}
