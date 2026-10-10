"use client";

import { useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import type { LiveStreamStatus } from "@/lib/api/live-stream";

export type SSEStatus = LiveStreamStatus;

/**
 * Counts the requests the proxy has scanned since the list was last read.
 *
 * The feed is opened with the key this browser uses for everything else.
 * Without one it is not opened; when the proxy refuses the key it stays shut
 * until the key changes. Leaving the page closes it.
 */
export function useLiveEventsStream(adminKey: string) {
  const [liveCount, setLiveCount] = useState(0);
  const [status, setStatus] = useState<SSEStatus>("connecting");

  useEffect(
    () =>
      api.openLiveEvents(adminKey, {
        onEvent: () => setLiveCount((c) => c + 1),
        onStatus: setStatus,
      }),
    [adminKey],
  );

  const resetCounter = useCallback(() => setLiveCount(0), []);

  return { liveCount, status, resetCounter };
}
