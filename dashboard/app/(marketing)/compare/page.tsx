"use client";

import { useTranslation } from "@/lib/i18n";

type Row = { feature: string; tamga: string; lakera: string; nemo: string; promptarmor: string };

const rows: Row[] = [
  { feature: "Türkçe PII (TCKN/IBAN/VKN)", tamga: "Native", lakera: "Yok", nemo: "Custom", promptarmor: "Kısmi" },
  { feature: "Checksum (Luhn + TCKN MERNIS)", tamga: "Var", lakera: "Sadece Luhn", nemo: "Yok", promptarmor: "Kısmi" },
  { feature: "Türkçe jailbreak korpusu", tamga: "Var", lakera: "Yok", nemo: "Yok", promptarmor: "Yok" },
  { feature: "KVKK veri yerleşimi (TR)", tamga: "Var", lakera: "Yok", nemo: "Kendin kur", promptarmor: "Yok" },
  { feature: "Inline proxy (OpenAI-compat)", tamga: "Var", lakera: "Var", nemo: "Yok", promptarmor: "Var" },
  { feature: "Shadow ML sidecar (UDS + Piiranha)", tamga: "Var", lakera: "Kısmi", nemo: "Yok", promptarmor: "Yok" },
  { feature: "Aho-Corasick DFA (sub-ms)", tamga: "Var", lakera: "Kısmi", nemo: "Yok", promptarmor: "Kısmi" },
  { feature: "Policy versioning + rollback", tamga: "Var", lakera: "Yok", nemo: "Yok", promptarmor: "Kısmi" },
  { feature: "İki-kişi onaylı değişiklik", tamga: "Var", lakera: "Yok", nemo: "Yok", promptarmor: "Yok" },
  { feature: "SCIM 2.0 provisioning", tamga: "Var", lakera: "Var", nemo: "Yok", promptarmor: "Var" },
  { feature: "Audit hash-chain", tamga: "Var", lakera: "Yok", nemo: "Yok", promptarmor: "Yok" },
  { feature: "PagerDuty / Opsgenie / ServiceNow presetleri", tamga: "Var", lakera: "Kısmi", nemo: "Yok", promptarmor: "Kısmi" },
  { feature: "Self-host (Helm + Terraform)", tamga: "Var", lakera: "Yok", nemo: "Var", promptarmor: "Yok" },
  { feature: "Canary token deteksiyonu", tamga: "Var", lakera: "Yok", nemo: "Yok", promptarmor: "Var" },
  { feature: "Reproducible benchmark (public JSON)", tamga: "Var", lakera: "Yok", nemo: "Yok", promptarmor: "Yok" },
  { feature: "Fiyat (1M istek ≈ tahmini)", tamga: "$0.30", lakera: "$2.00+", nemo: "Infra", promptarmor: "$1.50+" },
];

export default function ComparePage() {
  const { t } = useTranslation();
  return (
    <main className="mx-auto w-full max-w-6xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
        {t("compare.eyebrow")}
      </p>
      <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
        {t("compare.title")}
      </h1>
      <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        {t("compare.lede")}{" "}
        <a className="text-red-400 underline-offset-4 hover:underline" href="mailto:hello@tamga.io">
          hello@tamga.io
        </a>
        .
      </p>

      <div className="mt-10 overflow-x-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60">
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/70 font-mono text-[11px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
              <th className="px-4 py-3 text-left">Özellik</th>
              <th className="px-4 py-3 text-left text-red-400">Tamga</th>
              <th className="px-4 py-3 text-left">Lakera Guard</th>
              <th className="px-4 py-3 text-left">NeMo Guardrails</th>
              <th className="px-4 py-3 text-left">PromptArmor</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, idx) => (
              <tr
                key={idx}
                className="border-b border-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900/40"
              >
                <td className="px-4 py-3 font-medium">{row.feature}</td>
                <Cell value={row.tamga} highlight />
                <Cell value={row.lakera} />
                <Cell value={row.nemo} />
                <Cell value={row.promptarmor} />
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className="mt-6 text-xs text-zinc-500 dark:text-zinc-400">
        *Fiyatlar 1 milyon scan için bağımsız tahminlerdir. Kurumsal lisans teklifi
        için sales@tamga.io.
      </p>
    </main>
  );
}

function Cell({ value, highlight = false }: { value: string; highlight?: boolean }) {
  const bad = value === "Yok";
  const good = value === "Var" || value === "Native";
  const partial = value === "Kısmi" || value.startsWith("Sadece") || value === "Custom" || value === "Kendin kur" || value === "Infra";
  return (
    <td className={`px-4 py-3 font-mono text-xs ${highlight ? "text-red-300" : ""}`}>
      <span
        className={`rounded-sm px-2 py-0.5 ${
          good
            ? "bg-emerald-500/10 text-emerald-300"
            : bad
              ? "bg-red-500/10 text-red-300"
              : partial
                ? "bg-amber-500/10 text-amber-300"
                : "bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400"
        }`}
      >
        {value}
      </span>
    </td>
  );
}
