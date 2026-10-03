"use client";

import { useMemo } from "react";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useHealth, useHealthDetail } from "@/hooks/useHealth";

export function useProxyPage() {
  const [adminKey] = useAdminKey();

  const { data: health, isLoading: healthLoading, error: healthError } = useHealth();
  const { data: detail, isLoading: detailLoading } = useHealthDetail();

  const isLoading = healthLoading || detailLoading;
  const hasError = !!healthError;

  const isOnline = health?.proxy === "up" || health?.proxy_status?.up === true;

  const componentRows = useMemo(() => {
    const rows: { component: string; status: "ok" | "warning" | "error" | "disabled"; detail: string; dependsOn?: string }[] = [];

    // Proxy server
    rows.push({
      component: "HTTP Server",
      status: isOnline ? "ok" : "error",
      detail: detail?.tls_enabled ? ":8443 (TLS)" : ":8443",
      dependsOn: "network, TLS certs",
    });

    // Policy engine
    rows.push({
      component: "Policy Engine",
      status: health?.policy_path ? "ok" : "error",
      detail: health?.policy_path ? `${health.policy_path}${detail?.policy_name ? ` · ${detail.policy_name}` : ""}` : "not configured",
      dependsOn: "file system, policy YAML",
    });

    // Scanner pool
    rows.push({
      component: "Scanner Pool",
      status: (health?.scanner_count ?? 0) > 0 ? "ok" : "warning",
      detail: `${health?.scanner_count ?? 0} scanners ready`,
      dependsOn: "analyzer, gRPC",
    });

    // Database
    const dbOk = health?.database === "connected";
    rows.push({
      component: "Database",
      status: dbOk ? "ok" : health?.database === "not_configured" ? "disabled" : "error",
      detail: health?.database === "connected" ? "postgres · connected" : health?.database ?? "unknown",
      dependsOn: "network, disk",
    });

    // Redis: the health check knows whether it answers; the runtime detail
    // only knows whether it is configured.
    const redis = health?.redis ?? (detail?.redis_enabled ? "connected" : "not_configured");
    rows.push({
      component: "Redis Cache",
      status: redis === "connected" ? "ok" : redis === "not_configured" ? "disabled" : "error",
      detail: redis === "connected" ? "connected" : redis === "not_configured" ? "not configured" : redis,
      dependsOn: "network",
    });

    // Analyzer
    const analyzer = health?.analyzer ?? "not_configured";
    rows.push({
      component: "Analyzer",
      status: analyzer === "reachable" ? "ok" : analyzer === "not_configured" ? "disabled" : "error",
      detail: analyzer === "reachable" ? "gRPC, reachable" : analyzer === "not_configured" ? "not configured" : analyzer,
      dependsOn: "Python runtime, gRPC",
    });

    // Event bus
    const dropped = health?.events_dropped ?? 0;
    rows.push({
      component: "Event Bus",
      status: dropped > 0 ? "warning" : "ok",
      detail: dropped > 0 ? `${dropped} events dropped` : "no events dropped",
      dependsOn: "in-process channel",
    });

    // Retention
    rows.push({
      component: "Data Retention",
      status: detail?.retention_enabled ? "ok" : "disabled",
      detail: detail?.retention_enabled
        ? `enabled · last run ${detail.retention_last_run ?? "—"}`
        : "not configured",
      dependsOn: "database, cron scheduler",
    });

    return rows;
  }, [health, detail, isOnline]);

  return {
    adminKey,
    isLoading,
    hasError,
    isOnline,
    health,
    detail,
    componentRows,
  };
}
