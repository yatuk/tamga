"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { X } from "lucide-react";
import { useTranslation } from "@/lib/i18n";

const STORAGE_KEY = "tamga_cookie_consent_v1";

export function CookieBanner() {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const consent = window.localStorage.getItem(STORAGE_KEY);
    if (!consent) setVisible(true);
  }, []);

  if (!visible) return null;

  const accept = (value: "all" | "essential") => {
    try {
      window.localStorage.setItem(STORAGE_KEY, value);
    } catch {}
    setVisible(false);
  };

  return (
    <div
      role="dialog"
      aria-label="Cookie consent"
      aria-live="polite"
      className="fixed bottom-4 left-1/2 z-[60] w-[calc(100%-2rem)] max-w-2xl -translate-x-1/2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 shadow-2xl"
    >
      <div className="flex items-start gap-3">
        <div className="flex-1 text-xs text-zinc-700 dark:text-zinc-300">
          <p className="font-mono">
            Tamga; oturumu, tema tercihini ve analitik tercihlerini saklamak için tarayıcı
            depolama alanını kullanır. Detaylar için{" "}
            <Link href="/docs" className="text-red-400 hover:underline">
              docs
            </Link>
            .
          </p>
        </div>
        <button
          type="button"
          onClick={() => accept("essential")}
          aria-label={t("nav.close")}
          className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 p-1 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800"
        >
          <X className="h-3.5 w-3.5" aria-hidden />
        </button>
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        <button
          type="button"
          onClick={() => accept("essential")}
          className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-3 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
        >
          Yalnızca gerekli
        </button>
        <button
          type="button"
          onClick={() => accept("all")}
          className="h-7 cursor-pointer rounded-sm bg-red-600 px-3 font-mono text-[11px] text-white hover:bg-red-700"
        >
          Tümünü kabul et
        </button>
      </div>
    </div>
  );
}
