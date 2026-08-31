package config

const (
	// ConfigDirName is the single directory, under a service's dataDir,
	// where every config file lives - config.json, and (for the backend)
	// json_schema.json and system_prompt.md too - so there's exactly one
	// place to look, not one per file.
	ConfigDirName  = "config"
	configFileName = "config.json"
)

// ServerConfig holds settings for the Go API backend.
type ServerConfig struct {
	Port         int    `json:"port"`
	ProxyURL     string `json:"proxy_url"`
	RosbridgeURL string `json:"rosbridge_url"`
}

// LLMConfig holds settings for the upstream LLM provider the proxy talks to.
type LLMConfig struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	BaseURL        string  `json:"base_url"`
	APIKey         string  `json:"api_key"` // TODO: support environment-variable override before any shared/production deployment (see design.md Risks/Trade-offs)
	MaxTokens      int     `json:"max_tokens"`
	Temperature    float64 `json:"temperature"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

// ProxyConfig holds settings for the Go LLM proxy.
type ProxyConfig struct {
	Port      int       `json:"port"`
	LLMConfig LLMConfig `json:"llm_config"`
}

// AppConfig is the single configuration document at data/config/config.json,
// shared by the backend (Server) and the LLM proxy (Proxy) so operators only
// ever edit one file.
type AppConfig struct {
	Server ServerConfig `json:"server"`
	Proxy  ProxyConfig  `json:"proxy"`
}

func defaultAppConfig() AppConfig {
	return AppConfig{
		Server: ServerConfig{
			Port:         DefaultServerPort,
			ProxyURL:     DefaultProxyURL,
			RosbridgeURL: DefaultRosbridgeURL,
		},
		Proxy: ProxyConfig{
			Port: DefaultProxyPort,
			LLMConfig: LLMConfig{
				Provider:       DefaultLLMProvider,
				Model:          DefaultLLMModel,
				BaseURL:        DefaultLLMBaseURL,
				APIKey:         DefaultLLMAPIKey,
				MaxTokens:      DefaultLLMMaxTokens,
				Temperature:    DefaultLLMTemperature,
				TimeoutSeconds: DefaultLLMTimeoutSeconds,
			},
		},
	}
}

// Initialise loads the shared config.json from dataDir/config, falling back
// to defaults for any part of the file (or the whole file) that's missing.
func Initialise(dataDir string) (*AppConfig, error) {
	return Load(dataDir, ConfigDirName, configFileName, defaultAppConfig())
}
