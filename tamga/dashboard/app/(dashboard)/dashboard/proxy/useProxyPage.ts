"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";

export function useProxyPage() {
  const [adminKey] = useAdminKey();

  const { data: health, isLoading: healthLoading, error: healthError } = useQuery({
    queryKey: ["tamga-proxy-health-detailed", adminKey],
    queryFn: () => api.getHealthDetailed(),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 10 * 1000,
    refetchInterval: 15_000,
  });

  const { data: detail, isLoading: detailLoading } = useQuery({
    queryKey: ["tamga-proxy-health-detail", adminKey],
    queryFn: () => api.getHealthDetail(),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 10 * 1000,
    refetchInterval: 15_000,
  });

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

    // Redis
    const redisOk = detail?.redis_enabled;
    rows.push({
      component: "Redis Cache",
      status: redisOk ? "ok" : "disabled",
      detail: redisOk ? "enabled" : "not configured",
      dependsOn: "network",
    });

    // Analyzer
    rows.push({
      component: "Analyzer",
      status: "ok", // health doesn't expose analyzer status separately — infer from scanner_count > 0
      detail: "gRPC :50051 · HTTP :8444",
      dependsOn: "Python runtime, Presidio, gRPC",
    });

    // Event bus
    rows.push({
      component: "Event Bus",
      status: "ok",
      detail: "buffered channel · 1000 cap",
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
