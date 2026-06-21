import type { Metadata } from "next";
import Link from "next/link";
import {
  ArrowUpRight,
  Brain,
  Cpu,
  FileText,
  Gauge,
  ShieldCheck,
} from "lucide-react";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";

export const metadata: Metadata = {
  title: "Whitepaper — Tamga Hybrid Detection Engine",
  description:
    "High-Performance Hybrid PCI/PII Detection Engine for Inline LLM Proxies — mimari notlar, latans bütçesi ve reproducible benchmark.",
};

// The whitepaper page renders the public-facing outline of the
// architectural memo we circulate with design partners. The full PDF
// is served as a static asset out of the repo once it is rendered;
// until then we ship the canonical outline + the reproducible
// benchmark + a mailto for long-form. We intentionally keep it
// Turkish-first to match our go-to-market audience.

const SECTIONS = [
  {
    icon: Gauge,
    title: "Latans bütçesi",
    body: "Inline hat üzerinde 5 ms katı tavan. Python NLP pipeline'ları hot path'te yasak; Go DFA + SIMD ile 6 GB/s'in üstünde tarama.",
  },
  {
    icon: Cpu,
    title: "Deterministik tarama",
    body: "Aho-Corasick tabanlı DFA (coregx/ahocorasick profili), SIMD öncül filtre, Luhn + BIN/IIN doğrulama, MERNIS TCKN checksum.",
  },
  {
    icon: ShieldCheck,
    title: "Obfuscation & evasion",
    body: "Unicode normalizasyonu (NFC/NFD), Türkçe diakritik temizliği, word-to-number dönüşümü (EN + TR), tokenless contextual proximity.",
  },
  {
    icon: Brain,
    title: "Shadow ML yönlendirici",
    body: "UDS + FastAPI sidecar, Piiranha transformer modeli (opt-in), feedback JSONL ve insan-onaylı pattern promotion.",
  },
  {
    icon: FileText,
    title: "Confidence matrix",
    body: "PASS / LOG / REDACT / BLOCK aksiyonlarını format + algoritmik + BIN + context signal'larının ağırlıklı toplamına bağlar.",
  },
];

export default function WhitepaperPage() {
  return (
    <>
      <main className="mx-auto w-full max-w-4xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
        <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
          Whitepaper · v0.8
        </p>
        <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
          High-Performance Hybrid PCI/PII Detection Engine for Inline LLM Proxies
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
          Bu teknik not, Tamga&apos;nın inline-hat tarayıcı mimarisini ve Shadow
          ML geribesleme döngüsünü özetler. Tüm benchmark verileri{" "}
          <a
            className="text-red-400 hover:underline"
            href="https://github.com/yatuk/tamga/tree/dev/tamga/docs/benchmarks"
            target="_blank"
            rel="noreferrer"
          >
            GitHub&apos;daki docs/benchmarks dizininde
          </a>{" "}
          halka açık ve <code className="rounded-sm bg-zinc-100 dark:bg-zinc-900 px-1 font-mono text-[11px]">make redteam-report</code> ile yerelinizde yeniden üretilebilir.
        </p>

        <div className="mt-10 grid gap-4 sm:grid-cols-2">
          {SECTIONS.map(({ icon: Icon, title, body }) => (
            <section
              key={title}
              className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-5"
            >
              <div className="flex items-center gap-2 font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
                <Icon className="h-3.5 w-3.5" aria-hidden />
                {title}
              </div>
              <p className="mt-2 text-sm leading-6 text-zinc-700 dark:text-zinc-300">{body}</p>
            </section>
          ))}
        </div>

        <section className="mt-10 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-6">
          <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
            Reproducible benchmark
          </div>
          <p className="mt-2 text-sm text-zinc-700 dark:text-zinc-300">
            Tamga red-team benchmark&apos;i Tamga&apos;nın kendi tarayıcısı
            üzerinde çalışır, toplam + kategori bazlı precision/recall/F1 ve
            P50/P95/P99/Max tarama gecikmelerini JSON olarak yayınlar.
          </p>
          <pre className="mt-3 overflow-x-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 font-mono text-[11px] text-zinc-700 dark:text-zinc-300">
{`# proxy kökünden
make redteam-report   # tamga/docs/benchmarks/redteam_latest.json`}
          </pre>
          <div className="mt-4 flex flex-wrap gap-2 text-xs">
            <a
              href="https://github.com/yatuk/tamga/blob/dev/tamga/docs/benchmarks/redteam_latest.json"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              redteam_latest.json
              <ArrowUpRight className="h-3 w-3" aria-hidden />
            </a>
            <a
              href="https://github.com/yatuk/tamga/blob/dev/tamga/docs/benchmarks/README.md"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              Methodology (README.md)
              <ArrowUpRight className="h-3 w-3" aria-hidden />
            </a>
            <Link
              href="/evals"
              className="inline-flex items-center gap-1 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-2 py-1 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              Live eval tablosu
              <ArrowUpRight className="h-3 w-3" aria-hidden />
            </Link>
          </div>
        </section>

        <section className="mt-10 rounded-sm border border-red-500/30 bg-red-500/5 p-6">
          <div className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
            Tam sürüm PDF
          </div>
          <h3 className="mt-2 text-lg font-semibold text-white">
            Detaylı teknik notu PDF olarak mı istiyorsunuz?
          </h3>
          <p className="mt-2 text-sm text-zinc-600 dark:text-zinc-400">
            Tam sürüm (36 sayfa, mimari çizimler + latans bütçesi detayları)
            yalnızca kurumsal talebe cevaben paylaşılır. Satış ekibine yazın,
            NDA altında 24 saat içinde paylaşırız.
          </p>
          <div className="mt-4 flex flex-wrap gap-3">
            <a
              href="mailto:sales@tamga.dev?subject=Tamga%20whitepaper%20request"
              className="inline-flex items-center gap-2 rounded-sm bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700"
            >
              Whitepaper iste
              <ArrowUpRight className="h-4 w-4" aria-hidden />
            </a>
            <Link
              href="/contact"
              className="inline-flex items-center gap-2 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-4 py-2 text-sm text-zinc-800 dark:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              Mimari oturum
            </Link>
          </div>
        </section>
      </main>
      <MarketingFooter />
    </>
  );
}
