"use client";

import { useState } from "react";
import { Key } from "lucide-react";
import { type ApiKey } from "@/lib/api";
import { Button } from "@/components/ui/button";

export function CreateApiKeyInline({ onCreate }: { onCreate: (label: string, scope: ApiKey["scope"]) => void }) {
  const [label, setLabel] = useState("");
  const [scope, setScope] = useState<ApiKey["scope"]>("read");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <input
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        id="apikey-label-input" placeholder="label"
        className="h-8 w-28 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 text-xs text-zinc-900 dark:text-zinc-100 focus:outline-none"
      />
      <select
        value={scope}
        onChange={(e) => setScope(e.target.value as ApiKey["scope"])}
        className="h-8 cursor-pointer rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 text-xs text-zinc-900 dark:text-zinc-100 focus:outline-none"
      >
        <option value="read">read</option>
        <option value="write">write</option>
        <option value="admin">admin</option>
      </select>
      <Button
        className="h-8 cursor-pointer rounded-sm bg-red-600 px-3 text-white hover:bg-red-700"
        onClick={() => {
          onCreate(label, scope);
          setLabel("");
        }}
      >
        <Key className="mr-1 h-3.5 w-3.5" /> New
      </Button>
    </div>
  );
}
