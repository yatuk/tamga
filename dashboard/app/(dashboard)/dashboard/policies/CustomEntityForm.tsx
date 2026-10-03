"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { ConfirmButton } from "@/components/app/confirm-button";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { EmptyState, SkeletonRows } from "@/components/app/states";
import { ActionBadge, SeverityBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api } from "@/lib/api/client";
import type { CustomEntity } from "@/lib/api/types-core";
import { toast } from "@/lib/toast";

function isValidRegex(pattern: string): boolean {
  try {
    new RegExp(pattern);
    return true;
  } catch {
    return false;
  }
}

const SEVERITIES = ["critical", "high", "medium", "low"] as const;
const ACTIONS = ["block", "redact", "warn", "log"] as const;

const customEntitySchema = z.object({
  name: z.string().min(1, "Give the entity a name."),
  pattern: z.string().min(1, "Enter a regular expression.").refine(isValidRegex, "This is not a valid regular expression."),
  description: z.string().optional(),
  severity: z.enum(SEVERITIES),
  action: z.enum(ACTIONS),
});

type CustomEntityFormValues = z.infer<typeof customEntitySchema>;

const DEFAULTS: CustomEntityFormValues = {
  name: "",
  pattern: "",
  description: "",
  severity: "medium",
  action: "log",
};

function FieldError({ id, message }: { id: string; message?: string }) {
  if (!message) return null;
  return (
    <p id={id} role="alert" className="text-xs text-status-critical">
      {message}
    </p>
  );
}

/** Organisation-specific patterns: project names, internal IDs, customer codes. */
export function CustomEntityForm({ adminKey }: { adminKey: string }) {
  const qc = useQueryClient();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CustomEntityFormValues>({
    resolver: zodResolver(customEntitySchema),
    defaultValues: DEFAULTS,
  });

  const { data, isLoading } = useQuery({
    queryKey: ["custom-entities", adminKey],
    queryFn: () => api.listCustomEntities(adminKey),
    enabled: !!adminKey,
  });

  const createMut = useMutation({
    mutationFn: (entity: CustomEntity) => api.createCustomEntity(adminKey, entity),
    onSuccess: (_data, entity) => {
      qc.invalidateQueries({ queryKey: ["custom-entities", adminKey] });
      reset(DEFAULTS);
      toast.success("Entity added", entity.name);
    },
    onError: (e) => toast.error("Could not add the entity", e.message),
  });

  const deleteMut = useMutation({
    mutationFn: (name: string) => api.deleteCustomEntity(adminKey, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["custom-entities", adminKey] }),
    onError: (e) => toast.error("Could not delete the entity", e.message),
  });

  const items = data?.items ?? [];

  return (
    <div className="space-y-6">
      <Panel title="Add a custom entity" description="A regular expression the scanners match in addition to the built-in ones">
        <form
          noValidate
          className="space-y-4 p-4"
          onSubmit={handleSubmit((values) => createMut.mutate({ ...values, confidence: 0.85 }))}
        >
          <div className="grid gap-4 md:grid-cols-2">
            <FormField label="Name" htmlFor="entity-name">
              <Input
                id="entity-name"
                autoComplete="off"
                spellCheck={false}
                placeholder="project_mercury…"
                aria-invalid={!!errors.name}
                aria-describedby={errors.name ? "entity-name-error" : undefined}
                {...register("name")}
              />
              <FieldError id="entity-name-error" message={errors.name?.message} />
            </FormField>
            <FormField label="Pattern (regular expression)" htmlFor="entity-pattern">
              <Input
                id="entity-pattern"
                autoComplete="off"
                spellCheck={false}
                className="font-mono"
                placeholder="Project[ -]?Mercury…"
                aria-invalid={!!errors.pattern}
                aria-describedby={errors.pattern ? "entity-pattern-error" : undefined}
                {...register("pattern")}
              />
              <FieldError id="entity-pattern-error" message={errors.pattern?.message} />
            </FormField>
            <FormField label="Severity" htmlFor="entity-severity">
              <NativeSelect id="entity-severity" {...register("severity")}>
                {SEVERITIES.map((s) => (
                  <NativeSelectOption key={s} value={s}>
                    {s}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </FormField>
            <FormField label="Action" htmlFor="entity-action">
              <NativeSelect id="entity-action" {...register("action")}>
                {ACTIONS.map((a) => (
                  <NativeSelectOption key={a} value={a}>
                    {a}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </FormField>
          </div>
          <FormField label="Description (optional)" htmlFor="entity-description">
            <Input
              id="entity-description"
              autoComplete="off"
              placeholder="Confidential project code name…"
              {...register("description")}
            />
          </FormField>
          <Button type="submit" disabled={createMut.isPending}>
            <Plus />
            {createMut.isPending ? "Adding…" : "Add Entity"}
          </Button>
        </form>
      </Panel>

      <Panel title="Custom entities" aside={`${items.length} defined`}>
        {isLoading ? (
          <SkeletonRows rows={3} />
        ) : items.length === 0 ? (
          <EmptyState title="No custom entities" description="Add one above to detect values specific to your organisation." />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Pattern</TableHead>
                <TableHead>Severity</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((ce) => (
                <TableRow key={ce.name}>
                  <TableCell className="font-medium">
                    {ce.name}
                    {ce.description ? <p className="text-xs font-normal text-muted-foreground">{ce.description}</p> : null}
                  </TableCell>
                  <TableCell className="max-w-72 truncate font-mono text-xs" title={ce.pattern}>
                    {ce.pattern}
                  </TableCell>
                  <TableCell>
                    <SeverityBadge severity={ce.severity} />
                  </TableCell>
                  <TableCell>
                    <ActionBadge action={ce.action} />
                  </TableCell>
                  <TableCell className="text-right">
                    <ConfirmButton
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`Delete entity ${ce.name}`}
                      title={`Delete entity ${ce.name}?`}
                      description="The scanners stop matching this pattern."
                      onConfirm={() => deleteMut.mutate(ce.name)}
                      disabled={deleteMut.isPending}
                    >
                      <Trash2 />
                    </ConfirmButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Panel>
    </div>
  );
}
