package hook

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

func testHook(cfg *config.Config, prof *profile.Profile) *Hook {
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

// ----- visibleDevices -----

func TestVisibleDevices_Set(t *testing.T) {
	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
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
	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin", "HOME=/root"},
		},
	}
	if got := h.visibleDevices(spec); got != "" {
		t.Errorf("visibleDevices = %q, want \"\"", got)
	}
}

func TestVisibleDevices_DefaultEnvAllProfile(t *testing.T) {
	prof := metaxProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"PATH=/usr/bin", "HOME=/root"},
		},
	}
	if got := h.visibleDevices(spec); got != "all" {
		t.Errorf("visibleDevices = %q, want %q", got, "all")
	}
}

func TestVisibleDevices_NilProcess(t *testing.T) {
	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
	spec := &specs.Spec{}
	if got := h.visibleDevices(spec); got != "" {
		t.Errorf("visibleDevices on nil Process = %q, want \"\"", got)
	}
}

func TestVisibleDevices_CustomEnvvar(t *testing.T) {
	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	prof := &profile.Profile{
		Device: profile.Device{
			SelectorEnvVars: []string{"MY_GPUS"},
		},
	}
	h := testHook(cfg, prof)
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
	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
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

func TestVisibleDevices_ProfileSelectorEnvVars(t *testing.T) {
	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	prof := &profile.Profile{
		Device: profile.Device{
			SelectorEnvVars: []string{"GPU_VISIBLE_DEVICES", "ALT_GPU_VISIBLE_DEVICES"},
		},
	}
	h := New(runtimeview.New(cfg, prof), logrus.New())
	spec := &specs.Spec{
		Process: &specs.Process{
			Env: []string{"ALT_GPU_VISIBLE_DEVICES=1"},
		},
	}
	if got := h.visibleDevices(spec); got != "1" {
		t.Errorf("visibleDevices with profile selector envvars = %q, want %q", got, "1")
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

	prof := defaultProfile(t)
	cfg := defaultCfg(t, prof)
	h := testHook(cfg, prof)

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

func TestArtifactTargetDir_PreservesRelativePath(t *testing.T) {
	got := artifactTargetDir("/rootfs", "/usr/local/corex", "/usr/local/corex", "/usr/local/corex/bin")
	want := filepath.Join("/rootfs", "/usr/local/corex", "bin")
	if got != want {
		t.Fatalf("artifactTargetDir = %q, want %q", got, want)
	}
}

func TestControlDevicePaths_FromProfileGlobs(t *testing.T) {
	devDir := t.TempDir()
	for _, name := range []string{"davinci_manager", "devmm_svm", "hisi_hdc"} {
		if err := os.WriteFile(filepath.Join(devDir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	prof := &profile.Profile{
		Device: profile.Device{
			ControlDeviceGlobs: []string{
				filepath.Join(devDir, "davinci_*"),
				filepath.Join(devDir, "devmm_*"),
				filepath.Join(devDir, "hisi_*"),
			},
		},
	}
	h := New(runtimeview.New(cfg, prof), logrus.New())

	paths := h.controlDevicePaths()
	if len(paths) != 3 {
		t.Fatalf("len(paths) = %d, want 3 (%v)", len(paths), paths)
	}
}

func TestInjectProfileLinker_WritesConfiguredPaths(t *testing.T) {
	rootfs := t.TempDir()
	baseProf := defaultProfile(t)
	cfg := defaultCfg(t, baseProf)
	prof := &profile.Profile{
		Inject: profile.Inject{
			ContainerRoot: "/usr/local/corex",
			Linker: profile.Linker{
				ConfigPath:  "/etc/ld.so.conf.d/accelerator-toolkit.conf",
				Paths:       []string{"/usr/local/corex/lib64", "/usr/local/corex/lib"},
				RunLdconfig: false,
			},
		},
	}
	h := New(runtimeview.New(cfg, prof), logrus.New())

	if err := h.injectProfileLinker(rootfs); err != nil {
		t.Fatalf("injectProfileLinker returned error: %v", err)
	}

	confFile := filepath.Join(rootfs, "etc", "ld.so.conf.d", "accelerator-toolkit.conf")
	data, err := os.ReadFile(confFile)
	if err != nil {
		t.Fatalf("expected linker conf file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "/usr/local/corex/lib64") {
		t.Fatalf("missing lib64 path: %q", content)
	}
	if !strings.Contains(content, "/usr/local/corex/lib") {
		t.Fatalf("missing lib path: %q", content)
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

	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
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

	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
	err := h.Run(strings.NewReader(string(stateJSON)))
	if err != nil {
		t.Fatalf("Run should succeed for non-GPU container, got: %v", err)
	}
}

func TestRun_BadStateJSON(t *testing.T) {
	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
	err := h.Run(strings.NewReader("{invalid json"))
	if err == nil {
		t.Error("Run should return error for invalid state JSON")
	}
}

func TestRun_MissingBundle(t *testing.T) {
	state := ociState{ID: "ctr", Bundle: "/nonexistent/bundle", Status: "creating"}
	stateJSON, _ := json.Marshal(state)
	prof := defaultProfile(t)
	h := testHook(defaultCfg(t, prof), prof)
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
