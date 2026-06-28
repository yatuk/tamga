import { toLowerEn } from "@/lib/utils/tr-string";

export const HCAPTCHA_SITEKEY = process.env.NEXT_PUBLIC_HCAPTCHA_SITEKEY || "";

export type Intent = "demo" | "quote" | "disclosure";

export function pickIntent(search: URLSearchParams): Intent {
  const i = toLowerEn(search.get("intent") || "");
  if (i === "demo") return "demo";
  if (i === "disclosure") return "disclosure";
  if (search.get("plan") === "enterprise" || search.get("plan") === "business") return "quote";
  if (i === "quote") return "quote";
  return "demo";
}

export const INDUSTRIES = [
  "Bankacılık & Fintech",
  "Sigorta",
  "Sağlık",
  "Kamu & Savunma",
  "Telekom",
  "E-ticaret & Lojistik",
  "Diğer",
];
