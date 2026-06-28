"use client";

import { useEffect } from "react";
import dynamic from "next/dynamic";
import { useTranslation } from "@/lib/i18n";
import { Hero } from "@/app/(marketing)/_components/landing/Hero";
import { TTVTerminal } from "@/app/(marketing)/_components/landing/TTVTerminal";
import { DropInSnippet } from "@/app/(marketing)/_components/landing/DropInSnippet";
import { SiemEcosystem } from "@/app/(marketing)/_components/landing/SiemEcosystem";
import { ComplianceRow } from "@/app/(marketing)/_components/landing/ComplianceRow";
import { LiveDemo } from "@/app/(marketing)/_components/landing/LiveDemo";
import { TryTamgaLive } from "@/app/(marketing)/_components/landing/TryTamgaLive";
import { InteractivePolicySimulator } from "@/app/(marketing)/_components/landing/InteractivePolicySimulator";
import { CTAFooter } from "@/app/(marketing)/_components/landing/CTAFooter";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";
import { Pricing } from "@/app/(marketing)/_components/landing/Pricing";
import { BenchmarkRow } from "@/app/(marketing)/_components/landing/BenchmarkRow";

// Lazy-load OWASP — pulls @tremor/react (~150KB gzip)
const OwaspCoverage = dynamic(
  () => import("@/app/(marketing)/_components/landing/OwaspCoverage").then((m) => m.OwaspCoverage),
  {
    ssr: false,
    loading: () => (
      <div
        className="h-[320px] w-full animate-pulse rounded-sm border border-zinc-200 dark:border-zinc-800/80 bg-zinc-100 dark:bg-zinc-900/40"
        aria-hidden
      />
    ),
  },
);

export default function MarketingLandingPage() {
  const { lang } = useTranslation();
  useEffect(() => {
    document.documentElement.style.scrollBehavior = "smooth";
    return () => {
      document.documentElement.style.scrollBehavior = "auto";
    };
  }, []);

  return (
    <>
      <main lang={lang} className="w-full bg-white dark:bg-zinc-950">
        {/* 1. HERO — Value proposition */}
        <section className="py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <Hero />
          </div>
        </section>

        {/* 2. TTV TERMINAL — Deploy in 5 minutes */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-3xl px-6">
            <div className="mb-8 text-center">
              <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
                Deploy in minutes, not weeks
              </h2>
              <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
                One Docker Compose command. Zero architecture changes.
                Your existing LLM SDK calls route through Tamga transparently.
              </p>
            </div>
            <TTVTerminal />
          </div>
        </section>

        {/* 3. DROP-IN SNIPPET — Zero code rewrites */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-3xl px-6">
            <div className="mb-8 text-center">
              <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
                One line changes everything
              </h2>
              <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
                Change your <code className="text-[12px] bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 rounded-sm text-zinc-700 dark:text-zinc-300">base_url</code>.
                That&apos;s the only code modification required. No SDK wrappers, no middleware.
              </p>
            </div>
            <DropInSnippet />
          </div>
        </section>

        {/* 4. LIVE DEMO — Before/After proof */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <div className="mb-8 text-center">
              <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
                See the engine work
              </h2>
              <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
                Unprotected prompt vs. Tamga-protected request. PII redacted, injection blocked, audit logged.
              </p>
            </div>
            <LiveDemo />
          </div>
        </section>

        {/* 5. TRY TAMGA LIVE — Interactive sandbox */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <div className="mb-8 text-center">
              <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
                Try Tamga Live
              </h2>
              <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
                Paste a prompt, run the policy engine, and inspect the findings in real time.
              </p>
            </div>
            <div className="space-y-8">
              <InteractivePolicySimulator />
              <TryTamgaLive />
            </div>
          </div>
        </section>

        {/* 5.5. BENCHMARKS — Verified performance */}

        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-5xl px-6">
            <BenchmarkRow />
          </div>
        </section>

        {/* 6. OWASP MATRIX — Security coverage proof */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <div className="mb-8 text-center">
              <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
                OWASP LLM Top 10 Coverage
              </h2>
              <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
                Tamga detects and mitigates every category in the OWASP LLM Application Security framework.
              </p>
            </div>
            <OwaspCoverage />
          </div>
        </section>

        {/* 7. SIEM ECOSYSTEM — Plug into existing SOC */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <SiemEcosystem />
          </div>
        </section>

        {/* 8. COMPLIANCE ROW — Legal boxes checked */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <ComplianceRow />
          </div>
        </section>

        {/* 9. PRICING — Plans */}
        <section id="pricing" className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <Pricing />
          </div>
        </section>

        {/* 10. CTA — Final conversion */}
        <section className="border-t border-zinc-200 dark:border-zinc-800 py-16 sm:py-20">
          <div className="mx-auto max-w-7xl px-6">
            <CTAFooter />
          </div>
        </section>
      </main>
      <MarketingFooter />
    </>
  );
}
