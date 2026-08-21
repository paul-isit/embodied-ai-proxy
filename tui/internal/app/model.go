package app

import (
	"bytes"
	"embodied-ai-proxy/tui/internal/client"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	statusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565F89"))
	promptLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ECE6A")).Render("> ")
)

// fixedLines is the number of rows the layout always reserves outside the
// scrollable viewport: top+bottom padding, the header, the status/in-flight
// line, the input line, and the footer hint - used to size the viewport
// against the real terminal height.
const fixedLines = 6

// Model is the MVP root Bubble Tea model: it connects to the backend over
// WebSocket, accepts a single-line natural language prompt, and prints the
// raw backend response in a scrollable viewport.
type Model struct {
	AppServerURL string
	Width        int
	Height       int
	Ready        bool

	ws       *client.WSClient
	input    textinput.Model
	viewport viewport.Model

	entries         []string
	connMsg         string
	bridgeConnected *bool
	inFlight        bool
}

// NewModel creates a new initial Model instance
func NewModel(appServerURL string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a command and press Enter..."
	ti.Focus()
	ti.CharLimit = 500

	return Model{
		AppServerURL: appServerURL,
		ws:           client.NewWSClient(appServerURL),
		input:        ti,
		viewport:     viewport.New(0, 0),
		connMsg:      "connecting...",
	}
}

// waitForWSMsg returns a tea.Cmd that blocks for the next message off the
// WebSocket client's channel. The Bubble Tea runtime does not poll external
// channels on its own, so Update must re-issue this command after every
// message it receives to keep draining the channel.
func waitForWSMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// Init initialises the event loop and runs the startup commands
func (m Model) Init() tea.Cmd {
	m.ws.Start()
	return tea.Batch(textinput.Blink, waitForWSMsg(m.ws.MsgChan()))
}

// Update handles incoming messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
		m.viewport.Width = contentWidth(m.Width)
		m.viewport.Height = max(3, m.Height-fixedLines)
		m.refreshViewport()
		return m, nil

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.ws.Close()
			return m, tea.Quit
		case tea.KeyEnter:
			return m.submitPrompt()
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case client.Connected:
		m.connMsg = "connected"
		return m, waitForWSMsg(m.ws.MsgChan())

	case client.Disconnected:
		if msg.Err != nil {
			m.connMsg = fmt.Sprintf("disconnected (%v) - reconnecting...", msg.Err)
		} else {
			m.connMsg = "disconnected - reconnecting..."
		}
		return m, waitForWSMsg(m.ws.MsgChan())

	case client.Envelope:
		if msg.Type == client.TypeStatusUpdate {
			if bc := decodeBridgeConnected(msg.Payload); bc != nil {
				m.bridgeConnected = bc
			}
			return m, waitForWSMsg(m.ws.MsgChan())
		}
		m.inFlight = false
		m.entries = append(m.entries, formatEnvelope(msg))
		m.refreshViewport()
		return m, waitForWSMsg(m.ws.MsgChan())
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// submitPrompt dispatches the current input text as a prompt_submit
// envelope, unless a command is already in flight or the input is empty.
func (m Model) submitPrompt() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || m.inFlight {
		return m, nil
	}

	if err := m.ws.SendPrompt(text); err != nil {
		m.entries = append(m.entries, errorStyle.Render("error: "+err.Error()))
		m.refreshViewport()
		return m, nil
	}

	m.entries = append(m.entries, promptLabel+text)
	m.refreshViewport()
	m.input.SetValue("")
	m.inFlight = true
	return m, nil
}

// refreshViewport re-wraps every entry to the viewport's current width and
// scrolls to the bottom, so new messages are always visible immediately
// while pgup/pgdn (or the mouse wheel) can still scroll back through history.
func (m *Model) refreshViewport() {
	width := m.viewport.Width
	if width <= 0 {
		return
	}
	wrapped := make([]string, len(m.entries))
	for i, e := range m.entries {
		wrapped[i] = lipgloss.NewStyle().Width(width).Render(e)
	}
	m.viewport.SetContent(strings.Join(wrapped, "\n\n"))
	m.viewport.GotoBottom()
}

// decodeBridgeConnected extracts the optional bridge_connected field from a
// status_update payload, if present.
func decodeBridgeConnected(payload json.RawMessage) *bool {
	var v struct {
		BridgeConnected *bool `json:"bridge_connected"`
	}
	if err := json.Unmarshal(payload, &v); err != nil {
		return nil
	}
	return v.BridgeConnected
}

// formatEnvelope renders a raw backend envelope as plain text
func formatEnvelope(env client.Envelope) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, env.Payload, "", "  "); err != nil {
		return fmt.Sprintf("[%s] %s", env.Type, string(env.Payload))
	}
	return fmt.Sprintf("[%s]\n%s", env.Type, pretty.String())
}

func contentWidth(termWidth int) int {
	return max(1, termWidth-4)
}

func bridgeStatusText(connected *bool) string {
	switch {
	case connected == nil:
		return "bridge: unknown"
	case *connected:
		return "bridge: connected"
	default:
		return "bridge: disconnected"
	}
}

// View renders the TUI
func (m Model) View() string {
	if !m.Ready {
		return "Initializing TUI..."
	}

	var b strings.Builder
	b.WriteString(statusStyle.Render(fmt.Sprintf("Backend: %s [%s] | %s", m.AppServerURL, m.connMsg, bridgeStatusText(m.bridgeConnected))) + "\n")
	b.WriteString(m.viewport.View() + "\n")

	if m.inFlight {
		b.WriteString(statusStyle.Render("waiting for response...") + "\n")
	} else {
		b.WriteString("\n")
	}

	b.WriteString(m.input.View() + "\n")
	b.WriteString(mutedStyle.Render("(enter to submit • pgup/pgdn or mouse wheel to scroll • ctrl+c to quit)"))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
