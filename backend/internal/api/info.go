package api

import (
	"embodied-ai-proxy/backend/internal/prompt"
	"embodied-ai-proxy/backend/internal/websocket"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"net/http"
)

type infoResponse struct {
	Server           sharedconfig.ServerConfig `json:"server"`
	LLM              sharedconfig.LLMConfig    `json:"llm"`
	BridgeConnected  bool                      `json:"bridge_connected"`
	ClientsConnected int                       `json:"clients_connected"`
	SystemPrompt     string                    `json:"system_prompt"`
}

// InfoHandler exposes GET /api/info: a single snapshot combining the
// backend's own config, the LLM proxy's config, live hub stats, and the raw
// system prompt template. Both configs come from the one shared
// data/config/config.json, so no cross-service call to the LLM proxy is
// needed. Powers the TUI's System Info / LLM Info / Copy-System-Prompt
// features.
func InfoHandler(cfg *sharedconfig.AppConfig, hub *websocket.Hub, pipeline *prompt.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, bridgeConnected := hub.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infoResponse{
			Server:           cfg.Server,
			LLM:              cfg.Proxy.LLMConfig,
			BridgeConnected:  bridgeConnected,
			ClientsConnected: clients,
			SystemPrompt:     pipeline.SystemPrompt(),
		})
	}
}
