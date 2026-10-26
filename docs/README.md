# Embodied AI Proxy documentation

This documentation describes the proxy of the LLM-controlled Kinova robot arm: how to install, configure and run it, how it processes a command, and which interfaces it provides. The ROS 2 middleware, its actions and its recipes are described in the [Kinova Middleware Manual](https://jeremy-allan.github.io/ROS2-middleware/).

| Section | Contents |
|---|---|
| [Installation](installation/installation.md) | Requirements, build and language model. |
| [Configuration](installation/configuration.md) | `config.json`, providers, the prompt and the schema. |
| [Run the proxy](user-guide/running.md) | Start and stop the programs, connect to the middleware, logs. |
| [Terminal UI](user-guide/terminal-ui.md) | Screen, commands, keys and detail levels. |
| [Evaluate a language model](user-guide/evaluation.md) | Run test suites without the robot. |
| [Troubleshooting](user-guide/troubleshooting.md) | Symptoms, causes and resolutions. |
| [Architecture](technical/architecture.md) | Programs, request flow, modules, limits and security. |
| [Command pipeline](technical/command-pipeline.md) | The steps from a command to a recipe and their failures. |
| [Prompt and schema](technical/prompt-and-schema.md) | The prompt template and the recipe schema. |
| [Interfaces](technical/interfaces.md) | HTTP endpoints, websocket messages and the services used on the middleware. |
