"use client";

import { useCallback } from "react";
import { saveBlob } from "@/lib/utils/download";

/** Saves a Blob as a file. */
export function useDownload() {
  return useCallback(saveBlob, []);
}
