import type { Metadata, Viewport } from "next";
import { Archivo, Atkinson_Hyperlegible_Mono, Atkinson_Hyperlegible_Next } from "next/font/google";
import "./tamga.css";
import { NuqsAdapter } from "nuqs/adapters/next/app";
import { QueryProvider } from "@/lib/query-provider";
import { Toaster } from "@/components/console/toaster";

// Atkinson Hyperlegible for text: it was drawn to keep 0/O and 1/l/I apart,
// and this console is read for request IDs, keys and card numbers. Archivo,
// set narrow, for headings and figures. The mono cut only where text is data.
// All three are variable fonts: one file each.
const atkinson = Atkinson_Hyperlegible_Next({
  subsets: ["latin", "latin-ext"],
  variable: "--font-atkinson",
  // Next has no fallback metrics for this face; the fallbacks below are close enough.
  adjustFontFallback: false,
  display: "swap",
  fallback: ["Segoe UI", "Arial", "sans-serif"],
});
const archivo = Archivo({
  subsets: ["latin", "latin-ext"],
  axes: ["wdth"],
  variable: "--font-archivo",
  display: "swap",
  fallback: ["Arial Narrow", "Arial", "sans-serif"],
});
const atkinsonMono = Atkinson_Hyperlegible_Mono({
  subsets: ["latin", "latin-ext"],
  variable: "--font-atkinson-mono",
  // Next has no fallback metrics for this face; the fallbacks below are close enough.
  adjustFontFallback: false,
  display: "swap",
  fallback: ["Consolas", "Menlo", "monospace"],
});

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://tamga.dev";

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

// Tints the browser chrome to match the paper.
/* eslint-disable tamga/no-hardcoded-color -- a meta tag cannot read CSS variables */
export const viewport: Viewport = { themeColor: "#f4f4f4" };
/* eslint-enable tamga/no-hardcoded-color */

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      {/* The body only carries the font variables; colours and faces are set in tamga.css. */}
      <body className={`${atkinson.variable} ${archivo.variable} ${atkinsonMono.variable}`}>
        <NuqsAdapter>
          <QueryProvider>{children}</QueryProvider>
        </NuqsAdapter>
        <Toaster />
      </body>
    </html>
  );
}
