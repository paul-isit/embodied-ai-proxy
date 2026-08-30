package app

import (
	"bytes"
	"embodied-ai-proxy/tui/internal/client"
	"encoding/json"
	"fmt"
	"strings"

	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	statusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565F89"))

	sysTag  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7")).Render("[SYS] ")
	userTag = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BB9AF7")).Render("[USER] ")
	errTag  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F7768E")).Render("[ERR] ")
	okTag   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("[OK] ")
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

	history      []string
	historyIndex int
	historyDraft string

	verbosity    int
	promptSentAt time.Time
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
		historyIndex: -1,
		verbosity:    1,
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
		case tea.KeyUp, tea.KeyDown:
			return m.navigateHistory(msg.Type)
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case tea.KeyF2:
			return m.cycleVerbosity()
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
		switch msg.Type {
		case client.TypeStatusUpdate:
			if bc := decodeBridgeConnected(msg.Payload); bc != nil {
				m.bridgeConnected = bc
			}
			return m, waitForWSMsg(m.ws.MsgChan())

		case client.TypeActionRecipe:
			latency := time.Since(m.promptSentAt)
			m.inFlight = false
			m.appendEntry("", formatActionRecipe(msg.Payload, m.verbosity, latency))
			return m, waitForWSMsg(m.ws.MsgChan())

		case client.TypeLogEvent:
			m.appendEntry("", formatLogEvent(msg.Payload))
			return m, waitForWSMsg(m.ws.MsgChan())

		default:
			m.inFlight = false
			m.appendEntry("", formatEnvelope(msg))
			return m, waitForWSMsg(m.ws.MsgChan())
		}
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

	m.history = append([]string{text}, m.history...)
	m.historyIndex = -1
	m.historyDraft = ""

	if err := m.ws.SendPrompt(text); err != nil {
		m.appendEntry(errTag, err.Error())
		return m, nil
	}

	m.appendEntry(userTag, text)
	m.input.SetValue("")
	m.inFlight = true
	m.promptSentAt = time.Now()
	return m, nil
}

// cycleVerbosity advances response detail level 1 (Filtered) -> 2 (Full
// Context) -> 3 (Debug) -> back to 1, logging the change as a SYS line.
func (m Model) cycleVerbosity() (tea.Model, tea.Cmd) {
	m.verbosity = (m.verbosity % 3) + 1
	labels := map[int]string{1: "L1 - Filtered", 2: "L2 - Full Context", 3: "L3 - Debug"}
	m.appendEntry(sysTag, "Verbosity set to "+labels[m.verbosity])
	return m, nil
}

// navigateHistory moves through previously submitted prompts on Up/Down,
// preserving whatever was being typed (historyDraft) so paging back past
// the newest entry restores it rather than losing it.
func (m Model) navigateHistory(key tea.KeyType) (tea.Model, tea.Cmd) {
	if len(m.history) == 0 {
		return m, nil
	}

	switch key {
	case tea.KeyUp:
		if m.historyIndex == -1 {
			m.historyDraft = m.input.Value()
		}
		if m.historyIndex+1 < len(m.history) {
			m.historyIndex++
			m.input.SetValue(m.history[m.historyIndex])
			m.input.CursorEnd()
		}

	case tea.KeyDown:
		if m.historyIndex == -1 {
			return m, nil
		}
		m.historyIndex--
		if m.historyIndex < 0 {
			m.historyIndex = -1
			m.input.SetValue(m.historyDraft)
		} else {
			m.input.SetValue(m.history[m.historyIndex])
		}
		m.input.CursorEnd()
	}

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

// appendEntry appends a tagged line and refreshes the viewport in one step.
func (m *Model) appendEntry(tag, text string) {
	m.entries = append(m.entries, tag+text)
	m.refreshViewport()
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

// formatActionRecipe renders an action_recipe envelope: a validated recipe
// and its steps on success, or the schema/parsing error otherwise, per
// data/config/json_schema.json's success/error document shapes.
// beyond the base status line is gated by verbosity:
//
//	L1 - status/steps or error only
//	L2 - adds RawOutput, when present
//	L3 - adds latency/step-count metadata
func formatActionRecipe(payload json.RawMessage, verbosity int, latency time.Duration) string {
	var recipe ActionRecipeMsg
	if err := json.Unmarshal(payload, &recipe); err != nil {
		return errTag + "failed to parse action_recipe: " + err.Error()
	}

	var b strings.Builder

	if recipe.Status != "success" {
		b.WriteString(errTag)
		b.WriteString("Schema parsing failure")
		if recipe.ErrorType != "" {
			b.WriteString(" (" + recipe.ErrorType + ")")
		}
		if recipe.Message != "" {
			b.WriteByte('\n')
			b.WriteString(recipe.Message)
		}
	} else {
		b.WriteString(okTag)
		b.WriteString("Validated Robot Recipe")
		if recipe.RecipeName != "" {
			b.WriteString(": " + recipe.RecipeName)
		}
		for _, step := range recipe.Steps {
			b.WriteByte('\n')
			b.WriteString(fmt.Sprintf("  %d. %s", step.StepID, step.Action))
			if step.Description != "" {
				b.WriteString(" - " + step.Description)
			}
		}
	}

	if verbosity >= 2 && recipe.RawOutput != "" {
		b.WriteString("\n--- RAW OUTPUT ---\n")
		b.WriteString(recipe.RawOutput)
	}

	if verbosity >= 3 {
		b.WriteString(fmt.Sprintf("\n--- METADATA ---\nLatency: %dms | Steps: %d", latency.Milliseconds(), len(recipe.Steps)))
	}

	return b.String()
}

// formatLogEvent renders a log_event envelope, tagging it as an error or
// system line depending on its reported level.
func formatLogEvent(payload json.RawMessage) string {
	var evt LogEventMsg
	if err := json.Unmarshal(payload, &evt); err != nil {
		return errTag + "failed to parse log_event: " + err.Error()
	}

	tag := sysTag
	if strings.EqualFold(evt.Level, "error") {
		tag = errTag
	}
	return tag + evt.Message
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
