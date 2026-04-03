package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/ix-toolkit/ix-toolkit/pkg/config"
)

func testRuntime(cfg *config.Config) *Runtime {
	log := logrus.New()
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.DebugLevel)
	return New(cfg, log)
}

func defaultCfg() *config.Config {
	return config.Defaults()
}

// ----- parseArgs -----

func TestParseArgs_CreateWithBundleSpace(t *testing.T) {
	cmd, bundle := parseArgs([]string{"create", "--bundle", "/var/run/bundle", "container-id"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/var/run/bundle" {
		t.Errorf("bundle = %q, want \"/var/run/bundle\"", bundle)
	}
}

func TestParseArgs_CreateWithBundleEquals(t *testing.T) {
	cmd, bundle := parseArgs([]string{"create", "--bundle=/path/to/bundle"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/path/to/bundle" {
		t.Errorf("bundle = %q, want \"/path/to/bundle\"", bundle)
	}
}

func TestParseArgs_CreateWithShortBundleFlag(t *testing.T) {
	cmd, bundle := parseArgs([]string{"create", "-bundle", "/path/to/bundle"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/path/to/bundle" {
		t.Errorf("bundle = %q, want \"/path/to/bundle\"", bundle)
	}
}

func TestParseArgs_CreateWithShortBundleEquals(t *testing.T) {
	cmd, bundle := parseArgs([]string{"create", "-bundle=/path"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/path" {
		t.Errorf("bundle = %q, want \"/path\"", bundle)
	}
}

func TestParseArgs_CreateWithShortBFlag(t *testing.T) {
	cmd, bundle := parseArgs([]string{"create", "-b", "/path/to/bundle"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/path/to/bundle" {
		t.Errorf("bundle = %q, want \"/path/to/bundle\"", bundle)
	}
}

func TestParseArgs_CreateWithShortBEquals(t *testing.T) {
	cmd, bundle := parseArgs([]string{"create", "-b=/path"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/path" {
		t.Errorf("bundle = %q, want \"/path\"", bundle)
	}
}

func TestParseArgs_StartCommand(t *testing.T) {
	cmd, bundle := parseArgs([]string{"start", "container-id"})
	if cmd != "start" {
		t.Errorf("cmd = %q, want \"start\"", cmd)
	}
	if bundle != "" {
		t.Errorf("bundle should be empty for start, got %q", bundle)
	}
}

func TestParseArgs_DeleteCommand(t *testing.T) {
	cmd, bundle := parseArgs([]string{"delete", "container-id"})
	if cmd != "delete" {
		t.Errorf("cmd = %q, want \"delete\"", cmd)
	}
	if bundle != "" {
		t.Errorf("bundle should be empty for delete, got %q", bundle)
	}
}

func TestParseArgs_NoSubCommand(t *testing.T) {
	cmd, bundle := parseArgs([]string{})
	if cmd != "" {
		t.Errorf("cmd should be empty, got %q", cmd)
	}
	if bundle != "" {
		t.Errorf("bundle should be empty, got %q", bundle)
	}
}

func TestParseArgs_FlagsBeforeCommand(t *testing.T) {
	// Global flags before the sub-command: first non-flag arg is the command.
	cmd, bundle := parseArgs([]string{"--root=/run/runc", "create", "--bundle=/b"})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/b" {
		t.Errorf("bundle = %q, want \"/b\"", bundle)
	}
}

func TestParseArgs_FlagsBeforeCommandWithSeparateValues(t *testing.T) {
	cmd, bundle := parseArgs([]string{
		"--root", "/run/containerd/runc/k8s.io",
		"--log", "/tmp/log.json",
		"--log-format", "json",
		"create",
		"--bundle", "/b",
		"container-id",
	})
	if cmd != "create" {
		t.Errorf("cmd = %q, want \"create\"", cmd)
	}
	if bundle != "/b" {
		t.Errorf("bundle = %q, want \"/b\"", bundle)
	}
}

// ----- containerRequestsGPU -----

func TestContainerRequestsGPU_WithEnv(t *testing.T) {
	r := testRuntime(defaultCfg())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin", "ILUVATAR_COREX_VISIBLE_DEVICES=0"},
		},
	}
	if !r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = true")
	}
}

func TestContainerRequestsGPU_WithoutEnv(t *testing.T) {
	r := testRuntime(defaultCfg())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin"},
		},
	}
	if r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = false")
	}
}

