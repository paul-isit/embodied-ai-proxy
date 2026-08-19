package config

// Default values used when data/config/config.json is missing or omits a
// field. Kept in one place so the backend and LLM proxy never drift.
const (
	DefaultServerPort = 8080
	DefaultProxyPort  = 8081
	DefaultProxyURL   = "http://localhost:8081"

	DefaultLLMProvider       = "ollama"
	DefaultLLMModel          = "gemma3:1b"
	DefaultLLMBaseURL        = "http://localhost:11434/api/generate"
	DefaultLLMAPIKey         = ""
	DefaultLLMMaxTokens      = 1024
	DefaultLLMTemperature    = 0.1
	DefaultLLMTimeoutSeconds = 30
)
