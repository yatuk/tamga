"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Badge } from "@/components/ui/badge";
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
import { cn } from "@/lib/utils";

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
      <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/50">
        <div className="border-b border-zinc-200 dark:border-zinc-800 px-4 py-2">
          <span className="text-[11px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">
            Custom Entities ({items.length})
          </span>
        </div>
        {isLoading ? (
          <div className="px-4 py-3 text-xs text-zinc-600 dark:text-zinc-400">Yükleniyor…</div>
        ) : items.length === 0 ? (
          <div className="px-4 py-3 text-xs text-zinc-600 dark:text-zinc-400">
            Henüz custom entity yok. Aşağıdan ekle.
          </div>
        ) : (
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-zinc-200 dark:border-zinc-800">
                <th className="px-4 py-2 text-left font-normal text-zinc-600 dark:text-zinc-400">Name</th>
                <th className="px-4 py-2 text-left font-normal text-zinc-600 dark:text-zinc-400">Pattern</th>
                <th className="px-4 py-2 text-left font-normal text-zinc-600 dark:text-zinc-400">Severity</th>
                <th className="px-4 py-2 text-left font-normal text-zinc-600 dark:text-zinc-400">Action</th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {items.map((ce) => (
                <tr key={ce.name} className="border-b border-zinc-200 dark:border-zinc-800/50 last:border-0">
                  <td className="px-4 py-2 text-zinc-800 dark:text-zinc-200">{ce.name}</td>
                  <td className="px-4 py-2 max-w-[200px] truncate text-zinc-600 dark:text-zinc-400">
                    {ce.pattern}
                  </td>
                  <td className="px-4 py-2">
                    <SeverityBadge severity={ce.severity} />
                  </td>
                  <td className="px-4 py-2 uppercase text-zinc-600 dark:text-zinc-400">{ce.action}</td>
                  <td className="px-4 py-2 text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 cursor-pointer rounded-sm px-2 text-[10px] text-red-400 hover:bg-red-500/10 hover:text-red-300"
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
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/50 p-4">
        <span className="text-[11px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">
          Yeni Custom Entity
        </span>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">Name *</label>
            <input
              {...register("name")}
              className="w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-600 dark:text-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-600"
              placeholder="ProjectMercury"
            />
            {errors.name && <p className="text-[10px] text-red-400">{errors.name.message}</p>}
          </div>
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">Pattern (regex) *</label>
            <input
              {...register("pattern")}
              className="w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-600 dark:text-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-600"
              placeholder="Project[ -]?Mercury"
            />
            {errors.pattern && <p className="text-[10px] text-red-400">{errors.pattern.message}</p>}
          </div>
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">Severity</label>
            <Select value={watchedSeverity} onValueChange={(v) => setValue("severity", v as CustomEntityFormValues["severity"])}>
              <SelectTrigger className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 text-xs text-zinc-800 dark:text-zinc-200">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
                {["critical", "high", "medium", "low"].map((s) => (
                  <SelectItem key={s} value={s} className="text-xs uppercase">{s}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <label className="block text-[10px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">Action</label>
            <Select value={watchedAction} onValueChange={(v) => setValue("action", v as "block" | "redact" | "warn" | "log")}>
              <SelectTrigger className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 text-xs text-zinc-800 dark:text-zinc-200">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
                {["block", "redact", "warn", "log"].map((a) => (
                  <SelectItem key={a} value={a} className="text-xs uppercase">{a}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <div className="space-y-1">
          <label className="block text-[10px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">Description (opsiyonel)</label>
          <input
            {...register("description")}
            className="w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1.5 text-xs text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-600 dark:text-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-600"
            placeholder="Confidential project code name"
          />
        </div>
        {createMut.error && (
          <p className="text-[11px] text-red-400">{createMut.error.message}</p>
        )}
        <Button
          type="submit"
          className="cursor-pointer rounded-sm bg-red-600 text-white hover:bg-red-700"
          disabled={createMut.isPending}
        >
          {createMut.isPending ? "Ekleniyor…" : "Entity Ekle"}
        </Button>
      </form>
    </div>
  );
}

function SeverityBadge({ severity }: { severity: string }) {
  const map: Record<string, string> = {
    critical: "border-red-500/40 bg-red-500/10 text-red-400",
    high: "border-orange-500/40 bg-orange-500/10 text-orange-400",
    medium: "border-amber-500/40 bg-amber-500/10 text-amber-400",
    low: "border-zinc-600 bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400",
  };
  return (
    <Badge className={cn("rounded-sm border text-[9px] uppercase tracking-widest", map[severity] ?? map.low)}>
      {severity}
    </Badge>
  );
}
