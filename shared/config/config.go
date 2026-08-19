package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads dir/subDir/fileName as JSON into a value of type T, starting
// from defaults and overlaying whatever fields the JSON sets. If the file
// does not exist (or the path is a directory), defaults is returned as-is.
// If the file exists but omits a field (or omits an entire nested section),
// that field keeps its value from defaults rather than becoming T's zero
// value - a config.json that only sets a couple of fields still gets sane
// values (e.g. a non-zero timeout) for everything else.
func Load[T any](dir, subDir, fileName string, defaults T) (*T, error) {
	path := filepath.Join(dir, subDir, fileName)
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (info != nil && info.IsDir()) {
		cfg := defaults // TODO: create the config file in the correct dir instead of just falling back to the default
		return &cfg, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %s: %w", path, err)
	}
	defer file.Close()

	cfg := defaults
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config JSON file %s: %w", path, err)
	}
	return &cfg, nil
}
