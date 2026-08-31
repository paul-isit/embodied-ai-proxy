package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

const (
	TypePromptSubmit = "prompt_submit"
	TypeActionRecipe = "action_recipe"
	TypeStatusUpdate = "status_update"
	TypeLogEvent     = "log_event"
)

// Reconnect backoff bounds: doubles from the base delay up to the cap after
// each failed attempt, and resets back to the base delay after any
// successful connection.
const (
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
)

// Envelope matches the backend WebSocket envelope shape
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Connected is sent on MsgChan() whenever the WebSocket connection is
// (re)established.
type Connected struct{}

// Disconnected is sent on MsgChan() whenever the connection drops or an
// initial dial attempt fails, before the next reconnect attempt begins.
type Disconnected struct {
	Err error
}

// WSClient manages the WebSocket connection to ws://backend/ws/client,
// automatically reconnecting with exponential backoff for as long as the
// client is open - whether the initial dial fails, the backend refuses the
// connection (e.g. 409 because a stale connection hasn't been cleaned up
// yet), or an established connection's read loop errors out.
type WSClient struct {
	backendURL string
	wsURL      string

	mu     sync.Mutex
	conn   *websocket.Conn
	closed bool
	stopCh chan struct{}

	msgChan chan tea.Msg
}

// NewWSClient creates a new WSClient.
func NewWSClient(backendURL string) *WSClient {
	wsURL := toWebSocketURL(backendURL, "/ws/client")
	return &WSClient{
		backendURL: backendURL,
		wsURL:      wsURL,
		msgChan:    make(chan tea.Msg, 50),
		stopCh:     make(chan struct{}),
	}
}

// MsgChan returns the channel receiving messages for Bubble Tea: Connected,
// Disconnected, and Envelope values arrive here as the connection's state
// changes and as messages are read from the backend.
func (c *WSClient) MsgChan() <-chan tea.Msg {
	return c.msgChan
}

// Start begins connecting in the background and keeps reconnecting with
// exponential backoff for as long as the client is open. It returns
// immediately; connection state and incoming messages are reported on
// MsgChan().
func (c *WSClient) Start() {
	go c.connectLoop()
}

func (c *WSClient) connectLoop() {
	delay := reconnectBaseDelay
	for {
		if c.isClosed() {
			return
		}

		conn, err := c.dial()
		if err != nil {
			log.Printf("[WSClient] connect to %s failed: %v (retrying in %s)", c.wsURL, err, delay)
			c.msgChan <- Disconnected{Err: err}
			if !c.sleepOrStop(delay) {
				return
			}
			delay = nextBackoff(delay)
			continue
		}

		delay = reconnectBaseDelay
		c.setConn(conn)
		c.msgChan <- Connected{}

		err = c.readLoop(conn)

		c.setConn(nil)
		if c.isClosed() {
			return
		}
		log.Printf("[WSClient] connection to %s lost: %v (reconnecting)", c.wsURL, err)
		c.msgChan <- Disconnected{Err: err}
	}
}

// dial performs a single connection attempt, folding a rejected handshake
// (e.g. 409 Conflict from the hub) and a lower-level dial failure into the
// same error - the caller's response (back off and retry) is the same
// either way.
func (c *WSClient) dial() (*websocket.Conn, error) {
	conn, resp, err := websocket.DefaultDialer.Dial(c.wsURL, nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial backend websocket (%s): %w (status %s)", c.wsURL, err, resp.Status)
		}
		return nil, fmt.Errorf("dial backend websocket (%s): %w", c.wsURL, err)
	}
	return conn, nil
}

// readLoop blocks reading envelopes off conn and forwarding them to
// msgChan until conn errors or the client is closed.
func (c *WSClient) readLoop(conn *websocket.Conn) error {
	for {
		var env Envelope
		if err := conn.ReadJSON(&env); err != nil {
			return err
		}
		if c.isClosed() {
			return nil
		}
		c.msgChan <- env
	}
}

// SendPrompt dispatches a prompt_submit envelope to the Go backend.
func (c *WSClient) SendPrompt(prompt string) error {
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return fmt.Errorf("marshal prompt payload: %w", err)
	}
	env := Envelope{Type: TypePromptSubmit, Payload: payload}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("websocket is not connected")
	}
	return c.conn.WriteJSON(env)
}

// Close stops reconnect attempts and closes any open connection.
func (c *WSClient) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.stopCh)
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}

func (c *WSClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *WSClient) setConn(conn *websocket.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

// sleepOrStop waits for d, returning false early (without waiting out the
// full delay) if the client is closed in the meantime.
func (c *WSClient) sleepOrStop(d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-c.stopCh:
		return false
	}
}

func nextBackoff(d time.Duration) time.Duration {
	next := d * 2
	if next > reconnectMaxDelay {
		return reconnectMaxDelay
	}
	return next
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
