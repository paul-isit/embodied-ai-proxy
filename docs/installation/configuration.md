# Configuration

The proxy reads its configuration from the folder `config` in its data folder. The option `-dataDir` of each program sets the data folder. A relative path refers to the working directory.

- `config.json` holds the addresses, the time limits and the settings of the language model. The backend and the LLM proxy read it. It is optional: a missing file or a missing key uses the default.
- `json_schema.json` holds the schema that every recipe must satisfy. The prompt contains it. The backend reads it, and the file is required.
- `system_prompt.md` holds the prompt template. The backend reads it. It is optional: a missing file uses a built-in copy of the shipped file.

The programs read the files at start. Restart them to apply a change. A `config.json` that is not valid JSON stops the program at start.

## Settings

`config.json` has two sections: `server` for the backend and `proxy` for the LLM proxy.

### server

| Setting | Default | Description |
|---|---|---|
| `port` | `8080` | Reported only. |
| `proxy_url` | `http://localhost:8081` | Address of the LLM proxy. |
| `rosbridge_url` | `ws://localhost:9090` | Address of `rosbridge_server`. |
| `execution_timeout_seconds` | `300` | Time limit for a recipe. |

- `port` is reported by `/api/info` and by the `/system` command. It does not change the port of the backend. The backend listens on the port of the option `-httpPort`, default 8080.
- `rosbridge_url` must use the port of the launch argument `bridge_port` of the middleware.
- `execution_timeout_seconds` is the time that the backend waits for the middleware to finish a recipe. One command, including the call to the model, stops after 5 minutes whatever the value.

### proxy

| Setting | Default | Description |
|---|---|---|
| `port` | `8081` | Reported only. |
| `llm_config.provider` | `ollama` | The provider. |
| `llm_config.model` | `gemma3:1b` | The model name. |
| `llm_config.base_url` | `http://localhost:11434/api/generate` | Address of the provider. |
| `llm_config.api_key` | empty | Key of the provider. |
| `llm_config.max_tokens` | `1024` | Limit for the answer. |
| `llm_config.temperature` | `0.1` | Sampling temperature. |
| `llm_config.timeout_seconds` | `30` | Time limit for a request. |

- `port` does not change the port of the LLM proxy. The LLM proxy listens on the port of the option `-httpPort`, default 8081.
- `provider` is `ollama`, `gemini`, `openai` or `anthropic`. The value is case-sensitive.
- `model` is the name that the provider uses.
- `api_key` is not used by Ollama.
- `max_tokens` limits the length of the answer, in tokens.
- `timeout_seconds` is the time limit for a request to the provider, including the retry. The backend stops waiting for the LLM proxy after 60 s, whatever the value.

## Providers

| `provider` | Key |
|---|---|
| `ollama` | None. |
| `gemini` | `api_key`, in the parameter `key` of the URL. |
| `openai` | `api_key`, as a bearer token. |
| `anthropic` | `api_key`, in the header `x-api-key`. |

Use these values of `base_url`:

```text
ollama     http://localhost:11434/api/generate
gemini     https://generativelanguage.googleapis.com/v1beta/models
openai     https://api.openai.com/v1/chat/completions
anthropic  https://api.anthropic.com/v1/messages
```

Each adapter sends the prompt with a `POST` request:

- `ollama` sends the request to `base_url`, with `stream` set to false and `format` set to `json`.
- `gemini` sends the request to `<base_url>/<model>:generateContent`.
- `openai` sends the request to `base_url`, with the chat completions API.
- `anthropic` sends the request to `base_url`, with the Messages API and the header `anthropic-version` set to `2023-06-01`.

The LLM proxy repeats a failed request once, after 250 ms, for a connection error, the HTTP status 429 or a 5xx status. It does not repeat a request after a timeout or after another status.

To use Google Gemini, set the provider, the model, the address and the key. The other settings keep their defaults:

```json
{
  "proxy": {
    "llm_config": {
      "provider": "gemini",
      "model": "gemini-2.5-flash",
      "base_url": "https://generativelanguage.googleapis.com/v1beta/models",
      "api_key": "<key>"
    }
  }
}
```

Restart the proxy, then type `/llm` in the terminal UI. The output shows the provider and the model.

## Choose a model

- The model must return one JSON document. The Ollama adapter requests the JSON format. The other adapters rely on the prompt.
- The model must hold the prompt, 62 KB with the shipped files and the six default objects. See [Installation](installation.md#set-up-a-language-model).
- The backend stops waiting for an answer after 60 s, whatever `timeout_seconds` is. A model that needs longer fails the command.

## Protect the key

`api_key` is stored in plain text in `config.json`. Do not share or commit a file that contains a key. `/api/info` and the terminal UI leave the key out.

## Edit the prompt or the schema

See [Prompt and schema](../technical/prompt-and-schema.md).
