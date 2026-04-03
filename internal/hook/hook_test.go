package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/ix-toolkit/ix-toolkit/pkg/config"
)

func testHook(cfg *config.Config) *Hook {
	log := logrus.New()
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.DebugLevel)
	return New(cfg, log)
}

func defaultCfg() *config.Config {
	return config.Defaults()
}

// ----- visibleDevices -----

func TestVisibleDevices_Set(t *testing.T) {
	h := testHook(defaultCfg())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES=0,1"},
		},
	}
	if got := h.visibleDevices(spec); got != "0,1" {
		t.Errorf("visibleDevices = %q, want \"0,1\"", got)
	}
}

func TestVisibleDevices_NotSet(t *testing.T) {
	h := testHook(defaultCfg())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin", "HOME=/root"},
		},
	}
	if got := h.visibleDevices(spec); got != "" {
		t.Errorf("visibleDevices = %q, want \"\"", got)
	}
}

func TestVisibleDevices_NilProcess(t *testing.T) {
	h := testHook(defaultCfg())
	spec := &specs.Spec{}
	if got := h.visibleDevices(spec); got != "" {
		t.Errorf("visibleDevices on nil Process = %q, want \"\"", got)
	}
}

func TestVisibleDevices_CustomEnvvar(t *testing.T) {
	cfg := defaultCfg()
	cfg.Hook.DeviceListEnvvar = "MY_GPUS"
	h := testHook(cfg)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"MY_GPUS=all", "ILUVATAR_COREX_VISIBLE_DEVICES=none"},
		},
	}
	if got := h.visibleDevices(spec); got != "all" {
		t.Errorf("visibleDevices with custom envvar = %q, want \"all\"", got)
	}
}

func TestVisibleDevices_EmptyValue(t *testing.T) {
	h := testHook(defaultCfg())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES="},
		},
	}
	// The variable IS set but to empty string.
	if got := h.visibleDevices(spec); got != "" {
		t.Errorf("visibleDevices empty value = %q, want \"\"", got)
	}
}

// ----- injectLdSoConf -----

func TestInjectLdSoConf_CreatesFile(t *testing.T) {
	rootfs := t.TempDir()
	cfg := defaultCfg()
	cfg.Hook.ContainerDriverRoot = "/usr/local/corex"
	cfg.Hook.DriverLibraryPaths = []string{"/usr/local/corex/lib64", "/usr/local/corex/lib"}
	h := testHook(cfg)

	if err := h.injectLdSoConf(rootfs, cfg.Hook.ContainerDriverRoot); err != nil {
		t.Fatalf("injectLdSoConf returned error: %v", err)
	}

	confFile := filepath.Join(rootfs, "etc", "ld.so.conf.d", "ix-toolkit.conf")
	data, err := os.ReadFile(confFile)
	if err != nil {
		t.Fatalf("conf file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "/usr/local/corex/lib64") {
		t.Errorf("conf file missing lib64 path, content: %q", content)
	}
	if !strings.Contains(content, "/usr/local/corex/lib") {
		t.Errorf("conf file missing lib path, content: %q", content)
	}
	if !strings.HasSuffix(content, "\n") {
		t.Error("conf file should end with newline")
	}
}

func TestInjectLdSoConf_IdempotentOverwrite(t *testing.T) {
	rootfs := t.TempDir()
	cfg := defaultCfg()
	h := testHook(cfg)

	// Call twice — should succeed both times and produce same content.
	if err := h.injectLdSoConf(rootfs, cfg.Hook.ContainerDriverRoot); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if err := h.injectLdSoConf(rootfs, cfg.Hook.ContainerDriverRoot); err != nil {
		t.Fatalf("second call error: %v", err)
	}
}

func TestInjectLdSoConf_EmptyDriverPaths(t *testing.T) {
	rootfs := t.TempDir()
	cfg := defaultCfg()
	cfg.Hook.DriverLibraryPaths = nil
	h := testHook(cfg)

	if err := h.injectLdSoConf(rootfs, cfg.Hook.ContainerDriverRoot); err != nil {
		t.Fatalf("injectLdSoConf with no paths returned error: %v", err)
	}

	confFile := filepath.Join(rootfs, "etc", "ld.so.conf.d", "ix-toolkit.conf")
	data, err := os.ReadFile(confFile)
	if err != nil {
		t.Fatalf("conf file not created: %v", err)
	}
	// Only the trailing newline.
	if string(data) != "\n" {
		t.Errorf("unexpected content for empty paths: %q", string(data))
	}
}

func TestUniquePreserveOrder_RemovesDuplicates(t *testing.T) {
	in := []string{"/usr/local/corex-4.3.0/lib64", "/usr/local/corex-4.3.0/lib64", "/usr/local/corex-4.3.0/bin"}
	got := uniquePreserveOrder(in)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != "/usr/local/corex-4.3.0/lib64" {
		t.Fatalf("got[0] = %q", got[0])
	}
	if got[1] != "/usr/local/corex-4.3.0/bin" {
		t.Fatalf("got[1] = %q", got[1])
	}
}

func TestEnsureLibrarySymlink_CreatesAlias(t *testing.T) {
	rootfs := t.TempDir()
	hostRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hostRoot, "lib64"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("lib64", filepath.Join(hostRoot, "lib")); err != nil {
		t.Fatal(err)
	}

	cfg := defaultCfg()
	cfg.Hook.ContainerDriverRoot = hostRoot
	h := testHook(cfg)

	if err := h.ensureLibrarySymlink(rootfs, hostRoot); err != nil {
		t.Fatalf("ensureLibrarySymlink returned error: %v", err)
	}

	linkPath := filepath.Join(rootfs, hostRoot, "lib")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", linkPath, err)
	}
	if target != "lib64" {
		t.Fatalf("symlink target = %q, want %q", target, "lib64")
	}
}

