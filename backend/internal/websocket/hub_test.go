package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dial(t *testing.T, server *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + path
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	t.Cleanup(func() { ws.Close() })
	return ws
}

type recordingHandler struct {
	mu      sync.Mutex
	prompts []string
	done    chan struct{}
}

func (r *recordingHandler) HandlePrompt(ctx context.Context, prompt string) {
	r.mu.Lock()
	r.prompts = append(r.prompts, prompt)
	r.mu.Unlock()
	close(r.done)
}

func TestHub_ClientPromptSubmit_DispatchesToHandler(t *testing.T) {
	hub := NewHub()
	handler := &recordingHandler{done: make(chan struct{})}
	hub.SetPromptHandler(handler)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	ws := dial(t, server, "/ws/client")
	payload, _ := json.Marshal(map[string]string{"prompt": "pick up the cube"})
	if err := ws.WriteJSON(Envelope{Type: TypePromptSubmit, Payload: payload}); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case <-handler.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt handler to be invoked")
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.prompts) != 1 || handler.prompts[0] != "pick up the cube" {
		t.Errorf("prompts = %v", handler.prompts)
	}
}

func TestHub_BridgeObserver_BroadcastsToClient(t *testing.T) {
	hub := NewHub()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	clientWS := dial(t, server, "/ws/client")
	time.Sleep(50 * time.Millisecond)

	// Consume initial bridge_connected: false status
	var initial Envelope
	if err := clientWS.ReadJSON(&initial); err != nil {
		t.Fatalf("read initial status: %v", err)
	}

	// Trigger connection change
	hub.OnBridgeConnectionChange(true)

	clientWS.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got Envelope
	if err := clientWS.ReadJSON(&got); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if got.Type != TypeStatusUpdate {
		t.Errorf("got.Type = %q, want %q", got.Type, TypeStatusUpdate)
	}

	if !hub.BridgeConnected() {
		t.Error("expected BridgeConnected() to be true")
	}

	// Trigger object updates
	hub.OnObjectsUpdated([]string{"cup", "bottle"})

	if err := clientWS.ReadJSON(&got); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if !strings.Contains(string(got.Payload), "bottle") {
		t.Errorf("expected object_list in payload, got %s", string(got.Payload))
	}
}

func TestHub_ServeClient_RejectsSecondClient(t *testing.T) {
	hub := NewHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/client", hub.ServeClient)
	server := httptest.NewServer(mux)
	defer server.Close()

	first := dial(t, server, "/ws/client")
	defer first.Close()
	time.Sleep(50 * time.Millisecond)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/client"
	_, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		t.Fatal("expected a second client connection to be rejected")
	}
	if resp == nil || resp.StatusCode != http.StatusConflict {
		t.Errorf("response = %v, want status %d", resp, http.StatusConflict)
	}

	if clients, _ := hub.Stats(); clients != 1 {
		t.Errorf("clients = %d, want 1 (the first client must remain connected after the second was rejected)", clients)
	}
}
