#!/usr/bin/env python3
import json
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

# --- CONFIG ---
# Proxy listens on 8000. It talks to Ollama on 11434.
OLLAMA_URL = "http://localhost:11434/api/generate"
PORT = 8000
MODEL = "llama3.1:8b"
ALLOWED_ACTIONS = ["move_to", "move_forward", "move_backward", "rotate", "stop", "wait"]

SYSTEM_PROMPT = f"""You are a Kinova Robot Command Parser.
Convert the user instruction into JSON. 

CRITICAL RULES:
1. ONLY use these actions: {ALLOWED_ACTIONS}.
2. If the user asks for an action that is not in the list (like 'fly', 'jump', or 'dance'), do NOT substitute it. 
3. If an instruction is impossible or contains unsupported actions, you MUST return: {{"commands": [], "error": "unsupported_action"}}
4. Do not make assumptions. If the command is 'fly', it is an error.

User instruction: """

class StrictKinovaHandler(BaseHTTPRequestHandler):
    def _respond(self, code, data):
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(data, indent=2).encode())

    def do_POST(self):
        try:
            content_length = int(self.headers['Content-Length'])
            post_data = self.rfile.read(content_length)
            user_input = json.loads(post_data).get("instruction", "")
            
            payload = {
                "model": MODEL,
                "prompt": SYSTEM_PROMPT + user_input,
                "stream": False,
                "options": {"temperature": 0.0}
            }
            
            req = urllib.request.Request(OLLAMA_URL, data=json.dumps(payload).encode())
            with urllib.request.urlopen(req) as resp:
                raw_llm = json.loads(resp.read().decode())['response'].strip()

            # --- SAFER JSON EXTRACTION (FIX #2) ---
            # Handles markdown fences and conversational filler text
            if "```" in raw_llm:
                parts = raw_llm.split("```")
                for p in parts:
                    if "{" in p:
                        raw_llm = p.replace("json", "").strip()
                        break

            start = raw_llm.find("{")
            end = raw_llm.rfind("}")

            if start == -1 or end == -1:
                return self._respond(200, {"status": "blocked", "reason": "No JSON detected"})

            # Extract only the content between the first and last curly braces
            parsed_data = json.loads(raw_llm[start:end+1])
            commands = parsed_data.get("commands", [])

            # --- STRICT VALIDATION ---
            if not commands:
                return self._respond(200, {"status": "blocked", "reason": "No valid robot commands detected."})

            for cmd in commands:
                if cmd.get("action") not in ALLOWED_ACTIONS:
                    return self._respond(200, {"status": "blocked", "reason": f"Action '{cmd.get('action')}' unauthorized."})

            self._respond(200, {"status": "success", "payload": parsed_data})

        except Exception as e:
            self._respond(400, {"status": "error", "message": str(e)})

if __name__ == "__main__":
    print(f"Kinova Proxy active on port {PORT}")
    HTTPServer(("0.0.0.0", PORT), StrictKinovaHandler).serve_forever()
