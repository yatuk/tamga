"use client";

import { useTranslation } from "@/lib/i18n";

type EvalRow = { category: string; total: number; precision: number; recall: number; f1: number };

const rows: EvalRow[] = [
  { category: "PII — TCKN", total: 42, precision: 0.98, recall: 0.95, f1: 0.96 },
  { category: "PII — Kredi kartı (Luhn+BIN)", total: 38, precision: 0.99, recall: 0.92, f1: 0.95 },
  { category: "PII — IBAN (TR)", total: 24, precision: 1.0, recall: 0.96, f1: 0.98 },
  { category: "PII — E-posta", total: 31, precision: 0.97, recall: 0.97, f1: 0.97 },
  { category: "Jailbreak — override", total: 18, precision: 0.94, recall: 0.94, f1: 0.94 },
  { category: "Jailbreak — many-shot", total: 12, precision: 1.0, recall: 0.83, f1: 0.91 },
  { category: "Jailbreak — base64/hex obfuscation", total: 14, precision: 0.93, recall: 0.86, f1: 0.89 },
  { category: "Jailbreak — TR rol ele geçirme", total: 16, precision: 0.94, recall: 0.88, f1: 0.91 },
  { category: "Secret — AWS / OpenAI / GitHub", total: 22, precision: 1.0, recall: 1.0, f1: 1.0 },
  { category: "Indirect — Canary token", total: 10, precision: 1.0, recall: 1.0, f1: 1.0 },
];

const overall = rows.reduce(
  (acc, r) => ({
    total: acc.total + r.total,
    precision: acc.precision + r.precision * r.total,
    recall: acc.recall + r.recall * r.total,
    f1: acc.f1 + r.f1 * r.total,
  }),
  { total: 0, precision: 0, recall: 0, f1: 0 },
);
const avgPrec = (overall.precision / overall.total).toFixed(3);
const avgRec = (overall.recall / overall.total).toFixed(3);
const avgF1 = (overall.f1 / overall.total).toFixed(3);

export default function EvalsPage() {
  const { t } = useTranslation();
  return (
    <main className="mx-auto w-full max-w-5xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
        {t("evals.eyebrow")}
      </p>
      <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
        {t("evals.title")}
      </h1>
      <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        {t("evals.lede")}
      </p>

      <div className="mt-6 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-4 text-sm text-zinc-700 dark:text-zinc-300">
        <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-red-400">
          Reproducible benchmark
        </div>
        <p className="mt-2 leading-6">
          Full corpus + per-category table + scan-latency percentiles are published as JSON and
          Markdown in the repository. Anyone can rerun the same command and diff the numbers.
        </p>
        <div className="mt-3 flex flex-wrap gap-3 font-mono text-[11px]">
          <a
            href="https://github.com/yatuk/tamga/blob/main/tamga/docs/benchmarks/redteam_latest.json"
            target="_blank"
            rel="noreferrer"
            className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-zinc-800 dark:text-zinc-200 hover:border-red-500/50 hover:text-white"
          >
            redteam_latest.json
          </a>
          <a
            href="https://github.com/yatuk/tamga/blob/main/tamga/docs/benchmarks/README.md"
            target="_blank"
            rel="noreferrer"
            className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-zinc-800 dark:text-zinc-200 hover:border-red-500/50 hover:text-white"
          >
            Methodology
          </a>
          <code className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-black px-2 py-1 text-zinc-600 dark:text-zinc-400">
            make redteam-report
          </code>
        </div>
      </div>

      <div className="mt-8 grid gap-3 md:grid-cols-3">
        <Metric label="Ortalama precision" value={avgPrec} />
        <Metric label="Ortalama recall" value={avgRec} />
        <Metric label="Ortalama F1" value={avgF1} />
      </div>

      <div className="mt-10 overflow-x-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60">
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/70 font-mono text-[11px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
              <th className="px-4 py-3 text-left">Kategori</th>
              <th className="px-4 py-3 text-right">Örnek</th>
              <th className="px-4 py-3 text-right">Precision</th>
              <th className="px-4 py-3 text-right">Recall</th>
              <th className="px-4 py-3 text-right">F1</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.category} className="border-b border-zinc-900">
                <td className="px-4 py-3 text-zinc-700 dark:text-zinc-300">{row.category}</td>
                <td className="px-4 py-3 text-right font-mono text-zinc-600 dark:text-zinc-400">{row.total}</td>
                <Cell value={row.precision} />
                <Cell value={row.recall} />
                <Cell value={row.f1} />
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </main>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-sm border border-emerald-500/40 bg-emerald-500/5 p-4">
      <div className="font-mono text-[10px] uppercase tracking-wide text-emerald-300">{label}</div>
      <div className="mt-2 text-3xl font-bold tracking-tight text-emerald-200">{value}</div>
    </div>
  );
}

function Cell({ value }: { value: number }) {
  const pct = value * 100;
  const color = pct >= 95 ? "text-emerald-300" : pct >= 85 ? "text-amber-300" : "text-red-300";
  return (
    <td className={`px-4 py-3 text-right font-mono ${color}`}>{value.toFixed(3)}</td>
  );
}
