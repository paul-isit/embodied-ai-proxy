package app

import (
	"bytes"
	"embodied-ai-proxy/tui/internal/client"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
const fixedLines = 12


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
				params, err := json.Marshal(step.Parameters)
				if err != nil {
					b.WriteString(fmt.Sprintf("\n     params: <failed to render: %v>", err))
				} else {
					b.WriteString(fmt.Sprintf("\n     params: %s", params))
				}
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
func formatSystemInfo(msg SystemInfoMsg) string {
	if msg.Err != nil {
		return errTag + "failed to fetch system info: " + msg.Err.Error()
	}
	info := msg.Info

	switch msg.Use {
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
		"  Info & Display /commands",
		"    /help             View this help message",
		"    /filtered        Set response detail to L1 - Filtered",
		"    /context         Set response detail to L2 - Full Context",
		"    /debug           Set response detail to L3 - Debug",
		"    (currently: " + labels[verbosity] + ")",
		"    /system          Fetch system info",
		"    /llm             Fetch LLM info",
		"    /sidebar         Toggle telemetry sidebar",
		"    /save            Save session to a text file",
		"",
		"  Session",
		"    Enter          Submit prompt",
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
func renderSidebar(telemetry *MiddlewareStatus, hwLog []string, width, height int) string {
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

	b.WriteString("\n\n")
	b.WriteString(sidebarTitle.Width(innerWidth).Render("--- CURRENT EXECUTION ---"))
	if len(hwLog) == 0 {
		b.WriteByte('\n')
		b.WriteString(mutedStyle.Width(innerWidth).Render("No prompt running"))
	} else {
		for _, entry := range hwLog {
			b.WriteByte('\n')
			b.WriteString(mutedStyle.Width(innerWidth).Render(entry))
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
		"Backend: %s [%s] | %s | LLM: %s",
		m.AppServerURL, connStatusText(m.connMsg), bridgeStatusText(m.bridgeConnected), llmStatusText(m.llmProvider, m.llmModel),
	)
	main.WriteString(statusStyle.Render(statusLine))

	main.WriteByte('\n')
	if len(m.availableObjects) > 0 {
		main.WriteString(mutedStyle.Render("Objects: ") + lipgloss.NewStyle().Foreground(lipgloss.Color("#E0AF68")).Render(strings.Join(m.availableObjects, ", ")))
	} else {
		main.WriteString(mutedStyle.Render("Objects: (none discovered yet)"))
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

	main.WriteString(mutedStyle.Render("(Enter to submit • /help to view help • ctrl+c to quit)"))

	mainCol := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3B4261")).
		Render(main.String())

	if !m.showSidebar {
		return mainCol
	}

	if m.Width >= minWidthForSidebar {
		sidebar := renderSidebar(m.telemetry, m.hardwareClientLog, sidebarWidth, m.Height)
		return lipgloss.JoinHorizontal(lipgloss.Top, mainCol, sidebar)
	}

	stackedWidth := contentWidth(m.Width) + 4
	sidebar := renderSidebar(m.telemetry, m.hardwareClientLog, stackedWidth, 8)
	return lipgloss.JoinVertical(lipgloss.Left, mainCol, sidebar)
}

func llmStatusText(provider, model string) string {
	if provider == "" || model == "" {
		return mutedStyle.Render("unknown")
	}
	return provider + "/" + model
}