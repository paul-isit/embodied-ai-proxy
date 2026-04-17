Overview:

This demonstration spike package provides a means of Natural Language Processing (NLP) bridge for the Kinova Robotic Arm. This package consists of a locally hosted Llama 3.1 model, and a Python based validation proxy that ensures human generated instructions and converted into safe, executable ROS2 JSON commands. This is purely for research and/or testing purposes only, and has been developed as part of an educational spike experience

Architecture Pipeline:

User Text < HTTP POST to Proxy < LLM Analysis < Safety / Whitelist Validation < Structured JSON Output

What?s a Proxy, and What is its Role?

A Proxy is an intermediary server between the client (user), and a service (LLM). In this demonstration, the proxy?s role is to serve as a robust safety enforcement mechanism. This is facilitated through

? Abstraction: Complexity of prompt engineering and API management is kept hidden from the user
? Validation: Active interception of LLM?s response against a predetermined set of hardware specific actions on whitelist
? Safety: Prevents robot from attempting hallucinated commands such as flight by rejecting any responses that do not match the Kinova arm?s physical capabilities

Installation & Setup:

1) Install Local LLM Engine (Ollama) https://www.canirun.ai/model/llama3.1-8b
? sudo apt update
? sudo apt install -y zstd
? curl -fsSl https://ollama.com/install.sh | sh
? sudo systemctl enable ollama
? sudo systemctl start ollama
? ollama pull llama3.1:8b
? ollama run llama3.1:8b

2) Setup Workspace
? mkdir ~/ros2_llm_ws
? cd ~/ros2_llm_ws
? Save ?ask_llama.py & llm_proxy.py here?

3) Launch Service
. Launch 3 terminals
. In terminal 1 (LLM Engine), launch Ollama by entering ?ollama serve?
. In terminal 2, start Proxy by entering ?python3 llm_proxy.py?
. In terminal 3 (User Client), run program with ask_llama.py
) ?python3 ask_llama.py ?move forward 1 meter then stop?

Testing & Troubleshooting:

Command Suite -

Test Case
Command
Expected Result
Valid Move
python3 ask_llama.py "rotate 90 degrees"
SUCCESS with JSON payload.
Invalid Move
python3 ask_llama.py "fly to the ceiling"
BLOCKED (Safety whitelist triggered).
Not Applicable / Invalid
python3 ask_llama.py "tell me a joke"
BLOCKED (No valid actions detected).

Troubleshooting

? Error: Connection Refused
? Unexpected ?SUCCESS? for unsafe/invalid commands
? JSON parsing error:

How this builds upon original ask_llama.py

The original ask_llama.py was utilized as a standalone CLI tool, this package facilitates that concept through an active verification mechanism. The core enhancements made include

1) Ongoing persistent services: Unlike the original script which executed single commands prior to closing, the Proxy is constantly running and capable for ROS2 integration
2) Separation of concerns: ask_llama.py now only handles display, while logic and safety parameters exist in llm_proxy.py
3) Intent validation: LLM is bound by negative constraint prompt, ensuring LLM does not guess but explicitly reject commands that are not on whitelist, adhering to core requirement of physical hardware safety.
