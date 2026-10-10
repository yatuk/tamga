import { API_BASE } from "@/lib/api/fetch-core";

/**
 * The proxy's live feed of scanned and stopped requests, read in the browser.
 *
 * The browser connects to the proxy itself, as it does for every other call,
 * and sends the caller's own key in the X-Tamga-Admin-Key header. EventSource
 * cannot send a header, which is why this is `fetch` with a streamed body: the
 * key is never in a URL, and no server holds a key on anyone's behalf.
 */

/**
 * - "connecting": the first attempt is on its way.
 * - "open": the proxy accepted the key and the stream is being read.
 * - "error": the stream dropped or the proxy could not be reached; another attempt is scheduled.
 * - "no-key": there is no key to send, so nothing was sent.
 * - "refused": the proxy did not accept the key (401). No further attempt.
 * - "forbidden": the key's role may not read the feed (403). No further attempt.
 * - "closed": the proxy answered in a way another attempt would not change. No further attempt.
 */
export type LiveStreamStatus = "connecting" | "open" | "error" | "no-key" | "refused" | "forbidden" | "closed";

export interface LiveEvent {
  /** The SSE event name, e.g. "request_scanned". */
  type: string;
  data: string;
}

export interface LiveStreamHandlers {
  onEvent: (event: LiveEvent) => void;
  onStatus: (status: LiveStreamStatus) => void;
}

const LIVE_PATH = "/api/v1/live/events";
const FIRST_DELAY_MS = 1_000;
const MAX_DELAY_MS = 30_000;
/** A stream that held this long was a working one: the next drop starts the waits over. */
const STABLE_MS = 5_000;
/** No event is near this size. A stream that sends this much without a line break is not the feed. */
const MAX_BUFFER = 1_000_000;

/** The wait before attempt number `attempt` (0 for the first retry): 1s, 2s, 4s, up to 30s. */
export function retryDelay(attempt: number): number {
  return Math.min(FIRST_DELAY_MS * 2 ** Math.min(attempt, 10), MAX_DELAY_MS);
}

/**
 * Opens the feed and keeps it open until the returned function is called.
 *
 * A dropped stream or an unreachable proxy is tried again, with a longer wait
 * each time. An answer that says the key is the problem (401, 403) ends it:
 * asking again with the same key gets the same answer, so the caller opens a
 * new stream when the key changes. Without a key nothing is sent at all.
 *
 * Closing aborts the request in flight and silences both handlers.
 */
export function openLiveStream(adminKey: string, { onEvent, onStatus }: LiveStreamHandlers): () => void {
  if (!adminKey) {
    onStatus("no-key");
    return () => {};
  }

  const abort = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopped = false;
  let failures = 0;

  const say = (status: LiveStreamStatus) => {
    if (!stopped) onStatus(status);
  };
  /** The last word: nothing is tried or reported after it. */
  const end = (status: LiveStreamStatus) => {
    say(status);
    stopped = true;
  };
  const retry = () => {
    if (stopped) return;
    say("error");
    timer = setTimeout(() => void connect(), retryDelay(failures));
    failures += 1;
  };

  async function connect() {
    if (stopped) return;
    if (failures === 0) say("connecting");

    let res: Response;
    try {
      res = await fetch(`${API_BASE}${LIVE_PATH}`, {
        headers: { Accept: "text/event-stream", "X-Tamga-Admin-Key": adminKey },
        cache: "no-store",
        signal: abort.signal,
      });
    } catch {
      // Not reachable, or aborted by close(); retry() does nothing after close.
      return retry();
    }

    if (!res.ok || !res.body) {
      void res.body?.cancel().catch(() => {});
      if (res.status === 401) return end("refused");
      if (res.status === 403) return end("forbidden");
      // Another 4xx is the proxy's answer to this request and will not change. 408 and 429 pass.
      if (res.status >= 400 && res.status < 500 && res.status !== 408 && res.status !== 429) return end("closed");
      return retry();
    }

    say("open");
    const openedAt = Date.now();
    try {
      await readEvents(res.body, (event) => {
        if (!stopped) onEvent(event);
      });
    } catch {
      // The connection broke mid-stream, or close() aborted it.
    }
    if (Date.now() - openedAt >= STABLE_MS) failures = 0;
    retry();
  }

  void connect();

  return () => {
    stopped = true;
    if (timer !== undefined) clearTimeout(timer);
    abort.abort();
  };
}

/** Reads a text/event-stream body to its end, handing over each event that carries data. */
async function readEvents(body: ReadableStream<Uint8Array>, onEvent: (event: LiveEvent) => void): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let type = "message";
  let data: string[] = [];

  for (;;) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    if (buffer.length > MAX_BUFFER) {
      void reader.cancel().catch(() => {});
      throw new Error("live feed: line too long");
    }

    let end: number;
    while ((end = buffer.indexOf("\n")) >= 0) {
      const line = buffer.slice(0, end).replace(/\r$/, "");
      buffer = buffer.slice(end + 1);

      // A blank line ends an event.
      if (line === "") {
        if (data.length > 0) onEvent({ type, data: data.join("\n") });
        type = "message";
        data = [];
        continue;
      }
      // A comment: the proxy opens the stream with one.
      if (line.startsWith(":")) continue;

      const colon = line.indexOf(":");
      const field = colon < 0 ? line : line.slice(0, colon);
      const text = colon < 0 ? "" : line.slice(colon + 1).replace(/^ /, "");
      if (field === "event") type = text;
      else if (field === "data") data.push(text);
    }
  }
}
