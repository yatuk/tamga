"use client";

import { useCallback, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAdminKey } from "@/hooks/useAdminKey";

export type RevealedKey = {
  rawKey: string;
  id: string;
  label: string;
} | null;

export function useKeysPage() {
  const [adminKey] = useAdminKey();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; label: string } | null>(null);
  const [revealedKey, setRevealedKey] = useState<RevealedKey>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);
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
  const total = data?.total ?? 0;

  const invalidate = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["tamga-apikeys", adminKey] });
  }, [queryClient, adminKey]);

  const createMutation = useMutation({
    mutationFn: ({ label, scope }: { label: string; scope: string }) =>
      api.createApiKey(adminKey, label, scope),
    onSuccess: (result) => {
      invalidate();
      setCreateOpen(false);
      setRevealedKey({
        rawKey: result.raw_key,
        id: result.id,
        label: result.label,
      });
    },
    onError: () => {
      toast.error("Failed to create API key");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteApiKey(adminKey, id),
    onSuccess: () => {
      invalidate();
      setDeleteTarget(null);
      toast.success("API key revoked");
    },
    onError: () => {
      toast.error("Failed to revoke API key");
    },
  });

  const copyToClipboard = async (text: string, id?: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success("Copied to clipboard");
      if (id) {
        setCopiedId(id);
        setTimeout(() => setCopiedId(null), 2000);
      }
    } catch {
      toast.error("Failed to copy");
    }
  };

  const dismissReveal = () => setRevealedKey(null);

  return {
    adminKey,
    isLoading,
    hasError,
    apiKeys,
    total,
    createOpen,
    setCreateOpen,
    createMutation,
    deleteTarget,
    setDeleteTarget,
    deleteMutation,
    revealedKey,
    dismissReveal,
    copyToClipboard,
    copiedId,
  };
}
