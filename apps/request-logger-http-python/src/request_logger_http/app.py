from __future__ import annotations

import json
import os
import socket
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse
from urllib.request import urlopen





class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/ready":
            self.ready()
            return
        if self.path == "/demo":
            self.demo()
            return
        self.send_response(404)
        self.end_headers()
        self.wfile.write(b"not found\n")

    def ready(self) -> None:
        todos = check_todos_api()
        cache = check_redis()
        if todos is None and cache is None:
            self.send_response(204)
            self.end_headers()
            return
        self.send_response(503)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"todosApi": status(todos), "cache": status(cache)}).encode())

    def demo(self) -> None:
        todos = check_todos_api()
        cache = check_redis()
        healthy = todos is None and cache is None
        self.send_response(200 if healthy else 500)
        self.send_header("content-type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"todosApi": status(todos), "cache": status(cache)}).encode())

    def log_message(self, format: str, *args: object) -> None:
        return


def check_todos_api() -> Exception | None:
    base_url = os.getenv("TODOS_API_URL", "").rstrip("/")
    if not base_url:
        return RuntimeError("TODOS_API_URL is not set")
    try:
        with urlopen(f"{base_url}/todos/1", timeout=3) as response:
            if response.status != 200:
                return RuntimeError(f"todos-api returned HTTP {response.status}")
            payload = response.read().decode("utf-8")
            if '"id"' not in payload or '"title"' not in payload:
                return RuntimeError("todos-api response was incomplete")
    except Exception as exc:
        return exc
    return None


def check_redis() -> Exception | None:
    try:
        host, port = redis_address()
        with socket.create_connection((host, port), timeout=3) as client:
            client.sendall(b"*1\r\n$4\r\nPING\r\n")
            response = client.recv(7).decode("utf-8", "replace")
            if not response.startswith("+PONG"):
                return RuntimeError(f"redis ping returned {response.strip()}")
    except Exception as exc:
        return exc
    return None


def redis_address() -> tuple[str, int]:
    raw_url = os.getenv("REDIS_URL", "")
    if raw_url:
        parsed = urlparse(raw_url)
        if parsed.hostname and parsed.port:
            return parsed.hostname, parsed.port
    host = os.getenv("REDIS_HOST", "")
    if not host:
        raise RuntimeError("REDIS_URL or REDIS_HOST must be set")
    return host, int(os.getenv("REDIS_PORT", "6379"))


def status(error: Exception | None) -> str:
    return "ok" if error is None else "error"


def main() -> None:
    port = int(os.getenv("PORT", "8080"))
    HTTPServer(("", port), Handler).serve_forever()


if __name__ == "__main__":
    main()

