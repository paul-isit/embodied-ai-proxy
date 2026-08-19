package api

import (
	"embodied-ai-proxy/backend/internal/websocket"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInfoHandler_ReportsServerProxyAndHubState(t *testing.T) {
	pipeline := testPipeline(t, `{}`)
	hub := websocket.NewHub()

	cfg := &sharedconfig.AppConfig{
		Server: sharedconfig.ServerConfig{Port: 8080, ProxyURL: "http://localhost:8081"},
		Proxy: sharedconfig.ProxyConfig{
			Port: 8081,
			LLMConfig: sharedconfig.LLMConfig{
				Provider: "ollama",
				Model:    "gemma3:1b",
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	w := httptest.NewRecorder()
	InfoHandler(cfg, hub, pipeline)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var got infoResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Server.Port != 8080 || got.LLM.Provider != "ollama" || got.LLM.Model != "gemma3:1b" {
		t.Errorf("unexpected response: %+v", got)
	}
	if got.BridgeConnected {
		t.Error("expected bridge_connected=false with no bridge dialed")
	}
	if got.SystemPrompt == "" {
		t.Error("expected system_prompt to be populated")
	}
}
