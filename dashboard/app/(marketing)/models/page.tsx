"use client";

import { useTranslation } from "@/lib/i18n";

type Model = { id: string; input: number; output: number };

const providers: { id: string; label: string; path: string; models: Model[] }[] = [
  {
    id: "openai",
    label: "OpenAI",
    path: "/v1/",
    models: [
      { id: "gpt-4o", input: 2.5, output: 10.0 },
      { id: "gpt-4o-mini", input: 0.15, output: 0.6 },
      { id: "gpt-4.1", input: 2.0, output: 8.0 },
    ],
  },
  {
    id: "anthropic",
    label: "Anthropic",
    path: "/anthropic/",
    models: [
      { id: "claude-3-5-sonnet", input: 3.0, output: 15.0 },
      { id: "claude-3-5-haiku", input: 0.8, output: 4.0 },
      { id: "claude-opus-4", input: 15.0, output: 75.0 },
    ],
  },
  {
    id: "gemini",
    label: "Google Gemini",
    path: "/gemini/",
    models: [
      { id: "gemini-2.0-flash", input: 0.1, output: 0.4 },
      { id: "gemini-1.5-pro", input: 1.25, output: 5.0 },
    ],
  },
  {
    id: "bedrock",
    label: "AWS Bedrock",
    path: "/bedrock/",
    models: [
      { id: "claude-3-5-sonnet-v2", input: 3.0, output: 15.0 },
      { id: "llama-3.1-70b", input: 0.99, output: 0.99 },
    ],
  },
  {
    id: "mistral",
    label: "Mistral",
    path: "/mistral/",
    models: [
      { id: "mistral-large", input: 2.0, output: 6.0 },
      { id: "mistral-small", input: 0.2, output: 0.6 },
    ],
  },
  {
    id: "local",
    label: "Self-hosted (vLLM / Ollama)",
    path: "/local/",
    models: [
      { id: "llama3.1:8b", input: 0, output: 0 },
      { id: "qwen2.5:14b", input: 0, output: 0 },
    ],
  },
];

export default function ModelsPage() {
  const { t } = useTranslation();
  return (
    <main className="mx-auto w-full max-w-6xl px-6 py-16 text-zinc-800 dark:text-zinc-200">
      <p className="font-mono text-[11px] uppercase tracking-[0.2em] text-red-400">
        {t("models.eyebrow")}
      </p>
      <h1 className="mt-2 text-4xl font-extrabold tracking-tight text-white">
        {t("models.title")}
      </h1>
      <p className="mt-4 max-w-2xl text-sm leading-7 text-zinc-600 dark:text-zinc-400">
        {t("models.lede")}
      </p>

      <div className="mt-10 grid gap-6 md:grid-cols-2">
        {providers.map((p) => (
          <div
            key={p.id}
            className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-5"
          >
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-lg font-semibold tracking-tight text-white">{p.label}</h2>
              <code className="rounded-sm bg-zinc-100 dark:bg-zinc-900 px-2 py-0.5 font-mono text-xs text-amber-300">
                {p.path}
              </code>
            </div>
            <div className="divide-y divide-zinc-900">
              {p.models.map((m) => (
                <div
                  key={m.id}
                  className="flex items-center justify-between py-2 text-sm"
                >
                  <span className="font-mono text-zinc-700 dark:text-zinc-300">{m.id}</span>
                  <span className="font-mono text-xs text-zinc-500 dark:text-zinc-400">
                    in <span className="text-zinc-700 dark:text-zinc-300">${m.input.toFixed(2)}</span> / out{" "}
                    <span className="text-zinc-700 dark:text-zinc-300">${m.output.toFixed(2)}</span>
                  </span>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </main>
  );
}
