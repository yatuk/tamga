import type { Metadata, Viewport } from "next";
import { Barlow, Barlow_Condensed, JetBrains_Mono } from "next/font/google";
import { cookies } from "next/headers";
import "./globals.css";
import { QueryProvider } from "@/lib/query-provider";
import { ThemeProvider } from "@/components/theme-provider";
import { Toaster } from "@/components/ui/sonner";
import { toLowerEn } from "@/lib/utils/case";

// Same families as tamgaproxy.com. display: "swap" keeps fallback text
// painted while the fonts download.
const barlow = Barlow({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-barlow",
  display: "swap",
  fallback: ["system-ui", "Segoe UI", "Arial", "sans-serif"],
});
const barlowCondensed = Barlow_Condensed({
  subsets: ["latin"],
  weight: ["600", "700", "800"],
  variable: "--font-barlow-condensed",
  display: "swap",
  fallback: ["Arial Narrow", "Arial", "sans-serif"],
});
const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500", "700"],
  variable: "--font-jetbrains-mono",
  display: "swap",
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
    default: "Tamga: AI Security Proxy",
    template: "%s · Tamga",
  },
  description:
    "Inline AI security proxy for LLM traffic: PII/PCI redaction, prompt injection defense, policy engine, SOC dashboard.",
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
    title: "Tamga: AI Security Proxy",
    description:
      "Scans LLM traffic inline, redacts PII and secrets, blocks prompt injection.",
    url: SITE_URL,
  },
  twitter: {
    card: "summary_large_image",
    title: "Tamga: AI Security Proxy",
    description:
      "LLM firewall, PII redaction, policy engine, SOC dashboard.",
  },
  robots: {
    index: true,
    follow: true,
  },
};

// Tints the browser chrome to match the page background in each theme.
export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: dark)", color: "#10120f" },
    { media: "(prefers-color-scheme: light)", color: "#e7e8e2" },
  ],
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

  // Colors and font families come from the tokens in globals.css; the body
  // only carries the font variables.
  const bodyClass = `${barlow.variable} ${barlowCondensed.variable} ${jetbrainsMono.variable} bg-background font-sans text-foreground antialiased`;

  if (!clerkEnabled) {
    return (
      <html lang="en" className={htmlClassName} suppressHydrationWarning>
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
    <html lang="en" className={htmlClassName} suppressHydrationWarning>
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
