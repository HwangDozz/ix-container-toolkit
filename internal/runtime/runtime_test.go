package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/config"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/profile"
	"github.com/accelerator-toolkit/accelerator-toolkit/pkg/runtimeview"
)

func testRuntime(cfg *config.Config, prof *profile.Profile) *Runtime {
	log := logrus.New()
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.DebugLevel)
	return New(runtimeview.New(cfg, prof), log)
}

func defaultProfile(t *testing.T) *profile.Profile {
	t.Helper()
	prof, err := profile.Load(filepath.Join("..", "..", "profiles", "iluvatar-bi-v150.yaml"))
	if err != nil {
		t.Fatalf("profile.Load returned error: %v", err)
	}
	return prof
}

func metaxProfile(t *testing.T) *profile.Profile {
	t.Helper()
	prof, err := profile.Load(filepath.Join("..", "..", "profiles", "metax-c500.yaml"))
	if err != nil {
		t.Fatalf("profile.Load returned error: %v", err)
	}
	return prof
}

func defaultCfg(t *testing.T, prof *profile.Profile) *config.Config {
	t.Helper()
	cfg, err := config.DefaultsFromProfile(prof)
	if err != nil {
		t.Fatalf("DefaultsFromProfile returned error: %v", err)
	}
	return cfg
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
	prof := defaultProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
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
	prof := defaultProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin"},
		},
	}
	if r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = false")
	}
}

func TestContainerRequestsGPU_DefaultEnvAllProfile(t *testing.T) {
	prof := metaxProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin"},
		},
	}
	if !r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = true for env-all profile")
	}
	if got := r.visibleDevices(spec); got != "all" {
		t.Fatalf("visibleDevices = %q, want %q", got, "all")
	}
}

func TestContainerRequestsGPU_NilProcess(t *testing.T) {
	prof := defaultProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
	spec := &specs.Spec{}
	if r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = false for nil Process")
	}
}

func TestContainerRequestsGPU_EmptyEnvValue(t *testing.T) {
	prof := defaultProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
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
	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	prof := &profile.Profile{
		Device: profile.Device{
			SelectorEnvVars: []string{"MY_GPU"},
		},
	}
	r := testRuntime(cfg, prof)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"MY_GPU=all"},
		},
	}
	if !r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = true with custom envvar")
	}
}

func TestContainerRequestsGPU_ProfileSelectorEnvVars(t *testing.T) {
	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	prof := &profile.Profile{
		Device: profile.Device{
			SelectorEnvVars: []string{"GPU_VISIBLE_DEVICES", "ALT_GPU_VISIBLE_DEVICES"},
		},
	}
	log := logrus.New()
	r := New(runtimeview.New(cfg, prof), log)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"ALT_GPU_VISIBLE_DEVICES=0"},
		},
	}
	if !r.containerRequestsGPU(spec) {
		t.Error("expected containerRequestsGPU = true with profile selector env vars")
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

	prof := defaultProfile(t)
	cfg := defaultCfg(t, prof)
	cfg.HookPath = "/usr/bin/accelerator-container-hook"
	cfg.Hook.DisableRequire = true
	r := testRuntime(cfg, prof)

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

func TestInjectHook_DefaultEnvAllProfileInjectsSelectorEnv(t *testing.T) {
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin"},
		},
	}
	writeSpec(t, bundleDir, spec)

	prof := metaxProfile(t)
	cfg := defaultCfg(t, prof)
	cfg.HookPath = "/usr/bin/accelerator-container-hook"
	cfg.Hook.DisableRequire = true
	r := testRuntime(cfg, prof)

	if err := r.injectHook(bundleDir); err != nil {
		t.Fatalf("injectHook returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	if modified.Hooks == nil || len(modified.Hooks.Prestart) == 0 { //nolint:staticcheck
		t.Fatal("expected prestart hooks to be injected")
	}
	envSet := map[string]string{}
	for _, env := range modified.Process.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envSet[parts[0]] = parts[1]
		}
	}
	if envSet["METAX_VISIBLE_DEVICES"] != "all" {
		t.Fatalf("METAX_VISIBLE_DEVICES = %q, want %q", envSet["METAX_VISIBLE_DEVICES"], "all")
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

	prof := defaultProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
	if err := r.injectHook(bundleDir); err != nil {
		t.Fatalf("injectHook returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	if modified.Hooks != nil && len(modified.Hooks.Prestart) > 0 { //nolint:staticcheck
		t.Error("expected no prestart hooks for non-GPU container")
	}
}

func TestExec_DelegateOnlySkipsSpecMutation(t *testing.T) {
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices"},
		},
	}
	writeSpec(t, bundleDir, spec)

	prof := &profile.Profile{
		Runtime: profile.Runtime{InjectMode: profile.InjectModeDelegateOnly},
		Device: profile.Device{
			SelectorEnvVars: []string{"NVIDIA_VISIBLE_DEVICES"},
		},
	}
	cfg := &config.Config{
		UnderlyingRuntime: "true",
		HookPath:          "/usr/bin/accelerator-container-hook",
		LogLevel:          "debug",
	}
	r := testRuntime(cfg, prof)

	if err := r.Exec([]string{"accelerator-container-runtime", "create", "--bundle", bundleDir, "container-id"}); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	if modified.Hooks != nil && len(modified.Hooks.Prestart) > 0 { //nolint:staticcheck
		t.Fatal("delegate-only profile should not inject prestart hooks")
	}
	if got := modified.Process.Env; len(got) != 1 || got[0] != "NVIDIA_VISIBLE_DEVICES=/var/run/nvidia-container-devices" {
		t.Fatalf("Process.Env = %v, want original NVIDIA_VISIBLE_DEVICES only", got)
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

	prof := defaultProfile(t)
	cfg := defaultCfg(t, prof)
	cfg.HookPath = "/usr/bin/accelerator-container-hook"
	cfg.Hook.DisableRequire = true
	r := testRuntime(cfg, prof)

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

func TestInjectHook_InjectsProfileExtraEnv(t *testing.T) {
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES=0", "PATH=/usr/bin"},
		},
	}
	writeSpec(t, bundleDir, spec)

	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	cfg.HookPath = "/usr/bin/accelerator-container-hook"
	cfg.Hook.DisableRequire = true
	prof := &profile.Profile{
		Device: profile.Device{
			SelectorEnvVars: []string{"ILUVATAR_COREX_VISIBLE_DEVICES"},
		},
		Inject: profile.Inject{
			ExtraEnv: map[string]string{
				"LD_LIBRARY_PATH": "/usr/local/corex/lib64",
				"PATH":            "/should/not/override",
			},
		},
	}
	log := logrus.New()
	r := New(runtimeview.New(cfg, prof), log)

	if err := r.injectHook(bundleDir); err != nil {
		t.Fatalf("injectHook returned error: %v", err)
	}

	modified := readSpec(t, bundleDir)
	envSet := map[string]string{}
	for _, env := range modified.Process.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envSet[parts[0]] = parts[1]
		}
	}

	if envSet["LD_LIBRARY_PATH"] != "/usr/local/corex/lib64" {
		t.Fatalf("LD_LIBRARY_PATH = %q, want %q", envSet["LD_LIBRARY_PATH"], "/usr/local/corex/lib64")
	}
	if envSet["PATH"] != "/usr/bin" {
		t.Fatalf("PATH = %q, want original value", envSet["PATH"])
	}
}

