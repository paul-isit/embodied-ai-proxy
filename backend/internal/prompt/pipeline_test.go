package prompt

import (
	"context"
	"embodied-ai-proxy/backend/internal/validator"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	path, err := filepath.Abs("../../../configs/json_schema.json")
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
// (e.g. skipping the hub's now-standard "current bridge state" status_update
// that every newly-connected client receives up front).
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

const testSystemPrompt = "Schema:\n{schema_template}\n\nObjects:\n{available_objects}\n\nCommand: {user_command}"

func TestPipeline_Run_ValidRecipe(t *testing.T) {
	llmProxy := fakeLLMProxy(t, `{"status":"success","recipe_name":"test","steps":[{"step_id":1,"action":"home","description":"go home","parameters":{}}]}`)
	defer llmProxy.Close()

	p := New(websocket.NewHub(), testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
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

	p := New(websocket.NewHub(), testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	result := p.Run(context.Background(), "go home", nil)

	if result.Error == "" {
		t.Fatal("expected schema validation error, got none")
	}
}

func TestPipeline_Run_StripsMarkdownFences(t *testing.T) {
	llmProxy := fakeLLMProxy(t, "```json\n"+`{"status":"error","error_type":"missing_object","message":"no cube"}`+"\n```")
	defer llmProxy.Close()

	p := New(websocket.NewHub(), testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	result := p.Run(context.Background(), "pick up cube", nil)

	if result.Error != "" {
		t.Fatalf("Run() error = %q", result.Error)
	}
}

func TestPipeline_HandlePrompt_BroadcastsActionRecipeAndDispatchesToBridge(t *testing.T) {
	llmProxy := fakeLLMProxy(t, `{"status":"success","recipe_name":"test","steps":[{"step_id":1,"action":"home","description":"go home","parameters":{}}]}`)
	defer llmProxy.Close()

	hub := websocket.NewHub()
	p := New(hub, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	mux.HandleFunc("/ws/bridge", hub.ServeBridge)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	bridgeWS := dialTestWS(t, server.URL, "/ws/bridge")
	time.Sleep(50 * time.Millisecond)

	p.HandlePrompt("go home")

	bridgeMsg := readUntil(t, bridgeWS, websocket.TypeActionRecipe, 2*time.Second)
	if bridgeMsg.Type != websocket.TypeActionRecipe {
		t.Errorf("bridge got type %q, want %q", bridgeMsg.Type, websocket.TypeActionRecipe)
	}

	// The client also receives one or two status_update envelopes first (its
	// own initial bridge state, then the bridge's connect broadcast) before
	// the action_recipe - skip past those.
	clientMsg := readUntil(t, clientWS, websocket.TypeActionRecipe, 2*time.Second)
	if clientMsg.Type != websocket.TypeActionRecipe {
		t.Errorf("client got type %q, want %q", clientMsg.Type, websocket.TypeActionRecipe)
	}
}

func TestPipeline_HandleBridgeStatus_CachesObjectList(t *testing.T) {
	p := New(websocket.NewHub(), testValidator(t), "http://unused", testSystemPrompt, []byte(`{}`))

	payload, _ := json.Marshal(map[string]any{"object_list": []string{"red_cube", "blue_cube"}})
	p.HandleBridgeStatus(websocket.Envelope{Type: websocket.TypeStatusUpdate, Payload: payload})

	got := p.availableObjects()
	if len(got) != 2 || got[0] != "red_cube" || got[1] != "blue_cube" {
		t.Errorf("availableObjects() = %v", got)
	}
}

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
	p := New(hub, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	p.HandlePrompt("go home")

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

// TestHub_ServeClient_ProcessesPromptSubmitsSequentially covers the
// serialization guarantee that used to be enforced by a "busy" flag inside
// Pipeline itself: since the hub only ever has one connected client and
// calls HandlePrompt synchronously from that client's read loop (see
// websocket.Hub.ServeClient), a second prompt_submit sent before the first
// finishes must sit unprocessed on the socket rather than run concurrently.
func TestHub_ServeClient_ProcessesPromptSubmitsSequentially(t *testing.T) {
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
	p := New(hub, testValidator(t), llmProxy.URL, testSystemPrompt, []byte(`{}`))
	hub.SetPromptHandler(p)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	mux.HandleFunc("/ws/bridge", hub.ServeBridge)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dialTestWS(t, server.URL, "/ws/client")
	bridgeWS := dialTestWS(t, server.URL, "/ws/bridge")
	time.Sleep(50 * time.Millisecond)

	sendPrompt := func(text string) {
		payload, _ := json.Marshal(map[string]string{"prompt": text})
		if err := clientWS.WriteJSON(websocket.Envelope{Type: websocket.TypePromptSubmit, Payload: payload}); err != nil {
			t.Fatalf("write prompt: %v", err)
		}
	}

	sendPrompt("first")
	<-started // the client's read loop is now blocked inside HandlePrompt, mid-LLM-call

	sendPrompt("second") // must sit unread on the socket until "first" finishes

	time.Sleep(200 * time.Millisecond)
	if n := callCount.Load(); n != 1 {
		t.Fatalf("callCount = %d before unblocking, want 1 (prompt_submit must not be processed concurrently)", n)
	}

	close(unblock)

	// Both prompts still complete, one after the other, once unblocked.
	readUntil(t, bridgeWS, websocket.TypeActionRecipe, 2*time.Second)
	readUntil(t, bridgeWS, websocket.TypeActionRecipe, 2*time.Second)
}