// ----- Run: none value skips without error -----

func TestRun_NoneValue_Skips(t *testing.T) {
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"ILUVATAR_COREX_VISIBLE_DEVICES=none"},
		},
	}
	writeSpec(t, bundleDir, spec)

	state := ociState{ID: "test-ctr-none", Bundle: bundleDir, Status: "creating"}
	stateJSON, _ := json.Marshal(state)

	h := testHook(defaultCfg())
	err := h.Run(strings.NewReader(string(stateJSON)))
	if err != nil {
		t.Fatalf("Run should succeed for ILUVATAR_COREX_VISIBLE_DEVICES=none, got: %v", err)
	}
	// rootfs/dev should be empty — no devices injected.
	entries, _ := os.ReadDir(filepath.Join(bundleDir, "rootfs", "dev"))
	if len(entries) != 0 {
		t.Errorf("expected empty dev dir for none, got %d entries", len(entries))
	}
}

// ----- Run: no GPU env -----

func TestRun_NoGPUEnv_Skips(t *testing.T) {
	// Build a minimal bundle with a config.json that has no ILUVATAR_COREX_VISIBLE_DEVICES.
	bundleDir := t.TempDir()
	spec := specs.Spec{
		Version: "1.0.0",
		Root:    &specs.Root{Path: "rootfs"},
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin"},
		},
	}
	writeSpec(t, bundleDir, spec)

	state := ociState{ID: "test-ctr", Bundle: bundleDir, Status: "creating"}
	stateJSON, _ := json.Marshal(state)

	h := testHook(defaultCfg())
	err := h.Run(strings.NewReader(string(stateJSON)))
	if err != nil {
		t.Fatalf("Run should succeed for non-GPU container, got: %v", err)
	}
}

func TestRun_BadStateJSON(t *testing.T) {
	h := testHook(defaultCfg())
	err := h.Run(strings.NewReader("{invalid json"))
	if err == nil {
		t.Error("Run should return error for invalid state JSON")
	}
}

func TestRun_MissingBundle(t *testing.T) {
	state := ociState{ID: "ctr", Bundle: "/nonexistent/bundle", Status: "creating"}
	stateJSON, _ := json.Marshal(state)
	h := testHook(defaultCfg())
	err := h.Run(strings.NewReader(string(stateJSON)))
	if err == nil {
		t.Error("Run should return error when bundle doesn't exist")
	}
}

// helper

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
