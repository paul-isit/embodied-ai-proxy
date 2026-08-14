package config

import (
	"embodied-ai-proxy/backend/internal/config/constants"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Port      int       `json:"port"`
	LLMConfig LLMConfig `json:"llm_config"`
}

func getDefaultAppConfig() AppConfig {
	appConfig := AppConfig{
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
	return appConfig
}

func Initialise(dir string) (*AppConfig, error) {
	configFilePath := filepath.Join(dir, constants.ConfigDirName, constants.ConfigFileName)
	info, err := os.Stat(configFilePath)
	var appConfig AppConfig
	if os.IsNotExist(err) || info.IsDir() {
		defaultAppConfig := getDefaultAppConfig() // TODO: create the config file in the correct dir instead of just falling back to the default
		// TODO add log line here
		return &defaultAppConfig, nil
	}

	file, err := os.Open(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %s: %w", configFilePath, err)
	}

	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&appConfig); err != nil {
		return nil, fmt.Errorf("failed to decode config JSON file %s: %w", configFilePath, err)
	}

	return &appConfig, nil
}
