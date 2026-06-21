"use client";

import Link from "next/link";
import type { ResourceLink } from "./MarketingNavResourceGroups";

export function MegaColumn({
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
              className="group flex cursor-pointer items-start gap-3 rounded-sm px-2 py-2 transition-colors duration-200 hover:bg-zinc-100 dark:hover:bg-zinc-900/80"
            >
              <item.icon
                className="mt-0.5 h-4 w-4 shrink-0 text-zinc-600 dark:text-zinc-400 transition-colors duration-200 group-hover:text-red-400"
                aria-hidden
              />
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
