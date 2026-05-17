# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project facilitates communication between high-level Large Language Models (LLMs) and robotic systems by bridging a pure Python inference environment with a ROS2 hardware abstraction layer.

## System Architecture

The project is divided into two distinct domains to ensure hardware stability and dependency isolation:

1.  **Inference Domain (Python):** A pure Python environment that manages LLM logic (via local Ollama APIs), prompt engineering, and the Textual user interface. It acts as a WebSocket client.
2.  **ROS2 Bridge Domain:** A dedicated ROS2 workspace owned by the proxy. It acts as a wrapper around the system-level `rosbridge_suite`, launching a WebSocket server (port `9090`) to translate JSON requests into native ROS2 Service calls for the Swinburne Kinova middleware.

## Project Structure

```text
embodied-ai-proxy/
├── main.py                         # System entry point
├── evaluate_proxy.py               # Bulk testing from YAML file  
├── mock_middleware.py              # Mock ROS2 node for testing without hardware
├── configs/                        # Configuration and validation rules
│   ├── llm_config.json             # Ollama API settings and model selection
│   ├── system_prompt.md            # Base instructions for the LLM
│   └── json_schema.json            # JSON schema for output validation
│
├── ros2_bridge_ws/                 # Domain 1: ROS2 Workspace
│   └── src/
│       └── custom_bridge_pkg/      # ROS2 package wrapping rosbridge
│           ├── package.xml
│           ├── setup.py
│           └── launch/
│               └── proxy_bridge.launch.py # Launches rosbridge_server
│
├── tests                           # YAML files containing test scripts
|    ├── basic_tests.yaml           # Short list of tests 
|    
|
│
└── src/                            # Domain 2: Python Inference Environment
    ├── requirements.txt            # Python dependencies (websockets, requests, textual, pydantic)
    ├── frontend/                   # UI Components
    │   ├── tui_app.py              # Terminal User Interface
    │   ├── styles.css              # TUI styling and layout config
    │   ├── proxy_client.py         # Handles backend / bridge communication
    │   │        
    │   └── components/             # Further configurations for TUI
    │        ├── formatter.py       # Formats output for display in TUI
    │        ├── input_bar.py       # Handles user command input
    │        ├── log_panel.py       # Displays logs, recipes and system output
    │        └── status_panel.py    # Displays connection and system status
    │
    └── backend/                    # Core Logic
        ├── defaults.py
        └── llm_proxy.py            # Main proxy class handling LLM and ROS WebSocket comms
```

## Setup and Installation

### 1. Install System Dependencies
The bridge relies on the ROS2 `rosbridge_suite` to handle complex WebSocket-to-ROS translation.
```bash
sudo apt-get update
sudo apt-get install ros-humble-rosbridge-suite
```

### 2. Install Python Dependencies
```bash
pip install -r src/requirements.txt
```

### 3. Build the Bridge Workspace
```bash
cd ros2_bridge_ws
colcon build
```

## Running the System

To run the full end-to-end pipeline, you need to open multiple terminals:

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
```bash
python3 main.py
```

## Configuration
Update `configs/llm_config.json` to point to your local LLM provider. By default, it expects an Ollama instance running locally (e.g., `http://localhost:11434/api/generate`). Ensure the `format` is set properly in `llm_proxy.py` to enforce strict JSON output from the model.
## Testing the proxy with YAML scripts

Run Ollama and pull the LLM
```bash
ollama serve
ollama pull gemma3:1b
```

## Set PYTHONPATH

This function uses local imports
```bash
echo 'export PYTHONPATH=.' >> ~/.bashrc
source ~/.bashrc
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
```bash
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

```bash
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

The TUI can be utilized with 3 varying levels of access to diagnostics and backend information.

General Access - Minimal Info

```bash
python3 main.py
```

Level 1 - All prior access + raw prompt

```bash
python3 main.py -v
```

Level 2 - All prior + rendered system prompt

```bash
python3 main.py -vv
```
