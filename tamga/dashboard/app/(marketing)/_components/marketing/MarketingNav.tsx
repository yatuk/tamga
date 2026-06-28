"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { motion } from "framer-motion";
import { ChevronDown, Menu } from "lucide-react";
import { TamgaLogo } from "@/components/TamgaLogo";
import { ThemeToggle } from "@/components/theme-toggle";
import { LangToggle } from "@/app/(marketing)/_components/marketing/LangToggle";
import { useTranslation } from "@/lib/i18n";
import { MegaColumn } from "./MarketingNavMegaColumn";
import { MobileMenu } from "./MarketingNavMobileMenu";
import { useResourceGroups } from "./MarketingNavResourceGroups";

export function MarketingNav() {
  const [scrolled, setScrolled] = useState(false);
  const [resourcesOpen, setResourcesOpen] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const pathname = usePathname();
  const { t } = useTranslation();
  const resourcesRef = useRef<HTMLDivElement | null>(null);
  const resourcesTriggerRef = useRef<HTMLButtonElement | null>(null);
  const { product, learn } = useResourceGroups();

  const isActive = (href: string) => {
    if (href === "/" || href.startsWith("/#")) return pathname === "/";
    if (href === "/pricing") return pathname === "/pricing";
    if (href === "/changelog") return pathname === "/changelog";
    return pathname.startsWith(href);
  };
  const resourcesActive = ["/blog", "/compare", "/roi", "/evals", "/models", "/case-studies", "/whitepaper", "/status"].some((p) => pathname.startsWith(p));

  const tabCls = (href: string) =>
    `cursor-pointer rounded-sm px-3 py-2 text-sm transition-colors duration-200 ${
      isActive(href)
        ? "bg-zinc-100 dark:bg-zinc-900 text-zinc-50"
        : "text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900/80 hover:text-zinc-50"
    }`;

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 16);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    if (!resourcesOpen) return;
    const onDown = (e: MouseEvent) => {
      if (resourcesRef.current && !resourcesRef.current.contains(e.target as Node)) {
        setResourcesOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setResourcesOpen(false);
        resourcesTriggerRef.current?.focus();
      }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [resourcesOpen]);

  return (
    <motion.header
      className="sticky top-0 z-30 border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/90"
      initial={false}
      animate={{
        paddingTop: scrolled ? 10 : 16,
        paddingBottom: scrolled ? 10 : 16,
        boxShadow: scrolled ? "0 8px 30px -12px rgba(15,23,42,0.12)" : "0 0 0 rgba(0,0,0,0)",
      }}
      transition={{ type: "spring", damping: 28, stiffness: 320 }}
      style={{ backdropFilter: scrolled ? "blur(12px)" : "blur(8px)" }}
    >
      <div className="mx-auto flex w-full max-w-7xl items-center justify-between gap-4 px-4 sm:px-6">
        <Link href="/" className="flex cursor-pointer items-center gap-2">
          <TamgaLogo size={28} priority />
          <div className="flex flex-col leading-tight">
            <span className="text-sm font-mono font-medium tracking-tight text-zinc-800 dark:text-zinc-200">tamga</span>
            <span className="text-[10px] font-mono uppercase tracking-wider text-zinc-500 dark:text-zinc-400">AI Proxy // v0.1.1</span>
          </div>
        </Link>

        <nav className="hidden items-center gap-1 lg:flex">
          <Link href="/pricing" className={tabCls("/pricing")}>
            {t("nav.pricing")}
          </Link>

          <Link href="/docs" className={tabCls("/docs")}>
            {t("nav.docs")}
          </Link>

          <Link href="/trust" className={tabCls("/trust")}>
            {t("nav.trust")}
          </Link>

          <Link href="/changelog" className={tabCls("/changelog")}>
            {t("nav.changelog")}
          </Link>

          <div className="relative" ref={resourcesRef}>
            <button
              ref={resourcesTriggerRef}
              type="button"
              onClick={() => setResourcesOpen((v) => !v)}
              aria-haspopup="true"
              aria-expanded={resourcesOpen}
              aria-controls="marketing-nav-resources"
              className={`inline-flex cursor-pointer items-center gap-1 rounded-sm px-3 py-2 text-sm transition-colors duration-200 ${
                resourcesOpen || resourcesActive
                  ? "bg-zinc-100 dark:bg-zinc-900 text-zinc-50"
                  : "text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900/80 hover:text-zinc-50"
              }`}
            >
              {t("nav.resources")}
              <ChevronDown
                className={`h-3.5 w-3.5 transition-transform duration-200 ${resourcesOpen ? "rotate-180" : ""}`}
                aria-hidden
              />
            </button>

            {resourcesOpen && (
              <motion.div
                initial={{ opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.15 }}
                id="marketing-nav-resources"
                aria-label={t("nav.resources")}
                className="absolute left-1/2 top-[calc(100%+10px)] z-40 w-[640px] -translate-x-1/2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4 shadow-2xl"
              >
                <div className="grid grid-cols-2 gap-4">
                  <MegaColumn title={t("nav.resources.product")} items={product} onNavigate={() => setResourcesOpen(false)} />
                  <MegaColumn title={t("nav.resources.learn")} items={learn} onNavigate={() => setResourcesOpen(false)} />
                </div>
              </motion.div>
            )}
          </div>
        </nav>

        <div className="flex items-center gap-2">
          <div className="hidden items-center gap-1 border-r border-zinc-200 dark:border-zinc-800 pr-2 sm:flex">
            <LangToggle />
            <ThemeToggle />
          </div>

          <Link
            href="/sign-in"
            className="hidden cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 px-3 py-1.5 text-sm text-zinc-700 dark:text-zinc-300 transition-colors duration-200 hover:border-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-900 sm:inline-flex"
          >
            {t("nav.sign_in")}
          </Link>

          <Link
            href="/contact?intent=demo"
            className="hidden cursor-pointer items-center gap-1.5 rounded-sm bg-red-600 px-3 py-1.5 text-sm font-medium text-white transition-colors duration-200 hover:bg-red-700 sm:inline-flex"
          >
            {t("nav.demo_cta")}
          </Link>

          <button
            type="button"
            className="inline-flex cursor-pointer items-center justify-center rounded-sm border border-zinc-200 dark:border-zinc-800 p-2 text-zinc-700 dark:text-zinc-300 transition-colors duration-200 hover:border-zinc-600 hover:bg-zinc-100 dark:hover:bg-zinc-900 lg:hidden"
            aria-label={t("nav.open")}
            onClick={() => setMobileOpen(true)}
          >
            <Menu className="h-4 w-4" aria-hidden />
          </button>
        </div>
      </div>

      <MobileMenu open={mobileOpen} onOpenChange={setMobileOpen} product={product} learn={learn} />
    </motion.header>
  );
}
