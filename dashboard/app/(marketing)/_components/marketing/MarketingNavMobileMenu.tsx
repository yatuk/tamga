"use client";

import Link from "next/link";
import * as Dialog from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { TamgaLogo } from "@/components/TamgaLogo";
import { ThemeToggle } from "@/components/theme-toggle";
import { LangToggle } from "@/app/(marketing)/_components/marketing/LangToggle";
import { useTranslation } from "@/lib/i18n";
import type { ResourceLink } from "./MarketingNavResourceGroups";

function MobileSection({
  title,
  items,
  onNavigate,
}: {
  title: string;
  items: ResourceLink[];
  onNavigate: () => void;
}) {
  return (
    <div>
      <p className="mb-2 px-2 font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-500 dark:text-zinc-400">{title}</p>
      <ul className="space-y-0.5">
        {items.map((item) => (
          <li key={item.href + item.label}>
            <Link
              href={item.href}
              onClick={onNavigate}
              className="flex items-start gap-3 rounded-sm px-2 py-2 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              <item.icon className="mt-0.5 h-4 w-4 shrink-0 text-zinc-600 dark:text-zinc-400" aria-hidden />
              <div className="min-w-0">
                <div className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{item.label}</div>
                <div className="mt-0.5 text-[11px] leading-4 text-zinc-500 dark:text-zinc-400">{item.caption}</div>
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}

export function MobileMenu({
  open,
  onOpenChange,
  product,
  learn,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  product: ResourceLink[];
  learn: ResourceLink[];
}) {
  const { t } = useTranslation();
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in" />
        <Dialog.Content
          aria-describedby="marketing-mobile-desc"
          className="fixed right-0 top-0 z-50 flex h-full w-[86%] max-w-sm flex-col border-l border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-5 shadow-2xl"
        >
          <Dialog.Title className="sr-only">{t("nav.resources")}</Dialog.Title>
          <Dialog.Description id="marketing-mobile-desc" className="sr-only">
            {t("nav.resources")}
          </Dialog.Description>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <TamgaLogo size={24} />
              <span className="text-sm font-mono font-medium text-zinc-800 dark:text-zinc-200">tamga</span>
            </div>
            <Dialog.Close asChild>
              <button
                type="button"
                aria-label={t("nav.close")}
                className="cursor-pointer rounded-sm border border-zinc-200 dark:border-zinc-800 p-1.5 text-zinc-600 dark:text-zinc-400 transition-colors hover:border-zinc-600 hover:text-zinc-100"
              >
                <X className="h-4 w-4" aria-hidden />
              </button>
            </Dialog.Close>
          </div>

          <nav className="mt-6 flex-1 space-y-6 overflow-y-auto">
            <div>
              <Link
                href="/pricing"
                onClick={() => onOpenChange(false)}
                className="block rounded-sm px-2 py-2 text-base font-medium text-zinc-900 dark:text-zinc-100 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900"
              >
                {t("nav.pricing")}
              </Link>
              <Link
                href="/docs"
                onClick={() => onOpenChange(false)}
                className="block rounded-sm px-2 py-2 text-base font-medium text-zinc-900 dark:text-zinc-100 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900"
              >
                {t("nav.docs")}
              </Link>
              <Link
                href="/trust"
                onClick={() => onOpenChange(false)}
                className="block rounded-sm px-2 py-2 text-base font-medium text-zinc-900 dark:text-zinc-100 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900"
              >
                {t("nav.trust")}
              </Link>
              <Link
                href="/changelog"
                onClick={() => onOpenChange(false)}
                className="block rounded-sm px-2 py-2 text-base font-medium text-zinc-900 dark:text-zinc-100 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900"
              >
                {t("nav.changelog")}
              </Link>
            </div>

            <MobileSection title={t("nav.resources.product")} items={product} onNavigate={() => onOpenChange(false)} />
            <MobileSection title={t("nav.resources.learn")} items={learn} onNavigate={() => onOpenChange(false)} />
          </nav>

          <div className="mt-6 space-y-2 border-t border-zinc-200 dark:border-zinc-800 pt-4">
            <Link
              href="/sign-in"
              onClick={() => onOpenChange(false)}
              className="block rounded-sm border border-zinc-300 dark:border-zinc-700 px-3 py-2 text-center text-sm text-zinc-800 dark:text-zinc-200 transition-colors hover:border-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              {t("nav.sign_in")}
            </Link>
            <Link
              href="/contact?intent=demo"
              onClick={() => onOpenChange(false)}
              className="block rounded-sm bg-red-600 px-3 py-2 text-center text-sm font-medium text-white transition-colors hover:bg-red-700"
            >
              {t("nav.demo_cta")}
            </Link>
            <div className="flex items-center justify-between pt-2">
              <LangToggle />
              <ThemeToggle />
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
