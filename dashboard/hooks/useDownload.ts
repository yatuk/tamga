"use client";

import { useCallback } from "react";

/** Saves a Blob as a file through a temporary object URL. */
export function useDownload() {
  return useCallback((blob: Blob, filename: string) => {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }, []);
}
