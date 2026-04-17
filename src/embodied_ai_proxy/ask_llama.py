#!/usr/bin/env python3
import argparse
import json
import sys
import urllib.request
import urllib.error

# FIX: Point to the PROXY (8000), not Ollama directly (11434)
PROXY_URL = "http://localhost:8000"

def query_proxy(user_input: str) -> str:
    """Sends the raw instruction to the Proxy Middleware."""
    payload = json.dumps({"instruction": user_input}).encode("utf-8")
    
    req = urllib.request.Request(
        PROXY_URL,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=60) as response:
            return response.read().decode("utf-8")
    except urllib.error.URLError as e:
        print(f"[ERROR] Proxy unreachable at {PROXY_URL}. Is llm_proxy.py running?", file=sys.stderr)
        sys.exit(1)

def main():
    parser = argparse.ArgumentParser(description="Kinova ROS2 LLM Client")
    parser.add_argument("instruction", nargs="+", help="Natural language command")
    args = parser.parse_args()
    user_instruction = " ".join(args.instruction)

    print(f"\n[Input ] {user_instruction}")
    print(f"{'─' * 40}")
    print("[Waiting for Proxy + LLM...]\n")

    # 1. Get response from Proxy
    raw_response = query_proxy(user_instruction)

    # 2. Parse the Proxy's response
    try:
        data = json.loads(raw_response)
        
        # 3. Check the status set by our Proxy
        if data.get("status") == "success":
            print("SUCCESS: Valid Commands Generated")
            print(json.dumps(data["payload"], indent=2))
        elif data.get("status") == "blocked":
            print("BLOCKED: Safety Filter Triggered")
            print(f"Reason: {data.get('reason')}")
        else:
            print("ERROR: Middleware Failure")
            print(json.dumps(data, indent=2))

    except json.JSONDecodeError:
        print("CRITICAL ERROR: Proxy returned non-JSON text.")
        print(raw_response)

if __name__ == "__main__":
    main()
