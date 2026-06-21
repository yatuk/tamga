"use client";

import { AlertCircle, AlertTriangle, CircleAlert, Info } from "lucide-react";
import type { SimAction, SimFinding } from "@/lib/tamga-simulate";

export const SAMPLE_CLEAN = `{
  "model": "claude-sonnet-4",
  "messages": [
    { "role": "user", "content": "Merhaba, bugun hava nasil?" }
  ]
}`;

export function actionBadge(action: SimAction) {
  switch (action) {
    case "BLOCK":
      return "rounded-sm border border-red-500/30 bg-red-500/10 text-red-500";
    case "REDACT":
      return "rounded-sm border border-amber-500/30 bg-amber-500/10 text-amber-500";
    default:
      return "rounded-sm border border-emerald-500/30 bg-emerald-500/10 text-emerald-500";
  }
}

export function severityBadge(severity: SimFinding["severity"]) {
  if (severity === "critical") return "rounded-sm bg-[#dc2626] text-red-50";
  if (severity === "high") return "rounded-sm bg-[#d97706] text-amber-50";
  if (severity === "medium") return "rounded-sm bg-[#ca8a04] text-yellow-50";
  return "rounded-sm bg-[#0ea5e9] text-sky-50";
}

export function SeverityIcon({ severity }: { severity: SimFinding["severity"] }) {
  if (severity === "critical") return <AlertTriangle className="h-3.5 w-3.5" />;
  if (severity === "high") return <CircleAlert className="h-3.5 w-3.5" />;
  if (severity === "medium") return <AlertCircle className="h-3.5 w-3.5" />;
  return <Info className="h-3.5 w-3.5" />;
}

export function findingRiskScore(severity: SimFinding["severity"]) {
  if (severity === "critical") return 96;
  if (severity === "high") return 84;
  if (severity === "medium") return 67;
  return 35;
}

export function findingAction(f: SimFinding) {
  if (f.severity === "critical") return "BLOCK";
  if (f.severity === "high") return "REDACT";
  if (f.severity === "medium") return "WARN";
  return "PASS";
}

export function findingConfidence(severity: SimFinding["severity"]) {
  if (severity === "critical") return 0.95;
  if (severity === "high") return 0.89;
  if (severity === "medium") return 0.71;
  return 0.52;
}

export function meterColor(score: number) {
  if (score >= 90) return "bg-[#dc2626]";
  if (score >= 75) return "bg-[#d97706]";
  if (score >= 60) return "bg-[#ca8a04]";
  return "bg-[#0ea5e9]";
}

export function hashToReqId(input: string) {
  let h = 2166136261;
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return `req_${(h >>> 0).toString(16).padStart(8, "0").slice(0, 8)}`;
}

export function maskLineNumbered(input: string) {
  return input
    .split("\n")
    .map((line, idx) => `${String(idx + 1).padStart(2, "0")}  ${line}`)
    .join("\n");
}

function escapeHtml(text: string) {
  return text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

export function highlightJson(text: string) {
  const escaped = escapeHtml(text);
  return escaped
    .replace(/"(.*?)"(?=\s*:)/g, '<span class="text-sky-400">"$1"</span>')
    .replace(/:\s*"(.*?)"/g, ': <span class="text-emerald-400">"$1"</span>')
    .replace(/:\s*(\d+(\.\d+)?)/g, ': <span class="text-amber-400">$1</span>')
    .replace(/\b(true|false|null)\b/g, '<span class="text-amber-400">$1</span>');
}
