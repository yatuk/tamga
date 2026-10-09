import type { Metadata, Viewport } from "next";
import { Archivo, Atkinson_Hyperlegible_Mono, Atkinson_Hyperlegible_Next } from "next/font/google";
import { cookies } from "next/headers";
import "./globals.css";
import { NuqsAdapter } from "nuqs/adapters/next/app";
import { QueryProvider } from "@/lib/query-provider";
import { ClerkBoundary } from "@/components/clerk-boundary";
import { ThemeProvider } from "@/components/theme-provider";
import { Toaster } from "@/components/ui/sonner";
import { toLowerEn } from "@/lib/utils/case";

// Atkinson Hyperlegible for text: it was drawn to keep 0/O and 1/l/I apart,
// and this console is read for request IDs, keys and card numbers. Archivo,
// set narrow, for headings and figures. The mono cut only where text is data.
// All three are variable fonts: one file each.
const atkinson = Atkinson_Hyperlegible_Next({
  subsets: ["latin", "latin-ext"],
  variable: "--font-atkinson",
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

// Tints the browser chrome to match the page background in each theme.
/* eslint-disable tamga/no-hardcoded-color -- a meta tag cannot read CSS variables */
export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: dark)", color: "#10120f" },
    { media: "(prefers-color-scheme: light)", color: "#e7e8e2" },
  ],
};
/* eslint-enable tamga/no-hardcoded-color */

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
  const bodyClass = `${atkinson.variable} ${archivo.variable} ${atkinsonMono.variable} bg-background font-sans text-foreground antialiased`;

  const tree = (
    <ThemeProvider attribute="class" defaultTheme={defaultTheme} enableSystem={true}>
      <NuqsAdapter>
        <QueryProvider>{children}</QueryProvider>
      </NuqsAdapter>
      <Toaster richColors position="bottom-right" />
    </ThemeProvider>
  );

  // Clerk is only loaded when a real publishable key is configured.
  const body = clerkEnabled ? <ClerkBoundary>{tree}</ClerkBoundary> : tree;

  return (
    <html lang="en" className={htmlClassName} suppressHydrationWarning>
      <body className={bodyClass}>{body}</body>
    </html>
  );
}
