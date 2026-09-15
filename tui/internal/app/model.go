package app

import (
	"bytes"
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

var (
	statusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565F89"))

	sysTag  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7")).Render("[SYS] ")
	userTag = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BB9AF7")).Render("[USER] ")
	errTag  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F7768E")).Render("[ERR] ")
	okTag   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render("[OK] ")

	nodeReadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ECE6A"))
	nodeBusyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68"))
	nodeFaultStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F7768E"))
	sidebarTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7AA2F7"))

	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#C0CAF5"))
	dividerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B4261"))
	connectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ECE6A"))
	disconnStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F7768E"))
	inputBoxStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#565F89")).Padding(0, 1)
)

// fixedLines is the number of rows the layout always reserves outside the
// scrollable viewport: top+bottom padding, the header, the status/in-flight
// line, the input line, and the footer hint - used to size the viewport
// against the real terminal height.
const fixedLines = 11

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
	pendingInfoUse string
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
func fetchSystemInfo(api *client.APIClient) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		info, err := api.FetchInfo(ctx)
		return SystemInfoMsg{Info: info, Err: err}
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
		if m.showSidebar && m.Width >= minWidthForSidebar {
			m.viewport.Width = contentWidth(m.Width) - sidebarWidth
		} else {
			m.viewport.Width = contentWidth(m.Width)
		}
		m.viewport.Height = max(3, m.Height-fixedLines)
		m = m.refreshViewport()
		return m, tea.ClearScreen

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
		case tea.KeyF1:
			m.appendEntry(sysTag, helpText(m.verbosity))
			return m, nil
		case tea.KeyF2:
			return m.cycleVerbosity()
		case tea.KeyF3:
			m.pendingInfoUse = "system"
			return m, fetchSystemInfo(m.api)
		case tea.KeyF4:
			m.pendingInfoUse = "llm"
			return m, fetchSystemInfo(m.api)
		case tea.KeyF5:
			m.showSidebar = !m.showSidebar
			if m.showSidebar && m.Width >= minWidthForSidebar {
				m.viewport.Width = contentWidth(m.Width) - sidebarWidth
			} else {
				m.viewport.Width = contentWidth(m.Width)
			}
			m = m.refreshViewport()
			return m, tea.ClearScreen

		case tea.KeyF6:
			path, err := m.saveSession()
			if err != nil {
				m.appendEntry(errTag, "failed to save session: "+err.Error())
			} else {
				m.appendEntry(sysTag, "Session saved to "+path)
			}
			return m, nil
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

	case SystemInfoMsg:
		m.appendEntry("", formatSystemInfo(msg, m.pendingInfoUse))
		m.pendingInfoUse = ""
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
				m.telemetry = ms
			}
			return m, waitForWSMsg(m.ws.MsgChan())

		case client.TypeActionRecipe:
			latency := time.Since(m.promptSentAt)
			m.inFlight = false
			m.appendEntry("", formatActionRecipe(msg.Payload, m.verbosity, latency))
			return m, waitForWSMsg(m.ws.MsgChan())

		case client.TypeLogEvent:
			m.appendEntry("", formatLogEvent(msg.Payload))
			if isErrorLevel(msg.Payload) {
				m.inFlight = false
			}
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
	return m, tea.Batch(m.spin.Tick, elapsedTick())
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
func (m *Model) appendEntry(tag, text string) {
	m.entries = append(m.entries, tag+text)
	*m = m.refreshViewport()
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
// data/config/json_schema.json's success/error document shapes. Detail
// beyond the base status line is gated by verbosity:
//
//	L1 - status/steps or error only
//	L2 - adds step parameters (or RawOutput on failure) plus the raw JSON payload
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
		if verbosity >= 2 && recipe.RawOutput != "" {
			b.WriteString("\n--- RAW OUTPUT ---\n")
			b.WriteString(recipe.RawOutput)
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
			if verbosity >= 2 && len(step.Parameters) > 0 {
				params, _ := json.Marshal(step.Parameters)
				b.WriteString(fmt.Sprintf("\n     params: %s", params))
			}
		}
	}

	if verbosity >= 2 {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, payload, "", "  "); err == nil {
			b.WriteString("\n--- RAW JSON ---\n")
			b.WriteString(pretty.String())
		}
	}

	if verbosity >= 3 {
		b.WriteString(fmt.Sprintf("\n--- METADATA ---\nLatency: %dms | Steps: %d", latency.Milliseconds(), len(recipe.Steps)))
	}

	return b.String()
}

// formatSystemInfo renders a SystemInfoMsg according to which action
// triggered the fetch (system / llm / copy).
func formatSystemInfo(msg SystemInfoMsg, use string) string {
	if msg.Err != nil {
		return errTag + "failed to fetch system info: " + msg.Err.Error()
	}
	info := msg.Info

	switch use {
	case "llm":
		return sysTag + fmt.Sprintf(
			"LLM Inference Configuration\n  Provider: %s\n  Model: %s\n  Max Tokens: %d\n  Temperature: %v",
			info.LLM.Provider, info.LLM.Model, info.LLM.MaxTokens, info.LLM.Temperature,
		)

	default: // "system"
		return sysTag + fmt.Sprintf(
			"System Operational Status\n  Bridge: %v\n  Clients Connected: %d\n  Server Port: %d\n  Proxy URL: %s",
			info.BridgeConnected, info.ClientsConnected, info.Server.Port, info.Server.ProxyURL,
		)
	}
}

