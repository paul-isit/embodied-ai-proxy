"""Checks serve.py's HTTP/OpenAI plumbing with the model stubbed out, so it
runs without torch/transformers or a download:
cd hf-server && python -m unittest test_serve.py"""

import json
import socket
import threading
import unittest
import urllib.error
import urllib.request
from http.server import HTTPServer

from serve import make_handler


class ServeTest(unittest.TestCase):
    def setUp(self) -> None:
        self.calls: list[tuple[list[dict], int, float]] = []

        def fake_generate(messages: list[dict], max_tokens: int, temperature: float) -> tuple[str, str]:
            self.calls.append((messages, max_tokens, temperature))
            if messages[0]["content"] == "boom":
                raise ValueError("model blew up")
            return '{"ok": true}', "stop"

        handler = make_handler(fake_generate)
        handler.timeout = 0.5  # so the stalled-client test is quick
        self.server = HTTPServer(("127.0.0.1", 0), handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.server.server_port}"

    def tearDown(self) -> None:
        self.server.shutdown()
        self.server.server_close()

    def post(self, path: str, data: bytes) -> tuple[int, dict]:
        req = urllib.request.Request(self.url + path, data=data, headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req) as resp:
                return resp.status, json.load(resp)
        except urllib.error.HTTPError as e:
            return e.code, json.load(e)

    def test_chat_completion(self) -> None:
        messages = [{"role": "system", "content": "be terse"}, {"role": "user", "content": "hi"}]
        body = {"model": "m", "messages": messages, "max_tokens": 50, "temperature": 0,
                "response_format": {"type": "json_object"}}
        status, resp = self.post("/v1/chat/completions", json.dumps(body).encode())
        self.assertEqual(status, 200)
        self.assertEqual(resp["choices"][0]["message"]["content"], '{"ok": true}')
        self.assertEqual(resp["choices"][0]["finish_reason"], "stop")
        self.assertEqual(self.calls, [(messages, 50, 0.0)])

    def test_defaults_when_params_missing(self) -> None:
        self.post("/v1/chat/completions", json.dumps({"messages": [{"role": "user", "content": "hi"}]}).encode())
        self.assertEqual(self.calls[0][1:], (1024, 0.0))

    def test_bad_requests(self) -> None:
        hi = [{"role": "user", "content": "hi"}]
        bodies = [{"messages": []}, {"messages": ["x"]}, {"messages": [{"role": 1, "content": "hi"}]},
                  {"messages": [{"role": "user"}]},{"messages": hi, "max_tokens": 0}, {"messages": hi, "max_tokens": -5},
                  {"messages": hi, "max_tokens": [1]}, {"messages": hi, "max_tokens": 4097},
                  {"messages": hi, "temperature": -1}]
        for data in [b"not json", b"[]"] + [json.dumps(b).encode() for b in bodies]:
            status, resp = self.post("/v1/chat/completions", data)
            self.assertEqual(status, 400, data)
            self.assertIn("message", resp["error"])
        self.assertEqual(self.calls, [])

    def test_generation_error_is_500(self) -> None:
        body = {"messages": [{"role": "user", "content": "boom"}]}
        status, resp = self.post("/v1/chat/completions", json.dumps(body).encode())
        self.assertEqual(status, 500)
        self.assertIn("model blew up", resp["error"]["message"])

    def raw(self, request: bytes) -> bytes:
        with socket.create_connection(("127.0.0.1", self.server.server_port), timeout=5) as s:
            s.sendall(request)
            return s.recv(1024)

    def test_bad_content_length(self) -> None:
        for length in (b"-1", b"0", b"4000000000", b"abc"):
            resp = self.raw(b"POST /v1/chat/completions HTTP/1.1\r\nContent-Length: " + length + b"\r\n\r\n")
            self.assertIn(b" 400 ", resp.split(b"\r\n")[0], length)

    def test_stalled_client_does_not_block_others(self) -> None:
        with socket.create_connection(("127.0.0.1", self.server.server_port)) as stalled:
            stalled.sendall(b"POST /v1/chat/completions HTTP/1.1\r\nContent-Length: 100\r\n\r\n")
            status, _ = self.post("/v1/chat/completions", json.dumps({"messages": [{"role": "user", "content": "hi"}]}).encode())
        self.assertEqual(status, 200)

    def test_unknown_path(self) -> None:
        status, _ = self.post("/v1/completions", b"{}")
        self.assertEqual(status, 404)


if __name__ == "__main__":
    unittest.main()
