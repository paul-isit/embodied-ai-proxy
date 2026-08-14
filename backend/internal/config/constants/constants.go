package constants

const (
	DefaultServerPort        = 8080
	DefaultLLMProvider       = "ollama"
	DefaultLLMModel          = "gemma3:1b"
	DefaultLLMBaseURL        = "http://localhost:11434/api/generate"
	DefaultLLMAPIKey         = ""
	DefaultLLMMaxTokens      = 1024
	DefaultLLMTemperature    = 0.1
	DefaultLLMTimeoutSeconds = 30
	ConfigDirName            = "config"
	ConfigFileName           = "config.json"
)
