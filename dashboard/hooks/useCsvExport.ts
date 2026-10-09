"use client";

import { useCallback } from "react";
import { saveBlob } from "@/lib/utils/download";

interface CsvExportOptions {
  /** If true, each cell value is wrapped in double quotes with proper escaping. */
  quote?: boolean;
}

/**
 * Reusable CSV export hook. Creates a Blob download link for the given
 * rows and triggers a browser download.
 */
export function useCsvExport() {
  const exportCsv = useCallback(
    (filename: string, headers: string[], rows: string[][], options?: CsvExportOptions) => {
      const quote = options?.quote ?? false;

      const esc = (v: string): string =>
        quote ? `"${v.replaceAll('"', '""')}"` : v;

      const headerLine = headers.map(esc).join(",");
      const bodyLines = rows.map((row) => row.map(esc).join(","));
      const csv = [headerLine, ...bodyLines].join("\n");

      saveBlob(new Blob([csv], { type: "text/csv;charset=utf-8;" }), filename);
    },
    [],
  );

  return { exportCsv };
}
