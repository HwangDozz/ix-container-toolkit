package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
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
	if cfg.Hook.DisableRequire {
		t.Error("DisableRequire = true, want false")
	}
}

func TestLoad_RequiresProfile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("Load should fail when no active profile is available")
	}
	if !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("Load error = %v, want ErrProfileRequired", err)
	}
}

func TestLoad_EmptyPath_RequiresProfile(t *testing.T) {
	_, err := Load("")
	if err == nil {
		t.Fatal("Load(\"\") should fail when no active profile is available")
	}
	if !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("Load error = %v, want ErrProfileRequired", err)
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	profilePath := filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml")

	input := &Config{
		UnderlyingRuntime: "crun",
		HookPath:          "/opt/accelerator/hook",
		LogLevel:          "debug",
		Hook: HookConfig{
			DisableRequire: true,
		},
	}
	data, _ := json.Marshal(input)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithProfile(cfgPath, profilePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.UnderlyingRuntime != "crun" {
		t.Errorf("UnderlyingRuntime = %q, want \"crun\"", cfg.UnderlyingRuntime)
	}
	if cfg.HookPath != "/opt/accelerator/hook" {
		t.Errorf("HookPath = %q, want \"/opt/accelerator/hook\"", cfg.HookPath)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want \"debug\"", cfg.LogLevel)
	}
	if !cfg.Hook.DisableRequire {
		t.Error("DisableRequire = false, want true")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.json")
	profilePath := filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml")
	if err := os.WriteFile(cfgPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWithProfile(cfgPath, profilePath)
	if err == nil {
		t.Error("Load should return error for invalid JSON")
	}
}

func TestLoad_AppliesDefaultsForEmptyFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "partial.json")
	profilePath := filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml")

	// Write a config with only some fields set; empty fields should be filled
	// from the active profile-derived config.
	partial := `{"logLevel": "warn"}`
	if err := os.WriteFile(cfgPath, []byte(partial), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithProfile(cfgPath, profilePath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want \"warn\"", cfg.LogLevel)
	}
	// Generic process defaults are still filled in.
	if cfg.UnderlyingRuntime != DefaultUnderlyingRuntime {
		t.Errorf("UnderlyingRuntime = %q, want %q", cfg.UnderlyingRuntime, DefaultUnderlyingRuntime)
	}
	if cfg.HookPath != DefaultHookPath {
		t.Errorf("HookPath = %q, want %q", cfg.HookPath, DefaultHookPath)
	}
}

func TestApplyDefaults_PreservesExistingValues(t *testing.T) {
	cfg := &Config{
		UnderlyingRuntime: "custom-runtime",
		HookPath:          "/my/hook",
		LogLevel:          "error",
		Hook: HookConfig{
			DisableRequire: true,
		},
	}
	cfg.applyDefaults()

	if cfg.UnderlyingRuntime != "custom-runtime" {
		t.Errorf("applyDefaults should not overwrite UnderlyingRuntime, got %q", cfg.UnderlyingRuntime)
	}
	if cfg.HookPath != "/my/hook" {
		t.Errorf("applyDefaults should not overwrite HookPath, got %q", cfg.HookPath)
	}
	if !cfg.Hook.DisableRequire {
		t.Error("applyDefaults should preserve DisableRequire=true")
	}
}

func TestDefaultsFromProfile(t *testing.T) {
	p, err := profile.Load(filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("profile.Load returned error: %v", err)
	}

	cfg, err := DefaultsFromProfile(p)
	if err != nil {
		t.Fatalf("DefaultsFromProfile returned error: %v", err)
	}

	if cfg.UnderlyingRuntime != "runc" {
		t.Fatalf("UnderlyingRuntime = %q, want %q", cfg.UnderlyingRuntime, "runc")
	}
	if cfg.HookPath != "/usr/local/bin/accelerator-container-hook" {
		t.Fatalf("HookPath = %q, want %q", cfg.HookPath, "/usr/local/bin/accelerator-container-hook")
	}
	if cfg.Hook.DisableRequire {
		t.Fatal("DisableRequire = true, want false")
	}
}

func TestLoadWithProfile_UsesProfileAsBase(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := []byte(`{"logLevel":"debug"}`)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithProfile(cfgPath, filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("LoadWithProfile returned error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
