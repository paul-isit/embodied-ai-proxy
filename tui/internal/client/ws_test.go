package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestWSClientConnectAndReceive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		// Send a mock status_update
		payload, _ := json.Marshal(map[string]bool{"bridge_connected": true})
		conn.WriteJSON(Envelope{
			Type:    TypeStatusUpdate,
			Payload: payload,
		})

		// Read expected prompt_submit
		var env Envelope
		if err := conn.ReadJSON(&env); err != nil {
			return
		}
		if env.Type != TypePromptSubmit {
			t.Errorf("expected TypePromptSubmit, got %s", env.Type)
		}
	}))
	defer server.Close()

	client := NewWSClient(server.URL)
	// Point directly to test server wsURL
	client.wsURL = toWebSocketURL(server.URL, "")

	if err := client.Connect(); err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	defer client.Close()

	select {
	case msg := <-client.MsgChan():
		env, ok := msg.(Envelope)
		if !ok {
			t.Fatalf("expected Envelope type, got %T", msg)
		}
		if env.Type != TypeStatusUpdate {
			t.Errorf("expected %s, got %s", TypeStatusUpdate, env.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for WS message")
	}

	if err := client.SendPrompt("pick up red block"); err != nil {
		t.Fatalf("SendPrompt failed: %v", err)
	}
}
