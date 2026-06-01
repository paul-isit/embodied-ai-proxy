# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project facilitates communication between high-level Large Language Models (LLMs) and robotic systems by bridging a pure Python inference environment with a ROS2 hardware abstraction layer.

## Prerequisites 
Before installing the proxy, ensure your host machine meets the following requirements

### Software Requirements##
**Operating System** Ubuntu 22.04 LTS
**ROS2** Active installation of ROS2 Humble
**Language** Python 3.10+
**Local Inference Engine** An active installation of Ollama (https://ollama.com/) is required if running local models

### Hardware Requirements##
The project allows for an interchangeable LLM API to be utilized, scaling to users host machine's hardware capacity. Reccomended specs for stable performance are:
| Component | Requirement                            |
|-----------|----------------------------------------|
| RAM       | 8GB minimum, 32GB recommended          |
| CPU       | 6+ cores recommended                   |
| GPU       | Optional (recommended for larger LLMs) |
| Storage   | ≥15GB free space                       |

### External Dependency
This application relies on an external ROS2 Kinova Middleware repository (not included in this repo)

## System Architecture

The project is divided into two distinct domains to ensure hardware stability and dependency isolation:

1.  **Inference Domain (Python):** A pure Python environment that manages LLM logic (via local Ollama APIs), prompt engineering, and the Textual user interface. It acts as a WebSocket client.
Responsible for
```text
- LLM orchestration (Ollama / OpenAI / Anthropic / Gemini etc.)
- Prompt construction and system instruction enforcement
- JSON schema validation
- Terminal User Interface (TUI)
- WebSocket client communication with ROS bridge
```
This layer never interacts directly with robot hardware

2.  **ROS2 Bridge Domain:** A dedicated ROS2 workspace owned by the proxy. It acts as a wrapper around the system-level `rosbridge_suite`, launching a WebSocket server (port `9090`) to translate JSON requests into native ROS2 Service calls for the Swinburne Kinova middleware.
Responsible for
```text
- Hosting 'rosbridge_suite' WebSocket server (ws://localhost:9090)
- Translating JSON requests into ROS2 service calls
- Interfacing with external robot middleware (Kinova arm)
- Executing validation motion and gripper commands
```
This layer is the only component permitted to interact with ROS2 hardware or simulation

## System Contract Definition

The Embodied AI Proxy enforces a strict execution contract between the LLM layer and ROS2 middleware.

### Input Contract (User < LLM)
- Input: Natural language command (string)
- Processed by: LLM adapter (Ollama / OpenAI / Anthropic / Gemini)

No direct robot commands are accepted from user input.

### Output Contract (LLM → Proxy)

All LLM outputs MUST conform to the validated JSON schema defined in:
```bash
configs/json_schema.json
```
Expected structure:
```bash
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
├── tests                           # YAML files containing test scripts
|    └── basic_tests.yaml           # Short list of tests 
|    
|
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

## Setup and Installation

### 1. Install System Dependencies
The bridge relies on the ROS2 `rosbridge_suite` to handle complex WebSocket-to-ROS translation.
```bash
sudo apt-get update
sudo apt-get install ros-humble-rosbridge-suite
```

### 2. Install Python Dependencies
The project will require active installation of Python dependencies
```bash
pip install -r src/requirements.txt
```

### 3. Build the Bridge Workspace
```bash
cd ros2_bridge_ws
colcon build
```

## Set PYTHONPATH

This function uses local imports
```bash
echo 'export PYTHONPATH=.' >> ~/.bashrc
source ~/.bashrc
```

## Running the System

To run the full end-to-end pipeline, you need to open three terminals:

### Terminal 1: Start the ROS2 Bridge
```bash
source /opt/ros/humble/setup.bash
cd ros2_bridge_ws
source install/setup.bash
ros2 launch custom_bridge_pkg proxy_bridge.launch.py
```
*(The bridge is now listening on ws://localhost:9090)*

### Terminal 2: Start the Middleware
Run the ROS2 Middleware using rviz2
```bash
cd /path/to/your/ROS2-middleware/
colcon build
source install/setup.bash
ros2 launch kinova_interface robot.launch.py
```

### Terminal 3: Start the Proxy UI
Run the following command to enter main application
```bash
python3 main.py
```

## Configuration
Update `configs/llm_config.json` to point to your LLM provider. By default, it expects an Ollama instance running locally (e.g., `http://localhost:11434/api/generate`), leaving the API key field blank. No code changes are needed to switch providers or models, just update the 'llm_configs.json' file to your respective configurations of your provider. Ensure all fields are correct to the provider and the API key is working. The config file defines:
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

## Testing the proxy with YAML scripts

Run Ollama and pull the LLM
```bash
ollama serve
ollama pull gemma3:1b
```

## Running bulk scripts

From project root:
```bash
python3 evaluate_proxy.py \
  --config-dir ./configs \
  --tests ./tests/basic_tests.yaml

python3 evaluate_proxy.py \
  --config-dir ./configs \
  --tests ./tests/extended_tests.yaml
```

## Create testing YAML script

Test cases are defined here, example structure here:
```yaml
tests:

  - name: Pick apple
    prompt: "pick up the apple"

    available_objects:
      - apple
      - banana
      - tray

    expected_actions:
      - home
      - gripper
      - move_arm
      - gripper
      - relative_move
```
Required fiels per test include
```bash
| Field             | Type   | Description                         |
|------------------|--------|-------------------------------------|
| name             | string | Human-readable test label           |
| prompt           | string | User command sent to the LLM        |
| available_objects| list   | Objects available in environment    |
| expected_actions | list   | Expected robot action sequence      |
```
Place completed YAML file in the "tests" folder

## Example Test Execution

Run
```bash
python3 evaluate_proxy.py \
  --config-dir ./configs \
  --tests ./tests/basic_tests.yaml
```

## Output Example

```yaml
[Test 1] Pick apple and place on tray
  Expected Actions: ['home', 'gripper', 'move_arm', 'gripper']
  Actual Actions  : ['home', 'gripper', 'move_arm', 'gripper', 'relative_move', 'move_arm', 'gripper', 'home']
  Result: PASS

[Test 2] Inspect apple
  Expected Actions: ['home', 'move_arm', 'relative_move']
  Actual Actions  : ['home', 'move_arm', 'relative_move', 'home']
  Result: PASS
```
## Configuring accessible info for TUI

The TUI can be utilized with 3 varying levels of verbosity to diagnostics and backend information.
These settings can be cycled through via the "Cycle Mode Button" at any time via the TUI

```bash
Level 1 (Filtered): Action recipe + execution trace

Level 2 (Full Context): All prior + workspace object map

Level 3 (Engineering): All prior + latency, CPU, model metadata
```
## First Run Checklist

If the system does not respond, ensure
```bash
- Rosbridge is running on port 9090
- Middleware has been successfuly launched
- Ollama is reachable (if used)
- Correct PYTHONPATH or editable install used
- No firewall blocking WebSocket connection
```
## Common Issues & Troubleshooting ##

rosbridge connection failure
```bash
- Confirm rosbridge_serve is running
- Check port 9090 availability
```
LLM not responding
```bash
- Verify llm_config.json
- Confirm local server status or provider API key
```
Import errors
```bash
- Run from project root
- Ensure pip install -e or PYTHONPATH is set
```
Robot not moving
```bash
- Middleware may not be fully initialized
- Check ROS2 topic/service availability
```

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

## Safety Constraints

- All LLM outputs are validated prior to execution
- Only whitelisted ROS actions are permitted
- Invalid or malformed JSON is rejected
- No direct hardware commands bypass the proxy layer

## Extensibility

Add new LLM provider
```bash
- Implement adapter in llm_adapters/
- Register in adapter factory
```
Add new robot actions
```bash
- Extend middleware ROS2 action schema
- Update validation schema in configs/json_schema.json
```

## Notes on Performance

- LLM latency will vary between providers and model size
- ROS2 execution latency is non-deterministic under load
- Recommended to run on dedicated machine for robotic experiments

## Mininmum Working System Definition
The system is only operational if the following criteria are met:
```text
- Rosbridge is active and running on 'ws://localhost:9090'
- Middleware is running and responsive
- Proxy successfully connects to ROS2 layer
- LLM returns valid schema compliant JSON
- At least one action cycle executes end-to-end
```
