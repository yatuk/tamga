import type { Metadata } from "next";
import { Fira_Code, Inter } from "next/font/google";
import { cookies } from "next/headers";
import "./globals.css";
import { QueryProvider } from "@/lib/query-provider";
import { ThemeProvider } from "@/components/theme-provider";
import { Toaster } from "@/components/ui/sonner";
import { toLowerEn } from "@/lib/utils/tr-string";

// display: "swap" keeps fallback text painted while Inter downloads,
// eliminating the "blank text" CLS flash; preload + system fallback
// stacks also reduce layout shift on first paint.
const inter = Inter({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-inter",
  display: "swap",
  preload: true,
  fallback: ["system-ui", "Segoe UI", "Arial", "sans-serif"],
});
const firaCode = Fira_Code({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-fira-code",
  display: "swap",
  preload: true,
  fallback: ["ui-monospace", "SFMono-Regular", "Menlo", "Consolas", "monospace"],
});

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://tamga.dev";

const DESIGN_CONTRACT = `<!--
THESIS: Tamga is a chain-of-custody workspace for live LLM risk, not a wall of interchangeable security cards.
OWN-WORLD: Ink-black and paper-white records, graphite rules, Tamga red evidence seals, compact sans labels, and mono measurements.
STORY: Read posture, isolate the material change, then enter the incident with its source, time range, and evidence intact.
FIRST VIEWPORT: Persistent case-index navigation; an operational header; one risk disposition; a live evidence queue; compact supporting measurements.
FORM: Forensic chain-of-custody docket, grounded direction 5, seed 3015f63b.
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
-->`;

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: "Tamga — AI Security Proxy",
    template: "%s · Tamga",
  },
  description:
    "LLM trafiğine inline yerleşen AI güvenlik proxy'si: PII/PCI redaction, prompt injection defense, policy engine, SOC dashboard.",
  keywords: [
    "AI security proxy",
    "LLM firewall",
    "prompt injection",
    "PII redaction",
    "OWASP LLM Top 10",
    "SOC dashboard",
    "Tamga",
  ],
  applicationName: "Tamga",
  authors: [{ name: "Tamga", url: SITE_URL }],
  openGraph: {
    type: "website",
    siteName: "Tamga",
    title: "Tamga — AI Security Proxy",
    description:
      "LLM trafiğini inline tarar, PII/secret'i redakte eder, prompt injection'ı bloklar.",
    url: SITE_URL,
  },
  twitter: {
    card: "summary_large_image",
    title: "Tamga — AI Security Proxy",
    description:
      "LLM firewall, PII redaction, policy engine, SOC dashboard.",
  },
  robots: {
    index: true,
    follow: true,
  },
};

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const cookieStore = await cookies();
  const themeCookie = cookieStore.get("tamga-theme")?.value;
  const defaultTheme = themeCookie === "light" ? "light" : "dark";
  const htmlClassName = defaultTheme === "dark" ? "dark" : undefined;
  const pk = process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY || "";
  const clerkEnabled = pk && !toLowerEn(pk).includes("placeholder");

  // Body background/text come from globals.css tokens (--surface-base / --fg);
  // here we only carry the font variables + Inter so the shell stays token-driven.
  const bodyClass = `${inter.variable} ${firaCode.variable} ${inter.className} bg-surface-base text-fg`;

  if (!clerkEnabled) {
    return (
      <html lang="tr" className={htmlClassName} suppressHydrationWarning>
        <body className={bodyClass}>
          <template
            data-impeccable-contract="3015f63b"
            dangerouslySetInnerHTML={{ __html: DESIGN_CONTRACT }}
          />
          <ThemeProvider
            attribute="class"
            defaultTheme={defaultTheme}
            enableSystem={true}
          >
            <QueryProvider>{children}</QueryProvider>
            <Toaster richColors position="bottom-right" />
          </ThemeProvider>
        </body>
      </html>
    );
  }

  const { ClerkProvider } = await import("@clerk/nextjs");
  return (
    <html lang="tr" className={htmlClassName} suppressHydrationWarning>
      <body className={bodyClass}>
        <template
          data-impeccable-contract="3015f63b"
          dangerouslySetInnerHTML={{ __html: DESIGN_CONTRACT }}
        />
        <ClerkProvider>
          <ThemeProvider
            attribute="class"
            defaultTheme={defaultTheme}
            enableSystem={true}
          >
            <QueryProvider>{children}</QueryProvider>
            <Toaster richColors position="bottom-right" />
          </ThemeProvider>
        </ClerkProvider>
      </body>
    </html>
  );
}
