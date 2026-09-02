package rosbridge

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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type mockObserver struct {
	mu           sync.Mutex
	connStates   []bool
	objectsList  [][]string
	telemetryMsg []string
}

func (m *mockObserver) OnBridgeConnectionChange(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connStates = append(m.connStates, connected)
}

func (m *mockObserver) OnObjectsUpdated(objects []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objectsList = append(m.objectsList, objects)
}

func (m *mockObserver) OnTelemetry(msg json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.telemetryMsg = append(m.telemetryMsg, string(msg))
}

func TestClient_Connect_SubscribesAndFetchesObjects(t *testing.T) {
	subscribed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var msg map[string]any
			if err := ws.ReadJSON(&msg); err != nil {
				return
			}
			op, _ := msg["op"].(string)
			if op == "subscribe" {
				close(subscribed)
			} else if op == "call_service" {
				service, _ := msg["service"].(string)
				id, _ := msg["id"].(string)
				if service == "/get_robot_parameters" {
					trueVal := true
					ws.WriteJSON(map[string]any{
						"op":      "service_response",
						"id":      id,
						"service": service,
						"result":  &trueVal,
						"values": map[string]any{
							"object_list": []string{"red_cube", "blue_tray"},
						},
					})
				}
			}
		}
	}))
	defer server.Close()

	obs := &mockObserver{}
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewClient(wsURL, obs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client.Start(ctx)

	select {
	case <-subscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscribe")
	}

	// Allow initial fetch to arrive
	time.Sleep(100 * time.Millisecond)

	if !client.IsConnected() {
		t.Error("expected client to be connected")
	}

	objs := client.GetAvailableObjects()
	if len(objs) != 2 || objs[0] != "red_cube" || objs[1] != "blue_tray" {
		t.Errorf("GetAvailableObjects() = %v, want [red_cube, blue_tray]", objs)
	}
}

func TestClient_ExecuteRecipe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var msg map[string]any
			if err := ws.ReadJSON(&msg); err != nil {
				return
			}
			if msg["op"] == "call_service" && msg["service"] == "/execute_recipe" {
				id, _ := msg["id"].(string)
				trueVal := true
				ws.WriteJSON(map[string]any{
					"op":      "service_response",
					"id":      id,
					"service": "/execute_recipe",
					"result":  &trueVal,
					"values": map[string]any{
						"success": true,
					},
				})
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewClient(wsURL, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client.Start(ctx)

	// Wait for connection
	for i := 0; i < 20; i++ {
		if client.IsConnected() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	err := client.ExecuteRecipe(context.Background(), []byte(`{"recipe_name":"test"}`))
	if err != nil {
		t.Fatalf("ExecuteRecipe() error = %v", err)
	}
}

func TestClient_ExecuteRecipe_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var msg map[string]any
			if err := ws.ReadJSON(&msg); err != nil {
				return
			}
			if msg["op"] == "call_service" && msg["service"] == "/execute_recipe" {
				id, _ := msg["id"].(string)
				trueVal := true
				ws.WriteJSON(map[string]any{
					"op":      "service_response",
					"id":      id,
					"service": "/execute_recipe",
					"result":  &trueVal,
					"values": map[string]any{
						"success": false,
						"message": "arm trajectory failed",
					},
				})
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client := NewClient(wsURL, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client.Start(ctx)

	for i := 0; i < 20; i++ {
		if client.IsConnected() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	err := client.ExecuteRecipe(context.Background(), []byte(`{"recipe_name":"test"}`))
	if err == nil || !strings.Contains(err.Error(), "arm trajectory failed") {
		t.Fatalf("ExecuteRecipe() expected arm trajectory failed error, got = %v", err)
	}
}
