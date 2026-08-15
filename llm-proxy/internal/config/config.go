package config

import (
	"embodied-ai-proxy/llm-proxy/internal/config/constants"
	shared "embodied-ai-proxy/shared/config"
)

type LLMConfig struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	BaseURL        string  `json:"base_url"`
	APIKey         string  `json:"api_key"` // TODO: support environment-variable override before any shared/production deployment (see design.md Risks/Trade-offs)
	MaxTokens      int     `json:"max_tokens"`
	Temperature    float64 `json:"temperature"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

type AppConfig struct {
	Port      int       `json:"port"`
	LLMConfig LLMConfig `json:"llm_config"`
}

func getDefaultAppConfig() AppConfig {
	return AppConfig{
		Port: constants.DefaultServerPort,
		LLMConfig: LLMConfig{
			Provider:       constants.DefaultLLMProvider,
			Model:          constants.DefaultLLMModel,
			BaseURL:        constants.DefaultLLMBaseURL,
			APIKey:         constants.DefaultLLMAPIKey,
			MaxTokens:      constants.DefaultLLMMaxTokens,
			Temperature:    constants.DefaultLLMTemperature,
			TimeoutSeconds: constants.DefaultLLMTimeoutSeconds,
		},
	}
}

func Initialise(dir string) (*AppConfig, error) {
	return shared.Load(dir, constants.ConfigDirName, constants.ConfigFileName, getDefaultAppConfig())
}
