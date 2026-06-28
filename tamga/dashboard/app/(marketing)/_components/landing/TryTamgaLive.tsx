"use client";

import { useMemo, useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SAMPLE_PII, simulateTamga } from "@/lib/tamga-simulate";
import { TryTamgaLiveAnalysisColumn } from "./TryTamgaLiveAnalysisColumn";
import { TryTamgaLivePayloadColumn } from "./TryTamgaLivePayloadColumn";

export function TryTamgaLive() {
  const [text, setText] = useState(SAMPLE_PII);
  const [submittedText, setSubmittedText] = useState(SAMPLE_PII);
  const [submittedAt, setSubmittedAt] = useState<string>("initial");
  const [copied, setCopied] = useState(false);
  const [copiedCurl, setCopiedCurl] = useState(false);
  const [copiedJson, setCopiedJson] = useState(false);

  const result = useMemo(() => simulateTamga(submittedText), [submittedText]);

  function onAnalyze() {
    setSubmittedText(text);
    setSubmittedAt(new Date().toLocaleString("tr-TR"));
  }

  async function onCopyOutput() {
    await navigator.clipboard.writeText(result.masked);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  }

  async function onCopyCurl() {
    const body = (() => {
      try {
        return JSON.stringify(JSON.parse(text));
      } catch {
        return JSON.stringify({ prompt: text });
      }
    })();
    const curl = [
      "curl -X POST https://proxy.tamga.dev/v1/chat/completions \\",
      '  -H "Content-Type: application/json" \\',
      '  -H "Authorization: Bearer $OPENAI_API_KEY" \\',
      '  -H "X-Tamga-Policy: default" \\',
      `  -d '${body.replace(/'/g, "'\\''")}'`,
    ].join("\n");
    await navigator.clipboard.writeText(curl);
    setCopiedCurl(true);
    window.setTimeout(() => setCopiedCurl(false), 1200);
  }

  async function onCopyJson() {
    const pretty = (() => {
      try {
        return JSON.stringify(JSON.parse(text), null, 2);
      } catch {
        return text;
      }
    })();
    await navigator.clipboard.writeText(pretty);
    setCopiedJson(true);
    window.setTimeout(() => setCopiedJson(false), 1200);
  }

  return (
    <section id="try-live" className="scroll-mt-24">
      <div className="space-y-2">
        <div>
          <h2 className="text-3xl font-semibold tracking-tight">Try Tamga Live</h2>
          <p className="mt-1 text-sm text-zinc-600 dark:text-zinc-400">
            Threat-hunting sandbox: payload inspector, analysis meter, and findings table.
          </p>
        </div>

        <Card className="overflow-hidden rounded-sm border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
          <CardHeader className="border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 py-3">
            <CardTitle className="text-base font-mono uppercase tracking-wide text-zinc-800 dark:text-zinc-200">THREAT HUNTING SANDBOX</CardTitle>
            <CardDescription className="font-mono text-xs text-zinc-500 dark:text-zinc-400">
              Patterns: tc_kimlik, phone_tr, email, credit_card, aws_access_key, prompt_injection
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2 p-3 lg:grid-cols-2">
            <TryTamgaLivePayloadColumn
              text={text}
              setText={setText}
              onAnalyze={onAnalyze}
              copiedCurl={copiedCurl}
              copiedJson={copiedJson}
              onCopyCurl={onCopyCurl}
              onCopyJson={onCopyJson}
            />
            <TryTamgaLiveAnalysisColumn
              submittedText={submittedText}
              submittedAt={submittedAt}
              result={result}
              copied={copied}
              onCopyOutput={onCopyOutput}
            />
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
