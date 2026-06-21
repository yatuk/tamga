"use client";

import { useMemo } from "react";
import {
  Activity,
  BookOpenText,
  BookText,
  Boxes,
  Briefcase,
  ChartLine,
  Gauge,
  PlayCircle,
  Scale,
  type LucideIcon,
} from "lucide-react";
import { useTranslation } from "@/lib/i18n";

export type ResourceLink = {
  href: string;
  label: string;
  caption: string;
  icon: LucideIcon;
  external?: boolean;
};

export function useResourceGroups() {
  const { t } = useTranslation();
  return useMemo(() => {
    const product: ResourceLink[] = [
      {
        href: "#try-live",
        label: t("nav.try_live"),
        caption: t("nav.try_live_cap"),
        icon: PlayCircle,
      },
      {
        href: "/models",
        label: t("nav.models"),
        caption: t("nav.models_cap"),
        icon: Boxes,
      },
      {
        href: "/status",
        label: t("nav.status"),
        caption: t("nav.status_cap"),
        icon: Activity,
      },
    ];
    const learn: ResourceLink[] = [
      {
        href: "/blog",
        label: "Blog",
        caption: "LLM güvenliği mühendislik notları",
        icon: BookOpenText,
      },
      {
        href: "/compare",
        label: t("nav.compare"),
        caption: t("nav.compare_cap"),
        icon: Scale,
      },
      {
        href: "/roi",
        label: t("nav.roi"),
        caption: t("nav.roi_cap"),
        icon: Gauge,
      },
      {
        href: "/evals",
        label: t("nav.evals"),
        caption: t("nav.evals_cap"),
        icon: ChartLine,
      },
      {
        href: "/case-studies",
        label: "Case studies",
        caption: "Saha notları ve metrikler",
        icon: Briefcase,
      },
      {
        href: "/whitepaper",
        label: "Whitepaper",
        caption: "Hibrit tarayıcı mimari notu",
        icon: BookText,
      },
    ];
    return { product, learn };
  }, [t]);
}
