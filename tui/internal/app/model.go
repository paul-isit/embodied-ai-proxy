package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	AppServerURL string
	Width        int
	Height       int
	Ready        bool
}

// NewModel creates a new initial Model instance
func NewModel(appServerURL string) Model {
	return Model{AppServerURL: appServerURL}
}

// Init initialises the event loop and runs the startup commands
func (m Model) Init() tea.Cmd {
	return nil
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
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the TUI
func (m Model) View() string {
	if !m.Ready {
		return "Initializing TUI..."
	}
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7AA2F7")).
		Padding(1, 2)

	return style.Render("Bubble Tea TUI\nBackend: " + m.AppServerURL + "\n\nPress 'q' or 'Ctrl+C' to exit.")
}
