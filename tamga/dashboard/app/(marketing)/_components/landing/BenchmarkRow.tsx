"use client";

const METRICS = [
  { label: "Scan Latency p95", value: "0.52 ms", detail: "single request, commodity hardware" },
  { label: "Scan Latency p99", value: "0.58 ms", detail: "worst-case deterministic path" },
  { label: "Precision", value: "96.9%", detail: "false positives block real traffic" },
  { label: "Total Overhead", value: "< 2 ms", detail: "under 2% of total request time" },
  { label: "Corpus", value: "309 prompts", detail: "public, reproducible, CI-gated" },
  { label: "Scanners", value: "7 inline", detail: "PII, secret, injection, jailbreak, moderation, competitor, custom" },
];

export function BenchmarkRow() {
  return (
    <div>
      <div className="mb-8 text-center">
        <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-emerald-500">
          Verified Performance
        </p>
        <h2 className="mt-2 text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
          Sub-millisecond scanning at wire speed
        </h2>
        <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400 max-w-2xl mx-auto">
          Every benchmark is reproducible from a single <code className="text-[12px] bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 rounded-sm text-zinc-700 dark:text-zinc-300">go run ./cmd/redteam</code> command
          against a public 309-prompt adversarial corpus. No cherry-picking, no marketing dataset.
        </p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {METRICS.map((m) => (
          <div
            key={m.label}
            className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-5"
          >
            <div className="font-mono text-[11px] uppercase tracking-[0.14em] text-zinc-500 dark:text-zinc-400">
              {m.label}
            </div>
            <div className="mt-2 text-2xl font-extrabold tracking-tight text-emerald-600 dark:text-emerald-400 font-mono">
              {m.value}
            </div>
            <div className="mt-1 text-xs text-zinc-500 dark:text-zinc-500">
              {m.detail}
            </div>
          </div>
        ))}
      </div>

      <div className="mt-6 text-center">
        <a
          href="https://github.com/yatuk/tamga/blob/dev/docs/benchmarks/README.md"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1.5 font-mono text-xs text-emerald-600 dark:text-emerald-400 hover:underline"
        >
          View full benchmark report →
        </a>
        <span className="mx-3 text-zinc-300 dark:text-zinc-700">|</span>
        <span className="font-mono text-[11px] text-zinc-500 dark:text-zinc-400">
          Precision 0.969 · Recall 0.484 · F1 0.646
        </span>
      </div>
    </div>
  );
}
