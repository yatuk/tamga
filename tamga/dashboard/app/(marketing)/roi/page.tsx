"use client";

import { useState } from "react";
import { useTranslation } from "@/lib/i18n";

export default function ROIPage() {
  const { t } = useTranslation();
  const [volume, setVolume] = useState(1_000_000);
  const [breachCost, setBreachCost] = useState(500_000);
  const [breachProb, setBreachProb] = useState(5);
  const [cacheHitRate, setCacheHitRate] = useState(20);
  const [avgLLMCost, setAvgLLMCost] = useState(0.002);

  const tamgaCost = Math.round(volume * 0.0003);
  const breachExpected = Math.round((breachCost * breachProb) / 100);
  const cacheSavings = Math.round((volume * avgLLMCost * cacheHitRate) / 100);
  const totalSaving = breachExpected + cacheSavings - tamgaCost;
  const roi = tamgaCost > 0 ? Math.round((totalSaving / tamgaCost) * 100) : 0;

  return (
    <main className="mx-auto w-full max-w-5xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
        {t("roi.eyebrow")}
      </p>
      <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
        {t("roi.title")}
      </h1>
      <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        {t("roi.lede")}
      </p>

      <section className="mt-10 grid gap-6 md:grid-cols-2">
        <Panel title="Girdi parametreleri">
          <Slider
            label="Aylık LLM istek adedi"
            value={volume}
            min={100_000}
            max={50_000_000}
            step={100_000}
            format={(v) => v.toLocaleString("tr-TR")}
            onChange={setVolume}
          />
          <Slider
            label="Tek sızıntı cezası (USD)"
            value={breachCost}
            min={50_000}
            max={5_000_000}
            step={50_000}
            format={(v) => `$${v.toLocaleString("en-US")}`}
            onChange={setBreachCost}
          />
          <Slider
            label="Yıllık sızıntı olasılığı (%)"
            value={breachProb}
            min={1}
            max={30}
            step={1}
            format={(v) => `${v}%`}
            onChange={setBreachProb}
          />
          <Slider
            label="Cache isabet oranı (%)"
            value={cacheHitRate}
            min={0}
            max={60}
            step={1}
            format={(v) => `${v}%`}
            onChange={setCacheHitRate}
          />
          <Slider
            label="Ortalama LLM maliyeti/istek (USD)"
            value={avgLLMCost}
            min={0.0005}
            max={0.02}
            step={0.0001}
            format={(v) => `$${v.toFixed(4)}`}
            onChange={setAvgLLMCost}
          />
        </Panel>

        <Panel title="Tahmini yıllık etki">
          <Row label="Beklenen sızıntı maliyeti (olmasaydı)" value={`$${breachExpected.toLocaleString()}`} />
          <Row label="Cache ile tasarruf" value={`$${cacheSavings.toLocaleString()}`} />
          <Row label="Tamga maliyeti" value={`-$${tamgaCost.toLocaleString()}`} />
          <div className="mt-4 rounded-sm border border-emerald-500/40 bg-emerald-500/10 p-4">
            <div className="font-mono text-[11px] uppercase tracking-wide text-emerald-300">
              Net tasarruf
            </div>
            <div className="mt-1 text-3xl font-bold tracking-tight text-emerald-200">
              ${totalSaving.toLocaleString()}
            </div>
            <div className="mt-1 font-mono text-xs text-emerald-400">
              ROI ≈ {roi}% ({(roi / 100).toFixed(1)}x)
            </div>
          </div>
        </Panel>
      </section>

      <p className="mt-8 text-xs text-zinc-500 dark:text-zinc-400">
        Hesaplama: breach_expected = cost × probability; cache_savings = volume × avg_cost × hit%;
        tamga_cost = volume × $0.0003. Değerler temsilidir.
      </p>
    </main>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-6">
      <h2 className="mb-4 font-mono text-[11px] uppercase tracking-[0.2em] text-zinc-600 dark:text-zinc-400">
        {title}
      </h2>
      <div className="space-y-4">{children}</div>
    </div>
  );
}

function Slider({
  label,
  value,
  min,
  max,
  step,
  format,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  format: (v: number) => string;
  onChange: (v: number) => void;
}) {
  return (
    <div>
      <div className="mb-1 flex justify-between text-xs">
        <span className="text-zinc-600 dark:text-zinc-400">{label}</span>
        <span className="font-mono text-zinc-800 dark:text-zinc-200">{format(value)}</span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(parseFloat(e.target.value))}
        className="w-full accent-red-500"
      />
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between border-b border-zinc-900 py-2 text-sm">
      <span className="text-zinc-600 dark:text-zinc-400">{label}</span>
      <span className="font-mono text-zinc-900 dark:text-zinc-100">{value}</span>
    </div>
  );
}
