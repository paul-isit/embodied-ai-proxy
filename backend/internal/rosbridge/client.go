package rosbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// BridgeObserver receives state transitions and telemetry from the rosbridge client.
type BridgeObserver interface {
	OnBridgeConnectionChange(connected bool)
	OnObjectsUpdated(objects []string)
	OnTelemetry(msg json.RawMessage)
}

type serviceResponse struct {
	Result bool            `json:"result"`
	Values json.RawMessage `json:"values"`
}

type incomingMessage struct {
	Op      string          `json:"op"`
	ID      string          `json:"id"`
	Service string          `json:"service"`
	Topic   string          `json:"topic"`
	Values  json.RawMessage `json:"values"`
	Result  *bool           `json:"result"`
	Msg     json.RawMessage `json:"msg"`
}

// Client connects to the ROS 2 rosbridge_server WebSocket endpoint (e.g. ws://localhost:9090).
type Client struct {
	url      string
	observer BridgeObserver

	mu        sync.Mutex
	ws        *websocket.Conn
	connected atomic.Bool
	reqSeq    atomic.Uint64

	objectsMu     sync.RWMutex
	availableObjs []string

	pendingMu sync.Mutex
	pending   map[string]chan serviceResponse
}

// NewClient creates a new rosbridge WebSocket client.
func NewClient(url string, observer BridgeObserver) *Client {
	if url == "" {
		url = "ws://localhost:9090"
	}
	return &Client{
		url:      url,
		observer: observer,
		pending:  make(map[string]chan serviceResponse),
	}
}

// IsConnected reports whether the client is connected to rosbridge.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// GetAvailableObjects returns the cached list of workspace objects.
func (c *Client) GetAvailableObjects() []string {
	c.objectsMu.RLock()
	defer c.objectsMu.RUnlock()
	return c.availableObjs
}

func (c *Client) setAvailableObjects(objects []string) {
	c.objectsMu.Lock()
	c.availableObjs = objects
	c.objectsMu.Unlock()

	if c.observer != nil {
		c.observer.OnObjectsUpdated(objects)
	}
}

// Start launches background connection and message handling in a goroutine.
func (c *Client) Start(ctx context.Context) {
	go c.run(ctx)
}

func (c *Client) run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := c.connectAndServe(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[Rosbridge] connection failed or dropped (%v); reconnecting in 2s...", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (c *Client) connectAndServe(ctx context.Context) error {
	log.Printf("[Rosbridge] connecting to %s...", c.url)
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, c.url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	c.mu.Lock()
	c.ws = ws
	c.mu.Unlock()

	c.connected.Store(true)
	log.Printf("[Rosbridge] connected to %s", c.url)
	if c.observer != nil {
		c.observer.OnBridgeConnectionChange(true)
	}

	defer func() {
		c.connected.Store(false)
		c.mu.Lock()
		if c.ws != nil {
			_ = c.ws.Close()
			c.ws = nil
		}
		c.mu.Unlock()

		c.setAvailableObjects(nil)
		if c.observer != nil {
			c.observer.OnBridgeConnectionChange(false)
		}

		c.pendingMu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	}()

	// Subscribe to telemetry
	_ = c.sendJSON(map[string]string{
		"op":    "subscribe",
		"topic": "/system/status",
		"type":  "kinova_interfaces/msg/SystemSummary",
	})

	// Fetch initial workspace objects once on connect
	go func() {
		fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if objs, err := c.FetchObjects(fetchCtx); err == nil {
			log.Printf("[Rosbridge] workspace objects fetched: %v", objs)
			c.setAvailableObjects(objs)
		} else {
			log.Printf("[Rosbridge] initial object fetch warning: %v", err)
		}
	}()

	// Read and dispatch incoming messages
	for {
		var msg incomingMessage
		if err := ws.ReadJSON(&msg); err != nil {
			return fmt.Errorf("read: %w", err)
		}

		switch msg.Op {
		case "service_response":
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.ID]
			c.pendingMu.Unlock()
			if ok {
				var res bool
				if msg.Result != nil {
					res = *msg.Result
				}
				ch <- serviceResponse{
					Result: res,
					Values: msg.Values,
				}
			}

		case "publish":
			if msg.Topic == "/system/status" && c.observer != nil {
				c.observer.OnTelemetry(msg.Msg)
			}
		}
	}
}

func (c *Client) sendJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws == nil {
		return errors.New("rosbridge websocket is not connected")
	}
	return c.ws.WriteJSON(v)
}

// CallService invokes a ROS 2 service over rosbridge and waits for the response.
func (c *Client) CallService(ctx context.Context, service string, args any) (json.RawMessage, error) {
	if !c.IsConnected() {
		return nil, errors.New("rosbridge is not connected")
	}

	reqID := fmt.Sprintf("srv:%s:%d", service, c.reqSeq.Add(1))
	respChan := make(chan serviceResponse, 1)

	c.pendingMu.Lock()
	c.pending[reqID] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, reqID)
		c.pendingMu.Unlock()
	}()

	callMsg := map[string]any{
		"op":      "call_service",
		"id":      reqID,
		"service": service,
		"args":    args,
	}

	if err := c.sendJSON(callMsg); err != nil {
		return nil, fmt.Errorf("send service request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-respChan:
		if !ok {
			return nil, errors.New("connection closed while waiting for service response")
		}
		if !resp.Result {
			return nil, fmt.Errorf("ROS service %s failed: %s", service, string(resp.Values))
		}
		return resp.Values, nil
	}
}

// FetchObjects queries the /get_robot_parameters ROS service for the detected object list.
func (c *Client) FetchObjects(ctx context.Context) ([]string, error) {
	values, err := c.CallService(ctx, "/get_robot_parameters", map[string]any{})
	if err != nil {
		return nil, err
	}

	var resp struct {
		ObjectList []string `json:"object_list"`
	}
	if err := json.Unmarshal(values, &resp); err != nil {
		return nil, fmt.Errorf("decode /get_robot_parameters response: %w", err)
	}
	return resp.ObjectList, nil
}

// ExecuteRecipe dispatches a validated action recipe to the /execute_recipe ROS service.
func (c *Client) ExecuteRecipe(ctx context.Context, recipeJSON []byte) error {
	args := map[string]string{
		"recipe_json": string(recipeJSON),
	}

	values, err := c.CallService(ctx, "/execute_recipe", args)
	if err != nil {
		return fmt.Errorf("call /execute_recipe: %w", err)
	}

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}
	if err := json.Unmarshal(values, &resp); err != nil {
		return fmt.Errorf("decode /execute_recipe response: %w", err)
	}

	if !resp.Success {
		if resp.Message != "" {
			return fmt.Errorf("recipe execution failed: %s", resp.Message)
		}
		return errors.New("robot failed to execute the recipe")
	}

	return nil
}
