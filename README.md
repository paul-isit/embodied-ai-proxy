# Embodied AI Proxy

A proxy layer for embodied AI systems leveraging ROS2 middleware. This project facilitates communication between high-level Large Language Models (LLMs) and robotic systems by bridging a pure Python inference environment with a ROS2 hardware abstraction layer.

## System Architecture

The project is divided into two distinct domains to ensure hardware stability and dependency isolation:

1.  **ROS2 Domain:** A dedicated workspace that handles real-time robotics communication and exposes hardware states via a WebSocket server.
2.  **Inference Domain:** A pure Python environment that manages LLM logic, prompt engineering, and the user interface without requiring ROS2 dependencies locally.

## Project Structure

```text
embodied-ai-proxy/
├── main.py                         # System entry point
├── configs/                        # Configuration and validation rules
│   ├── system_prompt.txt           # Base instructions for the LLM
│   └── json_schema.json            # JSON schema for output validation
│
├── ros2_bridge_ws/                 # Domain 1: ROS2 Workspace
│   └── src/
│       └── custom_bridge_pkg/      # ROS2 Python package
│           ├── package.xml
│           ├── setup.py
│           └── custom_bridge_pkg/
│               ├── __init__.py
│               └── bridge_node.py  # ROS2 node and WebSocket server
│
└── src/                            # Domain 2: Python Inference Environment
    ├── requirements.txt            # Python dependencies
    │
    ├── frontend/                   # UI Components
    │   ├── __init__.py
    │   └── tui_app.py              # Terminal User Interface
    │
    └── backend/                    # Core Logic
        ├── __init__.py
        ├── proxy_client.py         # WebSocket client for ROS2 bridge
        ├── prompt_engine.py        # Context management and formatting
        ├── json_validator.py       # LLM output verification
        ├── llm_handler.py          # Model loading and inference
        └── llm_proxy.py            # Main backbone class for the proxy server
```

## Running the proxy with TUI

```bash
pip install -r src/requirements.txt
python3 main.py
```