package client

import (
	"context"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SystemInfo matches the JSON response returned by the backend GET /api/info
type SystemInfo struct {
	Server           sharedconfig.ServerConfig `json:"server"`
	LLM              sharedconfig.LLMConfig    `json:"llm"`
	BridgeConnected  bool                      `json:"bridge_connected"`
	ClientsConnected int                       `json:"clients_connected"`
	SystemPrompt     string                    `json:"system_prompt"`
}

// APIClient interacts with the backend's REST endpoints
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new REST client for the Go backend
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// FetchInfo calls GET /api/info and decodes the response
func (c *APIClient) FetchInfo(ctx context.Context) (*SystemInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/info", nil)
	if err != nil {
		return nil, fmt.Errorf("build info request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("info request returned with status: %d", resp.StatusCode)
	}

	var info SystemInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode info response: %w", err)
	}

	return &info, nil
}

// ResetResult matches the JSON response returned by the backend POST /api/reset
type ResetResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ResetEnvironment calls POST /api/reset, which triggers the middleware's
// /reset_environment service (reload objects/obstacles, clear held-object
// tracking) - powers the TUI's /reset-env command.
func (c *APIClient) ResetEnvironment(ctx context.Context) (*ResetResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/reset", nil)
	if err != nil {
		return nil, fmt.Errorf("build reset request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute reset request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reset request returned with status: %d", resp.StatusCode)
	}

	var result ResetResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode reset response: %w", err)
	}

	return &result, nil
}
