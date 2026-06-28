import { Siren, AlertTriangle, Shield, Info, ShieldBan, AlertCircle, EyeOff, FileText } from "lucide-react";
import type { ComponentType } from "react";

type IconComponent = ComponentType<{ className?: string }>;

/**
 * Returns the severity badge icon and Tailwind class string.
 * Icon + class pair suitable for inline badge rendering in policy competitor cards.
 */
export function getSeverityBadge(severity: string): { icon: IconComponent; cls: string } {
  switch (severity) {
    case "critical":
      return { icon: Siren, cls: "text-red-400 bg-red-500/10 border-red-500/30" };
    case "high":
      return { icon: AlertTriangle, cls: "text-orange-400 bg-orange-500/10 border-orange-500/30" };
    case "medium":
      return { icon: Shield, cls: "text-amber-300 bg-amber-500/10 border-amber-500/30" };
    default:
      return { icon: Info, cls: "text-zinc-400 bg-zinc-500/10 border-zinc-500/30" };
  }
}

/**
 * Returns the action badge icon and Tailwind class string.
 * Icon + class pair suitable for inline badge rendering in policy competitor cards.
 */
export function getActionBadge(action: string): { icon: IconComponent; cls: string } {
  switch (action) {
    case "block":
      return { icon: ShieldBan, cls: "bg-red-500/10 text-red-400 border-red-500/30" };
    case "warn":
      return { icon: AlertCircle, cls: "bg-orange-500/10 text-orange-400 border-orange-500/30" };
    case "redact":
      return { icon: EyeOff, cls: "bg-amber-500/10 text-amber-300 border-amber-500/30" };
    default:
      return { icon: FileText, cls: "bg-zinc-500/10 text-zinc-400 border-zinc-500/30" };
  }
}

/**
 * Numeric rank for severity sorting.
 * critical=4, high=3, medium=2, low=1, unknown/missing=0
 */
export function severityRank(severity: string): number {
  switch (severity) {
    case "critical":
      return 4;
    case "high":
      return 3;
    case "medium":
      return 2;
    case "low":
      return 1;
    default:
      return 0;
  }
}
