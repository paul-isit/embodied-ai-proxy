package ollama

import (
	"bytes"
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Adapter is a provider.Provider backed by a local Ollama instance.
// API reference: https://github.com/ollama/ollama/blob/main/docs/api.md
type Adapter struct {
	model       string
	baseURL     string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

func New(model, baseURL string, maxTokens int, temperature float64, httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Adapter{
		model:       model,
		baseURL:     baseURL,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  httpClient,
	}
}

var _ provider.Provider = (*Adapter)(nil)

type generateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	System  string          `json:"system,omitempty"`
	Stream  bool            `json:"stream"`
	Format  string          `json:"format,omitempty"`
	Options generateOptions `json:"options"`
}

type generateOptions struct {
	Temperature float64 `json:"temperature"`
	NumPredict  int     `json:"num_predict"`
}

type generateResponse struct {
	Response   string `json:"response"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason"`
}

// Generate implements provider.Provider. Per-request MaxTokens/Temperature
// override the adapter's configured defaults when non-zero.
func (a *Adapter) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	maxTokens, temperature := provider.ResolveParams(a.maxTokens, a.temperature, req)

	body, err := json.Marshal(generateRequest{
		Model:  a.model,
		Prompt: req.Prompt,
		System: req.SystemInstruction,
		Stream: false,
		Format: "json",
		Options: generateOptions{
			Temperature: temperature,
			NumPredict:  maxTokens,
		},
	})
	if err != nil {
		return provider.Response{}, fmt.Errorf("ollama: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := provider.Do(ctx, a.httpClient, httpReq, "ollama")
	if err != nil {
		return provider.Response{}, err
	}

	var parsed generateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return provider.Response{}, fmt.Errorf("ollama: decode response: %w", err)
	}

	return provider.Response{
		Text:         strings.TrimSpace(parsed.Response),
		FinishReason: parsed.DoneReason,
	}, nil
}
