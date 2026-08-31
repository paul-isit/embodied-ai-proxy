package pipeline

import (
	"context"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

func dialTestWS(t *testing.T, httpURL, path string) *gorillaws.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(httpURL, "http") + path
	ws, _, err := gorillaws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

func testValidator(t *testing.T) *validator.Validator {
	t.Helper()
	path, err := filepath.Abs("../../../data/config/json_schema.json")
	if err != nil {
		t.Fatalf("resolve schema path: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema file: %v", err)
	}
	v, err := validator.New(path, raw)
	if err != nil {
		t.Fatalf("validator.New() error = %v", err)
	}
	return v
}

// readUntil reads and discards envelopes until one of type wantType arrives
func readUntil(t *testing.T, ws *gorillaws.Conn, wantType string, timeout time.Duration) websocket.Envelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ws.SetReadDeadline(deadline)
		var env websocket.Envelope
		if err := ws.ReadJSON(&env); err != nil {
			t.Fatalf("read (want %s): %v", wantType, err)
		}
		if env.Type == wantType {
			return env
		}
	}
}

func fakeLLMProxy(t *testing.T, responseText string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(generateResponsePayload{Text: responseText})
	}))
}

type mockROSBridge struct {
	mu        sync.Mutex
	connected bool
	objects   []string
	executed  [][]byte
}

func (m *mockROSBridge) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockROSBridge) GetAvailableObjects() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.objects
}

func (m *mockROSBridge) ExecuteRecipe(ctx context.Context, recipeJSON []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = append(m.executed, recipeJSON)
	return nil
}

const testSystemPrompt = "Schema:\n{schema_template}\n\nObjects:\n{available_objects}\n\nCommand: {user_command}"

