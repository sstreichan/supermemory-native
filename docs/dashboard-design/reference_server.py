#!/usr/bin/env python3
"""Serves the reference dashboard (docs/supermemory/) for design baseline screenshots.

The reference index.html is served at "/" and local-console.js at "/local-console.js".
The two API routes the reference console calls (/v3/documents/documents,
/v3/container-tags/list) are answered with empty payloads so the "Memory" tab
renders its empty state instead of an error. Everything else is static.

Usage:
    REF_ROOT=<repo>/docs/supermemory REF_PORT=8799 python reference_server.py
"""

import http.server
import json
import os
import socketserver

ROOT = os.environ["REF_ROOT"]
PORT = int(os.environ.get("REF_PORT", "8799"))

EMPTY_DOCUMENTS = {
    "documents": [],
    "pagination": {"totalItems": 0, "totalPages": 1, "currentPage": 1},
}


class ReferenceHandler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=ROOT, **kwargs)

    def _json(self, obj):
        body = json.dumps(obj).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        if length:
            self.rfile.read(length)
        if self.path == "/v3/documents/documents":
            return self._json(EMPTY_DOCUMENTS)
        if self.path == "/v3/container-tags/list":
            return self._json([])
        return self._json({})

    def do_GET(self):
        if self.path.startswith("/v3/container-tags/list"):
            return self._json([])
        return super().do_GET()

    def log_message(self, *args):
        pass


with socketserver.TCPServer(("127.0.0.1", PORT), ReferenceHandler) as httpd:
    httpd.serve_forever()
