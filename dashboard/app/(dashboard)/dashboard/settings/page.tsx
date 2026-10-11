"use client";

import { useEffect, useState } from "react";
import NextLink from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import {
  Button,
  InlineNotification,
  Link,
  NumberInput,
  PasswordInput,
  Select,
  SelectItem,
  SkeletonText,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TabList,
  TabPanel,
  TabPanels,
  Tabs,
  TextInput,
  Toggle,
} from "@carbon/react";
import { AsOf, asOfWords, failureWords, HealthDetailNotice, held } from "@/components/console/health";
import { uptimeText } from "@/components/console/names";
import { PageHead, Refused, ScrollTable, Section } from "@/components/console/page";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { BUILT_IN_ADMIN_KEY } from "@/lib/built-in-key";
import { HEALTH_KEY } from "@/hooks/useHealth";
import { api } from "@/lib/api";
import { permissionDenied } from "@/lib/api/fetch-core";
import type { SSOSettings } from "@/lib/api/client";
import { toast } from "@/lib/toast";
import { formatSince } from "@/lib/utils/format";
import { API_BASE, SETTINGS_TABS } from "./_constants";
import { useSettingsPage } from "./useSettingsPage";

type Page = ReturnType<typeof useSettingsPage>;

export default function SettingsPage() {
  const m = useSettingsPage();
  const index = Math.max(0, SETTINGS_TABS.findIndex((t) => t.id === m.tab));

  return (
    <>
      <PageHead
        title="Settings"
        lede={
          <>
            How this console reaches the proxy at <code>{API_BASE}</code>, and how that proxy is running.
          </>
        }
      />

      <div className="t-tabs">
        <Tabs selectedIndex={index} onChange={({ selectedIndex }) => m.setTab(SETTINGS_TABS[selectedIndex].id)}>
          <TabList aria-label="Settings">
            {SETTINGS_TABS.map((t) => (
              <Tab key={t.id}>{t.label}</Tab>
            ))}
          </TabList>
          <TabPanels>
            <TabPanel>{m.tab === "access" ? <Access m={m} /> : null}</TabPanel>
            <TabPanel>{m.tab === "retention" ? <Window m={m} /> : null}</TabPanel>
            <TabPanel>{m.tab === "providers" ? <Providers m={m} /> : null}</TabPanel>
            <TabPanel>{m.tab === "runtime" ? <Runtime m={m} /> : null}</TabPanel>
            <TabPanel>{m.tab === "sso" ? <SSO m={m} /> : null}</TabPanel>
          </TabPanels>
        </Tabs>
      </div>
    </>
  );
}

function Access({ m }: { m: Page }) {
  // The key in use may come from the build, with nothing stored here.
  const [inBrowser, setInBrowser] = useState(false);
  useEffect(() => {
    try {
      setInBrowser(!!window.localStorage.getItem(ADMIN_KEY_STORAGE));
    } catch {
      setInBrowser(false);
    }
  }, [m.saved]);
  const builtIn = !inBrowser && !!m.saved && m.saved === BUILT_IN_ADMIN_KEY;
  return (
    <Section id="admin-key" title="Admin key" note={inBrowser ? "One is stored in this browser" : builtIn ? "Using the one built into this page" : "None stored in this browser"} style={{ maxWidth: "44rem" }}>
      {builtIn ? (
        <InlineNotification
          kind="info"
          lowContrast
          hideCloseButton
          title="This key is built into the page, for local development."
          subtitle="It comes from NEXT_PUBLIC_ADMIN_KEY and anyone who can load this page can read it. A production build refuses it: there, each person enters a key here."
          style={{ marginBottom: "1rem", maxWidth: "none" }}
        />
      ) : null}
      <form
        className="t-form"
        onSubmit={(e) => {
          e.preventDefault();
          m.saveAdminKey();
        }}
      >
        <PasswordInput
          id="admin-key-input"
          labelText="Admin key"
          helperText="Sent as X-Tamga-Admin-Key with every call to the management API. It is kept only in this browser."
          autoComplete="off"
          spellCheck={false}
          value={m.draft}
          onChange={(e) => m.setDraft(e.target.value)}
        />
        <div className="t-buttons">
          <Button type="submit" kind="primary" size="md" disabled={m.draft === m.saved}>
            Save the key
          </Button>
        </div>
      </form>
      <p className="t-footnote">
        Applications use their own keys, not this one: see{" "}
        <Link as={NextLink} href="/dashboard/keys">
          API keys
        </Link>
        . Webhooks and other places alerts are sent are under{" "}
        <Link as={NextLink} href="/dashboard/integrations">
          Integrations
        </Link>
        .
      </p>
    </Section>
  );
}

