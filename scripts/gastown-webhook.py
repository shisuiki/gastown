#!/usr/bin/env python3
"""
Simple webhook server for GitHub Actions to trigger gastown sync.

Usage:
    python3 gastown-webhook.py [port]

GitHub Actions can POST to:
    http://your-server:9876/sync?token=YOUR_TOKEN

Set GASTOWN_WEBHOOK_TOKEN env var for authentication.
"""

import http.server
import subprocess
import os
import sys
import json
from urllib.parse import urlparse, parse_qs

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 9876
TOKEN = os.environ.get('GASTOWN_WEBHOOK_TOKEN', '')
SYNC_SCRIPT = os.path.expanduser('~/gt/scripts/gastown-sync.sh')

class WebhookHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        parsed = urlparse(self.path)

        if parsed.path != '/sync':
            self.send_error(404)
            return

        # Check token if configured
        if TOKEN:
            query = parse_qs(parsed.query)
            req_token = query.get('token', [''])[0]
            if req_token != TOKEN:
                self.send_error(401, 'Invalid token')
                return

        # Trigger sync
        try:
            result = subprocess.run(
                [SYNC_SCRIPT, 'sync'],
                capture_output=True,
                text=True,
                timeout=120
            )
            response = {
                'success': result.returncode == 0,
                'stdout': result.stdout,
                'stderr': result.stderr
            }
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())
        except Exception as e:
            self.send_error(500, str(e))

    def do_GET(self):
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()
            self.wfile.write(b'ok')
        else:
            self.send_error(404)

    def log_message(self, format, *args):
        print(f"[webhook] {args[0]}")

if __name__ == '__main__':
    print(f"Starting webhook server on port {PORT}")
    print(f"Token auth: {'enabled' if TOKEN else 'disabled'}")
    server = http.server.HTTPServer(('0.0.0.0', PORT), WebhookHandler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down")
