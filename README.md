# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project bridges high-level Large Language Models (LLMs) and robotic systems through a decoupled set of services: a Go API backend, a Go LLM proxy, and a Go Bubble Tea terminal UI (TUI). It doesn't need ROS2: it reaches the robot through the middleware's websocket bridge.

---

## Prerequisites

Before installing the proxy, ensure your host machine meets the following requirements:

### Software Requirements
- **Operating System:** Linux, macOS or WSL2 (needs `bash`, plus `lsof` or `ss` for `run.sh`)
- **Go:** 1.26.5+ (for the Backend, LLM Proxy, and TUI)
- **Local Inference Engine (Optional):** An active installation of [Ollama](https://ollama.com/) if running local models (e.g. Gemma, Llama)

### Hardware Requirements
Only relevant when running a local model through Ollama:
| Component | Requirement |
| :--- | :--- |
| **RAM** | 8GB minimum, 16GB+ recommended |
| **CPU** | 4+ cores |
| **GPU** | Optional (recommended if running local models via Ollama) |
| **Storage** | ≥15GB free space |

### External Dependency
This application drives the ROS2 Kinova middleware (`ROS2-middleware`), real or simulated. The middleware's launch also starts the `rosbridge_server` websocket (`ws://localhost:9090`) that the backend connects to, so ROS2 only needs to be installed where the middleware runs.

---

## Running the System

Use the [`run.sh`](run.sh) script to launch the LLM Proxy, Backend Server, and Terminal UI:

```bash
# 1. Run all services (Proxy on :8081, Backend on :8080, and TUI in foreground)
./run.sh

# 2. Build binaries first, then run
./run.sh --build

# 3. Headless Mode (starts Proxy + Backend only, useful for ROS2 testing)
./run.sh --headless

# 4. Evaluate the configured LLM against JSON test suites (Proxy only, no robot needed).
#    --eval must be the last option.
./run.sh --eval tests/*.json

# 5. Build Only (compiles all Go binaries to bin/ and exits)
./run.sh --build-only

# 6. Clean (removes compiled binaries and log files)
./run.sh --clean
```

> **Note:** When running `./run.sh`, quitting the TUI (`Ctrl+C`) stops the background `backend` and `llm-proxy` processes. `-b` and `-s` are short for `--build` and `--headless`.

---

## Manual Build & Launch (Development Mode)

If you prefer to manually build and run the Go services in separate terminal windows:

### 1. Build Binaries
```bash
go build -o bin/llm-proxy ./llm-proxy/cmd/server
go build -o bin/backend ./backend/cmd/server
go build -o bin/tui ./tui/cmd/tui
```
*(The repository includes a root `go.work` file that links `backend`, `llm-proxy`, `tui`, and `shared` modules together).*

### 2. Launch Services in Separate Terminals

#### Terminal 1: Start LLM Proxy
```bash
./bin/llm-proxy -dataDir data -httpPort 8081
# Or run directly: go run ./llm-proxy/cmd/server -dataDir data -httpPort 8081
```

#### Terminal 2: Start Backend Server
```bash
./bin/backend -dataDir data -httpPort 8080
# Or run directly: go run ./backend/cmd/server -dataDir data -httpPort 8080
```

#### Terminal 3: Start Terminal UI (TUI)
```bash
./bin/tui -serverURL http://localhost:8080 -dataDir data
# Or run directly: go run ./tui/cmd/tui -serverURL http://localhost:8080 -dataDir data
```

---

## Connecting the ROS2 Robot Bridge

To connect a live or simulated robot arm:

### 1. Start the ROS2 Middleware
In your workspace/ros2_kortex_ws:
```bash
source /opt/ros/humble/setup.bash
source /path/to/your/workspace/ros2_kortex_ws/install/setup.bash
ros2 launch kinova_interface robot.launch.py
```

This also starts the `rosbridge_server` websocket on `ws://localhost:9090` once the middleware is up. If you change its port (`bridge_port:=<port>`), set `rosbridge_url` in `data/config/config.json` to match.

### 2. Start the Proxy/Backend with the TUI
```bash
# if building and running for the first time
./run.sh --build
```
```bash
# if binaries are already built
./run.sh
```

---

## Configuring the LLM

The backend and the LLM proxy share **one unified configuration file** at `data/config/config.json`:

```json
{
  "server": {
    "port": 8080,
    "proxy_url": "http://localhost:8081",
    "rosbridge_url": "ws://localhost:9090",
    "execution_timeout_seconds": 300
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

Missing keys fall back to the defaults shown. The `port` values are informational: the services listen on their `-httpPort` flag (default 8080 and 8081), so change `proxy_url` too if you change the proxy's port. API keys are only read from this file, so don't commit real keys.

### Supported Providers
1. **Ollama** (Local or remote endpoint)
2. **Google Gemini** (API key)
3. **OpenAI** (API key)
4. **Anthropic** (API key)

### Provider Configuration Examples

#### 1. Ollama
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
  "model": "gemini-2.5-flash",
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
  "model": "claude-sonnet-5-5",
  "base_url": "https://api.anthropic.com/v1/messages",
  "api_key": "YOUR_ANTHROPIC_API_KEY",
  "max_tokens": 1024,
  "temperature": 0.1,
  "timeout_seconds": 30
}
```

---

## System Architecture

```text
User Input (TUI over WebSocket /ws/client, or test suites via backend -eval)
            ↓
Environment refresh (objects, movements, orientations, table bounds from /get_robot_parameters)
            ↓
Prompt Synthesis (Go Backend: system prompt + schema + environment + command)
            ↓
LLM Dispatch (Go LLM Proxy -> Ollama / OpenAI / Anthropic / Gemini)
            ↓
Schema Validation (Go Backend, data/config/json_schema.json)
            ↓
Name Resolution (fuzzy-match object names; one LLM retry if an object is missing)
            ↓
Direct ROS2 Rosbridge Client (Go Backend -> ws://localhost:9090 /execute_recipe)
            ↓
ROS2 Middleware Execution Layer (Kinova Gen3 Lite)
```

### Components

1. **Go Backend** (`backend/`):
   - Owns application state, environment object registry, and WebSocket client hub (`/ws/client`).
   - Connects directly as a WebSocket client to ROS 2 `rosbridge_server` (`ws://localhost:9090`), querying the environment (`/get_robot_parameters`), listening to telemetry (`/system/status`), executing recipes (`/execute_recipe`), resetting objects (`/reset_environment`) and triggering camera scans (`/vision/snapshot`, needs the middleware's `enable_CV:=true`).
   - Uses `internal/pipeline` to build prompts, query the LLM proxy, validate output against `data/config/json_schema.json`, resolve names, and dispatch action recipes.
   - Accepts one TUI client at a time.
   - Exposes `GET /api/info` (live stats, configuration, prompt), `POST /api/reset` and `POST /api/scan`.
2. **Go LLM Proxy** (`llm-proxy/`):
   - Standalone microservice abstracting provider APIs behind `POST /generate`, with a configurable timeout and one retry on transient errors.
3. **Go Terminal UI** (`tui/`):
   - Interactive Bubble Tea / Lip Gloss terminal interface connecting to the backend over WebSocket (`/ws/client`). Type `/help` for its commands, including `/reset-env`, `/scan`, `/save` and `/copy`.
4. **Evaluation Suite** (`backend -eval` + `tests/*.json`):
   - Runs each test prompt through the real pipeline in-process (LLM call, schema validation, name resolution, retry) and checks the result against the expected outcomes, without needing a live robot.

---

## Project Structure

```text
embodied-ai-proxy/
├── run.sh                           # Unified build & run automation script
├── go.work                          # Multi-module Go workspace
│
├── data/                            # Shared runtime data
│   ├── config/config.json           # Single unified configuration file
│   ├── config/json_schema.json      # JSON Schema for action recipe validation
│   ├── config/system_prompt.md      # System prompt template
│   └── logs/                        # Created at runtime: service logs and /save sessions
│
├── backend/                         # Go Backend Service
│   ├── cmd/server/main.go           # Server entrypoint (-eval runs test suites instead)
│   └── internal/
│       ├── api/                     # REST handlers (GET /api/info, POST /api/reset, POST /api/scan)
│       ├── eval/                    # Test suite runner for -eval
│       ├── matcher/                 # Fuzzy object-name matching
│       ├── pipeline/                # Prompt synthesis, LLM dispatch, validation, name resolution, retry
│       ├── rosbridge/               # Direct ROS 2 rosbridge WebSocket client
│       ├── validator/               # jsonschema/v5 validator engine
│       ├── websocket/               # Hub for /ws/client
│       └── server/                  # Server bootstrap & routing
│
├── llm-proxy/                       # Go LLM Proxy Gateway
│   ├── cmd/server/main.go           # Proxy entrypoint
│   └── internal/
│       ├── api/                     # POST /generate handler
│       ├── provider/                # Adapters: Ollama, Gemini, OpenAI, Anthropic
│       ├── router/                  # Provider routing & retry logic
│       └── server/                  # HTTP server bootstrap
│
├── tui/                             # Go Bubble Tea Terminal UI
│   ├── cmd/tui/main.go              # TUI entrypoint
│   └── internal/
│       ├── app/                     # Model, Update, View, styles
│       └── client/                  # WebSocket client & API client
│
├── shared/                          # Shared Go Packages
│   ├── config/                      # Unified config loader
│   ├── logging/                     # Dual-output logging (stdout + file)
│   └── httpserver/                  # Graceful shutdown HTTP server
│
└── tests/                           # JSON test suites for -eval
    └── basic.json
```

---

## Evaluating an LLM with Test Suites

A test suite checks whether the configured LLM turns prompts into correct recipes. Each prompt goes through the same pipeline as a real command (JSON extraction, schema validation, name resolution, missing-object retry), using the suite's environment instead of the robot's:

```bash
./run.sh --build --eval tests/*.json           # starts the LLM proxy, runs the suites, exits 1 if any case fails
bin/backend -dataDir data -eval tests/basic.json   # same, if the proxy is already running
```

A suite is a JSON file:

```json
{
  "environment": {
    "objects": ["blue_cube", "delivery_tray"],
    "movements": ["move_upwards"],
    "orientations": ["top_down"],
    "table_bounds": { "available": true, "x_min": 0.2, "x_max": 0.8, "y_min": -0.4, "y_max": 0.4 }
  },
  "cases": [
    {
      "name": "pick and place",
      "prompt": "put the blue cube on the tray",
      "expect": [
        { "status": "success", "steps": [
          { "action": "pickup", "parameters": { "target": "blue_cube" } },
          { "action": "dropoff", "parameters": { "destination": "delivery_tray" } },
          { "action": "home" }
        ]}
      ]
    }
  ]
}
```

- A case fails if the pipeline can't produce a schema-valid result. With no `expect`, that's the only check.
- `expect` lists acceptable outcomes. The case passes if **any one** matches.
- Matching is partial: only the keys you write are checked (`step_id`, `description`, `recipe_name` etc. can be left out), and `null` means "this key must be absent". `steps` must match in order and in number.
- Expectations are checked after name resolution, so `"blu cube"` → `"blue_cube"` counts as correct.
- A case can set its own `environment`, which replaces the suite's for that case.

Full prompts and LLM responses are logged to `data/logs/eval.log`.

---

## Logging & Auditing

Logs are written under `data/logs/`. The backend and LLM proxy also log to stdout when run by hand; `run.sh` discards their stdout.
- `server.log` - Go Backend logs (WebSocket events, pipeline execution, schema validation)
- `proxy.log` - LLM Proxy logs (HTTP request/response timings, provider errors)
- `tui.log` - TUI debug logs
- `eval.log` - full prompts and responses from `-eval`
- `session_<timestamp>.txt` - sessions saved with the TUI's `/save`

`./run.sh --clean` deletes all of them.

---

## License

Apache 2.0. See [LICENSE](LICENSE).
