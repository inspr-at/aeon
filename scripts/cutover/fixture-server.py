# SPDX-License-Identifier: AGPL-3.0-only
"""Serve a fixed classic GET transcript on loopback for the CR1 rehearsal."""

import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixture", type=Path, required=True)
    parser.add_argument("--port-file", type=Path, required=True)
    args = parser.parse_args()
    responses = json.loads(args.fixture.read_text())

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.headers.get("Authorization") != "Bearer cr1-recorded-fixture":
                self.send_error(401)
                return
            if self.path not in responses:
                self.send_error(404, "GET absent from recorded fixture")
                return
            body = json.dumps(responses[self.path], separators=(",", ":")).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_POST(self):
            self.send_error(405)

        def do_PUT(self):
            self.send_error(405)

        def do_PATCH(self):
            self.send_error(405)

        def do_DELETE(self):
            self.send_error(405)

        def log_message(self, *_args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    args.port_file.write_text(str(server.server_port))
    server.serve_forever()


if __name__ == "__main__":
    main()
