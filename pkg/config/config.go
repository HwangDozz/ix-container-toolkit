// Package config defines the configuration for ix-toolkit components.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultConfigPath = "/etc/ix-toolkit/config.json"

	// DefaultUnderlyingRuntime is the OCI runtime that ix-container-runtime delegates to.
	DefaultUnderlyingRuntime = "runc"

	// DefaultHookPath is where ix-container-hook binary is installed.
	DefaultHookPath = "/usr/local/bin/ix-container-hook"

	// DevicePrefix is the prefix of Iluvatar GPU device nodes.
	DevicePrefix = "/dev/iluvatar"

	// ResourceName is the Kubernetes extended resource name for Iluvatar GPU.
	ResourceName = "iluvatar.ai/gpu"
)

// Config holds the global ix-toolkit configuration.
type Config struct {
	// UnderlyingRuntime is the OCI runtime that ix-container-runtime will delegate to.
	// Defaults to "runc".
	UnderlyingRuntime string `json:"underlyingRuntime,omitempty"`

	// HookPath is the path to the ix-container-hook binary.
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
	// DriverLibraryPaths is a list of host paths containing Iluvatar driver shared libraries
	// that will be bind-mounted into the container.
	DriverLibraryPaths []string `json:"driverLibraryPaths,omitempty"`

	// DriverBinaryPaths is a list of host paths containing Iluvatar driver tools/binaries.
	DriverBinaryPaths []string `json:"driverBinaryPaths,omitempty"`

	// ContainerDriverRoot is the path inside the container where driver libs are mounted.
	ContainerDriverRoot string `json:"containerDriverRoot,omitempty"`

	// DisableRequire controls whether to proceed even if the GPU environment is missing.
	DisableRequire bool `json:"disableRequire,omitempty"`

	// DeviceListEnvvar is the environment variable that specifies which GPUs to expose.
	// Defaults to ILUVATAR_COREX_VISIBLE_DEVICES.
	DeviceListEnvvar string `json:"deviceListEnvvar,omitempty"`

	// LibraryFilterMode controls how driver library directories are mounted:
	//   "directory" — bind-mount the entire directory (legacy behavior).
	//   "so-only"   — only mount .so/.so.* shared libraries, skipping subdirectories
	//                  and static archives. This avoids mounting ~12GB of Python
	//                  packages that live under lib64/python3/.
	// Defaults to "so-only".
	LibraryFilterMode string `json:"libraryFilterMode,omitempty"`

	// LibraryExcludeDirs is a list of subdirectory names to exclude when
	// LibraryFilterMode is "so-only". Defaults to ["python3", "cmake", "clang"].
	LibraryExcludeDirs []string `json:"libraryExcludeDirs,omitempty"`
}

// Defaults returns a Config populated with sensible defaults.
func Defaults() *Config {
	return &Config{
		UnderlyingRuntime: DefaultUnderlyingRuntime,
		HookPath:          DefaultHookPath,
		LogLevel:          "info",
		Hook: HookConfig{
			DriverLibraryPaths:  []string{"/usr/local/corex/lib64", "/usr/local/corex/lib"},
			DriverBinaryPaths:   []string{"/usr/local/corex/bin"},
			ContainerDriverRoot: "/usr/local/corex",
			DeviceListEnvvar:    "ILUVATAR_COREX_VISIBLE_DEVICES",
			LibraryFilterMode:   "so-only",
			LibraryExcludeDirs:  []string{"python3", "cmake", "clang"},
		},
	}
}

// Load reads the config file at path (or DefaultConfigPath) and merges with defaults.
func Load(path string) (*Config, error) {
	cfg := Defaults()

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
	if c.Hook.DeviceListEnvvar == "" {
		c.Hook.DeviceListEnvvar = "ILUVATAR_COREX_VISIBLE_DEVICES"
	}
	if c.Hook.ContainerDriverRoot == "" {
		c.Hook.ContainerDriverRoot = "/usr/local/corex"
	}
	if c.Hook.LibraryFilterMode == "" {
		c.Hook.LibraryFilterMode = "so-only"
	}
	if len(c.Hook.LibraryExcludeDirs) == 0 {
		c.Hook.LibraryExcludeDirs = []string{"python3", "cmake", "clang"}
	}
}
