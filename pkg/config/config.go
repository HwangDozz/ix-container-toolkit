// Package config defines the configuration for accelerator-toolkit components.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
)

const (
	DefaultConfigPath = "/etc/accelerator-toolkit/config.json"

	// Deprecated: legacy JSON compatibility default. Profile/runtimeview is the primary model.
	// DefaultUnderlyingRuntime is the OCI runtime that accelerator-container-runtime delegates to.
	DefaultUnderlyingRuntime = "runc"

	// Deprecated: legacy JSON compatibility default. Profile/runtimeview is the primary model.
	// DefaultHookPath is where accelerator-container-hook binary is installed.
	DefaultHookPath = "/usr/local/bin/accelerator-container-hook"

	// DefaultProfilePath is the default profile path copied to the host by accelerator-installer.
	DefaultProfilePath = "/etc/accelerator-toolkit/profiles/active.yaml"
)

var ErrProfileRequired = errors.New("active profile is required; set ACCELERATOR_PROFILE_FILE or install /etc/accelerator-toolkit/profiles/active.yaml")

// Config holds the global accelerator-toolkit configuration.
type Config struct {
	// UnderlyingRuntime is the OCI runtime that accelerator-container-runtime will delegate to.
	// Defaults to "runc".
	UnderlyingRuntime string `json:"underlyingRuntime,omitempty"`

	// HookPath is the path to the accelerator-container-hook binary.
	HookPath string `json:"hookPath,omitempty"`

	// Hook contains configuration for the container hook behavior.
	Hook HookConfig `json:"hook"`

	// LogLevel controls log verbosity: "debug", "info", "warn", "error".
	LogLevel string `json:"logLevel,omitempty"`

	// LogFile is an optional file path for logs (defaults to stderr).
	LogFile string `json:"logFile,omitempty"`
}

// HookConfig controls what the hook injects into the container.
type HookConfig struct {
	// DisableRequire controls whether to proceed even if the selector
	// environment variable is missing or no matching device is found.
	DisableRequire bool `json:"disableRequire,omitempty"`
}

// Defaults returns a Config populated with generic process-level defaults.
func Defaults() *Config {
	return &Config{
		UnderlyingRuntime: DefaultUnderlyingRuntime,
		HookPath:          DefaultHookPath,
		LogLevel:          "info",
	}
}

// Load reads the config file at path (or DefaultConfigPath) and merges it on
// top of the active profile-derived defaults.
func Load(path string) (*Config, error) {
	return LoadWithProfile(path, "")
}

// ResolveProfilePath returns the explicit profile path when provided, otherwise
// falls back to DefaultProfilePath if it exists.
func ResolveProfilePath(profilePath string) string {
	if profilePath != "" {
		return profilePath
	}
	if _, err := os.Stat(DefaultProfilePath); err == nil {
		return DefaultProfilePath
	}
	return ""
}

// LoadWithProfile reads config JSON and merges it on top of defaults derived
// from the given profile path. A readable profile is required.
func LoadWithProfile(path, profilePath string) (*Config, error) {
	profilePath = ResolveProfilePath(profilePath)
	if profilePath == "" {
		return nil, ErrProfileRequired
	}

	prof, err := profile.Load(profilePath)
	if err != nil {
		return nil, fmt.Errorf("loading profile %s: %w", profilePath, err)
	}
	return LoadWithLoadedProfile(path, prof)
}

// LoadWithLoadedProfile reads config JSON and merges it on top of process-level
// defaults derived from an already loaded profile.
func LoadWithLoadedProfile(path string, prof *profile.Profile) (*Config, error) {
	if prof == nil {
		return nil, ErrProfileRequired
	}

	cfg, err := DefaultsFromProfile(prof)
	if err != nil {
		return nil, fmt.Errorf("deriving config defaults from profile %s: %w", prof.Metadata.Name, err)
	}

	if path == "" {
		path = DefaultConfigPath
	}

	// If the config file doesn't exist, return defaults.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg.applyDefaults()
	return cfg, nil
}

// DefaultsFromProfile extracts process-level defaults from a loaded profile.
func DefaultsFromProfile(p *profile.Profile) (*Config, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}

	cfg := &Config{
		UnderlyingRuntime: p.Runtime.UnderlyingRuntime,
		HookPath:          p.Runtime.HookBinary,
		LogLevel:          "info",
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.UnderlyingRuntime == "" {
		c.UnderlyingRuntime = DefaultUnderlyingRuntime
	}
	if c.HookPath == "" {
		c.HookPath = DefaultHookPath
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
}
