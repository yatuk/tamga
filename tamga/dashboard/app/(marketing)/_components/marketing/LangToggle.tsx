"use client";

import { useTranslation } from "@/lib/i18n";

export function LangToggle() {
  const { lang, setLang } = useTranslation();

  return (
    <div className="inline-flex overflow-hidden rounded-sm border border-zinc-300 dark:border-zinc-700 text-[11px]">
      <button
        type="button"
        onClick={() => setLang("tr")}
        aria-pressed={lang === "tr"}
        className={`h-7 cursor-pointer px-2 transition-colors ${lang === "tr" ? "bg-red-600 text-white" : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"}`}
      >
        TR
      </button>
      <button
        type="button"
        onClick={() => setLang("en")}
        aria-pressed={lang === "en"}
        className={`h-7 cursor-pointer px-2 transition-colors ${lang === "en" ? "bg-red-600 text-white" : "bg-white dark:bg-zinc-950 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"}`}
      >
        EN
      </button>
    </div>
  );
}
