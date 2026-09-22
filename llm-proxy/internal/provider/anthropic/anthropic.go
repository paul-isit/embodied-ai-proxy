package anthropic

import (
	"bytes"
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const apiVersion = "2023-06-01"

// Adapter is a provider.Provider backed by Anthropic's Messages API.
// API reference: https://platform.claude.com/docs/en/api/messages/create
type Adapter struct {
	model       string
	baseURL     string
	apiKey      string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

func New(model, baseURL, apiKey string, maxTokens int, temperature float64, httpClient *http.Client) *Adapter {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Adapter{
		model:       model,
		baseURL:     baseURL,
		apiKey:      apiKey,
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  httpClient,
	}
}

var _ provider.Provider = (*Adapter)(nil)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	System      string    `json:"system,omitempty"`
	Messages    []message `json:"messages"`
}

type messagesResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

func (a *Adapter) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	maxTokens, temperature := provider.ResolveParams(a.maxTokens, a.temperature, req)

	body, err := json.Marshal(messagesRequest{
		Model:       a.model,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		System:      req.SystemInstruction,
		Messages:    []message{{Role: "user", Content: req.Prompt}},
	})
	if err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	respBody, err := provider.Do(ctx, a.httpClient, httpReq, "anthropic")
	if err != nil {
		return provider.Response{}, err
	}

	var parsed messagesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return provider.Response{}, fmt.Errorf("anthropic: decode response: %w", err)
	}
	if len(parsed.Content) == 0 {
		return provider.Response{}, fmt.Errorf("anthropic: response contained no content blocks")
	}

	return provider.Response{
		Text:         strings.TrimSpace(parsed.Content[0].Text),
		FinishReason: parsed.StopReason,
	}, nil
}
