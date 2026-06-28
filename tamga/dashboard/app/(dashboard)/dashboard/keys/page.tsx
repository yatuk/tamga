"use client";

import { KeysBody } from "./KeysBody";
import { useKeysPage } from "./useKeysPage";

export default function KeysPage() {
  const p = useKeysPage();
  return <KeysBody {...p} />;
}