func TestInjectHook_MissingBundle(t *testing.T) {
	prof := defaultProfile(t)
	r := testRuntime(defaultCfg(t, prof), prof)
	err := r.injectHook("/nonexistent/bundle")
	if err == nil {
		t.Error("injectHook should return error for missing bundle")
	}
}

func TestInjectLinuxDevicePath_AddsDeviceAndCgroupRule(t *testing.T) {
	spec := specs.Spec{}

	added, err := injectLinuxDevicePath(&spec, "/dev/null")
	if err != nil {
		t.Fatalf("injectLinuxDevicePath returned error: %v", err)
	}
	if !added {
		t.Fatal("expected /dev/null to be added")
	}
	if spec.Linux == nil {
		t.Fatal("expected Linux spec to be initialized")
	}
	if len(spec.Linux.Devices) != 1 {
		t.Fatalf("len(Linux.Devices) = %d, want 1", len(spec.Linux.Devices))
	}
	dev := spec.Linux.Devices[0]
	if dev.Path != "/dev/null" {
		t.Fatalf("device path = %q, want /dev/null", dev.Path)
	}
	if dev.Type != "c" {
		t.Fatalf("device type = %q, want c", dev.Type)
	}
	if spec.Linux.Resources == nil || len(spec.Linux.Resources.Devices) != 1 {
		t.Fatalf("expected one cgroup device rule, got %#v", spec.Linux.Resources)
	}
	rule := spec.Linux.Resources.Devices[0]
	if !rule.Allow || rule.Type != "c" || rule.Access != "rwm" || rule.Major == nil || rule.Minor == nil {
		t.Fatalf("unexpected cgroup device rule: %#v", rule)
	}
}

func TestInjectLinuxDevicePath_DeduplicatesByPath(t *testing.T) {
	spec := specs.Spec{}

	if _, err := injectLinuxDevicePath(&spec, "/dev/null"); err != nil {
		t.Fatalf("first injectLinuxDevicePath returned error: %v", err)
	}
	added, err := injectLinuxDevicePath(&spec, "/dev/null")
	if err != nil {
		t.Fatalf("second injectLinuxDevicePath returned error: %v", err)
	}
	if added {
		t.Fatal("expected duplicate /dev/null not to be added")
	}
	if len(spec.Linux.Devices) != 1 {
		t.Fatalf("len(Linux.Devices) = %d, want 1", len(spec.Linux.Devices))
	}
	if len(spec.Linux.Resources.Devices) != 1 {
		t.Fatalf("len(cgroup device rules) = %d, want 1", len(spec.Linux.Resources.Devices))
	}
}

func TestInjectLinuxDevicePath_RejectsRegularFile(t *testing.T) {
	spec := specs.Spec{}
	path := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(path, []byte("not a device"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := injectLinuxDevicePath(&spec, path); err == nil {
		t.Fatal("expected regular file to be rejected")
	}
}
