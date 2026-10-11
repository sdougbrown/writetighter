#!/usr/bin/env python3
"""OpenAI-compatible stub endpoint for writetighter verification (offline)."""
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

CANNED_FINDING = {
    "kind": "rewrite",
    "source_text": "Restart the service after changing the file.",
    "source_range": {"start": 0, "end": 44},
    "principle_ids": ["CORE.SHORT_SENTENCE"],
    "reason": "Source states the action but not the expected effect.",
    "replacement": "After you change the file, restart the service so the new configuration takes effect.",
    "confidence": 0.9,
}

class Handler(BaseHTTPRequestHandler):
    def _send(self, payload):
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/v1/models":
            self._send({"object": "list", "data": [
                {"id": "stub-model", "object": "model",
                 "context_length": 8192}]})
        else:
            self.send_error(404)

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        self.rfile.read(n)
        if self.path == "/v1/chat/completions":
            content = json.dumps({"findings": [CANNED_FINDING]})
            self._send({"id": "stub-1", "model": "stub-model", "choices": [
                {"index": 0, "finish_reason": "stop",
                 "message": {"role": "assistant", "content": content}}],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1}})
        else:
            self.send_error(404)

    def log_message(self, *a):
        pass

if __name__ == "__main__":
    HTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
