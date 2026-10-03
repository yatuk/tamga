"use client";

import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Props = {
  retention: string;
  setRetention: (v: string) => void;
  saveRetention: () => void;
};

export function SettingsRetentionSection({ retention, setRetention, saveRetention }: Props) {
  return (
    <Panel title="Dashboard window" description="How far back this browser looks">
      <form
        className="flex flex-wrap items-end gap-3 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          saveRetention();
        }}
      >
        <FormField label="Days" htmlFor="retention-days-input">
          <Input
            id="retention-days-input"
            name="retention-days"
            type="number"
            inputMode="numeric"
            min={1}
            max={365}
            value={retention}
            onChange={(e) => setRetention(e.target.value)}
            className="w-28 font-mono"
          />
        </FormField>
        <Button type="submit">Save</Button>
      </form>
      <p className="border-t px-4 py-3 text-xs text-muted-foreground">
        This only limits what the dashboard filters show in this browser. How long the proxy keeps events is set in the
        proxy configuration, not here.
      </p>
    </Panel>
  );
}
