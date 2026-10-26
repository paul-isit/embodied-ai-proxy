# Run the proxy

`run.sh` starts the LLM proxy and the backend in the background and the terminal UI in the foreground. Closing the terminal UI stops the other two programs.

## Prerequisites

- The proxy is built. See [Installation](../installation/installation.md).
- The language model is reachable. See [Configuration](../installation/configuration.md).
- The middleware runs, in simulation or on the real arm. See the [Quickstart](https://jeremy-allan.github.io/ROS2-middleware/user-guide/quickstart/) of the middleware. The proxy starts without the middleware, but it refuses commands until the bridge connects.

## Start the proxy

1. Start the programs from the folder of the proxy:

    ```bash
    ./run.sh
    ```

    The script checks that the ports 8081 and 8080 are free, then starts the programs:

    ```text
    [Runner] Starting LLM Proxy on :8081 (log: data/logs/proxy.log)...
    [Runner] LLM Proxy is UP (PID: 14247)
    [Runner] Starting Backend Server on :8080 (log: data/logs/server.log)...
    [Runner] Backend Server is UP (PID: 14255)
    [Runner] Launching TUI...
    ```

2. Read the header of the terminal UI. The proxy is ready when the first line shows `[connected]` and `bridge: connected`:

    ```text
    Backend: http://localhost:8080 [connected] | bridge: connected
    LLM: ollama/gemma3:1b
    ```

3. Type a command. See [Terminal UI](terminal-ui.md).

## Options of run.sh

| Option | Description |
|---|---|
| none | Starts the LLM proxy, the backend and the terminal UI. |
| `--build`, `-b` | Builds the programs first. |
| `--headless`, `-s` | Starts the LLM proxy and the backend only. |
| `--eval FILE...` | Runs test suites and exits. Must be the last option. |
| `--build-only` | Builds the programs and exits. |
| `--clean` | Deletes the files in `bin` and in `data/logs`. |
| `--help`, `-h` | Prints the options. |

- With `--headless`, press Ctrl+C to stop the programs.
- With `--eval`, the exit code is 1 when a case fails. See [Evaluate a language model](evaluation.md).
- The script stops with an error when the port 8081 or 8080 is in use or when a program is not built. See [Troubleshooting](troubleshooting.md).
- The script passes its own `data` folder to the programs and uses the default ports 8080 and 8081.
- The output of the LLM proxy and the backend goes to the log files.

## Run the programs separately

Start each program in its own terminal, from the folder of the proxy:

```bash
./bin/llm-proxy -dataDir data -httpPort 8081
./bin/backend -dataDir data -httpPort 8080
./bin/tui -serverURL http://localhost:8080 -dataDir data
```

The option `-dataDir` is the data folder. It holds the folder `config`, with the files that the program reads, and the folder `logs`. A relative path refers to the working directory of the program.

**llm-proxy**

| Option | Default | Description |
|---|---|---|
| `-dataDir` | `data` | Data folder. |
| `-httpPort` | `8081` | Port of the LLM proxy. |

**backend**

| Option | Default | Description |
|---|---|---|
| `-dataDir` | `data` | Data folder. |
| `-httpPort` | `8080` | Port of the backend. |
| `-eval` | off | Runs the test suites that follow, then exits. |
| `-evalJSON` | none | Writes one JSON object per case to this file. |

**tui**

| Option | Default | Description |
|---|---|---|
| `-serverURL` | `http://localhost:8080` | Address of the backend. |
| `-dataDir` | `data` | Data folder. |

`-evalJSON` works with `-eval` and must come before it. The terminal UI writes `logs/tui.log` and the saved sessions to its data folder.

## Connect to the middleware

The backend connects to `rosbridge_server` at the address `server.rosbridge_url`, by default `ws://localhost:9090`. The launch file of the middleware starts `rosbridge_server` after `json_parser_node` starts.

- The backend retries every 2 s while the bridge is down and after a lost connection.
- While the bridge is down, the header shows `bridge: disconnected`, and the backend refuses commands with `cannot execute commands: no ROS 2 bridge connected`.
- After the connection, the backend subscribes to `/system/status`. Before every command it reads the objects, movements, orientations, table bounds and held objects from the middleware.
- If the launch argument `bridge_port` of the middleware is not 9090, set the same port in `server.rosbridge_url`. See [Configuration](../installation/configuration.md#server).

## Stop the proxy

Press Ctrl+C in the terminal UI. `run.sh` then stops the backend and the LLM proxy. With `--headless`, press Ctrl+C in the terminal of `run.sh`.

Stop the proxy before the middleware. While the bridge is gone, the backend logs a failed connection every 2 s.

## Logs

The programs write their logs to the folder `data/logs`:

- `server.log`: the backend. Connections, prompts, recipes and calls to the middleware.
- `proxy.log`: the LLM proxy. Requests to the provider, retries and errors.
- `tui.log`: the terminal UI. Connection events.
- `eval.log`: the backend with `-eval`. Prompts and answers of an evaluation.
- `session_<date>_<time>.txt`: a session of the terminal UI, written by `/save`.

`server.log` holds the full prompt of each command and the answer of the model when it is invalid. The programs append to the files and do not rotate them. The backend and the LLM proxy also print the log to the terminal when they are started separately.
