"use client";

import { Suspense, useEffect, useRef, useState, type ReactNode } from "react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { motion } from "motion/react";
import {
  ActionableNotification,
  Header,
  HeaderGlobalAction,
  HeaderGlobalBar,
  HeaderName,
  SideNav,
  SideNavItems,
  SkeletonText,
  SkipToContent,
  Theme,
} from "@carbon/react";
import { Search } from "@carbon/icons-react";
import { EXPRESSIVE, Motion } from "./motion";
import { findNavItem, navGroups } from "./nav";
import { useProxyHealth } from "./use-proxy-health";
import { UserMenu } from "./user-menu";

// The palette is only needed once it is opened.
const CommandPalette = dynamic(() => import("./palette").then((m) => m.CommandPalette), { ssr: false });

/** The console frame: an ink header, the section list, and the page. */
export function TamgaShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const current = findNavItem(pathname)?.item.href;
  const health = useProxyHealth();
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [paletteLoaded, setPaletteLoaded] = useState(false);
  const router = useRouter();
  // What had the focus when the palette opened gets it back when it closes.
  const opener = useRef<HTMLElement | null>(null);
  const remember = () => {
    const el = document.activeElement;
    if (el instanceof HTMLElement && !el.closest(".cds--modal")) opener.current = el;
  };
  useEffect(() => {
    if (!paletteOpen) opener.current?.focus();
  }, [paletteOpen]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        remember();
        setPaletteLoaded(true);
        setPaletteOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <Motion>
      <Theme theme="g10">
        <Theme theme="g100">
          <Header aria-label="Tamga">
            <SkipToContent href="#main" />
            <HeaderName as={Link} href="/dashboard" prefix="">
              <span className="t-brand">
                TAMGA<i>.</i>
              </span>
            </HeaderName>
            <HeaderGlobalBar>
              <span className="t-shellstate">
                <span className="t-dot" data-state={health.up === undefined ? "checking" : health.up ? "up" : "down"} />
                {/* Only up or down is announced; the scan time changes with every check. */}
                <span role="status">{health.up === undefined ? "Checking the proxy" : health.up ? "Proxy up" : "Proxy unreachable"}</span>
                {health.up && health.scanP50 !== null ? <span>scanning in {health.scanP50} ms</span> : null}
              </span>
              <HeaderGlobalAction
                aria-label="Search or jump to a page (Ctrl K)"
                tooltipAlignment="end"
                onClick={() => {
                  remember();
                  setPaletteLoaded(true);
                  setPaletteOpen(true);
                }}
              >
                <Search size={20} />
              </HeaderGlobalAction>
              <span className="t-user">
                <UserMenu />
              </span>
            </HeaderGlobalBar>
          </Header>
        </Theme>

        <SideNav aria-label="Sections" isFixedNav expanded>
          <SideNavItems>
            {navGroups.map((group, i) => (
              <li key={group.label ?? i} className="t-navgroup">
                {group.label ? <span className="t-navlabel">{group.label}</span> : null}
                <ul>
                  {group.items.map((item) => {
                    const active = item.href === current;
                    return (
                      <li key={item.href} className="t-navitem">
                        {/* One mark, shared by every item: it travels to the page you open. */}
                        {active ? (
                          <motion.span layoutId="t-navmark" className="t-navmark" transition={{ duration: 0.32, ease: EXPRESSIVE }} />
                        ) : null}
                        <Link href={item.href} className="t-navlink" aria-current={active ? "page" : undefined}>
                          {item.label}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </li>
            ))}
          </SideNavItems>
        </SideNav>

        <main id="main" tabIndex={-1} className="t-main">
          {health.up === false ? (
            <ActionableNotification
              kind="error"
              lowContrast
              inline
              hideCloseButton
              title="The proxy is not answering."
              subtitle={health.reason ? `${health.reason}. Requests may be going out unscanned.` : "Requests may be going out unscanned."}
              actionButtonLabel="Proxy status"
              onActionButtonClick={() => router.push("/dashboard/proxy")}
              style={{ maxWidth: "none", marginBottom: "1.5rem" }}
            />
          ) : null}
          {/* Pages read filters from the URL, which suspends during prerender. */}
          <Suspense fallback={<SkeletonText paragraph lineCount={6} />}>
            {children}
          </Suspense>
        </main>

        {paletteLoaded ? <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} /> : null}
      </Theme>
    </Motion>
  );
}
