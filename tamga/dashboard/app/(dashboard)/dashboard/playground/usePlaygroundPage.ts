"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { api, type PolicySimulateResult } from "@/lib/api";
import { toast } from "@/lib/toast";
import { stringifyPolicy } from "../policies/policyUtils";
import { POLICY_DRAFT_STORAGE, type PolicySource } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";
import {
  BUNDLED_REDTEAM,
  classifyOutcome,
  parseRedTeamCsv,
  PLAYGROUND_SNIPPETS,
  type RedTeamRow,
  type RedTeamSample,
} from "./playgroundData";

export function usePlaygroundPage() {
  const [adminKey] = useAdminKey();
  const [prompt, setPrompt] = useState(PLAYGROUND_SNIPPETS[0].text);
  const [policySource, setPolicySource] = useState<PolicySource>("active");
  const [uploadYaml, setUploadYaml] = useState("");
  const [draftYaml, setDraftYaml] = useState("");
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<PolicySimulateResult | null>(null);
  const [batchSamples, setBatchSamples] = useState<RedTeamSample[]>([]);
  const [batchRows, setBatchRows] = useState<RedTeamRow[]>([]);
  const [batchRunning, setBatchRunning] = useState(false);
  const [batchProgress, setBatchProgress] = useState<{ done: number; total: number }>({ done: 0, total: 0 });
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (typeof window === "undefined") return;
    setDraftYaml(window.localStorage.getItem(POLICY_DRAFT_STORAGE) || "");
  }, []);

  const searchParams = useSearchParams();
  const prefillRequestId = searchParams?.get("request_id") || "";
  const [prefilled, setPrefilled] = useState(false);
  useEffect(() => {
    if (!prefillRequestId || prefilled || !adminKey) return;
    let cancelled = false;
    (async () => {
      try {
        const detail = await api.getEventDetail(adminKey, prefillRequestId);
        if (cancelled) return;
        const matches = (detail.findings || [])
          .map((f) => (f.match || "").trim())
          .filter(Boolean);
        const reconstructed = matches.length
          ? `# Reconstructed from incident ${prefillRequestId.slice(0, 12)}\n# (raw prompt is redacted by policy — below are the flagged fragments)\n\n${matches.join("\n")}`
          : `# Incident ${prefillRequestId.slice(0, 12)}\n# No flagged fragments available.`;
        setPrompt(reconstructed);
        setPrefilled(true);
        toast.success(`Loaded incident ${prefillRequestId.slice(0, 12)}`);
      } catch (err) {
        if (cancelled) return;
        toast.error(`Failed to load incident: ${(err as Error).message}`);
        setPrefilled(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [prefillRequestId, prefilled, adminKey]);

  const { data: policies } = useQuery({
    queryKey: ["tamga-policies-playground", adminKey],
    queryFn: () => api.getPolicies(adminKey),
    enabled: !!adminKey,
    retry: 1,
  });
  const activeYaml = useMemo(() => stringifyPolicy(policies?.[0] || null), [policies]);

  const effectiveYaml = policySource === "active" ? activeYaml : policySource === "draft" ? draftYaml : uploadYaml;

  async function runSimulate() {
    if (!prompt.trim()) {
      toast.error("Prompt boş");
      return;
    }
    setRunning(true);
    try {
      const res = await api.simulatePolicy(adminKey, effectiveYaml, prompt);
      setResult(res);
    } catch (e) {
      toast.error("Simulate hatası", (e as Error).message);
    } finally {
      setRunning(false);
    }
  }

  function copyCurl() {
    const base =
      (typeof process !== "undefined" && process.env.NEXT_PUBLIC_TAMGA_API_URL) || "http://localhost:8080";
    const payload = JSON.stringify({ policy_yaml: effectiveYaml, sample_text: prompt });
    const cmd = `curl -X POST ${base}/api/v1/policies/simulate \\
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${payload.replaceAll("'", "\\'")}'`;
    navigator.clipboard.writeText(cmd);
    toast.success("COPY CURL", "clipboard");
  }

  function copyJson() {
    const payload = JSON.stringify({ policy_yaml: effectiveYaml, sample_text: prompt }, null, 2);
    navigator.clipboard.writeText(payload);
    toast.success("COPY JSON", "clipboard");
  }

  function loadBundledSamples() {
    setBatchSamples(BUNDLED_REDTEAM);
    setBatchRows([]);
    toast.success("RED TEAM", `${BUNDLED_REDTEAM.length} sample loaded`);
  }

  function onUploadCsv(ev: React.ChangeEvent<HTMLInputElement>) {
    const file = ev.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const samples = parseRedTeamCsv(String(reader.result || ""));
      if (samples.length === 0) {
        toast.error("CSV empty", "no valid rows (expected id,category,expected,prompt)");
        return;
      }
      setBatchSamples(samples);
      setBatchRows([]);
      toast.success("RED TEAM", `${samples.length} sample loaded from ${file.name}`);
    };
    reader.readAsText(file);
    ev.target.value = "";
  }

  async function runBatch() {
    if (batchSamples.length === 0) {
      toast.error("Red team boş", "Önce örnek yükleyin veya CSV seçin");
      return;
    }
    setBatchRunning(true);
    setBatchProgress({ done: 0, total: batchSamples.length });
    const results: RedTeamRow[] = [];
    for (let i = 0; i < batchSamples.length; i++) {
      const s = batchSamples[i];
      try {
        const res = await api.simulatePolicy(adminKey, effectiveYaml, s.prompt);
        const maxConf = (res.findings || []).reduce((acc, f) => Math.max(acc, f.confidence || 0), 0);
        results.push({
          ...s,
          actual: res.action || "PASS",
          outcome: classifyOutcome(s.expected, res.action || "PASS"),
          confidence: maxConf,
        });
      } catch (err) {
        results.push({
          ...s,
          actual: "ERROR",
          outcome: "error",
          confidence: 0,
          error: (err as Error).message,
        });
      }
      setBatchProgress({ done: i + 1, total: batchSamples.length });
    }
    setBatchRows(results);
    setBatchRunning(false);
    const miss = results.filter((r) => r.outcome === "miss").length;
    const fp = results.filter((r) => r.outcome === "fp").length;
    toast.success("RED TEAM DONE", `miss ${miss} · fp ${fp} · n ${results.length}`);
  }

  const batchSummary = useMemo(() => {
    if (batchRows.length === 0) return null;
    let tp = 0,
      fp = 0,
      fn = 0,
      tn = 0,
      err = 0;
    for (const r of batchRows) {
      switch (r.outcome) {
        case "match":
          tp++;
          break;
        case "fp":
          fp++;
          break;
        case "miss":
          fn++;
          break;
        case "tn":
          tn++;
          break;
        case "error":
          err++;
          break;
      }
    }
    const precision = tp + fp > 0 ? tp / (tp + fp) : 0;
    const recall = tp + fn > 0 ? tp / (tp + fn) : 0;
    const f1 = precision + recall > 0 ? (2 * precision * recall) / (precision + recall) : 0;
    return { tp, fp, fn, tn, err, precision, recall, f1 };
  }, [batchRows]);

  return {
    prompt,
    setPrompt,
    policySource,
    setPolicySource,
    uploadYaml,
    setUploadYaml,
    running,
    result,
    batchSamples,
    batchRows,
    batchRunning,
    batchProgress,
    fileInputRef,
    effectiveYaml,
    batchSummary,
    copyCurl,
    copyJson,
    runSimulate,
    loadBundledSamples,
    onUploadCsv,
    runBatch,
  };
}
