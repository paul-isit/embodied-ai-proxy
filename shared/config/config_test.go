package config

import (
	"os"
	"path/filepath"
	"testing"
)

type testProxyConfig struct {
	TimeoutSeconds int     `json:"timeout_seconds"`
	Temperature    float64 `json:"temperature"`
}

type testConfig struct {
	Port  int             `json:"port"`
	Proxy testProxyConfig `json:"proxy"`
}

func writeConfigFile(t *testing.T, dir, contents string) {
	t.Helper()
	configDir := filepath.Join(dir, ConfigDirName)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, configFileName), []byte(contents), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func TestLoad_MissingFile_ReturnsDefaults(t *testing.T) {
	defaults := testConfig{Port: 8080, Proxy: testProxyConfig{TimeoutSeconds: 30, Temperature: 0.1}}

	got, err := Load(t.TempDir(), ConfigDirName, configFileName, defaults)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if *got != defaults {
		t.Errorf("Load() = %+v, want defaults %+v", *got, defaults)
	}
}

func TestLoad_PartialFile_FillsOmittedFieldsFromDefaults(t *testing.T) {
	dir := t.TempDir()
	// Only sets the top-level port and the nested provider's timeout;
	// omits proxy.temperature entirely.
	writeConfigFile(t, dir, `{"port": 9999, "proxy": {"timeout_seconds": 45}}`)
	defaults := testConfig{Port: 8080, Proxy: testProxyConfig{TimeoutSeconds: 30, Temperature: 0.1}}

	got, err := Load(dir, ConfigDirName, configFileName, defaults)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Port != 9999 {
		t.Errorf("Port = %d, want 9999 (from file)", got.Port)
	}
	if got.Proxy.TimeoutSeconds != 45 {
		t.Errorf("Proxy.TimeoutSeconds = %d, want 45 (from file)", got.Proxy.TimeoutSeconds)
	}
	if got.Proxy.Temperature != 0.1 {
		t.Errorf("Proxy.Temperature = %v, want 0.1 (default, since the file never set it)", got.Proxy.Temperature)
	}
}

func TestLoad_EmptyObjectFile_KeepsAllDefaults(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, `{}`)
	defaults := testConfig{Port: 8080, Proxy: testProxyConfig{TimeoutSeconds: 30, Temperature: 0.1}}

	got, err := Load(dir, ConfigDirName, configFileName, defaults)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if *got != defaults {
		t.Errorf("Load() = %+v, want all defaults %+v", *got, defaults)
	}
}
