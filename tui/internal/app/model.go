package app

import (
	"context"
	"embodied-ai-proxy/tui/internal/client"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the MVP root Bubble Tea model: it connects to the backend over
// WebSocket, accepts a single-line natural language prompt, and prints the
// raw backend response in a scrollable viewport.
type Model struct {
	AppServerURL string
	DataDir      string
	Width        int
	Height       int
	Ready        bool

	ws       *client.WSClient
	api      *client.APIClient
	input    textinput.Model
	viewport viewport.Model

	entries         []string
	connMsg         string
	bridgeConnected *bool
	inFlight        bool

	availableObjects []string
	telemetry        *MiddlewareStatus
	showSidebar       bool

	spin spinner.Model

	history      []string
	historyIndex int
	historyDraft string

	verbosity      int
	promptSentAt   time.Time

	llmProvider string
	llmModel    string

	hardwareClientLog []string

}

// NewModel creates a new initial Model instance
func NewModel(appServerURL, dataDir string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a command and press Enter..."
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = statusStyle

	return Model{
		AppServerURL: appServerURL,
		DataDir:      dataDir,
		ws:           client.NewWSClient(appServerURL),
		api:          client.NewAPIClient(appServerURL),
		input:        ti,
		viewport:     viewport.New(0, 0),
		connMsg:      "connecting...",
		historyIndex: -1,
		verbosity:    1,
		showSidebar:  true,
		spin:         sp,
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

// elapsedTickMsg drives the live "(Ns)" counter shown next to the spinner
// while a prompt is in flight.
type elapsedTickMsg struct{}

func elapsedTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return elapsedTickMsg{}
	})
}

// fetchSystemInfo returns a tea.Cmd that calls GET /api/info in the
// background, since APIClient.FetchInfo blocks on HTTP and must not run
// directly inside Update.
func fetchSystemInfo(api *client.APIClient, use string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		info, err := api.FetchInfo(ctx)
		return SystemInfoMsg{Info: info, Err: err, Use: use}
	}
}

// Init initialises the event loop and runs the startup commands
func (m Model) Init() tea.Cmd {
	m.ws.Start()
	return tea.Batch(textinput.Blink, waitForWSMsg(m.ws.MsgChan()), fetchSystemInfo(m.api, "llm_header"),)
}

// Update handles incoming messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true

		if m.showSidebar {
			m.viewport.Width = contentWidth(m.Width) - sidebarWidth - 6
		} else {
			m.viewport.Width = contentWidth(m.Width) - 6
		}

		if m.viewport.Width < 1 {
			m.viewport.Width = 1
		}

		m.viewport.Height = calculateViewportHeight(m)

		m = m.refreshViewport()
		return m, tea.ClearScreen

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

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

	case SystemInfoMsg:
		if msg.Use == "llm_header" {
			if msg.Err == nil && msg.Info != nil {
				m.llmProvider = msg.Info.LLM.Provider
				m.llmModel = msg.Info.LLM.Model
			}
			return m, nil
		}	

		m = m.appendEntry("", formatSystemInfo(msg))
		return m, nil

	case spinner.TickMsg:
		if !m.inFlight {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case elapsedTickMsg:
		if !m.inFlight {
			return m, nil
		}
		return m, elapsedTick()

	case client.Envelope:
		return m.handleEnvelope(msg)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleKeyMsg dispatches a tea.KeyMsg to the appropriate action.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	}

	// Not a recognized shortcut — let the text input handle it (typing, backspace, etc.)
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleEnvelope dispatches a client.Envelope by its Type.
func (m Model) handleEnvelope(msg client.Envelope) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case client.TypeStatusUpdate:
		bc, objList := decodeBridgeConnected(msg.Payload)
		if bc != nil {
			m.bridgeConnected = bc
			if !*bc {
				m.availableObjects = nil
			}
		}
		if objList != nil {
			m.availableObjects = objList
		}
		if ms := decodeMiddlewareStatus(msg.Payload); ms != nil {
			m = m.appendHardwareClientLog(ms)
			m.telemetry = ms
		}
		return m, waitForWSMsg(m.ws.MsgChan())

	case client.TypeActionRecipe:
		latency := time.Since(m.promptSentAt)
		m.inFlight = false
		m = m.appendEntry("", formatActionRecipe(msg.Payload, m.verbosity, latency))
		return m, waitForWSMsg(m.ws.MsgChan())

	case client.TypeLogEvent:
		m = m.appendEntry("", formatLogEvent(msg.Payload))
		if isErrorLevel(msg.Payload) {
			m.inFlight = false
		}
		return m, waitForWSMsg(m.ws.MsgChan())

	default:
		m.inFlight = false
		m = m.appendEntry("", formatEnvelope(msg))
		return m, waitForWSMsg(m.ws.MsgChan())
	}
}

// submitPrompt dispatches the current input text as a prompt_submit
// envelope, unless a command is already in flight or the input is empty.
func (m Model) submitPrompt() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || m.inFlight {
		return m, nil
	}

	if strings.HasPrefix(text, "/") {
		m.input.SetValue("")
		return m.handleSlashCommand(text)
	}

	if m.inFlight {
		return m, nil
	}

	m.history = append([]string{text}, m.history...)
	m.historyIndex = -1
	m.historyDraft = ""

	if err := m.ws.SendPrompt(text); err != nil {
		m = m.appendEntry(errTag, err.Error())
		return m, nil
	}

	m = m.appendEntry(userTag, text)
	m.input.SetValue("")
	m.inFlight = true
	m.promptSentAt = time.Now()
	m.hardwareClientLog = nil
	return m, tea.Batch(m.spin.Tick, elapsedTick())
}

