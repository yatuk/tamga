"use client";

import { useState } from "react";
import { Globe, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { toast } from "@/lib/toast";
import { type SSOSettings } from "@/lib/api/client";

type Props = {
  config: SSOSettings | undefined;
  loading: boolean;
  error: string | null;
  onSave: (cfg: Partial<SSOSettings>) => Promise<void>;
};

export function SettingsSSOSection({ config, loading, error, onSave }: Props) {
  const [providerType, setProviderType] = useState(config?.provider_type ?? "");
  const [metadataUrl, setMetadataUrl] = useState(config?.metadata_url ?? "");
  const [domain, setDomain] = useState(config?.domain ?? "");
  const [enabled, setEnabled] = useState(config?.enabled ?? false);
  const [saving, setSaving] = useState(false);

  if (loading) {
    return (
      <div>
        <TerminalFrame title="Enterprise SSO">
          <div className="space-y-3 p-3 animate-pulse">
            <div className="h-4 w-2/3 rounded-sm bg-zinc-200 dark:bg-zinc-800" />
            <div className="h-10 w-full rounded-sm bg-zinc-200 dark:bg-zinc-800" />
            <div className="h-10 w-full rounded-sm bg-zinc-200 dark:bg-zinc-800" />
            <div className="h-10 w-full rounded-sm bg-zinc-200 dark:bg-zinc-800" />
          </div>
        </TerminalFrame>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <TerminalFrame title="Enterprise SSO">
          <div className="space-y-3 p-3">
            <Badge className="rounded-sm border-red-500/30 bg-red-500/10 text-[10px] text-red-400">
              LOAD ERROR
            </Badge>
            <div className="text-xs text-zinc-600 dark:text-zinc-400">{error}</div>
          </div>
        </TerminalFrame>
      </div>
    );
  }

  const handleSave = async () => {
    try {
      setSaving(true);
      await onSave({
        provider_type: providerType,
        metadata_url: metadataUrl,
        domain,
        enabled,
      });
      toast.success("SSO settings saved", "Enterprise SSO configuration updated.");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Unknown error";
      toast.error("SSO save failed", msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <TerminalFrame
        title="Enterprise SSO"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            <Globe className="mr-1 inline h-3 w-3" />
            {enabled ? providerType.toUpperCase() : "DISABLED"}
          </span>
        }
      >
        <div className="space-y-3 p-3">
          <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
            SAML 2.0 or OpenID Connect (OIDC) enterprise SSO. Requires Clerk Enterprise plan.
          </div>

          {/* Provider Type */}
          <div>
            <label className="mb-1 block text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
              Provider Type
            </label>
            <select
              value={providerType}
              onChange={(e) => setProviderType(e.target.value)}
              className="h-10 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 text-sm text-zinc-900 dark:text-zinc-100 focus:border-emerald-500/40 focus:outline-none"
            >
              <option value="">None (Disabled)</option>
              <option value="saml">SAML 2.0</option>
              <option value="oidc">OpenID Connect (OIDC)</option>
            </select>
          </div>

          {/* Metadata URL */}
          <div>
            <label className="mb-1 block text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
              Metadata URL
            </label>
            <input
              type="url"
              value={metadataUrl}
              onChange={(e) => setMetadataUrl(e.target.value)}
              placeholder="https://idp.example.com/metadata"
              className="h-10 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 text-sm text-zinc-900 dark:text-zinc-100 focus:border-emerald-500/40 focus:outline-none"
            />
          </div>

          {/* Domain */}
          <div>
            <label className="mb-1 block text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
              Domain
            </label>
            <input
              type="text"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="example.com"
              className="h-10 w-full rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 text-sm text-zinc-900 dark:text-zinc-100 focus:border-emerald-500/40 focus:outline-none"
            />
          </div>

          {/* Enabled Toggle */}
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="sso-enabled"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className="h-4 w-4 rounded-sm border-zinc-300 dark:border-zinc-700"
            />
            <label htmlFor="sso-enabled" className="text-xs text-zinc-700 dark:text-zinc-300">
              Enable SSO for this domain
            </label>
          </div>

          {/* Status Chips */}
          <div className="flex flex-wrap gap-2">
            <Badge
              className={`rounded-sm border text-[10px] ${
                enabled
                  ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
                  : "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400"
              }`}
            >
              {enabled ? "ENABLED" : "DISABLED"}
            </Badge>
            <Badge className="rounded-sm border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-[10px] text-zinc-700 dark:text-zinc-300">
              {providerType ? providerType.toUpperCase() : "NONE"}
            </Badge>
          </div>

          {/* Save Button */}
          <Button
            className="h-9 cursor-pointer rounded-sm bg-emerald-600 text-white hover:bg-emerald-700"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? (
              <>
                <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> Saving...
              </>
            ) : (
              "Save SSO Configuration"
            )}
          </Button>
        </div>
      </TerminalFrame>
    </div>
  );
}
