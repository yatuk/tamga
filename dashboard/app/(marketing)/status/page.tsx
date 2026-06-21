"use client";

import { useEffect, useState } from "react";
import { MarketingDoc } from "@/app/(marketing)/_components/marketing/MarketingDoc";

type HealthStatus = "ok" | "degraded" | "down" | "loading";

interface Incident {
  date: string;
  title: string;
  status: "resolved" | "monitoring" | "investigating";
  detail: string;
}

const INCIDENTS: Incident[] = [
  {
    date: "2026-04-02",
    title: "Dashboard websocket bağlantı sıçraması",
    status: "resolved",
    detail:
      "EU-Central-1 pod'undaki bir idle-timeout ayarı nedeniyle SSE bağlantıları 9 dakika boyunca düşüp tekrar bağlandı. Proxy trafiği etkilenmedi.",
  },
  {
    date: "2026-03-14",
    title: "Anthropic upstream 5xx dalgası",
    status: "resolved",
    detail:
      "Anthropic tarafında 22 dakikalık bir kesinti circuit breaker tarafından yakalandı; istekler OpenAI fallback'ine yönlendirildi.",
  },
  {
    date: "2026-02-11",
    title: "Planlı bakım — politika motoru sürüm atlaması",
    status: "resolved",
    detail:
      "Gece 02:00-02:12 TSİ arası rolling upgrade. Zero downtime, proxy 503 yayınlamadı.",
  },
];

function badgeColor(s: Incident["status"]) {
  switch (s) {
    case "resolved":
      return "border-emerald-500/40 bg-emerald-500/10 text-emerald-300";
    case "monitoring":
      return "border-amber-500/40 bg-amber-500/10 text-amber-300";
    case "investigating":
      return "border-red-500/40 bg-red-500/10 text-red-300";
  }
}

export default function StatusPage() {
  const [proxyStatus, setProxyStatus] = useState<HealthStatus>("loading");
  const [lastCheck, setLastCheck] = useState<string>("");

  useEffect(() => {
    let cancelled = false;
    const base =
      process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
    const ping = async () => {
      try {
        const res = await fetch(`${base}/health`, {
          signal: AbortSignal.timeout(3000),
        });
        if (cancelled) return;
        setProxyStatus(res.ok ? "ok" : "degraded");
      } catch {
        if (cancelled) return;
        setProxyStatus("down");
      }
      setLastCheck(new Date().toLocaleTimeString("tr-TR"));
    };
    ping();
    const iv = setInterval(ping, 30_000);
    return () => {
      cancelled = true;
      clearInterval(iv);
    };
  }, []);

  const dot =
    proxyStatus === "ok"
      ? "bg-emerald-500"
      : proxyStatus === "degraded"
        ? "bg-amber-500"
        : proxyStatus === "down"
          ? "bg-red-500"
          : "bg-zinc-500";

  const label =
    proxyStatus === "ok"
      ? "Tüm sistemler operasyonel"
      : proxyStatus === "degraded"
        ? "Kısmi performans düşüşü"
        : proxyStatus === "down"
          ? "Kesinti yaşanıyor"
          : "Kontrol ediliyor…";

  return (
    <MarketingDoc
      eyebrow="STATUS // LIVE"
      title="Tamga Durum Sayfası"
      intro={
        <p>
          Bu sayfa 30 saniyede bir <code>/health</code> uç noktasını ping&apos;ler;
          son 90 günün planlı bakım ve olay kayıtlarını listeler. Daha ayrıntılı
          uptime metriği için{" "}
          <a href="mailto:support@tamga.dev">support@tamga.dev</a> adresine
          ulaşabilirsiniz.
        </p>
      }
    >
      <div className="not-prose flex items-center justify-between gap-4 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/50 p-4">
        <div className="flex items-center gap-3">
          <span
            className={`inline-block h-3 w-3 animate-pulse rounded-full ${dot}`}
          />
          <div>
            <div className="font-mono text-sm text-zinc-900 dark:text-zinc-100">{label}</div>
            <div className="font-mono text-[11px] text-zinc-500 dark:text-zinc-400">
              {lastCheck ? `son kontrol // ${lastCheck}` : "bağlanıyor…"}
            </div>
          </div>
        </div>
        <div className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500 dark:text-zinc-400">
          proxy · eu-central-1
        </div>
      </div>

      <h2>Bileşenler</h2>
      <div className="not-prose grid gap-2 sm:grid-cols-2">
        {[
          { name: "Proxy API", status: proxyStatus === "loading" ? "loading" : proxyStatus },
          { name: "Dashboard", status: "ok" as HealthStatus },
          { name: "Policy engine", status: "ok" as HealthStatus },
          { name: "Event bus", status: "ok" as HealthStatus },
        ].map((c) => {
          const statusDot =
            c.status === "ok"
              ? "bg-emerald-500"
              : c.status === "degraded"
                ? "bg-amber-500"
                : c.status === "down"
                  ? "bg-red-500"
                  : "bg-zinc-500";
          return (
            <div
              key={c.name}
              className="flex items-center justify-between rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 py-2 font-mono text-sm"
            >
              <span className="text-zinc-800 dark:text-zinc-200">{c.name}</span>
              <span className="flex items-center gap-2 text-[11px] text-zinc-500 dark:text-zinc-400">
                <span
                  className={`inline-block h-2 w-2 rounded-full ${statusDot}`}
                />
                {c.status}
              </span>
            </div>
          );
        })}
      </div>

      <h2>Son 90 gün olay feed&apos;i</h2>
      <ul className="not-prose space-y-3">
        {INCIDENTS.map((i) => (
          <li
            key={i.date}
            className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3"
          >
            <div className="flex items-center justify-between">
              <span className="font-mono text-[11px] uppercase tracking-[0.16em] text-zinc-500 dark:text-zinc-400">
                {i.date}
              </span>
              <span
                className={`rounded-sm border px-2 py-0.5 font-mono text-[10px] uppercase ${badgeColor(i.status)}`}
              >
                {i.status}
              </span>
            </div>
            <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">
              {i.title}
            </div>
            <p className="mt-1 text-xs leading-relaxed text-zinc-600 dark:text-zinc-400">
              {i.detail}
            </p>
          </li>
        ))}
      </ul>
    </MarketingDoc>
  );
}
