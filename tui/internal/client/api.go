package client

import (
	"context"
	sharedconfig "embodied-ai-proxy/shared/config"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SystemInfo matches the JSON response returned by the backend GET /api/info
type SystemInfo struct {
	Server           sharedconfig.ServerConfig `json:"server"`
	LLM              sharedconfig.LLMConfig    `json:"llm"`
	BridgeConnected  bool                      `json:"bridge_connected"`
	ClientsConnected int                       `json:"clients_connected"`
	SystemPrompt     string                    `json:"system_prompt"`
}

// Backend REST endpoints
const (
	infoPath  = "/api/info"
	resetPath = "/api/reset"
	scanPath  = "/api/scan"
)

// APIClient interacts with the backend's REST endpoints
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new REST client for the Go backend
func NewAPIClient(baseURL string) *APIClient {
	// No client-wide timeout: each call sets its own deadline, and a /scan takes a while
	return &APIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

// FetchInfo calls GET /api/info and decodes the response
func (c *APIClient) FetchInfo(ctx context.Context) (*SystemInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+infoPath, nil)
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
	var result ResetResult
	if err := c.post(ctx, resetPath, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ScanResult matches the JSON response returned by the backend POST /api/scan
type ScanResult struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	Added     []string `json:"added"`
	Removed   []string `json:"removed"`
	Unchanged int      `json:"unchanged"`
}

// Scan calls POST /api/scan, which takes a camera snapshot and updates the
// middleware's objects - powers the TUI's /scan command.
func (c *APIClient) Scan(ctx context.Context) (*ScanResult, error) {
	var result ScanResult
	if err := c.post(ctx, scanPath, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// post sends an empty POST to the backend and decodes the JSON reply into out
func (c *APIClient) post(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute %s request: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s request returned with status: %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}
