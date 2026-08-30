package pipeline

import (
	"bytes"
	"context"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Pipeline builds prompts from the system prompt template + schema +
// available objects, dispatches them to the LLM proxy, and validates the
// result against the action-recipe schema. Its Run method also
// backs the synchronous HTTP endpoint.
type Pipeline struct {
	hub          *websocket.Hub
	validator    *validator.Validator
	proxyURL     string
	systemPrompt string
	schemaBlock  string
	httpClient   *http.Client

	mu      sync.RWMutex
	objects []string
}

// New creates a new Pipeline coordinator.
func New(hub *websocket.Hub, v *validator.Validator, proxyURL, systemPrompt string, schemaRaw []byte) *Pipeline {
	var formattedJsonSchema bytes.Buffer
	if err := json.Indent(&formattedJsonSchema, schemaRaw, "", "  "); err != nil {
		formattedJsonSchema.Write(schemaRaw)
	}
	return &Pipeline{
		hub:          hub,
		validator:    v,
		proxyURL:     strings.TrimRight(proxyURL, "/"),
		systemPrompt: systemPrompt,
		schemaBlock:  formattedJsonSchema.String(),
		httpClient:   &http.Client{Timeout: 60 * time.Second},
	}
}

// SystemPrompt returns the raw system prompt template
func (p *Pipeline) SystemPrompt() string {
	return p.systemPrompt
}

type generateRequestPayload struct {
	Prompt string `json:"prompt"`
}

type generateResponsePayload struct {
	Text  string `json:"text"`
	Error string `json:"error"`
}

func (p *Pipeline) callLLMProxy(ctx context.Context, fullPrompt string) (string, error) {
	body, err := json.Marshal(generateRequestPayload{Prompt: fullPrompt})
	if err != nil {
		return "", fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.proxyURL+"/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed generateResponsePayload
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode llm proxy response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm proxy error: %s", parsed.Error)
	}
	return parsed.Text, nil
}

// Result is the outcome of running a prompt through the full pipeline.
type Result struct {
	RawOutput string          `json:"raw_output,omitempty"`
	Parsed    json.RawMessage `json:"parsed,omitempty"`
	Error     string          `json:"error,omitempty"`

	// Doc is the schema-validated document, already decoded - callers that
	// only need e.g. the "status" field should use this instead of
	// re-unmarshaling Parsed. Not serialized.
	Doc any `json:"-"`
}

// Run builds the prompt, dispatches it to the LLM proxy, and validates the
// result. It does not touch the WebSocket hub - callers decide what to do
// with the outcome.
func (p *Pipeline) Run(ctx context.Context, userText string, objects []string) Result {
	fullPrompt := p.buildPrompt(userText, objects)
	log.Printf("[Pipeline] command received: %q", userText)

	rawOutput, err := p.callLLMProxy(ctx, fullPrompt)
	if err != nil {
		log.Printf("[Pipeline] command %q: LLM proxy request failed: %v", userText, err)
		return Result{Error: err.Error()}
	}

	candidate := extractJSON(rawOutput)
	doc, err := p.validator.Validate([]byte(candidate))
	if err != nil {
		log.Printf("[Pipeline] command %q: schema validation failed: %v; raw LLM output: %s", userText, err, rawOutput)
		return Result{RawOutput: rawOutput, Error: fmt.Sprintf("schema validation failed: %v", err)}
	}

	log.Printf("[Pipeline] command %q: produced valid recipe: %s", userText, candidate)
	return Result{RawOutput: rawOutput, Parsed: json.RawMessage(candidate), Doc: doc}
}

func recipeStatus(doc any) string {
	obj, ok := doc.(map[string]any)
	if !ok {
		return ""
	}
	status, _ := obj["status"].(string)
	return status
}
