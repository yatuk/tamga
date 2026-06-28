"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { AnimatePresence, motion, useInView, useReducedMotion } from "framer-motion";
import { ArrowRight, Check, Sparkles, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

// ── Plan data ──────────────────────────────────────────────────────────────────

interface Plan {
  name: string;
  tagline: string;
  price: { monthly: number; yearly: number };
  priceUnit?: string;
  customPrice?: string;
  description: string;
  features: string[];
  limits: string[];
  cta: string;
  ctaVariant: "solid" | "outline";
  ctaHref?: string;
  highlight: boolean;
  badge?: string;
}

const PLANS: Plan[] = [
  {
    name: "Community",
    tagline: "Self-hosted, open source",
    price: { monthly: 0, yearly: 0 },
    description: "For individuals and small teams getting started with LLM security.",
    features: [
      "Unlimited self-hosted requests",
      "PII + Secret + Injection scanners",
      "YAML policy engine",
      "Dashboard (single tenant)",
      "Community support (GitHub)",
      "7/10 OWASP LLM coverage",
    ],
    limits: ["No managed cloud", "No SSO/SAML", "No SLA"],
    cta: "Clone on GitHub",
    ctaVariant: "outline",
    highlight: false,
  },
  {
    name: "Team",
    tagline: "Managed cloud for teams",
    price: { monthly: 25, yearly: 20 },
    priceUnit: "per developer / month",
    description: "Managed Tamga cloud for development teams up to 50 engineers.",
    features: [
      "Everything in Community",
      "Managed cloud (EU/US regions)",
      "10M requests / month",
      "Custom entity patterns",
      "Slack + email alerts",
      "Email support (24h SLA)",
      "14-day audit log retention",
    ],
    limits: ["No SSO/SAML", "No custom contract"],
    cta: "Start free trial",
    ctaVariant: "solid",
    highlight: true,
    badge: "Most popular",
  },
  {
    name: "Business",
    tagline: "Production workloads",
    price: { monthly: 500, yearly: 400 },
    priceUnit: "per month, starting at",
    description: "For production AI workloads with compliance and SLA requirements.",
    features: [
      "Everything in Team",
      "Unlimited requests",
      "SSO/SAML (Okta, Entra)",
      "90-day audit log retention",
      "Priority support (4h SLA)",
      "Quarterly security review",
      "Dedicated Slack channel",
      "Custom MSA/DPA",
    ],
    limits: [],
    cta: "Contact sales",
    ctaVariant: "outline",
    ctaHref: "/contact?plan=business",
    highlight: false,
  },
  {
    name: "Enterprise",
    tagline: "Regulated industries",
    price: { monthly: 0, yearly: 0 },
    customPrice: "Custom",
    description: "Air-gapped deployment, custom compliance, dedicated engineering.",
    features: [
      "Everything in Business",
      "Air-gapped / on-premise deployment",
      "BYOK encryption",
      "Custom compliance frameworks",
      "Dedicated account engineer",
      "1-year+ audit retention",
      "99.99% uptime SLA",
      "Custom integration support",
    ],
    limits: [],
    cta: "Build your quote",
    ctaVariant: "outline",
    ctaHref: "/contact?plan=enterprise",
    highlight: false,
  },
];

// ── Price Display ──────────────────────────────────────────────────────────────

function PriceDisplay({
  plan,
  billing,
}: {
  plan: Plan;
  billing: "monthly" | "yearly";
}) {
  if (plan.customPrice) {
    return (
      <div className="font-mono text-3xl font-semibold text-zinc-900 dark:text-zinc-100">
        {plan.customPrice}
      </div>
    );
  }

  const price = billing === "monthly" ? plan.price.monthly : plan.price.yearly;

  return (
    <div className="flex items-baseline gap-0.5">
      <span className="font-mono text-lg text-zinc-500 dark:text-zinc-400">$</span>
      <AnimatePresence mode="wait">
        <motion.span
          key={`${billing}-${price}`}
          className="font-mono text-3xl font-semibold tabular-nums text-zinc-900 dark:text-zinc-100"
          initial={{ opacity: 0, y: -8, filter: "blur(2px)" }}
          animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
          exit={{ opacity: 0, y: 8, filter: "blur(2px)" }}
          transition={{ duration: 0.2, ease: "easeOut" }}
        >
          {price}
        </motion.span>
      </AnimatePresence>
    </div>
  );
}

// ── Plan Card ──────────────────────────────────────────────────────────────────

function PlanCard({
  plan,
  billing,
  visible,
  index,
}: {
  plan: Plan;
  billing: "monthly" | "yearly";
  visible: boolean;
  index: number;
}) {
  const reduce = useReducedMotion();

  const borderClass = plan.highlight
    ? "border-emerald-500/40 hover:border-emerald-500/60"
    : "border-zinc-200 dark:border-zinc-800 hover:border-zinc-700";

  const glowClass = plan.highlight
    ? "hover:shadow-[0_0_32px_-8px_rgba(16,185,129,0.2)]"
    : "hover:shadow-[0_0_24px_-8px_rgba(0,0,0,0.4)]";

  const bgClass = plan.highlight ? "bg-emerald-500/[0.03]" : "bg-white dark:bg-zinc-950";

  return (
    <motion.div
      className="h-full"
      initial={reduce ? {} : { opacity: 0, y: 20, scale: 0.97 }}
      animate={visible ? { opacity: 1, y: 0, scale: 1 } : {}}
      transition={{
        duration: 0.45,
        delay: reduce ? 0 : 0.1 + index * 0.08,
        ease: [0.22, 0.61, 0.36, 1],
      }}
    >
      <Card
        className={`group relative flex h-full flex-col rounded-sm border transition-all duration-300 ${borderClass} ${glowClass} ${bgClass}`}
      >
        {/* Highlight glow ring */}
        {plan.highlight && (
          <div
            className="pointer-events-none absolute inset-0 rounded-sm opacity-0 transition-opacity duration-300 group-hover:opacity-100"
            style={{ boxShadow: "inset 0 0 1px 0 rgba(16,185,129,0.4)" }}
            aria-hidden
          />
        )}

        <CardHeader className="space-y-2">
          <div className="flex items-center justify-between">
            <CardTitle className="font-mono text-lg text-zinc-900 dark:text-zinc-100">{plan.name}</CardTitle>
            {plan.badge && (
              <Badge className="rounded-sm border-emerald-500/30 bg-emerald-500/10 text-[10px] text-emerald-400">
                <Sparkles className="mr-1 h-2.5 w-2.5" />
                {plan.badge}
              </Badge>
            )}
          </div>
          <CardDescription className="font-mono text-[11px] text-zinc-500 dark:text-zinc-400">
            {plan.tagline}
          </CardDescription>

          {/* Price */}
          <div>
            <PriceDisplay plan={plan} billing={billing} />
            <div className="mt-0.5 font-mono text-[10px] text-zinc-600 dark:text-zinc-400">
              {plan.customPrice
                ? "tailored to your requirements"
                : plan.priceUnit || "per month"}
              {!plan.customPrice && billing === "yearly" && plan.price.yearly > 0 && (
                <span className="ml-1 text-emerald-600">— save 20%</span>
              )}
            </div>
          </div>
          <p className="text-sm leading-relaxed text-zinc-600 dark:text-zinc-400">{plan.description}</p>
        </CardHeader>

        <CardContent className="flex flex-1 flex-col justify-between gap-3">
          {/* Features */}
          <ul className="space-y-1.5">
            {plan.features.map((feature) => (
              <li key={feature} className="flex items-start gap-2">
                <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
                <span className="text-[13px] text-zinc-700 dark:text-zinc-300">{feature}</span>
              </li>
            ))}
          </ul>

          {/* Limitations */}
          {plan.limits.length > 0 && (
            <ul className="space-y-1.5 border-t border-zinc-200 dark:border-zinc-800/60 pt-3">
              {plan.limits.map((limit) => (
                <li key={limit} className="flex items-start gap-2">
                  <X className="mt-0.5 h-3.5 w-3.5 shrink-0 text-zinc-600 dark:text-zinc-400" />
                  <span className="text-[13px] text-zinc-500 dark:text-zinc-400">{limit}</span>
                </li>
              ))}
            </ul>
          )}

          {/* CTA */}
          {plan.ctaHref ? (
            <Link
              href={plan.ctaHref}
              className={`mt-2 inline-flex cursor-pointer items-center justify-center gap-2 rounded-sm px-4 py-2.5 font-mono text-xs font-medium transition-all duration-200 ${
                plan.ctaVariant === "solid"
                  ? "bg-emerald-600 text-white hover:bg-emerald-500 hover:shadow-[0_0_16px_-4px_rgba(16,185,129,0.3)]"
                  : plan.name === "Enterprise"
                    ? "border border-zinc-600 bg-zinc-200 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 hover:border-zinc-500 hover:bg-zinc-700"
                    : "border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-800 dark:text-zinc-200 hover:border-red-500 hover:bg-red-600 hover:text-white"
              }`}
            >
              {plan.cta}
              <ArrowRight className="h-3 w-3 transition-transform duration-200 group-hover:translate-x-0.5" />
            </Link>
          ) : (
            <Button
              className={`mt-2 cursor-pointer rounded-sm font-mono text-xs ${
                plan.ctaVariant === "solid"
                  ? "bg-emerald-600 text-white hover:bg-emerald-500"
                  : "border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-800 dark:text-zinc-200 hover:border-zinc-600 hover:bg-zinc-200 dark:hover:bg-zinc-800"
              }`}
            >
              {plan.cta}
            </Button>
          )}
        </CardContent>
      </Card>
    </motion.div>
  );
}

// ── Billing Toggle ─────────────────────────────────────────────────────────────

function BillingToggle({
  billing,
  onChange,
}: {
  billing: "monthly" | "yearly";
  onChange: (b: "monthly" | "yearly") => void;
}) {
  return (
    <div className="inline-flex items-center gap-1 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 p-1">
      <button
        type="button"
        onClick={() => onChange("monthly")}
        className={`relative cursor-pointer rounded-sm px-3.5 py-1.5 font-mono text-xs transition-all duration-200 ${
          billing === "monthly"
            ? "bg-zinc-700 text-zinc-900 dark:text-zinc-100"
            : "text-zinc-500 dark:text-zinc-400 hover:text-zinc-300"
        }`}
      >
        Monthly
      </button>
      <button
        type="button"
        onClick={() => onChange("yearly")}
        className={`relative cursor-pointer rounded-sm px-3.5 py-1.5 font-mono text-xs transition-all duration-200 ${
          billing === "yearly"
            ? "bg-zinc-700 text-zinc-900 dark:text-zinc-100"
            : "text-zinc-500 dark:text-zinc-400 hover:text-zinc-300"
        }`}
      >
        Yearly
        <span className="ml-1.5 inline-flex items-center rounded-sm bg-emerald-500/15 px-1 py-0.5 text-[9px] text-emerald-400">
          -20%
        </span>
      </button>
    </div>
  );
}

// ── Comparison Strip ───────────────────────────────────────────────────────────

const COMPARISON_ROWS = [
  { feature: "Self-hosted", community: true, team: false, business: false, enterprise: true },
  { feature: "Managed cloud", community: false, team: true, business: true, enterprise: true },
  { feature: "PII + Secret + Injection", community: true, team: true, business: true, enterprise: true },
  { feature: "YAML policy engine", community: true, team: true, business: true, enterprise: true },
  { feature: "Custom entity patterns", community: false, team: true, business: true, enterprise: true },
  { feature: "SSO / SAML", community: false, team: false, business: true, enterprise: true },
  { feature: "SSE streaming audit log", community: true, team: true, business: true, enterprise: true },
  { feature: "OWASP LLM compliance reports", community: true, team: true, business: true, enterprise: true },
  { feature: "Prioritized support SLA", community: false, team: "24h", business: "4h", enterprise: "1h" },
  { feature: "Air-gapped deployment", community: false, team: false, business: false, enterprise: true },
  { feature: "Custom compliance framework", community: false, team: false, business: false, enterprise: true },
] as const;

function ComparisonStrip({ visible }: { visible: boolean }) {
  const reduce = useReducedMotion();

  return (
    <motion.div
      className="overflow-x-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950"
      initial={reduce ? {} : { opacity: 0, y: 12 }}
      animate={visible ? { opacity: 1, y: 0 } : {}}
      transition={{ duration: 0.5, delay: 0.5 }}
    >
      <div className="min-w-[640px]">
        {/* Header row */}
        <div className="grid grid-cols-[1fr_80px_80px_80px_100px] gap-2 border-b border-zinc-200 dark:border-zinc-800 px-4 py-2 font-mono text-[9px] uppercase tracking-[0.12em] text-zinc-500 dark:text-zinc-400">
          <span>Feature</span>
          <span className="text-center">Community</span>
          <span className="text-center">Team</span>
          <span className="text-center">Business</span>
          <span className="text-center">Enterprise</span>
        </div>

        {/* Rows */}
        {COMPARISON_ROWS.map((row, i) => (
          <motion.div
            key={row.feature}
            className="grid grid-cols-[1fr_80px_80px_80px_100px] items-center gap-2 border-b border-zinc-200 dark:border-zinc-800/50 px-4 py-2 last:border-b-0"
            initial={reduce ? {} : { opacity: 0 }}
            animate={visible ? { opacity: 1 } : {}}
            transition={{ duration: 0.25, delay: 0.6 + i * 0.03 }}
          >
            <span className="font-mono text-[11px] text-zinc-600 dark:text-zinc-400">{row.feature}</span>
            {(["community", "team", "business", "enterprise"] as const).map((tier) => {
              const val = row[tier];
              if (val === true) {
                return (
                  <span key={tier} className="flex justify-center">
                    <Check className="h-3.5 w-3.5 text-emerald-500" />
                  </span>
                );
              }
              if (val === false) {
                return (
                  <span key={tier} className="flex justify-center">
                    <X className="h-3.5 w-3.5 text-zinc-700" />
                  </span>
                );
              }
              return (
                <span key={tier} className="text-center font-mono text-[10px] tabular-nums text-zinc-600 dark:text-zinc-400">
                  {val}
                </span>
              );
            })}
          </motion.div>
        ))}
      </div>
    </motion.div>
  );
}

// ── Main Export ─────────────────────────────────────────────────────────────────

export function Pricing() {
  const [billing, setBilling] = useState<"monthly" | "yearly">("monthly");
  const ref = useRef<HTMLDivElement | null>(null);
  const inView = useInView(ref, { once: true, margin: "-80px", amount: 0.05 });
  const reduce = useReducedMotion();

  return (
    <section id="pricing" ref={ref} className="scroll-mt-24 space-y-6">
      {/* ── Header ── */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <motion.p
            className="font-mono text-[11px] uppercase tracking-[0.16em] text-red-400"
            initial={reduce ? {} : { opacity: 0, y: 8 }}
            animate={inView ? { opacity: 1, y: 0 } : {}}
            transition={{ duration: 0.4 }}
          >
            PRICING //
          </motion.p>
          <motion.h2
            className="mt-2 text-3xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-4xl"
            initial={reduce ? {} : { opacity: 0, y: 8 }}
            animate={inView ? { opacity: 1, y: 0 } : {}}
            transition={{ duration: 0.4, delay: 0.05 }}
          >
            Start free, scale with confidence
          </motion.h2>
          <motion.p
            className="mt-2 max-w-2xl text-sm text-zinc-600 dark:text-zinc-400"
            initial={reduce ? {} : { opacity: 0 }}
            animate={inView ? { opacity: 1 } : {}}
            transition={{ duration: 0.4, delay: 0.1 }}
          >
            From self-hosted open-source to air-gapped enterprise — one pricing
            model that scales with your AI security maturity.
          </motion.p>
        </div>
        <motion.div
          initial={reduce ? {} : { opacity: 0, scale: 0.95 }}
          animate={inView ? { opacity: 1, scale: 1 } : {}}
          transition={{ duration: 0.4, delay: 0.15 }}
        >
          <BillingToggle billing={billing} onChange={setBilling} />
        </motion.div>
      </div>

      {/* ── Plan cards ── */}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
        {PLANS.map((plan, i) => (
          <PlanCard
            key={plan.name}
            plan={plan}
            billing={billing}
            visible={inView}
            index={i}
          />
        ))}
      </div>

      {/* ── Feature comparison ── */}
      <ComparisonStrip visible={inView} />

      {/* ── Bottom note ── */}
      <motion.p
        className="text-center font-mono text-[11px] text-zinc-600 dark:text-zinc-400"
        initial={reduce ? {} : { opacity: 0 }}
        animate={inView ? { opacity: 1 } : {}}
        transition={{ duration: 0.4, delay: 0.8 }}
      >
        All prices in USD. Volume discounts available for 50+ developers.{" "}
        <Link href="/contact" className="text-zinc-500 dark:text-zinc-400 underline underline-offset-4 hover:text-zinc-300">
          Contact sales
        </Link>{" "}
        for custom pricing.
      </motion.p>
    </section>
  );
}
