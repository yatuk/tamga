"use client";

import { useMemo } from "react";
import { Button } from "@/components/ui/button";
import { SAMPLE_INJECTION, SAMPLE_PII, SAMPLE_SECRET } from "@/lib/tamga-simulate";
import { highlightJson, maskLineNumbered, SAMPLE_CLEAN } from "./tryTamgaLiveHelpers";

type Props = {
  text: string;
  setText: (v: string) => void;
  onAnalyze: () => void;
  copiedCurl: boolean;
  copiedJson: boolean;
  onCopyCurl: () => void;
  onCopyJson: () => void;
};

export function TryTamgaLivePayloadColumn({
  text,
  setText,
  onAnalyze,
  copiedCurl,
  copiedJson,
  onCopyCurl,
  onCopyJson,
}: Props) {
  const highlightedInput = useMemo(() => {
    try {
      const parsed = JSON.parse(text);
      return highlightJson(JSON.stringify(parsed, null, 2));
    } catch {
      return "";
    }
  }, [text]);

  return (
    <div className="space-y-3">
      <div className="font-mono text-[11px] uppercase tracking-wide text-zinc-500 dark:text-zinc-400">PAYLOAD INSPECTOR</div>
      <label htmlFor="try-live-input" className="text-xs font-mono text-zinc-600 dark:text-zinc-400">
        request payload
      </label>
      <textarea
        id="try-live-input"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={SAMPLE_CLEAN}
        rows={13}
        className="w-full resize-y rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-3 font-mono text-xs text-zinc-900 dark:text-zinc-100 shadow-sm placeholder:text-zinc-600 dark:text-zinc-400 focus:border-zinc-600 focus:outline-none"
      />
      {highlightedInput && (
        <pre
          className="max-h-32 overflow-auto rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-2 font-mono text-[11px] leading-4"
          dangerouslySetInnerHTML={{ __html: maskLineNumbered(highlightedInput) }}
        />
      )}
      <div className="flex flex-wrap gap-1">
        <Button
          type="button"
          className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
          onClick={() => setText(SAMPLE_PII)}
        >
          PII_TR
        </Button>
        <Button
          type="button"
          className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
          onClick={() => setText(SAMPLE_SECRET)}
        >
          SECRET_AWS
        </Button>
        <Button
          type="button"
          className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
          onClick={() => setText(SAMPLE_INJECTION)}
        >
          INJECTION
        </Button>
        <Button
          type="button"
          className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 font-mono text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
          onClick={() => setText(SAMPLE_CLEAN)}
        >
          CLEAN
        </Button>
      </div>
      <div className="flex gap-1">
        <Button
          type="button"
          variant="destructive" size="md" className="flex-1"
          onClick={onAnalyze}
        >
          ANALYZE
        </Button>
        <Button
          type="button"
          variant="outline" size="md"
          onClick={onCopyCurl}
        >
          {copiedCurl ? "COPIED" : "COPY CURL"}
        </Button>
        <Button
          type="button"
          variant="outline" size="md"
          onClick={onCopyJson}
        >
          {copiedJson ? "COPIED" : "COPY JSON"}
        </Button>
      </div>
    </div>
  );
}
