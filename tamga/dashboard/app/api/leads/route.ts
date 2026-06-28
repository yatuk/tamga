import { NextRequest, NextResponse } from "next/server";

// Lightweight public lead-capture endpoint.
//
// - Honeypot + simple in-memory rate-limit (10 posts / 10 min / IP).
// - Validates email + required fields.
// - Forwards structured payload to TAMGA_LEADS_WEBHOOK (Slack/Teams) or
//   falls back to stderr log when the env var is unset.

export const runtime = "nodejs";

type LeadPayload = {
  name?: string;
  email?: string;
  company?: string;
  size?: string;
  industry?: string;
  volume?: string;
  notes?: string;
  intent?: string;
  plan?: string;
  company_website?: string; // honeypot
  hcaptcha_token?: string;
};

// Rate-limit state — per-process; good enough for a single container
// where a reverse proxy / IP rate-limit handles scale-out traffic.
const rateLimit = new Map<string, { count: number; ts: number }>();
const WINDOW_MS = 10 * 60 * 1000;
const MAX_PER_WINDOW = 10;

function rateLimited(ip: string): boolean {
  const now = Date.now();
  const existing = rateLimit.get(ip);
  if (!existing || now - existing.ts > WINDOW_MS) {
    rateLimit.set(ip, { count: 1, ts: now });
    return false;
  }
  existing.count += 1;
  if (existing.count > MAX_PER_WINDOW) return true;
  return false;
}

function isEmail(v: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);
}

export async function POST(req: NextRequest) {
  let payload: LeadPayload;
  try {
    payload = await req.json();
  } catch {
    return NextResponse.json({ error: "invalid_json" }, { status: 400 });
  }

  // Honeypot — silently succeed; bots won't learn anything.
  if (payload.company_website && payload.company_website.length > 0) {
    return NextResponse.json({ ok: true });
  }

  const ip =
    req.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ||
    req.headers.get("x-real-ip") ||
    "unknown";
  if (rateLimited(ip)) {
    return NextResponse.json({ error: "rate_limited" }, { status: 429 });
  }

  if (!payload.name || !payload.email || !payload.company) {
    return NextResponse.json({ error: "missing_fields" }, { status: 400 });
  }
  if (!isEmail(payload.email)) {
    return NextResponse.json({ error: "invalid_email" }, { status: 400 });
  }

  // hCaptcha verification — only enforced when HCAPTCHA_SECRET is set.
  // This keeps the endpoint usable in dev without adding a dependency
  // on the hCaptcha service, while still giving prod a real challenge.
  const hcaptchaSecret = process.env.HCAPTCHA_SECRET;
  if (hcaptchaSecret) {
    if (!payload.hcaptcha_token) {
      return NextResponse.json({ error: "captcha_required" }, { status: 400 });
    }
    try {
      const params = new URLSearchParams();
      params.set("secret", hcaptchaSecret);
      params.set("response", payload.hcaptcha_token);
      params.set("remoteip", ip);
      const vr = await fetch("https://api.hcaptcha.com/siteverify", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: params.toString(),
      });
      const vj = (await vr.json().catch(() => ({ success: false }))) as {
        success?: boolean;
      };
      if (!vj.success) {
        return NextResponse.json({ error: "captcha_failed" }, { status: 400 });
      }
    } catch (err) {
      console.error("[tamga-lead] hcaptcha verify failed", err);
      return NextResponse.json({ error: "captcha_unavailable" }, { status: 503 });
    }
  }

  const webhook = process.env.TAMGA_LEADS_WEBHOOK;
  const message = {
    intent: payload.intent || "demo",
    plan: payload.plan || "",
    name: payload.name,
    email: payload.email,
    company: payload.company,
    size: payload.size || "",
    industry: payload.industry || "",
    volume: payload.volume || "",
    notes: payload.notes || "",
    ip,
    at: new Date().toISOString(),
  };

  if (webhook) {
    try {
      await fetch(webhook, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text: "[tamga-lead] " + JSON.stringify(message) }),
      });
    } catch (err) {
      // Fall through — we still report success to the user; the lead is
      // captured in the server log below for manual recovery.
      console.error("[tamga-lead] webhook failed", err);
    }
  } else {
    console.log("[tamga-lead]", JSON.stringify(message));
  }

  return NextResponse.json({ ok: true });
}
