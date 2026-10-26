# Interfaces

The backend provides three HTTP endpoints and one websocket to the terminal UI. The LLM proxy provides one HTTP endpoint to the backend. The backend uses services and one topic of the middleware through `rosbridge_server`.

## Backend HTTP

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/info` | Configuration and state of the backend. |
| `POST` | `/api/reset` | Resets the scene of the middleware. |
| `POST` | `/api/scan` | Takes a camera snapshot. |

- `/api/info` replies with the sections `server` and `llm` of `config.json`, the state of the bridge, the number of connected terminal UIs and the prompt template. The key `api_key` of `llm` is always empty.
- `/api/reset` calls the service `/reset_environment`. The reply is `{"success": true, "message": "<text>"}`. On a failure `success` is false and `message` holds the reason.
- `/api/scan` calls the service `/vision/snapshot` with `remove_missing` set to true. The reply has `success`, `message`, `added` and `removed` (lists of names) and `unchanged` (a number). The call has 30 s.
- `/api/reset` and `/api/scan` reply to any other method with the HTTP status 405 and `{"success": false, "message": "method not allowed"}`. A failed call to the middleware replies with the status 200 and `success` set to false.

```bash
curl -s http://localhost:8080/api/info
```

```json
{
  "server": {
    "port": 8080,
    "proxy_url": "http://localhost:8081",
    "rosbridge_url": "ws://localhost:9090",
    "execution_timeout_seconds": 300
  },
  "llm": {
    "provider": "ollama",
    "model": "gemma3:1b",
    "base_url": "http://localhost:11434/api/generate",
    "api_key": "",
    "max_tokens": 1024,
    "temperature": 0.1,
    "timeout_seconds": 30
  },
  "bridge_connected": true,
  "clients_connected": 0,
  "system_prompt": "You are an advanced robotic assistant that translates natural language commands ..."
}
```

## Backend websocket

The terminal UI connects to `ws://<host>:8080/ws/client`. The backend accepts one connection at a time and refuses a second one with the HTTP status 409. Every message is a JSON object with a `type` and a `payload`:

```json
{ "type": "prompt_submit", "payload": { "prompt": "go home" } }
```

| Type | Direction | Payload |
|---|---|---|
| `prompt_submit` | Terminal UI to backend | A command. |
| `status_update` | Backend to terminal UI | The state of the bridge, the scene or the middleware. |
| `action_recipe` | Backend to terminal UI | A recipe that the middleware has run. |
| `log_event` | Backend to terminal UI | A message or an error document. |

- `prompt_submit` has the payload `{"prompt": "<text>"}`. The backend ignores messages of other types.
- `action_recipe` holds the recipe after name resolution. The backend sends it when the middleware has finished the recipe.
- `log_event` has the payload `{"level": "info", "message": "<text>"}` or `{"level": "error", "message": "<text>"}`. A refusal is sent as the error document itself, with `status`, `error_type` and `message` and without `level`.

A `status_update` payload has one of these keys:

| Key | Value |
|---|---|
| `bridge_connected` | `true` or `false`. |
| `object_list` | The object names. |
| `movement_names` | The movement names. |
| `orientation_names` | The orientation names. |
| `table_bounds` | `available`, `x_min`, `x_max`, `y_min` and `y_max`. |
| `middleware_status` | A message of `/system/status`. |

- `bridge_connected` is sent when a terminal UI connects and when the state of the bridge changes.
- The names and `table_bounds` are sent each time the backend reads the scene.
- `middleware_status` has `summary_state` and `individual_states`. Each entry of `individual_states` has `node_name`, `state`, `status_message` and `last_command_valid`. The states are 0 (ready), 1 (busy) and 2 (fault).

For a command, the backend sends four `status_update` messages with the scene. A recipe that ran is followed by an `action_recipe` message and a `log_event` message with the level `info`. A refusal or an error is followed by one `log_event` message.

## LLM proxy

`POST /generate` returns the text that the configured provider produces for a prompt. The body of a request can have up to 1 MiB.

| Request key | Description |
|---|---|
| `prompt` | Required. The text to send. |
| `system_instruction` | Optional. A system message. |
| `max_tokens` | Optional. Replaces the value of `config.json`. |
| `temperature` | Optional. Replaces the value of `config.json`. |

The backend sends only `prompt`.

| Status | Body |
|---|---|
| 200 | `{"text": "<text>", "finish_reason": "<reason>"}` |
| 400 | `{"error": "invalid request body: <reason>"}` or `{"error": "prompt is required"}` |
| 405 | `{"error": "method not allowed"}` |
| 502 | `{"error": "upstream provider request failed"}` or `{"error": "upstream provider returned an error (status <n>)"}` |
| 504 | `{"error": "upstream provider request timed out"}` |

The answer of the provider is written to `proxy.log` and is not returned to the caller.

## Middleware

The backend is a client of `rosbridge_server` and uses these names of the middleware. The interfaces are described in [ROS interfaces](https://jeremy-allan.github.io/ROS2-middleware/technical/ros-interfaces/) of the manual of the middleware.

| Name | Kind | Use |
|---|---|---|
| `/system/status` | Topic | Status of the nodes. |
| `/get_robot_parameters` | Service | Names, table bounds and held objects. |
| `/execute_recipe` | Service | Runs a recipe. |
| `/reset_environment` | Service | Resets the scene. |
| `/vision/snapshot` | Service | Takes a camera snapshot. |

- The backend subscribes to `/system/status`, of the type `kinova_interfaces/msg/SystemSummary`, when it connects, and forwards each message as `middleware_status`.
- The backend calls `/get_robot_parameters` at connection and before every command.
- `/execute_recipe` has the request field `recipe_json`, which holds the recipe.
- `/reset_environment` is a `std_srvs/Trigger` service. The command `/reset-env` calls it.
- `/vision/snapshot` has the request field `remove_missing`, which the backend sets to true. The command `/scan` calls it.
