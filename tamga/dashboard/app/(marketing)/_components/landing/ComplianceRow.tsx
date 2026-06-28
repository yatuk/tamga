import { Shield, Lock, FileText, EyeOff } from "lucide-react";

const CARDS = [
  {
    icon: Shield,
    title: "KVKK & GDPR Ready",
    desc: "Local processing with zero data retention. No PII leaves your infrastructure. All redaction happens inline before any external API call.",
  },
  {
    icon: Lock,
    title: "PCI-DSS Masking",
    desc: "Real-time credit card number detection via strict Modulus 10 (Luhn) validation. PAN data is redacted before it reaches the LLM provider.",
  },
  {
    icon: FileText,
    title: "Immutable Audit Trail",
    desc: "Cryptographically verifiable event logging to PostgreSQL. Every prompt, redaction, and policy decision is timestamped and queryable via SQL.",
  },
  {
    icon: EyeOff,
    title: "Zero-Trust Architecture",
    desc: "Complete control over outbound LLM API traffic. No model sees your data unless your policy explicitly allows it. Block shadow AI providers by default.",
  },
];

export function ComplianceRow() {
  return (
    <div>
      <div className="mb-8 text-center">
        <h2 className="text-2xl font-semibold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-3xl">
          Enterprise compliance, built in
        </h2>
        <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400 max-w-2xl mx-auto">
          Tamga is designed for regulated industries. Banking, healthcare, and government
          teams can deploy with confidence.
        </p>
      </div>

      <div className="grid gap-4 grid-cols-1 md:grid-cols-2 lg:grid-cols-4">
        {CARDS.map((card) => (
          <div
            key={card.title}
            className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-6"
          >
            <div className="mb-3 inline-flex h-9 w-9 items-center justify-center rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900">
              <card.icon className="h-4 w-4 text-zinc-600 dark:text-zinc-400" />
            </div>
            <h3 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
              {card.title}
            </h3>
            <p className="mt-2 text-[12px] leading-relaxed text-zinc-500 dark:text-zinc-400">
              {card.desc}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
}
