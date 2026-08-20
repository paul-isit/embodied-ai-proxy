package main

import (
	"embodied-ai-proxy/tui/internal/app"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	appServerURL := flag.String("serverURL", "http://localhost:8080", "Base URL of the App server")
	flag.Parse()

	m := app.NewModel(*appServerURL)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running the TUI: %v\n", err)
		os.Exit(1)
	}
}
