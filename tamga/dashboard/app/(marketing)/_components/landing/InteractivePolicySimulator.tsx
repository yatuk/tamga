"use client";

import { useMemo, useState } from "react";
import { simulateTamga, SAMPLE_PII, SAMPLE_SECRET, SAMPLE_INJECTION } from "@/lib/tamga-simulate";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toUpperEn } from "@/lib/utils/tr-string";

const DEFAULT_POLICY = `version: "1.0"
name: default
rules:
  - id: block-secrets
    when:
      type: secret
    action: BLOCK
  - id: redact-pii-tr
    when:
      type: pii
      category_in: [tc_kimlik, phone_tr, email, credit_card]
    action: REDACT
  - id: block-prompt-injection
    when:
      type: injection
    action: BLOCK
  - id: default-pass
    action: PASS
`;

type Rule = {
  id: string;
  when?: { type?: string; category_in?: string[] };
  action: "BLOCK" | "REDACT" | "LOG" | "PASS";
};

function parseSimpleYaml(yaml: string): Rule[] {
  // Intentionally tiny heuristic parser — enough for the marketing sim and
  // robust to the format above; not a general YAML implementation.
  const out: Rule[] = [];
  let current: Partial<Rule> | null = null;
  let inWhen = false;
  for (const rawLine of yaml.split("\n")) {
    const line = rawLine.replace(/\t/g, "  ");
    if (/^\s*-\s*id:\s*(\S+)/.test(line)) {
      if (current) out.push(finalizeRule(current));
      const m = line.match(/id:\s*(\S+)/);
      current = { id: m?.[1] ?? "rule", action: "PASS" };
      inWhen = false;
      continue;
    }
    if (!current) continue;
    if (/^\s{4,}when:\s*$/.test(line)) {
      current.when = {};
      inWhen = true;
      continue;
    }
    if (/^\s{4,}action:\s*(\w+)/.test(line)) {
      const m = line.match(/action:\s*(\w+)/);
      const act = toUpperEn(m?.[1] || "PASS");
      if (act === "BLOCK" || act === "REDACT" || act === "LOG" || act === "PASS") {
        current.action = act;
      }
      inWhen = false;
      continue;
    }
    if (inWhen) {
      const t = line.match(/type:\s*(\S+)/);
      if (t) current.when = { ...(current.when ?? {}), type: t[1] };
      const cs = line.match(/category_in:\s*\[(.*?)\]/);
      if (cs) {
        const list = cs[1].split(",").map((s) => s.trim().replace(/['"]/g, "")).filter(Boolean);
        current.when = { ...(current.when ?? {}), category_in: list };
      }
    }
  }
  if (current) out.push(finalizeRule(current));
  return out;
}

function finalizeRule(r: Partial<Rule>): Rule {
  return {
    id: r.id ?? "rule",
    when: r.when,
    action: r.action ?? "PASS",
  };
}

function chooseAction(rules: Rule[], f: { type: string; category: string }): Rule | null {
  for (const rule of rules) {
    if (!rule.when) return rule; // catch-all
    if (rule.when.type && rule.when.type !== f.type) continue;
    if (rule.when.category_in && !rule.when.category_in.includes(f.category)) continue;
    return rule;
  }
  return null;
}

function actionClass(a: Rule["action"]) {
  switch (a) {
    case "BLOCK":
      return "border-red-500/30 bg-red-500/10 text-red-400";
    case "REDACT":
      return "border-amber-500/30 bg-amber-500/10 text-amber-300";
    case "LOG":
      return "border-sky-500/30 bg-sky-500/10 text-sky-300";
    default:
      return "border-emerald-500/30 bg-emerald-500/10 text-emerald-400";
  }
}

export function InteractivePolicySimulator() {
  const [policy, setPolicy] = useState(DEFAULT_POLICY);
  const [prompt, setPrompt] = useState(SAMPLE_PII);

  const rules = useMemo(() => parseSimpleYaml(policy), [policy]);
  const sim = useMemo(() => simulateTamga(prompt), [prompt]);

  const evaluated = useMemo(() => {
    return sim.findings.map((f) => {
      const hit = chooseAction(rules, f);
      return { finding: f, rule: hit };
    });
  }, [sim.findings, rules]);

  const finalAction = useMemo<Rule["action"]>(() => {
    let worst: Rule["action"] = "PASS";
    const order: Record<Rule["action"], number> = { PASS: 0, LOG: 1, REDACT: 2, BLOCK: 3 };
    for (const e of evaluated) {
      if (!e.rule) continue;
      if (order[e.rule.action] > order[worst]) worst = e.rule.action;
    }
    return worst;
  }, [evaluated]);

  return (
    <section id="policy-simulator" className="scroll-mt-24">
      <div className="space-y-2">
        <div>
          <h2 className="text-3xl font-semibold tracking-tight">Policy Simulator</h2>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
            Politikayı YAML ile yazın, örnek prompt&apos;a canlı aksiyon ve finding çıktısı alın — client-side demo.
          </p>
        </div>

        <Card className="rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
          <CardHeader className="border-b border-zinc-200 dark:border-zinc-800 py-3">
            <CardTitle className="font-mono text-sm uppercase text-zinc-800 dark:text-zinc-200">Policy editor</CardTitle>
            <CardDescription className="font-mono text-xs text-zinc-500 dark:text-zinc-400">
              YAML → finding eşleşmesi; LOG/REDACT/BLOCK önceliğine göre final action hesaplanır.
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 p-3 lg:grid-cols-2">
            <div className="space-y-2">
              <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">policy.yaml</div>
              <textarea
                value={policy}
                onChange={(e) => setPolicy(e.target.value)}
                rows={16}
                className="w-full resize-y rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 font-mono text-xs text-zinc-900 dark:text-zinc-100"
              />
              <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">sample prompt</div>
              <textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                rows={5}
                className="w-full resize-y rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 font-mono text-xs text-zinc-900 dark:text-zinc-100"
              />
              <div className="flex flex-wrap gap-1">
                <Button
                  type="button"
                  variant="outline" size="sm"
                  onClick={() => setPrompt(SAMPLE_PII)}
                >
                  PII
                </Button>
                <Button
                  type="button"
                  variant="outline" size="sm"
                  onClick={() => setPrompt(SAMPLE_SECRET)}
                >
                  SECRET
                </Button>
                <Button
                  type="button"
                  variant="outline" size="sm"
                  onClick={() => setPrompt(SAMPLE_INJECTION)}
                >
                  INJECTION
                </Button>
              </div>
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">Final action</div>
                <Badge className={`rounded-sm border font-mono ${actionClass(finalAction)}`}>{finalAction}</Badge>
              </div>

              <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 text-xs">
                {evaluated.length === 0 ? (
                  <div className="font-mono text-zinc-500 dark:text-zinc-400">Finding yok — clean prompt.</div>
                ) : (
                  <table className="w-full font-mono text-[11px]">
                    <thead className="text-zinc-500 dark:text-zinc-400">
                      <tr>
                        <th className="px-1 py-1 text-left">type</th>
                        <th className="px-1 py-1 text-left">category</th>
                        <th className="px-1 py-1 text-left">rule</th>
                        <th className="px-1 py-1 text-left">action</th>
                      </tr>
                    </thead>
                    <tbody>
                      {evaluated.map((e, i) => (
                        <tr key={i} className="border-t border-zinc-200 dark:border-zinc-800 text-zinc-800 dark:text-zinc-200">
                          <td className="px-1 py-1">{e.finding.type}</td>
                          <td className="px-1 py-1">{e.finding.category}</td>
                          <td className="px-1 py-1 text-zinc-600 dark:text-zinc-400">{e.rule?.id ?? "—"}</td>
                          <td className="px-1 py-1">
                            <Badge className={`rounded-sm border font-mono ${actionClass(e.rule?.action ?? "PASS")}`}>
                              {e.rule?.action ?? "PASS"}
                            </Badge>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900 p-2 font-mono text-[11px] text-zinc-700 dark:text-zinc-300">
                {rules.length} rule &middot; {evaluated.length} finding &middot; {sim.riskPct}% risk
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
