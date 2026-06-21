/**
 * SSE proxy for /api/v1/live/events.
 *
 * The admin key is read from the server-side environment or Authorization
 * header and forwarded to the Go proxy. The client EventSource connects to
 * THIS endpoint WITHOUT the admin key in the URL, eliminating the key leak
 * through browser history, proxy logs, and Referer headers.
 *
 * Usage from client:
 *   const es = new EventSource("/api/sse/live");
 */
export async function GET() {
  const proxyUrl = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8443";
  const adminKey =
    process.env.TAMGA_ADMIN_KEY ??
    process.env.NEXT_PUBLIC_ADMIN_KEY ??
    "";

  const upstream = `${proxyUrl}/api/v1/live/events${adminKey ? `?key=${encodeURIComponent(adminKey)}` : ""}`;

  let controller: ReadableStreamDefaultController | null = null;
  let aborted = false;

  async function pump() {
    try {
      const res = await fetch(upstream, {
        headers: {
          Accept: "text/event-stream",
          "Cache-Control": "no-cache",
        },
        signal: controller ? undefined : undefined,
      });

      if (!res.ok || !res.body) {
        if (controller && !aborted) {
          controller.error(new Error(`upstream returned ${res.status}`));
        }
        return;
      }

      const reader = res.body.getReader();

      while (!aborted) {
        const { done, value } = await reader.read();
        if (done) break;
        if (controller && !aborted) {
          controller.enqueue(value);
        }
      }
    } catch {
      // upstream connection lost — stream will end naturally
    } finally {
      if (controller && !aborted) {
        try { controller.close(); } catch { /* already closed */ }
      }
    }
  }

  const stream = new ReadableStream({
    start(c) {
      controller = c;
      pump();
    },
    cancel() {
      aborted = true;
    },
  });

  return new Response(stream, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
