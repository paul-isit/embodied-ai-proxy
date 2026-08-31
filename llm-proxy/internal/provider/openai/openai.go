package openai

import (
	"bytes"
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Adapter is a provider.Provider backed by the OpenAI chat completions API.
// API reference: https://developers.openai.com/api/docs
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

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

func (a *Adapter) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	maxTokens, temperature := provider.ResolveParams(a.maxTokens, a.temperature, req)

	messages := []message{}
	if req.SystemInstruction != "" {
		messages = append(messages, message{Role: "system", Content: req.SystemInstruction})
	}
	messages = append(messages, message{Role: "user", Content: req.Prompt})

	body, err := json.Marshal(chatRequest{
		Model:       a.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	})
	if err != nil {
		return provider.Response{}, fmt.Errorf("openai: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	respBody, err := provider.Do(ctx, a.httpClient, httpReq, "openai")
	if err != nil {
		return provider.Response{}, err
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return provider.Response{}, fmt.Errorf("openai: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return provider.Response{}, fmt.Errorf("openai: response contained no choices")
	}

	return provider.Response{
		Text:         strings.TrimSpace(parsed.Choices[0].Message.Content),
		FinishReason: parsed.Choices[0].FinishReason,
	}, nil
}
