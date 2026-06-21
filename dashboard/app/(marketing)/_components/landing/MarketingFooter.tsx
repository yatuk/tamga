"use client";

import Link from "next/link";
import { CircleDot, Github, Linkedin, Mail } from "lucide-react";
import { TamgaLogo } from "@/components/TamgaLogo";
import { useTranslation } from "@/lib/i18n";

const SOCIAL_ICON_CLS =
  "cursor-pointer text-zinc-500 dark:text-zinc-400 transition-colors duration-200 hover:text-zinc-200";
const FOOTER_LINK_CLS =
  "cursor-pointer text-zinc-600 dark:text-zinc-400 transition-colors duration-200 hover:text-zinc-200";
const FOOTER_EYEBROW_CLS =
  "font-mono text-[11px] uppercase tracking-[0.16em] text-zinc-500 dark:text-zinc-400";

export function MarketingFooter() {
  const { t } = useTranslation();
  return (
    <footer className="border-t border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
      <div className="mx-auto max-w-7xl px-6 py-12">
        <div className="grid grid-cols-2 gap-8 md:grid-cols-5">
          <div className="col-span-2">
            <div className="flex items-center gap-2">
              <TamgaLogo size={24} />
              <span className="font-mono font-medium text-zinc-800 dark:text-zinc-200">tamga</span>
            </div>
            <p className="mt-3 max-w-xs text-sm text-zinc-600 dark:text-zinc-400">{t("footer.tagline")}</p>
            <div className="mt-4 flex items-center gap-3">
              <a href="https://github.com/tamga-dev/tamga" aria-label="GitHub" className={SOCIAL_ICON_CLS}>
                <Github className="h-4 w-4" />
              </a>
              <a href="https://linkedin.com/company/tamga" aria-label="LinkedIn" className={SOCIAL_ICON_CLS}>
                <Linkedin className="h-4 w-4" />
              </a>
              <a href="mailto:hello@tamga.dev" aria-label="Email" className={SOCIAL_ICON_CLS}>
                <Mail className="h-4 w-4" />
              </a>
            </div>
          </div>

          <div>
            <div className={FOOTER_EYEBROW_CLS}>{t("footer.product")}</div>
            <ul className="mt-3 space-y-2 text-sm">
              <li><a href="#features" className={FOOTER_LINK_CLS}>{t("footer.features")}</a></li>
              <li><Link href="/pricing" className={FOOTER_LINK_CLS}>{t("footer.pricing")}</Link></li>
              <li><a href="#demo" className={FOOTER_LINK_CLS}>{t("footer.live_demo")}</a></li>
              <li><Link href="/docs" className={FOOTER_LINK_CLS}>{t("footer.docs")}</Link></li>
              <li><Link href="/changelog" className={FOOTER_LINK_CLS}>{t("footer.changelog")}</Link></li>
            </ul>
          </div>

          <div>
            <div className={FOOTER_EYEBROW_CLS}>{t("footer.resources")}</div>
            <ul className="mt-3 space-y-2 text-sm">
              <li><Link href="/docs/owasp-llm" className={FOOTER_LINK_CLS}>OWASP LLM Top 10</Link></li>
              <li><Link href="/docs/architecture" className={FOOTER_LINK_CLS}>Architecture</Link></li>
              <li><Link href="/docs/quickstart" className={FOOTER_LINK_CLS}>Quickstart</Link></li>
              <li><Link href="/blog" className={FOOTER_LINK_CLS}>Blog</Link></li>
              <li><Link href="/case-studies" className={FOOTER_LINK_CLS}>Case Studies</Link></li>
              <li><Link href="/whitepaper" className={FOOTER_LINK_CLS}>Whitepaper</Link></li>
              <li></li>
              <li><Link href="/security" className={FOOTER_LINK_CLS}>Security</Link></li>
            </ul>
          </div>

          <div>
            <div className={FOOTER_EYEBROW_CLS}>{t("footer.legal")}</div>
            <ul className="mt-3 space-y-2 text-sm">
              <li><Link href="/privacy" className={FOOTER_LINK_CLS}>{t("footer.privacy")}</Link></li>
              <li><Link href="/terms" className={FOOTER_LINK_CLS}>{t("footer.terms")}</Link></li>
              <li><Link href="/dpa" className={FOOTER_LINK_CLS}>{t("footer.dpa")}</Link></li>
              <li><Link href="/kvkk" className={FOOTER_LINK_CLS}>{t("footer.kvkk")}</Link></li>
              <li><Link href="/responsible-disclosure" className={FOOTER_LINK_CLS}>{t("footer.disclosure")}</Link></li>
            </ul>
          </div>
        </div>

        <div className="mt-12 flex flex-col items-start justify-between gap-4 border-t border-zinc-200 dark:border-zinc-800 pt-6 text-xs font-mono text-zinc-500 dark:text-zinc-400 sm:flex-row sm:items-center">
          <div>© {new Date().getFullYear()} Tamga. {t("footer.rights")}</div>
          <div className="flex items-center gap-4">
            <span>v0.1.1</span>
            <span className="text-zinc-700">|</span>
            <span className="inline-flex items-center gap-1.5">
              <CircleDot className="h-3 w-3 animate-pulse text-emerald-500" aria-hidden />
              {t("common.all_systems")}
            </span>
            <span className="text-zinc-700">|</span>
            <Link href="/status" className="cursor-pointer transition-colors duration-200 hover:text-zinc-200">
              {t("footer.status")}
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}
