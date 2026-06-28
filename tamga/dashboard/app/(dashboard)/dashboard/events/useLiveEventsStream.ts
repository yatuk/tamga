"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { api } from "@/lib/api";

export type SSEStatus = "connecting" | "open" | "error" | "closed";

export function useLiveEventsStream(adminKey: string) {
  const [liveCount, setLiveCount] = useState(0);
  const [status, setStatus] = useState<SSEStatus>("connecting");
  const reconnectAttempts = useRef(0);
  const esRef = useRef<EventSource | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    if (!adminKey) {
      setStatus("closed");
      return;
    }

    const connect = () => {
      if (!mountedRef.current) return;
      setStatus("connecting");

      const es = api.openLiveEvents(adminKey, () => {
        if (mountedRef.current) {
          setLiveCount((c) => c + 1);
        }
      });

      es.onopen = () => {
        if (mountedRef.current) {
          setStatus("open");
          reconnectAttempts.current = 0;
        }
      };

      es.onerror = () => {
        if (!mountedRef.current) return;
        es.close();
        setStatus("error");

        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), 30000);
        reconnectAttempts.current += 1;

        if (reconnectAttempts.current > 10) {
          setStatus("closed");
          return;
        }

        setTimeout(connect, delay);
      };

      esRef.current = es;
    };

    connect();

    return () => {
      mountedRef.current = false;
      esRef.current?.close();
      esRef.current = null;
    };
  }, [adminKey]);

  const resetCounter = useCallback(() => setLiveCount(0), []);

  return { liveCount, status, resetCounter };
}
