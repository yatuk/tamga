"use client";

import { useCallback, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { permissionDenied } from "@/lib/api/fetch-core";
import { toast } from "@/lib/toast";
import { useAdminKey } from "@/hooks/useAdminKey";

type RevealedKey = {
  rawKey: string;
  id: string;
  label: string;
} | null;

export function useKeysPage() {
  const [adminKey] = useAdminKey();
  const [createOpen, setCreateOpen] = useState(false);
  const [revealedKey, setRevealedKey] = useState<RevealedKey>(null);
  const queryClient = useQueryClient();

  const { data, isLoading, error: listError } = useQuery({
    queryKey: ["tamga-apikeys", adminKey],
    queryFn: () => api.listApiKeys(adminKey),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 30 * 1000,
  });

  const hasError = !!listError;
  const apiKeys = data?.items ?? [];

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["tamga-apikeys", adminKey] });
  }, [queryClient, adminKey]);

  const createMutation = useMutation({
    mutationFn: ({ label, scope, orgId }: { label: string; scope: string; orgId?: string }) =>
      api.createApiKey(adminKey, label, scope, orgId),
    onSuccess: (result) => {
      invalidate();
      setCreateOpen(false);
      setRevealedKey({
        rawKey: result.raw_key,
        id: result.id,
        label: result.label,
      });
    },
    onError: (e: Error) => {
      toast.error("Could not create the API key", permissionDenied(e)?.refused ?? e.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteApiKey(adminKey, id),
    onSuccess: () => {
      invalidate();
      toast.success("API key revoked");
    },
    onError: (e: Error) => {
      toast.error("Could not revoke the API key", permissionDenied(e)?.refused ?? e.message);
    },
  });

  const dismissReveal = () => setRevealedKey(null);

  return {
    adminKey,
    isLoading,
    hasError,
    listError,
    apiKeys,
    createOpen,
    setCreateOpen,
    createMutation,
    deleteMutation,
    revealedKey,
    dismissReveal,
  };
}
