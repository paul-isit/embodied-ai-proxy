package main

import (
	"embodied-ai-proxy/tui/internal/app"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// setupFileLogging points the standard logger at <dataDir>/logs/tui.log only.
// Unlike shared/logging.Setup (used by the backend and proxy), it must NOT
// also write to stdout: this is a full-screen alt-screen program, and
// anything printed straight to stdout corrupts the rendered UI instead of
// scrolling normally.
func setupFileLogging(dataDir string) (*os.File, error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, "tui.log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}

	log.SetOutput(file)
	return file, nil
}

func main() {
	appServerURL := flag.String("serverURL", "http://localhost:8080", "Base URL of the App server")
	dataDir := flag.String("dataDir", "data", "Directory to write logs/tui.log under")
	flag.Parse()

	logFile, err := setupFileLogging(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up logging: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	m := app.NewModel(*appServerURL)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running the TUI: %v\n", err)
		os.Exit(1)
	}
}
