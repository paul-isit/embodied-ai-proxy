# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project facilitates communication between high-level Large Language Models (LLMs) and robotic systems by bridging a pure Python inference environment with a ROS2 hardware abstraction layer.

## System Architecture

The project is divided into two distinct domains to ensure hardware stability and dependency isolation:

1.  **Inference Domain (Python):** A pure Python environment that manages LLM logic (via local Ollama APIs), prompt engineering, and the Textual user interface. It acts as a WebSocket client.
2.  **ROS2 Bridge Domain:** A dedicated ROS2 workspace owned by the proxy. It acts as a wrapper around the system-level `rosbridge_suite`, launching a WebSocket server (port `9090`) to translate JSON requests into native ROS2 Service calls for the Swinburne Kinova middleware.

## Project Structure

```text
embodied-ai-proxy/
├── main.py                         # System entry point (Starts the UI)
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
└── src/                            # Domain 2: Python Inference Environment
    ├── requirements.txt            # Python dependencies (websockets, requests, textual, pydantic)
    ├── frontend/                   # UI Components
    │   └── tui_app.py              # Terminal User Interface
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