// helpText builds the F1 help message, listing every active keybinding
// grouped by category.
func helpText(verbosity int) string {
	labels := map[int]string{1: "L1 - Filtered", 2: "L2 - Full Context", 3: "L3 - Debug"}
	lines := []string{
		"Key functions:",
		"",
		"  Navigation",
		"    ↑ / ↓          Recall previous prompts",
		"    PgUp / PgDn    Scroll log",
		"    Home / End     Jump to top/bottom of log",
		"",
		"  Info & Display",
		"    F1             View this help message",
		"    F2             Cycle response detail (currently: " + labels[verbosity] + ")",
		"    F3             Fetch system info",
		"    F4             Fetch LLM info",
		"    F5             Toggle telemetry sidebar",
		"",
		"  Session",
		"    Enter          Submit prompt",
		"    F6             Save session to a text file",
		"    Ctrl+C         Quit",
	}
	return strings.Join(lines, "\n")
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

func isErrorLevel(payload json.RawMessage) bool {
	var evt LogEventMsg
	if err := json.Unmarshal(payload, &evt); err != nil {
		return false
	}
	return strings.EqualFold(evt.Level, "error")
}

func contentWidth(termWidth int) int {
	return max(1, termWidth-6)
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

// connStatusText renders the connection state word ("connected" / "disconnected...") 
func connStatusText(connMsg string) string {
	if connMsg == "connected" {
		return connectedStyle.Render(connMsg)
	}
	return disconnStyle.Render(connMsg)
}

func stateLabel(state int) string {
	switch state {
	case StateBusy:
		return "BUSY"
	case StateFault:
		return "FAULT"
	default:
		return "READY"
	}
}

func stateStyle(state int) lipgloss.Style {
	switch state {
	case StateBusy:
		return nodeBusyStyle
	case StateFault:
		return nodeFaultStyle
	default:
		return nodeReadyStyle
	}
}

// renderSidebar builds the telemetry panel shown alongside the main log.
// width is the sidebar's total rendered width, height its total rendered
// height, both already accounting for border/padding via the returned style.
func renderSidebar(telemetry *MiddlewareStatus, width, height int) string {
    innerWidth := width - 4
    if innerWidth < 1 {
        innerWidth = 1
    }

    title := sidebarTitle.Width(innerWidth).Render("--- MIDDLEWARE STATUS ---")

    var b strings.Builder
    b.WriteString(title)
    b.WriteByte('\n')

    if telemetry == nil {
        b.WriteString(mutedStyle.Width(innerWidth).Render("No telemetry yet"))
    } else {
        overallStyle := stateStyle(telemetry.SummaryState).Width(innerWidth)
        b.WriteString(overallStyle.Render("System: " + stateLabel(telemetry.SummaryState)))
        b.WriteByte('\n')

        for _, n := range telemetry.IndividualStates {
            b.WriteByte('\n')
            lineStyle := stateStyle(n.State).Width(innerWidth)
            line := fmt.Sprintf("• %s: %s", n.NodeName, stateLabel(n.State))
            b.WriteString(lineStyle.Render(line))
            if n.StatusMessage != "" {
                b.WriteByte('\n')
                b.WriteString(mutedStyle.Width(innerWidth).Render("  " + n.StatusMessage))
            }
        }
    }

    return lipgloss.NewStyle().
        Width(width).
        Height(height).
        Padding(1, 1, 0, 1). // Top: 1 (matches mainCol padding), Right: 1, Bottom: 0, Left: 1
        Border(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color("#565F89")).
        Render(b.String())
}


const sidebarWidth = 34
const minWidthForSidebar = 140 //sidebar will not render if terminal width is less than this

// View renders the TUI
func (m Model) View() string {
	if !m.Ready {
		return "Initializing TUI..."
	}

	var main strings.Builder

	main.WriteString(titleStyle.Render("Embodied AI Proxy — TUI"))
	main.WriteByte('\n')

	statusLine := fmt.Sprintf(
		"Backend: %s [%s] | %s",
		m.AppServerURL, connStatusText(m.connMsg), bridgeStatusText(m.bridgeConnected),
	)
	main.WriteString(statusStyle.Render(statusLine))

	if len(m.availableObjects) > 0 {
		main.WriteByte('\n')
		main.WriteString(mutedStyle.Render("Objects: ") + lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68")).Render(strings.Join(m.availableObjects, ", ")))
	}

	main.WriteByte('\n')
	dividerWidth := m.viewport.Width
	if dividerWidth < 1 {
		dividerWidth = 1
	}
	main.WriteString(dividerStyle.Render(strings.Repeat("─", dividerWidth)))
	main.WriteByte('\n')

	main.WriteString(m.viewport.View())
	main.WriteByte('\n')

	if m.inFlight {
		main.WriteByte('\n')
		elapsed := time.Since(m.promptSentAt).Round(time.Second)
		main.WriteString(statusStyle.Render(m.spin.View() + " waiting for response... (" + elapsed.String() + ")"))
	} else {
		main.WriteByte('\n')
	}
	main.WriteByte('\n')

	main.WriteString(inputBoxStyle.Width(dividerWidth).Render(m.input.View()))
	main.WriteByte('\n')

	main.WriteString(mutedStyle.Render("(Enter to submit • F1 to view help • ctrl+c to quit)"))

	mainCol := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3B4261")).
		Render(main.String())

	if !m.showSidebar {
		return mainCol
	}

	if m.Width >= minWidthForSidebar {
		sidebar := renderSidebar(m.telemetry, sidebarWidth, m.Height)
		return lipgloss.JoinHorizontal(lipgloss.Top, mainCol, sidebar)
	}

	stackedWidth := contentWidth(m.Width) + 4
	sidebar := renderSidebar(m.telemetry, stackedWidth, 8)
	return lipgloss.JoinVertical(lipgloss.Left, mainCol, sidebar)
}