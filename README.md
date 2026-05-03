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
  --tests ./tests/basic_tests.yaml / --tests ./tests/extended_tests.yaml
```

## Create testing YAML script

Test cases are defined here, example structure here:
```bash
tests:

  - name: Pick apple
    prompt: "pick up apple"
    available_objects: ["apple", "banana"]
    expected:
      action: "pick"
      target: "apple"
```
Required fiels per test include
```bash
| Field             | Type   | Description          |
| ----------------- | ------ | -------------------- |
| name              | string | Test label           |
| prompt            | string | User input to LLM    |
| available_objects | list   | Environment context  |
| expected          | dict   | Expected JSON output |
```
Place completed YAML file in the "tests" folder

## Example Test Execution

Run
```bash
python3 evaluate_proxy.py --config-dir ./configs --tests ./tests/basic_tests.yaml
```

## Output Example

```bash
[Test 1] Pick apple
  Result: PASS
  Expected: {'action': 'pick', 'target': 'apple'}
  Got     : {'action': 'pick', 'target': 'apple'}

[Test 2] Wave greeting
  Result: FAIL
  Expected: {'action': 'wave', 'target': 'self'}
  Got     : {'action': 'wave', 'target': 'goodbye'}
```

