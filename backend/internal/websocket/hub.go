package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Envelope is the common message shape exchanged over /ws/client
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

const (
	TypePromptSubmit = "prompt_submit"
	TypeActionRecipe = "action_recipe"
	TypeStatusUpdate = "status_update"
	TypeLogEvent     = "log_event"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// conn wraps a websocket connection with a write mutex - gorilla connections
// do not support concurrent writers, but the hub can be asked to write to a
// connection (e.g. a broadcast) from a different goroutine than the one
// reading it.
type conn struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *conn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.WriteJSON(v)
}

// PromptHandler processes a prompt_submit payload received from a client connection.
type PromptHandler interface {
	HandlePrompt(ctx context.Context, prompt string)
}

// Hub tracks at most one connected TUI client - only one is supported at a time,
// so a second connection attempt is refused rather than replacing or multiplexing.
// It also bridges state transitions and telemetry from the backend's rosbridge client
// to the connected TUI client.
type Hub struct {
	mu              sync.RWMutex
	client          *conn
	bridgeConnected atomic.Bool

	promptHandler PromptHandler
}

func NewHub() *Hub {
	return &Hub{}
}

func (h *Hub) SetPromptHandler(handler PromptHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.promptHandler = handler
}

// OnBridgeConnectionChange implements rosbridge.BridgeObserver
func (h *Hub) OnBridgeConnectionChange(connected bool) {
	h.bridgeConnected.Store(connected)
	h.BroadcastBridgeConnected(connected)
}

// OnObjectsUpdated implements rosbridge.BridgeObserver
func (h *Hub) OnObjectsUpdated(objects []string) {
	payload, err := json.Marshal(map[string]any{"object_list": objects})
	if err != nil {
		return
	}
	h.SendToClient(Envelope{Type: TypeStatusUpdate, Payload: payload})
}

// OnTelemetry implements rosbridge.BridgeObserver
func (h *Hub) OnTelemetry(msg json.RawMessage) {
	payload, err := json.Marshal(map[string]any{"middleware_status": msg})
	if err != nil {
		return
	}
	h.SendToClient(Envelope{Type: TypeStatusUpdate, Payload: payload})
}

// ServeClient upgrades r to a WebSocket and registers it as the TUI/eval
// client connection for the lifetime of the connection. If a client is
// already connected, the request is refused with 409 Conflict rather than
// replacing the existing connection.
func (h *Hub) ServeClient(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	alreadyConnected := h.client != nil
	h.mu.RUnlock()
	if alreadyConnected {
		log.Printf("[Hub] rejecting client connection: a client is already connected")
		http.Error(w, "a client is already connected", http.StatusConflict)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Hub] client upgrade failed: %v", err)
		return
	}
	c := &conn{ws: ws}

	h.mu.Lock()
	if h.client != nil {
		h.mu.Unlock()
		log.Printf("[Hub] rejecting client connection: another client connected first")
		ws.Close()
		return
	}
	h.client = c
	h.mu.Unlock()
	log.Printf("[Hub] client connected")

	// Notify client of bridge state up-front
	bridgeConnected := h.bridgeConnected.Load()
	if payload, err := json.Marshal(map[string]bool{"bridge_connected": bridgeConnected}); err == nil {
		c.writeJSON(Envelope{Type: TypeStatusUpdate, Payload: payload})
	}

	clientCtx, clientCancel := context.WithCancel(r.Context())

	var (
		promptMu     sync.Mutex
		promptCancel context.CancelFunc
		isBusy       bool
	)

	defer func() {
		clientCancel()
		promptMu.Lock()
		if promptCancel != nil {
			promptCancel()
		}
		promptMu.Unlock()

		h.mu.Lock()
		if h.client == c {
			h.client = nil
		}
		h.mu.Unlock()
		ws.Close()
		log.Printf("[Hub] client disconnected")
	}()

	for {
		var env Envelope
		if err := ws.ReadJSON(&env); err != nil {
			logReadError("client", err)
			return
		}
		if env.Type != TypePromptSubmit {
			log.Printf("[Hub] ignoring unexpected message type from client: %s", env.Type)
			continue
		}

		var payload struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			log.Printf("[Hub] invalid prompt_submit payload: %v", err)
			continue
		}

		h.mu.RLock()
		handler := h.promptHandler
		h.mu.RUnlock()
		if handler != nil {
			promptMu.Lock()
			if isBusy {
				promptMu.Unlock()
				log.Printf("[Hub] rejecting prompt_submit: a command is already in progress")
				h.SendToClient(Envelope{
					Type:    TypeLogEvent,
					Payload: []byte(`{"level":"error","message":"command rejected: another command is currently in progress"}`),
				})
				continue
			}
			isBusy = true
			promptCtx, cancel := context.WithTimeout(clientCtx, 5*time.Minute)
			promptCancel = cancel
			promptMu.Unlock()

			go func(pText string, pCtx context.Context) {
				defer func() {
					promptMu.Lock()
					isBusy = false
					promptCancel = nil
					promptMu.Unlock()
				}()
				handler.HandlePrompt(pCtx, pText)
			}(payload.Prompt, promptCtx)
		}
	}
}

// BroadcastBridgeConnected tells the connected client whether the ROS 2
// bridge is currently connected.
func (h *Hub) BroadcastBridgeConnected(connected bool) {
	payload, err := json.Marshal(map[string]bool{"bridge_connected": connected})
	if err != nil {
		return
	}
	h.SendToClient(Envelope{Type: TypeStatusUpdate, Payload: payload})
}

func logReadError(who string, err error) {
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		log.Printf("[Hub] %s read error: %v", who, err)
	}
}

// SendToClient delivers env to the connected TUI client, if any.
func (h *Hub) SendToClient(env Envelope) {
	h.mu.RLock()
	c := h.client
	h.mu.RUnlock()

	if c == nil {
		return
	}
	if err := c.writeJSON(env); err != nil {
		log.Printf("[Hub] failed to write to client: %v", err)
	}
}

// BridgeConnected reports whether a ROS 2 bridge is currently connected.
func (h *Hub) BridgeConnected() bool {
	return h.bridgeConnected.Load()
}

// Stats reports whether a client is currently connected and whether the ROS 2 bridge
// is connected.
func (h *Hub) Stats() (clientCount int, bridgeConnected bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.client != nil {
		clientCount = 1
	}
	return clientCount, h.bridgeConnected.Load()
}
