"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { SeverityBadge } from "@/components/common/badges";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { api } from "@/lib/api/client";
import type { CustomEntity } from "@/lib/api/types-core";

function isValidRegex(pattern: string): boolean {
  try {
    new RegExp(pattern);
    return true;
  } catch {
    return false;
  }
}

const customEntitySchema = z.object({
  name: z.string().min(1, "Name is required"),
  pattern: z.string().min(1, "Pattern is required").refine(isValidRegex, "Pattern is not a valid regular expression"),
  description: z.string().optional(),
  severity: z.enum(["critical", "high", "medium", "low"]),
  action: z.enum(["block", "redact", "warn", "log"]),
});

type CustomEntityFormValues = z.infer<typeof customEntitySchema>;

const DEFAULTS: CustomEntityFormValues = {
  name: "",
  pattern: "",
  description: "",
  severity: "medium",
  action: "log",
};

export function CustomEntityForm({ adminKey }: { adminKey: string }) {
  const qc = useQueryClient();

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    formState: { errors },
  } = useForm<CustomEntityFormValues>({
    resolver: zodResolver(customEntitySchema),
    defaultValues: DEFAULTS,
  });

  const watchedSeverity = watch("severity");
  const watchedAction = watch("action");

  const { data, isLoading } = useQuery({
    queryKey: ["custom-entities", adminKey],
    queryFn: () => api.listCustomEntities(adminKey),
    enabled: !!adminKey,
  });

  const createMut = useMutation({
    mutationFn: (entity: CustomEntity) => api.createCustomEntity(adminKey, entity),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["custom-entities", adminKey] });
      reset(DEFAULTS);
    },
  });

  const deleteMut = useMutation({
    mutationFn: (name: string) => api.deleteCustomEntity(adminKey, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["custom-entities", adminKey] });
    },
  });

  function onSubmit(values: CustomEntityFormValues) {
    createMut.mutate({ ...values, confidence: 0.85 });
  }

  const items = data?.items ?? [];

  return (
    <div className="space-y-6">
      {/* Entity list */}
      <div className="rounded-sm border border-border bg-surface-subtle/50">
        <div className="border-b border-border px-4 py-2">
          <span className="text-[11px] uppercase tracking-widest text-fg-muted">
            Custom Entities ({items.length})
          </span>
        </div>
        {isLoading ? (
          <div className="px-4 py-3 text-xs text-fg-muted">Yükleniyor…</div>
        ) : items.length === 0 ? (
          <div className="px-4 py-3 text-xs text-fg-muted">
            Henüz custom entity yok. Aşağıdan ekle.
          </div>
        ) : (
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-border">
                <th className="px-4 py-2 text-left font-normal text-fg-muted">Name</th>
                <th className="px-4 py-2 text-left font-normal text-fg-muted">Pattern</th>
                <th className="px-4 py-2 text-left font-normal text-fg-muted">Severity</th>
                <th className="px-4 py-2 text-left font-normal text-fg-muted">Action</th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {items.map((ce) => (
                <tr key={ce.name} className="border-b border-border/50 last:border-0">
                  <td className="px-4 py-2 text-fg">{ce.name}</td>
                  <td className="px-4 py-2 max-w-[200px] truncate text-fg-muted">
                    {ce.pattern}
                  </td>
                  <td className="px-4 py-2">
                    <SeverityBadge severity={ce.severity} />
                  </td>
                  <td className="px-4 py-2 uppercase text-fg-muted">{ce.action}</td>
                  <td className="px-4 py-2 text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 cursor-pointer rounded-sm px-2 text-[10px] text-status-critical hover:bg-status-critical/10 hover:text-status-critical"
                      onClick={() => deleteMut.mutate(ce.name)}
                      disabled={deleteMut.isPending}
                    >
                      Sil
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Add form */}
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 rounded-sm border border-border bg-surface-subtle/50 p-4">
        <span className="text-[11px] uppercase tracking-widest text-fg-muted">
          Yeni Custom Entity
        </span>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-fg-muted">Name *</label>
            <input
              {...register("name")}
              className="w-full rounded-sm border border-border-strong bg-surface-card px-2 py-1.5 text-xs text-fg placeholder:text-fg-muted focus:outline-none focus:ring-1 focus:ring-ring"
              placeholder="ProjectMercury"
            />
            {errors.name && <p className="text-[10px] text-status-critical">{errors.name.message}</p>}
          </div>
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-fg-muted">Pattern (regex) *</label>
            <input
              {...register("pattern")}
              className="w-full rounded-sm border border-border-strong bg-surface-card px-2 py-1.5 text-xs text-fg placeholder:text-fg-muted focus:outline-none focus:ring-1 focus:ring-ring"
              placeholder="Project[ -]?Mercury"
            />
            {errors.pattern && <p className="text-[10px] text-status-critical">{errors.pattern.message}</p>}
          </div>
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-fg-muted">Severity</label>
            <Select value={watchedSeverity} onValueChange={(v) => setValue("severity", v as CustomEntityFormValues["severity"])}>
              <SelectTrigger className="rounded-sm border-border-strong bg-surface-card text-xs text-fg">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="rounded-sm border-border bg-surface-card">
                {["critical", "high", "medium", "low"].map((s) => (
                  <SelectItem key={s} value={s} className="text-xs uppercase">{s}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-fg-muted">Action</label>
            <Select value={watchedAction} onValueChange={(v) => setValue("action", v as "block" | "redact" | "warn" | "log")}>
              <SelectTrigger className="rounded-sm border-border-strong bg-surface-card text-xs text-fg">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="rounded-sm border-border bg-surface-card">
                {["block", "redact", "warn", "log"].map((a) => (
                  <SelectItem key={a} value={a} className="text-xs uppercase">{a}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="space-y-1">
          <label className="block text-[10px] uppercase tracking-widest text-fg-muted">Description (opsiyonel)</label>
          <input
            {...register("description")}
            className="w-full rounded-sm border border-border-strong bg-surface-card px-2 py-1.5 text-xs text-fg placeholder:text-fg-muted focus:outline-none focus:ring-1 focus:ring-ring"
            placeholder="Confidential project code name"
          />
        </div>
        {createMut.error && (
          <p className="text-[11px] text-status-critical">{createMut.error.message}</p>
        )}
        <Button
          type="submit"
          className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical"
          disabled={createMut.isPending}
        >
          {createMut.isPending ? "Ekleniyor…" : "Entity Ekle"}
        </Button>
      </form>
    </div>
  );
}

