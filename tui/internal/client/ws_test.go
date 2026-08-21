package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
			t.Errorf("upgrade failed: %v", err)
			return
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
	client.Start()
	defer client.Close()

	select {
	case msg := <-client.MsgChan():
		if _, ok := msg.(Connected); !ok {
			t.Fatalf("expected Connected, got %T", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Connected")
	}

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

// TestWSClientReconnectsAfterRejection simulates a backend that refuses the
// first connection attempt (mirroring the hub's 409 Conflict when a stale
// connection hasn't been cleaned up yet) and accepts the next one, asserting
// the client reports Disconnected then retries into a successful Connected.
func TestWSClientReconnectsAfterRejection(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			http.Error(w, "a client is already connected", http.StatusConflict)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	client := NewWSClient(server.URL)
	client.wsURL = toWebSocketURL(server.URL, "")
	client.Start()
	defer client.Close()

	select {
	case msg := <-client.MsgChan():
		if _, ok := msg.(Disconnected); !ok {
			t.Fatalf("expected Disconnected on rejected first attempt, got %T", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first Disconnected")
	}

	select {
	case msg := <-client.MsgChan():
		if _, ok := msg.(Connected); !ok {
			t.Fatalf("expected Connected after backoff retry, got %T", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for reconnect")
	}

	if got := atomic.LoadInt32(&attempts); got < 2 {
		t.Fatalf("expected at least 2 connection attempts, got %d", got)
	}
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(reconnectBaseDelay); got != 2*reconnectBaseDelay {
		t.Errorf("expected backoff to double, got %s", got)
	}
	if got := nextBackoff(reconnectMaxDelay); got != reconnectMaxDelay {
		t.Errorf("expected backoff to stay capped at %s, got %s", reconnectMaxDelay, got)
	}
	if got := nextBackoff(reconnectMaxDelay / 2); got > reconnectMaxDelay {
		t.Errorf("expected backoff never to exceed cap, got %s", got)
	}
}
