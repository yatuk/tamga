"use client";

import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { ArrowRight, Calendar, CheckCircle2, ShieldAlert } from "lucide-react";
import { HCAPTCHA_SITEKEY, INDUSTRIES, pickIntent, type Intent } from "@/app/(marketing)/_components/marketing/contact/contact-constants";
import { ContactField, ContactSelectField } from "@/app/(marketing)/_components/marketing/contact/ContactFormFields";
import { toUpperLocale } from "@/lib/utils/tr-string";

declare global {
  interface Window {
    hcaptcha?: {
      render: (
        el: HTMLElement,
        opts: {
          sitekey: string;
          theme?: "dark" | "light";
          callback?: (token: string) => void;
          "expired-callback"?: () => void;
        },
      ) => number;
      reset: (id?: number) => void;
      getResponse: (id?: number) => string;
      remove?: (id?: number) => void;
    };
  }
}

export function ContactBody() {
  const params = useSearchParams();
  const intent = useMemo(() => pickIntent(new URLSearchParams(params?.toString() || "")), [params]);
  const initialPlan = params?.get("plan") || "";
  const initialIndustry = params?.get("industry") || "";
  const initialSize = params?.get("size") || "";

  const [submitted, setSubmitted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [captchaToken, setCaptchaToken] = useState<string>("");
  const captchaRef = useRef<HTMLDivElement | null>(null);
  const captchaIdRef = useRef<number | null>(null);

  useEffect(() => {
    if (!HCAPTCHA_SITEKEY || !captchaRef.current) return;
    const tryRender = () => {
      if (!window.hcaptcha || captchaIdRef.current !== null || !captchaRef.current) return;
      captchaIdRef.current = window.hcaptcha.render(captchaRef.current, {
        sitekey: HCAPTCHA_SITEKEY,
        theme: "dark",
        callback: (token: string) => setCaptchaToken(token),
        "expired-callback": () => setCaptchaToken(""),
      });
    };
    tryRender();
    const t = window.setInterval(tryRender, 500);
    return () => window.clearInterval(t);
  }, [intent]);

  const heading =
    intent === "demo"
      ? "Tamga demo planla"
      : intent === "disclosure"
        ? "Responsible disclosure"
        : "Build your quote";

  const lede =
    intent === "demo"
      ? "20 dakika — inline LLM proxy'yi kendi örneklerinizle çalıştıralım. Teknik ve/veya alım karar verici davet edin."
      : intent === "disclosure"
        ? "Güvenlik zafiyeti bildirimi. 72 saat içinde ilk geri dönüş, koordineli açıklama politikamız ile."
        : "Kurumsal plan için ihtiyaçlarınızı anlatın — 1 iş günü içinde size özel SOW ve teklif ile dönüyoruz.";

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setLoading(true);
    setError(null);
    const data = new FormData(e.currentTarget);
    const payload = Object.fromEntries(data.entries());
    if ((payload as { company_website?: string }).company_website) {
      setSubmitted(true);
      setLoading(false);
      return;
    }
    if (HCAPTCHA_SITEKEY && !captchaToken) {
      setError("Lütfen doğrulama kutusunu tamamlayın.");
      setLoading(false);
      return;
    }
    try {
      const r = await fetch("/api/leads", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...payload,
          intent,
          plan: initialPlan || undefined,
          hcaptcha_token: captchaToken || undefined,
        }),
      });
      if (!r.ok) {
        const body = await r.text().catch(() => "");
        throw new Error(body || `Request failed (${r.status})`);
      }
      setSubmitted(true);
    } catch (err) {
      setError((err as Error).message || "Something went wrong");
    } finally {
      setLoading(false);
    }
  }

  if (submitted) {
    return <ContactThankYou intent={intent} />;
  }

  return (
    <div className="mx-auto max-w-3xl px-6 py-16">
      <div className="mb-8">
        <div className="inline-flex items-center gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 px-2.5 py-1 font-mono text-[11px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">
          {intent === "demo" ? (
            <Calendar className="h-3 w-3" aria-hidden />
          ) : intent === "disclosure" ? (
            <ShieldAlert className="h-3 w-3" aria-hidden />
          ) : (
            <ArrowRight className="h-3 w-3" aria-hidden />
          )}
          {toUpperLocale(intent)}
        </div>
        <h1 className="mt-3 text-3xl font-extrabold tracking-tight text-zinc-900 dark:text-zinc-100 sm:text-4xl">
          {heading}
        </h1>
        <p className="mt-3 max-w-2xl text-base leading-7 text-zinc-600 dark:text-zinc-400">{lede}</p>
      </div>

      <form onSubmit={onSubmit} className="space-y-4 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40 p-6">
        <div className="grid gap-4 sm:grid-cols-2">
          <ContactField label="Adınız Soyadınız" name="name" required />
          <ContactField label="İş e-postası" name="email" type="email" required />
          <ContactField label="Şirket" name="company" required />
          <ContactField label="Kullanıcı sayısı" name="size" defaultValue={initialSize} placeholder="örn. 50" />
          {intent !== "disclosure" && (
            <>
              <ContactSelectField label="Sektör" name="industry" defaultValue={initialIndustry} options={INDUSTRIES} />
              <ContactField label="Aylık LLM çağrı hacmi" name="volume" placeholder="örn. 250k / ay" />
            </>
          )}
        </div>

        <label className="block">
          <span className="mb-1 block font-mono text-[11px] uppercase tracking-[0.16em] text-zinc-500 dark:text-zinc-400">
            {intent === "disclosure" ? "Zafiyet detayı" : "Notlar"}
          </span>
          <textarea
            name="notes"
            rows={4}
            className="w-full rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 outline-none transition-colors focus:border-red-500"
            placeholder={
              intent === "disclosure"
                ? "Reproduksiyon adımları, etkilenen versiyon, PoC link..."
                : "Hangi LLM sağlayıcısı? Hangi sektör? Neler önemli?"
            }
          />
        </label>

        <input
          type="text"
          name="company_website"
          tabIndex={-1}
          autoComplete="off"
          aria-hidden
          className="absolute left-[-9999px] h-0 w-0 opacity-0"
        />

        {HCAPTCHA_SITEKEY ? (
          <div className="flex flex-col items-start gap-2">
            <span className="font-mono text-[11px] uppercase tracking-[0.16em] text-zinc-500 dark:text-zinc-400">
              Doğrulama
            </span>
            <div ref={captchaRef} />
          </div>
        ) : null}

        {error && (
          <div className="rounded-sm border border-red-700/40 bg-red-950/30 px-3 py-2 text-sm text-red-300">
            {error}
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
          <p className="font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-600 dark:text-zinc-400">
            KVKK uyumlu · verileriniz satılmaz
          </p>
          <button
            type="submit"
            disabled={loading}
            className="inline-flex cursor-pointer items-center gap-2 rounded-sm bg-red-600 px-5 py-2.5 text-sm font-medium text-white transition-colors duration-200 hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {loading ? "Gönderiliyor…" : intent === "disclosure" ? "Bildir" : "Gönder"}
            <ArrowRight className="h-3.5 w-3.5" aria-hidden />
          </button>
        </div>
      </form>
    </div>
  );
}

function ContactThankYou({ intent }: { intent: Intent }) {
  return (
    <div className="mx-auto max-w-2xl px-6 py-20">
      <div className="rounded-sm border border-emerald-500/30 bg-emerald-900/10 p-8 text-center">
        <CheckCircle2 className="mx-auto h-12 w-12 text-emerald-400" aria-hidden />
        <h1 className="mt-4 text-2xl font-bold text-zinc-900 dark:text-zinc-100">Teşekkürler!</h1>
        <p className="mt-3 text-sm leading-7 text-zinc-600 dark:text-zinc-400">
          {intent === "disclosure"
            ? "Güvenlik ekibimiz 72 saat içinde dönecek."
            : "Ekibimiz 1 iş günü içinde size dönüyor."}
        </p>
        <div className="mt-6 flex justify-center gap-3">
          <Link
            href="/"
            className="inline-flex items-center gap-2 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-4 py-2 text-sm text-zinc-800 dark:text-zinc-200 transition-colors hover:border-zinc-500"
          >
            Back to home
          </Link>
          <Link
            href="/docs/quickstart"
            className="inline-flex items-center gap-2 rounded-sm bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700"
          >
            Quickstart
            <ArrowRight className="h-3.5 w-3.5" aria-hidden />
          </Link>
        </div>
      </div>
    </div>
  );
}
