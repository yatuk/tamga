import type { Metadata } from "next";
import { Pricing } from "@/app/(marketing)/_components/landing/Pricing";
import { MarketingFooter } from "@/app/(marketing)/_components/landing/MarketingFooter";

export const metadata: Metadata = {
  title: "Pricing — Tamga",
  description:
    "Community (free, self-hosted), Team ($25/dev/mo), Business ($500/mo), Enterprise (custom). Transparent per-tier LLM security pricing.",
};

export default function PricingPage() {
  return (
    <>
      <main className="mx-auto w-full max-w-7xl px-6 py-16">
        <Pricing />
      </main>
      <MarketingFooter />
    </>
  );
}
