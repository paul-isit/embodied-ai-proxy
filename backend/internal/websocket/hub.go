package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Envelope is the common message shape exchanged over both /ws/client and /ws/bridge
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

// StatusHandler observes each status_update received from the bridge before
// it is broadcast to the client - e.g. to cache a workspace object list.
type StatusHandler interface {
	HandleBridgeStatus(env Envelope)
}

// Hub tracks at most one connected TUI client and at most one ROS 2 bridge
// connection - only one of each is supported at a time, so a second
// connection attempt on either endpoint is refused rather than replacing or
// multiplexing across the existing one. (Should the system ever need
// multiple simultaneous TUI clients, this is the type to revisit.)
type Hub struct {
	mu     sync.RWMutex
	client *conn
	bridge *conn

	promptHandler PromptHandler
	statusHandler StatusHandler
}

func NewHub() *Hub {
	return &Hub{}
}

func (h *Hub) SetPromptHandler(handler PromptHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.promptHandler = handler
}

func (h *Hub) SetStatusHandler(handler StatusHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statusHandler = handler
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
		// Lost a race against another connection attempt between the check
		// above and now - refuse this one too.
		h.mu.Unlock()
		log.Printf("[Hub] rejecting client connection: another client connected first")
		ws.Close()
		return
	}
	h.client = c
	bridgeConnected := h.bridge != nil
	h.mu.Unlock()
	log.Printf("[Hub] client connected")

	// A client that connects after the bridge already did would otherwise
	// never learn the bridge is up - broadcastBridgeConnected only fires on
	// the bridge's own connect/disconnect transition, not for later joiners.
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
			promptCtx, cancel := context.WithTimeout(clientCtx, 60*time.Second)
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

// ServeBridge upgrades r to a WebSocket and registers it as the ROS 2 bridge
// connection. If a bridge is already connected, the request is refused with
// 409 Conflict rather than replacing the existing connection.
func (h *Hub) ServeBridge(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	alreadyConnected := h.bridge != nil
	h.mu.RUnlock()
	if alreadyConnected {
		log.Printf("[Hub] rejecting bridge connection: a bridge is already connected")
		http.Error(w, "a ROS 2 bridge is already connected", http.StatusConflict)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Hub] bridge upgrade failed: %v", err)
		return
	}
	c := &conn{ws: ws}

	h.mu.Lock()
	if h.bridge != nil {
		// Lost a race against another connection attempt between the check
		// above and now - refuse this one too.
		h.mu.Unlock()
		log.Printf("[Hub] rejecting bridge connection: another bridge connected first")
		ws.Close()
		return
	}
	h.bridge = c
	h.mu.Unlock()
	log.Printf("[Hub] ROS 2 bridge connected")
	h.broadcastBridgeConnected(true)

	defer func() {
		h.mu.Lock()
		if h.bridge == c {
			h.bridge = nil
		}
		h.mu.Unlock()
		ws.Close()
		log.Printf("[Hub] ROS 2 bridge disconnected")
		h.broadcastBridgeConnected(false)
	}()

	for {
		var env Envelope
		if err := ws.ReadJSON(&env); err != nil {
			logReadError("bridge", err)
			return
		}
		if env.Type != TypeStatusUpdate {
			log.Printf("[Hub] ignoring unexpected message type from bridge: %s", env.Type)
			continue
		}

		h.mu.RLock()
		sh := h.statusHandler
		h.mu.RUnlock()
		if sh != nil {
			sh.HandleBridgeStatus(env)
		}

		h.SendToClient(env)
	}
}

// broadcastBridgeConnected tells the connected client whether the ROS 2
// bridge is currently connected - a distinct, push-based signal from the
// client's own WebSocket connection state.
func (h *Hub) broadcastBridgeConnected(connected bool) {
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

// SendToClient delivers env to the connected TUI client, if any. It is a
// no-op (not an error) when no client is connected, since most callers are
// pushing best-effort status/log events that simply have no one to reach.
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

// SendToBridge delivers env (typically an action_recipe) to the connected
// ROS 2 bridge.
func (h *Hub) SendToBridge(env Envelope) error {
	h.mu.RLock()
	bridge := h.bridge
	h.mu.RUnlock()

	if bridge == nil {
		return errors.New("no ROS 2 bridge connected")
	}
	return bridge.writeJSON(env)
}

// BridgeConnected reports whether a ROS 2 bridge is currently connected.
func (h *Hub) BridgeConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bridge != nil
}

// Stats reports whether a client is currently connected (as 0 or 1, for the
// /api/info "clients_connected" field's benefit) and whether a ROS 2 bridge
// is connected, for health/diagnostic reporting.
func (h *Hub) Stats() (clientCount int, bridgeConnected bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.client != nil {
		clientCount = 1
	}
	return clientCount, h.bridge != nil
}
