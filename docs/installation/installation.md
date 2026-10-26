# Installation

This page builds the proxy and sets up a language model. Configure the proxy afterwards. See [Configuration](configuration.md).

## Requirements

| Item | Requirement |
|---|---|
| Operating system | Linux, macOS or WSL 2, with `bash`. `run.sh` also needs `lsof` or `ss`. |
| Go | 1.26.5 or later. |
| Language model | One provider: Ollama on this computer or on a reachable host, or an API key for Google Gemini, OpenAI or Anthropic. |
| Middleware | The ROS 2 middleware, in simulation or on the real arm, with `rosbridge_server`. The proxy starts without it, but commands need it. |

The proxy uses these ports:

| Port | Service |
|---|---|
| 8080 | Backend. |
| 8081 | LLM proxy. |
| 9090 | `rosbridge_server`, started by the launch file of the middleware. |
| 11434 | Ollama. |

## Install the proxy

1. Clone the repository:

    ```bash
    git clone https://github.com/paul-isit/embodied-ai-proxy.git
    cd embodied-ai-proxy
    ```

2. Build the three programs. The first build downloads the Go modules.

    ```bash
    ./run.sh --build-only
    ```

    The output lists the programs:

    ```text
    [Build] Building Go binaries...
      -> Building bin/llm-proxy...
      -> Building bin/backend...
      -> Building bin/tui...
    [Build] Build complete!
    ```

3. Check that the programs exist:

    ```bash
    ls bin
    ```

    The output is `backend  llm-proxy  tui`.

To build without `run.sh`, run these commands. The file `go.work` links the modules `backend`, `llm-proxy`, `shared` and `tui`.

```bash
go build -o bin/llm-proxy ./llm-proxy/cmd/server
go build -o bin/backend ./backend/cmd/server
go build -o bin/tui ./tui/cmd/tui
```

## Set up a language model

The default configuration uses the Ollama model `gemma3:1b` at `http://localhost:11434`.

1. Install [Ollama](https://ollama.com) and download the model:

    ```bash
    ollama pull gemma3:1b
    ```

2. Check that Ollama answers:

    ```bash
    curl http://localhost:11434
    ```

    The answer is `Ollama is running`.

The backend sends the whole prompt with every command. With the shipped files and the six default objects, the prompt is 62 KB. The model needs a context length that holds it. Ollama chooses the context length from the video memory, and uses 4k tokens below 24 GiB (see the [Ollama documentation](https://docs.ollama.com/context-length)). Start Ollama with a larger value, for example `OLLAMA_CONTEXT_LENGTH=32768 ollama serve`.

To use Google Gemini, OpenAI or Anthropic, set the provider, the model and the key in `config.json`. See [Providers](configuration.md#providers).

Next: [Run the proxy](../user-guide/running.md).
