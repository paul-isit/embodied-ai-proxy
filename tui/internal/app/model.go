package app

import (
	"bytes"
	"embodied-ai-proxy/tui/internal/client"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	statusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	promptLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ECE6A")).Render("> ")
)

// Model is the MVP root Bubble Tea model: it connects to the backend over
// WebSocket, accepts a single-line natural language prompt, and prints the
// raw backend response. The multi-panel dashboard and dual-mode keybindings
// are deferred to a later phase (see design.md Decision 5).
type Model struct {
	AppServerURL string
	Width        int
	Height       int
	Ready        bool

	ws       *client.WSClient
	input    textinput.Model
	output   []string
	connMsg  string
	inFlight bool
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
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.ws.Close()
			return m, tea.Quit
		case "enter":
			return m.submitPrompt()
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
		m.inFlight = false
		m.output = append(m.output, formatEnvelope(msg))
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
		m.output = append(m.output, errorStyle.Render("error: "+err.Error()))
		return m, nil
	}

	m.output = append(m.output, promptLabel+text)
	m.input.SetValue("")
	m.inFlight = true
	return m, nil
}

// formatEnvelope renders a raw backend envelope as plain text
func formatEnvelope(env client.Envelope) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, env.Payload, "", "  "); err != nil {
		return fmt.Sprintf("[%s] %s", env.Type, string(env.Payload))
	}
	return fmt.Sprintf("[%s]\n%s", env.Type, pretty.String())
}

// View renders the TUI
func (m Model) View() string {
	if !m.Ready {
		return "Initializing TUI..."
	}

	var b strings.Builder
	b.WriteString(statusStyle.Render("Backend: "+m.AppServerURL+" ["+m.connMsg+"]") + "\n\n")

	for _, line := range m.output {
		b.WriteString(line + "\n\n")
	}

	if m.inFlight {
		b.WriteString(statusStyle.Render("waiting for response...") + "\n")
	}

	b.WriteString(m.input.View() + "\n")
	b.WriteString("(ctrl+c to quit)")

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
