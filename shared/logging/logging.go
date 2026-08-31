// Package logging sends the standard library's log output to both stdout
// and a per-service file under <dataDir>/logs, so interaction/audit logs
// survive a restart without needing a dedicated audit-log format.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// Setup opens <dataDir>/logs/<name>.log (creating the directory if needed)
// and points the standard logger at both it and stdout. The returned Closer
// should be closed on shutdown; no log rotation is performed.
func Setup(dataDir, name string) (io.Closer, error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, name+".log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}

	log.SetOutput(io.MultiWriter(os.Stdout, file))
	log.Printf("logging to %s", path)
	return file, nil
}
