"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { SeverityBadge } from "@/components/app/status-badge";
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
import { Input } from "@/components/ui/input";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

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
          <span className="text-xs uppercase tracking-widest text-fg-muted">
            Custom Entities ({items.length})
          </span>
        </div>
        {isLoading ? (
          <div className="px-4 py-3 text-xs text-fg-muted">Loading…</div>
        ) : items.length === 0 ? (
          <div className="px-4 py-3 text-xs text-fg-muted">
            No custom entities yet. Add one below.
          </div>
        ) : (
          <Table className="w-full">
            <TableHeader>
              <TableRow>
                <TableHead className="text-left">Name</TableHead>
                <TableHead className="text-left">Pattern</TableHead>
                <TableHead className="text-left">Severity</TableHead>
                <TableHead className="text-left">Action</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((ce) => (
                <TableRow key={ce.name}>
                  <TableCell>{ce.name}</TableCell>
                  <TableCell className="max-w-[200px] truncate">
                    {ce.pattern}
                  </TableCell>
                  <TableCell>
                    <SeverityBadge severity={ce.severity} />
                  </TableCell>
                  <TableCell className="uppercase">{ce.action}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-6 rounded-sm px-2 text-xs text-status-critical hover:bg-status-critical/10 hover:text-status-critical"
                      onClick={() => deleteMut.mutate(ce.name)}
                      disabled={deleteMut.isPending}
                    >
                      Delete
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      {/* Add form */}
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 rounded-sm border border-border bg-surface-subtle/50 p-4">
        <span className="text-xs uppercase tracking-widest text-fg-muted">
          New custom entity
        </span>
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1">
            <label className="block text-xs uppercase tracking-widest text-fg-muted">Name *</label>
            <Input
              {...register("name")}
              className="w-full"
              placeholder="ProjectMercury" aria-label="ProjectMercury" />
            {errors.name && <p className="text-xs text-status-critical">{errors.name.message}</p>}
          </div>
          <div className="space-y-1">
            <label className="block text-xs uppercase tracking-widest text-fg-muted">Pattern (regex) *</label>
            <Input
              {...register("pattern")}
              className="w-full"
              placeholder="Project[ -]?Mercury" aria-label="Project[ -]?Mercury" />
            {errors.pattern && <p className="text-xs text-status-critical">{errors.pattern.message}</p>}
          </div>
          <div className="space-y-1">
            <label className="block text-xs uppercase tracking-widest text-fg-muted">Severity</label>
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
            <label className="block text-xs uppercase tracking-widest text-fg-muted">Action</label>
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
          <label className="block text-xs uppercase tracking-widest text-fg-muted">Description (optional)</label>
          <Input
            {...register("description")}
            className="w-full"
            placeholder="Confidential project code name" aria-label="Confidential project code name" />
        </div>
        {createMut.error && (
          <p className="text-xs text-status-critical">{createMut.error.message}</p>
        )}
        <Button variant="outline"
          type="submit"
          className="rounded-sm bg-status-critical text-white hover:bg-status-critical"
          disabled={createMut.isPending}
        >
          {createMut.isPending ? "Adding…" : "Add entity"}
        </Button>
      </form>
    </div>
  );
}

