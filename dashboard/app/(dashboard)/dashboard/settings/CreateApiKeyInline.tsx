"use client";

import { useState } from "react";
import { Key } from "lucide-react";
import { type ApiKey } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";

export function CreateApiKeyInline({ onCreate }: { onCreate: (label: string, scope: ApiKey["scope"]) => void }) {
  const [label, setLabel] = useState("");
  const [scope, setScope] = useState<ApiKey["scope"]>("read");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        id="apikey-label-input"
        placeholder="label"
        className="w-28"
      />
      <NativeSelect value={scope} onChange={(e) => setScope(e.target.value as ApiKey["scope"])}>
        <NativeSelectOption value="read">read</NativeSelectOption>
        <NativeSelectOption value="write">write</NativeSelectOption>
        <NativeSelectOption value="admin">admin</NativeSelectOption>
      </NativeSelect>
      <Button
        size="sm"
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
