"""Minimal OpenAI-compatible chat server for any Hugging Face transformers model.

The fallback for running a model the proxy can't otherwise reach.
Point the proxy at it with provider "openai" and
base_url "http://<host>:<port>/v1/chat/completions".
"""

import argparse
import json
from collections.abc import Callable
from http.server import BaseHTTPRequestHandler, HTTPServer

DEFAULT_MAX_TOKENS = 1024
MAX_TOKENS_LIMIT = 4096
MAX_BODY_BYTES = 1 << 20  # 1 MiB

# generate(messages, max_tokens, temperature) -> (text, finish_reason)
Generate = Callable[[list[dict], int, float], tuple[str, str]]


def parse_request(body: object) -> tuple[list[dict], int, float]:
    """Raises ValueError or TypeError for a malformed request."""
    if not isinstance(body, dict):
        raise ValueError("request body must be a JSON object")
    messages = body.get("messages")
    if not isinstance(messages, list) or not messages:
        raise ValueError("messages must be a non-empty list")
    if not all(isinstance(m, dict) and isinstance(m.get("role"), str) and isinstance(m.get("content"), str) for m in messages):
        raise ValueError("each message must be an object with string role and content")

    max_tokens = body.get("max_tokens")
    max_tokens = DEFAULT_MAX_TOKENS if max_tokens is None else int(max_tokens)
    if not 1 <= max_tokens <= MAX_TOKENS_LIMIT:
        raise ValueError(f"max_tokens must be between 1 and {MAX_TOKENS_LIMIT}")
    temperature = float(body.get("temperature") or 0.0)
    if temperature < 0:
        raise ValueError("temperature must not be negative")
    # response_format is accepted but not enforced: the backend already
    # strips code fences and validates/retries against the JSON schema.
    return messages, max_tokens, temperature


def load_model(model_id: str, device: str, trust_remote_code: bool) -> tuple[Generate, str]:
    """torch/transformers are imported here so the request handling stays
    testable without them."""
    import torch
    from transformers import AutoModelForCausalLM, AutoTokenizer

    if device == "auto":
        if torch.cuda.is_available():
            device = "cuda"
        elif torch.backends.mps.is_available():
            device = "mps"
        else:
            device = "cpu"

    tokenizer = AutoTokenizer.from_pretrained(model_id, trust_remote_code=trust_remote_code)
    if tokenizer.chat_template is None:
        raise SystemExit(f"{model_id} has no chat template; serve an instruct/chat model")
    model = AutoModelForCausalLM.from_pretrained(
        model_id, dtype="auto", trust_remote_code=trust_remote_code
    ).to(device)
    pad_token_id = tokenizer.pad_token_id if tokenizer.pad_token_id is not None else tokenizer.eos_token_id

    def generate(messages: list[dict], max_tokens: int, temperature: float) -> tuple[str, str]:
        inputs = tokenizer.apply_chat_template(
            messages, add_generation_prompt=True, return_tensors="pt", return_dict=True
        ).to(device)
        sampling = {"do_sample": True, "temperature": temperature} if temperature > 0 else {"do_sample": False}
        output = model.generate(**inputs, max_new_tokens=max_tokens, pad_token_id=pad_token_id, **sampling)
        new_tokens = output[0][inputs["input_ids"].shape[1]:]
        finish_reason = "length" if len(new_tokens) >= max_tokens else "stop"
        return tokenizer.decode(new_tokens, skip_special_tokens=True), finish_reason

    return generate, device


def make_handler(generate: Generate) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        # Socket timeout, so a client that stalls mid-request can't hang the
        # single-threaded server. Only covers reads/writes, not generation.
        timeout = 30

        def do_POST(self) -> None:
            if self.path != "/v1/chat/completions":
                return self._reply(404, {"error": {"message": f"unknown path {self.path}"}})
            try:
                length = int(self.headers.get("Content-Length", 0))
                if not 0 < length <= MAX_BODY_BYTES:
                    raise ValueError(f"Content-Length must be between 1 and {MAX_BODY_BYTES}")
                body = json.loads(self.rfile.read(length))
                messages, max_tokens, temperature = parse_request(body)
            except (ValueError, TypeError) as e:  # ValueError includes json.JSONDecodeError
                return self._reply(400, {"error": {"message": str(e)}})
            try:
                text, finish_reason = generate(messages, max_tokens, temperature)
            except Exception as e:
                return self._reply(500, {"error": {"message": f"{type(e).__name__}: {e}"}})
            self._reply(200, {
                "object": "chat.completion",
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": text},
                    "finish_reason": finish_reason,
                }],
            })

        def _reply(self, status: int, payload: dict) -> None:
            data = json.dumps(payload).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    return Handler


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("model", help="Hugging Face Hub ID or local checkpoint path")
    parser.add_argument("--host", default="127.0.0.1", help="address to listen on; there is no authentication, see the README before exposing it beyond localhost")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--device", default="auto", choices=["auto", "cpu", "cuda", "mps"])
    parser.add_argument("--trust-remote-code", action="store_true", help="allow custom model code from the checkpoint")
    args = parser.parse_args()

    print(f"Loading {args.model}...", flush=True)
    generate, device = load_model(args.model, args.device, args.trust_remote_code)
    # Single-threaded on purpose: one model on one device should generate one request at a time.
    server = HTTPServer((args.host, args.port), make_handler(generate))
    print(f"Serving {args.model} on {device} at http://{args.host}:{args.port}/v1/chat/completions", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
