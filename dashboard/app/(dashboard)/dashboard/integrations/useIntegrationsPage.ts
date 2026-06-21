"use client";

import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Webhook, type WebhookKind } from "@/lib/api";
import { toast } from "@/lib/toast";
import { INTEGRATION_PRESETS } from "./integrationPresets";
import { type IntegrationDraft, openIntegrationDraft } from "./integrationDraft";
import { useAdminKey } from "@/hooks/useAdminKey";

export function useIntegrationsPage() {
  const qc = useQueryClient();
  const router = useRouter();
  const params = useSearchParams();
  const [adminKey] = useAdminKey();
  const [draft, setDraft] = useState<IntegrationDraft | null>(null);

  useEffect(() => {
    const kind = params.get("connect") as WebhookKind | null;
    if (!kind) return;
    const preset = INTEGRATION_PRESETS.find((p) => p.kind === kind);
    if (preset) setDraft(openIntegrationDraft(preset.kind, preset.name));
    router.replace("/dashboard/integrations");
  }, [params, router]);

  const { data } = useQuery({
    queryKey: ["tamga-webhooks-integrations", adminKey],
    queryFn: () => api.listWebhooks(adminKey),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });

  const hooks = data?.items ?? [];

  const createMut = useMutation({
    mutationFn: (d: IntegrationDraft) => {
      let headers: Record<string, string> | undefined;
      if (d.headers.trim()) {
        headers = {};
        for (const line of d.headers.split(/\r?\n/)) {
          const [k, ...rest] = line.split(":");
          if (k && rest.length) headers[k.trim()] = rest.join(":").trim();
        }
      }
      const body: Omit<Webhook, "id" | "created_at"> = {
        label: d.label || d.kind,
        kind: d.kind,
        url: d.url,
        enabled: d.enabled,
        headers,
      };
      if (d.kind === "jira") {
        body.project_key = d.projectKey.trim() || undefined;
        body.issue_type = d.issueType.trim() || undefined;
      }
      if (d.kind === "pagerduty" || d.kind === "opsgenie") {
        body.auth_token = d.authToken.trim() || undefined;
      }
      return api.createWebhook(adminKey, body);
    },
    onSuccess: () => {
      toast.success("Integration connected");
      setDraft(null);
      qc.invalidateQueries({ queryKey: ["tamga-webhooks-integrations", adminKey] });
    },
    onError: (e: Error) => toast.error("Create failed", e.message),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => api.testWebhook(adminKey, id),
    onSuccess: (res) => toast.success("TEST", `status ${res.status_code}`),
    onError: (e: Error) => toast.error("Test failed", e.message),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteWebhook(adminKey, id),
    onSuccess: () => {
      toast.success("Disconnected");
      qc.invalidateQueries({ queryKey: ["tamga-webhooks-integrations", adminKey] });
    },
    onError: (e: Error) => toast.error("Delete failed", e.message),
  });

  return {
    draft,
    setDraft,
    hooks,
    createMut,
    testMut,
    deleteMut,
  };
}
