# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project bridges high-level Large Language Models (LLMs) and robotic systems through a small set of decoupled services: a Go API backend, a Go LLM proxy, a TypeScript terminal UI, and a Python ROS2 bridge client.

## Prerequisites

Before installing the proxy, ensure your host machine meets the following requirements:

### Software Requirements
**Operating System** Ubuntu 22.04 LTS
**ROS2** Active installation of ROS2 Humble
**Go** 1.26+ (for the backend and LLM proxy services)
**Node.js** 20+ and npm (for the terminal UI)
**Python** 3.10+ (for the ROS2 bridge client and the evaluation scripts)
**Local Inference Engine** An active installation of Ollama (https://ollama.com/) is required if running local models

### Hardware Requirements
The project allows for an interchangeable LLM API to be utilized, scaling to user's host machine hardware capacity. Recommended specs for stable performance are:
| Component | Requirement                            |
|-----------|-----------------------------------------|
| RAM       | 8GB minimum, 32GB recommended          |
| CPU       | 6+ cores recommended                   |
| GPU       | Optional (recommended for larger LLMs) |
| Storage   | ≥15GB free space                       |

### External Dependency
This application relies on an external ROS2 Kinova Middleware repository (`ROS2-middleware`).

---

## Setup and Installation

### 1. Install System Dependencies
The bridge relies on the ROS2 `rosbridge_suite` to handle complex WebSocket-to-ROS translation.
```bash
sudo apt-get update
sudo apt-get install ros-humble-rosbridge-suite
```

### 2. Build the Go Services
```bash
cd backend && go build -o ../bin/backend ./cmd/server && cd ..
cd llm-proxy && go build -o ../bin/llm-proxy ./cmd/server && cd ..
```
(A `go.work` file at the repo root ties the `backend`, `llm-proxy`, and `shared` Go modules together for local development - editors/`gopls` will resolve across all three automatically.)

### 3. Install the Terminal UI Dependencies
```bash
cd tui
npm install
cd ..
```

### 4. Install Python Dependencies (evaluation scripts)
```bash
pip install -r requirements.txt
```

### 5. Build the ROS2 Bridge Workspace
```bash
cd ros2_bridge_ws
colcon build
```
This installs the `custom_bridge_pkg` ROS2 package, which now includes `bridge_client` - a Python node that connects to the Go backend as a WebSocket client and translates action recipes into ROS2 service calls against the Kinova middleware via the existing `rosbridge_websocket` server. See `System Architecture` below.

---

## Configuring the LLM

The backend and the LLM proxy now share **one config file**, `data/config/config.json` at the repo root, so there's nowhere else to look and nothing to keep in sync across services. By default both services look for `<dataDir>/config/config.json` relative to wherever you run them from (`dataDir` defaults to `data`, overridable with `-dataDir`) - run them from the repo root (see "Running the System") so they resolve to the same file.

```json
{
  "server": {
    "port": 8080,
    "proxy_url": "http://localhost:8081"
  },
  "proxy": {
    "port": 8081,
    "llm_config": {
      "provider": "ollama",
      "model": "gemma3:1b",
      "base_url": "http://localhost:11434/api/generate",
      "api_key": "",
      "max_tokens": 1024,
      "temperature": 0.1,
      "timeout_seconds": 30
    }
  }
}
```
`server` configures the Go backend, `proxy` configures the Go LLM proxy. No code changes are needed to switch providers or models; just edit the `proxy.llm_config` block and restart the LLM proxy. Ensure all fields are valid and your API key (if required) is active.

> **Note:** API keys are currently stored in plaintext in this config file. Environment-variable overrides are a tracked follow-up (see `openspec/changes/decouple-go-backend-llm-proxy/design.md`), not yet implemented.

The following LLM providers are supported:
1. Ollama (running locally)
2. Google Gemini (API key)
3. OpenAI (API key)
4. Anthropic (API key)

### Local LLM Setup & Hosting (Ollama)

When using local LLMs (e.g., Gemma via Ollama), follow these steps to download the model and host Ollama so it can be accessed locally, in WSL, or across a Virtual Machine (VM):

1. **Install Gemma (or any ollama model) via Ollama:**
   ```bash
   ollama run gemma3:1b
   ```
   Once it is running, exit the chat session by typing:
   ```text
   /bye
   ```

2. **Host Ollama for Local / VM Access:**
   ```bash
   OLLAMA_HOST=0.0.0.0 ollama serve
   ```
   *(Do not close this terminal after running this command).*

3. **Verify Connectivity:**
   * **If using a VM:** `curl http://{your-local-ip-address}:11434` should output `Ollama is running`.
   * **If using WSL or Linux locally:** `curl http://localhost:11434` should output `Ollama is running`.

### Provider Configuration Examples

Below are the exact `llm_config` blocks for the four supported providers. Copy your desired configuration into the `proxy.llm_config` section of `data/config/config.json` (see above):

#### 1. Ollama (Local or Network-hosted)
```json
{
    "provider": "ollama",
    "model": "gemma3:1b",
    "base_url": "http://localhost:11434/api/generate",
    "api_key": "",
    "max_tokens": 1024,
    "temperature": 0.1,
    "timeout_seconds": 30
}
```

#### 2. Google Gemini
```json
{
    "provider": "gemini",
    "model": "gemini-1.5-flash",
    "base_url": "https://generativelanguage.googleapis.com/v1beta/models",
    "api_key": "YOUR_GEMINI_API_KEY",
    "max_tokens": 1024,
    "temperature": 0.1,
    "timeout_seconds": 30
}
```

#### 3. OpenAI
```json
{
    "provider": "openai",
    "model": "gpt-4o-mini",
    "base_url": "https://api.openai.com/v1/chat/completions",
    "api_key": "YOUR_OPENAI_API_KEY",
    "max_tokens": 1024,
    "temperature": 0.1,
    "timeout_seconds": 30
}
```

#### 4. Anthropic
```json
{
    "provider": "anthropic",
    "model": "claude-3-5-sonnet-20241022",
    "base_url": "https://api.anthropic.com/v1/messages",
    "api_key": "YOUR_ANTHROPIC_API_KEY",
    "max_tokens": 1024,
    "temperature": 0.1,
    "timeout_seconds": 30
}
```

---

## Running the System

To run the full end-to-end pipeline, open five terminals:

### Terminal 1: Start the ROS2 Bridge (rosbridge + bridge client)
> **Important:** You must source `install/setup.bash` from the built `ROS2-middleware` project in this terminal before launching, so ROS2 can locate the custom message/service/action interfaces used by the middleware.

```bash
source /opt/ros/humble/setup.bash
source /path/to/your/ROS2-middleware/install/setup.bash

cd ros2_bridge_ws
source install/setup.bash
ros2 launch custom_bridge_pkg proxy_bridge.launch.py
```
This launches **both** the `rosbridge_websocket` server (`ws://localhost:9090`, talks to the Kinova middleware) **and** `bridge_client` (connects out to the Go backend at `ws://localhost:8080/ws/bridge`). `bridge_client` accepts `--backend-url`, `--rosbridge-url`, `--reconnect-delay`, and `--object-list-refresh-interval` (how often it re-reports the workspace object list, default 10s) if you need to change any of them - all four are exposed as launch arguments, e.g. `ros2 launch custom_bridge_pkg proxy_bridge.launch.py backend_url:=ws://otherhost:8080/ws/bridge`.

### Terminal 2: Start the Middleware
```bash
cd /path/to/your/ROS2-middleware/
colcon build
source install/setup.bash
ros2 launch kinova_interface robot.launch.py
```

### Terminal 3: Start the Go LLM Proxy
Run from the **repo root** (the `go.work` file makes this work without `cd`-ing into `llm-proxy/`) so `-dataDir data` resolves to the same `data/` directory the backend uses:
```bash
go run ./llm-proxy/cmd/server -httpPort 8081
```

### Terminal 4: Start the Go Backend
Also from the **repo root**:
```bash
go run ./backend/cmd/server -httpPort 8080
```
`-dataDir` (default `data`) must point at the directory containing `config/config.json`, `config/json_schema.json`, and `config/system_prompt.md` - the backend fails to start if `json_schema.json` is missing. This is also what makes it share `data/config/config.json` and `data/logs/server.log` with the LLM proxy - only pass `-dataDir` explicitly if you're running from somewhere other than the repo root.

### Terminal 5: Start the Terminal UI
```bash
cd tui
npm start
```
Connects to `ws://localhost:8080/ws/client` by default; override with `--url <ws-url>` or `TUI_BACKEND_WS_URL`. Press `Ctrl+V`/`Ctrl+G`/`Ctrl+L`/`Ctrl+Y`/`Alt+1`-`Alt+5` for verbosity/system-info/LLM-info/copy-prompt/quick-commands - the running shortcut legend at the bottom of the UI lists all of them.

---

## System Architecture

The project is split into five independently-run components:

1. **Go Backend** (`backend/`): standalone API server owning application state, the workspace object registry, JSON schema validation, and the WebSocket hub (`/ws/client` for the TUI, `/ws/bridge` for the ROS2 bridge client). Builds the full LLM prompt (system prompt + schema + workspace objects + user command), dispatches it to the LLM proxy, validates the result, and routes it onward - one command at a time; a `prompt_submit` received while another is still in flight is rejected immediately rather than queued. Exposes `GET /api/info` (combined server/LLM config + live hub stats + the system prompt, used by the TUI's info/copy shortcuts) and `POST /api/prompt` (synchronous, used by `evaluate_proxy.py`). (There is currently no `/health` endpoint on either Go service - it was removed pending a redesign.)
2. **Go LLM Proxy** (`llm-proxy/`): standalone service abstracting Ollama/OpenAI/Gemini/Anthropic behind one HTTP endpoint (`POST /generate`), with per-request timeout handling. The backend is its only client.
3. **TypeScript Ink TUI** (`tui/`): terminal client connecting to the backend over WebSocket, with auto-reconnect. Renders connection state, ROS 2 bridge connectivity, per-node middleware telemetry, the workspace object list, a log feed, and the last action recipe/error at a configurable verbosity (`Ctrl+V`); submits user commands as `prompt_submit` messages, with command history (`↑`/`↓`), 5 quick commands (`Alt+1`-`Alt+5`), and system/LLM info + copy-system-prompt shortcuts backed by `GET /api/info`.
4. **Python ROS2 Bridge Client** (`ros2_bridge_ws/src/custom_bridge_pkg/custom_bridge_pkg/bridge_client.py`): connects to the Go backend as a WebSocket client (not a server) and to the still-unchanged `rosbridge_websocket` server to reach ROS2 services. Reports the workspace object list to the backend on connect, dispatches successful action recipes to the Kinova middleware via `/execute_recipe`, and reports execution results back.
5. **Evaluation scripts** (`evaluate_proxy.py` + `tests/*.yaml`): batch-test the backend's `POST /api/prompt` endpoint directly, without needing the TUI or a real robot.

This layer separation means only the ROS2 bridge client is permitted to reach ROS2 hardware or simulation - the Go backend, LLM proxy, and TUI never touch ROS2 directly.

---

## System Contract Definition

The Embodied AI Proxy enforces a strict execution contract between the LLM layer and ROS2 middleware.

### Input Contract (User -> LLM)
- Input: Natural language command (string), submitted via the TUI or `POST /api/prompt`
- Processed by: the Go LLM Proxy (Ollama / OpenAI / Anthropic / Gemini)

No direct robot commands are accepted from user input.

### Output Contract (LLM -> Proxy)

All LLM outputs MUST conform to the validated JSON schema defined in `data/config/json_schema.json`. The schema is a discriminated union of two shapes:
```json
{
  "status": "success",
  "recipe_name": "Pick and Place Routine",
  "steps": [
    { "step_id": 1, "action": "home", "description": "Start at home", "parameters": {} },
    { "step_id": 2, "action": "move_arm", "description": "Move to target", "parameters": { "target": "red_cube" } }
  ]
}
```
or, when the command can't be safely executed:
```json
{ "status": "error", "error_type": "missing_object", "message": "..." }
```

### Validation Rules

Before an action recipe is broadcast to the ROS2 bridge, it must pass:
```text
1. JSON parsing validation
2. JSON Schema validation (data/config/json_schema.json, enforced by the Go backend)
3. Action whitelist enforcement (home / move_arm / relative_move / gripper)
4. Field completeness checks
```

### Rejection Policy

If any validation step fails:
```text
- The recipe is not broadcast to the ROS2 bridge
- The failure is surfaced to the TUI as a log_event
- System defaults to safe idle state
```

### Execution Boundary
```text
- Only validated, schema-conformant, "success" action recipes may be transmitted to:
    - ws://localhost:8080/ws/bridge (Go backend -> bridge client)
    - ws://localhost:9090 (bridge client -> rosbridge server, unchanged)
- No component other than the ROS2 bridge client is permitted to call ROS2 services.
```

---

## System Data Flow
```text
User Input (TUI, prompt_submit over WebSocket)
            ↓
Prompt Construction (Go Backend: system prompt + schema + workspace objects + command)
            ↓
LLM Dispatch (Go LLM Proxy -> Ollama / OpenAI / Anthropic / Gemini)
            ↓
Structured JSON Action Recipe
            ↓
Schema Validation Layer (Go Backend, data/config/json_schema.json)
            ↓
WebSocket Broadcast (action_recipe -> /ws/bridge and /ws/client)
            ↓
ROS2 Bridge Client -> rosbridge (:9090) -> ROS2 Middleware Execution Layer
            ↓
Robot / Simulation (Kinova Gen3 Lite)
```
Return path:
```text
Robot State → ROS2 → rosbridge → Bridge Client → status_update (/ws/bridge) → Go Backend → TUI (/ws/client)
```

---

## Project Structure

```text
embodied-ai-proxy/
├── evaluate_proxy.py                # Batch evaluation framework (YAML based), hits the Go backend's HTTP API
├── requirements.txt                 # Python deps for evaluate_proxy.py (requests, PyYAML)
├── go.work                          # Ties the backend/llm-proxy/shared Go modules together for local dev
│
├── data/                            # Shared runtime data for both Go services (repo root)
│   ├── config/config.json           # Single config file: {"server": {...}, "proxy": {...}}
│   ├── config/json_schema.json      # JSON schema for output validation (read by the Go backend)
│   ├── config/system_prompt.md      # System-level LLM behaviour constraints (read by the Go backend)
│   └── logs/                        # Gitignored - server.log / proxy.log, created at startup
│
├── backend/                         # Go API backend
│   ├── cmd/server/main.go           # Entrypoint (flags: -dataDir, -httpPort)
│   └── internal/
│       ├── server/                  # HTTP/WebSocket server bootstrap
│       ├── websocket/               # Hub: /ws/client, /ws/bridge, envelope broadcast
│       ├── prompt/                  # Prompt template + pipeline (LLM dispatch, validation, routing, busy-lock)
│       ├── validator/               # JSON Schema validation (data/config/json_schema.json)
│       └── api/                     # POST /api/prompt and GET /api/info HTTP handlers
│
├── llm-proxy/                       # Go LLM proxy (standalone service)
│   ├── cmd/server/main.go           # Entrypoint (flags: -dataDir, -httpPort)
│   └── internal/
│       ├── provider/                # Provider interface + shared Do() helper + ollama/openai/gemini/anthropic adapters
│       ├── router/                  # Picks the configured provider, applies request timeout
│       ├── api/                     # POST /generate HTTP handler
│       └── server/                  # HTTP server bootstrap
│
├── shared/                          # Shared Go library, used by both backend and llm-proxy
│   ├── config/                      # Single AppConfig{Server, Proxy} loader (data/config/config.json)
│   ├── logging/                     # Points the standard logger at stdout + data/logs/<name>.log
│   └── httpserver/                  # Graceful HTTP server lifecycle
│
├── tui/                              # TypeScript Ink terminal UI (standalone Node client)
│   ├── package.json / tsconfig.json
│   └── src/                         # WebSocket client context + header/sidebar/log/result/input/shortcuts components
│
├── ros2_bridge_ws/                  # ROS2 Workspace
│   └── src/
│       └── custom_bridge_pkg/       # ROS2 package: rosbridge_websocket + bridge_client
│           ├── package.xml
│           ├── setup.py             # console_scripts: bridge_client
│           ├── launch/
│           │   └── proxy_bridge.launch.py   # Launches rosbridge_websocket + bridge_client together
│           └── custom_bridge_pkg/
│               └── bridge_client.py # WebSocket client: Go backend <-> rosbridge translator
│
└── tests/                           # YAML test suites for evaluate_proxy.py
    ├── basic_tests.yaml
    └── test_cases.yaml
```

---

## Interaction Logging

The previous Python monolith wrote a per-interaction log file to `logs/` for every prompt/response pair. The Go backend and LLM proxy restore that with a simpler mechanism: both services point Go's standard `log` package at **both** stdout and a file under `data/logs/` (`server.log` and `proxy.log` respectively, tagged `[Server]`/`[Pipeline]`/`[Hub]`/`[LLMProxy]`) via `shared/logging`. The backend's prompt pipeline logs every command it receives, the raw LLM response (or the failure), and the validation/dispatch outcome, so `tail -f data/logs/server.log` gives you a full audit trail without a separate log format or per-interaction files. There is currently no log rotation - these files grow unbounded, so rotate/truncate them yourself if that matters for your deployment.

---

## Testing the Proxy with YAML Scripts

Run Ollama and pull the target LLM model:
```bash
ollama serve
ollama pull gemma3:1b
```

Then start the Go LLM proxy and Go backend as described in "Running the System" (Terminals 3 and 4) - `evaluate_proxy.py` talks to the backend's HTTP API, not a real ROS2 bridge or TUI, so those aren't needed for evaluation.

### Running Bulk Evaluation Scripts

From project root:
```bash
python3 evaluate_proxy.py \
  --api-url http://localhost:8080 \
  --tests ./tests/basic_tests.yaml

python3 evaluate_proxy.py \
  --api-url http://localhost:8080 \
  --tests ./tests/test_cases.yaml
```

### Creating YAML Test Case Files

Test cases are defined in YAML. Example structure:
```yaml
test_cases:
  - name: Pick apple
    prompt: "pick up the apple"

    available_objects:
      - apple
      - banana
      - tray

    expected:
      first_action: "gripper"
      last_action: "relative_move"
      must_contain_actions:
        - "move_arm"
        - "gripper"
      must_contain_targets:
        - "apple"
```

| Field            | Type   | Description                         |
|------------------|--------|-------------------------------------|
| name             | string | Human-readable test label           |
| prompt           | string | User command sent to the LLM        |
| available_objects| list   | Objects available in environment    |
| expected         | dict   | Expected actions/targets rules      |

Place completed YAML files in the `tests/` directory.

---

## First Run Checklist

If the system does not respond, ensure:
```text
- Go backend is running (curl http://localhost:8080/api/info)
- Go LLM proxy is running (its process is up and listening on :8081 - there is currently no `/health` endpoint)
- Rosbridge is running on port 9090, and bridge_client shows "Connected to Go backend" in its log
- Middleware has been successfully launched
- Ollama (or your chosen provider) is reachable
- No firewall blocking WebSocket connections on 8080/8081/9090
```

---

## Common Issues & Troubleshooting

**Go backend won't start**
```bash
- Confirm -dataDir's config/ subdirectory (data/config/ by default) contains json_schema.json and system_prompt.md
- Check the port isn't already in use (-httpPort)
```
**LLM not responding**
```bash
- Verify the proxy.llm_config section of data/config/config.json (provider, base_url, api_key)
- Confirm local server status or provider API key
- curl -X POST http://localhost:8081/generate -d '{"prompt":"hello"}' to test the LLM proxy in isolation
- Check data/logs/proxy.log and data/logs/server.log for the actual request/response
```
**rosbridge / bridge_client connection failure**
```bash
- Confirm rosbridge_server is running on port 9090
- Confirm bridge_client's log shows it reached ws://localhost:8080/ws/bridge
- Check for "Connection refused" in bridge_client's log - it means rosbridge or the Go backend isn't reachable yet
```
**TUI won't connect**
```bash
- Confirm the Go backend is running first
- Check --url / TUI_BACKEND_WS_URL points at the right ws://host:port/ws/client
```
**Robot not moving**
```bash
- Middleware may not be fully initialized
- Check ROS2 topic/service availability
- bridge_client logs the execution_result it sends back to the Go backend - check there first
```

---

## Failure Mode Summary Table

| Failure Mode | Cause | Layer | Resolution |
|--------------|-------|--------|-----------|
| Invalid JSON output | LLM hallucination or formatting error | LLM Proxy / Backend | Regenerate prompt or adjust system prompt |
| Schema validation failure | Missing or malformed fields | Go Backend | Check `data/config/json_schema.json` |
| rosbridge disconnect | WebSocket server not running | ROS2 Bridge | Restart rosbridge on port 9090 |
| bridge_client disconnected from backend | Go backend not running / wrong URL | ROS2 Bridge Client | Check `--backend-url`, confirm backend is up |
| No robot movement | Middleware inactive or uninitialized | ROS2 Middleware | Ensure `robot.launch.py` is active |
| Stale or repeated actions | LLM loop without state reset | LLM / Backend | Clear context or reset session |

---

## Safety Constraints

- All LLM outputs are validated against the JSON schema before being broadcast
- Only whitelisted ROS actions are permitted (`home`, `move_arm`, `relative_move`, `gripper`)
- Invalid or malformed JSON is rejected and never reaches the ROS2 bridge
- No direct hardware commands bypass the Go backend / bridge client layer

---

## Extensibility

**Add new LLM provider**
```text
- Implement the provider.Provider interface in llm-proxy/internal/provider/<name>/
- Register it in the switch statement in llm-proxy/internal/router/router.go
```
**Add new robot actions**
```text
- Extend the middleware's ROS2 action schema
- Update data/config/json_schema.json (the Go backend validates against this at runtime, no code changes needed)
```

---

## Notes on Performance

- LLM latency will vary between providers and model size
- ROS2 execution latency is non-deterministic under load
- Recommended to run on dedicated machine for robotic experiments

---

## Minimum Working System Definition
The system is only operational if the following criteria are met:
```text
- Go backend reachable on :8080, Go LLM proxy reachable on :8081
- Rosbridge active on 'ws://localhost:9090', bridge_client connected to the Go backend
- Middleware is running and responsive
- LLM returns valid schema-compliant JSON
- At least one action cycle executes end-to-end
```
