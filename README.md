# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project bridges high-level Large Language Models (LLMs) and robotic systems through a decoupled set of services: a Go API backend, a Go LLM proxy, a Go Bubble Tea terminal UI (TUI), and a Python ROS2 bridge client.

---

## Prerequisites

Before installing the proxy, ensure your host machine meets the following requirements:

### Software Requirements
- **Operating System:** Ubuntu 22.04 LTS (or Linux / WSL2)
- **ROS2:** Active installation of ROS2 Humble
- **Go:** 1.22+ (for the Backend, LLM Proxy, and TUI)
- **Python:** 3.10+ (for the ROS2 bridge client and the evaluation scripts)
- **Local Inference Engine (Optional):** An active installation of [Ollama](https://ollama.com/) if running local models (e.g. Gemma, Llama)

### Hardware Requirements
The proxy allows interchangeable LLM APIs scaling to your hardware capacity:
| Component | Requirement |
| :--- | :--- |
| **RAM** | 8GB minimum, 16GB+ recommended |
| **CPU** | 4+ cores |
| **GPU** | Optional (recommended if running local models via Ollama) |
| **Storage** | ≥15GB free space |

### External Dependency
This application interacts with an external ROS2 Kinova Middleware repository (`ROS2-middleware`) or simulation environment.

---

## Setup and Installation

### 1. Install System Dependencies
The bridge relies on ROS2 Humble and `rosbridge_suite` to handle WebSocket-to-ROS translation:
```bash
sudo apt-get update
sudo apt-get install ros-humble-rosbridge-suite
```

### 2. Build the ROS2 Bridge Workspace
```bash
cd ros2_bridge_ws
colcon build
source install/setup.bash
cd ..
```

### 3. Install Python Dependencies (Evaluation Suite)
```bash
pip install -r requirements.txt
```

---

## Running the System

Use the [`run.sh`](run.sh) script to launch the LLM Proxy, Backend Server, and Terminal UI:

```bash
# 1. Run all services (Proxy on :8081, Backend on :8080, and TUI in foreground)
./run.sh

# 2. Build binaries first, then run
./run.sh --build

# 3. Headless Mode (starts Proxy + Backend only, useful for ROS2 testing & Python eval)
./run.sh --headless

# 4. Build Only (compiles all Go binaries to bin/ and exits)
./run.sh --build-only

# 5. Clean (removes compiled binaries and log files)
./run.sh --clean
```

> **Note:** When running `./run.sh`, quitting the TUI (`Ctrl+C` or `q`) automatically terminates the background `backend` and `llm-proxy` processes gracefully.

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

### 2. Start the ROS2 Bridge Client + Rosbridge
```bash
source /opt/ros/humble/setup.bash
source /path/to/your/workspace/ros2_kortex_ws/install/setup.bash
cd ros2_bridge_ws
source install/setup.bash
ros2 launch custom_bridge_pkg proxy_bridge.launch.py
```

### 3. Start the Proxy/Backend with the TUI
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

## System Architecture

```text
User Input (TUI over WebSocket /ws/client or HTTP POST /api/prompt)
            ↓
Prompt Synthesis (Go Backend: system prompt + schema + workspace objects + command)
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

### Components

1. **Go Backend** (`backend/`):
   - Owns application state, workspace object registry, and WebSocket hub (`/ws/client`, `/ws/bridge`).
   - Uses `internal/pipeline` to build prompts, query the LLM proxy, validate output against `data/config/json_schema.json`, and dispatch action recipes.
   - Exposes `GET /api/info` (live stats, configuration, prompt) and `POST /api/prompt` (synchronous evaluation API).
2. **Go LLM Proxy** (`llm-proxy/`):
   - Standalone microservice abstracting provider APIs behind `POST /generate` with configurable timeouts and retries.
3. **Go Terminal UI** (`tui/`):
   - Interactive Bubble Tea / Lip Gloss terminal interface connecting to the backend over WebSocket (`/ws/client`).
4. **Python ROS2 Bridge Client** (`ros2_bridge_ws/`):
   - Connects to the Go backend as a WebSocket client (`/ws/bridge`) and to `rosbridge_websocket` (`ws://localhost:9090`).
   - Re-reports detected workspace objects and dispatches verified recipes to the robot middleware.
5. **Evaluation Suite** (`evaluate_proxy.py` + `tests/*.yaml`):
   - Batch test runner that queries `POST /api/prompt` directly against YAML-defined test cases without needing a live robot.

---

## Project Structure

```text
embodied-ai-proxy/
├── run.sh                           # Unified build & run automation script
├── go.work                          # Multi-module Go workspace
├── evaluate_proxy.py                # Batch evaluation framework (YAML based)
├── requirements.txt                 # Python dependencies for evaluate_proxy.py
│
├── data/                            # Shared runtime data
│   ├── config/config.json           # Single unified configuration file
│   ├── config/json_schema.json      # JSON Schema for action recipe validation
│   ├── config/system_prompt.md      # System prompt template
│   └── logs/                        # Service logs (server.log, proxy.log, tui.log)
│
├── backend/                         # Go Backend Service
│   ├── cmd/server/main.go           # Server entrypoint
│   └── internal/
│       ├── api/                     # REST handlers (GET /api/info, POST /api/prompt)
│       ├── pipeline/                # Prompt synthesis, LLM dispatch, schema validation
│       ├── validator/               # jsonschema/v5 validator engine
│       ├── websocket/               # Hub for /ws/client and /ws/bridge
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
├── ros2_bridge_ws/                  # ROS 2 Humble Workspace
│   └── src/custom_bridge_pkg/       # ROS 2 package containing bridge_client node & launch file
│
└── tests/                           # YAML test suites for evaluate_proxy.py
    ├── basic_tests.yaml
    └── test_cases.yaml
```

---

## Batch Evaluation with YAML Test Suites

Start the backend and proxy in headless mode:
```bash
./run.sh --headless
```

In another terminal, run the evaluation scripts:
```bash
python3 evaluate_proxy.py --api-url http://localhost:8080 --tests ./tests/basic_tests.yaml
python3 evaluate_proxy.py --api-url http://localhost:8080 --tests ./tests/test_cases.yaml
```

---

## Logging & Auditing

Logs are written simultaneously to standard output and persisted under `data/logs/`:
- `data/logs/server.log` - Go Backend logs (WebSocket events, pipeline execution, schema validation)
- `data/logs/proxy.log` - LLM Proxy logs (HTTP request/response timings, provider errors)
- `data/logs/tui.log` - TUI debug logs