function Window({ m }: { m: Page }) {
  // The field keeps what was typed, an empty field included, so it can be cleared and retyped.
  const days = Number(m.retention);
  const valid = m.retention !== "" && Number.isInteger(days) && days >= 1 && days <= 365;

  return (
    <Section id="window" title="How far back this browser looks" style={{ maxWidth: "44rem" }}>
      <form
        className="t-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (valid) m.saveRetention();
        }}
      >
        <NumberInput
          id="retention-days-input"
          label="Days"
          min={1}
          max={365}
          allowEmpty
          invalid={!valid}
          invalidText="Enter a whole number of days from 1 to 365."
          value={m.retention === "" ? "" : days}
          onChange={(_, { value }) => m.setRetention(String(value))}
          style={{ maxWidth: "12rem" }}
        />
        <div className="t-buttons">
          <Button type="submit" kind="primary" size="md" disabled={!valid}>
            Save
          </Button>
        </div>
      </form>
      <p className="t-footnote">
        This only limits what the filters of this console show in this browser. How long the proxy keeps events is set in the proxy&apos;s own
        configuration, not here.
      </p>
    </Section>
  );
}

const CIRCUIT: Record<string, { word: string; action?: "FAILING" | "STRUGGLING" }> = {
  closed: { word: "Taking traffic" },
  open: { word: "Cut off", action: "FAILING" },
  "half-open": { word: "Being tried again", action: "STRUGGLING" },
  half_open: { word: "Being tried again", action: "STRUGGLING" },
};

