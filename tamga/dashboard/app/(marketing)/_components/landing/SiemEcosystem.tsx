"use client";

const PLATFORMS = [
  { name: "Splunk HEC", format: "JSON/CEF", desc: "SIEM ingestion" },
  { name: "Microsoft Sentinel", format: "Log Analytics", desc: "Cloud-native SIEM" },
  { name: "IBM QRadar", format: "LEEF", desc: "Enterprise SOC" },
  { name: "Datadog", format: "JSON", desc: "Observability" },
  { name: "Slack", format: "Webhook", desc: "Instant alerts" },
  { name: "PagerDuty", format: "Events API v2", desc: "On-call escalation" },
  { name: "Generic Webhook", format: "JSON POST", desc: "Custom integration" },
  { name: "Syslog", format: "RFC 5424", desc: "Legacy systems" },
];

export function SiemEcosystem() {
  return (
    <div>
      <div className="mb-8 text-center">
        <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
          Plugs into your existing SOC
        </h2>
        <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400 max-w-2xl mx-auto">
          Tamga streams every security event to your SIEM, alerting, and logging infrastructure.
          No rip-and-replace — just add a webhook.
        </p>
      </div>

      {/* Grid */}
      <div className="grid gap-3 grid-cols-2 sm:grid-cols-3 lg:grid-cols-4">
        {PLATFORMS.map((p) => (
          <div
            key={p.name}
            className="group relative rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-4 transition-colors hover:border-zinc-300 dark:hover:border-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-900/80"
          >
            <div className="text-sm font-semibold text-zinc-700 dark:text-zinc-300 group-hover:text-zinc-900 dark:group-hover:text-zinc-100 transition-colors">
              {p.name}
            </div>
            <div className="mt-1 text-[10px] uppercase tracking-wider text-zinc-500">
              {p.format}
            </div>
            <div className="mt-0.5 text-[11px] text-zinc-500 dark:text-zinc-500">
              {p.desc}
            </div>
            {/* Subtle glow on hover */}
            <div className="absolute inset-0 rounded-sm opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none ring-1 ring-inset ring-zinc-300 dark:ring-zinc-700" />
          </div>
        ))}
      </div>
    </div>
  );
}
