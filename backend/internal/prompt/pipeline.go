package prompt

import (
	"bytes"
	"context"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	placeholderSchema  = "{schema_template}"
	placeholderObjects = "{available_objects}"
	placeholderCommand = "{user_command}"
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

func New(hub *websocket.Hub, v *validator.Validator, proxyURL, systemPrompt string, schemaRaw []byte) *Pipeline {
	var formattedJsonSchema bytes.Buffer
	if err := json.Indent(&formattedJsonSchema, schemaRaw, "", "  "); err != nil {
		formattedJsonSchema.Write(schemaRaw) // TODO: improve error handling
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

// SystemPrompt returns the raw system prompt template, e.g. for a TUI
// "copy system prompt" action.
func (p *Pipeline) SystemPrompt() string {
	return p.systemPrompt
}

// SetAvailableObjects updates the cached workspace object list, typically
// from a status_update pushed by the ROS 2 bridge (see HandleBridgeStatus).
func (p *Pipeline) SetAvailableObjects(objects []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.objects = objects
}

func (p *Pipeline) availableObjects() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.objects
}

// HandleBridgeStatus implements websocket.StatusHandler: it inspects a
// status_update from the bridge for an object_list field and caches it.
func (p *Pipeline) HandleBridgeStatus(env websocket.Envelope) {
	var payload struct {
		ObjectList []string `json:"object_list"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	if payload.ObjectList != nil {
		p.SetAvailableObjects(payload.ObjectList)
	}
}

func (p *Pipeline) buildPrompt(userText string, objects []string) string {
	objectsStr := "No objects currently mapped."
	if len(objects) > 0 {
		lines := make([]string, len(objects))
		for i, obj := range objects {
			lines[i] = "- " + obj
		}
		objectsStr = strings.Join(lines, "\n")
	}

	result := p.systemPrompt
	result = strings.ReplaceAll(result, placeholderSchema, p.schemaBlock)
	result = strings.ReplaceAll(result, placeholderObjects, objectsStr)
	result = strings.ReplaceAll(result, placeholderCommand, userText)
	return result
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

// jsonObjectPattern is the same last-resort recovery the original Python
// validate_and_extract_json used: if the response isn't valid JSON even
// after stripping markdown fences, grab the first "{...}" block out of
// whatever conversational filler the LLM wrapped it in.
var jsonObjectPattern = regexp.MustCompile(`(?s)\{.*\}`)

// extractJSON strips optional markdown code fences from raw LLM output and,
// if the result still isn't valid JSON, falls back to a regex scan for a
// JSON object embedded in surrounding text - mirroring the original Python
// validate_and_extract_json behaviour in full.
func extractJSON(raw string) string {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	if json.Valid([]byte(text)) {
		return text
	}
	if match := jsonObjectPattern.FindString(text); match != "" {
		return match
	}
	return text
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

// HandlePrompt implements websocket.PromptHandler: it runs the pipeline
// using the cached workspace object list and sends the outcome to the
// connected client.
func (p *Pipeline) HandlePrompt(userText string) {
	if !p.hub.BridgeConnected() {
		log.Printf("[Pipeline] rejecting command %q: no ROS 2 bridge connected", userText)
		p.sendLog("error", "cannot execute commands: no ROS 2 bridge connected")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := p.Run(ctx, userText, p.availableObjects())
	if result.Error != "" {
		p.sendLog("error", result.Error)
		return
	}

	if recipeStatus(result.Doc) == "success" {
		if err := p.hub.SendToBridge(websocket.Envelope{Type: websocket.TypeActionRecipe, Payload: result.Parsed}); err != nil {
			log.Printf("[Pipeline] command %q: failed to dispatch action recipe to bridge: %v", userText, err)
			p.sendLog("error", fmt.Sprintf("failed to dispatch action recipe to the ROS 2 bridge: %v", err))
			return
		}
		p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeActionRecipe, Payload: result.Parsed})
	} else {
		p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeLogEvent, Payload: result.Parsed})
	}
}

func (p *Pipeline) sendLog(level, message string) {
	payload, err := json.Marshal(map[string]string{"level": level, "message": message})
	if err != nil {
		return
	}
	p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeLogEvent, Payload: payload})
}
