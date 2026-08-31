package api

import (
	"embodied-ai-proxy/backend/internal/pipeline"
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
func InfoHandler(cfg *sharedconfig.AppConfig, hub *websocket.Hub, p *pipeline.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, bridgeConnected := hub.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infoResponse{
			Server:           cfg.Server,
			LLM:              cfg.Proxy.LLMConfig,
			BridgeConnected:  bridgeConnected,
			ClientsConnected: clients,
			SystemPrompt:     p.SystemPrompt(),
		})
	}
}

type promptRequestPayload struct {
	Prompt           string   `json:"prompt"`
	AvailableObjects []string `json:"available_objects"`
}

// PromptHandler exposes the prompt pipeline over HTTP as POST /api/prompt,
// for batch evaluation (evaluate_proxy.py) and other HTTP-based queries that
// don't need a persistent WebSocket connection. Response shape mirrors the
// original Python LLMProxy.generate() return value: {raw_output, parsed}.
func PromptHandler(p *pipeline.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(pipeline.Result{Error: "method not allowed"})
			return
		}

		var payload promptRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(pipeline.Result{Error: "invalid request body: " + err.Error()})
			return
		}
		if payload.Prompt == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(pipeline.Result{Error: "prompt is required"})
			return
		}

		result := p.Run(r.Context(), payload.Prompt, payload.AvailableObjects)
		if result.Error != "" && result.RawOutput == "" {
			w.WriteHeader(http.StatusBadGateway) // transport/upstream failure, not a validation failure
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(result)
	}
}
