"use client";

import { CheckCircle2, Fingerprint, Link2, Lock, ShieldAlert, ShieldCheck } from "lucide-react";
import { humanizeAuditKind } from "@/lib/humanize";

// ── Types ──────────────────────────────────────────────────────────────────────

interface ChainBlock {
  index: number;
  hash: string;
  prevHash: string;
  kind: string;
  timestamp: string;
  actor?: string;
  verified: boolean;
}

interface AuditChainVizProps {
  blocks?: ChainBlock[];
  chainOk?: boolean;
  brokenAt?: number;
  totalEntries?: number;
  className?: string;
}

// ── Helpers ────────────────────────────────────────────────────────────────────

function hashShort(hash: string): string {
  if (!hash || hash.length < 16) return hash || "—";
  return `${hash.slice(0, 8)}…${hash.slice(-6)}`;
}

// ── Demo data ──────────────────────────────────────────────────────────────────

const DEMO_CHAIN: ChainBlock[] = [
  { index: 0, hash: "a3f2b9d1c8e7f6a5", prevHash: "0000000000000000", kind: "genesis", timestamp: "2025-06-10T08:00:00Z", verified: true },
  { index: 1, hash: "b4c3d2e1f0a9b8c7", prevHash: "a3f2b9d1c8e7f6a5", kind: "policy.update", timestamp: "2025-06-10T08:15:22Z", actor: "admin", verified: true },
  { index: 2, hash: "c5d4e3f2a1b0c9d8", prevHash: "b4c3d2e1f0a9b8c7", kind: "apikey.create", timestamp: "2025-06-10T09:30:45Z", actor: "sre-bot", verified: true },
  { index: 3, hash: "d6e5f4a3b2c1d0e9", prevHash: "c5d4e3f2a1b0c9d8", kind: "incident.block", timestamp: "2025-06-10T10:12:03Z", actor: "proxy", verified: true },
  { index: 4, hash: "e7f6a5b4c3d2e1f0", prevHash: "d6e5f4a3b2c1d0e9", kind: "webhook.test", timestamp: "2025-06-10T11:05:18Z", actor: "admin", verified: true },
  { index: 5, hash: "f8a7b6c5d4e3f2a1", prevHash: "e7f6a5b4c3d2e1f0", kind: "team.invite", timestamp: "2025-06-10T12:45:00Z", actor: "owner", verified: true },
];

// ── Main Export ────────────────────────────────────────────────────────────────

