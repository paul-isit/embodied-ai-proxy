# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project facilitates communication between high-level Large Language Models (LLMs) and robotic systems by bridging a pure Python inference environment with a ROS2 hardware abstraction layer.

## Prerequisites 
Before installing the proxy, ensure your host machine meets the following requirements:

### Software Requirements
**Operating System** Ubuntu 22.04 LTS  
**ROS2** Active installation of ROS2 Humble  
**Language** Python 3.10+  
**Local Inference Engine** An active installation of Ollama (https://ollama.com/) is required if running local models  

### Hardware Requirements
The project allows for an interchangeable LLM API to be utilized, scaling to user's host machine hardware capacity. Recommended specs for stable performance are:
| Component | Requirement                            |
|-----------|----------------------------------------|
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

### 2. Install Python Dependencies
The project requires installation of Python dependencies:
```bash
pip install -r src/requirements.txt
```

### 3. Build the Bridge Workspace
```bash
cd ros2_bridge_ws
colcon build
```

### 4. Set PYTHONPATH
Ensure local package imports resolve correctly by setting `PYTHONPATH`:
```bash
echo 'export PYTHONPATH=.' >> ~/.bashrc
source ~/.bashrc
```

---

## Configuring the LLM

Update `configs/llm_config.json` to point to your LLM provider. By default, it expects an Ollama instance running locally (e.g., `http://localhost:11434/api/generate`), leaving the API key field blank. No code changes are needed to switch providers or models; simply update the `llm_config.json` file with your provider settings. Ensure all fields are valid and your API key (if required) is active.

The configuration file defines:
```text
- Provider selection
- API endpoints
- Model parameters
- Temperature
- API key (if any)
- Max tokens
```

The following LLM providers are supported:
1. Ollama (running locally)
2. Google Gemini (API key)
3. OpenAI (API key)
4. Anthropic (API key)

### Local LLM Setup & Hosting (Ollama)

When using local LLMs (e.g., Gemma via Ollama), follow these steps to download the model and host Ollama so it can be accessed locally, in WSL, or across a Virtual Machine (VM):

1. **Install Gemma (or any ollama model) via Ollama:**
   Run the following command on your local machine:
   ```bash
   ollama run gemma3:1b
   ```
   Once it is running, exit the chat session by typing:
   ```text
   /bye
   ```

2. **Host Ollama for Local / VM Access:**
   Run Ollama on your host machine bound to all network interfaces so it can be accessed from your VM or local network:
   ```bash
   OLLAMA_HOST=0.0.0.0 ollama serve
   ```
   *(Do not close this terminal after running this command).*

3. **Verify Connectivity:**
   * **If using a VM:** Confirm that your VM can access Ollama by running the following command inside your VM:
     ```bash
     curl http://{your-local-ip-address}:11434
     ```
     It should output: `Ollama is running`
   * **If using WSL or Linux locally:** Run `ollama serve` and verify in your terminal with:
     ```bash
     curl http://localhost:11434
     ```
     It should output: `Ollama is running`

### Provider Configuration Examples

Below are the exact configurations for the four supported providers. Copy your desired configuration into `configs/llm_config.json`:

#### 1. Ollama (Local or Network-hosted)
Used for running models locally (such as Gemma, Llama, or Mistral).
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
Utilizes Google's Gemini models via the Google AI Studio API.
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
Utilizes OpenAI's GPT models via the OpenAI API key.
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
Utilizes Anthropic's Claude models via the Anthropic API key.
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

To run the full end-to-end pipeline, open three terminals:

### Terminal 1: Start the ROS2 Bridge
> **Important:** You must source `install/setup.bash` from the built `ROS2-middleware` project in this terminal before launching rosbridge. This enables rosbridge to locate custom ROS2 message, service, and action interfaces used by the middleware.

```bash
source /opt/ros/humble/setup.bash

# Source the ROS2-middleware workspace setup file
source /path/to/your/ROS2-middleware/install/setup.bash

# Source and launch the rosbridge workspace
cd ros2_bridge_ws
source install/setup.bash
ros2 launch custom_bridge_pkg proxy_bridge.launch.py
```
*(The bridge is now listening on ws://localhost:9090)*

### Terminal 2: Start the Middleware
Run the ROS2 Middleware workspace:
```bash
cd /path/to/your/ROS2-middleware/
colcon build
source install/setup.bash
ros2 launch kinova_interface robot.launch.py
```

### Terminal 3: Start the Proxy UI
Run the following command to enter the main application:
```bash
python3 main.py
```

---

## System Architecture

The project is divided into two distinct domains to ensure hardware stability and dependency isolation:

1. **Inference Domain (Python):** A pure Python environment that manages LLM logic (via local Ollama or cloud APIs), prompt engineering, and the Textual user interface. It acts as a WebSocket client.
Responsible for:
```text
- LLM orchestration (Ollama / OpenAI / Anthropic / Gemini etc.)
- Prompt construction and system instruction enforcement
- JSON schema validation
- Terminal User Interface (TUI)
- WebSocket client communication with ROS bridge
```
This layer never interacts directly with robot hardware.

2. **ROS2 Bridge Domain:** A dedicated ROS2 workspace owned by the proxy. It acts as a wrapper around the system-level `rosbridge_suite`, launching a WebSocket server (port `9090`) to translate JSON requests into native ROS2 Service calls for the Kinova middleware.
Responsible for:
```text
- Hosting 'rosbridge_suite' WebSocket server (ws://localhost:9090)
- Translating JSON requests into ROS2 service calls
- Interfacing with external robot middleware (Kinova arm)
- Executing validation motion and gripper commands
```
This layer is the only component permitted to interact with ROS2 hardware or simulation.

---

## System Contract Definition

The Embodied AI Proxy enforces a strict execution contract between the LLM layer and ROS2 middleware.

### Input Contract (User -> LLM)
- Input: Natural language command (string)
- Processed by: LLM adapter (Ollama / OpenAI / Anthropic / Gemini)

No direct robot commands are accepted from user input.

### Output Contract (LLM -> Proxy)

All LLM outputs MUST conform to the validated JSON schema defined in:
```bash
configs/json_schema.json
```
Expected structure:
```json
{
  "action": "move_arm",
  "target": "object_name",
  "mode": "absolute | relative"
}
```

### Validation Rules

Before execution, all outputs must pass:
```text
1. JSON parsing validation
2. Schema validation 
3. Action whitelist enforcement
4. Field completeness checks
```

### Rejection Policy

If any validation step fails:
```text
- Output is rejected
- No ROS2 message is transmitted
- System defaults to safe idle state
```

### Execution Boundary
```text
- Only validated and whitelisted action recipes may be transmitted to:
    - ws://localhost:9090 (rosbridge server)
- No direct hardware or ROS2 service calls are permitted from the Python layer.
```

---

## System Data Flow
```text
User Input (TUI)
            ↓
LLM Prompt Construction (Python Proxy)
            ↓
Structured JSON Action Recipe
            ↓
Schema Validation Layer (Pydantic / JSON Schema)
            ↓
WebSocket Transmission (rosbridge :9090)
            ↓
ROS2 Middleware Execution Layer
            ↓
Robot / Simulation (Kinova Gen3 Lite)
```
Return path:
```text
Robot State → ROS2 → rosbridge → Proxy → TUI
```

---

## Project Structure

```text
embodied-ai-proxy/
├── main.py                         # System entry point (TUI + runtime)
├── evaluate_proxy.py               # Batch evaluation framework (YAML based)
│
├── configs/                        # Configuration and validation rules
│   ├── llm_config.json             # LLM provider configuration
│   ├── system_prompt.md            # System level LLM behaviour constraints
│   └── json_schema.json            # JSON schema for output validation (safety layer)
│
├── ros2_bridge_ws/                 # Domain 1: ROS2 Workspace
│   └── src/
│       └── custom_bridge_pkg/      # ROS2 package wrapping rosbridge
│           ├── package.xml
│           ├── setup.py
│           └── launch/
│               └── proxy_bridge.launch.py # Launches WebSocket rosbridge_server
│
├── tests/                          # YAML files containing test scripts
│   └── basic_tests.yaml            # Short list of tests 
│
└── src/                            # Domain 2: Python Inference Environment
    ├── requirements.txt            # Python dependencies (websockets, requests, textual, pydantic)
    ├── frontend/                   # UI Components
    │   ├── tui_app.py              # Terminal User Interface
    │   ├── styles.css              # TUI styling and layout config
    │   └── components/             # Further configurations for TUI
    │        ├── input_bar.py       # Handles user command input
    │        ├── log_panel.py       # Displays logs, recipes and system output
    │        ├── sidebar.py         # Column displaying info in TUI
    │        └── status_panel.py    # Displays connection and system status
    │
    └── backend/                    # Core Logic
        ├── defaults.py
        ├── llm_proxy.py            # Main proxy class handling LLM and ROS WebSocket comms
        └── llm_adapters/           # Modular LLM provider adapters
            ├── __init__.py         # Adapter registry
            ├── base.py             # Base adapter class
            ├── ollama.py           # Ollama adapter (local)
            ├── openai.py           # OpenAI adapter
            ├── anthropic.py        # Anthropic adapter
            └── gemini.py           # Google Gemini adapter
```

---

## Interaction Logging

Every interaction with the LLM (from either the Proxy UI or the YAML automated tests) is automatically logged to the `logs/` directory at the project root. 

The logging mechanism records:
- **Timestamp** and unique interaction identifier.
- **The full structured query prompt** (including system prompt, workspace description, object lists, JSON schemas, and user commands).
- **The raw LLM response** or any connection/API errors encountered during generation.

### Log Outputs
- **Individual Logs**: Each prompt-response pair is written to a dedicated, timestamped file under `logs/interaction_<timestamp>_<uuid>.log` for easy, isolated analysis.
- **Master Log**: All interactions are sequentially appended to a unified thread-safe master file under `logs/all_interactions.log`.

---

## Testing the Proxy with YAML Scripts

Run Ollama and pull the target LLM model:
```bash
ollama serve
ollama pull gemma3:1b
```

### Running Bulk Evaluation Scripts

From project root:
```bash
python3 evaluate_proxy.py \
  --config-dir ./configs \
  --tests ./tests/basic_tests.yaml

python3 evaluate_proxy.py \
  --config-dir ./configs \
  --tests ./tests/extended_tests.yaml
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

## Configuring Accessible Info in TUI

The TUI can be utilized with 3 varying levels of verbosity for diagnostics and backend information.
These settings can be cycled through via the "Cycle Mode" button at any time in the TUI:

```text
Level 1 (Filtered): Action recipe + execution trace

Level 2 (Full Context): All prior + workspace object map and full prompt

Level 3 (Engineering): All prior + latency, CPU, and model metadata
```

---

## First Run Checklist

If the system does not respond, ensure:
```text
- Rosbridge is running on port 9090
- Middleware has been successfully launched
- Ollama is reachable (if used)
- Correct PYTHONPATH or editable install used
- No firewall blocking WebSocket connection
```

---

## Common Issues & Troubleshooting

**rosbridge connection failure**
```bash
- Confirm rosbridge_server is running
- Check port 9090 availability
```
**LLM not responding**
```bash
- Verify llm_config.json
- Confirm local server status or provider API key
```
**Import errors**
```bash
- Run from project root
- Ensure PYTHONPATH is set (export PYTHONPATH=.)
```
**Robot not moving**
```bash
- Middleware may not be fully initialized
- Check ROS2 topic/service availability
```

---

## Failure Mode Summary Table

The following table outlines common system failure modes and their origin layers.

| Failure Mode | Cause | Layer | Resolution |
|--------------|-------|--------|-----------|
| Invalid JSON output | LLM hallucination or formatting error | LLM / Proxy | Regenerate prompt or adjust system prompt |
| Schema validation failure | Missing or malformed fields | Proxy | Check json_schema.json |
| rosbridge disconnect | WebSocket server not running | ROS2 Bridge | Restart rosbridge on port 9090 |
| No robot movement | Middleware inactive or uninitialized | ROS2 Middleware | Ensure robot.launch.py is active |
| Import / module errors | Incorrect PYTHONPATH | Python Runtime | Run from repo root or set PYTHONPATH |
| Stale or repeated actions | LLM loop without state reset | LLM / Proxy | Clear context or reset session |

---

## Safety Constraints

- All LLM outputs are validated prior to execution
- Only whitelisted ROS actions are permitted
- Invalid or malformed JSON is rejected
- No direct hardware commands bypass the proxy layer

---

## Extensibility

**Add new LLM provider**
```text
- Implement adapter in src/backend/llm_adapters/
- Register in adapter factory (__init__.py)
```
**Add new robot actions**
```text
- Extend middleware ROS2 action schema
- Update validation schema in configs/json_schema.json
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
- Rosbridge is active and running on 'ws://localhost:9090'
- Middleware is running and responsive
- Proxy successfully connects to ROS2 layer
- LLM returns valid schema compliant JSON
- At least one action cycle executes end-to-end
```