func TestPipeline_Run_ValidRecipe(t *testing.T) {
	llmProxy := fakeLLMProxy(t, `{"status":"success","recipe_name":"test","steps":[{"step_id":1,"action":"home","description":"go home","parameters":{}}]}`)
	defer llmProxy.Close()

	p := New(websocket.NewHub(), &mockROSBridge{connected: true}, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	result := p.Run(context.Background(), "go home", []string{"red_cube"})

	if result.Error != "" {
		t.Fatalf("Run() error = %q", result.Error)
	}
	if result.Parsed == nil {
		t.Fatal("expected Parsed to be set")
	}
}

func TestPipeline_Run_InvalidRecipeFailsSchemaValidation(t *testing.T) {
	llmProxy := fakeLLMProxy(t, `{"status":"success","steps":[]}`) // missing recipe_name
	defer llmProxy.Close()

	p := New(websocket.NewHub(), &mockROSBridge{connected: true}, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	result := p.Run(context.Background(), "go home", nil)

	if result.Error == "" {
		t.Fatal("expected schema validation error, got none")
	}
}

func TestPipeline_Run_StripsMarkdownFences(t *testing.T) {
	llmProxy := fakeLLMProxy(t, "```json\n"+`{"status":"error","error_type":"missing_object","message":"no cube"}`+"\n```")
	defer llmProxy.Close()

	p := New(websocket.NewHub(), &mockROSBridge{connected: true}, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	result := p.Run(context.Background(), "pick up cube", nil)

	if result.Error != "" {
		t.Fatalf("Run() error = %q", result.Error)
	}
}

func TestPipeline_HandlePrompt_BroadcastsActionRecipeAndExecutesOnBridge(t *testing.T) {
	llmProxy := fakeLLMProxy(t, `{"status":"success","recipe_name":"test","steps":[{"step_id":1,"action":"home","description":"go home","parameters":{}}]}`)
	defer llmProxy.Close()

	hub := websocket.NewHub()
	bridge := &mockROSBridge{
		connected: true,
		objects:   []string{"red_cube"},
	}
	p := New(hub, bridge, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	p.HandlePrompt(context.Background(), "go home")

	clientMsg := readUntil(t, clientWS, websocket.TypeActionRecipe, 2*time.Second)
	if clientMsg.Type != websocket.TypeActionRecipe {
		t.Errorf("client got type %q, want %q", clientMsg.Type, websocket.TypeActionRecipe)
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if len(bridge.executed) != 1 {
		t.Fatalf("bridge executed %d recipes, want 1", len(bridge.executed))
	}
}

func TestPipeline_HandlePrompt_ExecutionFailure_BroadcastsErrorWithoutActionRecipe(t *testing.T) {
	llmProxy := fakeLLMProxy(t, `{"status":"success","recipe_name":"test","steps":[{"step_id":1,"action":"home","description":"go home","parameters":{}}]}`)
	defer llmProxy.Close()

	hub := websocket.NewHub()
	failBridge := &failingROSBridge{connected: true}
	p := New(hub, failBridge, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	p.HandlePrompt(context.Background(), "go home")

	msg := readUntil(t, clientWS, websocket.TypeLogEvent, 2*time.Second)
	if !strings.Contains(string(msg.Payload), "robot recipe execution failed") {
		t.Errorf("expected robot recipe execution failed log, got: %s", string(msg.Payload))
	}

	// Assert that no TypeActionRecipe is sent after execution failure
	clientWS.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var trailing websocket.Envelope
	for {
		if err := clientWS.ReadJSON(&trailing); err != nil {
			break // Read timed out as expected with no further messages
		}
		if trailing.Type == websocket.TypeActionRecipe {
			t.Fatalf("unexpected %s envelope received after execution failure: %s", websocket.TypeActionRecipe, string(trailing.Payload))
		}
	}
}

type failingROSBridge struct {
	connected bool
}

func (f *failingROSBridge) IsConnected() bool                                     { return f.connected }
func (f *failingROSBridge) GetAvailableObjects() []string                         { return nil }
func (f *failingROSBridge) ExecuteRecipe(ctx context.Context, recipe []byte) error { return errors.New("gripper jammed") }

func TestExtractJSON_RecoversFromConversationalFiller(t *testing.T) {
	raw := "Sure! Here's the recipe you asked for:\n" +
		`{"status":"success","recipe_name":"test","steps":[]}` +
		"\nLet me know if you need anything else."

	got := extractJSON(raw)
	if !json.Valid([]byte(got)) {
		t.Fatalf("extractJSON() = %q, not valid JSON", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("unmarshal extracted JSON: %v", err)
	}
	if parsed["status"] != "success" {
		t.Errorf("parsed = %v, want status=success", parsed)
	}
}

func TestPipeline_HandlePrompt_NoBridgeConnected_BroadcastsErrorWithoutCallingLLM(t *testing.T) {
	llmCalled := false
	llmProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalled = true
		json.NewEncoder(w).Encode(generateResponsePayload{Text: `{"status":"error","error_type":"invalid_command","message":"should not be reached"}`})
	}))
	defer llmProxy.Close()

	hub := websocket.NewHub()
	bridge := &mockROSBridge{connected: false}
	p := New(hub, bridge, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	p.HandlePrompt(context.Background(), "go home")

	msg := readUntil(t, clientWS, websocket.TypeLogEvent, 2*time.Second)
	var payload struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Level != "error" || !strings.Contains(payload.Message, "no ROS 2 bridge connected") {
		t.Errorf("payload = %+v, want a no-bridge-connected error", payload)
	}
	if llmCalled {
		t.Error("expected the LLM proxy not to be called when no bridge is connected")
	}
}

func TestHub_ServeClient_RejectsConcurrentPromptSubmits(t *testing.T) {
	var callCount atomic.Int32
	started := make(chan struct{}, 1)
	unblock := make(chan struct{})
	llmProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callCount.Add(1) == 1 {
			started <- struct{}{}
			<-unblock
		}
		json.NewEncoder(w).Encode(generateResponsePayload{Text: `{"status":"success","recipe_name":"test","steps":[{"step_id":1,"action":"home","description":"go home","parameters":{}}]}`})
	}))
	defer llmProxy.Close()

	hub := websocket.NewHub()
	bridge := &mockROSBridge{connected: true}
	p := New(hub, bridge, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	hub.SetPromptHandler(p)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	sendPrompt := func(text string) {
		payload, _ := json.Marshal(map[string]string{"prompt": text})
		if err := clientWS.WriteJSON(websocket.Envelope{Type: websocket.TypePromptSubmit, Payload: payload}); err != nil {
			t.Fatalf("write prompt: %v", err)
		}
	}

	sendPrompt("first")
	<-started // first prompt is executing inside LLM call

	sendPrompt("second") // concurrent prompt while busy

	// Second prompt should receive an immediate log_event rejection without blocking read loop
	rejectionMsg := readUntil(t, clientWS, websocket.TypeLogEvent, 2*time.Second)
	if !strings.Contains(string(rejectionMsg.Payload), "currently in progress") {
		t.Errorf("expected currently in progress rejection, got: %s", string(rejectionMsg.Payload))
	}

	if n := callCount.Load(); n != 1 {
		t.Fatalf("callCount = %d, want 1", n)
	}

	close(unblock)

	// First prompt completes and delivers action recipe to client
	readUntil(t, clientWS, websocket.TypeActionRecipe, 2*time.Second)
}

type cancelTrackingHandler struct {
	started  chan struct{}
	canceled chan struct{}
}

func (c *cancelTrackingHandler) HandlePrompt(ctx context.Context, prompt string) {
	close(c.started)
	<-ctx.Done()
	close(c.canceled)
}

func TestHub_ServeClient_DisconnectCancelsInFlightPrompt(t *testing.T) {
	handler := &cancelTrackingHandler{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}

	hub := websocket.NewHub()
	hub.SetPromptHandler(handler)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	payload, _ := json.Marshal(map[string]string{"prompt": "long task"})
	_ = clientWS.WriteJSON(websocket.Envelope{Type: websocket.TypePromptSubmit, Payload: payload})

	// Wait until HandlePrompt is actually running
	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt handler to start")
	}

	// Close client connection while prompt is in-flight
	clientWS.Close()

	select {
	case <-handler.canceled:
		// Success: context was canceled when client disconnected
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight prompt context to be canceled on disconnect")
	}
}
