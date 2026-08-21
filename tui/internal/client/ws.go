package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

const (
	TypePromptSubmit = "prompt_submit"
	TypeActionRecipe = "action_recipe"
	TypeStatusUpdate = "status_update"
	TypeLogEvent     = "log_event"
)

// Envelope matches the backend WebSocket envelope shape
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// WSClient manages the WebSocket connection to ws://backend/ws/client
type WSClient struct {
	backendURL string
	wsURL      string
	conn       *websocket.Conn
	mu         sync.Mutex
	msgChan    chan tea.Msg
	closed     bool
}

// NewWSClient creates a new WSClient
func NewWSClient(backendURL string) *WSClient {
	wsURL := toWebSocketURL(backendURL, "/ws/client")
	return &WSClient{
		backendURL: backendURL,
		wsURL:      wsURL,
		msgChan:    make(chan tea.Msg, 50),
	}
}

// MsgChan returns the channel receiving messages for Bubble Tea
func (c *WSClient) MsgChan() <-chan tea.Msg {
	return c.msgChan
}

// Connect starts the background connection and the read loop
func (c *WSClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial backend websocket (%s): %w", c.wsURL, err)
	}

	c.conn = conn
	go c.readLoop(conn)
	return nil
}

func (c *WSClient) readLoop(conn *websocket.Conn) {
	for {
		var env Envelope
		err := conn.ReadJSON(&env)
		if err != nil {
			c.mu.Lock()
			if !c.closed {
				log.Printf("[WSClient] read error: %v", err)
			}
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()

		if !closed {
			c.msgChan <- env
		}
	}
}

// SendPrompt dispatches a prompt_submit envelope to this Go backend
func (c *WSClient) SendPrompt(prompt string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("websocket is not connected")
	}

	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return fmt.Errorf("marshal prompt payload: %w", err)
	}

	env := Envelope{
		Type:    TypePromptSubmit,
		Payload: payload,
	}

	return c.conn.WriteJSON(env)
}

// Close closes the WebSocket connection cleanly
func (c *WSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil

}

// toWebSocketURL converts a http(s) URL into a ws(s) endpoint URL
func toWebSocketURL(rawURL, path string) string {
	rawURL = strings.TrimRight(rawURL, "/")
	u, err := url.Parse(rawURL)
	if err != nil {
		return "ws://localhost:8080" + path
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}

	return fmt.Sprintf("%s://%s%s", scheme, u.Host, path)
}
