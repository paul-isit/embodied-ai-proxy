package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	defaultServerPort        = "8080"
	defaultLLMProvider       = "ollama"
	defaultLLMModel          = "gemma3:1b"
	defaultLLMBaseURL        = "http://localhost:11434/api/generate"
	defaultLLMAPIKey         = ""
	defaultLLMMaxTokens      = 1024
	defaultLLMTemperature    = 0.1
	defaultLLMTimeoutSeconds = 30
)

type LLMConfig struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	BaseURL        string  `json:"base_url"`
	APIKey         string  `json:"api_key"`
	MaxTokens      int     `json:"max_tokens"`
	Temperature    float64 `json:"temperature"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

type AppConfig struct {
	Port      string    `json:"port"`
	LLMConfig LLMConfig `json:"llm_config"`
}

func LoadConfig(configPath string) (*AppConfig, error) {
	appConfig := &AppConfig{
		Port: defaultServerPort,
		LLMConfig: LLMConfig{
			Provider:       defaultLLMProvider,
			Model:          defaultLLMModel,
			BaseURL:        defaultLLMBaseURL,
			APIKey:         defaultLLMAPIKey,
			MaxTokens:      defaultLLMMaxTokens,
			Temperature:    defaultLLMTemperature,
			TimeoutSeconds: defaultLLMTimeoutSeconds,
		},
	}

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// fallback to default values if config file is missing
			return appConfig, nil
		}
		return nil, fmt.Errorf("failed to open config file %s: %w", configPath, err)
	}

	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(appConfig); err != nil {
		return nil, fmt.Errorf("failed to decode config JSON file %s: %w", configPath, err)
	}

	return appConfig, nil
}