export function AuditChainViz({
  blocks = DEMO_CHAIN,
  chainOk = true,
  brokenAt,
  totalEntries,
  className = "",
}: AuditChainVizProps) {
  const displayBlocks = blocks.slice(-8); // Show last 8 blocks
  const count = totalEntries ?? blocks.length;

  return (
    <div className={`rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2.5">
        <div className="flex items-center gap-2">
          <Fingerprint className="h-3.5 w-3.5 text-zinc-500" />
          <span className="text-[11px] font-semibold uppercase tracking-[0.1em] text-zinc-700 dark:text-zinc-300">
            Hash Chain
          </span>
        </div>
        <div className="flex items-center gap-3">
          {/* Chain status badge */}
          <span
            className={`inline-flex items-center gap-1 rounded-sm border px-2 py-0.5 text-[10px] ${
              chainOk
                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
                : "border-red-500/30 bg-red-500/10 text-red-400"
            }`}
          >
            {chainOk ? (
              <ShieldCheck className="h-3 w-3" />
            ) : (
              <ShieldAlert className="h-3 w-3" />
            )}
            {chainOk ? "CHAIN VERIFIED" : `BROKEN @ #${brokenAt ?? "?"}`}
          </span>
          <span className="text-[10px] tabular-nums text-zinc-600 dark:text-zinc-400">
            {count} blocks
          </span>
        </div>
      </div>

      {/* Chain visualization */}
      <div className="relative px-4 py-4">
        {/* Vertical chain line */}
        <div className="absolute left-[31px] top-4 bottom-4 w-0.5 bg-zinc-200 dark:bg-zinc-800" aria-hidden />

        <div className="space-y-1">
          {displayBlocks.map((block, i) => {
            const isLast = i === displayBlocks.length - 1;
            const isBroken = !chainOk && brokenAt !== undefined && block.index === brokenAt;

            return (
              <div
                key={block.index}
                className="relative flex items-start gap-4"
              >
                {/* Node on the chain line */}
                <div className="relative z-10 flex shrink-0 flex-col items-center">
                  <div
                    className={`flex h-5 w-5 items-center justify-center rounded-full border-2 ${
                      isBroken
                        ? "border-red-500 bg-red-500/10 text-red-400"
                        : "border-emerald-500/50 bg-emerald-500/10 text-emerald-400"
                    }`}
                  >
                    {isBroken ? (
                      <ShieldAlert className="h-2.5 w-2.5" />
                    ) : (
                      <CheckCircle2 className="h-2.5 w-2.5" />
                    )}
                  </div>
                </div>

                {/* Block content */}
                <div
                  className={`flex-1 rounded-sm border p-2.5 ${
                    isBroken
                      ? "border-red-500/20 bg-red-500/[0.04]"
                      : isLast
                        ? "border-emerald-500/15 bg-emerald-500/[0.03]"
                        : "border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40"
                  }`}
                >
                  {/* Top row: index + kind + time */}
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span className="text-[10px] font-semibold tabular-nums text-zinc-600 dark:text-zinc-400">
                        #{block.index}
                      </span>
                      <span
                        className={`rounded-sm border px-1.5 py-0.5 text-[9px] ${
                          block.kind === "genesis"
                            ? "border-zinc-400/30 bg-zinc-400/10 text-zinc-400"
                            : block.kind.startsWith("policy")
                              ? "border-amber-500/30 bg-amber-500/10 text-amber-400"
                              : block.kind.startsWith("incident")
                                ? "border-sky-500/30 bg-sky-500/10 text-sky-400"
                                : "border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400"
                        }`}
                      >
                        {humanizeAuditKind(block.kind)}
                      </span>
                    </div>
                    <span className="text-[9px] text-zinc-600 dark:text-zinc-400">
                      {new Date(block.timestamp).toLocaleTimeString("tr-TR", { hour: "2-digit", minute: "2-digit" })}
                    </span>
                  </div>

                  {/* Hash row */}
                  <div className="mt-1.5 space-y-0.5">
                    <div className="flex items-center gap-2 text-[9px]">
                      <span className="text-zinc-600 dark:text-zinc-400">hash</span>
                      <code className="text-zinc-600 dark:text-zinc-400">{hashShort(block.hash)}</code>
                      {isLast && chainOk && (
                        <span title="Cryptographically sealed">
                          <Lock className="h-2.5 w-2.5 text-emerald-500" />
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-1 text-[9px]">
                      <span className="text-zinc-600 dark:text-zinc-400">← prev</span>
                      <code className="text-zinc-700">{hashShort(block.prevHash)}</code>
                      {i > 0 && (
                        <span title={`Linked to block #${block.index - 1}`}>
                          <Link2 className="h-2.5 w-2.5 text-zinc-700" />
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Actor */}
                  {block.actor && (
                    <div className="mt-1 text-[9px] text-zinc-600 dark:text-zinc-400">
                      by: <span className="text-zinc-600 dark:text-zinc-400">{block.actor}</span>
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Footer — tamper-evident explanation */}
      <div className="flex items-center gap-2 border-t border-zinc-200 dark:border-zinc-800 px-4 py-2 text-[9px] text-zinc-600 dark:text-zinc-400">
        <Lock className="h-3 w-3" />
        Each block is cryptographically linked to the previous via SHA-256 hash.
        Any tampering breaks the chain and is immediately detectable.
      </div>
    </div>
  );
}
