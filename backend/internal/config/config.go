package config

import (
	"embodied-ai-proxy/backend/internal/config/constants"
	shared "embodied-ai-proxy/shared/config"
)

type AppConfig struct {
	Port     int    `json:"port"`
	ProxyURL string `json:"proxy_url"`
}

func getDefaultAppConfig() AppConfig {
	return AppConfig{
		Port:     constants.DefaultServerPort,
		ProxyURL: constants.DefaultProxyURL,
	}
}

func Initialise(dir string) (*AppConfig, error) {
	return shared.Load(dir, constants.ConfigDirName, constants.ConfigFileName, getDefaultAppConfig())
}