function Providers({ m }: { m: Page }) {
  const pools = m.health?.providers ?? [];
  const [pending, setPending] = useState<string | null>(null);
  // The reset the proxy last refused, so the row can say why beside its button.
  const [refused, setRefused] = useState<{ row: string; error: unknown } | null>(null);
  const qc = useQueryClient();

  async function reset(pool: string, endpoint: string) {
    setPending(`${pool}:${endpoint}`);
    setRefused(null);
    try {
      await api.resetUpstreamCircuit(m.saved, pool, endpoint);
      toast.success("Sending traffic again", `${pool}, ${endpoint}`);
      await qc.invalidateQueries({ queryKey: HEALTH_KEY });
    } catch (e) {
      if (permissionDenied(e)) setRefused({ row: `${pool}:${endpoint}`, error: e });
      else toast.error("Could not reset the endpoint", (e as Error).message);
    } finally {
      setPending(null);
    }
  }

  const { reading } = m;
  if (!m.health) {
    return reading.standing === "checking" ? (
      <p className="t-empty">The proxy has not answered yet.</p>
    ) : reading.standing === "unreachable" || reading.stale ? (
      <p className="t-empty">The proxy did not answer, so the state of its provider pools is not known.</p>
    ) : (
      <>
        <HealthDetailNotice state={reading.standing} />
        <p className="t-empty">The provider pools are shown once the proxy accepts an admin key from this browser.</p>
      </>
    );
  }

  if (pools.length === 0) {
    return (
      <p className="t-empty">
        No provider pool is defined. Add <code>providers.pools</code> to the policy and reload the proxy to see each upstream here.
      </p>
    );
  }

  return (
    <>
      {pools.map((pl, i) => (
        <Section
          key={pl.pool}
          id={`pool-${pl.pool}`}
          title={`${pl.pool} pool`}
          note={reading.stale ? asOfWords(reading.answeredAt, true) : `${pl.healthy_count} of ${pl.total_count} taking traffic`}
          style={i > 0 ? { marginTop: "2.5rem" } : undefined}
        >
          <ScrollTable className="t-table">
            <Table size="md" aria-label={`${pl.pool} pool`}>
              <TableHead>
                <TableRow>
                  <TableHeader>Endpoint</TableHeader>
                  <TableHeader>State</TableHeader>
                  <TableHeader className="t-right">Requests</TableHeader>
                  <TableHeader className="t-right">Succeeded</TableHeader>
                  <TableHeader>Last failure</TableHeader>
                  <TableHeader>
                    <span className="cds--visually-hidden">Reset</span>
                  </TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {pl.providers.map((p) => {
                  const state = CIRCUIT[(p.state || "").toLowerCase()] ?? { word: p.state || "Not known" };
                  const busy = pending === `${pl.pool}:${p.name}`;
                  return (
                    <TableRow key={p.name} data-stale={reading.stale ? "true" : undefined}>
                      <TableCell className="t-data">{p.name}</TableCell>
                      <TableCell>
                        {reading.stale ? (
                          // No mark: a mark says what the endpoint is doing now.
                          held(reading, state.word)
                        ) : state.action ? (
                          <span className="t-action" data-action={state.action}>
                            {state.word}
                          </span>
                        ) : (
                          state.word
                        )}
                      </TableCell>
                      <TableCell className="t-right">{p.requests_in_window ?? "None"}</TableCell>
                      <TableCell className="t-right">
                        {typeof p.success_rate_observed === "number" ? `${(p.success_rate_observed * 100).toFixed(1)}%` : "Not measured"}
                      </TableCell>
                      <TableCell>
                        {p.last_failure ? formatSince(p.last_failure) : "None"}
                        {p.failure_reason ? `, ${failureWords(p.failure_reason)}` : ""}
                      </TableCell>
                      <TableCell className="t-right">
                        <Button kind="ghost" size="sm" disabled={busy || !m.saved} onClick={() => void reset(pl.pool, p.name)}>
                          {busy ? "Resetting" : "Send traffic again"}
                        </Button>
                        {refused?.row === `${pl.pool}:${p.name}` ? <Refused error={refused.error} /> : null}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </ScrollTable>
        </Section>
      ))}
      <p className="t-footnote">An endpoint that is cut off receives no traffic. Reset it once the upstream is healthy again.</p>
    </>
  );
}

function Runtime({ m }: { m: Page }) {
  const { up, health, runtime, reading } = m;
  // Two reads feed this tab. A value is shown by the reading of the one it came from.
  const rt = runtime || m.runtimeReading.standing === "checking" ? m.runtimeReading : reading;
  const proxy = up?.proxy;
  const database = runtime?.database ?? health?.database;
  const uptime = runtime?.uptime_seconds ?? health?.uptime_seconds;
  const locked = reading.standing === "no-key" || reading.standing === "rejected";
  const list = (rows: [string, React.ReactNode][]) => (
    <dl className="t-facts" style={{ borderTop: 0 }}>
      {rows.map(([label, value]) => (
        <div key={label}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );

  return (
    <>
      <HealthDetailNotice state={reading.standing} />
      <AsOf reading={reading.stale ? reading : rt} />
      <div className="t-columns" data-even="true" style={{ marginTop: locked ? "1.5rem" : 0 }}>
        <Section id="transport" title="Transport and storage">
          {list([
            // Known now, whatever else is old: the check either came back or it did not.
            [
              "Proxy",
              reading.stale || reading.standing === "unreachable"
                ? "Unreachable"
                : reading.standing === "checking"
                  ? "Checking"
                  : proxy === "up"
                    ? "Up"
                    : (proxy ?? "Not known"),
            ],
            ["TLS", held(rt, !runtime ? undefined : runtime.tls_enabled ? "On" : "Off, plain HTTP")],
            ["Mutual TLS", held(rt, !runtime ? undefined : runtime.mtls_enabled ? "Required" : "Off")],
            ["Rate limit state", held(rt, !runtime ? undefined : runtime.redis_enabled ? "In Redis, shared between nodes" : "In memory, this node only")],
            ["Database", held(rt, database === "connected" ? "Connected" : database?.replace(/_/g, " "))],
          ])}
        </Section>
        <Section id="build" title="Build and policy">
          {list([
            ["Version", held(rt, runtime?.version)],
            ["Policy", held(rt, runtime?.policy_name)],
            ["Policy file", <span key="p" className="t-data">{held(rt, runtime?.policy_path ?? health?.policy_path)}</span>],
            ["Scanners", held(rt, runtime?.scanner_count ?? health?.scanner_count)],
            ["Running for", held(rt, typeof uptime === "number" ? uptimeText(uptime) : undefined)],
          ])}
        </Section>
      </div>
    </>
  );
}

function SSO({ m }: { m: Page }) {
  if (!m.saved) return <p className="t-empty">Add an admin key under Access to read the sign-in configuration.</p>;
  if (m.ssoLoading) return <SkeletonText paragraph lineCount={4} />;
  if (m.ssoError) return <p className="t-empty">The sign-in configuration could not be read.</p>;
  // The form starts from the loaded configuration, so it mounts only once that has arrived.
  return <SSOForm config={m.ssoConfig} onSave={m.saveSSO} />;
}

function SSOForm({ config, onSave }: { config: SSOSettings | undefined; onSave: (cfg: Partial<SSOSettings>) => Promise<void> }) {
  const [providerType, setProviderType] = useState(config?.provider_type ?? "");
  const [metadataUrl, setMetadataUrl] = useState(config?.metadata_url ?? "");
  const [domain, setDomain] = useState(config?.domain ?? "");
  const [enabled, setEnabled] = useState(config?.enabled ?? false);
  const [saving, setSaving] = useState(false);

  const save = async () => {
    try {
      setSaving(true);
      // The proxy replaces the whole configuration, so the attribute mapping this form does not show goes back with it.
      await onSave({ provider_type: providerType, metadata_url: metadataUrl, domain, enabled, attribute_mapping: config?.attribute_mapping });
      toast.success("Sign-in configuration saved");
    } catch (e) {
      toast.error("Could not save the sign-in configuration", e instanceof Error ? e.message : "Unknown error");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Section id="sso" title="Single sign-on" note="Stored, not used by any sign-in" style={{ maxWidth: "44rem" }}>
      <InlineNotification
        kind="info"
        lowContrast
        hideCloseButton
        title="No sign-in uses these settings yet."
        subtitle="The proxy stores them and reads them nowhere: nobody signs in over SAML or OpenID Connect, and saving this form admits or refuses no one. Today people sign in through GitHub OAuth, which is configured on the proxy, or use a key."
        style={{ marginTop: "1rem", maxWidth: "none" }}
      />
      <form
        className="t-form"
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        <Select id="sso-provider-type" labelText="Protocol" value={providerType} onChange={(e) => setProviderType(e.target.value)}>
          <SelectItem value="" text="None" />
          <SelectItem value="saml" text="SAML 2.0" />
          <SelectItem value="oidc" text="OpenID Connect" />
        </Select>
        <TextInput
          id="sso-metadata-url"
          labelText="Identity provider metadata URL"
          helperText="The SAML metadata document or the OIDC discovery URL of your identity provider."
          type="url"
          placeholder="https://idp.example.com/metadata"
          autoComplete="off"
          spellCheck={false}
          value={metadataUrl}
          onChange={(e) => setMetadataUrl(e.target.value)}
        />
        <TextInput
          id="sso-domain"
          labelText="Email domain"
          helperText="The domain whose addresses would sign in through the identity provider."
          placeholder="example.com"
          autoComplete="off"
          spellCheck={false}
          value={domain}
          onChange={(e) => setDomain(e.target.value)}
        />
        <Toggle id="sso-enabled" size="sm" labelText="Marked as required for this domain" labelA="No" labelB="Yes" toggled={enabled} onToggle={setEnabled} />
        <div className="t-buttons">
          <Button type="submit" kind="primary" size="md" disabled={saving}>
            {saving ? "Saving" : "Save"}
          </Button>
        </div>
      </form>
    </Section>
  );
}