func (m Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	name := strings.ToLower(strings.TrimPrefix(text, "/"))
	name = strings.Fields(name)[0]

	switch name {
	case "help", "h":
		m = m.appendEntry(sysTag, helpText(m.verbosity))
		return m, nil
	case "filtered":
		return m.setVerbosity(1, "L1 - Filtered")
	case "context":
		return m.setVerbosity(2, "L2 - Full Context")
	case "debug":
		return m.setVerbosity(3, "L3 - Debug")
	case "system", "sys":
		return m, fetchSystemInfo(m.api, "system")
	case "llm":
		return m, fetchSystemInfo(m.api, "llm")
	case "sidebar":
		m.showSidebar = !m.showSidebar

		if m.showSidebar {
			m.viewport.Width = contentWidth(m.Width) - sidebarWidth - 6
		} else {
			m.viewport.Width = contentWidth(m.Width) - 6
		}

		if m.viewport.Width < 1 {
			m.viewport.Width = 1
		}

		m = m.refreshViewport()
		return m, tea.ClearScreen
	case "save":
		path, err := m.saveSession()
		if err != nil {
			m = m.appendEntry(errTag, "failed to save session: "+err.Error())
		} else {
			m = m.appendEntry(sysTag, "Session saved to "+path)
		}
		return m, nil
	default:
		m = m.appendEntry(errTag, "unknown command: /"+name+" (try /help)")
		return m, nil
	}
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// saveSession writes the current session's entries to a timestamped plain
// text file under DataDir/logs.
func (m Model) saveSession() (string, error) {
	dir := filepath.Join(m.DataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create log directory: %w", err)
	}

	filename := fmt.Sprintf("session_%s.txt", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, filename)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Session export - %s\n", time.Now().Format(time.RFC1123)))
	b.WriteString(fmt.Sprintf("Backend: %s\n", m.AppServerURL))
	b.WriteString(strings.Repeat("-", 60) + "\n\n")
	for _, e := range m.entries {
		b.WriteString(stripANSI(e))
		b.WriteString("\n\n")
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write session file: %w", err)
	}
	return path, nil
}

// setVerbosity sets the response detail level directly and logs the change
// as a SYS line, unless it's already at that level.
func (m Model) setVerbosity(level int, label string) (tea.Model, tea.Cmd) {
	if m.verbosity == level {
		m = m.appendEntry(sysTag, "Already at "+label)
		return m, nil
	}
	m.verbosity = level
	m = m.appendEntry(sysTag, "Verbosity set to "+label)
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
func (m Model) refreshViewport() Model {
	width := m.viewport.Width
	if width <= 0 {
		return m
	}
	wrapped := make([]string, len(m.entries))
	for i, e := range m.entries {
		wrapped[i] = lipgloss.NewStyle().Width(width).Render(e)
	}
	m.viewport.SetContent(strings.Join(wrapped, "\n\n"))
	m.viewport.GotoBottom()
	return m
}

// appendEntry appends a tagged line and refreshes the viewport in one step.
func (m Model) appendEntry(tag, text string) Model {
	m.entries = append(m.entries, tag+text)
	return m.refreshViewport()
}

const hardwareClientNodeName = "kinova_hardware_client"

// appendHardwareClientLog records a new line in hardwareClientLog whenever
// kinova_hardware_client's status_message changes, capped to the most
// recent hardwareClientLogCap entries and prompt.
func (m Model) appendHardwareClientLog(status *MiddlewareStatus) Model {
	for _, n := range status.IndividualStates {
		if n.NodeName != hardwareClientNodeName {
			continue
		}
		last := ""
		if len(m.hardwareClientLog) > 0 {
			last = m.hardwareClientLog[len(m.hardwareClientLog)-1]
		}
		if last != "" && strings.HasSuffix(last, n.StatusMessage) {
			break
		}
		line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), n.StatusMessage)
		m.hardwareClientLog = append(m.hardwareClientLog, line)
		break
	}
	return m
}

// decodeBridgeConnected extracts the optional bridge_connected field from a
// status_update payload, if present.
func decodeBridgeConnected(payload json.RawMessage) (*bool, []string) {
	var v struct {
		BridgeConnected *bool    `json:"bridge_connected"`
		ObjectList      []string `json:"object_list"`
	}
	if err := json.Unmarshal(payload, &v); err != nil {
		return nil, nil
	}
	return v.BridgeConnected, v.ObjectList
}

// decodeMiddlewareStatus extracts the optional middleware_status field from
// a status_update payload, if present.
func decodeMiddlewareStatus(payload json.RawMessage) *MiddlewareStatus {
	var v struct {
		MiddlewareStatus *MiddlewareStatus `json:"middleware_status"`
	}
	if err := json.Unmarshal(payload, &v); err != nil {
		return nil
	}
	return v.MiddlewareStatus
}

func isErrorLevel(payload json.RawMessage) bool {
	var evt LogEventMsg
	if err := json.Unmarshal(payload, &evt); err != nil {
		return false
	}
	return strings.EqualFold(evt.Level, "error")
}
