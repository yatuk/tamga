"""A fake model for the LiteLLM end-to-end test.

It speaks enough of OpenAI's chat completions API for LiteLLM to call it, and
calls nothing itself. What it answers is chosen by a word in the last user
message, so that the test can make "the model" say something Tamga's output
rules act on:

    e2e-answer-tckn    an answer holding a Turkish national id (a test number)
    e2e-answer-email   an answer holding an e-mail address
    anything else      "ok"

GET /_seen returns the user texts it has received, in order, so the test can
tell what reached the model and what did not. Standard library only.
"""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Passes the TCKN checksum and belongs to no one.
TCKN = "10000000146"
EMAIL = "jane.doe@example.com"

_seen = []
_lock = threading.Lock()


def _text_of(content):
    """The text of a message's content, which is a string or a list of parts."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return " ".join(p.get("text", "") for p in content if isinstance(p, dict))
    return ""


def _answer_for(text):
    if "e2e-answer-tckn" in text:
        return "The number on file is " + TCKN + "."
    if "e2e-answer-email" in text:
        return "You can reach her at " + EMAIL + "."
    return "ok"


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def _send(self, status, payload):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self._send(200, {"status": "ok"})
        elif self.path == "/_seen":
            with _lock:
                self._send(200, {"seen": list(_seen)})
        else:
            self._send(404, {"error": "not found"})

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length)
        if not self.path.endswith("/chat/completions"):
            self._send(404, {"error": "not found"})
            return
        try:
            request = json.loads(raw)
        except ValueError:
            self._send(400, {"error": {"message": "invalid JSON", "type": "invalid_request_error"}})
            return

        last_user = ""
        for message in request.get("messages") or []:
            if isinstance(message, dict) and message.get("role") == "user":
                last_user = _text_of(message.get("content"))
        with _lock:
            _seen.append(last_user)

        self._send(200, {
            "id": "chatcmpl-e2e",
            "object": "chat.completion",
            "created": 0,
            "model": request.get("model") or "fake-model",
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": _answer_for(last_user)},
                "finish_reason": "stop",
            }],
            "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
        })

    def log_message(self, fmt, *args):
        # The default log line holds nothing of a body; keep it short anyway.
        print("fake-upstream " + self.command + " " + self.path, flush=True)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