func TestContainerRequestsGPU_NilProcess(t *testing.T) {
	r := testRuntime(defaultCfg())
	spec := &specs.Spec{}
	if r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = false for nil Process")
	}
}

func TestContainerRequestsGPU_EmptyEnvValue(t *testing.T) {
	r := testRuntime(defaultCfg())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES="},
		},
	}
	// The key is present (even with empty value) → should return true.
	if !r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = true when key is set to empty string")
	}
}

func TestContainerRequestsGPU_CustomEnvvar(t *testing.T) {
	cfg := defaultCfg()
	cfg.Hook.DeviceListEnvvar = "MY_GPU"
	r := testRuntime(cfg)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"MY_GPU=all"},
		},
	}
	if !r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = true with custom envvar")
	}
}

// ----- injectHook -----

func writeSpec(t *testing.T, bundleDir string, spec specs.Spec) {
	t.Helper()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func readSpec(t *testing.T, bundleDir string) specs.Spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(bundleDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec specs.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestInjectHook_InjectsWhenGPURequested(t *testing.T) {
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES=0"},
		},
	}
	writeSpec(t, bundleDir, spec)

	cfg := defaultCfg()
	cfg.HookPath = "/usr/bin/ix-container-hook"
	r := testRuntime(cfg)

	if err := r.injectHook(bundleDir); err != nil {
		t.Fatalf("injectHook returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	if modified.Hooks == nil || len(modified.Hooks.Prestart) == 0 { //nolint:staticcheck
		t.Fatal("expected prestart hooks to be injected")
	}
	if modified.Hooks.Prestart[0].Path != cfg.HookPath { //nolint:staticcheck
		t.Errorf("hook path = %q, want %q", modified.Hooks.Prestart[0].Path, cfg.HookPath) //nolint:staticcheck
	}
}

func TestInjectHook_SkipsWhenNoGPU(t *testing.T) {
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin"},
		},
	}
	writeSpec(t, bundleDir, spec)

	r := testRuntime(defaultCfg())
	if err := r.injectHook(bundleDir); err != nil {
		t.Fatalf("injectHook returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	if modified.Hooks != nil && len(modified.Hooks.Prestart) > 0 { //nolint:staticcheck
		t.Error("expected no prestart hooks for non-GPU container")
	}
}

func TestInjectHook_PrependsBefore_ExistingHooks(t *testing.T) {
	bundleDir := t.TempDir()
	existing := specs.Hook{Path: "/usr/bin/existing-hook"}
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES=0"},
		},
		Hooks: &specs.Hooks{
			Prestart: []specs.Hook{existing}, //nolint:staticcheck
		},
	}
	writeSpec(t, bundleDir, spec)

	cfg := defaultCfg()
	cfg.HookPath = "/usr/bin/ix-container-hook"
	r := testRuntime(cfg)

	if err := r.injectHook(bundleDir); err != nil {
		t.Fatalf("injectHook returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	if len(modified.Hooks.Prestart) != 2 { //nolint:staticcheck
		t.Fatalf("expected 2 prestart hooks, got %d", len(modified.Hooks.Prestart)) //nolint:staticcheck
	}
	// ix hook must be first.
	if modified.Hooks.Prestart[0].Path != cfg.HookPath { //nolint:staticcheck
		t.Errorf("first hook = %q, want %q", modified.Hooks.Prestart[0].Path, cfg.HookPath) //nolint:staticcheck
	}
	if modified.Hooks.Prestart[1].Path != existing.Path { //nolint:staticcheck
		t.Errorf("second hook = %q, want %q", modified.Hooks.Prestart[1].Path, existing.Path) //nolint:staticcheck
	}
}

func TestInjectHook_MissingBundle(t *testing.T) {
	r := testRuntime(defaultCfg())
	err := r.injectHook("/nonexistent/bundle")
	if err == nil {
		t.Error("injectHook should return error for missing bundle")
	}
}
