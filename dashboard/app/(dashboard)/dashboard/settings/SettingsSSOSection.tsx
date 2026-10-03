"use client";

import { useState } from "react";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { AdminKeyRequired, ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import type { SSOSettings } from "@/lib/api/client";
import { toast } from "@/lib/toast";

type Props = {
  adminKey: string;
  config: SSOSettings | undefined;
  loading: boolean;
  error: unknown;
  onSave: (cfg: Partial<SSOSettings>) => Promise<void>;
};

export function SettingsSSOSection({ adminKey, config, loading, error, onSave }: Props) {
  if (!adminKey) {
    return (
      <Panel title="Enterprise SSO">
        <AdminKeyRequired />
      </Panel>
    );
  }
  if (loading) {
    return (
      <Panel title="Enterprise SSO">
        <SkeletonRows rows={4} />
      </Panel>
    );
  }
  if (error) {
    return (
      <Panel title="Enterprise SSO">
        <ErrorState title="Could not load the SSO configuration" error={error} />
      </Panel>
    );
  }
  // The form starts from the loaded configuration, so it mounts only once
  // that has arrived.
  return <SSOForm config={config} onSave={onSave} />;
}

function SSOForm({ config, onSave }: Pick<Props, "config" | "onSave">) {
  const [providerType, setProviderType] = useState(config?.provider_type ?? "");
  const [metadataUrl, setMetadataUrl] = useState(config?.metadata_url ?? "");
  const [domain, setDomain] = useState(config?.domain ?? "");
  const [enabled, setEnabled] = useState(config?.enabled ?? false);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    try {
      setSaving(true);
      await onSave({ provider_type: providerType, metadata_url: metadataUrl, domain, enabled });
      toast.success("SSO configuration saved");
    } catch (e) {
      toast.error("Could not save the SSO configuration", e instanceof Error ? e.message : "Unknown error");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Panel
      title="Enterprise SSO"
      description="SAML 2.0 or OpenID Connect sign-in for one email domain"
      aside={
        <StatusBadge tone={enabled && providerType ? "pass" : "neutral"}>
          {enabled && providerType ? `${providerType} enabled` : "Disabled"}
        </StatusBadge>
      }
    >
      <form
        className="max-w-xl space-y-4 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          void handleSave();
        }}
      >
        <FormField label="Protocol" htmlFor="sso-provider-type">
          <NativeSelect
            id="sso-provider-type"
            name="provider_type"
            value={providerType}
            onChange={(e) => setProviderType(e.target.value)}
          >
            <NativeSelectOption value="">None</NativeSelectOption>
            <NativeSelectOption value="saml">SAML 2.0</NativeSelectOption>
            <NativeSelectOption value="oidc">OpenID Connect (OIDC)</NativeSelectOption>
          </NativeSelect>
        </FormField>

        <FormField
          label="Identity provider metadata URL"
          htmlFor="sso-metadata-url"
          hint="The SAML metadata document or the OIDC discovery URL of your identity provider."
        >
          <Input
            id="sso-metadata-url"
            name="metadata_url"
            type="url"
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            value={metadataUrl}
            onChange={(e) => setMetadataUrl(e.target.value)}
            placeholder="https://idp.example.com/metadata…"
            className="font-mono"
          />
        </FormField>

        <FormField label="Email domain" htmlFor="sso-domain" hint="Users with an address on this domain sign in through SSO.">
          <Input
            id="sso-domain"
            name="domain"
            autoComplete="off"
            spellCheck={false}
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            placeholder="example.com…"
            className="font-mono"
          />
        </FormField>

        <div className="flex items-center gap-3">
          <Switch id="sso-enabled" checked={enabled} onCheckedChange={setEnabled} />
          <Label htmlFor="sso-enabled">Require SSO for this domain</Label>
        </div>

        <div className="flex flex-wrap items-center gap-3 border-t pt-4">
          <Button type="submit" disabled={saving}>
            {saving ? <Spinner /> : null}
            {saving ? "Saving…" : "Save Configuration"}
          </Button>
          <p className="text-xs text-muted-foreground">Enterprise SSO needs an identity plan that supports it.</p>
        </div>
      </form>
    </Panel>
  );
}
