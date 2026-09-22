package gemini

import (
	"bytes"
	"context"
	"embodied-ai-proxy/llm-proxy/internal/provider"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Adapter is a provider.Provider backed by Google's Gemini generateContent API.
// API reference: https://ai.google.dev/api/generate-content
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

type part struct {
	Text string `json:"text"`
}

type content struct {
	Parts []part `json:"parts"`
}

type generateRequest struct {
	Contents          []content        `json:"contents"`
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

type generationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens"`
	Temperature     float64 `json:"temperature"`
}

type generateResponse struct {
	Candidates []struct {
		Content      content `json:"content"`
		FinishReason string  `json:"finishReason"`
	} `json:"candidates"`
}

func (a *Adapter) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	maxTokens, temperature := provider.ResolveParams(a.maxTokens, a.temperature, req)

	reqBody := generateRequest{
		Contents: []content{{Parts: []part{{Text: req.Prompt}}}},
		GenerationConfig: generationConfig{
			MaxOutputTokens: maxTokens,
			Temperature:     temperature,
		},
	}
	if req.SystemInstruction != "" {
		reqBody.SystemInstruction = &content{Parts: []part{{Text: req.SystemInstruction}}}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return provider.Response{}, fmt.Errorf("gemini: encode request: %w", err)
	}

	base := strings.TrimRight(a.baseURL, "/")
	reqURL := fmt.Sprintf("%s/%s:generateContent?key=%s", base, a.model, url.QueryEscape(a.apiKey))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return provider.Response{}, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := provider.Do(ctx, a.httpClient, httpReq, "gemini")
	if err != nil {
		return provider.Response{}, err
	}

	var parsed generateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return provider.Response{}, fmt.Errorf("gemini: decode response: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return provider.Response{}, fmt.Errorf("gemini: response contained no candidates")
	}

	return provider.Response{
		Text:         strings.TrimSpace(parsed.Candidates[0].Content.Parts[0].Text),
		FinishReason: parsed.Candidates[0].FinishReason,
	}, nil
}
