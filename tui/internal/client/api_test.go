package client

import (
	"context"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchInfoSuccess(t *testing.T) {
	mockInfo := SystemInfo{
		Server: sharedconfig.ServerConfig{
			Port:     8080,
			ProxyURL: "http://localhost:8081",
		},
		LLM: sharedconfig.LLMConfig{
			Provider: "ollama",
			Model:    "gemma3:1b",
		},
		BridgeConnected:  true,
		ClientsConnected: 1,
		SystemPrompt:     "You are a robot assistant.",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/info" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockInfo)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	info, err := client.FetchInfo(context.Background())
	if err != nil {
		t.Fatalf("FetchInfo failed: %v", err)
	}

	if info.LLM.Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got '%s'", info.LLM.Provider)
	}
	if info.LLM.Model != "gemma3:1b" {
		t.Errorf("expected model 'gemma3:1b', got '%s'", info.LLM.Model)
	}
	if !info.BridgeConnected {
		t.Errorf("expected BridgeConnected to be true")
	}
}

func TestResetEnvironmentSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/reset" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ResetResult{
			Success: true,
			Message: "Environment reset: 4 object(s), 2 obstacle(s) restored to configured defaults",
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	result, err := client.ResetEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ResetEnvironment failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected Success to be true")
	}
	if result.Message == "" {
		t.Errorf("expected a non-empty message")
	}
}
