package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.UnderlyingRuntime != DefaultUnderlyingRuntime {
		t.Errorf("UnderlyingRuntime = %q, want %q", cfg.UnderlyingRuntime, DefaultUnderlyingRuntime)
	}
	if cfg.HookPath != DefaultHookPath {
		t.Errorf("HookPath = %q, want %q", cfg.HookPath, DefaultHookPath)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want \"info\"", cfg.LogLevel)
	}
	if cfg.Hook.DeviceListEnvvar != "ILUVATAR_VISIBLE_DEVICES" {
		t.Errorf("DeviceListEnvvar = %q, want \"ILUVATAR_VISIBLE_DEVICES\"", cfg.Hook.DeviceListEnvvar)
	}
	if cfg.Hook.ContainerDriverRoot != "/usr/local/corex" {
		t.Errorf("ContainerDriverRoot = %q, want \"/usr/local/corex\"", cfg.Hook.ContainerDriverRoot)
	}
	if len(cfg.Hook.DriverLibraryPaths) == 0 {
		t.Error("DriverLibraryPaths should not be empty")
	}
	if len(cfg.Hook.DriverBinaryPaths) == 0 {
		t.Error("DriverBinaryPaths should not be empty")
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load with nonexistent file should not error, got: %v", err)
	}
	// Should return defaults.
	defaults := Defaults()
	if cfg.UnderlyingRuntime != defaults.UnderlyingRuntime {
		t.Errorf("UnderlyingRuntime = %q, want %q", cfg.UnderlyingRuntime, defaults.UnderlyingRuntime)
	}
}

func TestLoad_EmptyPath(t *testing.T) {
	// DefaultConfigPath (/etc/ix-toolkit/config.json) almost certainly doesn't
	// exist in the test environment, so Load("") should silently return defaults.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") should not error when default config is missing, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load(\"\") returned nil config")
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	input := &Config{
		UnderlyingRuntime: "crun",
		HookPath:          "/opt/ix/hook",
		LogLevel:          "debug",
		Hook: HookConfig{
			DriverLibraryPaths:  []string{"/opt/corex/lib"},
			DriverBinaryPaths:   []string{"/opt/corex/bin"},
			ContainerDriverRoot: "/opt/corex",
			DeviceListEnvvar:    "MY_GPU_DEVICES",
		},
	}
	data, _ := json.Marshal(input)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.UnderlyingRuntime != "crun" {
		t.Errorf("UnderlyingRuntime = %q, want \"crun\"", cfg.UnderlyingRuntime)
	}
	if cfg.HookPath != "/opt/ix/hook" {
		t.Errorf("HookPath = %q, want \"/opt/ix/hook\"", cfg.HookPath)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want \"debug\"", cfg.LogLevel)
	}
	if cfg.Hook.DeviceListEnvvar != "MY_GPU_DEVICES" {
		t.Errorf("DeviceListEnvvar = %q, want \"MY_GPU_DEVICES\"", cfg.Hook.DeviceListEnvvar)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(cfgPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("Load should return error for invalid JSON")
	}
}

func TestLoad_AppliesDefaultsForEmptyFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "partial.json")

	// Write a config with only some fields set; empty fields should get defaults.
	partial := `{"logLevel": "warn"}`
	if err := os.WriteFile(cfgPath, []byte(partial), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want \"warn\"", cfg.LogLevel)
	}
	// These should be filled by applyDefaults.
	if cfg.UnderlyingRuntime != DefaultUnderlyingRuntime {
		t.Errorf("UnderlyingRuntime = %q, want %q", cfg.UnderlyingRuntime, DefaultUnderlyingRuntime)
	}
	if cfg.HookPath != DefaultHookPath {
		t.Errorf("HookPath = %q, want %q", cfg.HookPath, DefaultHookPath)
	}
	if cfg.Hook.DeviceListEnvvar != "ILUVATAR_VISIBLE_DEVICES" {
		t.Errorf("DeviceListEnvvar = %q, want \"ILUVATAR_VISIBLE_DEVICES\"", cfg.Hook.DeviceListEnvvar)
	}
}

func TestApplyDefaults_PreservesExistingValues(t *testing.T) {
	cfg := &Config{
		UnderlyingRuntime: "custom-runtime",
		HookPath:          "/my/hook",
		LogLevel:          "error",
		Hook: HookConfig{
			DeviceListEnvvar:    "CUSTOM_ENV",
			ContainerDriverRoot: "/custom/root",
		},
	}
	cfg.applyDefaults()

	if cfg.UnderlyingRuntime != "custom-runtime" {
		t.Errorf("applyDefaults should not overwrite UnderlyingRuntime, got %q", cfg.UnderlyingRuntime)
	}
	if cfg.HookPath != "/my/hook" {
		t.Errorf("applyDefaults should not overwrite HookPath, got %q", cfg.HookPath)
	}
	if cfg.Hook.DeviceListEnvvar != "CUSTOM_ENV" {
		t.Errorf("applyDefaults should not overwrite DeviceListEnvvar, got %q", cfg.Hook.DeviceListEnvvar)
	}
	if cfg.Hook.ContainerDriverRoot != "/custom/root" {
		t.Errorf("applyDefaults should not overwrite ContainerDriverRoot, got %q", cfg.Hook.ContainerDriverRoot)
	}
}
