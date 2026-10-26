# Troubleshooting

Each table lists a symptom, its cause and the resolution. The messages are those of the software. The terminal UI shows the messages of the backend as `[ERR]` or `[SYS]` entries. The details are in the log files. See [Logs](running.md#logs).

## Start-up

| Symptom | Cause | Resolution |
|---|---|---|
| `[Runner] Port 8081 is already in use! Is llm-proxy already running?` | Another program listens on port 8081, for example an LLM proxy from an earlier run. | Stop that program. `lsof -i :8081` shows it. |
| `[Runner] Port 8080 is already in use! Is backend already running?` | Another program listens on port 8080. | Stop that program. `lsof -i :8080` shows it. |
| `[Runner] Binary not found: <path>` | The programs are not built. | Run `./run.sh --build`. |
| `[Runner] Timeout waiting for LLM Proxy on port 8081`, or `Backend Server on port 8080` | The program did not listen within 10 s, or neither `lsof` nor `ss` is installed. | Read `data/logs/proxy.log` or `data/logs/server.log`. Install `lsof` or `ss`. |
| `Failed to start LLM proxy server: initialize provider router: unknown LLM provider "<name>"` | `provider` is not `ollama`, `gemini`, `openai` or `anthropic`. The name is case-sensitive. | Correct `provider` in `config.json`. |
| `Failed to start app server: initialize config: failed to decode config JSON file <path>: ...` | `config.json` is not valid JSON. | Correct the file. |
| `Failed to start app server: read json schema data/config/json_schema.json: open ...: no such file or directory` | The backend does not find the data folder. A relative `-dataDir` refers to the working directory. | Start the backend in the folder of the proxy, or give an absolute `-dataDir`. |

## Connection

| Symptom | Cause | Resolution |
|---|---|---|
| `bridge: disconnected` in the header, and `cannot execute commands: no ROS 2 bridge connected` | The backend cannot reach `rosbridge_server`. `server.log` shows `[Rosbridge] connection failed or dropped (...); reconnecting in 2s...`. | Start the middleware and wait until `rosbridge_websocket` runs. Check that `server.rosbridge_url` uses the `bridge_port` of the launch. The backend connects without a restart. |
| `disconnected (... connection refused) - reconnecting...` in the header | The backend is not running. | Start the backend. The terminal UI connects by itself. |
| `disconnected (... status 409 Conflict) - reconnecting...` in the header | Another terminal UI is connected to the backend. | Close the other terminal UI. |

## Language model

| Symptom | Cause | Resolution |
|---|---|---|
| `llm proxy request failed: Post "http://localhost:8081/generate": ... connection refused` | The LLM proxy is not running. | Start the LLM proxy. |
| `llm proxy request failed: ... context deadline exceeded (Client.Timeout exceeded while awaiting headers)` | The LLM proxy did not answer within 60 s. The model needs longer for the prompt. | Use a faster model or a hosted provider. See [Choose a model](../installation/configuration.md#choose-a-model). |
| `llm proxy error: upstream provider request timed out` | The provider did not answer within `timeout_seconds`. | Raise `timeout_seconds`, up to 60, or use a faster model. |
| `llm proxy error: upstream provider request failed` | The LLM proxy cannot reach the provider: the provider is down, `base_url` is wrong, or Ollama is not running. | Check `base_url` and the network. `proxy.log` has the reason. |
| `llm proxy error: upstream provider returned an error (status <n>)` | The provider rejected the request. With Ollama, status 404 means that the model is not installed. | Read the answer of the provider in `proxy.log`. Run `ollama pull <model>`, or correct the model, the key or `base_url`. |
| `schema validation failed: not valid JSON: ...` | The answer of the model contains no JSON document. | Use a model that follows the output instructions. `server.log` has the answer. |
| `schema validation failed: schema validation failed: jsonschema: '<path>' does not validate with ...` | The JSON document does not satisfy the schema. The message names one failed constraint, at `<path>`. For an unknown action name, it names the first action of the schema, `home`. | Compare the answer in `server.log` with the schema. See [Prompt and schema](../technical/prompt-and-schema.md). |

## Commands

| Symptom | Cause | Resolution |
|---|---|---|
| `Execution aborted. Referenced object '<name>' was not found in the environment map. Available: <names>.` | The name matches no object. The same message exists for movements and orientations. | Use a name from the `Objects` line of the header. Run `/scan` if the camera adds objects. |
| `Execution aborted. '<name>' matches more than one object (<a>, <b>), and the command doesn't say which. Please specify which one.` | The name fits several objects. | Name the object more exactly. |
| `command rejected: another command is currently in progress` | A second command arrived while one was running. | Wait for the result. |
| `robot recipe execution failed: recipe execution failed: Recipe failed at step <n> (<action>): <reason>` | A step failed in the middleware. | See the [Troubleshooting](https://jeremy-allan.github.io/ROS2-middleware/user-guide/troubleshooting/) page of the middleware. |
| `robot recipe execution failed: call /execute_recipe: context deadline exceeded` | The recipe ran longer than `execution_timeout_seconds`, or the command ran longer than 5 minutes. The backend stops waiting. The middleware does not stop the recipe. | Wait until the arm stops. Raise the time limit or shorten the recipe. |

## Terminal UI

| Symptom | Cause | Resolution |
|---|---|---|
| `No response received within 300s - you can try again.` | No result arrived within 300 s. | Read `data/logs/server.log`. Send the command again. |
| `failed to copy to clipboard: ...` | No clipboard tool is installed. | Install `xclip`, `xsel` or `wl-clipboard`. |
| `environment reset failed: ...` | `/reset-env` could not call `/reset_environment` in the middleware, for example because the bridge is down. | Restore the connection, then repeat the command. |
| `scan failed: ... (is the vision node running?)` | The middleware does not run the vision node. | Start the middleware with `enable_CV:=true`. See [Camera and calibration](https://jeremy-allan.github.io/ROS2-middleware/installation/camera-and-calibration/). |

## Evaluation

| Symptom | Cause | Resolution |
|---|---|---|
| `FAIL -evalJSON` and `open -evalJSON: no such file or directory` | `-evalJSON` stands after `-eval`, so the backend reads it as a suite file. | Put `-evalJSON <file>` before `-eval`. |
| `parse <file>: json: unknown field "<name>"` | The suite has a key that the loader does not know. | Correct the key. See [Write a suite](evaluation.md#write-a-suite). |
| `<file>: case <n> has no prompt` | A case lacks `prompt`. | Add the prompt. |
| `llm proxy request failed: ...` in every case | The LLM proxy is not running. | Start it, or use `./run.sh --eval`. |
